// Writer and style-detection tests: the merged file must keep looking
// like the file the team already formats — indent unit, newline flavor
// and trailing newline all preserved.
package jsonval

import (
	"bytes"
	"testing"
)

func TestWriteRoundTripPreservesOrderAndNumbers(t *testing.T) {
	src := "{\n  \"zebra\": 1.50e3,\n  \"apple\": 12345678901234567890,\n  \"list\": [\n    1,\n    2\n  ]\n}\n"
	v := mustParse(t, src)
	if got := string(Write(v, DefaultStyle())); got != src {
		t.Fatalf("round trip changed the document:\n%q\nwant\n%q", got, src)
	}
}

func TestWriteEmptyContainers(t *testing.T) {
	src := "{\n  \"a\": {},\n  \"b\": []\n}\n"
	if got := string(Write(mustParse(t, src), DefaultStyle())); got != src {
		t.Fatalf("got %q", got)
	}
}

func TestWriteStringEscaping(t *testing.T) {
	v := &Value{Kind: String, Str: "a\"b\\c\nd\x01e"}
	got := string(Write(v, Style{Indent: "  ", Newline: "\n"}))
	want := `"a\"b\\c\nd` + `\u0001` + `e"`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestWriteTabIndent(t *testing.T) {
	v := mustParse(t, `{"a": [1]}`)
	got := string(Write(v, Style{Indent: "\t", Newline: "\n", TrailingNewline: true}))
	want := "{\n\t\"a\": [\n\t\t1\n\t]\n}\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteNewlineVariants(t *testing.T) {
	v := mustParse(t, `{"a": 1}`)
	got := string(Write(v, Style{Indent: "  ", Newline: "\r\n", TrailingNewline: true}))
	if got != "{\r\n  \"a\": 1\r\n}\r\n" {
		t.Fatalf("CRLF: got %q", got)
	}
	if got := string(Write(mustParse(t, `1`), Style{Indent: "  ", Newline: "\n", TrailingNewline: false})); got != "1" {
		t.Fatalf("no trailing newline: got %q", got)
	}
}

func TestAppendAtDepthIndentsNestedLines(t *testing.T) {
	// The conflict renderer splices values mid-document: the first line
	// starts at the cursor, nested lines and the closer use the depth.
	var b bytes.Buffer
	Append(&b, mustParse(t, `{"a": 1}`), DefaultStyle(), 2)
	want := "{\n      \"a\": 1\n    }"
	if b.String() != want {
		t.Fatalf("got %q, want %q", b.String(), want)
	}
}

func TestDetectStyleIndentUnits(t *testing.T) {
	if st := DetectStyle([]byte("{\n  \"a\": 1\n}\n")); st.Indent != "  " {
		t.Fatalf("two-space doc detected as %q", st.Indent)
	}
	if st := DetectStyle([]byte("{\n    \"a\": 1\n}\n")); st.Indent != "    " {
		t.Fatalf("four-space doc detected as %q", st.Indent)
	}
	if st := DetectStyle([]byte("{\n\t\"a\": 1\n}\n")); st.Indent != "\t" {
		t.Fatalf("tab doc detected as %q", st.Indent)
	}
	// A compact document reveals no indent: fall back to the default.
	if st := DetectStyle([]byte(`{"a":1}`)); st.Indent != "  " || st.Newline != "\n" {
		t.Fatalf("compact doc: got %+v", st)
	}
}

func TestDetectStyleNewlines(t *testing.T) {
	st := DetectStyle([]byte("{\r\n  \"a\": 1\r\n}\r\n"))
	if st.Newline != "\r\n" || !st.TrailingNewline {
		t.Fatalf("got %+v", st)
	}
	if st := DetectStyle([]byte("{\n  \"a\": 1\n}")); st.TrailingNewline {
		t.Fatal("missing final newline not detected")
	}
}
