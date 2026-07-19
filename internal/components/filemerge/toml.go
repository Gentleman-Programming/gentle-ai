package filemerge

import (
	"fmt"
	"strconv"
	"strings"
)

// UpsertCodexEngramBlock removes any existing [mcp_servers.engram] block from
// the given TOML content and appends a fresh block with the canonical engram
// MCP entry (including --tools=agent). All other sections are preserved.
//
// engramCmd is the command string to use (e.g. an absolute path like
// "/usr/local/bin/engram"). If engramCmd is empty, it falls back to "engram".
//
// This is a string-based helper (no TOML parser dependency) ported from
// engram/internal/setup/setup.go. It handles the limited TOML subset that
// Codex uses.
func UpsertCodexEngramBlock(content, engramCmd string) string {
	if engramCmd == "" {
		engramCmd = "engram"
	}
	// Escape backslashes for TOML double-quoted strings (Windows paths).
	// e.g. C:\Users\foo → C:\\Users\\foo — prevents TOML unicode escape errors (\U).
	escapedCmd := strings.ReplaceAll(engramCmd, `\`, `\\`)
	codexEngramBlock := "[mcp_servers.engram]\ncommand = \"" + escapedCmd + "\"\nargs = [\"mcp\", \"--tools=agent\"]"
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var kept []string
	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "[mcp_servers.engram]" {
			// Skip the old block header and all its key-value lines.
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

	base := strings.TrimSpace(strings.Join(kept, "\n"))
	if base == "" {
		return codexEngramBlock + "\n"
	}

	return base + "\n\n" + codexEngramBlock + "\n"
}

// UpsertCodexMCPServerBlock removes any existing [mcp_servers.<serverID>] block
// from the given TOML content and appends a fresh block with the provided
// command and args. This is a generalized helper for any stdio MCP server that
// Codex hosts via its config.toml. Backslashes in command and args are escaped
// for TOML double-quoted strings (Windows paths).
//
// This is a string-based helper (no TOML parser dependency) following the same
// pattern as UpsertCodexEngramBlock.
func UpsertCodexMCPServerBlock(content, serverID, command string, args []string) string {
	header := "[mcp_servers." + serverID + "]"

	escapedCmd := strings.ReplaceAll(command, `\`, `\\`)

	// Build TOML args array: args = ["-y", "--package=...", "--", "context7-mcp"]
	var quotedArgs []string
	for _, arg := range args {
		escaped := strings.ReplaceAll(arg, `\`, `\\`)
		quotedArgs = append(quotedArgs, `"`+escaped+`"`)
	}
	argsLine := "args = [" + strings.Join(quotedArgs, ", ") + "]"

	block := header + "\ncommand = \"" + escapedCmd + "\"\n" + argsLine

	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var kept []string
	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == header {
			// Skip the old block header and all its key-value lines.
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

	base := strings.TrimSpace(strings.Join(kept, "\n"))
	if base == "" {
		return block + "\n"
	}

	return base + "\n\n" + block + "\n"
}

// UpsertCodexRemoteMCPServerBlock removes any existing [mcp_servers.<serverID>]
// block from the given TOML content and appends a fresh remote MCP block using
// Codex's `url = "..."`
// shape. This migrates legacy local stdio blocks by dropping stale command/args
// lines while preserving unrelated config.
func UpsertCodexRemoteMCPServerBlock(content, serverID, url string) string {
	header := "[mcp_servers." + serverID + "]"
	escapedURL := strings.ReplaceAll(url, `\`, `\\`)
	block := header + "\nurl = \"" + escapedURL + "\""

	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	var kept []string
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

	base := strings.TrimSpace(strings.Join(kept, "\n"))
	if base == "" {
		return block + "\n"
	}

	return base + "\n\n" + block + "\n"
}

// UpsertTOMLTableKey upserts `key = rawValue` inside the named [section] table.
// rawValue is the already-formatted TOML right-hand side: the caller supplies a
// bare boolean/integer (false, 4) or a pre-quoted string ("value") — the helper
// writes it verbatim, staying type-agnostic and parser-free.
//
// Behaviour:
//   - If [section] exists: any existing line whose trimmed prefix is `key ` or
//     `key=` is removed, then `key = rawValue` is inserted as the first line
//     after the [section] header.
//   - If [section] does not exist: `\n[section]\nkey = rawValue` is appended at
//     EOF.
//
// All other sections and top-level keys are preserved verbatim. The result is
// idempotent: calling with the same arguments twice yields the same output.
// Only the simple single-line-per-key subset is handled (no inline tables or
// arrays-of-tables — the Codex config does not require those).
func UpsertTOMLTableKey(content, section, key, rawValue string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	header := "[" + section + "]"
	newLine := key + " = " + rawValue

	// Find the section header and collect the indices of the key lines within it.
	sectionLine := -1  // line index of the [section] header
	var keyLines []int // indices of lines matching key= or key = inside the section

	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			sectionLine = i
			inSection = true
			continue
		}
		if inSection {
			// A new [header] ends the current section.
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				inSection = false
				continue
			}
			// Detect an existing occurrence of the key within this section.
			if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=") {
				keyLines = append(keyLines, i)
			}
		}
	}

	if sectionLine == -1 {
		// Section absent — append it at EOF.
		base := strings.TrimSpace(strings.Join(lines, "\n"))
		if base == "" {
			return header + "\n" + newLine + "\n"
		}
		return base + "\n\n" + header + "\n" + newLine + "\n"
	}

	if len(keyLines) > 0 {
		// Key already exists in the section.
		// Replace the first occurrence in place; drop any duplicates.
		firstKey := keyLines[0]
		dupSet := make(map[int]bool, len(keyLines)-1)
		for _, idx := range keyLines[1:] {
			dupSet[idx] = true
		}

		var out []string
		for i, line := range lines {
			if dupSet[i] {
				continue // drop duplicates
			}
			if i == firstKey {
				out = append(out, newLine) // replace in place
				continue
			}
			out = append(out, line)
		}
		return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
	}

	// Section present but key absent — insert as the first line after the header.
	var out []string
	for i, line := range lines {
		out = append(out, line)
		if i == sectionLine {
			out = append(out, newLine)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// RemoveTOMLTableKeys removes simple single-line keys from the named [section]
// table while preserving every other top-level key and table verbatim.
//
// It intentionally matches only exact TOML keys in the target section. This is
// useful for cleaning up previously generated entries without disturbing
// unrelated user configuration.
func RemoveTOMLTableKeys(content, section string, keys []string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if len(keys) == 0 {
		return strings.TrimSpace(content) + "\n"
	}

	removeKeys := make(map[string]bool, len(keys))
	for _, key := range keys {
		removeKeys[key] = true
	}

	header := "[" + section + "]"
	lines := strings.Split(content, "\n")
	inSection := false
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			inSection = true
			out = append(out, line)
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = false
		}
		if inSection {
			removeLine := false
			for key := range removeKeys {
				if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=") {
					removeLine = true
					break
				}
			}
			if removeLine {
				continue
			}
		}
		out = append(out, line)
	}

	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// RemoveTOMLTable removes a complete single TOML table and its key-value lines.
// It preserves every other table and top-level key verbatim, normalizing CRLF to
// LF like the other string-based TOML merge helpers in this package.
//
// It matches only the exactly-spelled [tableName] header and leaves subtables
// alone. Use RemoveTOMLTableTree instead to remove a table together with all
// of its subtables with quote-aware segment matching.
func RemoveTOMLTable(content, tableName string) string {
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

// RemoveTOMLTableTree removes the named table and every subtable beneath it —
// the [tableName] header, all [tableName.*] headers, and their key-value
// lines — plus dotted assignments whose effective key path falls under the
// same prefix. Header and key segments are compared after TOML unquoting, so
// [permissions."gentle-dev"] matches permissions.gentle-dev. Every other line
// is preserved byte-for-byte so user-authored content, spacing, and comments
// stay untouched. If tableName itself does not parse as a TOML key path, the
// content is returned unchanged (silent no-op) — removal never guesses.
//
// Use RemoveTOMLTable instead when a single exactly-spelled [header] block
// should be replaced or dropped without touching its subtables.
func RemoveTOMLTableTree(content, tableName string) string {
	target, ok := parseTOMLKeyPath(tableName)
	if !ok {
		return content
	}

	lines := strings.SplitAfter(content, "\n")
	kept := make([]string, 0, len(lines))
	var tablePath []string
	tableKnown := true
	removing := false
	var multilineQuote byte
	removingMultilineValue := false
	removingArrayDepth := 0
	for _, line := range lines {
		if removingArrayDepth > 0 {
			advanceTOMLLexicalState(line, &multilineQuote, &removingArrayDepth)
			continue
		}
		insideMultiline := multilineQuote != 0
		advanceTOMLMultilineState(line, &multilineQuote)
		if removingMultilineValue {
			removingMultilineValue = multilineQuote != 0
			continue
		}
		if removing {
			if equals := tomlIndexOutsideQuotes(line, '='); equals != -1 &&
				strings.HasPrefix(strings.TrimSpace(line[equals+1:]), "[") {
				var valueMultiline byte
				advanceTOMLLexicalState(line[equals+1:], &valueMultiline, &removingArrayDepth)
				multilineQuote = valueMultiline
			}
		}
		if !insideMultiline && strings.HasPrefix(strings.TrimSpace(line), "[") {
			if path, isHeader := parseTOMLTableHeader(line); isHeader {
				tablePath = path
				tableKnown = true
				removing = hasTOMLKeyPathPrefix(path, target)
				if removing {
					continue
				}
			} else {
				// Malformed header-like line: the table context is now
				// unknown, so stop removing and keep every following
				// assignment until a well-formed header re-establishes
				// context. User content is never silently consumed.
				removing = false
				tableKnown = false
			}
			kept = append(kept, line)
			continue
		}
		if removing {
			continue
		}
		if tableKnown {
			if keyPath, isAssignment := parseTOMLAssignmentKey(line); isAssignment {
				fullPath := append(append([]string(nil), tablePath...), keyPath...)
				if hasTOMLKeyPathPrefix(fullPath, target) {
					if equals := tomlIndexOutsideQuotes(line, '='); equals != -1 &&
						strings.HasPrefix(strings.TrimSpace(line[equals+1:]), "[") {
						var valueMultiline byte
						advanceTOMLLexicalState(line[equals+1:], &valueMultiline, &removingArrayDepth)
						multilineQuote = valueMultiline
					} else {
						removingMultilineValue = multilineQuote != 0
					}
					continue
				}
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

// RemoveEmptyTOMLTableTree removes a target tree only when it contains no
// assignments, comments, or malformed content. Anything else may be
// user-authored and is preserved.
func RemoveEmptyTOMLTableTree(content, tableName string) string {
	target, ok := parseTOMLKeyPath(tableName)
	if !ok {
		return content
	}

	inTarget := false
	var tablePath []string
	for _, line := range strings.SplitAfter(content, "\n") {
		if path, isHeader := parseTOMLTableHeader(line); isHeader {
			tablePath = path
			inTarget = hasTOMLKeyPathPrefix(path, target)
			continue
		}
		if keyPath, isAssignment := parseTOMLAssignmentKey(line); isAssignment {
			fullPath := append(append([]string(nil), tablePath...), keyPath...)
			if hasTOMLKeyPathPrefix(fullPath, target) {
				return content
			}
		}
		if inTarget && strings.TrimSpace(line) != "" {
			return content
		}
	}
	return RemoveTOMLTableTree(content, tableName)
}

// RemoveTopLevelTOMLKeyIfValue removes a top-level `key = rawValue` assignment
// only when its right-hand side matches rawValue exactly (after trimming
// whitespace). The key is matched after TOML unquoting, so "approval_policy"
// matches approval_policy. Scanning stops at the first table header so
// identical keys inside tables are never touched, and a user-customized value
// — including one annotated with a trailing comment — is preserved. All other
// lines stay byte-for-byte identical.
func RemoveTopLevelTOMLKeyIfValue(content, key, rawValue string) string {
	lines := strings.SplitAfter(content, "\n")
	kept := make([]string, 0, len(lines))
	topLevel := true
	var multilineQuote byte
	arrayDepth := 0
	for _, line := range lines {
		if topLevel && arrayDepth > 0 {
			advanceTOMLLexicalState(line, &multilineQuote, &arrayDepth)
			kept = append(kept, line)
			continue
		}
		insideMultiline := multilineQuote != 0
		advanceTOMLMultilineState(line, &multilineQuote)
		if topLevel && !insideMultiline {
			code := tomlCodeBeforeComment(line)
			if equals := tomlIndexOutsideQuotes(code, '='); equals != -1 {
				keyPath, isKey := parseTOMLKeyPath(line[:equals])
				if isKey && len(keyPath) == 1 && keyPath[0] == key &&
					strings.TrimSpace(line[equals+1:]) == rawValue {
					continue
				}
				if isKey && strings.HasPrefix(strings.TrimSpace(code[equals+1:]), "[") {
					var arrayQuote byte
					advanceTOMLLexicalState(line[equals+1:], &arrayQuote, &arrayDepth)
				}
			} else if strings.HasPrefix(strings.TrimSpace(line), "[") {
				topLevel = false
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

// RemoveTOMLKeyIfValue removes one fully-qualified assignment only when its
// value matches exactly. Unknown values and neighboring keys remain untouched.
func RemoveTOMLKeyIfValue(content, key, rawValue string) string {
	target, ok := parseTOMLKeyPath(key)
	if !ok {
		return content
	}

	lines := strings.SplitAfter(content, "\n")
	kept := make([]string, 0, len(lines))
	var tablePath []string
	tableKnown := true
	var multilineQuote byte
	arrayDepth := 0
	for _, line := range lines {
		if arrayDepth > 0 {
			advanceTOMLLexicalState(line, &multilineQuote, &arrayDepth)
			kept = append(kept, line)
			continue
		}
		insideMultiline := multilineQuote != 0
		advanceTOMLMultilineState(line, &multilineQuote)
		if !insideMultiline {
			code := tomlCodeBeforeComment(line)
			if equals := tomlIndexOutsideQuotes(code, '='); equals != -1 {
				keyPath, isAssignment := parseTOMLKeyPath(code[:equals])
				fullPath := append(append([]string(nil), tablePath...), keyPath...)
				if tableKnown && isAssignment && len(fullPath) == len(target) &&
					hasTOMLKeyPathPrefix(fullPath, target) && strings.TrimSpace(line[equals+1:]) == rawValue {
					continue
				}
				if isAssignment && strings.HasPrefix(strings.TrimSpace(code[equals+1:]), "[") {
					var valueMultiline byte
					advanceTOMLLexicalState(line[equals+1:], &valueMultiline, &arrayDepth)
					multilineQuote = valueMultiline
				}
			} else if strings.HasPrefix(strings.TrimSpace(line), "[") {
				path, isHeader := parseTOMLTableHeader(line)
				tablePath, tableKnown = path, isHeader
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "")
}

// The helpers below are a minimal quote-aware TOML key-path parser used by the
// removal helpers. They handle bare, single-quoted, and double-quoted key
// segments plus comments outside quotes, without a TOML parser dependency.

func parseTOMLTableHeader(line string) ([]string, bool) {
	code := strings.TrimSpace(tomlCodeBeforeComment(line))
	if len(code) < 3 || code[0] != '[' {
		return nil, false
	}

	if strings.HasPrefix(code, "[[") {
		if len(code) < 5 || !strings.HasSuffix(code, "]]") {
			return nil, false
		}
		return parseTOMLKeyPath(code[2 : len(code)-2])
	}
	if !strings.HasSuffix(code, "]") {
		return nil, false
	}
	return parseTOMLKeyPath(code[1 : len(code)-1])
}

func parseTOMLAssignmentKey(line string) ([]string, bool) {
	code := tomlCodeBeforeComment(line)
	equals := tomlIndexOutsideQuotes(code, '=')
	if equals == -1 {
		return nil, false
	}
	return parseTOMLKeyPath(code[:equals])
}

func tomlCodeBeforeComment(line string) string {
	if comment := tomlIndexOutsideQuotes(line, '#'); comment != -1 {
		return line[:comment]
	}
	return line
}

func tomlIndexOutsideQuotes(text string, target byte) int {
	var quote byte
	escaped := false
	for i := 0; i < len(text); i++ {
		char := text[i]
		if quote == '"' && escaped {
			escaped = false
			continue
		}
		if quote == '"' && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' || char == '\'' {
			if quote == 0 {
				quote = char
			} else if quote == char {
				quote = 0
			}
			continue
		}
		if quote == 0 && char == target {
			return i
		}
	}
	return -1
}

// advanceTOMLMultilineState tracks whether subsequent lines are inside a TOML
// multiline basic or literal string. Single-line strings and comments are
// skipped so quote-like text in either cannot alter the lexical state.
func advanceTOMLMultilineState(line string, multilineQuote *byte) {
	advanceTOMLLexicalState(line, multilineQuote, nil)
}

func advanceTOMLLexicalState(line string, multilineQuote *byte, arrayDepth *int) {
	var quote byte
	for pos := 0; pos < len(line); {
		char := line[pos]
		if *multilineQuote != 0 {
			if char == *multilineQuote && pos+2 < len(line) &&
				line[pos+1] == char && line[pos+2] == char {
				for pos < len(line) && line[pos] == char {
					pos++
				}
				*multilineQuote = 0
				continue
			}
			if *multilineQuote == '"' && char == '\\' && pos+1 < len(line) {
				pos += 2
				continue
			}
			pos++
			continue
		}

		if quote != 0 {
			if quote == '"' && char == '\\' && pos+1 < len(line) {
				pos += 2
				continue
			}
			if char == quote {
				quote = 0
			}
			pos++
			continue
		}

		if char == '#' {
			return
		}
		if (char == '"' || char == '\'') && pos+2 < len(line) &&
			line[pos+1] == char && line[pos+2] == char {
			*multilineQuote = char
			pos += 3
			continue
		}
		if char == '"' || char == '\'' {
			quote = char
		}
		if arrayDepth != nil {
			switch char {
			case '[':
				(*arrayDepth)++
			case ']':
				if *arrayDepth > 0 {
					(*arrayDepth)--
				}
			}
		}
		pos++
	}
}

func parseTOMLKeyPath(text string) ([]string, bool) {
	var path []string
	for pos := 0; ; {
		pos = skipTOMLKeyWhitespace(text, pos)
		part, next, ok := parseTOMLKeyPart(text, pos)
		if !ok {
			return nil, false
		}
		path = append(path, part)
		pos = skipTOMLKeyWhitespace(text, next)
		if pos == len(text) {
			return path, true
		}
		if text[pos] != '.' {
			return nil, false
		}
		pos++
	}
}

func skipTOMLKeyWhitespace(text string, pos int) int {
	for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
		pos++
	}
	return pos
}

func parseTOMLKeyPart(text string, pos int) (string, int, bool) {
	if pos >= len(text) {
		return "", pos, false
	}
	if text[pos] == '\'' {
		if end := strings.IndexByte(text[pos+1:], '\''); end != -1 {
			return text[pos+1 : pos+1+end], pos + end + 2, true
		}
		return "", pos, false
	}
	if text[pos] == '"' {
		for end := pos + 1; end < len(text); end++ {
			if text[end] == '\\' {
				end++
				continue
			}
			if text[end] == '"' {
				value, ok := unquoteTOMLBasicKey(text[pos : end+1])
				return value, end + 1, ok
			}
		}
		return "", pos, false
	}

	end := pos
	for end < len(text) && isBareTOMLKeyByte(text[end]) {
		end++
	}
	if end == pos {
		return "", pos, false
	}
	return text[pos:end], end, true
}

func unquoteTOMLBasicKey(text string) (string, bool) {
	for pos := 1; pos < len(text)-1; pos++ {
		if text[pos] != '\\' {
			continue
		}
		pos++
		if pos >= len(text)-1 {
			return "", false
		}
		switch text[pos] {
		case 'b', 't', 'n', 'f', 'r', '"', '\\':
		case 'u':
			if !hasTOMLHexDigits(text, pos+1, 4) {
				return "", false
			}
			pos += 4
		case 'U':
			if !hasTOMLHexDigits(text, pos+1, 8) {
				return "", false
			}
			pos += 8
		default:
			return "", false
		}
	}

	value, err := strconv.Unquote(text)
	return value, err == nil
}

func hasTOMLHexDigits(text string, start, count int) bool {
	if start+count > len(text)-1 {
		return false
	}
	for _, char := range text[start : start+count] {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}

func isBareTOMLKeyByte(char byte) bool {
	return char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-'
}

func hasTOMLKeyPathPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

// UpsertTopLevelTOMLString inserts or replaces a top-level key = "value" pair
// in TOML content. The key is placed before the first [section] header so it
// remains a top-level (non-table) setting. Existing occurrences of the key are
// removed before inserting the new value (idempotent).
//
// Ported from engram/internal/setup/setup.go.
func UpsertTopLevelTOMLString(content, key, value string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	lineValue := fmt.Sprintf("%s = %q", key, value)

	// Remove all existing occurrences of the key.
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=") {
			continue
		}
		cleaned = append(cleaned, line)
	}

	// Find insertion point: before the first [section] header.
	insertAt := len(cleaned)
	for i, line := range cleaned {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			insertAt = i
			break
		}
	}

	var out []string
	out = append(out, cleaned[:insertAt]...)
	out = append(out, lineValue)
	out = append(out, cleaned[insertAt:]...)

	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}
