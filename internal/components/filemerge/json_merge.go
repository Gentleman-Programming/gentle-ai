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

// rejectDuplicateJSONKeys checks every object before a map decoder can collapse
// duplicate user keys. The migration must never serialize such a document.
func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(normalizeJSON(raw)))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key := keyToken.(string)
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delim)
		}
		_, err = decoder.Token()
		return err
	}
	return walk()
}

// RemoveLegacyOpenCodeAgentMarkers drops only the retired SDD ownership marker
// from every legacy agent definition. JSONC edits remain local to the
// property, preserving unrelated comments and formatting.
func RemoveLegacyOpenCodeAgentMarkers(path string, raw []byte, names []string) ([]byte, error) {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return raw, fmt.Errorf("refuse OpenCode settings with duplicate or malformed keys: %w", err)
	}
	root, err := unmarshalJSONObject(raw)
	if err != nil {
		return raw, fmt.Errorf("refuse malformed OpenCode settings: %w", err)
	}
	agents, _ := root["agent"].(map[string]any)
	eligible := make([]string, 0, len(agents))
	for name, value := range agents {
		def, _ := value.(map[string]any)
		if def["__managed_by"] == "gentle-ai/sdd" {
			eligible = append(eligible, name)
		}
	}
	sort.Strings(eligible)
	if len(eligible) == 0 {
		return raw, nil
	}
	if !strings.HasSuffix(path, ".jsonc") && json.Valid(raw) {
		for _, name := range eligible {
			delete(agents[name].(map[string]any), "__managed_by")
		}
		encoded, err := MarshalJSONPreservingPermissions(raw, root)
		if err != nil {
			return nil, err
		}
		return append(encoded, '\n'), nil
	}
	text := string(raw)
	if topLevelJSONCKeyCount(text, "agent") != 1 {
		return raw, fmt.Errorf("duplicate OpenCode agent key")
	}
	_, start, end, ok := topLevelJSONCPropertyValueRange(text, "agent")
	if !ok {
		return raw, fmt.Errorf("missing OpenCode agent object")
	}
	agentText := text[start:end]
	for _, name := range eligible {
		if topLevelJSONCKeyCount(agentText, name) != 1 {
			return raw, fmt.Errorf("duplicate OpenCode agent %q", name)
		}
		_, a, b, ok := topLevelJSONCPropertyValueRange(agentText, name)
		if !ok {
			return raw, fmt.Errorf("missing OpenCode agent %q", name)
		}
		defText := agentText[a:b]
		if topLevelJSONCKeyCount(defText, "__managed_by") != 1 {
			return raw, fmt.Errorf("duplicate OpenCode ownership marker in %q", name)
		}
		key, value, finish, ok := topLevelJSONCPropertyValueRange(defText, "__managed_by")
		if !ok {
			return raw, fmt.Errorf("missing OpenCode ownership marker in %q", name)
		}
		// Refuse comments in the property's syntax or immediately before its key:
		// deleting the property would otherwise silently discard user notes.
		colon := scanJSONCWhitespaceAndComments(defText, key+len(strconvQuote("__managed_by")))
		previous := key - 1
		for previous >= 0 && isJSONWhitespace(defText[previous]) {
			previous--
		}
		boundary := previous
		for boundary >= 0 && defText[boundary] != ',' && defText[boundary] != '{' {
			boundary--
		}
		if strings.Contains(defText[key+len(strconvQuote("__managed_by")):colon], "/") ||
			strings.Contains(defText[colon+1:value], "/") ||
			strings.Contains(defText[boundary+1:key], "/") {
			return raw, fmt.Errorf("refuse to remove OpenCode marker with attached comment in %q", name)
		}
		// The property value is a JSON string. Its scanner may include an
		// adjacent line comment in the value range; locate the closing quote
		// independently before deciding which bytes are safe to remove.
		closing := value + 1
		for closing < len(defText) {
			if defText[closing] == '\\' {
				closing += 2
				continue
			}
			if defText[closing] == '"' {
				closing++
				break
			}
			closing++
		}
		if closing > finish || strings.Contains(defText[closing:finish], "/") {
			return raw, fmt.Errorf("refuse to remove OpenCode marker with attached comment in %q", name)
		}
		// Include a following comma when present, otherwise the preceding comma.
		after := scanJSONCWhitespaceAndComments(defText, finish)
		if after < len(defText) && defText[after] == ',' {
			// A comment between the value and comma may belong to the user.
			// Refuse rather than deleting it with the marker property.
			if strings.Contains(defText[finish:after], "/") {
				return raw, fmt.Errorf("refuse to remove OpenCode marker with attached comment in %q", name)
			}
			finish = after + 1
		} else {
			before := key - 1
			for before >= 0 && isJSONWhitespace(defText[before]) {
				before--
			}
			if before >= 0 && defText[before] == ',' {
				key = before
			}
		}
		agentText = agentText[:a] + defText[:key] + defText[finish:] + agentText[b:]
	}
	return []byte(text[:start] + agentText + text[end:]), nil
}

