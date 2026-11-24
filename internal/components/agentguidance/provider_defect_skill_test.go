package agentguidance_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v5/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/agentguidance"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/reviewassets"
	"github.com/gentleman-programming/gentle-ai/v5/internal/components/skills"
	"github.com/gentleman-programming/gentle-ai/v5/internal/model"
)

// harness-continuity S9: the provider-defect report workflow is loaded only
// when used. Every orchestrator keeps the classification stub; the complete
// consent, privacy, duplicate, uncertainty, and resume workflow lives once in
// the shared reference the skills component installs. Shipped-policy and
// install checks, not model-compliance proof.
const (
	providerDefectHeading  = "#### Gentle AI Provider Defect Handoff (MANDATORY)"
	providerDefectAsset    = "skills/_shared/provider-defect-report.md"
	providerDefectFallback = "If that file is not installed, file no report, open no consent prompt, and run no GitHub operation; tell the user the defect-report skill is not installed and keep the evidence."
	providerDefectPointer  = "When a Gentle AI invocation produced it, read `_shared/provider-defect-report.md` from the runtime's installed skills directory and follow it before any report, consent prompt, or GitHub operation."
)

var providerDefectStub = []string{
	"Before losslessly relaying any blocking choice envelope, classify its semantic admissibility.",
	"**The test is what produced the failure, not what the work was doing when it happened.**",
	"When anything other than a Gentle AI invocation (its non-zero exit, typed envelope, refusal, or documented contract refusing) produced it, there is no report and no handoff: do not name the component you believe is responsible, suggest where else to file it, or ask; say plainly what blocked the work, then continue or stop as the workflow dictates.",
	providerDefectPointer,
	// Minimal and custom presets install no skills component: the stub must
	// fail safe instead of improvising the workflow it cannot read.
	providerDefectFallback,
}

// providerDefectCarriers renders every non-Pi orchestrator in both capability
// variants and returns the agents whose render carries the handoff heading.
func providerDefectCarriers(t *testing.T) []model.AgentID {
	t.Helper()
	var carriers []model.AgentID
	for _, agent := range catalog.AllAgents() {
		if agent.ID == model.AgentPi || agent.ID == model.AgentConductor {
			continue
		}
		for _, capability := range []string{"", "small"} {
			rendered, err := agentguidance.RenderOrchestratorWithSource(agent.ID, reviewassets.ReviewExecutionContractFor, capability)
			if err != nil {
				t.Fatalf("RenderOrchestratorWithSource(%q, %q) error = %v", agent.ID, capability, err)
			}
			if !strings.Contains(rendered, providerDefectHeading) {
				continue
			}
			for _, want := range providerDefectStub {
				if !strings.Contains(rendered, want) {
					t.Errorf("%s/%q provider-defect stub is missing %q", agent.ID, capability, want)
				}
			}
			// The workflow body is loaded on demand, never restated inline.
			for _, body := range []string{"report_and_continue", "complete a definitive lookup", "final privacy scan", "declined_this_candidate"} {
				if strings.Contains(rendered, body) {
					t.Errorf("%s/%q orchestrator still carries the provider-defect workflow body %q", agent.ID, capability, body)
				}
			}
			if !slices.Contains(carriers, agent.ID) {
				carriers = append(carriers, agent.ID)
			}
		}
	}
	return carriers
}

func TestRenderedOrchestratorsKeepOnlyTheProviderDefectStub(t *testing.T) {
	t.Parallel()

	carriers := providerDefectCarriers(t)
	// The handoff belongs to the review lifecycle: exactly the non-Pi runtimes
	// with receipt-driven development carry it.
	var want []model.AgentID
	for _, agent := range catalog.AllAgents() {
		if agent.ID != model.AgentPi && model.SupportsReceiptDrivenDevelopment(agent.ID) {
			want = append(want, agent.ID)
		}
	}
	slices.Sort(carriers)
	slices.Sort(want)
	if !slices.Equal(carriers, want) {
		t.Fatalf("provider-defect stub carriers = %v, want the RDD runtimes %v", carriers, want)
	}
}

// TestDefaultInstallShipsTheProviderDefectReportWhereTheStubShips installs the
// orchestrator and the default (full-gentleman) skills selection into one home
// and proves the stub's target exists, byte-identical to the embedded source.
func TestDefaultInstallShipsTheProviderDefectReportWhereTheStubShips(t *testing.T) {
	if !slices.Contains(model.ComponentsForPreset(model.PresetFullGentleman, model.PersonaGentleman), model.ComponentSkills) {
		t.Fatal("the default preset no longer installs the skills component")
	}
	want, err := fs.ReadFile(assets.FS, providerDefectAsset)
	if err != nil {
		t.Fatalf("embedded provider-defect report: %v", err)
	}
	carriers := providerDefectCarriers(t)
	if len(carriers) == 0 {
		t.Fatal("no orchestrator carries the provider-defect stub; the install check would be vacuous")
	}
	for _, agent := range carriers {
		t.Run(string(agent), func(t *testing.T) {
			home := t.TempDir()
			result, err := agentguidance.InjectRoutingWithOptions(home, agent, agentguidance.RoutingOptions{ReviewContract: reviewassets.ReviewExecutionContractFor})
			if err != nil {
				t.Fatalf("InjectRoutingWithOptions(%q) error = %v", agent, err)
			}
			guidance, err := os.ReadFile(result.Files[0])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(guidance, []byte("`_shared/provider-defect-report.md` from the runtime's installed skills directory")) {
				t.Fatalf("installed %s guidance does not point at the shared provider-defect report", agent)
			}
			adapter, err := agents.NewAdapter(agent)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := skills.Inject(home, adapter, skills.SkillsForPreset(model.PresetFullGentleman)); err != nil {
				t.Fatalf("skills.Inject(%q) error = %v", agent, err)
			}
			got, err := os.ReadFile(filepath.Join(adapter.SkillsDir(home), "_shared", "provider-defect-report.md"))
			if err != nil {
				t.Fatalf("default install of %s lacks the provider-defect report: %v", agent, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("installed %s provider-defect report differs from the embedded source", agent)
			}
		})
	}
}
