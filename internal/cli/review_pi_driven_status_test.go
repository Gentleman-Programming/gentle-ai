package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	runtimeopencode "github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

// gentle-pi issues some bound STATUS calls without --agent. Under its relay
// handshake that STATUS is Pi-driven, so it must render exactly what
// `STATUS --agent pi` renders -- the rctx2 relay inputs the Pi decoder reads --
// whatever runtime the lineage froze at START. Without the handshake the
// lineage's frozen runtime still decides (#4808).

func stubOpenCodeReviewRuntimeVersion(t *testing.T, version, declaration string) {
	t.Helper()
	t.Setenv(openCodeRelayContractEnvironment, declaration)
	old := runtimeopencode.VersionRunnerOverride
	t.Cleanup(func() { runtimeopencode.VersionRunnerOverride = old })
	freshOpenCodeRuntimeProbe(t)
	runtimeopencode.VersionRunnerOverride = func(context.Context, runtimeopencode.Command) (runtimeopencode.CommandOutput, error) {
		return runtimeopencode.CommandOutput{Stdout: []byte(version)}, nil
	}
}

func TestStatusWithoutAgentOnAnOpenCodeLineageRendersForTheDrivingRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	for _, host := range []struct {
		name, version, declaration string
	}{
		{name: "OpenCode V1", version: "1.18.30"},
		{name: "OpenCode V2", version: "2.0.23", declaration: openCodeRelayContractV2},
	} {
		t.Run(host.name+"/Pi relay handshake", func(t *testing.T) {
			stubOpenCodeReviewRuntimeVersion(t, host.version, host.declaration)
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			reviewEnabledHome(t)
			repo, started, _, _ := openCodeSealedLineage(t, "pi-driven-agentless")
			_, declaredJSON := frozenRuntimeStatusTransition(t, repo, started.LineageID, "--agent", string(model.AgentPi))
			_, agentlessJSON := frozenRuntimeStatusTransition(t, repo, started.LineageID)
			if !bytes.Equal(agentlessJSON, declaredJSON) {
				t.Fatalf("Pi-driven STATUS without --agent diverged from --agent pi\nwithout: %s\nwith:    %s", agentlessJSON, declaredJSON)
			}
			if bytes.Contains(agentlessJSON, []byte("rctx3_")) || bytes.Contains(agentlessJSON, []byte(`"provider_task"`)) {
				t.Fatalf("Pi-driven STATUS received the OpenCode format: %s", agentlessJSON)
			}
		})
		t.Run(host.name+"/no handshake", func(t *testing.T) {
			stubOpenCodeReviewRuntimeVersion(t, host.version, host.declaration)
			t.Setenv(reviewPiHostRelayContractEnvironment, "")
			reviewEnabledHome(t)
			repo, started, _, _ := openCodeSealedLineage(t, "opencode-agentless")
			declared, declaredJSON := frozenRuntimeStatusTransition(t, repo, started.LineageID, "--agent", string(model.AgentOpenCode))
			_, agentlessJSON := frozenRuntimeStatusTransition(t, repo, started.LineageID)
			if !bytes.Equal(agentlessJSON, declaredJSON) {
				t.Fatalf("STATUS without --agent diverged from the frozen OpenCode runtime\nwithout: %s\nwith:    %s", agentlessJSON, declaredJSON)
			}
			if declared.Collect == nil || len(declared.Collect.Inputs) == 0 || declared.Collect.Inputs[0].ProviderTask == nil {
				t.Fatalf("OpenCode STATUS = %s, want sealed provider tasks", declaredJSON)
			}
		})
	}
}

// A compiled-runtime lineage driven through the Pi relay is Pi-driven too: it
// receives the relay route, never in-process capture tokens bound to a runtime
// binary the Pi host may not have.
func TestStatusWithoutAgentUnderThePiHandshakeRendersForPiOnCompiledLineages(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	for _, runtime := range []model.AgentID{model.AgentClaudeCode, model.AgentCodex} {
		t.Run(string(runtime), func(t *testing.T) {
			t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
			repo, lineage := frozenRuntimeReview(t, runtime, false)
			_, declaredJSON := frozenRuntimeStatusTransition(t, repo, lineage, "--agent", string(model.AgentPi))
			_, agentlessJSON := frozenRuntimeStatusTransition(t, repo, lineage)
			if !bytes.Equal(agentlessJSON, declaredJSON) {
				t.Fatalf("Pi-driven STATUS without --agent diverged from --agent pi\nwithout: %s\nwith:    %s", agentlessJSON, declaredJSON)
			}
		})
	}
}

// The handshake never invents a runtime for a lineage that froze none: the
// manual route stays manual.
func TestStatusWithoutAgentUnderThePiHandshakeKeepsAnUnfrozenLineageManual(t *testing.T) {
	if testing.Short() {
		t.Skip("requires git fixtures")
	}
	t.Setenv(reviewPiHostRelayContractEnvironment, reviewPiHostRelayContract)
	repo, lineage := frozenRuntimeReview(t, "", false)
	_, manualJSON := frozenRuntimeStatusTransition(t, repo, lineage)
	for _, token := range []string{`"name":"agent"`, `"provider_task"`} {
		if bytes.Contains(manualJSON, []byte(token)) {
			t.Fatalf("unfrozen STATUS under the Pi handshake invented %s: %s", token, manualJSON)
		}
	}
}
