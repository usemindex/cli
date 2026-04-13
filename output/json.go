package output

import (
	"encoding/json"
	"io"
)

// JSON serializa data como JSON indentado e escreve em w.
func JSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}
