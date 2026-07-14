// Array-merge tests: the diff3 alignment, the recursion into both-edited
// single elements, the union and atomic strategies, and the size guard.
package merge

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JaydenCJ/keymerge/internal/jsonval"
)

func TestArrayOneSideChangeWinsAndConvergence(t *testing.T) {
	res := run(t, `[1, 2, 3]`, `[1, 2, 3, 4]`, `[1, 2, 3]`)
	wantClean(t, res, `[1, 2, 3, 4]`)
	res = run(t, `[1, 2, 3]`, `[1, 2, 3]`, `[9, 2, 3]`)
	wantClean(t, res, `[9, 2, 3]`)
	// Both sides made the same change: converged, no conflict.
	wantClean(t, run(t, `[1, 2]`, `[1, 2, 3]`, `[1, 2, 3]`), `[1, 2, 3]`)
}

func TestArrayNonOverlappingInsertionsMerge(t *testing.T) {
	// Ours prepends, theirs appends: different regions, both land.
	res := run(t, `["b", "c"]`, `["a", "b", "c"]`, `["b", "c", "d"]`)
	wantClean(t, res, `["a", "b", "c", "d"]`)
}

func TestArrayIndependentDeletionsMerge(t *testing.T) {
	res := run(t, `[1, 2, 3, 4]`, `[2, 3, 4]`, `[1, 2, 3]`)
	wantClean(t, res, `[2, 3]`)
}

func TestArrayBothAppendDifferentElementsConflicts(t *testing.T) {
	// Both appended at the same position with different values: that IS
	// a real collision — the relative order of the two additions is
	// genuinely ambiguous, exactly like two people appending to a list.
	res := run(t, `[1]`, `[1, 2]`, `[1, 3]`)
	wantConflicts(t, res, "/1")
	c := res.Conflicts[0]
	if c.Kind != AddAdd || !c.Splice {
		t.Fatalf("got %+v, want spliced add/add", c)
	}
}

func TestArrayDeleteVersusEditConflicts(t *testing.T) {
	res := run(t, `["a", "b", "c"]`, `["a", "c"]`, `["a", "B", "c"]`)
	wantConflicts(t, res, "/1")
	if res.Conflicts[0].Kind != DeleteEdit {
		t.Fatalf("kind = %s", res.Conflicts[0].Kind)
	}
}

func TestArrayOfObjectsRecursesIntoBothEditedElement(t *testing.T) {
	// Both sides edited different fields of the same array element: the
	// single-element region recurses into a key-level merge and stays clean.
	res := run(t,
		`[{"name": "db", "port": 5432, "tls": false}]`,
		`[{"name": "db", "port": 5433, "tls": false}]`,
		`[{"name": "db", "port": 5432, "tls": true}]`)
	wantClean(t, res, `[{"name": "db", "port": 5433, "tls": true}]`)
	// And when the recursion does collide, the path carries the index.
	res = run(t,
		`[{"port": 1}, {"port": 2}]`,
		`[{"port": 1}, {"port": 20}]`,
		`[{"port": 1}, {"port": 30}]`)
	wantConflicts(t, res, "/1/port")
}

func TestArrayEditFarApartMerges(t *testing.T) {
	// Edits at opposite ends with stable middle anchors merge cleanly.
	res := run(t, `[1, 2, 3, 4, 5]`, `[9, 2, 3, 4, 5]`, `[1, 2, 3, 4, 9]`)
	wantClean(t, res, `[9, 2, 3, 4, 9]`)
}

func TestArrayUnionStrategyNeverConflicts(t *testing.T) {
	res := Merge(
		v(t, `{"keywords": ["a", "b"]}`),
		v(t, `{"keywords": ["a", "b", "x"]}`),
		v(t, `{"keywords": ["a", "b", "y"]}`),
		Options{Arrays: ArraysUnion})
	wantClean(t, res, `{"keywords": ["a", "b", "x", "y"]}`)
}

func TestArrayUnionRespectsDeletions(t *testing.T) {
	// Ours removed "b"; theirs still has it — it stays removed, because
	// theirs did not add it, it just did not touch it.
	res := Merge(
		v(t, `["a", "b"]`),
		v(t, `["a"]`),
		v(t, `["a", "b", "c"]`),
		Options{Arrays: ArraysUnion})
	wantClean(t, res, `["a", "c"]`)
}

func TestArrayAtomicStrategyConflictsWholeArray(t *testing.T) {
	res := Merge(v(t, `[1]`), v(t, `[1, 2]`), v(t, `[1, 3]`), Options{Arrays: ArraysAtomic})
	wantConflicts(t, res, "")
	c := res.Conflicts[0]
	if c.Splice {
		t.Fatal("atomic conflicts must not splice")
	}
	if !jsonval.Equal(c.Ours, v(t, `[1, 2]`)) || !jsonval.Equal(c.Theirs, v(t, `[1, 3]`)) {
		t.Fatalf("payload wrong: %+v", c)
	}
}

func TestArrayOverLCSLimitDegradesToAtomicConflict(t *testing.T) {
	// Past maxLCSCells the quadratic table is not worth building; the
	// array degrades to one conflict instead of eating memory.
	big := func(first string) string {
		elems := make([]string, 2100)
		elems[0] = first
		for i := 1; i < len(elems); i++ {
			elems[i] = fmt.Sprint(i)
		}
		return "[" + strings.Join(elems, ",") + "]"
	}
	res := run(t, big("0"), big("-1"), big("-2"))
	wantConflicts(t, res, "")
	if res.Conflicts[0].Kind != EditEdit {
		t.Fatalf("kind = %s", res.Conflicts[0].Kind)
	}
}

func TestArrayDuplicateElementsStayStable(t *testing.T) {
	// Repeated equal elements must not confuse the alignment.
	res := run(t, `[1, 1, 1]`, `[1, 1, 1, 2]`, `[0, 1, 1, 1]`)
	wantClean(t, res, `[0, 1, 1, 1, 2]`)
}
