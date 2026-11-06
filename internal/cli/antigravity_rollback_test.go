package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/antigravityrules"
	"github.com/gentleman-programming/gentle-ai/v4/internal/pipeline"
)

type antigravityTestStep struct {
	id  string
	run func() error
}

func (s antigravityTestStep) ID() string { return s.id }
func (s antigravityTestStep) Run() error { return s.run() }

// TestRunInstallAntigravityRollbackRemovesNewRulesAndRestoresPriorOnes makes
// post-apply verification fail after the publisher ran: rollback must remove
// every rule this run created and restore the previous ones byte for byte.
func TestRunInstallAntigravityRollbackRemovesNewRulesAndRestoresPriorOnes(t *testing.T) {
	for _, scope := range []string{"global", "workspace"} {
		t.Run(scope, func(t *testing.T) {
			home := stubAntigravityHome(t, false)
			base := home
			args := []string{"--agent", "antigravity", "--component", "persona", "--component", "engram", "--component", "context7"}
			if scope == "workspace" {
				base = t.TempDir()
				t.Chdir(base)
				args = append(args, "--scope", "workspace")
			}
			rules := antigravityrules.RulesDir(base)
			prior := map[string]string{
				filepath.Join(rules, "gentle-ai-01-persona.md"): "---\ntrigger: always_on\n---\n\nprevious persona\n",
				filepath.Join(rules, "gentle-ai-07-retired.md"): "---\ntrigger: always_on\n---\n\nretired\n",
				filepath.Join(rules, "team.md"):                 "---\ntrigger: always_on\n---\n\nteam rule\n",
			}
			for path, content := range prior {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			previous := installStagePlan
			t.Cleanup(func() { installStagePlan = previous })
			published := false
			installStagePlan = func(rt *installRuntime) pipeline.StagePlan {
				plan := previous(rt)
				// Break a verified file only after the publisher wrote rules.
				plan.Apply = append(plan.Apply, antigravityTestStep{id: "test:break-staging", run: func() error {
					owned, _ := antigravityrules.OwnedRulePaths(base)
					published = len(owned) > 2
					return os.Remove(antigravityrules.StagingPath(base))
				}})
				return plan
			}

			_, err := RunInstall(args, macOSDetectionResult())
			if err == nil || !strings.Contains(err.Error(), "post-apply verification failed") {
				t.Fatalf("RunInstall() error = %v, want a post-apply verification failure", err)
			}
			if !published {
				t.Fatal("the publisher did not write new rules before verification failed")
			}
			entries, err := os.ReadDir(rules)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != len(prior) {
				var names []string
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				t.Errorf("rules after rollback = %v, want only the %d prior files", names, len(prior))
			}
			for path, content := range prior {
				if got := readString(t, path); got != content {
					t.Errorf("%s = %q after rollback, want %q", path, got, content)
				}
			}
		})
	}
}
