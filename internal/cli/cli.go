// Package cli implements the keymerge command-line interface. Run takes
// argv and two writers and returns an exit code, so the whole surface is
// testable in-process without building a binary.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/JaydenCJ/keymerge/internal/version"
)

// Exit codes. Git treats any non-zero merge-driver exit as "conflicts
// remain", so all three failure codes are safe from inside a merge; the
// distinction matters for scripts calling keymerge directly.
const (
	ExitClean    = 0 // merged without conflicts
	ExitConflict = 1 // real collisions were written as conflict markers
	ExitUsage    = 2 // bad flags or arguments
	ExitRuntime  = 3 // unreadable or invalid input (ours is left untouched)
)

// Run dispatches argv and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return ExitUsage
	}
	switch args[0] {
	case "merge":
		return runMerge(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "install":
		return runInstall(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "keymerge %s\n", version.Version)
		return ExitClean
	case "help", "--help", "-h":
		usage(stdout)
		return ExitClean
	default:
		fmt.Fprintf(stderr, "keymerge: unknown command %q\n\n", args[0])
		usage(stderr)
		return ExitUsage
	}
}

// parseInterleaved parses args allowing flags before, between and after
// positional arguments. The standard library stops at the first positional,
// but a git driver line naturally reads "keymerge merge %O %A %B -p %P -m
// %L" — flags last — so we keep consuming until everything is classified.
func parseInterleaved(fl *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fl.Parse(args); err != nil {
			return nil, err
		}
		args = fl.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// flagParseExit turns a flag-parsing error into an exit code: -h/--help on
// a subcommand prints the usage text and succeeds; anything else is a
// usage error worth reporting.
func flagParseExit(err error, stdout, stderr io.Writer) int {
	if errors.Is(err, flag.ErrHelp) {
		usage(stdout)
		return ExitClean
	}
	fmt.Fprintf(stderr, "keymerge: %v\n", err)
	return ExitUsage
}

func usage(w io.Writer) {
	fmt.Fprint(w, strings.TrimLeft(`
keymerge - git merge driver for JSON: key-level three-way merge

usage:
  keymerge merge [flags] <base> <ours> <theirs>   merge; rewrites <ours> with the result
  keymerge check [flags] <base> <ours> <theirs>   dry run; list colliding paths, write nothing
  keymerge install [flags]                        register the driver in git config
  keymerge version                                print the version

merge flags:
  -p, --path          display path used in messages (git passes %P)
  -m, --marker-size   conflict marker size (git passes %L; default 7)
  -o, --output        write result here instead of over <ours>; "-" for stdout
  --arrays            array strategy: merge, atomic or union (default merge)
  --ours-label        marker label after "<<<<<<<" (default ours)
  --theirs-label      marker label after ">>>>>>>" (default theirs)

check flags:
  -p, --path, --arrays as above

install flags:
  --global            write to the global git config instead of the repo's
  --pattern <glob>    also add "<glob> merge=keymerge" to .gitattributes
  --print             only print the commands, change nothing
  -C <dir>            operate on the repository at <dir>

exit codes: 0 clean merge, 1 conflicts, 2 usage error, 3 runtime error
`, "\n"))
}
