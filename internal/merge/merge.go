// Package merge implements a key-level three-way merge of JSON documents.
// Instead of comparing lines like git's default driver, it compares the
// document structure against the common ancestor: a change survives when
// only one side made it, converges when both sides made the same one, and
// becomes a conflict only when both sides changed the same key (or the
// same array region) to different values — a real collision.
package merge

import (
	"github.com/JaydenCJ/keymerge/internal/jsonval"
)

// Strategy selects how an array that changed on both sides is merged.
type Strategy int

const (
	// Arrays3Way aligns elements against the base with an LCS diff and
	// merges non-overlapping insertions, deletions and edits (default).
	Arrays3Way Strategy = iota
	// ArraysAtomic treats a both-changed array as a single conflict.
	ArraysAtomic
	// ArraysUnion keeps ours, then appends theirs' additions; an element
	// removed by either side stays removed. Never conflicts — meant for
	// order-insensitive scalar lists like keywords or file globs.
	ArraysUnion
)

// ParseStrategy maps a CLI flag value to a Strategy.
func ParseStrategy(s string) (Strategy, bool) {
	switch s {
	case "merge":
		return Arrays3Way, true
	case "atomic":
		return ArraysAtomic, true
	case "union":
		return ArraysUnion, true
	}
	return 0, false
}

// Options configures a merge.
type Options struct {
	Arrays Strategy
}

// ConflictKind classifies why a collision could not be auto-resolved.
type ConflictKind string

const (
	EditEdit   ConflictKind = "edit/edit"   // both sides changed the value differently
	AddAdd     ConflictKind = "add/add"     // both sides added different values
	DeleteEdit ConflictKind = "delete/edit" // ours deleted, theirs edited
	EditDelete ConflictKind = "edit/delete" // ours edited, theirs deleted
	TypeClash  ConflictKind = "type"        // both sides changed to different kinds
)

// Conflict is one real collision, addressed by an RFC 6901 JSON Pointer.
type Conflict struct {
	Path   string // JSON Pointer to the collision; "" is the document root
	Kind   ConflictKind
	Base   *jsonval.Value // nil when the value is absent in the base
	Ours   *jsonval.Value // nil when ours deleted it
	Theirs *jsonval.Value // nil when theirs deleted it
	// Splice marks an array-run conflict: Ours and Theirs then hold runs
	// of elements (as array values) that belong inside the surrounding
	// array, not standalone values. Path points at the run's first index
	// on the ours side.
	Splice bool
}

// NodeKind discriminates merge tree nodes.
type NodeKind int

const (
	NodeLeaf     NodeKind = iota // fully resolved subtree
	NodeObject                   // object that still contains conflicts below
	NodeArray                    // array that still contains conflicts below
	NodeConflict                 // an unresolved collision
)

// ObjEntry is one member of a partially merged object.
type ObjEntry struct {
	Key  string
	Node *Tree
}

// Tree is the merged document with conflicts embedded at their exact
// positions, so the renderer can put markers precisely where the collision
// is instead of failing the whole file. Conflict-free subtrees collapse to
// a single Leaf; a document that merged cleanly is always one Leaf.
type Tree struct {
	Kind NodeKind
	Leaf *jsonval.Value // NodeLeaf
	Obj  []ObjEntry     // NodeObject
	Arr  []*Tree        // NodeArray
	Conf *Conflict      // NodeConflict
}

// Result carries the merged tree and the flat list of collisions.
type Result struct {
	Tree      *Tree // nil when the merged document is absent (deleted on both sides)
	Conflicts []Conflict
}

// Merge performs the three-way merge. Any of base, ours or theirs may be
// nil, meaning that side has no document (git passes an empty ancestor for
// add/add merges).
func Merge(base, ours, theirs *jsonval.Value, opt Options) *Result {
	m := &merger{opt: opt}
	t := m.value("", base, ours, theirs)
	return &Result{Tree: t, Conflicts: m.conflicts}
}

type merger struct {
	opt       Options
	conflicts []Conflict
}

func leaf(v *jsonval.Value) *Tree { return &Tree{Kind: NodeLeaf, Leaf: v} }

