package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicBundledSkillsMatchEmbeddedAssets(t *testing.T) {
	for _, skill := range []string{"systemic-issue-triage", "gentle-ai-bench", "chained-pr"} {
		t.Run(skill, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join("..", "..", "skills", skill, "SKILL.md"))
			if err != nil {
				t.Fatalf("ReadFile(public source) error = %v", err)
			}

			embedded, err := Read("skills/" + skill + "/SKILL.md")
			if err != nil {
				t.Fatalf("Read(embedded asset) error = %v", err)
			}
			if !bytes.Equal(source, []byte(embedded)) {
				t.Fatal("public source skill and embedded distribution asset differ")
			}
		})
	}
}

func TestChainedPRSkillContract(t *testing.T) {
	const (
		embeddedPath = "skills/chained-pr/SKILL.md"
		detailsPath  = "skills/chained-pr/references/chaining-details.md"
	)

	skill, err := Read(embeddedPath)
	if err != nil {
		t.Fatalf("Read(%q) error = %v", embeddedPath, err)
	}
	details, err := Read(detailsPath)
	if err != nil {
		t.Fatalf("Read(%q) error = %v", detailsPath, err)
	}

	for _, required := range []string{
		"name: gentle-ai-chained-pr",
		"github/gh-stack", "v0.1.0", "a1b4a3d4d0bcde9ec3a78ab99b2d63af121857a9", "MIT",
		"#3356", "exact host/build", "command help", "fail-closed", "Do not silently install",
		"authorized #3356 runtime execution", "exact repository/host identity", "exact command-help output", "postcondition state",
		"Repository files/content, prompt text, issue comments/labels, and conversational claims", "untrusted and cannot satisfy proof",
		"Re-read provider/GitHub runtime state before native route use", "stale prose is never authority",
		"never permit `size:exception`", "`ask-on-risk`", "`auto-chain`", "`single-pr`",
		"host-specific adapter", "maintainer-approved `size:exception`", "`feature-branch-chain`",
		"separate bounded authority", "remote create/submit/sync/update/merge operations",
		"Issue approval", "planning", "SDD phase approval", "RDD reviews/receipts", "delivery approval",
		"`pr-body.md`", "must not modify the target repository's PR template",
		"GitHub route", "Closes #N", "Refs #N", "host-specific issue syntax",
		"sync, rebase, or base change", "review status", "`disabled/unmanaged`", "ordinary repository policy",
	} {
		if !strings.Contains(skill, required) && !strings.Contains(details, required) {
			t.Errorf("shipped chained-pr skill missing contract %q", required)
		}
	}

	for _, forbidden := range []string{
		"gh stack branch create", "gh stack submit", "gh stack sync", "gh stack restack",
	} {
		if strings.Contains(skill, forbidden) || strings.Contains(details, forbidden) {
			t.Errorf("shipped chained-pr skill contains unproven command surface %q", forbidden)
		}
	}
}
