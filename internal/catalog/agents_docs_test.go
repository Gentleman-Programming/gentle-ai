package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The docs inventory includes every catalog agent, Conductor too: docs/agents.md
// lists it as detection/catalog-only, and the README badge counts it.

func readRepoDoc(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}

// tableSeparatorCell matches a Markdown table separator cell, with or without
// alignment colons (---, :---, ---:, :---:).
var tableSeparatorCell = regexp.MustCompile(`^:?-{3,}:?$`)

// documentedAgentIDs returns the ID column of the agent table in docs/agents.md.
func documentedAgentIDs(t *testing.T, content string) []string {
	t.Helper()
	var ids []string
	inTable := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if !inTable {
			inTable = strings.HasPrefix(line, "| Agent | ID |")
			continue
		}
		if !strings.HasPrefix(line, "|") {
			break
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 || tableSeparatorCell.MatchString(strings.TrimSpace(cells[1])) {
			continue
		}
		ids = append(ids, strings.Trim(strings.TrimSpace(cells[2]), "`"))
	}
	if len(ids) == 0 {
		t.Fatal("docs/agents.md: agent table with header `| Agent | ID |` not found or empty")
	}
	return ids
}

func TestAgentsDocMatchesCatalog(t *testing.T) {
	documented := map[string]bool{}
	for _, id := range documentedAgentIDs(t, readRepoDoc(t, "docs/agents.md")) {
		if documented[id] {
			t.Errorf("docs/agents.md lists agent %q more than once", id)
		}
		documented[id] = true
	}

	var missing []string
	for _, agent := range AllAgents() {
		id := string(agent.ID)
		if !documented[id] {
			missing = append(missing, id)
		}
		delete(documented, id)
	}
	var extra []string
	for id := range documented {
		extra = append(extra, id)
	}
	sort.Strings(extra)

	if len(missing) > 0 || len(extra) > 0 {
		t.Errorf("docs/agents.md agent table drifted from catalog allAgents\nmissing from docs: %v\nnot in catalog: %v", missing, extra)
	}
}

func TestReadmeAgentsBadgeMatchesCatalog(t *testing.T) {
	readme := readRepoDoc(t, "README.md")
	want := len(AllAgents())

	for _, check := range []struct {
		label string
		re    *regexp.Regexp
	}{
		{"badge URL", regexp.MustCompile(`img\.shields\.io/badge/agents-(\d+)-`)},
		{"badge alt text", regexp.MustCompile(`alt="(\d+) agents"`)},
	} {
		match := check.re.FindStringSubmatch(readme)
		if match == nil {
			t.Errorf("README.md: agents %s not found (pattern %s)", check.label, check.re)
			continue
		}
		if got, _ := strconv.Atoi(match[1]); got != want {
			t.Errorf("README.md: agents %s says %d, catalog has %d agents", check.label, got, want)
		}
	}
}

func TestDocumentedAgentIDsParsesOnlyTheAgentTable(t *testing.T) {
	content := "intro | not a table\n\n| Agent | ID | Integration notes |\n| :--- | :---: | --- |\n" +
		"| <a id=\"pi\"></a>Pi | `pi` | Notes with `code` |\n| Codex | `codex` | Notes |\n\n| Other | ID |\n| x | `ghost` |\n"
	got := strings.Join(documentedAgentIDs(t, content), ",")
	if got != "pi,codex" {
		t.Fatalf("documentedAgentIDs = %q, want %q", got, "pi,codex")
	}
}
