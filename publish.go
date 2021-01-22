package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/go-sharp/color"
)

type publishCmd struct {
	PosArgs struct {
		Archive string `positional-arg-name:"ARCHIVE" description:"Path to archive with dependencies. " default:"gop_dependencies.zip"`
	} `positional-args:"yes" required:"1"`
}

type JFrogPublishCmd struct {
	publishCmd
	JFrogBinPath string `long:"jfrog-bin" env:"GOP_JFROG_BIN" description:"Set full path to the jfrog-cli binary"`
	Repo         string `short:"r" long:"repo" required:"yes" description:"Artifactory go repository name ex. go-local."`
}

// Execute will be called for the last active (sub)command. The
// args argument contains the remaining command line arguments. The
// error that Execute returns will be eventually passed out of the
// Parse method of the Parser.
func (j *JFrogPublishCmd) Execute(args []string) error {
	log.SetPrefix("Publish-JFrog: ")
	if j.JFrogBinPath == "" {
		if p, err := exec.LookPath("jfrog"); err == nil {
			if !filepath.IsAbs(p) {
				p, _ = filepath.Abs(p)
			}
			j.JFrogBinPath = p
		}
	}

	if j.JFrogBinPath == "" {
		log.Fatalln(errorRedPrefix, "missing jfrog cli: install jfrog-cli or specify valid binary path with --jfrog-bin")
	}

	cfg := j.getJFrogCfg()
	if len(cfg) == 0 {
		log.Fatalln(errorRedPrefix, "jfrog is not configured")
	}

	// Print config used
	for _, i := range cfg {
		log.Println("config:", color.BlueString(i))
	}

	workDir, cleanFn := createTempWorkDir()
	defer cleanFn()

	log.Println("extracting archive")
	if err := extractZipArchive(j.PosArgs.Archive, workDir); err != nil {
		log.Fatalln(errorRedPrefix, " failed to extract archive:", err)
	}

	workCh := make(chan string, 10)
	doneCh := make(chan struct{})
	go func() {
		for mod := range workCh {
			pkg := strings.Split(filepath.Base(mod), "@")
			if len(pkg) != 2 {
				log.Println(color.YellowString("warning:"), "invalid module directory:", filepath.Base(mod))
				continue
			}

			goModF := filepath.Join(mod, "go.mod")
			if _, err := os.Stat(goModF); errors.Is(err, os.ErrNotExist) {
				modName := filepath.Dir(strings.TrimPrefix(mod, workDir+string(filepath.Separator)))
				modName = strToModuleName(modName + "/" + pkg[0])
				if err := ioutil.WriteFile(goModF, []byte(fmt.Sprintf("module %v\n", modName)), 0664); err != nil {
					verboseF("%v: %v\n", errorRedPrefix, err)
				}
			}

			cmd := exec.Command(j.JFrogBinPath, "rt", "gp", j.Repo, pkg[1])
			cmd.Dir = mod

			verboseF("publishing module %v %v\n", color.BlueString(pkg[0]), color.BlueString(pkg[1]))
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Println(errorRedPrefix, "failed publish module:", pkg[0], pkg[1], err)
				if len(output) > 0 {
					verboseF("%v\n%v", errorRedPrefix, string(output))
				}
				continue
			}
		}
		doneCh <- struct{}{}
	}()

	log.Println("publishing modules")
	filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasPrefix(info.Name(), "cache") {
			return filepath.SkipDir
		}

		if !info.IsDir() || !strings.Contains(info.Name(), "@") {
			return nil
		}
		workCh <- path
		return filepath.SkipDir
	})
	close(workCh)

	<-doneCh

	log.Println("modules successfully uploaded")
	return nil
}

func (j JFrogPublishCmd) getJFrogCfg() (config []string) {
	data, err := exec.Command(j.JFrogBinPath, "rt", "c", "show").Output()
	if err != nil {
		log.Fatalln(errorRedPrefix, "failed to get jfrog config:", err)
	}

	for _, v := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.ToLower(v), "server") || strings.HasPrefix(strings.ToLower(v), "url") ||
			strings.HasPrefix(strings.ToLower(v), "user") {
			config = append(config, v)
		}
	}

	return config
}

// FolderPublishCmd publishes an archive of modules to a folder.
type FolderPublishCmd struct {
	publishCmd
	Output string `short:"o" long:"out" required:"yes" description:"Output folder for the archive."`
}

