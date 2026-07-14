package jsonval

import (
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// Equal reports semantic JSON equality: object member order is ignored
// (an object is an unordered set of members per RFC 8259), numbers compare
// by mathematical value (1.0 == 1 == 1e0), and strings compare by decoded
// content. A nil *Value ("absent") equals only another nil.
func Equal(a, b *Value) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case Null:
		return true
	case Bool:
		return a.B == b.B
	case Number:
		return numEqual(a.Num, b.Num)
	case String:
		return a.Str == b.Str
	case Array:
		if len(a.Arr) != len(b.Arr) {
			return false
		}
		for i := range a.Arr {
			if !Equal(a.Arr[i], b.Arr[i]) {
				return false
			}
		}
		return true
	case Object:
		if len(a.Mem) != len(b.Mem) {
			return false
		}
		bm := make(map[string]*Value, len(b.Mem))
		for _, m := range b.Mem {
			bm[m.Key] = m.Val
		}
		for _, m := range a.Mem {
			bv, ok := bm[m.Key]
			if !ok || !Equal(m.Val, bv) {
				return false
			}
		}
		return true
	}
	return false
}

// maxExp caps the exponent magnitude for exact rational comparison. A
// literal like 1e999999 would need a big.Int with ~a million digits, so
// beyond this cap numbers fall back to literal string comparison.
const maxExp = 1000

// numEqual compares two raw number literals by value where practical.
func numEqual(a, b string) bool {
	if a == b {
		return true
	}
	ra, oka := ratOf(a)
	rb, okb := ratOf(b)
	if !oka || !okb {
		return false
	}
	return ra.Cmp(rb) == 0
}

// ratOf converts a JSON number literal into an exact rational. It returns
// ok=false for exponents beyond maxExp; callers then compare literally.
func ratOf(lit string) (*big.Rat, bool) {
	mant := lit
	exp := 0
	if i := strings.IndexAny(lit, "eE"); i >= 0 {
		e, err := strconv.Atoi(lit[i+1:])
		if err != nil || e > maxExp || e < -maxExp {
			return nil, false
		}
		mant, exp = lit[:i], e
	}
	r, ok := new(big.Rat).SetString(mant)
	if !ok {
		return nil, false
	}
	if exp != 0 {
		mag := exp
		if mag < 0 {
			mag = -mag
		}
		p := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(mag)), nil))
		if exp > 0 {
			r.Mul(r, p)
		} else {
			r.Quo(r, p)
		}
	}
	return r, true
}

// Canon returns a canonical compact encoding of v: object keys sorted,
// numbers normalized to exact rationals. Two values are semantically equal
// iff their canonical forms match, which makes Canon a cheap hash key for
// the array LCS alignment.
func Canon(v *Value) string {
	var sb strings.Builder
	appendCanon(&sb, v)
	return sb.String()
}

func appendCanon(sb *strings.Builder, v *Value) {
	if v == nil {
		sb.WriteString("absent")
		return
	}
	switch v.Kind {
	case Null:
		sb.WriteString("null")
	case Bool:
		sb.WriteString(strconv.FormatBool(v.B))
	case Number:
		if r, ok := ratOf(v.Num); ok {
			sb.WriteString(r.RatString())
		} else {
			// Outside the exact range: the literal itself is the identity.
			sb.WriteString("~")
			sb.WriteString(v.Num)
		}
	case String:
		sb.WriteString(strconv.Quote(v.Str))
	case Array:
		sb.WriteByte('[')
		for i, e := range v.Arr {
			if i > 0 {
				sb.WriteByte(',')
			}
			appendCanon(sb, e)
		}
		sb.WriteByte(']')
	case Object:
		mem := make([]Member, len(v.Mem))
		copy(mem, v.Mem)
		sort.SliceStable(mem, func(i, j int) bool { return mem[i].Key < mem[j].Key })
		sb.WriteByte('{')
		for i, m := range mem {
			if i > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(strconv.Quote(m.Key))
			sb.WriteByte(':')
			appendCanon(sb, m.Val)
		}
		sb.WriteByte('}')
	}
}
