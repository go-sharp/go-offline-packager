package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-sharp/color"
)

type PackV2Cmd struct {
	Module       []string `short:"m" long:"module" description:"Modules to pack (github.com/jessevdk/go-flags or github.com/jessevdk/go-flags@v1.4.0)"`
	ModFile      string   `short:"g" long:"go-mod-file" description:"Pack all dependencies specified in go.mod file."`
	Output       string   `short:"o" long:"out" description:"Output file name of the zip archive." default:"gop_dependencies.zip"`
	DoTransitive bool     `short:"t" long:"transitive" description:"Ensure all transitive dependencies are included."`

	workDir       string
	modCache      string
	cleanFn       func()
	transitiveMod map[string]struct{}
	excludeMods   []string
}

// Execute will be called for the last active (sub)command. The
// args argument contains the remaining command line arguments. The
// error that Execute returns will be eventually passed out of the
// Parse method of the Parser.
func (p *PackV2Cmd) Execute(args []string) error {
	log.SetPrefix("Packaging: ")
	p.InitCommand()
	defer p.cleanFn()

	log.Println("prepare dependencies")

	if p.ModFile != "" {
		p.downloadDepsForModFile()
	} else {
		p.downloadModules()

	}

	log.Println("creating archive")
	if err := createZipArchive(p.modCache, p.Output); err != nil {
		log.Fatalln("failed to create zip archive with dependencies:", color.RedString(err.Error()))
	}

	log.Println("archive created:", color.GreenString(p.Output))
	return nil
}

func (p *PackV2Cmd) downloadModules2() {
	verboseF("processing modules\n")
	if err := os.WriteFile(filepath.Join(p.workDir, "go.mod"), []byte(gomodTemp), 0664); err != nil {
		log.Fatalf("failed to write go.mod file: %v\n", color.RedString(err.Error()))
	}

	for _, m := range p.Module {

		verboseF("downloading modules for: %v\n", color.BlueString(m))
		if output, err := getGoCommand(p.workDir, p.modCache, "get", m).CombinedOutput(); err != nil {
			log.Printf("failed to add module: %v\n", color.RedString(m))
			verboseF("%v: %v \n", color.RedString("error"), color.RedString(string(output)))
		}

	}

	// Ensure all dependencies are in cache
	// verboseF("ensure transitive modules are in cache\n")
	// if output, err := getGoCommand(p.workDir, p.modCache, "mod", "download", "-x").CombinedOutput(); err != nil {
	// 	verboseF("%v: %v \n", color.RedString("error"), color.RedString(string(output)))
	// }

}

func (p *PackV2Cmd) downloadModules() {
	verboseF("processing modules\n")
	if err := os.WriteFile(filepath.Join(p.workDir, "go.mod"), []byte(gomodTemp), 0664); err != nil {
		log.Fatalf("failed to write go.mod file: %v\n", color.RedString(err.Error()))
	}

	for _, m := range p.Module {
		m = versionizeModulName(m)

		verboseF("downloading module: %v\n", color.BlueString(m))
		output, _ := getGoCommand(p.workDir, p.modCache, "mod", "download", "-json", m).CombinedOutput()

		var modItem Module
		if err := json.Unmarshal(output, &modItem); err != nil || modItem.Error != "" {
			log.Printf("failed to add module: %v\n", color.RedString(m))
			verboseF("%v: %v \n", color.RedString("error"), color.RedString(getErrorStr(err, modItem)))
			continue
		}

		if p.DoTransitive {
			p.addTransitiveDeps(modItem)
		}
	}

	if len(p.transitiveMod) > 0 {
		producer := make(chan string)
		go func() {
			defer close(producer)
			for m, _ := range p.transitiveMod {
				producer <- m
			}
			// for m, _ := range p.transitiveMod {
			// 	verboseF("downloading transitive module: %v\n", color.BlueString(m))
			// 	if output, err := getGoCommand(p.workDir, p.modCache, "mod", "download", m).CombinedOutput(); err != nil {
			// 		log.Printf("failed to add module: %v\n", color.RedString(m))
			// 		verboseF("%v: %v \n", color.RedString("error"), color.RedString(string(output)))
			// 	}
			// }
		}()

		reporterCh := make(chan func())

		go func() {
			for fn := range reporterCh {
				fn()
			}
		}()

		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for m := range producer {
					reporterCh <- func() { verboseF("downloading transitive module: %v\n", color.BlueString(m)) }
					if output, err := getGoCommand(p.workDir, p.modCache, "mod", "download", m).CombinedOutput(); err != nil {
						reporterCh <- func() {
							log.Printf("failed to add module: %v\n", color.RedString(m))
							verboseF("%v: %v \n", color.RedString("error"), color.RedString(string(output)))
						}
					}
				}

			}()
		}

		wg.Wait()
		defer close(reporterCh)
	}

	// for m, _ := range p.transitiveMod {
	// 	verboseF("downloading transitive module: %v\n", color.BlueString(m))
	// 	if output, err := getGoCommand(p.workDir, p.modCache, "mod", "download", m).CombinedOutput(); err != nil {
	// 		log.Printf("failed to add module: %v\n", color.RedString(m))
	// 		verboseF("%v: %v \n", color.RedString("error"), color.RedString(string(output)))
	// 	}
	// }
}

