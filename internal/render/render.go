// Package render serializes a merge.Tree back to bytes. Resolved regions
// come out as clean JSON in the detected source style; unresolved
// conflicts materialize as git-style conflict markers wrapped around whole
// lines, exactly where the collision sits, so every editor and merge tool
// that understands "<<<<<<<" can take over from there.
package render

import (
	"bytes"
	"strings"

	"github.com/JaydenCJ/keymerge/internal/jsonval"
	"github.com/JaydenCJ/keymerge/internal/merge"
)

// Options controls marker shape and output style.
type Options struct {
	Style       jsonval.Style
	MarkerSize  int    // marker character count; git passes %L, default 7
	OursLabel   string // text after "<<<<<<<", default "ours"
	TheirsLabel string // text after ">>>>>>>", default "theirs"
}

// Render serializes the merged tree. A nil tree (document deleted on both
// sides) renders as no bytes at all.
func Render(t *merge.Tree, opt Options) []byte {
	if opt.MarkerSize <= 0 {
		opt.MarkerSize = 7
	}
	if opt.OursLabel == "" {
		opt.OursLabel = "ours"
	}
	if opt.TheirsLabel == "" {
		opt.TheirsLabel = "theirs"
	}
	if opt.Style.Indent == "" {
		opt.Style.Indent = "  "
	}
	if opt.Style.Newline == "" {
		opt.Style.Newline = "\n"
	}
	if t == nil {
		return nil
	}
	r := &renderer{
		st:    opt.Style,
		begin: strings.Repeat("<", opt.MarkerSize) + " " + opt.OursLabel,
		mid:   strings.Repeat("=", opt.MarkerSize),
		end:   strings.Repeat(">", opt.MarkerSize) + " " + opt.TheirsLabel,
	}
	if t.Kind == merge.NodeConflict {
		r.rootConflict(t.Conf)
		return r.buf.Bytes()
	}
	r.node(t, 0)
	if r.st.TrailingNewline {
		r.nl()
	}
	return r.buf.Bytes()
}

type renderer struct {
	buf   bytes.Buffer
	st    jsonval.Style
	begin string
	mid   string
	end   string
}

func (r *renderer) nl() { r.buf.WriteString(r.st.Newline) }

func (r *renderer) pad(depth int) {
	for i := 0; i < depth; i++ {
		r.buf.WriteString(r.st.Indent)
	}
}

// rootConflict wraps the two whole documents in markers — the only shape
// possible when the top-level values collide (e.g. object vs array).
func (r *renderer) rootConflict(c *merge.Conflict) {
	r.buf.WriteString(r.begin)
	r.nl()
	if c.Ours != nil {
		jsonval.Append(&r.buf, c.Ours, r.st, 0)
		r.nl()
	}
	r.buf.WriteString(r.mid)
	r.nl()
	if c.Theirs != nil {
		jsonval.Append(&r.buf, c.Theirs, r.st, 0)
		r.nl()
	}
	r.buf.WriteString(r.end)
	if r.st.TrailingNewline {
		r.nl()
	}
}

func (r *renderer) node(t *merge.Tree, depth int) {
	switch t.Kind {
	case merge.NodeLeaf:
		jsonval.Append(&r.buf, t.Leaf, r.st, depth)
	case merge.NodeObject:
		r.object(t.Obj, depth)
	case merge.NodeArray:
		r.array(t.Arr, depth)
	}
}

func (r *renderer) object(entries []merge.ObjEntry, depth int) {
	r.buf.WriteByte('{')
	for i, e := range entries {
		last := i == len(entries)-1
		if e.Node.Kind == merge.NodeConflict {
			r.memberConflict(e.Key, e.Node.Conf, depth+1, last)
			continue
		}
		r.nl()
		r.pad(depth + 1)
		jsonval.AppendQuoted(&r.buf, e.Key)
		r.buf.WriteString(": ")
		r.node(e.Node, depth+1)
		if !last {
			r.buf.WriteByte(',')
		}
	}
	r.nl()
	r.pad(depth)
	r.buf.WriteByte('}')
}

// memberConflict emits markers around the two candidate members. A side
// that deleted the key contributes no line between its markers, which is
// exactly how git renders a delete/edit textual conflict.
func (r *renderer) memberConflict(key string, c *merge.Conflict, depth int, last bool) {
	r.nl()
	r.buf.WriteString(r.begin)
	r.memberSide(key, c.Ours, depth, last)
	r.nl()
	r.buf.WriteString(r.mid)
	r.memberSide(key, c.Theirs, depth, last)
	r.nl()
	r.buf.WriteString(r.end)
}

func (r *renderer) memberSide(key string, v *jsonval.Value, depth int, last bool) {
	if v == nil {
		return
	}
	r.nl()
	r.pad(depth)
	jsonval.AppendQuoted(&r.buf, key)
	r.buf.WriteString(": ")
	jsonval.Append(&r.buf, v, r.st, depth)
	if !last {
		r.buf.WriteByte(',')
	}
}

func (r *renderer) array(elems []*merge.Tree, depth int) {
	r.buf.WriteByte('[')
	for i, e := range elems {
		last := i == len(elems)-1
		if e.Kind == merge.NodeConflict {
			r.elementConflict(e.Conf, depth+1, last)
			continue
		}
		r.nl()
		r.pad(depth + 1)
		r.node(e, depth+1)
		if !last {
			r.buf.WriteByte(',')
		}
	}
	r.nl()
	r.pad(depth)
	r.buf.WriteByte(']')
}

// elementConflict emits markers around the colliding element runs. Splice
// conflicts carry element runs; a plain value conflict (from a recursed
// single-element merge) renders as a one-element run per side.
func (r *renderer) elementConflict(c *merge.Conflict, depth int, last bool) {
	r.nl()
	r.buf.WriteString(r.begin)
	r.elementSide(sideElems(c.Ours, c.Splice), depth, last)
	r.nl()
	r.buf.WriteString(r.mid)
	r.elementSide(sideElems(c.Theirs, c.Splice), depth, last)
	r.nl()
	r.buf.WriteString(r.end)
}

func sideElems(v *jsonval.Value, splice bool) []*jsonval.Value {
	if v == nil {
		return nil
	}
	if splice {
		return v.Arr
	}
	return []*jsonval.Value{v}
}

func (r *renderer) elementSide(elems []*jsonval.Value, depth int, lastEntry bool) {
	for i, e := range elems {
		r.nl()
		r.pad(depth)
		jsonval.Append(&r.buf, e, r.st, depth)
		if i < len(elems)-1 || !lastEntry {
			r.buf.WriteByte(',')
		}
	}
}
