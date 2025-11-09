package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-sharp/color"
	"github.com/jessevdk/go-flags"
)

const version = "v0.2.0"

var commonOpts options
var parser = flags.NewParser(&commonOpts, flags.HelpFlag|flags.PassDoubleDash)

var errorRedPrefix = color.RedString("error:")

const gomodTemp = `
module go-offline-packager

go 1.13
`

type options struct {
	GoBinPath string `long:"go-bin" env:"GOP_GO_BIN" description:"Set full path to go binary"`
	Verbose   bool   `short:"v" long:"verbose" description:"Verbose output"`
}

func init() {
	log.SetFlags(0)

	var packVersion = os.Getenv("GOP_PACK_VERSION")

	if packVersion == "1" {
		_, _ = parser.AddCommand("pack", "Download modules and pack it into a zip file.",
			"Download modules and pack it into a zip file.", &PackCmd{})
	} else {
		_, _ = parser.AddCommand("pack", "Download modules and pack it into a zip file.",
			"Download modules and pack it into a zip file.", &PackV2Cmd{})
	}

	_, _ = parser.AddCommand("publish-folder", "Publish archive to a folder so it can be used as proxy source.",
		"Publish archive to a folder so it can be used as proxy source.", &FolderPublishCmd{})

	_, _ = parser.AddCommand("publish-jfrog", "Publish archive to jfrog artifactory (requires installed and configured jfrog-cli).",
		"Publish archive to jfrog artifactory (requires installed and configured jfrog-cli).", &JFrogPublishCmd{})

	_, _ = parser.AddCommand("version", "Show version.", "Show version.", &versionCmd{})

	if p, err := exec.LookPath("go"); err == nil {
		if !filepath.IsAbs(p) {
			p, _ = filepath.Abs(p)
		}
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
	dir, err := os.MkdirTemp(os.TempDir(), "gop_")
	if err != nil {
		log.Fatalln("failed to create temporary working directory: ", color.RedString(err.Error()))
	}

	return dir, func() { removeContent(dir) }
}

func removeContent(dir string) {
	defer func() {
		if err := os.Remove(dir); err != nil {
			verboseF("can't remove directory: %v\n", err)
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
			verboseF("can't remove directory: %v\n", err)
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

func extractZipArchive(src, dst string) error {
	verboseF("extracting to: %v\n", color.BlueString(dst))
	if _, err := os.Stat(dst); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if err := os.MkdirAll(dst, 0777); err != nil {
			return err
		}
	}

	zipReader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		dFName := filepath.FromSlash(filepath.Join(dst, f.Name))
		// We ignore the error here because we get one as soon we open the file
		_ = os.MkdirAll(filepath.Dir(dFName), 0777)
		extractToFile(f, dFName)
		os.Chtimes(dFName, f.Modified, f.Modified)
	}
	return nil
}

func extractToFile(f *zip.File, dst string) {
	destF, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(errorRedPrefix, "failed to extract file", f.Name, ":", err)
		return
	}
	defer destF.Close()

	srcF, err := f.Open()
	if err != nil {
		log.Println(errorRedPrefix, "failed to extract file", f.Name, ":", err)
		return
	}
	defer srcF.Close()

	if _, err := io.Copy(destF, srcF); err != nil {
		log.Println(errorRedPrefix, "failed to extract file", f.Name, ":", err)
		return
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
		if err := addFileToArchive(f, dir, zw); err != nil {
			log.Printf("%v failed to add to archive: %v\n", errorRedPrefix, err)
		}
	}

	return <-done
}

func addFileToArchive(file, dir string, zw *zip.Writer) error {
	reader, err := os.Open(file)
	if err != nil {
		return err
	}
	defer reader.Close()

	fiStat, err := os.Stat(file)
	if err != nil {
		return err
	}

	name := strings.TrimLeft(strings.TrimPrefix(file, dir), string(filepath.Separator))

	fh, err := zip.FileInfoHeader(fiStat)
	if err != nil {
		return err
	}
	fh.Name = filepath.ToSlash(name)

	writer, err := zw.CreateHeader(fh)
	if err != nil {
		return err
	}

	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}
	return nil
}

type versionCmd struct{}

// Execute will be called for the last active (sub)command. The
// args argument contains the remaining command line arguments. The
// error that Execute returns will be eventually passed out of the
// Parse method of the Parser.
func (v versionCmd) Execute(args []string) error {
	fmt.Println(version)

	return nil
}
