package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRunSyncPiPreservesUserSubagentRouting protects the sync contract associated
// with issue #4946. It does not reproduce gentle-pi's profile application logic.
func TestRunSyncPiPreservesUserSubagentRouting(t *testing.T) {
	home := setupPiSyncHost(t)
	t.Setenv("GENTLE_PI_CONFIG_HOME", "")
	agentDir := filepath.Join(home, ".pi", "agent")
	fixtures := []struct {
		path string
		body string
	}{
		{
			path: filepath.Join(agentDir, "subagents.json"),
			body: `{
  "model_profiles": {
    "gentle-ai-worker": {"model": "openai/user-worker", "effort": "high"},
    "review-refuter": {"model": "anthropic/user-refuter", "effort": "medium"},
    "custom-helper": {"model": "user-provider/user-model", "effort": "low"}
  },
  "max_concurrency": 3,
  "timeout_ms": 60000,
  "user_setting": {"keep": true}
}
`,
		},
		{
			path: filepath.Join(agentDir, "agents", "gentle-ai-worker.md"),
			body: "---\nname: gentle-ai-worker\ndescription: User-configured worker\nmodel: openai/user-worker\nthinking: high\n---\nKeep the user's worker instructions.\n",
		},
		{
			path: filepath.Join(agentDir, "agents", "review-refuter.md"),
			body: "---\nname: review-refuter\ndescription: User-configured refuter\nmodel: anthropic/user-refuter\nthinking: medium\n---\nKeep the user's refuter instructions.\n",
		},
		{
			path: filepath.Join(agentDir, "agents", "custom-helper.md"),
			body: "---\nname: custom-helper\ndescription: User-owned helper\nmodel: user-provider/user-model\nthinking: low\n---\nKeep the user's custom instructions.\n",
		},
	}
	for _, fixture := range fixtures {
		mustWriteFile(t, fixture.path, []byte(fixture.body))
	}

	result, err := RunSync([]string{"--agent", "pi", "--scope", "global"})
	if err != nil {
		t.Fatalf("RunSync() error = %v", err)
	}
	if !result.Verify.Ready || result.NoOp || result.FilesChanged == 0 {
		t.Fatalf("sync did not execute verified managed writes: %#v", result)
	}
	for _, fixture := range fixtures {
		got, err := os.ReadFile(fixture.path)
		if err != nil {
			t.Fatalf("read preserved file %q: %v", fixture.path, err)
		}
		if !bytes.Equal(got, []byte(fixture.body)) {
			t.Errorf("sync changed user-owned file %q:\n got %s\nwant %s", fixture.path, got, fixture.body)
		}
		if containsPath(result.ChangedFiles, fixture.path) {
			t.Errorf("sync reported user-owned file %q as changed", fixture.path)
		}
	}
}
