package filemerge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
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
	encoded, err := MarshalJSONPreservingPermissions(baseJSON, merged)
	if err != nil {
		return nil, fmt.Errorf("marshal merged json: %w", err)
	}

	return append(encoded, '\n'), nil
}

// MergeJSONObjectsForPath selects the JSON object merge mode appropriate for
// path. JSONC files preserve comments and formatting around untouched values;
// strict JSON files are normalized through standard JSON encoding.
func MergeJSONObjectsForPath(path string, baseJSON []byte, overlayJSON []byte) ([]byte, error) {
	if strings.HasSuffix(path, ".jsonc") {
		return MergeJSONObjectsPreserveJSONC(baseJSON, overlayJSON)
	}
	return MergeJSONObjects(baseJSON, overlayJSON)
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
		return baseJSON, fmt.Errorf("refuse to merge malformed jsonc: %w", err)
	}
	overlay, err := unmarshalJSONObject(overlayJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal overlay json: %w", err)
	}
	for key := range overlay {
		if topLevelJSONCKeyCount(string(baseJSON), key) > 1 {
			return baseJSON, fmt.Errorf("refuse to merge jsonc with duplicate touched top-level key %q", key)
		}
	}

	merged := mergeObjects(base, overlay)
	encoded, err := MarshalJSONPreservingPermissions(baseJSON, merged)
	if err != nil {
		return nil, err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return nil, err
	}
	updated := string(baseJSON)
	for key := range overlay {
		updated = upsertTopLevelJSONCValue(updated, key, string(members[key]))
	}
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return []byte(updated), nil
}

// MarshalJSONPreservingPermissions retains existing rule order when a settings
// writer changes unrelated fields. Permission objects are order-sensitive in
// OpenCode; ordinary objects keep the existing sorted encoding convention.
func MarshalJSONPreservingPermissions(base []byte, value any) ([]byte, error) {
	return marshalPermissionOrder(normalizeJSON(base), value, false)
}

// MergeJSONDefaultsForPath fills missing values without overriding user policy.
// New rules precede existing rules, except an initial catch-all allow remains
// the fallback before new defaults. Existing rule order and scalar values win.
func MergeJSONDefaultsForPath(path string, baseJSON, defaultsJSON []byte) ([]byte, error) {
	base, err := unmarshalJSONObject(baseJSON)
	if err != nil {
		return nil, fmt.Errorf("refuse defaults over unreadable settings: %w", err)
	}
	defaults, err := unmarshalJSONObject(defaultsJSON)
	if err != nil {
		return nil, err
	}
	encoded, err := marshalPermissionOrder(normalizeJSON(baseJSON), mergeObjectScope(defaults, base, false, false), false)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".jsonc") || len(bytes.TrimSpace(baseJSON)) == 0 {
		return append(encoded, '\n'), nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &members); err != nil {
		return nil, err
	}
	updated := string(baseJSON)
	for key := range defaults {
		if topLevelJSONCKeyCount(updated, key) > 1 {
			return nil, fmt.Errorf("duplicate defaults key %q", key)
		}
		updated = upsertTopLevelJSONCValue(updated, key, string(members[key]))
	}
	return []byte(strings.TrimRight(updated, "\n") + "\n"), nil
}

