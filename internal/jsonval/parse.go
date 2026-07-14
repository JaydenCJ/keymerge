package jsonval

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// maxDepth bounds parser recursion so a hostile document cannot overflow
// the goroutine stack. 2000 levels is far beyond any real config file.
const maxDepth = 2000

// SyntaxError describes a JSON parse failure with 1-based position info,
// so a merge driver can tell the user exactly where their file broke.
type SyntaxError struct {
	Line int
	Col  int
	Msg  string
}

func (e *SyntaxError) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Col, e.Msg)
}

// Parse decodes one complete JSON document per RFC 8259. Object member
// order is preserved, number literals are kept verbatim, and duplicate
// object keys are rejected outright — a key-level merge over duplicates
// would be ambiguous, and in config files they are always a bug.
func Parse(data []byte) (*Value, error) {
	p := &parser{data: data}
	p.skipWS()
	if p.pos >= len(p.data) {
		return nil, p.errf("empty document")
	}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.skipWS()
	if p.pos < len(p.data) {
		return nil, p.errf("unexpected content after top-level value")
	}
	return v, nil
}

type parser struct {
	data  []byte
	pos   int
	depth int
}

// errf builds a SyntaxError at the current byte offset, translating the
// offset into a 1-based line and column.
func (p *parser) errf(format string, args ...any) *SyntaxError {
	line, col := 1, 1
	for i := 0; i < p.pos && i < len(p.data); i++ {
		if p.data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return &SyntaxError{Line: line, Col: col, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) skipWS() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) value() (*Value, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		return nil, p.errf("document exceeds maximum nesting depth %d", maxDepth)
	}
	if p.pos >= len(p.data) {
		return nil, p.errf("unexpected end of input")
	}
	switch c := p.data[p.pos]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		s, err := p.stringLit()
		if err != nil {
			return nil, err
		}
		return &Value{Kind: String, Str: s}, nil
	case c == 't':
		return p.literal("true", &Value{Kind: Bool, B: true})
	case c == 'f':
		return p.literal("false", &Value{Kind: Bool})
	case c == 'n':
		return p.literal("null", &Value{Kind: Null})
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	default:
		return nil, p.errf("unexpected character %q", string(c))
	}
}

func (p *parser) literal(word string, v *Value) (*Value, error) {
	if p.pos+len(word) > len(p.data) || string(p.data[p.pos:p.pos+len(word)]) != word {
		return nil, p.errf("invalid literal (expected %q)", word)
	}
	p.pos += len(word)
	return v, nil
}

func (p *parser) object() (*Value, error) {
	p.pos++ // consume '{'
	v := &Value{Kind: Object}
	p.skipWS()
	if p.pos < len(p.data) && p.data[p.pos] == '}' {
		p.pos++
		return v, nil
	}
	seen := make(map[string]bool)
	for {
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != '"' {
			return nil, p.errf("expected object key string")
		}
		key, err := p.stringLit()
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, p.errf("duplicate object key %q", key)
		}
		seen[key] = true
		p.skipWS()
		if p.pos >= len(p.data) || p.data[p.pos] != ':' {
			return nil, p.errf("expected ':' after object key %q", key)
		}
		p.pos++
		p.skipWS()
		val, err := p.value()
		if err != nil {
			return nil, err
		}
		v.Mem = append(v.Mem, Member{Key: key, Val: val})
		p.skipWS()
		if p.pos >= len(p.data) {
			return nil, p.errf("unterminated object")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return v, nil
		default:
			return nil, p.errf("expected ',' or '}' in object")
		}
	}
}

func (p *parser) array() (*Value, error) {
	p.pos++ // consume '['
	v := &Value{Kind: Array}
	p.skipWS()
	if p.pos < len(p.data) && p.data[p.pos] == ']' {
		p.pos++
		return v, nil
	}
	for {
		p.skipWS()
		el, err := p.value()
		if err != nil {
			return nil, err
		}
		v.Arr = append(v.Arr, el)
		p.skipWS()
		if p.pos >= len(p.data) {
			return nil, p.errf("unterminated array")
		}
		switch p.data[p.pos] {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return v, nil
		default:
			return nil, p.errf("expected ',' or ']' in array")
		}
	}
}

