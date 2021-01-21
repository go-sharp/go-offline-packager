package main

import (
	"github.com/go-sharp/color"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

type PackCmd struct {
	Module  []string `short:"m" long:"module" description:"Modules to pack (github.com/jessevdk/go-flags or github.com/jessevdk/go-flags@v1.4.0)"`
	ModFile string   `short:"g" long:"go-mod-file" description:"Pack all dependencies specified in go.mod file."`
	Output  string   `short:"o" long:"out" description:"Output file name of the zip archive." default:"gop_dependencies.zip"`
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
	if err := os.Mkdir(modCache, 0777); err != nil {
		log.Fatalf("%v: failed to create mod cache directory: %v\n", color.RedString("error"), err)
	}

	if p.ModFile != "" {
		verboseF("copying go.mod file\n")
		modContent, err := ioutil.ReadFile(p.ModFile)
		if err != nil {
			log.Fatalf("failed to copy go.mod file: %v\n", color.RedString(err.Error()))
		}
		if err := ioutil.WriteFile(filepath.Join(workDir, "go.mod"), modContent, 0666); err != nil {
			log.Fatalf("failed to copy go.mod file: %v\n", color.RedString(err.Error()))
		}
	} else {
		verboseF("processing modules\n")
		if err := ioutil.WriteFile(filepath.Join(workDir, "go.mod"), []byte(gomodTemp), 0666); err != nil {
			log.Fatalf("failed to write go.mod file: %v\n", color.RedString(err.Error()))
		}

		for _, m := range p.Module {
			verboseF("adding module: %v\n", color.BlueString(m))
			cmd := exec.Command(commonOpts.GoBinPath, "get", m)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "GOMODCACHE="+modCache)
			if output, err := cmd.CombinedOutput(); err != nil {
				log.Printf("failed to add module: %v\n", color.RedString(m))
				verboseF("%v: \n%s", color.RedString("error"), output)
			}
		}
	}

	log.Println("download all dependencies")
	cmd := exec.Command(commonOpts.GoBinPath, "mod", "download", "all")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "GOMODCACHE="+modCache)
	if  err := cmd.Run(); err != nil {
		log.Fatalln("failed to download dependencies:", color.RedString(err.Error()))
	}

	log.Println("creating archive")
	if err := createZipArchive(modCache, p.Output); err != nil {
		log.Fatalln("failed to create zip archive with dependencies:", color.RedString(err.Error()))
	}
	log.Println("archive created:", color.GreenString(p.Output))
	return nil
}