func marshalPermissionOrder(base []byte, value any, ordered bool) ([]byte, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return json.Marshal(value)
	}
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(base, &raw)
	var existing, added []string
	if ordered && raw != nil {
		decoder := json.NewDecoder(bytes.NewReader(base))
		_, _ = decoder.Token()
		for decoder.More() {
			key, _ := decoder.Token()
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return nil, err
			}
			if _, present := object[key.(string)]; present {
				existing = append(existing, key.(string))
			}
		}
	}
	for key := range object {
		if !ordered || raw[key] == nil {
			added = append(added, key)
		}
	}
	sort.Strings(added)
	keys := append(append([]string{}, existing...), added...)
	// Newly introduced rules must not override existing custom rules.
	if ordered {
		keys = append(append([]string{}, added...), existing...)
		if len(existing) > 0 && existing[0] == "*" && string(raw["*"]) == `"allow"` {
			keys = append([]string{"*"}, append(added, existing[1:]...)...)
		}
	}
	var compact bytes.Buffer
	compact.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			compact.WriteByte(',')
		}
		name, _ := json.Marshal(key)
		compact.Write(name)
		compact.WriteByte(':')
		child, err := marshalPermissionOrder(raw[key], object[key], ordered || key == "permission")
		if err != nil {
			return nil, err
		}
		compact.Write(child)
	}
	compact.WriteByte('}')
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return indented.Bytes(), nil
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
	encoded, err := MarshalJSONPreservingPermissions(raw, root)
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

