package agentguidance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

// The routing goldens pin the exact bytes RenderRouting emits per runtime, so a
// runtime-specific branch cannot move another runtime's guidance unnoticed. Pi
// also keeps its full text: gentle-ai is the single source of Pi's ODD steps,
// which Gentle Shell consumes through the provider contract bundle.
//
// Regenerate after an intended canon change with
//
//	go test ./internal/components/agentguidance -run '^TestRenderRoutingMatchesGoldens$' -update
const (
	routingDigestsFile = "testdata/routing-sha256.json"
	routingPiFile      = "testdata/routing-pi.md"
)

func TestRenderRoutingMatchesGoldens(t *testing.T) {
	digests := map[string]string{}
	var pi string
	for _, agent := range catalog.AllAgents() {
		rendered, err := agentguidance.RenderRouting(agent.ID)
		if err != nil {
			t.Fatalf("RenderRouting(%q) error = %v", agent.ID, err)
		}
		sum := sha256.Sum256([]byte(rendered))
		digests[string(agent.ID)] = hex.EncodeToString(sum[:])
		if agent.ID == model.AgentPi {
			pi = rendered
		}
	}
	encoded, err := json.MarshalIndent(digests, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')

	if *update {
		if err := os.MkdirAll(filepath.Dir(routingDigestsFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routingDigestsFile, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routingPiFile, []byte(pi), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	current, err := os.ReadFile(routingDigestsFile)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update to create it)", routingDigestsFile, err)
	}
	var want map[string]string
	if err := json.Unmarshal(current, &want); err != nil {
		t.Fatalf("decode %s: %v", routingDigestsFile, err)
	}
	if len(want) != len(digests) {
		t.Errorf("%s pins %d runtimes, RenderRouting covers %d", routingDigestsFile, len(want), len(digests))
	}
	for agent, got := range digests {
		if want[agent] != got {
			t.Errorf("RenderRouting(%q) sha256 = %s, golden %s", agent, got, want[agent])
		}
	}

	wantPi, err := os.ReadFile(routingPiFile)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update to create it)", routingPiFile, err)
	}
	if pi != string(wantPi) {
		gotLines, wantLines := strings.Split(pi, "\n"), strings.Split(string(wantPi), "\n")
		for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
			var got, want string
			if i < len(gotLines) {
				got = gotLines[i]
			}
			if i < len(wantLines) {
				want = wantLines[i]
			}
			if got != want {
				t.Fatalf("RenderRouting(pi) drifted from %s at line %d:\n got: %q\nwant: %q", routingPiFile, i+1, got, want)
			}
		}
	}
}
