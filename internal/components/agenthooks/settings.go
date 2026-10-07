package agenthooks

import (
	"bytes"
	"encoding/json"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
)

// decodeHookSettings decodes hook settings that may be JSONC (comments and
// trailing commas). Empty data decodes to an empty object.
func decodeHookSettings(data []byte) (map[string]any, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(data)) == 0 {
		return root, nil
	}
	if json.Valid(data) {
		err := json.Unmarshal(data, &root)
		return root, err
	}
	return filemerge.UnmarshalJSONObject(data)
}

// encodeHookSettings returns data with its hooks object replaced. Strict JSON
// keeps the normalized encoding. JSONC keeps every byte outside the hooks
// value and refuses, rather than normalizes, a hooks key spelled with escapes,
// duplicated, or holding comments.
func encodeHookSettings(data []byte, root, hooks map[string]any) ([]byte, error) {
	if len(bytes.TrimSpace(data)) == 0 || json.Valid(data) {
		root["hooks"] = hooks
		out, err := json.MarshalIndent(root, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(out, '\n'), nil
	}
	overlay, err := json.Marshal(map[string]any{"hooks": map[string]any{"__replace__": hooks}})
	if err != nil {
		return nil, err
	}
	return filemerge.MergeOpenCodeJSONCObjects(data, overlay)
}
