// Package jsonval is an order-preserving JSON document model. Unlike
// encoding/json it keeps object members in source order, stores number
// literals verbatim (so 1.50e3 survives a round trip), and offers semantic
// deep equality — everything a structural merge needs and a map[string]any
// cannot provide.
package jsonval

// Kind enumerates the six JSON value kinds.
type Kind int

const (
	Null Kind = iota
	Bool
	Number
	String
	Array
	Object
)

// String returns the RFC 8259 name of the kind, used in error messages.
func (k Kind) String() string {
	switch k {
	case Null:
		return "null"
	case Bool:
		return "boolean"
	case Number:
		return "number"
	case String:
		return "string"
	case Array:
		return "array"
	case Object:
		return "object"
	}
	return "invalid"
}

// Member is one object member. The order of members in Value.Mem is the
// source order and is significant for serialization (never for equality).
type Member struct {
	Key string
	Val *Value
}

// Value is a JSON value. A nil *Value means "absent" — a key or document
// that does not exist on one side of a merge — which is deliberately
// distinct from an explicit JSON null.
type Value struct {
	Kind Kind
	B    bool     // Bool
	Num  string   // Number: the raw literal exactly as parsed, e.g. "1.50e3"
	Str  string   // String: the decoded contents
	Arr  []*Value // Array elements
	Mem  []Member // Object members, in source order
}

// Get returns the value of the member named key, or nil when v is not an
// object or has no such member.
func (v *Value) Get(key string) *Value {
	if v == nil || v.Kind != Object {
		return nil
	}
	for _, m := range v.Mem {
		if m.Key == key {
			return m.Val
		}
	}
	return nil
}

// NewArray builds an array value; used by the merge engine and by tests.
func NewArray(elems ...*Value) *Value {
	return &Value{Kind: Array, Arr: elems}
}

// NewObject builds an object value from members in the given order.
func NewObject(mem ...Member) *Value {
	return &Value{Kind: Object, Mem: mem}
}
