package agentguidance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// swapPackageReviewContractSource replaces the package-level fallback for one
// test and restores it on cleanup. Callers must not be parallel.
func swapPackageReviewContractSource(t *testing.T, source ReviewContractSource) {
	t.Helper()
	reviewContractMu.RLock()
	original := reviewContractSource
	reviewContractMu.RUnlock()
	t.Cleanup(func() { SetReviewContractSource(original) })
	SetReviewContractSource(source)
}

func fixedReviewContract(text string) ReviewContractSource {
	return func(agent model.AgentID) (string, error) {
		return text + " for " + string(agent), nil
	}
}

// routingTargetFor resolves the single file routing injection writes for agent
// under home, seeding it with seed when seed is not empty.
func routingTargetFor(t *testing.T, home string, agent model.AgentID, seed string) string {
	t.Helper()
	paths, err := RoutingPathsWithOptions(home, agent, RoutingOptions{})
	if err != nil || len(paths) != 1 {
		t.Fatalf("RoutingPathsWithOptions(%q) = %v, %v", agent, paths, err)
	}
	if seed != "" {
		if err := os.MkdirAll(filepath.Dir(paths[0]), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths[0], []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return paths[0]
}

// Not parallel: it swaps the package-level contract source.
func TestInjectRoutingUsesReviewContractOption(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fallback ReviewContractSource
	}{
		{"without a package-level source", nil},
		{"over the package-level source", fixedReviewContract("PACKAGE REVIEW CONTRACT")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			swapPackageReviewContractSource(t, tc.fallback)
			for _, agent := range []model.AgentID{model.AgentClaudeCode, model.AgentOpenCode} {
				home := t.TempDir()
				path := routingTargetFor(t, home, agent, "")
				options := RoutingOptions{ReviewContract: fixedReviewContract("OPTION REVIEW CONTRACT")}
				if _, err := InjectRoutingWithOptions(home, agent, options); err != nil {
					t.Fatalf("InjectRoutingWithOptions(%q) error = %v", agent, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(raw), "OPTION REVIEW CONTRACT for "+string(agent)) {
					t.Errorf("%s: injected guidance lacks the option contract:\n%s", agent, raw)
				}
				if strings.Contains(string(raw), "PACKAGE REVIEW CONTRACT") {
					t.Errorf("%s: package-level source won over RoutingOptions.ReviewContract", agent)
				}
			}
		})
	}
}

// Not parallel: it swaps the package-level contract source.
func TestInjectRoutingWithoutReviewContractSourceFailsClosedAndWritesNothing(t *testing.T) {
	swapPackageReviewContractSource(t, nil)

	for _, tc := range []struct {
		agent model.AgentID
		seed  string
	}{
		{model.AgentClaudeCode, "# My rules\n\nKeep this.\n"},
		{model.AgentOpenCode, "{\n  \"agent\": {\"mine\": {\"prompt\": \"keep\"}}\n}\n"},
	} {
		t.Run(string(tc.agent), func(t *testing.T) {
			home := t.TempDir()
			path := routingTargetFor(t, home, tc.agent, tc.seed)

			_, err := InjectRoutingWithOptions(home, tc.agent, RoutingOptions{})
			if !errors.Is(err, ErrMissingReviewContract) {
				t.Fatalf("InjectRoutingWithOptions(%q) error = %v, want ErrMissingReviewContract", tc.agent, err)
			}
			for _, want := range []string{string(tc.agent), "review contract source was not wired", "RoutingOptions.ReviewContract"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != tc.seed {
				t.Fatalf("failed injection changed %s:\n%s", path, got)
			}
		})
	}

	// A runtime without receipt-driven development needs no contract source.
	if _, err := InjectRoutingWithOptions(t.TempDir(), model.AgentGeminiCLI, RoutingOptions{}); err != nil {
		t.Fatalf("InjectRoutingWithOptions(gemini) error = %v; an ODD-only runtime needs no contract", err)
	}
}
