package main

import (
	"errors"
	"log"
	"os"
)

type publishCmd struct {
	PosArgs struct{
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

	defaultErrStr := errorRedPrefix + " failed to extract archive:"
	if err := extractZipArchive(f.PosArgs.Archive, workDir); err != nil {
		log.Fatalln(defaultErrStr, err)
	}

	fi, err := os.Stat(f.Output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalln(defaultErrStr, err)
		}
		if err := os.MkdirAll(f.Output, 0777); err != nil {
			log.Fatalln(defaultErrStr, err)
		}
	} else if !fi.IsDir() {
		log.Fatalln(errorRedPrefix, "output is not a directory:", f.Output)
	}

	return nil
}


