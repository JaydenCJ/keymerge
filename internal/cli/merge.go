package cli

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/JaydenCJ/keymerge/internal/jsonval"
	"github.com/JaydenCJ/keymerge/internal/merge"
	"github.com/JaydenCJ/keymerge/internal/render"
)

// docs holds the three parsed sides plus enough raw context to detect the
// output style and to name the file in messages.
type docs struct {
	base, ours, theirs *jsonval.Value
	baseRaw            []byte
	oursRaw            []byte
	theirsRaw          []byte
	name               string
}

// loadDocs reads and parses the three files. An empty (or whitespace-only)
// file counts as an absent side: git hands the driver an empty ancestor
// when both branches added the file independently.
func loadDocs(basePath, oursPath, theirsPath, display string, stderr io.Writer) (*docs, int) {
	d := &docs{name: display}
	if d.name == "" {
		d.name = oursPath
	}
	var code int
	read := func(path, role string) []byte {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "keymerge: cannot read %s file: %v\n", role, err)
			code = ExitRuntime
		}
		return raw
	}
	d.baseRaw = read(basePath, "base")
	d.oursRaw = read(oursPath, "ours")
	d.theirsRaw = read(theirsPath, "theirs")
	if code != 0 {
		return nil, code
	}
	parse := func(raw []byte, role, path string) *jsonval.Value {
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil // absent side
		}
		v, err := jsonval.Parse(raw)
		if err != nil {
			fmt.Fprintf(stderr, "keymerge: %s file %s: %v\n", role, path, err)
			code = ExitRuntime
		}
		return v
	}
	d.base = parse(d.baseRaw, "base", basePath)
	d.ours = parse(d.oursRaw, "ours", oursPath)
	d.theirs = parse(d.theirsRaw, "theirs", theirsPath)
	if code != 0 {
		return nil, code
	}
	return d, ExitClean
}

// mergeFlags declares the flags shared by merge and check on fs and
// returns pointers to the display path and the arrays strategy name.
func mergeFlags(fl *flag.FlagSet) (display, arrays *string) {
	display = new(string)
	fl.StringVar(display, "p", "", "display path for messages")
	fl.StringVar(display, "path", "", "display path for messages")
	arrays = fl.String("arrays", "merge", "array strategy: merge, atomic or union")
	return display, arrays
}

func runMerge(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("merge", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	display, arrays := mergeFlags(fl)
	var markerSize int
	fl.IntVar(&markerSize, "m", 7, "conflict marker size")
	fl.IntVar(&markerSize, "marker-size", 7, "conflict marker size")
	var output string
	fl.StringVar(&output, "o", "", "output destination")
	fl.StringVar(&output, "output", "", "output destination")
	oursLabel := fl.String("ours-label", "ours", "marker label for our side")
	theirsLabel := fl.String("theirs-label", "theirs", "marker label for their side")
	files, err := parseInterleaved(fl, args)
	if err != nil {
		return flagParseExit(err, stdout, stderr)
	}
	if len(files) != 3 {
		fmt.Fprintf(stderr, "keymerge: merge needs exactly three files: <base> <ours> <theirs>\n")
		return ExitUsage
	}
	strategy, ok := merge.ParseStrategy(*arrays)
	if !ok {
		fmt.Fprintf(stderr, "keymerge: unknown --arrays strategy %q (use merge, atomic or union)\n", *arrays)
		return ExitUsage
	}

	basePath, oursPath, theirsPath := files[0], files[1], files[2]
	d, code := loadDocs(basePath, oursPath, theirsPath, *display, stderr)
	if code != ExitClean {
		return code
	}

	res := merge.Merge(d.base, d.ours, d.theirs, merge.Options{Arrays: strategy})
	style := jsonval.DetectStyle(firstNonEmpty(d.oursRaw, d.theirsRaw, d.baseRaw))
	out := render.Render(res.Tree, render.Options{
		Style:       style,
		MarkerSize:  markerSize,
		OursLabel:   *oursLabel,
		TheirsLabel: *theirsLabel,
	})

	switch output {
	case "-":
		if _, err := stdout.Write(out); err != nil {
			fmt.Fprintf(stderr, "keymerge: cannot write result: %v\n", err)
			return ExitRuntime
		}
	case "":
		// Git driver contract: the result replaces the "ours" file.
		if err := writeFilePreservingMode(oursPath, out); err != nil {
			fmt.Fprintf(stderr, "keymerge: cannot write %s: %v\n", oursPath, err)
			return ExitRuntime
		}
	default:
		if err := os.WriteFile(output, out, 0o644); err != nil {
			fmt.Fprintf(stderr, "keymerge: cannot write %s: %v\n", output, err)
			return ExitRuntime
		}
	}

	if len(res.Conflicts) > 0 {
		for _, c := range res.Conflicts {
			fmt.Fprintf(stderr, "keymerge: conflict at %s (%s) in %s\n", displayPointer(c.Path), c.Kind, d.name)
		}
		fmt.Fprintf(stderr, "keymerge: %s left in %s\n", countNoun(len(res.Conflicts), "conflict"), d.name)
		return ExitConflict
	}
	return ExitClean
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("check", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	display, arrays := mergeFlags(fl)
	files, err := parseInterleaved(fl, args)
	if err != nil {
		return flagParseExit(err, stdout, stderr)
	}
	if len(files) != 3 {
		fmt.Fprintf(stderr, "keymerge: check needs exactly three files: <base> <ours> <theirs>\n")
		return ExitUsage
	}
	strategy, ok := merge.ParseStrategy(*arrays)
	if !ok {
		fmt.Fprintf(stderr, "keymerge: unknown --arrays strategy %q (use merge, atomic or union)\n", *arrays)
		return ExitUsage
	}
	d, code := loadDocs(files[0], files[1], files[2], *display, stderr)
	if code != ExitClean {
		return code
	}
	res := merge.Merge(d.base, d.ours, d.theirs, merge.Options{Arrays: strategy})
	if len(res.Conflicts) == 0 {
		fmt.Fprintf(stdout, "keymerge: %s merges cleanly (no conflicts)\n", d.name)
		return ExitClean
	}
	for _, c := range res.Conflicts {
		fmt.Fprintf(stdout, "%-40s %s\n", displayPointer(c.Path), c.Kind)
	}
	fmt.Fprintf(stdout, "keymerge: %s in %s\n", countNoun(len(res.Conflicts), "conflict"), d.name)
	return ExitConflict
}

// countNoun formats a count with a correctly pluralized noun ("1 conflict",
// "2 conflicts") — small, but "1 conflict(s)" reads like nobody looked.
func countNoun(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// displayPointer renders an RFC 6901 pointer for humans; the empty pointer
// addresses the whole document.
func displayPointer(p string) string {
	if p == "" {
		return "(document root)"
	}
	return p
}

func firstNonEmpty(candidates ...[]byte) []byte {
	for _, c := range candidates {
		if len(bytes.TrimSpace(c)) > 0 {
			return c
		}
	}
	return nil
}

// writeFilePreservingMode rewrites path in place, keeping its permission
// bits (merge drivers run on working-tree temp files, but being polite
// about modes costs nothing).
func writeFilePreservingMode(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}