func (f FolderPublishCmd) Execute(args []string) error {
	log.SetPrefix("Publish-Folder: ")

	workDir, cleanFn := createTempWorkDir()
	defer cleanFn()

	log.Println("extracting archive")

	defaultErrStr := errorRedPrefix + " failed to extract archive:"
	if err := extractZipArchive(f.PosArgs.Archive, workDir); err != nil {
		log.Fatalln(defaultErrStr, err)
	}

	// Prepare output folder
	fi, err := os.Stat(f.Output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalln(defaultErrStr, err)
		}
		if err := os.MkdirAll(f.Output, 0774); err != nil {
			log.Fatalln(defaultErrStr, err)
		}
	} else if !fi.IsDir() {
		log.Fatalln(errorRedPrefix, "output is not a directory:", f.Output)
	}

	log.Println("processing files")
	dirPrefix := filepath.Join(workDir, "cache", "download")
	var wg sync.WaitGroup
	err = filepath.Walk(dirPrefix, func(path string, info os.FileInfo, err error) error {
		relPath := strings.TrimLeft(strings.TrimPrefix(path, dirPrefix), string(filepath.Separator))

		if strings.HasPrefix(relPath, "sumdb") && !info.IsDir() {
			wg.Add(1)
			go func() {
				f.handleCopyFile(path, relPath)
				wg.Done()
			}()
			return nil
		}

		if info.IsDir() && strings.HasSuffix(relPath, "@v") {
			wg.Add(1)
			go func() {
				f.handleModule(path, dirPrefix)
				wg.Done()
			}()
			return filepath.SkipDir
		}

		return nil
	})

	if err != nil {
		log.Println(errorRedPrefix, err)
	}
	wg.Wait()

	ppath, _ := filepath.Abs(f.Output)
	log.Println("published archive to:", color.GreenString(ppath))
	log.Printf("hint: set GOPROXY to use folder for dependencies:\n\t%v\n", color.BlueString("go env -w GOPROXY=file:///%v", ppath))
	log.Printf("hint: in an air-gapped env set GOSUMDB to of:\n\t%v\n", color.BlueString("go env -w GOSUMDB=off"))
	return nil
}

func (f FolderPublishCmd) handleModule(path, prefix string) {
	modD, err := os.Open(path)
	if err != nil {
		log.Println(errorRedPrefix, "failed to read module directory: ", err)
		return
	}
	defer modD.Close()

	files, err := modD.Readdirnames(0)
	if err != nil {
		log.Println(errorRedPrefix, "failed to read module directory: ", err)
		return
	}

	// Copy files
	for _, fi := range files {
		if fi == "list" || fi == "list.lock" || fi == "lock" {
			continue
		}

		srcF := filepath.Join(path, fi)
		f.handleCopyFile(srcF, strings.TrimLeft(strings.TrimPrefix(srcF, prefix), string(filepath.Separator)))
	}

	var version []string
	dstPath := filepath.Join(f.Output, strings.TrimLeft(strings.TrimPrefix(path, prefix), string(filepath.Separator)))
	dstF, err := os.Open(dstPath)
	if err != nil {
		log.Println(errorRedPrefix, "failed to update list file: ", err)
		return
	}
	defer dstF.Close()

	modules, err := dstF.Readdirnames(0)
	if err != nil {
		log.Println(errorRedPrefix, "failed to update list file: ", err)
		return
	}

	for _, v := range modules {
		if strings.HasSuffix(v, ".mod") {
			version = append(version, strings.TrimSuffix(v, ".mod"))
		}
	}

	content := []byte(strings.Join(version, "\n"))
	content = append(content, '\n')
	if err := ioutil.WriteFile(filepath.Join(dstPath, "list"), content, 0664); err != nil {
		log.Println(errorRedPrefix, "failed to update list file: ", err)
		return
	}
}

func (f FolderPublishCmd) handleCopyFile(path, relPath string) {
	dstPath := filepath.Join(f.Output, relPath)
	if _, err := os.Stat(dstPath); !errors.Is(err, os.ErrNotExist) {
		reason := "file exists"
		if err != nil {
			reason = err.Error()
		}
		verboseF("skipping file %v: %v\n", color.YellowString(relPath), reason)
		return
	}

	dstDir := filepath.Dir(dstPath)
	if st, err := os.Stat(dstDir); errors.Is(err, os.ErrNotExist) {
		// We don't care if we can't create dir, it will fail when we try to copy the file
		_ = os.MkdirAll(dstDir, 0774)
	} else if !st.IsDir() {
		log.Println(errorRedPrefix, "failed to copy file destination is not a directory: ", dstDir)
		return
	}

	srcF, err := os.Open(path)
	if err != nil {
		log.Println(errorRedPrefix, "failed to read src:", err)
		return
	}
	defer srcF.Close()

	dstF, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0664)
	if err != nil {
		log.Println(errorRedPrefix, "failed to create file:", err)
		return
	}
	defer dstF.Close()

	if _, err := io.Copy(dstF, srcF); err != nil {

		log.Println(errorRedPrefix, "failed to copy file:", err)
		return
	}
}

func strToModuleName(name string) string {
	name = filepath.ToSlash(name)
	var modName []rune

	nextToUpper := false
	for _, v := range name {
		if nextToUpper {
			modName = append(modName, unicode.ToUpper(v))
			nextToUpper = false
			continue
		}

		if v == '!' {
			nextToUpper = true
			continue
		}
		modName = append(modName, v)
	}

	return string(modName)
}
