package main

import (
	"os"
	_ "time/tzdata"

	"github.com/rcmcsweeney/cxre/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
