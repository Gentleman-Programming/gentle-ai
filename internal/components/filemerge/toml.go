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
