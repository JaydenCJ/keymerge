// Parser tests: RFC 8259 conformance where it matters for merging real
// config files — order preservation, verbatim numbers, precise error
// positions, and the deliberate rejection of duplicate keys.
package jsonval

import (
	"errors"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Value {
	t.Helper()
	v, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return v
}

func parseErr(t *testing.T, src string) *SyntaxError {
	t.Helper()
	_, err := Parse([]byte(src))
	if err == nil {
		t.Fatalf("Parse(%q): expected error, got none", src)
	}
	var se *SyntaxError
	if !errors.As(err, &se) {
		t.Fatalf("Parse(%q): error is %T, want *SyntaxError", src, err)
	}
	return se
}

func TestParseScalarLiterals(t *testing.T) {
	if v := mustParse(t, "null"); v.Kind != Null {
		t.Fatalf("null parsed as %v", v.Kind)
	}
	if v := mustParse(t, "true"); v.Kind != Bool || !v.B {
		t.Fatalf("true parsed wrong: %+v", v)
	}
	if v := mustParse(t, "false"); v.Kind != Bool || v.B {
		t.Fatalf("false parsed wrong: %+v", v)
	}
}

func TestParseStringEscapes(t *testing.T) {
	if v := mustParse(t, `"hello world"`); v.Str != "hello world" {
		t.Fatalf("got %+v", v)
	}
	v := mustParse(t, `"\" \\ \/ \b \f \n \r \t"`)
	want := "\" \\ / \b \f \n \r \t"
	if v.Str != want {
		t.Fatalf("got %q, want %q", v.Str, want)
	}
}

func TestParseUnicodeStrings(t *testing.T) {
	// \u escapes, raw UTF-8 passthrough, a proper surrogate pair, and —
	// matching encoding/json's leniency — an unpaired surrogate decoding
	// to U+FFFD, so any file the ecosystem accepts still merges.
	if v := mustParse(t, `"café"`); v.Str != "café" {
		t.Fatalf("\\u escape: got %q", v.Str)
	}
	if v := mustParse(t, `"日本語のキー"`); v.Str != "日本語のキー" {
		t.Fatalf("raw UTF-8: got %q", v.Str)
	}
	if v := mustParse(t, `"😀"`); v.Str != "😀" {
		t.Fatalf("surrogate pair: got %q", v.Str)
	}
	if v := mustParse(t, `"\ud83dx"`); v.Str != "�x" {
		t.Fatalf("lone surrogate: got %q", v.Str)
	}
}

func TestParseBadStringsRejected(t *testing.T) {
	parseErr(t, "\"a\x01b\"") // raw control character
	parseErr(t, `"\q"`)       // unknown escape
	parseErr(t, `"\u12g4"`)   // bad hex digit
}

func TestParseNumberLiteralsKeptVerbatim(t *testing.T) {
	// The raw literal must survive: reformatting numbers corrupts files
	// that rely on big integers or fixed-point notation.
	for _, lit := range []string{"0", "-1", "3.14", "1e10", "-2.5E-3", "12345678901234567890", "0.50"} {
		v := mustParse(t, lit)
		if v.Kind != Number || v.Num != lit {
			t.Fatalf("literal %q parsed as %+v", lit, v)
		}
	}
}

func TestParseMalformedNumbersRejected(t *testing.T) {
	for _, lit := range []string{"-", "1.", ".5", "1e", "1e+", "+1", "0x10"} {
		if _, err := Parse([]byte(lit)); err == nil {
			t.Fatalf("literal %q should not parse", lit)
		}
	}
}

func TestParseObjectPreservesMemberOrder(t *testing.T) {
	v := mustParse(t, `{"zebra": 1, "apple": 2, "mango": 3}`)
	got := make([]string, len(v.Mem))
	for i, m := range v.Mem {
		got[i] = m.Key
	}
	if strings.Join(got, ",") != "zebra,apple,mango" {
		t.Fatalf("member order %v", got)
	}
}

func TestParseDuplicateKeyRejected(t *testing.T) {
	se := parseErr(t, `{"a": 1, "a": 2}`)
	if !strings.Contains(se.Msg, `duplicate object key "a"`) {
		t.Fatalf("message %q", se.Msg)
	}
}

func TestParseNestedAndEmptyContainers(t *testing.T) {
	v := mustParse(t, `{"a": [1, {"b": [true, null]}], "c": {}}`)
	inner := v.Get("a").Arr[1].Get("b")
	if inner.Arr[0].Kind != Bool || inner.Arr[1].Kind != Null {
		t.Fatalf("nested access failed: %+v", inner)
	}
	if c := v.Get("c"); c.Kind != Object || len(c.Mem) != 0 {
		t.Fatalf("empty object: %+v", c)
	}
	if a := mustParse(t, `[ ]`); a.Kind != Array || len(a.Arr) != 0 {
		t.Fatalf("empty array: %+v", a)
	}
}

func TestParseWhitespaceTolerance(t *testing.T) {
	mustParse(t, "\r\n\t {\n\"a\" \t:\r [ 1 ,\t2 ]\n}\r\n")
}

func TestParseTrailingContentRejected(t *testing.T) {
	se := parseErr(t, `{"a": 1} {"b": 2}`)
	if !strings.Contains(se.Msg, "after top-level value") {
		t.Fatalf("message %q", se.Msg)
	}
	parseErr(t, `{"a": 1,}`) // trailing comma in object
	parseErr(t, `[1, 2,]`)   // trailing comma in array
}

func TestParseErrorReportsLineAndColumn(t *testing.T) {
	// The colon is missing on line 3; a merge driver's user needs to know
	// exactly where their file broke, not just that it did.
	se := parseErr(t, "{\n  \"a\": 1,\n  \"b\" 2\n}")
	if se.Line != 3 {
		t.Fatalf("line = %d, want 3 (err: %v)", se.Line, se)
	}
	if !strings.Contains(se.Error(), "line 3, column") {
		t.Fatalf("Error() = %q", se.Error())
	}
}

func TestParseIncompleteInputsRejected(t *testing.T) {
	for _, src := range []string{"", "   \n\t ", `"abc`, `{"a": 1`, `[1, 2`, `{"a"`, `tru`} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Fatalf("input %q should not parse", src)
		}
	}
}

func TestParseDepthLimit(t *testing.T) {
	deep := strings.Repeat("[", maxDepth+1) + strings.Repeat("]", maxDepth+1)
	if _, err := Parse([]byte(deep)); err == nil {
		t.Fatal("expected depth-limit error")
	}
	fine := strings.Repeat("[", 100) + "1" + strings.Repeat("]", 100)
	mustParse(t, fine)
}
