package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The driver line git will run. %O/%A/%B are the base/ours/theirs temp
// files, %P is the repository-relative path (for messages) and %L the
// conflict-marker size a user may have set per path.
const (
	driverName    = "keymerge: key-level three-way JSON merge"
	driverCommand = "keymerge merge %O %A %B -p %P -m %L"
)

// runInstall registers the merge driver in git config and (optionally)
// routes a pattern to it in .gitattributes — the whole setup in one go.
func runInstall(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("install", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	global := fl.Bool("global", false, "write to the global git config")
	printOnly := fl.Bool("print", false, "print the commands, change nothing")
	pattern := fl.String("pattern", "", "also add \"<pattern> merge=keymerge\" to .gitattributes")
	dir := fl.String("C", ".", "operate on the repository at this directory")
	positional, err := parseInterleaved(fl, args)
	if err != nil {
		return flagParseExit(err, stdout, stderr)
	}
	if len(positional) != 0 {
		fmt.Fprintf(stderr, "keymerge: install takes no positional arguments\n")
		return ExitUsage
	}

	if *printOnly {
		fmt.Fprintf(stdout, "# register the driver (add --global for all repositories):\n")
		fmt.Fprintf(stdout, "git config merge.keymerge.name %q\n", driverName)
		fmt.Fprintf(stdout, "git config merge.keymerge.driver %q\n", driverCommand)
		fmt.Fprintf(stdout, "# then route files to it:\n")
		fmt.Fprintf(stdout, "echo '*.json merge=keymerge' >> .gitattributes\n")
		return ExitClean
	}

	scope, scopeName := "--local", "local"
	if *global {
		scope, scopeName = "--global", "global"
	}
	settings := [][2]string{
		{"merge.keymerge.name", driverName},
		{"merge.keymerge.driver", driverCommand},
	}
	for _, kv := range settings {
		out, err := exec.Command("git", "-C", *dir, "config", scope, kv[0], kv[1]).CombinedOutput()
		if err != nil {
			fmt.Fprintf(stderr, "keymerge: git config failed: %v\n%s", err, out)
			return ExitRuntime
		}
		fmt.Fprintf(stdout, "git config (%s): %s = %s\n", scopeName, kv[0], kv[1])
	}

	if *pattern != "" {
		root, err := gitTopLevel(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "keymerge: --pattern needs a repository: %v\n", err)
			return ExitRuntime
		}
		line := *pattern + " merge=keymerge"
		added, err := ensureLine(filepath.Join(root, ".gitattributes"), line)
		if err != nil {
			fmt.Fprintf(stderr, "keymerge: cannot update .gitattributes: %v\n", err)
			return ExitRuntime
		}
		if added {
			fmt.Fprintf(stdout, ".gitattributes: added %q\n", line)
		} else {
			fmt.Fprintf(stdout, ".gitattributes: %q already present\n", line)
		}
	} else {
		fmt.Fprintf(stdout, "next: echo '*.json merge=keymerge' >> .gitattributes\n")
	}
	fmt.Fprintln(stdout, "note: the keymerge binary must be on PATH when git runs the driver")
	return ExitClean
}

func gitTopLevel(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repository (%s)", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureLine appends line to the file unless an identical line is already
// there. It reports whether the file changed. Idempotent by design so
// repeated installs never grow .gitattributes.
func ensureLine(path, line string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	for _, existing := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(existing) == line {
			return false, nil
		}
	}
	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += line + "\n"
	return true, os.WriteFile(path, []byte(content), 0o644)
}
