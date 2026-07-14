// Renderer tests: conflict markers must land on whole lines exactly at
// the collision, commas must stay balanced around them, and the detected
// style (indent, CRLF, marker size, labels) must be honored.
package render

import (
	"strings"
	"testing"

	"github.com/JaydenCJ/keymerge/internal/jsonval"
	"github.com/JaydenCJ/keymerge/internal/merge"
)

func v(t *testing.T, src string) *jsonval.Value {
	t.Helper()
	if src == "" {
		return nil
	}
	val, err := jsonval.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return val
}

func renderMerge(t *testing.T, base, ours, theirs string, opt Options) string {
	t.Helper()
	res := merge.Merge(v(t, base), v(t, ours), v(t, theirs), merge.Options{})
	return string(Render(res.Tree, opt))
}

func defOpt() Options {
	return Options{Style: jsonval.DefaultStyle()}
}

func TestRenderCleanTreeMatchesWriter(t *testing.T) {
	got := renderMerge(t, `{"a": 1}`, `{"a": 2}`, `{"a": 1}`, defOpt())
	if got != "{\n  \"a\": 2\n}\n" {
		t.Fatalf("got %q", got)
	}
	// A document merged to absence renders as zero bytes.
	if out := Render(nil, defOpt()); len(out) != 0 {
		t.Fatalf("absent document rendered %q", out)
	}
}

func TestRenderMemberConflictGolden(t *testing.T) {
	got := renderMerge(t,
		`{"name": "app", "version": "1.0.0", "private": true}`,
		`{"name": "app", "version": "2.0.0", "private": true}`,
		`{"name": "app", "version": "3.0.0", "private": true}`,
		defOpt())
	want := `{
  "name": "app",
<<<<<<< ours
  "version": "2.0.0",
=======
  "version": "3.0.0",
>>>>>>> theirs
  "private": true
}
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDeleteEditConflictHasEmptyOursSide(t *testing.T) {
	got := renderMerge(t, `{"a": 1}`, `{}`, `{"a": 2}`, defOpt())
	want := `{
<<<<<<< ours
=======
  "a": 2
>>>>>>> theirs
}
`
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRenderMarkerSizeAndLabels(t *testing.T) {
	got := renderMerge(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`,
		Options{Style: jsonval.DefaultStyle(), MarkerSize: 3})
	if !strings.Contains(got, "<<< ours") || !strings.Contains(got, "\n===\n") {
		t.Fatalf("marker size 3 not honored:\n%s", got)
	}
	got = renderMerge(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`,
		Options{Style: jsonval.DefaultStyle(), OursLabel: "HEAD", TheirsLabel: "feature/x"})
	if !strings.Contains(got, "<<<<<<< HEAD") || !strings.Contains(got, ">>>>>>> feature/x") {
		t.Fatalf("labels not honored:\n%s", got)
	}
}

func TestRenderRootConflict(t *testing.T) {
	got := renderMerge(t, `1`, `{"a": 1}`, `[1]`, defOpt())
	want := `<<<<<<< ours
{
  "a": 1
}
=======
[
  1
]
>>>>>>> theirs
`
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRenderArraySpliceConflict(t *testing.T) {
	got := renderMerge(t, `[1]`, `[1, 2]`, `[1, 3]`, defOpt())
	want := `[
  1,
<<<<<<< ours
  2
=======
  3
>>>>>>> theirs
]
`
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRenderCommaAfterConflictWhenNotLast(t *testing.T) {
	// The conflicting member sits before "z": both candidate lines need
	// the trailing comma so either resolution stays parseable.
	got := renderMerge(t,
		`{"a": 1, "z": 9}`,
		`{"a": 2, "z": 9}`,
		`{"a": 3, "z": 9}`,
		defOpt())
	if !strings.Contains(got, "\"a\": 2,\n") || !strings.Contains(got, "\"a\": 3,\n") {
		t.Fatalf("missing commas:\n%s", got)
	}
	if !strings.Contains(got, "\"z\": 9\n") {
		t.Fatalf("z should be last, without comma:\n%s", got)
	}
}

func TestRenderNestedConflictIndentation(t *testing.T) {
	got := renderMerge(t,
		`{"deps": {"zod": "^1"}}`,
		`{"deps": {"zod": "^2"}}`,
		`{"deps": {"zod": "^3"}}`,
		defOpt())
	want := `{
  "deps": {
<<<<<<< ours
    "zod": "^2"
=======
    "zod": "^3"
>>>>>>> theirs
  }
}
`
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRenderCRLFStyleAppliesToMarkers(t *testing.T) {
	st := jsonval.Style{Indent: "  ", Newline: "\r\n", TrailingNewline: true}
	got := renderMerge(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`, Options{Style: st})
	if !strings.Contains(got, "<<<<<<< ours\r\n") {
		t.Fatalf("markers should use CRLF:\n%q", got)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("stray bare LF in CRLF document:\n%q", got)
	}
}

func TestRenderResolvableSidesStayValidJSON(t *testing.T) {
	// Taking either side of every marker block must yield valid JSON —
	// that is the whole point of line-level markers.
	got := renderMerge(t,
		`{"a": 1, "b": {"c": 1}, "z": [1]}`,
		`{"a": 2, "b": {"c": 2}, "z": [1, 5]}`,
		`{"a": 3, "b": {"c": 3}, "z": [1, 6]}`,
		defOpt())
	take := func(keepOurs bool) string {
		var out []string
		keep, inConflict := true, false
		for _, line := range strings.Split(got, "\n") {
			switch {
			case strings.HasPrefix(line, "<<<<<<<"):
				inConflict, keep = true, keepOurs
			case strings.HasPrefix(line, "======="):
				keep = !keepOurs
			case strings.HasPrefix(line, ">>>>>>>"):
				inConflict, keep = false, true
			default:
				if !inConflict || keep {
					out = append(out, line)
				}
			}
		}
		return strings.Join(out, "\n")
	}
	for _, side := range []bool{true, false} {
		if _, err := jsonval.Parse([]byte(take(side))); err != nil {
			t.Fatalf("side ours=%v does not parse: %v\n%s", side, err, take(side))
		}
	}
}