// MigrateLegacyOpenCodeRolesJSONC edits marked roles locally and refuses ambiguous comments or keys.
func MigrateLegacyOpenCodeRolesJSONC(raw []byte, current []string) ([]byte, error) {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return raw, fmt.Errorf("refuse duplicate OpenCode settings keys: %w", err)
	}
	root, err := unmarshalJSONObject(raw)
	if err != nil {
		return raw, err
	}
	agents, _ := root["agent"].(map[string]any)
	managed := make(map[string]bool, len(current))
	for _, name := range current {
		managed[name] = true
	}
	var selected []string
	for name, value := range agents {
		entry, ok := value.(map[string]any)
		if !ok || entry["__managed_by"] != "gentle-ai/sdd" {
			continue
		}
		if name == "general" || name == "explore" || strings.HasPrefix(name, "sdd-") || managed[name] {
			selected = append(selected, name)
		}
	}
	if len(selected) == 0 {
		return raw, nil
	}
	sort.Strings(selected)
	text := string(raw)
	if topLevelJSONCKeyCount(text, "agent") != 1 {
		return raw, fmt.Errorf("refuse ambiguous OpenCode agent object")
	}
	_, start, end, ok := topLevelJSONCPropertyValueRange(text, "agent")
	if !ok {
		return raw, fmt.Errorf("missing OpenCode agent object")
	}
	body := text[start:end]
	for _, name := range selected {
		if topLevelJSONCKeyCount(body, name) != 1 {
			return raw, fmt.Errorf("refuse ambiguous OpenCode agent %q", name)
		}
		key, value, finish, ok := topLevelJSONCPropertyValueRange(body, name)
		if !ok || value >= finish || body[value] != '{' {
			return raw, fmt.Errorf("refuse malformed OpenCode agent %q", name)
		}
		role := body[value:finish]
		if !bytes.Equal(stripJSONComments([]byte(role)), []byte(role)) || topLevelJSONCObjectEnd(role) != len(role)-1 {
			return raw, fmt.Errorf("refuse OpenCode agent %q with attached comment", name)
		}
		prev := key - 1
		for prev >= 0 && isJSONWhitespace(body[prev]) {
			prev--
		}
		boundary := prev
		for boundary >= 0 && body[boundary] != ',' && body[boundary] != '{' {
			boundary--
		}
		next := scanJSONCWhitespaceAndComments(body, finish)
		if boundary < 0 || strings.Contains(body[boundary+1:value], "/") || strings.Contains(body[finish:next], "/") {
			return raw, fmt.Errorf("refuse OpenCode agent %q with attached comment", name)
		}
		if managed[name] && name != "general" && name != "explore" && !strings.HasPrefix(name, "sdd-") {
			fresh := map[string]any{}
			for _, field := range []string{"model", "variant"} {
				if v, present := agents[name].(map[string]any)[field]; present {
					fresh[field] = v
				}
			}
			encoded, err := json.Marshal(fresh)
			if err != nil {
				return raw, err
			}
			body = body[:value] + string(encoded) + body[finish:]
			continue
		}
		if next < len(body) && body[next] == ',' {
			finish = next + 1
		} else if prev >= 0 && body[prev] == ',' {
			key = prev
		}
		body = body[:key] + body[finish:]
	}
	updated := []byte(text[:start] + body + text[end:])
	if _, err := unmarshalJSONObject(updated); err != nil {
		return raw, fmt.Errorf("refuse invalid migrated OpenCode settings: %w", err)
	}
	return updated, nil
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
