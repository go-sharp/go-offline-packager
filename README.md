# go-offline-packager
A simple tool to manage dependencies in an air-gapped environment.

## Usage

```bash
Usage:
  go-offline-packager.exe [OPTIONS] <command>

Application Options:
      --go-bin=  Set full path to go binary (default: C:\Program
                 Files\Go\bin\go.exe) [%GOP_GO_BIN%]
  -v, --verbose  Verbose output

Help Options:
  -h, --help     Show this help message

Available commands:
  pack            Download modules and pack it into a zip file.
  publish-folder  Publish archive to a folder so it can be used as proxy source.
  publish-jfrog   Publish archive to jfrog artifactory (requires installed and configured jfrog-cli).
  version         Show version.
```

### Pack
Pack will download all your dependencies and create a zip file with it.

```bash
Usage:
  go-offline-packager.exe [OPTIONS] pack [pack-OPTIONS]

Download modules and pack it into a zip file.

Application Options:
      --go-bin=          Set full path to go binary (default: C:\Program
                         Files\Go\bin\go.exe) [%GOP_GO_BIN%]
  -v, --verbose          Verbose output

Help Options:
  -h, --help             Show this help message

[pack command options]
      -m, --module=      Modules to pack (github.com/jessevdk/go-flags or
                         github.com/jessevdk/go-flags@v1.4.0)
      -g, --go-mod-file= Pack all dependencies specified in go.mod file.
      -o, --out=         Output file name of the zip archive. (default:
                         gop_dependencies.zip)
      -t, --transitive   Ensure all transitive dependencies are included.
```
One can either use `-m` to specify dependencies or use the `-g` flag to use an existing go.mod file.

#### Example
```bash
# Use the -m flag
go-offline-packager.exe pack -t -v -m github.com/jessevdk/go-flags -m github.com/go-sharp/color@v1.9.1
# Use a go.mod file
go-offline-packager.exe pack -t -v -g go.mod
```

### Publish Folder
On the computer in the air gapped environment one can use `publish-folder` to extract the dependencies into a folder.
```bash
Usage:
  go-offline-packager.exe [OPTIONS] publish-folder [publish-folder-OPTIONS] ARCHIVE

Publish archive to a folder so it can be used as proxy source.

Application Options:
      --go-bin=      Set full path to go binary (default: C:\Program
                     Files\Go\bin\go.exe) [%GOP_GO_BIN%]
  -v, --verbose      Verbose output

Help Options:
  -h, --help         Show this help message

[publish-folder command options]
      -o, --out=     Output folder for the archive.

[publish-folder command arguments]
  ARCHIVE:           Path to archive with dependencies.
```

#### Example
```bash
go-offline-packager.exe publish-folder  -o mymodules gop_dependencies.zip
Publish-Folder: extracting archive
Publish-Folder: processing files
Publish-Folder: published archive to: /home/snmed/mymodules
Publish-Folder: hint: set GOPROXY to use folder for dependencies:
        go env -w GOPROXY=file:////home/snmed/mymodules
Publish-Folder: hint: in an air-gapped env set GOSUMDB to of:
        go env -w GOSUMDB=off
```

### Publish JFrog Artifactory
On the computer in the air gapped environment one can use `publish-jfrog` to upload dependencies into a JFrog Artifactory.
> Caveat: jfrog-cli must be installed and configured, otherwise dependencies can't be uploaded. Binary will be found automatically if installed in a OS search path, otherwise one has to specify the path to the binary.

```bash
Usage:
  go-offline-packager.exe [OPTIONS] publish-jfrog [publish-jfrog-OPTIONS] ARCHIVE

Publish archive to jfrog artifactory (requires installed and configured
jfrog-cli).

Application Options:
      --go-bin=        Set full path to go binary (default: C:\Program
                       Files\Go\bin\go.exe) [%GOP_GO_BIN%]
  -v, --verbose        Verbose output

Help Options:
  -h, --help           Show this help message

[publish-jfrog command options]
          --jfrog-bin= Set full path to the jfrog-cli binary [%GOP_JFROG_BIN%]
      -r, --repo=      Artifactory go repository name ex. go-local.

[publish-jfrog command arguments]
  ARCHIVE:             Path to archive with dependencies.
```