// topLevelJSONCKeyCount counts exact top-level object keys without normalizing
// the document. It is intentionally narrow: callers only need to reject a
// duplicate key they are about to rewrite, while untouched duplicate keys remain
// user-owned and are left byte-for-byte intact.
func topLevelJSONCKeyCount(content, key string) int {
	count := 0
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
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
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
			if depth == 1 && strings.HasPrefix(content[i:], strconvQuote(key)) {
				end := i + len(strconvQuote(key))
				if end < len(content) && (isJSONWhitespace(content[end]) || content[end] == ':' || content[end] == '/') {
					end = scanJSONCWhitespaceAndComments(content, end)
					if end < len(content) && content[end] == ':' {
						count++
					}
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
	return count
}

func upsertTopLevelJSONCValue(content, key, encodedValue string) string {
	if start, end, ok := topLevelJSONCValueRange(content, key); ok {
		return content[:start] + indentJSONCValue(encodedValue, valueIndent(content, start)) + content[end:]
	}
	insert := topLevelJSONCObjectEnd(content)
	if insert < 0 {
		return content
	}
	prefix := content[:insert]
	suffix := content[insert:]
	trimmedPrefix := strings.TrimRight(prefix, " \t\r\n")
	commentStrippedPrefix := strings.TrimSpace(string(stripJSONComments([]byte(trimmedPrefix))))
	needsComma := commentStrippedPrefix != "{" && !strings.HasSuffix(commentStrippedPrefix, ",")
	if needsComma {
		withComma := addCommaBeforeTrailingLineComment(trimmedPrefix)
		if withComma != trimmedPrefix {
			needsComma = false
		}
		trimmedPrefix = withComma
	}
	var b strings.Builder
	b.WriteString(trimmedPrefix)
	if needsComma && !strings.HasSuffix(trimmedPrefix, ",") {
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

// RemoveTopLevelJSONCValue removes a single top-level JSON/JSONC object member
// while preserving unrelated bytes. If the key is absent, raw is returned.
func RemoveTopLevelJSONCValue(raw []byte, key string) []byte {
	content := string(raw)
	nameStart, valueEnd, ok := topLevelJSONCPropertyRange(content, key)
	if !ok {
		return raw
	}
	start := nameStart
	for start > 0 && (content[start-1] == ' ' || content[start-1] == '\t') {
		start--
	}
	if lineStart := strings.LastIndex(content[:start], "\n") + 1; strings.TrimSpace(content[lineStart:start]) == "" {
		start = lineStart
	}
	end := valueEnd
	for end < len(content) && isJSONWhitespace(content[end]) {
		end++
	}
	if end < len(content) && content[end] == ',' {
		end++
		if end < len(content) && content[end] == '\n' {
			end++
		}
		return []byte(content[:start] + content[end:])
	}
	comma := start - 1
	for comma >= 0 && isJSONWhitespace(content[comma]) {
		comma--
	}
	if comma >= 0 && content[comma] == ',' {
		start = comma
	}
	return []byte(content[:start] + content[end:])
}

func topLevelJSONCPropertyRange(content, key string) (int, int, bool) {
	nameStart, _, end, ok := topLevelJSONCPropertyValueRange(content, key)
	if ok {
		return nameStart, end, true
	}
	return 0, 0, false
}

func topLevelJSONCObjectEnd(content string) int {
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
			inString = true
			continue
		}
		if ch == '{' || ch == '[' {
			depth++
			continue
		}
		if ch == '}' || ch == ']' {
			depth--
			if depth == 0 && ch == '}' {
				return i
			}
		}
	}
	return -1
}

func addCommaBeforeTrailingLineComment(content string) string {
	commentStart := trailingLineCommentStart(content)
	if commentStart < 0 {
		return content
	}
	left := strings.TrimRight(content[:commentStart], " \t")
	if strings.HasSuffix(left, ",") {
		return content
	}
	return left + "," + content[len(left):]
}

func trailingLineCommentStart(content string) int {
	inString, escaped, lineComment, blockComment := false, false, false, false
	commentStart := -1
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if lineComment {
			if ch == '\n' {
				lineComment = false
				commentStart = -1
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
				commentStart = i
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
		}
	}
	return commentStart
}

func topLevelJSONCValueRange(content, key string) (int, int, bool) {
	_, start, end, ok := topLevelJSONCPropertyValueRange(content, key)
	return start, end, ok
}

func topLevelJSONCPropertyValueRange(content, key string) (int, int, int, bool) {
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
				j = scanJSONCWhitespaceAndComments(content, j)
				if j < len(content) && content[j] == ':' {
					start := j + 1
					for start < len(content) && isJSONWhitespace(content[start]) {
						start++
					}
					end := scanJSONCValueEnd(content, start)
					return i, start, end, true
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
	return 0, 0, 0, false
}

func scanJSONCWhitespaceAndComments(content string, i int) int {
	for i < len(content) {
		if isJSONWhitespace(content[i]) {
			i++
			continue
		}
		if content[i] == '/' && i+1 < len(content) {
			switch content[i+1] {
			case '/':
				i += 2
				for i < len(content) && content[i] != '\n' {
					i++
				}
				continue
			case '*':
				i += 2
				for i+1 < len(content) && !(content[i] == '*' && content[i+1] == '/') {
					i++
				}
				if i+1 < len(content) {
					i += 2
				}
				continue
			}
		}
		return i
	}
	return i
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
				return trimTrailingJSONWhitespace(content, start, i)
			}
			depth--
			continue
		}
		if depth == 0 && ch == ',' {
			return trimTrailingJSONWhitespace(content, start, i)
		}
	}
	return trimTrailingJSONWhitespace(content, start, len(content))
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

func trimTrailingJSONWhitespace(content string, start, end int) int {
	for end > start && isJSONWhitespace(content[end-1]) {
		end--
	}
	return end
}

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
	return mergeObjectScope(base, overlay, false, true)
}

func mergeObjectScope(base map[string]any, overlay map[string]any, permission, protect bool) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}

	for key, overlayValue := range overlay {
		// A generated overlay must not loosen an existing scalar restriction,
		// including when an agent permission object replaces a scalar deny.
		if action, ok := result[key].(string); ok && protect && (permission || key == "permission") {
			if action == "deny" || (action == "ask" && overlayValue != "deny") {
				continue
			}
		}
		// A scalar allow/ask cannot safely replace an ordered rule object:
		// it could erase a deny anywhere in that object.
		if _, ok := result[key].(map[string]any); ok && protect && (permission || key == "permission") {
			if overlayValue == "allow" || overlayValue == "ask" {
				continue
			}
		}
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
				result[key] = mergeObjectScope(map[string]any{}, overlayMap, permission || key == "permission", protect)
			} else {
				result[key] = overlayValue
			}
			continue
		}

		baseMap, baseIsMap := baseValue.(map[string]any)
		overlayMap, overlayIsMap := overlayValue.(map[string]any)
		if baseIsMap && overlayIsMap {
			result[key] = mergeObjectScope(baseMap, overlayMap, permission || key == "permission", protect)
			continue
		}

		result[key] = overlayValue
	}

	return result
}
