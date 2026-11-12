package opencodeagents_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/opencodeagents"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// S2: resolve the reference from its installed inline-settings carrier, not the source asset directory.
func TestPublicOpenCodeMethodReference(t *testing.T) {
	home := t.TempDir()
	adapter, err := agents.NewAdapter(model.AgentOpenCode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skills.Inject(home, adapter, nil); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(adapter.SkillsDir(home), "_shared", "verify-method.md")
	want, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range opencodeagents.Parity(model.AgentOpenCode) {
		if spec.Name != "gentle-ai-verify" {
			continue
		}
		entry, err := opencodeagents.Entry(spec)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(entry["permission"], map[string]any{"write": "deny", "edit": "deny", "task": "deny"}) {
			t.Fatal("verifier permission boundary changed")
		}
		prompt := entry["prompt"].(string)
		for _, clause := range []string{"Read-only means no edits to the repository or its git state", "Do not delegate to child agents, commit, or push.", "RDD review remains independent and parent-owned."} {
			if !strings.Contains(prompt, clause) {
				t.Fatalf("permission envelope lost %q", clause)
			}
		}
		match := regexp.MustCompile(`\[the shared verification method\]\(([^)]+)\)`).FindStringSubmatch(prompt)
		if match == nil {
			t.Fatal("mandatory installed method reference missing")
		}
		link := match[1]
		if strings.HasPrefix(link, "~/") {
			link = filepath.Join(home, strings.TrimPrefix(link, "~/"))
		}
		resolved := link
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Clean(filepath.Join(filepath.Dir(adapter.SettingsPath(home)), resolved))
		}
		got, err := os.ReadFile(resolved)
		if err != nil {
			t.Fatalf("inline-settings method reference %q resolves to %q, not installed %q (file-agent directory=%q): %v", match[1], resolved, installed, adapter.SubAgentsDir(home), err)
		}
		if string(got) != string(want) {
			t.Fatal("reference does not resolve to installed canonical method")
		}
		return
	}
	t.Fatal("existing OpenCode verifier role missing")
}
