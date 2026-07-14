// Merge-rule tests. Each test is one cell of the decision matrix in
// docs/merge-rules.md: who changed what relative to the base, and whether
// that is a clean merge or a real collision.
package merge

import (
	"strings"
	"testing"

	"github.com/JaydenCJ/keymerge/internal/jsonval"
)

func v(t *testing.T, src string) *jsonval.Value {
	t.Helper()
	if src == "" {
		return nil // absent side
	}
	val, err := jsonval.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return val
}

// run merges three documents ("" = absent) with the default options.
func run(t *testing.T, base, ours, theirs string) *Result {
	t.Helper()
	return Merge(v(t, base), v(t, ours), v(t, theirs), Options{})
}

// wantClean asserts a conflict-free merge whose result equals want.
func wantClean(t *testing.T, res *Result, want string) {
	t.Helper()
	if len(res.Conflicts) != 0 {
		t.Fatalf("expected clean merge, got conflicts: %+v", res.Conflicts)
	}
	if res.Tree == nil {
		t.Fatal("merged tree is absent")
	}
	if res.Tree.Kind != NodeLeaf {
		t.Fatalf("clean merge should collapse to a leaf, got kind %d", res.Tree.Kind)
	}
	if !jsonval.Equal(res.Tree.Leaf, v(t, want)) {
		t.Fatalf("merged to %s, want %s",
			jsonval.Write(res.Tree.Leaf, jsonval.DefaultStyle()), want)
	}
}

// wantConflicts asserts the exact conflict paths, in order.
func wantConflicts(t *testing.T, res *Result, paths ...string) {
	t.Helper()
	if len(res.Conflicts) != len(paths) {
		t.Fatalf("got %d conflicts (%+v), want %d", len(res.Conflicts), res.Conflicts, len(paths))
	}
	for i, p := range paths {
		if res.Conflicts[i].Path != p {
			t.Fatalf("conflict %d at %q, want %q", i, res.Conflicts[i].Path, p)
		}
	}
}

func TestSingleSideAndConvergentEdits(t *testing.T) {
	// The four trivial cells of the decision matrix: untouched, ours-only,
	// theirs-only, and both-made-the-same-change.
	wantClean(t, run(t, `{"a": 1}`, `{"a": 1}`, `{"a": 1}`), `{"a": 1}`)
	wantClean(t, run(t, `{"a": 1}`, `{"a": 2}`, `{"a": 1}`), `{"a": 2}`)
	wantClean(t, run(t, `{"a": 1}`, `{"a": 1}`, `{"a": 3}`), `{"a": 3}`)
	wantClean(t, run(t, `{"a": 1}`, `{"a": 9}`, `{"a": 9}`), `{"a": 9}`)
}

func TestIndependentKeyEditsMerge(t *testing.T) {
	// The flagship case: two sides touch different keys of the same
	// object — a guaranteed textual conflict, a trivial structural merge.
	res := run(t,
		`{"version": "1.0.0", "dependencies": {"a": "^1"}}`,
		`{"version": "2.0.0", "dependencies": {"a": "^1"}}`,
		`{"version": "1.0.0", "dependencies": {"a": "^2"}}`)
	wantClean(t, res, `{"version": "2.0.0", "dependencies": {"a": "^2"}}`)
}

func TestAdditionsFromBothSidesMerge(t *testing.T) {
	res := run(t, `{"a": 1}`, `{"a": 1, "b": 2}`, `{"a": 1, "c": 3}`)
	wantClean(t, res, `{"a": 1, "b": 2, "c": 3}`)
}

func TestDeletionInOneSideApplies(t *testing.T) {
	res := run(t, `{"a": 1, "b": 2}`, `{"a": 1}`, `{"a": 1, "b": 2}`)
	wantClean(t, res, `{"a": 1}`)
	res = run(t, `{"a": 1, "b": 2}`, `{"a": 1, "b": 2}`, `{"b": 2}`)
	wantClean(t, res, `{"b": 2}`)
}

