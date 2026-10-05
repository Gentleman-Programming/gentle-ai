package filemerge

import (
	"fmt"
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
	return replaceCodexMCPServerBlock(content, "[mcp_servers.engram]", codexEngramBlock)
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

	return replaceCodexMCPServerBlock(content, header, block)
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

	return replaceCodexMCPServerBlock(content, header, block)
}

// replaceCodexMCPServerBlock removes every existing table whose header is
// exactly header, together with its key-value lines, and appends block at EOF.
// Header-like lines inside a multiline string are value text: they neither
// start nor end a removed block, so user strings are kept byte-for-byte.
func replaceCodexMCPServerBlock(content, header, block string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	inString := tomlMultilineStringLines(lines)

	var kept []string
	for i := 0; i < len(lines); {
		if !inString[i] && strings.TrimSpace(lines[i]) == header {
			// Skip the old block header and all its key-value lines.
			i++
			for i < len(lines) {
				if !inString[i] && isTOMLTableHeader(strings.TrimSpace(lines[i])) {
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
// arrays-of-tables — the Codex config does not require those). Lines inside a
// multiline string are value text: they are never headers or key lines, and a
// replaced key's multiline string value is dropped along with it.
func UpsertTOMLTableKey(content, section, key, rawValue string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	header := "[" + section + "]"
	newLine := key + " = " + rawValue
	inString := tomlMultilineStringLines(lines)

	// Find the section header and collect the indices of the key lines within it.
	sectionLine := -1  // line index of the [section] header
	var keyLines []int // indices of lines matching key= or key = inside the section

	inSection := false
	for i, line := range lines {
		if inString[i] {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			sectionLine = i
			inSection = true
			continue
		}
		if inSection {
			// A new [header] ends the current section.
			if isTOMLTableHeader(trimmed) {
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
		for i := 0; i < len(lines); i++ {
			if i == firstKey {
				out = append(out, newLine) // replace in place
			} else if !dupSet[i] {
				out = append(out, lines[i])
				continue
			}
			// Drop the old or duplicate value's multiline string body too.
			for i+1 < len(lines) && inString[i+1] {
				i++
			}
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
// unrelated user configuration. Lines inside a multiline string are value text,
// never headers or keys; a removed key's multiline string value goes with it.
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
	inString := tomlMultilineStringLines(lines)
	inSection := false
	var out []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if inString[i] {
			out = append(out, line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			inSection = true
			out = append(out, line)
			continue
		}
		if inSection && isTOMLTableHeader(trimmed) {
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
				// Drop the removed value's multiline string body too.
				for i+1 < len(lines) && inString[i+1] {
					i++
				}
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
// alone. Header-like lines inside a multiline string are value text: they
// neither start nor end the removed table.
func RemoveTOMLTable(content, tableName string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	header := "[" + tableName + "]"
	inString := tomlMultilineStringLines(lines)

	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if !inString[i] && trimmed == header {
			i++
			for i < len(lines) {
				if !inString[i] && isTOMLTableHeader(strings.TrimSpace(lines[i])) {
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

	// Remove existing occurrences of the key from the root TOML scope only.
	// Assignments after a table or array-table header belong to that table.
	var cleaned []string
	inTopLevel := true
	rootArrayDepth := 0
	var multilineQuote byte
	var removingMultilineQuote byte
	removingArrayDepth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if removingMultilineQuote != 0 || removingArrayDepth != 0 {
			removingMultilineQuote = updateTOMLMultilineString(line, removingMultilineQuote, &removingArrayDepth)
			continue
		}
		if inTopLevel && rootArrayDepth == 0 && multilineQuote == 0 && isTOMLKeyAssignment(trimmed, key) {
			removingMultilineQuote = updateTOMLMultilineString(line, 0, &removingArrayDepth)
			continue
		}
		cleaned = append(cleaned, line)
		if rootArrayDepth == 0 && multilineQuote == 0 && isTOMLTableHeader(trimmed) {
			inTopLevel = false
		}
		multilineQuote = updateTOMLMultilineString(line, multilineQuote, &rootArrayDepth)
	}

	// Find insertion point: before the first [section] header.
	insertAt := len(cleaned)
	rootArrayDepth = 0
	multilineQuote = 0
	for i, line := range cleaned {
		trimmed := strings.TrimSpace(line)
		if rootArrayDepth == 0 && multilineQuote == 0 && isTOMLTableHeader(trimmed) {
			insertAt = i
			break
		}
		multilineQuote = updateTOMLMultilineString(line, multilineQuote, &rootArrayDepth)
	}

	var out []string
	out = append(out, cleaned[:insertAt]...)
	out = append(out, lineValue)
	out = append(out, cleaned[insertAt:]...)

	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

func isTOMLKeyAssignment(line, key string) bool {
	if !strings.HasPrefix(line, key) {
		return false
	}

	remainder := line[len(key):]
	for len(remainder) > 0 && (remainder[0] == ' ' || remainder[0] == '\t') {
		remainder = remainder[1:]
	}
	return strings.HasPrefix(remainder, "=")
}

// tomlMultilineStringLines reports, for each line, whether it begins inside a
// multiline string. Such a line is value text: it is never a table header or a
// key assignment, whatever it looks like.
func tomlMultilineStringLines(lines []string) []bool {
	inString := make([]bool, len(lines))
	var multilineQuote byte
	for i, line := range lines {
		inString[i] = multilineQuote != 0
		multilineQuote = ScanTOMLMultilineString(line, multilineQuote)
	}
	return inString
}

// ScanTOMLMultilineString reports the multiline string state after line,
// given the state before it (0 when outside a multiline string, otherwise the
// open quote byte). It honors quotes, escapes and comments, so delimiters that
// appear inside ordinary strings or comments do not change the state.
func ScanTOMLMultilineString(line string, multilineQuote byte) byte {
	return updateTOMLMultilineString(line, multilineQuote, nil)
}

// updateTOMLMultilineString tracks multiline basic and literal string values.
// When arrayDepth is provided, it also tracks brackets outside strings and comments.
func updateTOMLMultilineString(line string, multilineQuote byte, arrayDepth *int) byte {
	var quote byte
	escaped := false
	for i := 0; i < len(line); i++ {
		char := line[i]
		if multilineQuote != 0 {
			if char == multilineQuote && i+2 < len(line) && line[i+1] == multilineQuote && line[i+2] == multilineQuote && (multilineQuote == '\'' || !escapedTOMLCharacter(line, i)) {
				multilineQuote = 0
				i += 2
			}
			continue
		}

		if quote != 0 {
			if quote == '"' && char == '\\' && !escaped {
				escaped = true
				continue
			}
			if char == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}

		switch char {
		case '#':
			return 0
		case '"', '\'':
			if i+2 < len(line) && line[i+1] == char && line[i+2] == char {
				multilineQuote = char
				i += 2
				continue
			}
			quote = char
		case '[':
			if arrayDepth != nil {
				(*arrayDepth)++
			}
		case ']':
			if arrayDepth != nil && *arrayDepth > 0 {
				(*arrayDepth)--
			}
		}
	}

	return multilineQuote
}

func escapedTOMLCharacter(line string, index int) bool {
	backslashes := 0
	for index > 0 && line[index-1] == '\\' {
		backslashes++
		index--
	}
	return backslashes%2 == 1
}

func isTOMLTableHeader(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}

	var quote byte
	escaped := false
	for i := 0; i < len(line); i++ {
		char := line[i]
		if quote != 0 {
			if quote == '"' && char == '\\' && !escaped {
				escaped = true
				continue
			}
			if char == quote && !escaped {
				quote = 0
			}
			escaped = false
			continue
		}

		switch char {
		case '"', '\'':
			quote = char
		case '#':
			line = strings.TrimSpace(line[:i])
			return strings.HasSuffix(line, "]")
		}
	}

	return strings.HasSuffix(line, "]")
}
