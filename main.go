package main

import (
	"archive/zip"
	"github.com/go-sharp/color"
	"github.com/jessevdk/go-flags"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var commonOpts options
var parser = flags.NewParser(&commonOpts, flags.HelpFlag|flags.PassDoubleDash)

var errorRedPrefix = color.RedString("error:")

const gomodTemp = `
module go-offline-packager

go 1.13
`

type options struct {
	GoBinPath string `long:"go-bin" env:"GOP_GO_BIN" description:"Set full path to go binary (default: go binary in paths)"`
	Verbose   bool   `short:"v" long:"verbose" description:"Verbose output"`
}

func init() {
	log.SetFlags(0)
	_, _ = parser.AddCommand("pack", "Download modules and pack it into a zip file.", "Download modules and pack it into a zip file.", &packCmd)

	if p, err := exec.LookPath("go"); err == nil {
		commonOpts.GoBinPath = p
	}
}

func main() {
	if _, err := parser.Parse(); err != nil {
		if t, ok := err.(*flags.Error); ok && t.Type == flags.ErrHelp {
			parser.WriteHelp(os.Stdout)
			os.Exit(0)
		}
		color.Red("%s", err)
		os.Exit(1)
	}
}

func createTempWorkDir() (wd string, cleanFn func()) {
	dir, err := ioutil.TempDir(os.TempDir(), "gop_")
	if err != nil {
		log.Fatalln("failed to create temporary working directory: ", color.RedString(err.Error()))
	}

	return dir, func() { removeContent(dir) }
}

func removeContent(dir string) {
	defer func() {
		if err := os.Remove(dir); err != nil {
			verboseF("can't remove directory %v: %v\n", err)
		}
	}()

	f, err := os.Open(dir)
	if err != nil {
		verboseF("can't remove directory %v: %v\n", dir, color.YellowString(err.Error()))
		return
	}
	defer f.Close()

	fs, err := f.Readdirnames(0)
	if err != nil {
		verboseF("can't read directory %v: %v\n", dir, color.YellowString(err.Error()))
		return
	}

	for _, fi := range fs {
		fpath := filepath.Join(dir, fi)
		fstat, err := os.Stat(fpath)
		if err != nil {
			verboseF("can't read file stat %v: %v\n", dir, color.YellowString(err.Error()))
			continue
		}

		// Handle directory
		if fstat.IsDir() {
			_ = os.Chmod(fpath, 0777)
			removeContent(fpath)
			continue
		}

		_ = os.Chmod(fpath, 0666)
		if err := os.Remove(fpath); err != nil {
			verboseF("can't remove directory %v: %v\n", err)
		}
	}
}

func verboseF(format string, v ...interface{}) {
	if commonOpts.Verbose {
		log.Printf(format, v...)
	}
}

func checkGo() {
	if f, err := os.Stat(commonOpts.GoBinPath); err != nil || f.IsDir() {
		log.Fatalln(errorRedPrefix, "missing go binary, install go or specify path to go binary")
	}
}

func createZipArchive(dir, dst string) error {
	fw, err := os.OpenFile(dst, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer fw.Close()

	zw := zip.NewWriter(fw)
	defer zw.Close()

	done := make(chan error)
	work := make(chan string)
	go func() {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			work <- path
			return nil
		})
		close(work)
		done <- err
	}()

	for f := range work {
		reader, err := os.Open(f)
		if err != nil {
			log.Printf("%v failed to add to archive: %v\n", errorRedPrefix, err)
			continue
		}

		name := strings.TrimLeft(strings.TrimPrefix(f, dir), string(filepath.Separator))
		writer, err := zw.Create(name)
		if err != nil {
			log.Printf("%v failed to add to archive: %v\n", errorRedPrefix, err)
			continue
		}

		if _, err := io.Copy(writer, reader); err != nil {
			log.Printf("%v failed to add to archive: %v\n", errorRedPrefix, err)
			continue
		}
	}

	return <-done
}
