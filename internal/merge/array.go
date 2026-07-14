package merge

import (
	"fmt"

	"github.com/JaydenCJ/keymerge/internal/jsonval"
)

// maxLCSCells bounds the LCS dynamic-programming table (rows × columns).
// Config arrays are tiny; anything past this limit is almost certainly
// generated data where element-level merging has no meaning, so the whole
// array degrades to a single atomic conflict instead of burning memory.
const maxLCSCells = 4_000_000

// array3 is diff3 over JSON arrays: elements are aligned against the base
// by semantic equality (via canonical forms), regions where only one side
// changed take that side, and only overlapping different changes conflict.
// A both-edited region of exactly one element on every side recurses into
// a value merge, so arrays of objects merge field-by-field.
func (m *merger) array3(path string, base, ours, theirs []*jsonval.Value) *Tree {
	if tooBig(len(base), len(ours)) || tooBig(len(base), len(theirs)) {
		return m.conflict(path, EditEdit,
			jsonval.NewArray(base...), jsonval.NewArray(ours...), jsonval.NewArray(theirs...))
	}
	bc, oc, tc := canonAll(base), canonAll(ours), canonAll(theirs)
	matchO := lcsMatch(bc, oc)
	matchT := lcsMatch(bc, tc)

	var out []*Tree
	b0, o0, t0 := 0, 0, 0
	emitGap := func(b1, o1, t1 int) {
		switch {
		case seqEq(oc[o0:o1], bc[b0:b1]) && seqEq(tc[t0:t1], bc[b0:b1]):
			// Nothing changed in this region.
			for _, e := range ours[o0:o1] {
				out = append(out, leaf(e))
			}
		case seqEq(tc[t0:t1], bc[b0:b1]):
			// Only ours changed: take ours.
			for _, e := range ours[o0:o1] {
				out = append(out, leaf(e))
			}
		case seqEq(oc[o0:o1], bc[b0:b1]):
			// Only theirs changed: take theirs.
			for _, e := range theirs[t0:t1] {
				out = append(out, leaf(e))
			}
		case seqEq(oc[o0:o1], tc[t0:t1]):
			// Both sides made the same change: converged.
			for _, e := range ours[o0:o1] {
				out = append(out, leaf(e))
			}
		case b1-b0 == 1 && o1-o0 == 1 && t1-t0 == 1:
			// Both edited the same single element: merge it structurally.
			out = append(out, m.value(fmt.Sprintf("%s/%d", path, o0), base[b0], ours[o0], theirs[t0]))
		default:
			out = append(out, m.spliceConflict(path, base[b0:b1], ours[o0:o1], theirs[t0:t1], o0))
		}
	}
	for bi := range base {
		if matchO[bi] < 0 || matchT[bi] < 0 {
			continue
		}
		// base[bi] survives unchanged on both sides: a stable anchor.
		emitGap(bi, matchO[bi], matchT[bi])
		out = append(out, leaf(ours[matchO[bi]]))
		b0, o0, t0 = bi+1, matchO[bi]+1, matchT[bi]+1
	}
	emitGap(len(base), len(ours), len(theirs))

	allLeaf := true
	for _, t := range out {
		if t.Kind != NodeLeaf {
			allLeaf = false
			break
		}
	}
	if allLeaf {
		elems := make([]*jsonval.Value, len(out))
		for i, t := range out {
			elems[i] = t.Leaf
		}
		return leaf(jsonval.NewArray(elems...))
	}
	return &Tree{Kind: NodeArray, Arr: out}
}

// spliceConflict records an overlapping array-region collision. The runs
// are wrapped in array values so the renderer can splice each side's
// elements into the surrounding array between conflict markers.
func (m *merger) spliceConflict(path string, base, ours, theirs []*jsonval.Value, oursStart int) *Tree {
	kind := EditEdit
	switch {
	case len(base) == 0:
		kind = AddAdd
	case len(ours) == 0:
		kind = DeleteEdit
	case len(theirs) == 0:
		kind = EditDelete
	}
	c := &Conflict{
		Path:   fmt.Sprintf("%s/%d", path, oursStart),
		Kind:   kind,
		Base:   jsonval.NewArray(base...),
		Ours:   jsonval.NewArray(ours...),
		Theirs: jsonval.NewArray(theirs...),
		Splice: true,
	}
	m.conflicts = append(m.conflicts, *c)
	return &Tree{Kind: NodeConflict, Conf: c}
}

func tooBig(n, m int) bool {
	return n > 0 && m > 0 && n*m > maxLCSCells
}

func canonAll(vals []*jsonval.Value) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = jsonval.Canon(v)
	}
	return out
}

func seqEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lcsMatch computes a longest-common-subsequence alignment between a and
// b and returns, for every index of a, the matched index in b or -1. The
// classic O(n·m) table is fine here because tooBig caps the input size.
func lcsMatch(a, b []string) []int {
	match := make([]int, len(a))
	for i := range match {
		match[i] = -1
	}
	if len(a) == 0 || len(b) == 0 {
		return match
	}
	// dp[i][j] = LCS length of a[i:], b[j:].
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	// Walk the table to recover one alignment.
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			match[i] = j
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return match
}