// value merges one node. A nil return means the merged value is absent
// (the member disappears). The rule order below is the whole algorithm:
//
//	ours == theirs          → converged (also covers: both unchanged, both deleted)
//	ours == base            → only theirs changed: take theirs
//	theirs == base          → only ours changed: take ours
//	otherwise               → both changed differently: recurse into
//	                          containers, conflict on scalars
func (m *merger) value(path string, base, ours, theirs *jsonval.Value) *Tree {
	if jsonval.Equal(ours, theirs) {
		if ours == nil {
			return nil
		}
		return leaf(ours)
	}
	if jsonval.Equal(ours, base) {
		if theirs == nil {
			return nil
		}
		return leaf(theirs)
	}
	if jsonval.Equal(theirs, base) {
		if ours == nil {
			return nil
		}
		return leaf(ours)
	}
	switch {
	case ours == nil:
		return m.conflict(path, DeleteEdit, base, ours, theirs)
	case theirs == nil:
		return m.conflict(path, EditDelete, base, ours, theirs)
	case ours.Kind == jsonval.Object && theirs.Kind == jsonval.Object:
		// Both sides hold objects (base may be absent or another kind);
		// descend so only genuinely colliding keys conflict.
		return m.object(path, base, ours, theirs)
	case ours.Kind == jsonval.Array && theirs.Kind == jsonval.Array:
		return m.array(path, base, ours, theirs)
	case base == nil:
		return m.conflict(path, AddAdd, base, ours, theirs)
	case ours.Kind != theirs.Kind:
		return m.conflict(path, TypeClash, base, ours, theirs)
	default:
		return m.conflict(path, EditEdit, base, ours, theirs)
	}
}

func (m *merger) conflict(path string, kind ConflictKind, base, ours, theirs *jsonval.Value) *Tree {
	c := &Conflict{Path: path, Kind: kind, Base: base, Ours: ours, Theirs: theirs}
	m.conflicts = append(m.conflicts, *c)
	return &Tree{Kind: NodeConflict, Conf: c}
}

// object merges two objects member-wise against the base. Key order in the
// result follows ours; keys only theirs added are inserted right after the
// nearest preceding key they share with ours, so additions stay in context
// instead of piling up at the end.
func (m *merger) object(path string, base, ours, theirs *jsonval.Value) *Tree {
	keys := unionKeys(ours, theirs)
	entries := make([]ObjEntry, 0, len(keys))
	allLeaf := true
	for _, k := range keys {
		child := m.value(path+"/"+EscapePointer(k), memberOf(base, k), memberOf(ours, k), memberOf(theirs, k))
		if child == nil {
			continue // deleted on the winning side
		}
		if child.Kind != NodeLeaf {
			allLeaf = false
		}
		entries = append(entries, ObjEntry{Key: k, Node: child})
	}
	if allLeaf {
		mem := make([]jsonval.Member, len(entries))
		for i, e := range entries {
			mem[i] = jsonval.Member{Key: e.Key, Val: e.Node.Leaf}
		}
		return leaf(jsonval.NewObject(mem...))
	}
	return &Tree{Kind: NodeObject, Obj: entries}
}

// memberOf looks a key up treating absent documents and non-objects as
// having no members. Distinct from an explicit null, which Get returns.
func memberOf(v *jsonval.Value, key string) *jsonval.Value {
	if v == nil || v.Kind != jsonval.Object {
		return nil
	}
	return v.Get(key)
}

// unionKeys returns ours' keys in ours' order, with theirs-only keys
// inserted after the closest preceding key both sides share.
func unionKeys(ours, theirs *jsonval.Value) []string {
	out := make([]string, 0, len(ours.Mem)+len(theirs.Mem))
	for _, mem := range ours.Mem {
		out = append(out, mem.Key)
	}
	index := func(k string) int {
		for i, existing := range out {
			if existing == k {
				return i
			}
		}
		return -1
	}
	anchor := 0 // insertion cursor: just after the last shared or inserted key
	for _, mem := range theirs.Mem {
		if i := index(mem.Key); i >= 0 {
			anchor = i + 1
			continue
		}
		out = append(out, "")
		copy(out[anchor+1:], out[anchor:])
		out[anchor] = mem.Key
		anchor++
	}
	return out
}

// array dispatches a both-changed array to the configured strategy.
func (m *merger) array(path string, base, ours, theirs *jsonval.Value) *Tree {
	var baseArr []*jsonval.Value
	if base != nil && base.Kind == jsonval.Array {
		baseArr = base.Arr
	}
	switch m.opt.Arrays {
	case ArraysAtomic:
		kind := EditEdit
		if base == nil {
			kind = AddAdd
		}
		return m.conflict(path, kind, base, ours, theirs)
	case ArraysUnion:
		return leaf(unionArrays(baseArr, ours.Arr, theirs.Arr))
	default:
		return m.array3(path, baseArr, ours.Arr, theirs.Arr)
	}
}

// unionArrays implements the union strategy: ours as-is, then every theirs
// element that is neither already present nor was deliberately removed by
// ours (present in base but absent from ours).
func unionArrays(base, ours, theirs []*jsonval.Value) *jsonval.Value {
	inOurs := make(map[string]bool, len(ours))
	for _, e := range ours {
		inOurs[jsonval.Canon(e)] = true
	}
	inBase := make(map[string]bool, len(base))
	for _, e := range base {
		inBase[jsonval.Canon(e)] = true
	}
	out := make([]*jsonval.Value, 0, len(ours)+len(theirs))
	out = append(out, ours...)
	for _, e := range theirs {
		c := jsonval.Canon(e)
		if inOurs[c] || inBase[c] {
			continue
		}
		out = append(out, e)
		inOurs[c] = true
	}
	return jsonval.NewArray(out...)
}