func (p *PackV2Cmd) addTransitiveDeps(modItem Module) {
	output, err := getGoCommand(modItem.Dir, p.modCache, "mod", "graph").CombinedOutput()
	if err != nil {
		log.Printf("failed to get dependencies for module '%v@%v: %v\n", color.BlueString(modItem.Path), color.BlueString(modItem.Version), color.RedString(err.Error()))
		return
	}

	reader := bufio.NewScanner(bytes.NewReader(output))

	for reader.Scan() {
		parts := strings.Split(reader.Text(), " ")
		if len(parts) == 2 && !p.isExcludedModule(parts[1]) {
			verboseF("adding transitive module: %v\n", color.BlueString(parts[1]))
			p.transitiveMod[parts[1]] = struct{}{}
		}
	}
}

func (p *PackV2Cmd) isExcludedModule(s string) bool {
	for _, prefix := range p.excludeMods {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}

	return false
}

// versionizeModulName checks if a version is present and adds 'latest' if missing.
func versionizeModulName(modName string) string {
	if strings.Contains(modName, "@") {
		return modName
	}

	return fmt.Sprintf("%s@latest", modName)
}

func getErrorStr(e error, modItem Module) string {
	if e != nil {
		return e.Error()
	}

	return modItem.Error
}

func (p *PackV2Cmd) downloadDepsForModFile() {
	log.Println("downloading dependencies for mod file:", color.BlueString(p.ModFile))
	verboseF("copying go.mod file\n")
	modContent, err := os.ReadFile(p.ModFile)
	if err != nil {
		log.Fatalf("failed to copy go.mod file: %v\n", color.RedString(err.Error()))
	}

	if err := os.WriteFile(filepath.Join(p.workDir, "go.mod"), modContent, 0664); err != nil {
		log.Fatalf("failed to copy go.mod file: %v\n", color.RedString(err.Error()))
	}

	cmdArgs := []string{"mod", "download"}

	verboseF("download all dependencies\n")
	if err := getGoCommand(p.workDir, p.modCache, cmdArgs...).Run(); err != nil {
		log.Fatalln("failed to download dependencies:", color.RedString(err.Error()))

	}
	verboseF("successfully downloaded all dependencies\n")
}

func (p *PackV2Cmd) InitCommand() {
	checkGo()
	if len(p.Module) == 0 && p.ModFile == "" {
		log.Fatalln(color.RedString("failed:"), "either modul or go.mod file required")
	}

	p.transitiveMod = map[string]struct{}{}
	p.excludeMods = []string{
		"go@",
		"toolchain@",
	}

	p.workDir, p.cleanFn = createTempWorkDir()

	p.modCache = filepath.Join(p.workDir, "modcache")
	if err := os.Mkdir(p.modCache, 0774); err != nil {
		p.cleanFn()
		log.Fatalf("%v: failed to create mod cache directory: %v\n", color.RedString("error"), err)
	}

	log.Println(">>>>>>>>>>>>>>> workDir:", p.workDir)
}
