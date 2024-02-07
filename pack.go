package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-sharp/color"
)

type PackCmd struct {
	Module       []string `short:"m" long:"module" description:"Modules to pack (github.com/jessevdk/go-flags or github.com/jessevdk/go-flags@v1.4.0)"`
	ModFile      string   `short:"g" long:"go-mod-file" description:"Pack all dependencies specified in go.mod file."`
	Output       string   `short:"o" long:"out" description:"Output file name of the zip archive." default:"gop_dependencies.zip"`
	DoTransitive bool     `short:"t" long:"transitive" description:"Ensure all transitive dependencies are included."`
}

// Execute will be called for the last active (sub)command. The
// args argument contains the remaining command line arguments. The
// error that Execute returns will be eventually passed out of the
// Parse method of the Parser.
func (p *PackCmd) Execute(args []string) error {
	log.SetPrefix("Packaging: ")
	checkGo()
	if len(p.Module) == 0 && p.ModFile == "" {
		log.Fatalln(color.RedString("failed:"), "either modul or go.mod file required")
	}
	log.Println("prepare dependencies")

	workDir, cleanFn := createTempWorkDir()
	defer cleanFn()

	modCache := filepath.Join(workDir, "modcache")
	if err := os.Mkdir(modCache, 0774); err != nil {
		log.Fatalf("%v: failed to create mod cache directory: %v\n", color.RedString("error"), err)
	}

	if p.ModFile != "" {
		verboseF("copying go.mod file\n")
		modContent, err := os.ReadFile(p.ModFile)
		if err != nil {
			log.Fatalf("failed to copy go.mod file: %v\n", color.RedString(err.Error()))
		}
		if err := os.WriteFile(filepath.Join(workDir, "go.mod"), modContent, 0664); err != nil {
			log.Fatalf("failed to copy go.mod file: %v\n", color.RedString(err.Error()))
		}
	} else {
		verboseF("processing modules\n")
		if err := os.WriteFile(filepath.Join(workDir, "go.mod"), []byte(gomodTemp), 0664); err != nil {
			log.Fatalf("failed to write go.mod file: %v\n", color.RedString(err.Error()))
		}

		for _, m := range p.Module {
			verboseF("adding module: %v\n", color.BlueString(m))
			if output, err := getGoCommand(workDir, modCache, "get", m).CombinedOutput(); err != nil {
				log.Printf("failed to add module: %v\n", color.RedString(m))
				verboseF("%v: \n%s", color.RedString("error"), output)
			}
		}

	}

	cmdArgs := []string{"mod", "download"}
	if p.DoTransitive {
		p.addTransitive(workDir, modCache)
		cmdArgs = append(cmdArgs, "all")
	}

	log.Println("download all dependencies")
	if err := getGoCommand(workDir, modCache, cmdArgs...).Run(); err != nil {
		log.Fatalln("failed to download dependencies:", color.RedString(err.Error()))

	}

	log.Println("creating archive")
	if err := createZipArchive(modCache, p.Output); err != nil {
		log.Fatalln("failed to create zip archive with dependencies:", color.RedString(err.Error()))
	}
	log.Println("archive created:", color.GreenString(p.Output))
	return nil
}

func (p *PackCmd) addTransitive(workDir, modCache string) {
	hasMore := false
	modSet := map[string]struct{}{}

	for {
		output, err := getGoCommand(workDir, modCache, "mod", "graph").Output()
		if err != nil {
			log.Println("failed to add transitive dependencies:", color.RedString(err.Error()))
			return
		}

		deps := strings.Split(string(output), "\n")
		if len(deps) == 0 {
			return
		}

		for _, dep := range deps {
			mods := strings.Split(dep, " ")
			mod := strings.Trim(mods[len(mods)-1], " ")

			if _, exists := modSet[mod]; exists || mod == "" || folderExists(filepath.Join(modCache, moduleNameToCaseInsensitive(mod))) {
				continue
			}

			modSet[mod] = struct{}{}
			verboseF("adding transitive module: %v\n", color.BlueString(mod))
			if output, err := getGoCommand(workDir, modCache, "get", mod).CombinedOutput(); err != nil {
				log.Printf("failed to add module: %v\n", color.RedString(mod))
				verboseF("%v: \n%s", color.RedString("error"), output)
			}
			hasMore = true
		}

		if hasMore {
			hasMore = false
			continue
		}
		break
	}

}

func getGoCommand(workDir, modCache string, args ...string) *exec.Cmd {
	cmd := exec.Command(commonOpts.GoBinPath, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GOMODCACHE="+modCache)

	return cmd
}

func folderExists(name string) bool {
	if _, err := os.Stat(name); errors.Is(err, os.ErrNotExist) {
		return false
	}

	return true
}

func moduleNameToCaseInsensitive(name string) string {
	name = filepath.ToSlash(name)
	var modName []rune

	for _, v := range name {
		if unicode.IsUpper(v) {
			modName = append(modName, '!', unicode.ToLower(v))
			continue
		}

		modName = append(modName, v)
	}

	return string(modName)
}
