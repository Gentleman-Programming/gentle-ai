package assets

import (
	"strings"
	"testing"
)

// This proves shipped policy text, not live model compliance or context telemetry.
func TestOrchestratorsCarryEvidenceBudget(t *testing.T) {
	paths := append(allODDOrchestratorAssetPaths(t), "skills/_shared/odd-orchestrator-sections.md")
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			content := MustRead(path)
			if path == "antigravity/orchestrator.md" {
				// Antigravity references this canonical section instead of
				// repeating its exploration policy. Renderer tests separately
				// prove that the resolved clauses remain always-on.
				const section = "Orchestrator Routing and Delivery"
				const reference = "{{GENTLE_AI_ODD_SECTION:" + section + "}}"
				if !strings.Contains(content, reference) {
					t.Fatal("missing canonical routing section reference")
				}
				shared := MustRead("skills/_shared/odd-orchestrator-sections.md")
				_, rest, startFound := strings.Cut(shared, "<!-- odd-orchestrator-section:"+section+":start -->")
				body, _, endFound := strings.Cut(rest, "<!-- odd-orchestrator-section:"+section+":end -->")
				if !startFound || !endFound {
					t.Fatal("missing canonical routing section body")
				}
				content = strings.ReplaceAll(content, reference, body)
			}
			for _, want := range []string{
				"one parallel batch", "at most 3 calls", "approximately 10k tokens",
				"bounded search/line ranges", "whole large files",
				"more than approximately 5 sequential lookups", "re-evaluate task size",
				"one read-only explorer", "approximately 2k tokens", "path:line",
				"one parent spot check", "Do not reread the entire mapped evidence",
				"counts, --stat, tail, or summaries", "delegate long suites and builds",
				"approximately 150k", "advisory", "not mechanically observed or enforced",
			} {
				if !strings.Contains(content, want) {
					t.Errorf("missing evidence-budget clause %q", want)
				}
			}
			for _, stale := range []string{
				"1–3 files", "4+ files", "4-file rule", "up to 3 files inline",
				"20 tool calls", "5 exploratory reads", "2 non-mechanical edits",
			} {
				if strings.Contains(content, stale) {
					t.Errorf("retains stale routing trigger %q", stale)
				}
			}
		})
	}
}
