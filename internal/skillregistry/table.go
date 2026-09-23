package skillregistry

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// PathFormat selects how renderSkillsTable renders each skill's discovered
// path in the Path column (decision O-2, 2026-09-23).
type PathFormat int

const (
	// PathDiscovered renders the path exactly as the scan discovered it
	// (absolute unless --cwd is relative). This is what .atl/skill-registry.md
	// publishes: its role is a machine-local index of exact paths.
	PathDiscovered PathFormat = iota
	// PathRepoRelative renders project-scope skills relative to the repository
	// root (slash-separated) so the AGENTS.md table stays navigable in GitHub.
	// User-scope skills keep their discovered (absolute) path: they live
	// outside the repository and have no relative form (spec §3.4).
	PathRepoRelative
)

// skillsTableHeader is the fixed four-column header of spec §3.4. The three
// column table (`Skill | Trigger | Path`) it replaces is obsolete.
const skillsTableHeader = "| Skill | Trigger / description | Scope | Path |\n| --- | --- | --- | --- |"

// renderSkillsTable is the single table renderer shared by .atl/skill-registry.md
// and the managed `## Skills` content of AGENTS.md (spec §3.4: "Alineado con
// .atl/skill-registry.md para mantener un único formato reconocible"). The
// format parameter is decision O-2: same table everywhere, only the Path cell
// rendering differs. Rows are sorted by Skill ascending. The returned block has
// no trailing newline.
func renderSkillsTable(cwd string, entries []SkillEntry, format PathFormat) string {
	sorted := make([]SkillEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var b strings.Builder
	b.WriteString(skillsTableHeader)
	for _, entry := range sorted {
		scope := ScopeForPath(cwd, entry.Path)
		path := renderSkillPath(cwd, entry.Path, scope, format)
		fmt.Fprintf(&b, "\n| `%s` | %s | %s | `%s` |",
			markdownCell(entry.Name),
			markdownCell(entry.Description),
			markdownCell(scope),
			markdownCell(path))
	}
	return b.String()
}

// renderSkillPath applies decision O-2 to one Path cell. Under PathRepoRelative
// a project skill is rendered relative to the repository root with forward
// slashes (navigable from GitHub on every platform); anything else keeps the
// discovered path.
func renderSkillPath(cwd, path, scope string, format PathFormat) string {
	if format != PathRepoRelative || scope != "project" {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// markdownCell collapses newlines into spaces, escapes `|` and substitutes an
// em dash for empty values so a table cell never breaks the row.
func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "—"
	}
	return trimmed
}
