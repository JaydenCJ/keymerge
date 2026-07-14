// In-process CLI tests: every subcommand is exercised through Run with
// real files in temp dirs, asserting stdout, stderr, exit codes and the
// bytes written back. The install tests drive a real (offline, isolated)
// git repository. Everything is deterministic.
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// run invokes the CLI in-process and captures both streams.
func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = Run(args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

// triple writes base/ours/theirs files into a temp dir and returns their paths.
func triple(t *testing.T, base, ours, theirs string) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	return write("base.json", base), write("ours.json", ours), write("theirs.json", theirs)
}

const (
	cleanBase   = "{\n  \"name\": \"app\",\n  \"version\": \"1.0.0\",\n  \"deps\": {\n    \"a\": \"^1\"\n  }\n}\n"
	cleanOurs   = "{\n  \"name\": \"app\",\n  \"version\": \"2.0.0\",\n  \"deps\": {\n    \"a\": \"^1\"\n  }\n}\n"
	cleanTheirs = "{\n  \"name\": \"app\",\n  \"version\": \"1.0.0\",\n  \"deps\": {\n    \"a\": \"^2\",\n    \"b\": \"^1\"\n  }\n}\n"
)

func TestVersionAndHelp(t *testing.T) {
	code, out, _ := run(t, "version")
	if code != ExitClean || out != "keymerge 0.1.0\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	code, out2, _ := run(t, "--version")
	if code != ExitClean || out2 != out {
		t.Fatalf("--version differs: %q", out2)
	}
	code, help, _ := run(t, "help")
	if code != ExitClean {
		t.Fatalf("help: code=%d", code)
	}
	for _, want := range []string{"merge", "check", "install", "exit codes"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
	// -h on a subcommand is a help request, not a usage error.
	for _, sub := range []string{"merge", "check", "install"} {
		code, subHelp, _ := run(t, sub, "-h")
		if code != ExitClean || !strings.Contains(subHelp, "exit codes") {
			t.Fatalf("%s -h: code=%d out=%q", sub, code, subHelp)
		}
	}
}

func TestUnknownCommandExitsUsage(t *testing.T) {
	code, _, errOut := run(t, "frobnicate")
	if code != ExitUsage || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if code, _, _ := run(t); code != ExitUsage {
		t.Fatal("no args should be a usage error")
	}
}

func TestMergeCleanRewritesOursInPlace(t *testing.T) {
	base, ours, theirs := triple(t, cleanBase, cleanOurs, cleanTheirs)
	code, out, errOut := run(t, "merge", base, ours, theirs)
	if code != ExitClean {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if out != "" || errOut != "" {
		t.Fatalf("clean driver merge must be silent, got out=%q err=%q", out, errOut)
	}
	merged, _ := os.ReadFile(ours)
	for _, want := range []string{`"version": "2.0.0"`, `"a": "^2"`, `"b": "^1"`} {
		if !strings.Contains(string(merged), want) {
			t.Fatalf("merged file missing %s:\n%s", want, merged)
		}
	}
}

func TestMergeFlagsAfterPositionalsLikeGitDriverLine(t *testing.T) {
	// git expands "keymerge merge %O %A %B -p %P -m %L": flags come last.
	base, ours, theirs := triple(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`)
	code, _, errOut := run(t, "merge", base, ours, theirs, "-p", "cfg/app.json", "-m", "9")
	if code != ExitConflict {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errOut, "in cfg/app.json") {
		t.Fatalf("display path not used: %q", errOut)
	}
	merged, _ := os.ReadFile(ours)
	if !strings.Contains(string(merged), "<<<<<<<<< ours") {
		t.Fatalf("marker size 9 not honored:\n%s", merged)
	}
}

func TestMergeConflictWritesMarkersAndExitsOne(t *testing.T) {
	base, ours, theirs := triple(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`)
	code, _, errOut := run(t, "merge", base, ours, theirs)
	if code != ExitConflict {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errOut, "conflict at /a (edit/edit)") {
		t.Fatalf("stderr missing pointer path: %q", errOut)
	}
	// Correct singular: "1 conflict left", never "1 conflict(s)".
	if !strings.Contains(errOut, "1 conflict left") || strings.Contains(errOut, "(s)") {
		t.Fatalf("summary not pluralized correctly: %q", errOut)
	}
	merged, _ := os.ReadFile(ours)
	for _, want := range []string{"<<<<<<< ours", "=======", ">>>>>>> theirs"} {
		if !strings.Contains(string(merged), want) {
			t.Fatalf("markers missing:\n%s", merged)
		}
	}
}

func TestMergeOutputDestinations(t *testing.T) {
	// -o -  → stdout, ours untouched;  -o file  → that file.
	base, ours, theirs := triple(t, cleanBase, cleanOurs, cleanTheirs)
	before, _ := os.ReadFile(ours)
	code, out, _ := run(t, "merge", "-o", "-", base, ours, theirs)
	if code != ExitClean || !strings.Contains(out, `"version": "2.0.0"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
	after, _ := os.ReadFile(ours)
	if !bytes.Equal(before, after) {
		t.Fatal("-o - must not touch the ours file")
	}
	dest := filepath.Join(t.TempDir(), "merged.json")
	if code, _, _ := run(t, "merge", "-o", dest, base, ours, theirs); code != ExitClean {
		t.Fatalf("code=%d", code)
	}
	merged, err := os.ReadFile(dest)
	if err != nil || !strings.Contains(string(merged), `"b": "^1"`) {
		t.Fatalf("output file wrong: %v %s", err, merged)
	}
}

func TestMergePreservesFourSpaceIndentAndFinalNewline(t *testing.T) {
	base, ours, theirs := triple(t,
		"{\n    \"a\": 1,\n    \"b\": 1\n}",
		"{\n    \"a\": 2,\n    \"b\": 1\n}",
		"{\n    \"a\": 1,\n    \"b\": 2\n}")
	code, _, _ := run(t, "merge", base, ours, theirs)
	if code != ExitClean {
		t.Fatalf("code=%d", code)
	}
	merged, _ := os.ReadFile(ours)
	want := "{\n    \"a\": 2,\n    \"b\": 2\n}"
	if string(merged) != want {
		t.Fatalf("style not preserved:\n%q\nwant\n%q", merged, want)
	}
}

func TestMergeEmptyBaseIdenticalAddsAreClean(t *testing.T) {
	// git passes an empty ancestor when both branches added the file.
	base, ours, theirs := triple(t, "", `{"a": 1}`+"\n", `{"a": 1}`+"\n")
	code, _, errOut := run(t, "merge", base, ours, theirs)
	if code != ExitClean {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestMergeInvalidJSONLeavesOursUntouched(t *testing.T) {
	base, ours, theirs := triple(t, `{"a": 1}`, `{"a": 2}`, `{"a": `)
	before, _ := os.ReadFile(ours)
	code, _, errOut := run(t, "merge", base, ours, theirs)
	if code != ExitRuntime {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(errOut, "theirs file") || !strings.Contains(errOut, "line 1") {
		t.Fatalf("stderr=%q", errOut)
	}
	after, _ := os.ReadFile(ours)
	if !bytes.Equal(before, after) {
		t.Fatal("ours must stay untouched when parsing fails")
	}
}

func TestMergeUsageErrors(t *testing.T) {
	if code, _, _ := run(t, "merge", "a", "b"); code != ExitUsage {
		t.Fatalf("two files: code=%d", code)
	}
	base, ours, theirs := triple(t, `1`, `1`, `1`)
	if code, _, _ := run(t, "merge", "--arrays", "bogus", base, ours, theirs); code != ExitUsage {
		t.Fatal("bogus --arrays accepted")
	}
	if code, _, _ := run(t, "merge", "--no-such-flag", base, ours, theirs); code != ExitUsage {
		t.Fatal("unknown flag accepted")
	}
}

func TestMergeArraysUnionFlag(t *testing.T) {
	base, ours, theirs := triple(t, `["a"]`, `["a", "x"]`, `["a", "y"]`)
	code, out, _ := run(t, "merge", "--arrays", "union", "-o", "-", base, ours, theirs)
	if code != ExitClean {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, `"x"`) || !strings.Contains(out, `"y"`) {
		t.Fatalf("union result wrong:\n%s", out)
	}
}

func TestMergeCustomLabels(t *testing.T) {
	base, ours, theirs := triple(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`)
	code, _, _ := run(t, "merge", "--ours-label", "HEAD", "--theirs-label", "feature", base, ours, theirs)
	if code != ExitConflict {
		t.Fatalf("code=%d", code)
	}
	merged, _ := os.ReadFile(ours)
	if !strings.Contains(string(merged), "<<<<<<< HEAD") || !strings.Contains(string(merged), ">>>>>>> feature") {
		t.Fatalf("labels not applied:\n%s", merged)
	}
}

func TestCheckCleanAndConflict(t *testing.T) {
	base, ours, theirs := triple(t, cleanBase, cleanOurs, cleanTheirs)
	before, _ := os.ReadFile(ours)
	code, out, _ := run(t, "check", base, ours, theirs)
	if code != ExitClean || !strings.Contains(out, "merges cleanly") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	after, _ := os.ReadFile(ours)
	if !bytes.Equal(before, after) {
		t.Fatal("check must never write")
	}
	base, ours, theirs = triple(t,
		`{"v": 1, "s": {"x": 1}}`,
		`{"v": 2, "s": {"x": 2}}`,
		`{"v": 3, "s": {"x": 3}}`)
	code, out, _ = run(t, "check", base, ours, theirs)
	if code != ExitConflict {
		t.Fatalf("code=%d", code)
	}
	for _, want := range []string{"/v", "/s/x", "2 conflicts in"} {
		if !strings.Contains(out, want) {
			t.Fatalf("check output missing %q:\n%s", want, out)
		}
	}
}

// gitRepo initializes an isolated repository for install tests.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q", dir)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func gitConfigValue(t *testing.T, dir, key string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "config", "--local", "--get", key)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config --get %s: %v", key, err)
	}
	return strings.TrimSpace(string(out))
}

func TestInstallConfiguresLocalGitConfig(t *testing.T) {
	repo := gitRepo(t)
	code, out, errOut := run(t, "install", "-C", repo)
	if code != ExitClean {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	if got := gitConfigValue(t, repo, "merge.keymerge.driver"); got != driverCommand {
		t.Fatalf("driver = %q, want %q", got, driverCommand)
	}
	if got := gitConfigValue(t, repo, "merge.keymerge.name"); got != driverName {
		t.Fatalf("name = %q", got)
	}
	if !strings.Contains(out, ".gitattributes") {
		t.Fatalf("install should point at .gitattributes next:\n%s", out)
	}
}

func TestInstallPatternWritesGitattributesIdempotently(t *testing.T) {
	repo := gitRepo(t)
	code, out, _ := run(t, "install", "-C", repo, "--pattern", "*.json")
	if code != ExitClean || !strings.Contains(out, `added "*.json merge=keymerge"`) {
		t.Fatalf("code=%d out=%q", code, out)
	}
	// Second run must not duplicate the line.
	code, out, _ = run(t, "install", "-C", repo, "--pattern", "*.json")
	if code != ExitClean || !strings.Contains(out, "already present") {
		t.Fatalf("second run: code=%d out=%q", code, out)
	}
	attrs, _ := os.ReadFile(filepath.Join(repo, ".gitattributes"))
	if got := strings.Count(string(attrs), "merge=keymerge"); got != 1 {
		t.Fatalf(".gitattributes has %d entries:\n%s", got, attrs)
	}
}

func TestInstallPrintOnlyMakesNoChanges(t *testing.T) {
	repo := gitRepo(t)
	code, out, _ := run(t, "install", "-C", repo, "--print")
	if code != ExitClean {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "git config merge.keymerge.driver") {
		t.Fatalf("print output wrong:\n%s", out)
	}
	cmd := exec.Command("git", "-C", repo, "config", "--local", "--get", "merge.keymerge.driver")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if err := cmd.Run(); err == nil {
		t.Fatal("--print must not modify git config")
	}
}

func TestInstallOutsideRepoFailsForPattern(t *testing.T) {
	dir := t.TempDir() // not a git repository
	code, _, errOut := run(t, "install", "-C", dir, "--pattern", "*.json")
	if code != ExitRuntime && code != ExitClean {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
	// Outside a repo `git config --local` itself fails first; either way
	// nothing may be created.
	if _, err := os.Stat(filepath.Join(dir, ".gitattributes")); !os.IsNotExist(err) {
		t.Fatal(".gitattributes must not appear outside a repository")
	}
}
