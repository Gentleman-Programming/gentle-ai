package uninstall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
)

type jsonPath []string

const maxManagedFileSize = 16 << 20

var managedPersonaFingerprints = []string{
	"## Personality",
	"Senior Architect",
	"## Rules",
}

func removeMarkdownSections(content string, sectionIDs ...string) (string, bool) {
	updated := content
	changed := false
	for _, sectionID := range sectionIDs {
		next := filemerge.InjectMarkdownSection(updated, sectionID, "")
		if next != updated {
			changed = true
			updated = next
		}
	}
	return updated, changed
}

func removeManagedPersonaPreamble(content string) (string, bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	markerIdx := strings.Index(normalized, "<!-- gentle-ai:")

	prefix := normalized
	suffix := ""
	if markerIdx >= 0 {
		prefix = normalized[:markerIdx]
		suffix = normalized[markerIdx:]
	}

	if strings.TrimSpace(prefix) == "" || !looksLikeManagedPersonaPrefix(prefix) {
		return content, false
	}

	if markerIdx < 0 {
		return content, false
	}

	suffix = strings.TrimLeft(suffix, "\n")
	return suffix, true
}

func looksLikeManagedPersonaPrefix(prefix string) bool {
	if strings.Contains(prefix, "name: Gentle AI Persona") && strings.Contains(prefix, "description: Teaching-oriented persona") {
		return true
	}

	for _, fingerprint := range managedPersonaFingerprints {
		if !strings.Contains(prefix, fingerprint) {
			return false
		}
	}

	return true
}

func removeJSONPaths(raw []byte, paths ...jsonPath) ([]byte, bool, error) {
	root, err := unmarshalJSONObject(raw)
	if err != nil {
		return nil, false, err
	}

	changed := false
	for _, path := range paths {
		if deleteJSONPath(root, path) {
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal json after cleanup: %w", err)
	}

	eol := detectEOL(string(raw))
	encoded = bytes.ReplaceAll(encoded, []byte("\n"), []byte(eol))
	return append(encoded, []byte(eol)...), true, nil
}

func deleteJSONPath(root map[string]any, path jsonPath) bool {
	if len(path) == 0 {
		return false
	}

	key := path[0]
	value, ok := root[key]
	if !ok {
		return false
	}

	if len(path) == 1 {
		delete(root, key)
		return true
	}

	child, ok := value.(map[string]any)
	if !ok {
		return false
	}

	changed := deleteJSONPath(child, path[1:])
	if changed && len(child) == 0 {
		delete(root, key)
	}
	return changed
}

func jsonIsEmptyObject(raw []byte) bool {
	root, err := unmarshalJSONObject(raw)
	if err != nil {
		return false
	}
	return len(root) == 0
}

func cleanCodexTOML(content string) (string, bool) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	updated := removeTOMLTable(normalized, "mcp_servers.engram")
	updated = removeTopLevelTOMLKeys(updated, "model_instructions_file", "experimental_compact_prompt_file")
	updated = strings.TrimSpace(updated)
	if updated != "" {
		updated += "\n"
	}
	return updated, updated != normalized
}

func removeTOMLTable(content, tableName string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	header := "[" + tableName + "]"

	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == header {
			i++
			for i < len(lines) {
				next := strings.TrimSpace(lines[i])
				if strings.HasPrefix(next, "[") && strings.HasSuffix(next, "]") {
					break
				}
				i++
			}
			continue
		}
		kept = append(kept, lines[i])
		i++
	}

	return strings.Join(kept, "\n")
}

func removeTopLevelTOMLKeys(content string, keys ...string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}

	cleaned := make([]string, 0, len(lines))
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTable = true
			cleaned = append(cleaned, line)
			continue
		}
		if inTable {
			cleaned = append(cleaned, line)
			continue
		}
		remove := false
		for key := range keySet {
			if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=") {
				remove = true
				break
			}
		}
		if !remove {
			cleaned = append(cleaned, line)
		}
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func unmarshalJSONObject(raw []byte) (map[string]any, error) {
	object := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return object, nil
	}

	if err := decodeJSONObject(raw, &object); err == nil {
		return object, nil
	}

	normalized := normalizeJSON(raw)
	if err := decodeJSONObject(normalized, &object); err != nil {
		return nil, fmt.Errorf("unmarshal json object: %w", err)
	}

	return object, nil
}

func decodeJSONObject(raw []byte, target *map[string]any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON payload")
		}
		return err
	}
	return nil
}

func detectEOL(content string) string {
	if strings.Contains(content, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func restoreEOL(content, eol string) string {
	if eol == "\n" {
		return content
	}
	return strings.ReplaceAll(content, "\n", eol)
}

func readManagedFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fmt.Errorf("stat file %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read symlink %q", path)
	}
	if info.Size() > maxManagedFileSize {
		return nil, fmt.Errorf("file %q exceeds max managed size %d bytes", path, maxManagedFileSize)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file %q: %w", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxManagedFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}
	if len(data) > maxManagedFileSize {
		return nil, fmt.Errorf("file %q exceeds max managed size %d bytes", path, maxManagedFileSize)
	}
	return data, nil
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
