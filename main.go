// Command stlcheck validates EBU Tech 3264 subtitle files without modifying them.
package main

import (
	"os"

	"github.com/lsafjasf/20261124/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[0], os.Args[1:], os.Stdout, os.Stderr))
}
