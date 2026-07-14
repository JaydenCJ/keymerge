// Command keymerge is a git merge driver for JSON files: it performs a
// key-level three-way merge and produces conflicts only on real collisions.
// All behavior lives in internal/cli so the whole surface can be tested
// in-process; main only wires the real process streams and exit code.
package main

import (
	"os"

	"github.com/JaydenCJ/keymerge/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
