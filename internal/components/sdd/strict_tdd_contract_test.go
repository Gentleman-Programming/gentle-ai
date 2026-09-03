package sdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func readInjectedStrictTDDAsset(t *testing.T, home, phase, name string) string {
	t.Helper()

	path := filepath.Join(home, ".config", "opencode", "skills", phase, name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}

func assertContractClauses(t *testing.T, artifact string, clauses []string) {
	t.Helper()

	for _, clause := range clauses {
		if !strings.Contains(artifact, clause) {
			t.Errorf("generated Strict TDD artifact missing contract clause %q", clause)
		}
	}
}

func TestInjectStrictTDDBehavioralFalsificationContract(t *testing.T) {
	mockNoPackageManager(t)
	home := t.TempDir()

	if _, err := Inject(home, opencodeAdapter(), model.SDDModeSingle, InjectOptions{Capability: "capable"}); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}

	apply := readInjectedStrictTDDAsset(t, home, "sdd-apply", "strict-tdd.md")
	verify := readInjectedStrictTDDAsset(t, home, "sdd-verify", "strict-tdd-verify.md")
	applySkill := readInjectedStrictTDDAsset(t, home, "sdd-apply", "SKILL.md")
	verifySkill := readInjectedStrictTDDAsset(t, home, "sdd-verify", "SKILL.md")

	assertContractClauses(t, applySkill, []string{
		"Behavioral Falsification Evidence Contract",
		"strict-tdd.md",
		"executed behavioral RED",
	})
	assertContractClauses(t, verifySkill, []string{
		"Behavioral Falsification Verification Contract",
		"strict-tdd-verify.md",
		"structural RED",
	})

	assertContractClauses(t, apply, []string{
		"## Behavioral Falsification Evidence Contract",
		"implementation-independent oracle",
		"oracle_source",
		"focused_command",
		"test_identity",
		"production_identity",
		"executed behavioral RED",
		"qualifying RED gate MUST require an executed behavioral failure before GREEN",
		"written-only or structural RED is insufficient",
		"structural RED does not satisfy behavioral RED",
		"pre_green_snapshot",
		"fault",
		"counterfactual_result",
		"residual_risk",
		"verifier_selection: pending",
		"verifier_execution: pending",
		"initial_residual_risk",
		"verifier alone records final `fault`, `counterfactual_result`, and residual-risk decision",
		"retained binary diff",
		"retained untracked-file bytes",
		"full-worktree path/mode/SHA-256 index",
		"renewed RED",
		"material test changes",
		"semantic partitions",
		"anti_fake_it.applicable",
		"anti_fake_it.simplest_rejected_implementation",
		"anti_fake_it.discriminating_tests",
		"anti_fake_it.decision",
		"primary fault MUST target",
		"canonical candidate manifest",
		"candidate_root",
		"exact disposable detached Git worktree",
		"identity predicate",
		"cleanup failure",
		"two candidate runs",
		"two counterfactual runs",
		"no additional rerun loop",
		"verifier-owned fault selection",
		"one replacement only",
		"protected_test_or_behavior",
		"post_fault_production_root",
		"residual risk",
	})

	assertContractClauses(t, verify, []string{
		"## Behavioral Falsification Verification Contract",
		"executed behavioral RED",
		"structural RED does not satisfy behavioral RED",
		"oracle_source",
		"focused_command",
		"test_identity",
		"production_identity",
		"pre_green_snapshot",
		"fault",
		"counterfactual_result",
		"residual_risk",
		"renewed RED",
		"material test changes",
		"semantic partitions",
		"anti_fake_it.applicable",
		"anti_fake_it.simplest_rejected_implementation",
		"anti_fake_it.discriminating_tests",
		"anti_fake_it.decision",
		"primary fault MUST target",
		"candidate_root",
		"exact disposable detached Git worktree",
		"two candidate runs",
		"two counterfactual runs",
		"no additional rerun loop",
		"verifier-owned fault selection",
		"one replacement only",
		"killed",
		"survived",
		"equivalent",
		"invalid",
		"unavailable",
		"not-applicable",
		"protected_test_or_behavior",
		"expected_failure_class",
		"observed_failure_class",
		"observed_output_excerpt",
		"post_fault_production_root",
		"blocking",
		"degradation",
		"residual risk",
		"#262",
		"#986",
		"#1263",
		"does not claim native historical proof or universal enforcement",
	})
}
