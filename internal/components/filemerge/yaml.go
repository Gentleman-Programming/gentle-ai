package filemerge

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/versions"
)

// UpsertYAMLMCPServerBlock removes any existing <serverID>: block nested under
// the top-level mcp_servers: key and re-appends a fresh block with command/args
// (and optional env). If mcp_servers: is absent, it is created. Everything
// outside the managed server block — other servers, top-level keys, and user
// comments — is preserved.
//
// command/args/env are emitted with 2-space indentation:
//
//	mcp_servers:
//	  <serverID>:
//	    command: <command>
//	    args:
//	      - <arg0>
//	      - <arg1>
//	    env:
//	      KEY: value
//
// Idempotent: calling twice with the same arguments yields identical output.
func UpsertYAMLMCPServerBlock(content, serverID, command string, args []string, env map[string]string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	// Build the server sub-block (4-space indent for keys, 6-space for list items).
	block := buildServerBlock(serverID, command, args, env)

	// Locate the top-level mcp_servers: line (zero leading indent, key prefix match).
	// We match any zero-indent line whose trimmed form starts with "mcp_servers:" to
	// handle both the block form ("mcp_servers:") and inline forms ("mcp_servers: {}").
	mcpLineIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "mcp_servers:") && !hasLeadingSpaces(line) {
			mcpLineIdx = i
			break
		}
	}

	if mcpLineIdx == -1 {
		// mcp_servers: is absent — append it at EOF.
		base := strings.TrimRight(strings.Join(lines, "\n"), "\n")
		if base == "" {
			return "mcp_servers:\n" + block
		}
		return base + "\n\nmcp_servers:\n" + block
	}

	// Normalize an inline mcp_servers: value (e.g. "mcp_servers: {}" or
	// "mcp_servers: somevalue") to a bare block-form "mcp_servers:" so the
	// rest of the algorithm can treat it uniformly. This prevents duplicate
	// top-level keys when the user (or a tool) wrote an inline mapping.
	// Preserve any trailing inline comment (e.g. "mcp_servers: {}  # note" → "mcp_servers:  # note").
	if strings.TrimSpace(lines[mcpLineIdx]) != "mcp_servers:" {
		raw := lines[mcpLineIdx]
		// Strip the "mcp_servers:" prefix plus any inline value, keeping a trailing comment.
		rest := strings.TrimPrefix(raw, "mcp_servers:")
		if idx := strings.Index(rest, "#"); idx != -1 {
			// There is a comment — rebuild as "mcp_servers: <comment part>"
			comment := strings.TrimSpace(rest[idx:])
			lines[mcpLineIdx] = "mcp_servers:  # " + strings.TrimPrefix(comment, "# ")
		} else {
			lines[mcpLineIdx] = "mcp_servers:"
		}
	}

	// mcp_servers: is present — find the region of its child lines.
	// The child region spans from mcpLineIdx+1 to the next zero-indent non-blank
	// non-comment line (exclusive), or EOF.
	regionEnd := len(lines)
	for i := mcpLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// A full-line comment at zero indent is treated as part of the region if
		// it immediately precedes the next non-comment section. However to keep the
		// logic simple and consistent with the design requirement, any zero-indent
		// non-blank, non-comment line ends the region.
		if !hasLeadingSpaces(line) && !strings.HasPrefix(trimmed, "#") {
			regionEnd = i
			break
		}
	}

	// Within the region [mcpLineIdx+1, regionEnd), strip any existing <serverID>: block.
	// The server key is at exactly 2-space indent: "  <serverID>:".
	serverKey := "  " + serverID + ":"
	var kept []string
	kept = append(kept, lines[:mcpLineIdx+1]...)

	region := lines[mcpLineIdx+1 : regionEnd]
	i := 0
	for i < len(region) {
		line := region[i]
		trimmed := strings.TrimSpace(line)

		// Match the server key at exactly 2-space indent. The key may have a trailing
		// inline comment (e.g. "  engram: # managed"), so we check whether the
		// non-comment portion of the line matches "  <serverID>:".
		if isServerKeyLine(line, serverKey) {
			// Skip this server's sub-block: all lines that belong to the removed server
			// (indent > 2 spaces), until we reach a sibling boundary.
			// A sibling boundary is:
			//   - a non-blank line at exactly 2-space indent (server key or comment)
			//   - a zero-indent non-blank non-comment line (end of mcp_servers region)
			i++
			for i < len(region) {
				nextLine := region[i]
				nextTrimmed := strings.TrimSpace(nextLine)
				if nextTrimmed == "" {
					// Blank lines at the boundary — include until we hit a sibling.
					i++
					continue
				}
				// A line at exactly 2-space indent (server key OR comment) signals the
				// end of the removed server's sub-block; stop and let the outer loop keep it.
				if len(nextLine) >= 2 && nextLine[:2] == "  " {
					restAfterTwo := nextLine[2:]
					if len(restAfterTwo) > 0 && restAfterTwo[0] != ' ' {
						break // sibling server key or sibling comment — preserve it
					}
				}
				// Also break on zero-indent non-blank lines (end of mcp_servers region).
				if !hasLeadingSpaces(nextLine) && nextTrimmed != "" && !strings.HasPrefix(nextTrimmed, "#") {
					break
				}
				i++
			}
			continue
		}

		// Trim trailing blank lines that belong to the old block boundary.
		if trimmed == "" {
			// We'll handle trailing blanks after the loop.
			kept = append(kept, line)
			i++
			continue
		}

		kept = append(kept, line)
		i++
	}

	// Trim trailing blank lines at the end of the kept mcp_servers region before
	// appending the fresh block.
	for len(kept) > mcpLineIdx+1 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}

	// Append the fresh server block lines.
	blockLines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	kept = append(kept, blockLines...)

	// Append the rest (after the mcp_servers region).
	if regionEnd < len(lines) {
		// Add a blank separator if the next section doesn't start with a blank line.
		if strings.TrimSpace(lines[regionEnd]) != "" {
			kept = append(kept, "")
		}
		kept = append(kept, lines[regionEnd:]...)
	}

	result := strings.Join(kept, "\n")
	result = strings.TrimRight(result, "\n")
	return result + "\n"
}

