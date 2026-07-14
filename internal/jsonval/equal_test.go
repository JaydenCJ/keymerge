// Semantic equality tests. Equality is the merge engine's entire notion
// of "changed", so false negatives here mean fake conflicts and false
// positives mean silently dropped edits.
package jsonval

import "testing"

func eq(t *testing.T, a, b string, want bool) {
	t.Helper()
	va, vb := mustParse(t, a), mustParse(t, b)
	if got := Equal(va, vb); got != want {
		t.Fatalf("Equal(%s, %s) = %v, want %v", a, b, got, want)
	}
	if got := Equal(vb, va); got != want {
		t.Fatalf("Equal(%s, %s) = %v, want %v (asymmetric!)", b, a, got, want)
	}
}

func TestEqualScalars(t *testing.T) {
	eq(t, `"a"`, `"a"`, true)
	eq(t, `"a"`, `"b"`, false)
	eq(t, `true`, `true`, true)
	eq(t, `true`, `false`, false)
	eq(t, `null`, `null`, true)
	eq(t, `null`, `false`, false)
	eq(t, `0`, `"0"`, false) // kind mismatch is never equal
}

func TestEqualNumbersByMathematicalValue(t *testing.T) {
	// A formatter flipping 1 to 1.0 must not register as an edit.
	eq(t, `1.0`, `1`, true)
	eq(t, `1e2`, `100`, true)
	eq(t, `0.5`, `5e-1`, true)
	eq(t, `-0`, `0`, true)
	eq(t, `1.50`, `1.5`, true)
	eq(t, `1`, `2`, false)
	eq(t, `0.1`, `0.2`, false)
	eq(t, `1e2`, `1e3`, false)
}

func TestEqualHugeExponentsFallBackToLiteral(t *testing.T) {
	// Beyond maxExp exact rationals get too expensive; the literal itself
	// becomes the identity — still deterministic, never a crash.
	eq(t, `1e99999`, `1e99999`, true)
	eq(t, `1e99999`, `1e99998`, false)
}

func TestEqualObjects(t *testing.T) {
	// RFC 8259 objects are unordered: pure key reordering is not a change,
	// which is what kills a whole class of fake conflicts. Any difference
	// in keys or values still registers.
	eq(t, `{"a": 1, "b": 2}`, `{"b": 2, "a": 1}`, true)
	eq(t, `{"a": 1}`, `{"a": 2}`, false)
	eq(t, `{"a": 1}`, `{"a": 1, "b": 2}`, false)
	eq(t, `{"a": 1}`, `{"b": 1}`, false)
}

func TestEqualArraysAreOrderSensitive(t *testing.T) {
	// Arrays are sequences: [1,2] and [2,1] are different documents.
	eq(t, `[1, 2]`, `[1, 2]`, true)
	eq(t, `[1, 2]`, `[2, 1]`, false)
	eq(t, `[1]`, `[1, 1]`, false)
}

func TestEqualAbsentVersusNull(t *testing.T) {
	// nil (absent) must never equal an explicit null — deleting a key and
	// setting it to null are different edits.
	if !Equal(nil, nil) {
		t.Fatal("nil should equal nil")
	}
	if Equal(nil, mustParse(t, `null`)) {
		t.Fatal("absent must not equal explicit null")
	}
}

func TestCanonSortsKeysAndNormalizesNumbers(t *testing.T) {
	a := Canon(mustParse(t, `{"b": 1.0, "a": [1e2]}`))
	b := Canon(mustParse(t, `{"a": [100], "b": 1}`))
	if a != b {
		t.Fatalf("canonical forms differ:\n%s\n%s", a, b)
	}
	if a == Canon(mustParse(t, `{"a": [100], "b": 2}`)) {
		t.Fatal("different values must have different canonical forms")
	}
}