func TestBothDeleteSameKey(t *testing.T) {
	res := run(t, `{"a": 1, "b": 2}`, `{"b": 2}`, `{"b": 2}`)
	wantClean(t, res, `{"b": 2}`)
}

func TestDeleteEditConflictsBothDirections(t *testing.T) {
	res := run(t, `{"a": 1}`, `{}`, `{"a": 2}`)
	wantConflicts(t, res, "/a")
	if res.Conflicts[0].Kind != DeleteEdit {
		t.Fatalf("kind = %s, want %s", res.Conflicts[0].Kind, DeleteEdit)
	}
	if res.Conflicts[0].Ours != nil {
		t.Fatal("ours side should be absent")
	}
	res = run(t, `{"a": 1}`, `{"a": 2}`, `{}`)
	wantConflicts(t, res, "/a")
	if res.Conflicts[0].Kind != EditDelete {
		t.Fatalf("kind = %s, want %s", res.Conflicts[0].Kind, EditDelete)
	}
}

func TestScalarEditEditConflicts(t *testing.T) {
	res := run(t, `{"a": 1}`, `{"a": 2}`, `{"a": 3}`)
	wantConflicts(t, res, "/a")
	c := res.Conflicts[0]
	if c.Kind != EditEdit || c.Ours.Num != "2" || c.Theirs.Num != "3" || c.Base.Num != "1" {
		t.Fatalf("conflict payload wrong: %+v", c)
	}
}

func TestAddAddDifferentScalarsConflicts(t *testing.T) {
	res := run(t, `{}`, `{"a": 1}`, `{"a": 2}`)
	wantConflicts(t, res, "/a")
	if res.Conflicts[0].Kind != AddAdd {
		t.Fatalf("kind = %s, want %s", res.Conflicts[0].Kind, AddAdd)
	}
}

func TestAddAddObjectsRecurseInsteadOfConflicting(t *testing.T) {
	// Identical additions converge; both sides adding the same new
	// section with different keys inside is still no collision — merge
	// inside it.
	wantClean(t, run(t, `{}`, `{"a": {"x": 1}}`, `{"a": {"x": 1}}`), `{"a": {"x": 1}}`)
	res := run(t, `{}`, `{"scripts": {"build": "b"}}`, `{"scripts": {"test": "t"}}`)
	wantClean(t, res, `{"scripts": {"build": "b", "test": "t"}}`)
}

func TestNestedConflictHasDeepPointerPath(t *testing.T) {
	res := run(t,
		`{"a": {"b": {"c": 1}}}`,
		`{"a": {"b": {"c": 2}}}`,
		`{"a": {"b": {"c": 3}}}`)
	wantConflicts(t, res, "/a/b/c")
}

func TestTypeClashConflict(t *testing.T) {
	res := run(t, `{"a": 1}`, `{"a": "one"}`, `{"a": [1]}`)
	wantConflicts(t, res, "/a")
	if res.Conflicts[0].Kind != TypeClash {
		t.Fatalf("kind = %s, want %s", res.Conflicts[0].Kind, TypeClash)
	}
}

func TestFormatterNoiseIsNotAChange(t *testing.T) {
	// Ours only reordered keys; theirs edited a value. No collision, and
	// theirs' edit lands.
	res := run(t,
		`{"a": 1, "b": 2}`,
		`{"b": 2, "a": 1}`,
		`{"a": 1, "b": 99}`)
	wantClean(t, res, `{"a": 1, "b": 99}`)
	// Ours rewrote 1 as 1.0 (formatter noise); theirs changed it to 2.
	wantClean(t, run(t, `{"n": 1}`, `{"n": 1.0}`, `{"n": 2}`), `{"n": 2}`)
}

func TestConflictPathsEscapeSpecialKeys(t *testing.T) {
	res := run(t,
		`{"a/b": 1, "c~d": 1}`,
		`{"a/b": 2, "c~d": 2}`,
		`{"a/b": 3, "c~d": 3}`)
	wantConflicts(t, res, "/a~1b", "/c~0d")
}

