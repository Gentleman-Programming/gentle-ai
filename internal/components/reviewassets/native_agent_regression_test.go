package reviewassets

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// TestNativeAgentRenderStaysStableForExistingRuntimes pins the rendered
// native agents and their target directory for runtimes that already shipped
// them, so adding another runtime cannot change their output. Update a digest
// only when that runtime's agent assets or rendering intentionally change.
func TestNativeAgentRenderStaysStableForExistingRuntimes(t *testing.T) {
	t.Parallel()

	home := filepath.FromSlash("/home/user")
	cases := []struct {
		agent      model.AgentID
		dir        string
		wantDigest string
	}{
		{model.AgentClaudeCode, filepath.Join(home, ".claude", "agents"), "sha256:d7a7385460ccefd24c324c3282c376e425e8b9f304b28b47a4b1b209eeefa263"},
		{model.AgentCursor, filepath.Join(home, ".cursor", "agents"), "sha256:42ecb0185e9ae6b89d744c17092c8716030e6246cb8ac14d0e09c1055054ec7f"},
		{model.AgentKiroIDE, filepath.Join(home, ".kiro", "agents"), "sha256:9c77aad65d788632676be8b48c3fa6c00c0ff5984763f1c6b34e1533751d62b9"},
	}
	for _, tc := range cases {
		t.Run(string(tc.agent), func(t *testing.T) {
			t.Parallel()

			adapter, err := agents.NewAdapter(tc.agent)
			if err != nil {
				t.Fatal(err)
			}
			if got := adapter.SubAgentsDir(home); got != tc.dir {
				t.Fatalf("SubAgentsDir() = %q, want %q", got, tc.dir)
			}
			sum := sha256.New()
			for _, name := range NativeAgentManifest[tc.agent] {
				content, err := renderNativeAgent(adapter, name, InstallOptions{})
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintf(sum, "%s\x00%s\x00", name, content)
			}
			if got := fmt.Sprintf("sha256:%x", sum.Sum(nil)); got != tc.wantDigest {
				t.Fatalf("rendered native agents digest = %q, want %q", got, tc.wantDigest)
			}
		})
	}
}
