package sddstatus

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredLegacyBindingFixturesAreAbsent(t *testing.T) {
	repoRoot := retiredLegacyFixtureRepositoryRoot(t)

	for _, fixture := range []string{
		"e2e/organicruntime/organic_lifecycle_hardening_test.go",
		"e2e/organicruntime/organic_escalated_continuation_test.go",
		"e2e/organicruntime/organic_recovery_guard_rails_test.go",
		"internal/sddstatus/bounded_review_test.go",
		"internal/sddstatus/legacy_binding_read_test.go",
		"internal/sddstatus/review_binding_ledger_test.go",
		"internal/sddstatus/runtime_ledger_interrupted_legacy_test.go",
		"internal/sddstatus/runtime_ledger_remediation_test.go",
		"internal/sddstatus/runtime_ledger_self_remediation_test.go",
		"internal/sddstatus/runtime_review_acts_after_verify_test.go",
	} {
		requireRetiredLegacyFixtureAbsent(t, repoRoot, fixture)
	}
}

func TestRetiredLegacyBindingConsumersAreAbsent(t *testing.T) {
	repoRoot := retiredLegacyFixtureRepositoryRoot(t)

	for _, consumer := range []struct {
		fixture  string
		testName string
	}{
		{"internal/sddstatus/runtime_status_remediation_advice_test.go", "TestActiveAttemptGuidanceUsesOnlyOpaqueCompactContinuation"},
		{"internal/sddstatus/runtime_compact_test.go", "TestCompactSettleReviewDisabledClosesOrdinaryWithoutAdvancingBinding"},
		{"internal/sddstatus/runtime_compact_test.go", "TestCompactSettleTokenIgnoresBindingCAS"},
		{"internal/sddstatus/runtime_status_test.go", "TestResolveRoutesAtomicRuntimeRemediationSuccessorToFreshVerify"},
		{"internal/sddstatus/runtime_status_test.go", "TestResolveRoutesPureEngramRuntimeRemediationSuccessorToFreshVerify"},
		{"internal/sddstatus/runtime_status_test.go", "TestResolveDoesNotBypassMalformedFailedEvidenceWithACompletedRuntime"},
		{"internal/sddstatus/runtime_ledger_review_disabled_test.go", "TestRuntimeFinishDoesNotDemandAReviewSuccessorWhileReviewIsDisabled"},
		{"internal/sddstatus/runtime_ledger_review_disabled_test.go", "TestRuntimeFinishIgnoresReviewModeAndBindingMetadata"},
		{"internal/sddstatus/runtime_compact_sentinel_test.go", "TestCompactSettleRemediationRefusalIsClassifiedNotAuthorityFailure"},
	} {
		path := filepath.Join(repoRoot, consumer.fixture)
		contents, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				t.Errorf("retired legacy binding consumer fixture %q is missing", consumer.fixture)
			} else {
				t.Errorf("read retired legacy binding consumer fixture %q: %v", consumer.fixture, err)
			}
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, contents, 0)
		if err != nil {
			t.Errorf("parse retired legacy binding consumer fixture %q: %v", consumer.fixture, err)
			continue
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name.Name == consumer.testName {
				t.Errorf("retired legacy binding consumer %s remains in %q", consumer.testName, consumer.fixture)
			}
		}
	}
}

func retiredLegacyFixtureRepositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get test working directory: %v", err)
	}
	for {
		info, err := os.Stat(filepath.Join(dir, "go.mod"))
		switch {
		case err == nil && !info.IsDir():
			return dir
		case err == nil:
			t.Fatalf("go.mod under %q is not a file", dir)
		case !errors.Is(err, fs.ErrNotExist):
			t.Fatalf("stat go.mod under %q: %v", dir, err)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("locate repository root from test working directory: go.mod not found")
		}
		dir = parent
	}
}

func requireRetiredLegacyFixtureAbsent(t *testing.T, repoRoot, fixture string) {
	t.Helper()

	path := filepath.Join(repoRoot, fixture)
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("retired legacy fixture %q is still present", fixture)
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lstat retired legacy fixture %q: %v", fixture, err)
	}
}
