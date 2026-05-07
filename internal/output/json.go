package output

import (
	"encoding/json"
	"io"
)

// EmitJSON writes a single JSON object to w (compact).
func EmitJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// EmitNDJSON writes one item per line, compact.
func EmitNDJSON(w io.Writer, items []map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}
