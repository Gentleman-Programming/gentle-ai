package filemerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

func MergeJSONObjects(baseJSON []byte, overlayJSON []byte) ([]byte, error) {
	base, err := unmarshalJSONObject(baseJSON)
	if err != nil {
		// Real user machines may have a malformed or non-JSON mcp.json (e.g. a file
		// that starts with "a" or contains arbitrary text). The installer backup step
		// already snapshots the existing file before apply, so proceeding with an
		// empty base is safe and far preferable to aborting the whole install.
		base = map[string]any{}
	}

	overlay, err := unmarshalJSONObject(overlayJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal overlay json: %w", err)
	}

	merged := mergeObjects(base, overlay)
	encoded, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal merged json: %w", err)
	}

	return append(encoded, '\n'), nil
}

// MergeJSONObjectsForPath merges an overlay into a settings document. Existing
// JSONC documents keep their comments and JSONC syntax; strict JSON retains the
// established MergeJSONObjects output exactly.
func MergeJSONObjectsForPath(path string, baseJSON, overlayJSON []byte) ([]byte, error) {
	if !strings.EqualFold(filepath.Ext(path), ".jsonc") || len(bytes.TrimSpace(baseJSON)) == 0 || json.Valid(baseJSON) {
		return MergeJSONObjects(baseJSON, overlayJSON)
	}

	base, err := unmarshalJSONObject(baseJSON)
	if err != nil {
		return MergeJSONObjects(baseJSON, overlayJSON)
	}
	overlay, err := unmarshalJSONObject(overlayJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal overlay json: %w", err)
	}

	return rewriteJSONC(baseJSON, base, mergeObjects(base, overlay))
}

// RewriteJSONObjectForPath serializes an already-updated settings object. JSONC
// inputs retain their comments and trailing-comma syntax; strict JSON uses the
// same indented encoding as existing mutation call sites.
func RewriteJSONObjectForPath(path string, baseJSON []byte, updated map[string]any) ([]byte, error) {
	if !strings.EqualFold(filepath.Ext(path), ".jsonc") || len(bytes.TrimSpace(baseJSON)) == 0 || json.Valid(baseJSON) {
		encoded, err := json.MarshalIndent(updated, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal rewritten json: %w", err)
		}
		return append(encoded, '\n'), nil
	}
	base, err := unmarshalJSONObject(baseJSON)
	if err != nil {
		return RewriteJSONObjectForPath("", nil, updated)
	}
	return rewriteJSONC(baseJSON, base, updated)
}

func rewriteJSONC(baseJSON []byte, base, updated map[string]any) ([]byte, error) {
	value, err := hujson.Parse(baseJSON)
	if err != nil {
		return nil, fmt.Errorf("parse JSONC settings: %w", err)
	}
	operations := make([]jsonPatchOperation, 0)
	appendJSONPatchDifference(&operations, "", base, updated)
	if len(operations) == 0 {
		return baseJSON, nil
	}
	patch, err := json.Marshal(operations)
	if err != nil {
		return nil, fmt.Errorf("marshal JSONC patch: %w", err)
	}
	if err := value.Patch(patch); err != nil {
		return nil, fmt.Errorf("apply JSONC patch: %w", err)
	}
	value.Format()
	return value.Pack(), nil
}

type jsonPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

func appendJSONPatchDifference(operations *[]jsonPatchOperation, path string, base, updated map[string]any) {
	for _, key := range sortedObjectKeys(base) {
		if _, exists := updated[key]; !exists {
			*operations = append(*operations, jsonPatchOperation{Op: "remove", Path: path + "/" + escapeJSONPointerToken(key)})
		}
	}
	for _, key := range sortedObjectKeys(updated) {
		updatedValue := updated[key]
		keyPath := path + "/" + escapeJSONPointerToken(key)
		baseValue, exists := base[key]
		baseMap, baseIsMap := baseValue.(map[string]any)
		updatedMap, updatedIsMap := updatedValue.(map[string]any)
		if exists && baseIsMap && updatedIsMap {
			appendJSONPatchDifference(operations, keyPath, baseMap, updatedMap)
			continue
		}
		appendJSONPatchOperation(operations, keyPath, exists, baseValue, updatedValue)
	}
}

func sortedObjectKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendJSONPatchOperation(operations *[]jsonPatchOperation, path string, exists bool, baseValue, replacement any) {
	if exists && reflect.DeepEqual(baseValue, replacement) {
		return
	}
	op := "add"
	if exists {
		op = "replace"
	}
	*operations = append(*operations, jsonPatchOperation{Op: op, Path: path, Value: replacement})
}

func escapeJSONPointerToken(token string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(token)
}

func unmarshalJSONObject(raw []byte) (map[string]any, error) {
	object := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return object, nil
	}

	if err := json.Unmarshal(raw, &object); err == nil {
		return object, nil
	}

	normalized := normalizeJSON(raw)
	if err := json.Unmarshal(normalized, &object); err != nil {
		return nil, err
	}

	return object, nil
}

// UnmarshalJSONObject decodes a JSON object using the same JSONC normalization
// accepted by MergeJSONObjects: comments are stripped and trailing commas are
// removed before falling back to strict JSON decoding errors.
func UnmarshalJSONObject(raw []byte) (map[string]any, error) {
	return unmarshalJSONObject(raw)
}

func normalizeJSON(raw []byte) []byte {
	withoutComments := stripJSONComments(raw)
	return stripTrailingCommas(withoutComments)
}

func stripJSONComments(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				out = append(out, ch)
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < len(raw) && raw[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}

		if ch == '/' && i+1 < len(raw) {
			next := raw[i+1]
			if next == '/' {
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		out = append(out, ch)
	}

	return out
}

func stripTrailingCommas(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escaped := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}

		if ch == ',' {
			j := i + 1
			for j < len(raw) {
				next := raw[j]
				if next == ' ' || next == '\t' || next == '\n' || next == '\r' {
					j++
					continue
				}
				if next == '}' || next == ']' {
					ch = 0
				}
				break
			}
		}

		if ch != 0 {
			out = append(out, ch)
		}
	}

	return out
}

// replacesentinel is the key used in an overlay map to signal that the parent
// key should be replaced atomically rather than deep-merged. When mergeObjects
// encounters a nested map whose only key is "__replace__", the value stored
// under that key is used verbatim as the replacement — the corresponding base
// value is discarded entirely.
//
// Example overlay that forces atomic replacement of mcp.engram:
//
//	{"mcp": {"engram": {"__replace__": {"command": [...], "type": "local"}}}}
const replacesentinel = "__replace__"

// asSentinel checks if v is a map with exactly one key "__replace__".
// If so, it returns the replacement value and true. Otherwise it returns nil, false.
func asSentinel(v any) (any, bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		return nil, false
	}
	if replacement, hasSentinel := m[replacesentinel]; hasSentinel && len(m) == 1 {
		return replacement, true
	}
	return nil, false
}

func mergeObjects(base map[string]any, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}

	for key, overlayValue := range overlay {
		// Check for the replace sentinel: if the overlay value is a map with
		// exactly one key "__replace__", use the sentinel's value verbatim —
		// regardless of whether the key exists in base. This allows callers to
		// force atomic replacement of a nested object instead of deep-merging.
		if replacement, isSentinel := asSentinel(overlayValue); isSentinel {
			result[key] = replacement
			continue
		}

		baseValue, ok := result[key]
		if !ok {
			// Even when there is no base value, recurse into overlay maps so
			// that any nested __replace__ sentinels are unwrapped before
			// they reach the output.
			if overlayMap, isMap := overlayValue.(map[string]any); isMap {
				result[key] = mergeObjects(map[string]any{}, overlayMap)
			} else {
				result[key] = overlayValue
			}
			continue
		}

		baseMap, baseIsMap := baseValue.(map[string]any)
		overlayMap, overlayIsMap := overlayValue.(map[string]any)
		if baseIsMap && overlayIsMap {
			result[key] = mergeObjects(baseMap, overlayMap)
			continue
		}

		result[key] = overlayValue
	}

	return result
}
