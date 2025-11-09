//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
	"github.com/magefile/mage/sh" // mg contains helpful utility functions, like Deps
)

// Default target to run when none is specified
// If not set, running mage will list available targets

type Build mg.Namespace
type Publish mg.Namespace

var Default = Build.App

// ****************************************
// * Variables
// ****************************************
const appName = "go-offline-packager"
const publishFolder = "dist"

var publishConf = map[string]map[string]string{
	"windows-amd64": {
		"GOOS":   "windows",
		"GOARCH": "amd64",
	},
	"macos-amd64": {
		"GOOS":   "darwin",
		"GOARCH": "amd64",
	},
	"macos-arm64": {
		"GOOS":   "darwin",
		"GOARCH": "arm64",
	},
	"linux-amd64": {
		"GOOS":   "linux",
		"GOARCH": "amd64",
	},
}

// ****************************************
// * Helper functions
// ****************************************
var g0 = sh.RunCmd("go")

// A build step that requires additional params, or platform specific steps for example
func (Build) App() error {
	mg.Deps(Build.InstallDeps)
	fmt.Println("Building...")
	cmd := exec.Command("go", "build", "-o", appName, ".")
	return cmd.Run()
}

// Manage your deps, or running package managers.
func (Build) InstallDeps() error {
	fmt.Println("Installing Deps...")
	return g0("mod", "download")
}

// Clean up after yourself
func (Build) Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll(appName)
}

func (Publish) All() error {
	fmt.Println("Publishing apps...")
	if err := os.RemoveAll(publishFolder); err != nil {
		return err
	}

	if err := os.Mkdir(publishFolder, 0770); err != nil {
		return err
	}

	for k, e := range publishConf {
		fmt.Println("Publishing ", k)

		var outputPath = filepath.Join(publishFolder, k)
		if err := os.Mkdir(outputPath, 0770); err != nil {
			return err
		}

		var outputName = appName
		if strings.HasPrefix(k, "windows") {
			outputName += ".exe"
		}

		var output = filepath.Join(outputPath, outputName)
		if err := sh.RunWith(e, "go", "build", "-ldflags", "-w -s", "-o", output); err != nil {
			return err
		}
	}

	return nil
}
