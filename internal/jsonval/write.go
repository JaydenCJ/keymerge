package jsonval

import (
	"bytes"
	"strings"
)

// Style controls serialization so a merged file keeps looking like the
// file the team already has: same indent unit, same newline flavor, same
// presence of a final newline.
type Style struct {
	Indent          string // one indentation unit, e.g. "  ", "    ", "\t"
	Newline         string // "\n" or "\r\n"
	TrailingNewline bool   // whether the document ends with a newline
}

// DefaultStyle is two-space indentation with LF newlines — the dominant
// convention for package.json and friends.
func DefaultStyle() Style {
	return Style{Indent: "  ", Newline: "\n", TrailingNewline: true}
}

// DetectStyle infers a Style from raw document bytes: the indent of the
// first indented line becomes the unit, CRLF anywhere selects CRLF output,
// and the final byte decides the trailing newline. Compact or empty
// documents fall back to DefaultStyle.
func DetectStyle(data []byte) Style {
	st := DefaultStyle()
	if len(data) == 0 {
		return st
	}
	if bytes.Contains(data, []byte("\r\n")) {
		st.Newline = "\r\n"
	}
	st.TrailingNewline = data[len(data)-1] == '\n'
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 || len(trimmed) == len(line) {
			continue // blank or unindented line tells us nothing
		}
		lead := line[:len(line)-len(trimmed)]
		if bytes.ContainsRune(lead, '\t') {
			st.Indent = "\t"
		} else {
			st.Indent = strings.Repeat(" ", len(lead))
		}
		break
	}
	return st
}

// Write serializes v as a complete document in the given style.
func Write(v *Value, st Style) []byte {
	var b bytes.Buffer
	Append(&b, v, st, 0)
	if st.TrailingNewline {
		b.WriteString(st.Newline)
	}
	return b.Bytes()
}

// Append writes v into b at the given depth. The first token is written at
// the current cursor (the caller has already indented); nested lines are
// indented depth+1 units and the closing bracket lands at depth. The
// conflict renderer relies on this contract to splice values mid-document.
func Append(b *bytes.Buffer, v *Value, st Style, depth int) {
	switch v.Kind {
	case Null:
		b.WriteString("null")
	case Bool:
		if v.B {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case Number:
		b.WriteString(v.Num)
	case String:
		AppendQuoted(b, v.Str)
	case Array:
		if len(v.Arr) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range v.Arr {
			b.WriteString(st.Newline)
			writeIndent(b, st, depth+1)
			Append(b, e, st, depth+1)
			if i < len(v.Arr)-1 {
				b.WriteByte(',')
			}
		}
		b.WriteString(st.Newline)
		writeIndent(b, st, depth)
		b.WriteByte(']')
	case Object:
		if len(v.Mem) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, m := range v.Mem {
			b.WriteString(st.Newline)
			writeIndent(b, st, depth+1)
			AppendQuoted(b, m.Key)
			b.WriteString(": ")
			Append(b, m.Val, st, depth+1)
			if i < len(v.Mem)-1 {
				b.WriteByte(',')
			}
		}
		b.WriteString(st.Newline)
		writeIndent(b, st, depth)
		b.WriteByte('}')
	}
}

func writeIndent(b *bytes.Buffer, st Style, depth int) {
	for i := 0; i < depth; i++ {
		b.WriteString(st.Indent)
	}
}

const hexDigits = "0123456789abcdef"

// AppendQuoted writes s as a JSON string literal. Escaping is minimal:
// only the quote, the backslash and control characters are escaped; all
// other UTF-8 content is emitted raw, keeping diffs human-readable.
func AppendQuoted(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
}
