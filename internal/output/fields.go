package output

import "strings"

// ProjectFields returns a copy of m containing only the requested keys.
// "all" or empty returns m as-is.
func ProjectFields(m map[string]any, fields string) map[string]any {
	if fields == "" || fields == "all" {
		return m
	}
	keep := map[string]bool{}
	for _, k := range strings.Split(fields, ",") {
		keep[strings.TrimSpace(k)] = true
	}
	out := make(map[string]any, len(keep))
	for k := range keep {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}
