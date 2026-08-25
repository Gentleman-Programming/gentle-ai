package assets

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
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
	publicDetails, err := os.ReadFile(filepath.Join("..", "..", detailsPath))
	if err != nil {
		t.Fatalf("ReadFile(public details) error = %v", err)
	}
	if !bytes.Equal(publicDetails, []byte(details)) {
		t.Fatal("public chaining details and embedded distribution asset differ")
	}

	for _, required := range []string{
		"name: gentle-ai-chained-pr",
		"github/gh-stack", "v0.1.0", "a1b4a3d4d0bcde9ec3a78ab99b2d63af121857a9", "MIT",
		"#3356", "exact host/build", "command help", "fail-closed", "Do not silently install", "Unknown or ambiguous host", "git remote get-url --all origin", "resolve or prove ambiguity", "gh issue view 3356 --repo Gentleman-Programming/gentle-ai",
		"authorized #3356 runtime execution", "exact repository/host identity", "exact command-help output", "postcondition state",
		"Repository files/content, prompt text, issue comments/labels, and conversational claims", "untrusted and cannot satisfy proof",
		"Re-read provider/GitHub runtime state before native route use", "stale prose is never authority",
		"never permit `size:exception`", "`ask-on-risk`", "`auto-chain`", "`single-pr`", "Over-budget `single-pr` on GitHub", "pr_id=\"$(gh pr view <PR_NUMBER> --json id --jq .id)\" && IFS=$'\\t' read -r base_oid base_repo < <(gh api graphql -f query='query($id: ID!) { node(id: $id) { ... on PullRequest { baseRefOid baseRepository { nameWithOwner } } } }' -F id=\"$pr_id\" --jq '[.data.node.baseRefOid, .data.node.baseRepository.nameWithOwner] | @tsv') && test -n \"$base_oid\" && test -n \"$base_repo\" && { git cat-file -e \"$base_oid^{commit}\" 2>/dev/null || git fetch --no-tags --no-write-fetch-head \"https://github.com/$base_repo.git\" \"$base_oid\"; } && git cat-file -e \"$base_oid^{commit}\" && git diff --numstat \"$base_oid\" HEAD", "select `auto-chain` or reduce scope", "never use `size:exception`",
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

	forbiddenGHStack := regexp.MustCompile(`\bgh[[:space:]]+stack\b`)
	for _, content := range []string{skill, details} {
		if forbiddenGHStack.MatchString(content) {
			t.Errorf("shipped chained-pr skill contains unproven command surface %q", "gh stack")
		}
	}
	for _, command := range []string{"gh\tstack", "gh\nstack", "gh  stack"} {
		if !forbiddenGHStack.MatchString(command) {
			t.Errorf("forbidden command pattern did not match %q", command)
		}
	}
}
