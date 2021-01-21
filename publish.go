package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-sharp/color"
)

type publishCmd struct {
	PosArgs struct {
		Archive string `positional-arg-name:"ARCHIVE" description:"Path to archive with dependencies. " default:"gop_dependencies.zip"`
	} `positional-args:"yes" required:"1"`
}

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
	err = filepath.Walk(dirPrefix, func(path string, info os.FileInfo, err error) error {
		relPath := strings.TrimLeft(strings.TrimPrefix(path, dirPrefix), string(filepath.Separator))

		if strings.HasPrefix(relPath, "sumdb") && !info.IsDir() {
			go f.handleCopyFile(path, relPath)
			return nil
		}

		if info.IsDir() && strings.HasSuffix(relPath, "@v") {
			go f.handleModule(path, relPath)
			return nil
		}

		return nil
	})

	if err != nil {
		log.Println(errorRedPrefix, err)
	}

	return nil
}

func (f FolderPublishCmd) handleModule(path, relPath string) {
	//dstDir := filepath.Join(f.Output, relPath)

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
