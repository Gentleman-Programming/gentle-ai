package filemerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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

// MergeJSONObjectsPreserveJSONC merges JSON object overlays while preserving the
// surrounding JSONC document text. It rewrites only top-level values touched by
// the overlay, keeping unrelated comments and trailing commas intact.
func MergeJSONObjectsPreserveJSONC(baseJSON []byte, overlayJSON []byte) ([]byte, error) {
	if len(bytes.TrimSpace(baseJSON)) == 0 {
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

	updated := string(baseJSON)
	for key, overlayValue := range overlay {
		value := overlayValue
		if baseValue, ok := base[key]; ok {
			if replacement, isSentinel := asSentinel(overlayValue); isSentinel {
				value = replacement
			} else if baseMap, baseIsMap := baseValue.(map[string]any); baseIsMap {
				if overlayMap, overlayIsMap := overlayValue.(map[string]any); overlayIsMap {
					value = mergeObjects(baseMap, overlayMap)
				}
			}
		}
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal merged jsonc value %q: %w", key, err)
		}
		updated = upsertTopLevelJSONCValue(updated, key, string(encoded))
		base[key] = value
	}
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return []byte(updated), nil
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

func RemoveJSONAgentTools(raw []byte, names ...string) ([]byte, error) {
	root, err := unmarshalJSONObject(raw)
	if err != nil {
		return raw, nil
	}
	agents, _ := root["agent"].(map[string]any)
	changed := false
	for _, name := range names {
		if agent, ok := agents[name].(map[string]any); ok {
			if _, exists := agent["tools"]; exists {
				delete(agent, "tools")
				changed = true
			}
		}
	}
	if !changed {
		return raw, nil
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
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

func upsertTopLevelJSONCValue(content, key, encodedValue string) string {
	if start, end, ok := topLevelJSONCValueRange(content, key); ok {
		return content[:start] + indentJSONCValue(encodedValue, valueIndent(content, start)) + content[end:]
	}
	insert := strings.LastIndex(content, "}")
	if insert < 0 {
		return content
	}
	prefix := content[:insert]
	suffix := content[insert:]
	trimmedPrefix := strings.TrimRight(prefix, " \t\r\n")
	needsComma := strings.TrimSpace(trimmedPrefix) != "{" && !strings.HasSuffix(trimmedPrefix, ",")
	var b strings.Builder
	b.WriteString(trimmedPrefix)
	if needsComma {
		b.WriteString(",")
	}
	b.WriteString("\n  ")
	b.WriteString(strconvQuote(key))
	b.WriteString(": ")
	b.WriteString(indentJSONCValue(encodedValue, "  "))
	b.WriteString("\n")
	b.WriteString(suffix)
	return b.String()
}

func topLevelJSONCValueRange(content, key string) (int, int, bool) {
	target := strconvQuote(key)
	inString, escaped, lineComment, blockComment := false, false, false, false
	depth := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if lineComment {
			if ch == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if ch == '*' && i+1 < len(content) && content[i+1] == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if inString {
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
		if ch == '/' && i+1 < len(content) {
			if content[i+1] == '/' {
				lineComment = true
				i++
				continue
			}
			if content[i+1] == '*' {
				blockComment = true
				i++
				continue
			}
		}
		if ch == '"' {
			if depth == 1 && strings.HasPrefix(content[i:], target) {
				j := i + len(target)
				for j < len(content) && isJSONWhitespace(content[j]) {
					j++
				}
				if j < len(content) && content[j] == ':' {
					start := j + 1
					for start < len(content) && isJSONWhitespace(content[start]) {
						start++
					}
					end := scanJSONCValueEnd(content, start)
					return start, end, true
				}
			}
			inString = true
			continue
		}
		if ch == '{' || ch == '[' {
			depth++
		} else if ch == '}' || ch == ']' {
			depth--
		}
	}
	return 0, 0, false
}

func scanJSONCValueEnd(content string, start int) int {
	inString, escaped, lineComment, blockComment := false, false, false, false
	depth := 0
	for i := start; i < len(content); i++ {
		ch := content[i]
		if lineComment {
			if ch == '\n' {
				lineComment = false
			}
			continue
		}
		if blockComment {
			if ch == '*' && i+1 < len(content) && content[i+1] == '/' {
				blockComment = false
				i++
			}
			continue
		}
		if inString {
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
		if ch == '/' && i+1 < len(content) {
			if content[i+1] == '/' {
				lineComment = true
				i++
				continue
			}
			if content[i+1] == '*' {
				blockComment = true
				i++
				continue
			}
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '{' || ch == '[' {
			depth++
			continue
		}
		if ch == '}' || ch == ']' {
			if depth == 0 {
				return i
			}
			depth--
			continue
		}
		if depth == 0 && ch == ',' {
			return i
		}
	}
	return len(content)
}

func valueIndent(content string, start int) string {
	lineStart := strings.LastIndex(content[:start], "\n") + 1
	indent := content[lineStart:start]
	return indent[:len(indent)-len(strings.TrimLeft(indent, " \t"))]
}

func indentJSONCValue(value, indent string) string {
	return strings.ReplaceAll(value, "\n", "\n"+indent)
}

func isJSONWhitespace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' }

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
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