// buildServerBlock builds the 2-space-indented YAML block for a single MCP server.
// The block does NOT include mcp_servers: — it starts at "  <serverID>:".
func buildServerBlock(serverID, command string, args []string, env map[string]string) string {
	var sb strings.Builder
	sb.WriteString("  " + serverID + ":\n")
	sb.WriteString("    command: " + command + "\n")
	if len(args) > 0 {
		sb.WriteString("    args:\n")
		for _, arg := range args {
			sb.WriteString("      - " + arg + "\n")
		}
	}
	if len(env) > 0 {
		sb.WriteString("    env:\n")
		// Sort keys for deterministic output (idempotency).
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString("      " + k + ": " + env[k] + "\n")
		}
	}
	return sb.String()
}

// hasLeadingSpaces reports whether line starts with a space or tab character.
func hasLeadingSpaces(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

// isServerKeyLine reports whether line matches the given serverKey prefix, allowing
// for an optional trailing inline comment (e.g. "  engram: # managed").
// serverKey has the form "  <serverID>:".
func isServerKeyLine(line, serverKey string) bool {
	// Exact or trailing-whitespace match.
	if strings.TrimRight(line, " \t") == serverKey || line == serverKey {
		return true
	}
	// Match key followed by whitespace and an inline comment.
	if strings.HasPrefix(line, serverKey) {
		rest := line[len(serverKey):]
		rest = strings.TrimLeft(rest, " \t")
		if strings.HasPrefix(rest, "#") {
			return true
		}
	}
	return false
}

// stripInlineComment removes a trailing inline comment from an unquoted YAML scalar.
// It splits on the first occurrence of " #" (space followed by hash) and trims the result.
func stripInlineComment(s string) string {
	if idx := strings.Index(s, " #"); idx != -1 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

// normalizeYAMLScalar strips surrounding matching quotes and inline trailing comments
// from a YAML scalar value.
//
// Order matters: if the value starts with a quote character, locate the matching
// closing quote and return the content between the quotes, discarding everything
// after (including any trailing inline comment). A '#' inside the quotes is NOT
// treated as a comment. Only unquoted values use inline-comment stripping.
func normalizeYAMLScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		quote := s[0]
		// Find the matching closing quote (first occurrence after position 0).
		if close := strings.IndexByte(s[1:], quote); close != -1 {
			// close is relative to s[1:], so absolute index is close+1.
			return s[1 : close+1]
		}
	}
	return stripInlineComment(s)
}

// UpsertHermesEngramBlock is a thin convenience wrapper that upserts the canonical
// engram MCP server block (command=engramCmd, args=["mcp","--tools=agent"], no env).
// Falls back to "engram" when engramCmd is empty. Mirrors UpsertCodexEngramBlock.
func UpsertHermesEngramBlock(content, engramCmd string) string {
	if engramCmd == "" {
		engramCmd = "engram"
	}
	return UpsertYAMLMCPServerBlock(content, "engram", engramCmd, []string{"mcp", "--tools=agent"}, nil)
}

// UpsertHermesContext7Block is a thin convenience wrapper that upserts the canonical
// context7 MCP server block. Context7 is a stdio server; the first slice uses
// the same stdio shape as Codex (command + pinned args from versions.Context7MCP).
func UpsertHermesContext7Block(content string) string {
	args := []string{
		"-y",
		fmt.Sprintf("--package=@upstash/context7-mcp@%s", versions.Context7MCP),
		"--",
		"context7-mcp",
	}
	return UpsertYAMLMCPServerBlock(content, "context7", "npx", args, nil)
}

