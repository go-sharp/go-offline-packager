package main

import (
	"fmt"
	"os"

	"github.com/go-sharp/color"
	"github.com/jessevdk/go-flags"
)

var parser = flags.NewParser(nil, flags.HelpFlag|flags.PassDoubleDash)

const gomodTemp = `
module go-offline-packager

go 1.13
`

var packCmd PackCmd

type PackCmd struct {
	Modul []string `short:"m" long:"modul" description:"Modules to pack (github.com/jessevdk/go-flags or github.com/jessevdk/go-flags@v1.4.0)"`
}

// Execute will be called for the last active (sub)command. The
// args argument contains the remaining command line arguments. The
// error that Execute returns will be eventually passed out of the
// Parse method of the Parser.
func (p *PackCmd) Execute(args []string) error {
	fmt.Println(p)

	return nil
}

func init() {
	parser.AddCommand("pack", "Download modules and pack it into a zip file.", "Download modules and pack it into a zip file.", &packCmd)
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