func TestMergedKeyOrderFollowsOursWithTheirsAdditionsInContext(t *testing.T) {
	// Theirs inserted "b2" right after "b"; the merged order keeps it
	// there instead of dumping it at the end.
	res := run(t,
		`{"a": 1, "b": 2, "c": 3}`,
		`{"a": 1, "b": 2, "c": 3, "z": 26}`,
		`{"a": 1, "b": 2, "b2": 22, "c": 3}`)
	wantClean(t, res, `{"a": 1, "b": 2, "b2": 22, "c": 3, "z": 26}`)
	var keys []string
	for _, m := range res.Tree.Leaf.Mem {
		keys = append(keys, m.Key)
	}
	if got := strings.Join(keys, ","); got != "a,b,b2,c,z" {
		t.Fatalf("key order %s, want a,b,b2,c,z", got)
	}
}

func TestRootLevelConflicts(t *testing.T) {
	res := run(t, `1`, `2`, `3`)
	wantConflicts(t, res, "")
	if res.Tree.Kind != NodeConflict {
		t.Fatal("root should be a conflict node")
	}
	// No common ancestor (git passes an empty %O): identical adds
	// converge, different scalar adds collide at the root.
	wantClean(t, run(t, ``, `{"a": 1}`, `{"a": 1}`), `{"a": 1}`)
	res = run(t, ``, `"x"`, `"y"`)
	wantConflicts(t, res, "")
	if res.Conflicts[0].Kind != AddAdd {
		t.Fatalf("kind = %s, want %s", res.Conflicts[0].Kind, AddAdd)
	}
}

func TestMergeToAbsence(t *testing.T) {
	res := run(t, `{"a": 1}`, ``, ``)
	if res.Tree != nil || len(res.Conflicts) != 0 {
		t.Fatalf("both-deleted document should merge to absence, got %+v", res)
	}
}

func TestNullVersusDeleteIsARealCollision(t *testing.T) {
	// Ours set the key to null, theirs removed it: different intents.
	res := run(t, `{"a": 1}`, `{"a": null}`, `{}`)
	wantConflicts(t, res, "/a")
	if res.Conflicts[0].Kind != EditDelete {
		t.Fatalf("kind = %s, want %s", res.Conflicts[0].Kind, EditDelete)
	}
}

func TestSiblingConflictDoesNotPoisonCleanKeys(t *testing.T) {
	// One colliding key must not stop the clean keys from merging.
	res := run(t,
		`{"v": 1, "x": "a", "y": "p"}`,
		`{"v": 2, "x": "b", "y": "p"}`,
		`{"v": 3, "x": "a", "y": "q"}`)
	wantConflicts(t, res, "/v")
	if res.Tree.Kind != NodeObject {
		t.Fatal("root should stay a partially merged object")
	}
	for _, e := range res.Tree.Obj {
		switch e.Key {
		case "x":
			if !jsonval.Equal(e.Node.Leaf, v(t, `"b"`)) {
				t.Fatalf("x = %+v, want ours' edit", e.Node.Leaf)
			}
		case "y":
			if !jsonval.Equal(e.Node.Leaf, v(t, `"q"`)) {
				t.Fatalf("y = %+v, want theirs' edit", e.Node.Leaf)
			}
		}
	}
}

func TestParseStrategyNames(t *testing.T) {
	for name, want := range map[string]Strategy{
		"merge": Arrays3Way, "atomic": ArraysAtomic, "union": ArraysUnion,
	} {
		got, ok := ParseStrategy(name)
		if !ok || got != want {
			t.Fatalf("ParseStrategy(%q) = %v, %v", name, got, ok)
		}
	}
	if _, ok := ParseStrategy("bogus"); ok {
		t.Fatal("bogus strategy accepted")
	}
}

func TestEscapePointer(t *testing.T) {
	for in, want := range map[string]string{
		"plain": "plain", "a/b": "a~1b", "a~b": "a~0b", "~/": "~0~1",
	} {
		if got := EscapePointer(in); got != want {
			t.Fatalf("EscapePointer(%q) = %q, want %q", in, got, want)
		}
	}
}
