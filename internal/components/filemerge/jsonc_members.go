package filemerge

import (
	"bytes"
	"fmt"
	"strings"
)

// RemoveJSONCMembers deletes the named members of the object found by
// following path from the document root, editing only their own text: every
// other byte, comment, and trailing comma is preserved. A member with a
// comment inside it or attached to it, or one spelled with escapes, is kept
// and returned in kept, since deleting it would discard the user's note.
// When dropEmpty is set and the object ends up with no members, the object's
// own member is removed from its parent the same way. A missing path is not
// an error; duplicate keys anywhere in the document are refused.
func RemoveJSONCMembers(raw []byte, path []string, names []string, dropEmpty bool) ([]byte, []string, error) {
	if len(path) == 0 || len(names) == 0 {
		return raw, nil, nil
	}
	if err := RejectDuplicateJSONKeys(raw); err != nil {
		return raw, nil, fmt.Errorf("refuse settings with duplicate or malformed keys: %w", err)
	}
	root, err := unmarshalJSONObject(raw)
	if err != nil {
		return raw, nil, fmt.Errorf("refuse malformed settings: %w", err)
	}
	object := root
	for _, key := range path {
		next, ok := object[key].(map[string]any)
		if !ok {
			return raw, nil, nil
		}
		object = next
	}
	text := string(raw)
	// spans[i] is the [start, end) range of the object at depth i in text;
	// spans[0] is the whole document.
	spans := [][2]int{{0, len(text)}}
	for _, key := range path {
		parent := spans[len(spans)-1]
		_, start, end, ok := topLevelJSONCPropertyValueRange(text[parent[0]:parent[1]], key)
		if !ok || topLevelJSONCKeyCount(text[parent[0]:parent[1]], key) != 1 {
			// Present when decoded but absent or ambiguous as text: an
			// escaped spelling the text edit cannot locate.
			return raw, append([]string(nil), names...), nil
		}
		spans = append(spans, [2]int{parent[0] + start, parent[0] + end})
	}
	target := spans[len(spans)-1]
	content := text[target[0]:target[1]]
	var kept []string
	removed := 0
	for _, name := range names {
		if _, present := object[name]; !present {
			continue
		}
		updated, ok := removeJSONCMember(content, name)
		if !ok {
			kept = append(kept, name)
			continue
		}
		content = updated
		removed++
	}
	if removed == 0 {
		return raw, kept, nil
	}
	shift := len(content) - (target[1] - target[0])
	text = text[:target[0]] + content + text[target[1]:]
	if dropEmpty && removed == len(object) {
		parent := spans[len(spans)-2]
		start, end := parent[0], parent[1]+shift
		if updated, ok := removeJSONCMember(text[start:end], path[len(path)-1]); ok {
			text = text[:start] + updated + text[end:]
		}
	}
	return []byte(text), kept, nil
}

// removeJSONCMember removes one member from the object text, which must
// start at the object's opening brace (or be a whole document). It reports
// false, leaving the text unchanged, when the member is absent as text or a
// comment is inside it or attached to it.
func removeJSONCMember(object, name string) (string, bool) {
	if topLevelJSONCKeyCount(object, name) != 1 {
		return object, false
	}
	key, start, _, ok := topLevelJSONCPropertyValueRange(object, name)
	if !ok {
		return object, false
	}
	// The property range extends over comments that follow the value; the
	// member itself ends with its value.
	end := jsonValueEnd(object, start)
	member := []byte(object[key:end])
	if !bytes.Equal(member, stripJSONComments(member)) {
		return object, false
	}
	previous := key - 1
	for previous >= 0 && isJSONWhitespace(object[previous]) {
		previous--
	}
	if previous < 0 || (object[previous] != ',' && object[previous] != '{') {
		return object, false
	}
	next, sameLine := end, true
	for next < len(object) && isJSONWhitespace(object[next]) {
		sameLine = sameLine && object[next] != '\n'
		next++
	}
	commented := next+1 < len(object) && object[next] == '/' && (object[next+1] == '/' || object[next+1] == '*')
	if commented {
		// A comment on the member's last line is attached to it; one on a
		// later line belongs to the closing brace and stays.
		if sameLine {
			return object, false
		}
		next = scanJSONCWhitespaceAndComments(object, next)
	}
	switch {
	case next < len(object) && object[next] == ',' && commented:
		// The comma follows a comment that belongs to the next member: take
		// the preceding comma instead, as for a last member.
		if object[previous] != ',' {
			return object, false
		}
		return object[:previous] + object[end:], true
	case next < len(object) && object[next] == ',':
		after := next + 1
		for after < len(object) && (object[after] == ' ' || object[after] == '\t' || object[after] == '\r') {
			after++
		}
		if after+1 < len(object) && object[after] == '/' && (object[after+1] == '/' || object[after+1] == '*') {
			return object, false
		}
		if after < len(object) && object[after] == '\n' {
			after++
		}
		start := key
		if line := strings.LastIndexByte(object[:key], '\n') + 1; strings.TrimLeft(object[line:key], " \t") == "" {
			start = line
		}
		return object[:start] + object[after:], true
	case next < len(object) && object[next] == '}' && commented && object[previous] == ',':
		// Keep the preceding comma: without it the comment would follow the
		// previous member's value and count as nested in it.
		start, after := key, end
		if line := strings.LastIndexByte(object[:key], '\n') + 1; strings.TrimLeft(object[line:key], " \t") == "" {
			start = line
			after += len(object[after:]) - len(strings.TrimLeft(object[after:], " \t\r"))
			if after < len(object) && object[after] == '\n' {
				after++
			}
		}
		return object[:start] + object[after:], true
	case next < len(object) && object[next] == '}':
		// The last member takes the preceding comma; the only member leaves
		// an empty object.
		cut := previous + 1
		if object[previous] == ',' {
			cut = previous
		}
		return object[:cut] + object[end:], true
	}
	return object, false
}

// jsonValueEnd returns the index just past the JSON value starting at start.
func jsonValueEnd(content string, start int) int {
	if start >= len(content) {
		return start
	}
	switch content[start] {
	case '"':
		for i := start + 1; i < len(content); i++ {
			switch content[i] {
			case '\\':
				i++
			case '"':
				return i + 1
			}
		}
		return len(content)
	case '{', '[':
		depth := 0
		for i := start; i < len(content); i++ {
			switch ch := content[i]; {
			case ch == '"':
				i = jsonValueEnd(content, i) - 1
			case ch == '/' && i+1 < len(content) && (content[i+1] == '/' || content[i+1] == '*'):
				i = scanJSONCWhitespaceAndComments(content, i) - 1
			case ch == '{' || ch == '[':
				depth++
			case ch == '}' || ch == ']':
				depth--
				if depth == 0 {
					return i + 1
				}
			}
		}
		return len(content)
	}
	i := start
	for i < len(content) && !isJSONWhitespace(content[i]) && !strings.ContainsRune(",}]/", rune(content[i])) {
		i++
	}
	return i
}