// ReadYAMLMCPServerCommand recovers the executable of a named MCP server's
// command from a YAML config (read-only — never mutates). It is the YAML
// counterpart of the JSON path inside engram's existingMergedEngramCommand,
// enabling gentle-ai to preserve a command already written for a server
// (e.g. an absolute path) instead of clobbering it on re-run.
//
// Algorithm (hand-rolled, NO gopkg.in/yaml.v3 — read-only block scanning):
//  1. Normalize line endings; split into lines; ignore full-line comments (# ...).
//  2. Locate the top-level mcp_servers: line (trimmed == "mcp_servers:" AND
//     zero leading indent).
//  3. Within its child region (indent > 0, until the next zero-indent non-blank
//     line or EOF), find the <serverID>: block at exactly 2-space indent.
//  4. Within that server's sub-block (indent deeper than the server key, up to
//     the next 2-space sibling or end of region), read command::
//     - scalar string  -> command: engram     => "engram"
//     - YAML list       -> command: then - engram items => first element
//  5. Return ("", false) when mcp_servers / serverID / command is absent or the
//     file does not match the expected shape.
//
// Generalizes to any future YAML-backed agent; engram dispatches to it for Hermes.
func ReadYAMLMCPServerCommand(content string, serverID string) (string, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	// Step 1: Find the top-level mcp_servers: key.
	mcpLineIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue // skip blank and comment lines
		}
		// Accept "mcp_servers:" with an optional trailing inline comment
		// (e.g. "mcp_servers: # note"). The value portion (after the colon) must
		// either be empty or start with "#" after trimming spaces.
		if strings.HasPrefix(trimmed, "mcp_servers:") && !hasLeadingSpaces(line) {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "mcp_servers:"))
			if rest == "" || strings.HasPrefix(rest, "#") {
				mcpLineIdx = i
				break
			}
		}
	}
	if mcpLineIdx == -1 {
		return "", false
	}

	// Step 2: Find the child region of mcp_servers:.
	regionEnd := len(lines)
	for i := mcpLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !hasLeadingSpaces(line) {
			regionEnd = i
			break
		}
	}

	// Step 3: Within the region, find the server key at exactly 2-space indent.
	serverKey := "  " + serverID + ":"
	serverLineIdx := -1
	for i := mcpLineIdx + 1; i < regionEnd; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		// Accept a server key with an optional trailing inline comment
		// (e.g. "  engram: # note").
		if isServerKeyLine(line, serverKey) {
			serverLineIdx = i
			break
		}
	}
	if serverLineIdx == -1 {
		return "", false
	}

	// Step 4: Within the server's sub-block, find the command: line.
	// The sub-block ends at the next sibling (2-space indent) or end of region.
	for i := serverLineIdx + 1; i < regionEnd; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// A 2-space-indent line that is not a comment signals a sibling server.
		if len(line) >= 2 && line[:2] == "  " && len(line) > 2 && line[2] != ' ' {
			break // sibling server key — end of sub-block
		}

		// Look for command: key (at 4-space indent inside the server block).
		if strings.HasPrefix(trimmed, "command:") {
			rest := strings.TrimPrefix(trimmed, "command:")
			rest = strings.TrimSpace(rest)
			if rest != "" {
				// Scalar form: command: <value>
				// Order matters: if the value starts with a quote, find the matching
				// closing quote and extract the content between them, discarding any
				// trailing inline comment (or anything else) after the closing quote.
				// Only unquoted values use space-hash comment stripping.
				if len(rest) >= 2 && (rest[0] == '"' || rest[0] == '\'') {
					quote := rest[0]
					if close := strings.IndexByte(rest[1:], quote); close != -1 {
						rest = rest[1 : close+1]
					}
				} else {
					// Unquoted: strip trailing inline comment (split on first " #").
					if idx := strings.Index(rest, " #"); idx != -1 {
						rest = rest[:idx]
					}
					rest = strings.TrimSpace(rest)
				}
				return rest, true
			}
			// List form: command: followed by list items on the next lines.
			for j := i + 1; j < regionEnd; j++ {
				nextLine := lines[j]
				nextTrimmed := strings.TrimSpace(nextLine)
				if nextTrimmed == "" || strings.HasPrefix(nextTrimmed, "#") {
					continue
				}
				// A list item starts with "- ". Normalize the value: strip surrounding
				// quotes and trailing inline comments (e.g. `- "engram"` → "engram",
				// `- engram  # note` → "engram").
				if strings.HasPrefix(nextTrimmed, "- ") {
					item := strings.TrimPrefix(nextTrimmed, "- ")
					return normalizeYAMLScalar(item), true
				}
				// Anything else (non-list, non-comment, non-blank) means command: was empty
				// and no list items follow — treat as absent.
				break
			}
			return "", false
		}
	}

	return "", false
}
