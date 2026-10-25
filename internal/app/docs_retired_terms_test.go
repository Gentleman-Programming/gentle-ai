package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// retiredDocTerms are workflow terms retired in v4.0.0. Living docs must not
// present them as current guidance. "Strict TDD" and "shadow" stay allowed:
// the first is current guidance and the second is ambiguous.
var retiredDocTerms = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"/sdd-", regexp.MustCompile(regexp.QuoteMeta("/sdd-"))},
	{"/gentle-sdd-", regexp.MustCompile(regexp.QuoteMeta("/gentle-sdd-"))},
	{"gentle-ai sdd-", regexp.MustCompile(regexp.QuoteMeta("gentle-ai sdd-"))},
	{"SDD phases", regexp.MustCompile(regexp.QuoteMeta("SDD phases"))},
	{"SDD agents", regexp.MustCompile(regexp.QuoteMeta("SDD agents"))},
	{"OpenSpec", regexp.MustCompile(regexp.QuoteMeta("OpenSpec"))},
	{"--strict-tdd", regexp.MustCompile(regexp.QuoteMeta("--strict-tdd"))},
	{"SDD", regexp.MustCompile(`\bSDD\b`)},
}

// retirementNote is the whole allowlist: a line may name a retired term only
// when the same line says it is retired or legacy, such as the "retired in
// v4.0.0" notes and the legacy sdd-* telemetry schema values.
var retirementNote = regexp.MustCompile(`(?i)\b(retired|legacy)\b`)

// historicalBanner marks a page as a point-in-time record. Such pages open
// with a "> **Historical ..." callout near the top.
var historicalBanner = regexp.MustCompile(`^>\s*\*\*Historical\b`)

func TestLivingDocsDoNotAdvertiseRetiredTerms(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, name := range livingDocs(t, root) {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(content), "\n")
		if isHistoricalDoc(lines) {
			continue
		}
		for index, line := range lines {
			if retirementNote.MatchString(line) {
				continue
			}
			for _, term := range retiredDocTerms {
				if term.pattern.MatchString(line) {
					t.Errorf("%s:%d advertises retired term %q", name, index+1, term.name)
				}
			}
		}
	}
}

// livingDocs returns README.md, CONTRIBUTING.md and docs/**/*.md as
// slash-separated repository paths, without the historical trees.
func livingDocs(t *testing.T, root string) []string {
	t.Helper()
	docs := []string{"README.md", "CONTRIBUTING.md"}
	err := filepath.WalkDir(filepath.Join(root, "docs"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == "docs/audits" || rel == "docs/releases" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".md") || strings.HasPrefix(rel, "docs/architecture/rdd-") {
			return nil
		}
		docs = append(docs, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func isHistoricalDoc(lines []string) bool {
	for _, line := range lines[:min(12, len(lines))] {
		if historicalBanner.MatchString(line) {
			return true
		}
	}
	return false
}
