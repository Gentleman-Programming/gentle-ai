package legacyassets

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The SDD release set closed with v4.0.0, so regenerating from the full local
// tag set must reproduce the committed registry exactly. Shallow clones skip.
func TestReleasedSDDAgentDigestsMatchGenerator(t *testing.T) {
	if testing.Short() {
		t.Skip("regenerates from every release tag")
	}
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s unavailable: %v", tool, err)
		}
	}
	for _, anchor := range []string{"v1.10.0", "v3.7.0", "v4.0.0"} {
		if exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/"+anchor+"^{commit}").Run() != nil {
			t.Skipf("release tag %s unavailable; run git fetch --tags", anchor)
		}
	}
	out := filepath.Join(t.TempDir(), "sdd_agent_digests.go")
	cmd := exec.Command("go", "run", "../../../scripts/gen-sdd-agent-digests", out)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generator failed: %v\n%s", err, output)
	}
	want, err := os.ReadFile("sdd_agent_digests.go")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("sdd_agent_digests.go is stale; run go generate ./internal/components/legacyassets/")
	}
}
