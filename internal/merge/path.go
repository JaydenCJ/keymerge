package merge

import "strings"

// EscapePointer escapes one reference token per RFC 6901: "~" becomes
// "~0" and "/" becomes "~1", so conflict paths stay unambiguous even for
// keys like "a/b" (common in tool configs and JSON Schema documents).
func EscapePointer(token string) string {
	if !strings.ContainsAny(token, "~/") {
		return token
	}
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}
