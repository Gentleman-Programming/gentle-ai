package doctor

import (
	"encoding/json"
	"io"
)

// JSONRenderer outputs the report as structured JSON.
type JSONRenderer struct {
	writer io.Writer
}

// NewJSONRenderer creates a new JSON renderer.
func NewJSONRenderer(w io.Writer) Renderer {
	return &JSONRenderer{writer: w}
}

// Render outputs the report as pretty JSON.
func (r *JSONRenderer) Render(report DoctorReport) error {
	enc := json.NewEncoder(r.writer)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}