// stringLit decodes a JSON string starting at the opening quote.
func (p *parser) stringLit() (string, error) {
	p.pos++ // consume opening '"'
	var sb strings.Builder
	for {
		if p.pos >= len(p.data) {
			return "", p.errf("unterminated string")
		}
		c := p.data[p.pos]
		switch {
		case c == '"':
			p.pos++
			return sb.String(), nil
		case c == '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", p.errf("unterminated escape sequence")
			}
			if err := p.escape(&sb); err != nil {
				return "", err
			}
		case c < 0x20:
			return "", p.errf("raw control character 0x%02x in string", c)
		default:
			// UTF-8 payload bytes pass through unchanged.
			sb.WriteByte(c)
			p.pos++
		}
	}
}

func (p *parser) escape(sb *strings.Builder) error {
	c := p.data[p.pos]
	p.pos++
	switch c {
	case '"':
		sb.WriteByte('"')
	case '\\':
		sb.WriteByte('\\')
	case '/':
		sb.WriteByte('/')
	case 'b':
		sb.WriteByte('\b')
	case 'f':
		sb.WriteByte('\f')
	case 'n':
		sb.WriteByte('\n')
	case 'r':
		sb.WriteByte('\r')
	case 't':
		sb.WriteByte('\t')
	case 'u':
		r, err := p.hex4()
		if err != nil {
			return err
		}
		if utf16.IsSurrogate(r) {
			// Combine a valid surrogate pair; an unpaired surrogate decodes
			// to U+FFFD, the same lenient behavior as encoding/json, so
			// real-world files that encoding/json accepts still merge.
			if p.pos+1 < len(p.data) && p.data[p.pos] == '\\' && p.data[p.pos+1] == 'u' {
				save := p.pos
				p.pos += 2
				r2, err := p.hex4()
				if err != nil {
					return err
				}
				if dec := utf16.DecodeRune(r, r2); dec != utf8.RuneError {
					sb.WriteRune(dec)
					return nil
				}
				p.pos = save
			}
			sb.WriteRune(utf8.RuneError)
			return nil
		}
		sb.WriteRune(r)
	default:
		return p.errf("invalid escape sequence \\%s", string(c))
	}
	return nil
}

// hex4 reads exactly four hex digits of a \u escape.
func (p *parser) hex4() (rune, error) {
	if p.pos+4 > len(p.data) {
		return 0, p.errf("truncated \\u escape")
	}
	var r rune
	for i := 0; i < 4; i++ {
		c := p.data[p.pos+i]
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r |= rune(c-'A') + 10
		default:
			return 0, p.errf("invalid hex digit %q in \\u escape", string(c))
		}
	}
	p.pos += 4
	return r, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// number validates the RFC 8259 number grammar and stores the raw literal,
// so 1e10, -0.50 and 64-bit-overflowing integers survive byte-for-byte.
func (p *parser) number() (*Value, error) {
	start := p.pos
	if p.data[p.pos] == '-' {
		p.pos++
	}
	switch {
	case p.pos < len(p.data) && p.data[p.pos] == '0':
		p.pos++
	case p.pos < len(p.data) && p.data[p.pos] >= '1' && p.data[p.pos] <= '9':
		for p.pos < len(p.data) && isDigit(p.data[p.pos]) {
			p.pos++
		}
	default:
		return nil, p.errf("invalid number literal")
	}
	if p.pos < len(p.data) && p.data[p.pos] == '.' {
		p.pos++
		if p.pos >= len(p.data) || !isDigit(p.data[p.pos]) {
			return nil, p.errf("digit required after decimal point")
		}
		for p.pos < len(p.data) && isDigit(p.data[p.pos]) {
			p.pos++
		}
	}
	if p.pos < len(p.data) && (p.data[p.pos] == 'e' || p.data[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.data) && (p.data[p.pos] == '+' || p.data[p.pos] == '-') {
			p.pos++
		}
		if p.pos >= len(p.data) || !isDigit(p.data[p.pos]) {
			return nil, p.errf("digit required in exponent")
		}
		for p.pos < len(p.data) && isDigit(p.data[p.pos]) {
			p.pos++
		}
	}
	return &Value{Kind: Number, Num: string(p.data[start:p.pos])}, nil
}
