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
// env is an optional map of environment variables to include in the TOML block.
// When non-nil, each key=value pair is written as `key = "value"` after the
// standard command/args lines.
//
// This is a string-based helper (no TOML parser dependency) ported from
// engram/internal/setup/setup.go. It handles the limited TOML subset that
// Codex uses.
func UpsertCodexEngramBlock(content, engramCmd string, env map[string]string) string {
	if engramCmd == "" {
		engramCmd = "engram"
	}
	// Escape backslashes for TOML double-quoted strings (Windows paths).
	// e.g. C:\Users\foo → C:\\Users\\foo — prevents TOML unicode escape errors (\U).
	escapedCmd := strings.ReplaceAll(engramCmd, `\`, `\\`)

	var b strings.Builder
	b.WriteString("[mcp_servers.engram]\n")
	b.WriteString("command = \"" + escapedCmd + "\"\n")
	b.WriteString("args = [\"mcp\", \"--tools=agent\"]\n")
	for k, v := range env {
		escapedV := strings.ReplaceAll(v, `\`, `\\`)
		b.WriteString(k + " = \"" + escapedV + "\"\n")
	}
	// Remove trailing newline — the caller adds it.
	codexEngramBlock := strings.TrimSuffix(b.String(), "\n")

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
