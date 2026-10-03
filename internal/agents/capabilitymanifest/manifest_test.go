package capabilitymanifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
)

func TestCanonicalImplementationRoutingBoundaries(t *testing.T) {
	t.Parallel()

	got := CanonicalImplementationRouting()
	want := ImplementationRoutingFacts{
		DirectInline: DirectInlineFacts{
			MaxEvidenceBatches:                       1,
			MaxEvidenceCalls:                         3,
			ApproxEvidenceTokens:                     10000,
			EvidenceMustUseBoundedRanges:             true,
			MaxMechanicalWriteFiles:                  1,
			MechanicalWriteMustBeAlreadyUnderstood:   true,
			MechanicalWriteMustNotRequireResearch:    true,
			MechanicalWriteMustNotHaveOpenDesignWork: true,
		},
		DelegatedDirect: DelegatedDirectFacts{
			ApproxSequentialLookupLimit:    5,
			DelegateWhenLongSessionMapping: true,
			ApproxHandoffTokens:            2000,
			MaxParentSpotChecks:            1,
			ApproxParentContextTokens:      150000,
			ContextBackstopIsAdvisory:      true,
			WriterMinNonTrivialFiles:       2,
			DelegateWhenReadPreparesWrite:  true,
			DelegateWhenBroadResearch:      true,
		},
		SDD: SDDProposalFacts{
			ProposeWhenSubstantialOrAmbiguous:     true,
			DurableArtifactsMustReduceUncertainty: true,
			SelectionPolicy:                       SDDSelectionExplicitRequestOrAcceptedProposal,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CanonicalImplementationRouting() = %#v, want %#v", got, want)
	}
}

func TestManifestRejectsWeakenedRoutingFacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		weaken func(*AgentCapabilityManifest)
	}{
		{
			name: "legacy minimum file count is rejected",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DirectInline.MinUnderstandingFiles = 1
			},
		},
		{
			name: "legacy maximum file count is rejected",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DirectInline.MaxUnderstandingFiles = 4
			},
		},
		{
			name: "legacy mapping file count is rejected",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.MappingMinUnderstandingFiles = 5
			},
		},
		{
			name:   "multiple inline batches",
			weaken: func(m *AgentCapabilityManifest) { m.ImplementationRouting.DirectInline.MaxEvidenceBatches = 2 },
		},
		{
			name:   "four inline calls",
			weaken: func(m *AgentCapabilityManifest) { m.ImplementationRouting.DirectInline.MaxEvidenceCalls = 4 },
		},
		{
			name:   "larger inline evidence",
			weaken: func(m *AgentCapabilityManifest) { m.ImplementationRouting.DirectInline.ApproxEvidenceTokens = 20000 },
		},
		{
			name: "unbounded reads",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DirectInline.EvidenceMustUseBoundedRanges = false
			},
		},
		{
			name: "longer sequential exploration",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DelegatedDirect.ApproxSequentialLookupLimit = 6
			},
		},
		{
			name: "long-session mapping stays inline",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DelegatedDirect.DelegateWhenLongSessionMapping = false
			},
		},
		{
			name:   "larger mapper handoff",
			weaken: func(m *AgentCapabilityManifest) { m.ImplementationRouting.DelegatedDirect.ApproxHandoffTokens = 10000 },
		},
		{
			name:   "parent repeats mapped evidence",
			weaken: func(m *AgentCapabilityManifest) { m.ImplementationRouting.DelegatedDirect.MaxParentSpotChecks = 2 },
		},
		{
			name: "later context backstop",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DelegatedDirect.ApproxParentContextTokens = 200000
			},
		},
		{
			name: "claims mechanical context enforcement",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DelegatedDirect.ContextBackstopIsAdvisory = false
			},
		},
		{
			name:   "multiple mechanical write files",
			weaken: func(m *AgentCapabilityManifest) { m.ImplementationRouting.DirectInline.MaxMechanicalWriteFiles = 2 },
		},
		{
			name: "mechanical write not already understood",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DirectInline.MechanicalWriteMustBeAlreadyUnderstood = false
			},
		},
		{
			name: "mechanical write requires research",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DirectInline.MechanicalWriteMustNotRequireResearch = false
			},
		},
		{
			name: "mechanical write has open design work",
			weaken: func(m *AgentCapabilityManifest) {
				m.ImplementationRouting.DirectInline.MechanicalWriteMustNotHaveOpenDesignWork = false
			},
		},
		{
			name: "writer starts after two non-trivial files",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.WriterMinNonTrivialFiles = 3
			},
		},
		{
			name: "read preparing write no longer delegates",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.DelegateWhenReadPreparesWrite = false
			},
		},
		{
			name: "broad research no longer delegates",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.DelegateWhenBroadResearch = false
			},
		},
		{
			name: "substantial ambiguity no longer proposes SDD",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.SDD.ProposeWhenSubstantialOrAmbiguous = false
			},
		},
		{
			name: "SDD proposal need not reduce durable uncertainty",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.SDD.DurableArtifactsMustReduceUncertainty = false
			},
		},
		{
			name: "SDD selection bypasses explicit consent",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.SDD.SelectionPolicy = "automatic"
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			manifest := MustForAgent(model.AgentClaudeCode)
			test.weaken(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() = nil, want non-canonical routing rejection")
			}
		})
	}
}

func TestEveryManifestKeepsWorkRoutingDormantAndHashesCanonically(t *testing.T) {
	t.Parallel()

	const wantRoutingDigest = "sha256:1c343ca2792be730e02b35f1655a19884a2672dc0d2915ef2afb33b6ac01bc1c"
	// Digests pin the four providers with an enforceable fresh-reviewer
	// boundary: Claude Code's generated reviewer has no live tools, OpenCode
	// relays one ordinary task through Go-owned admission, Codex's provider
	// subprocess reaches the same contract, and gentle-pi's host relay
	// forwards the Go-issued opaque task to a fresh locked-down pi
	// subprocess (gentle-pi#311, gentle-ai#3249).
	wantManifestDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:4666df6712fc63b0aacf1227cb28d0afdace1f98cbdd611aa2d5e8d4048b87ce",
		model.AgentClaudeCode:    "sha256:0644de1b6539cffee24ed3d673b450bf1f460fe5db7e8f05c5a0911aace8b280",
		model.AgentCodex:         "sha256:b47855dc0acdae65aa2215eba09135087d32a890814dc73baab13595ecb6602b",
		model.AgentConductor:     "sha256:f06b2f6250b4fe2795a4f3c5dba242f4d19933ba2601311f1e8ed1b9fc95e48f",
		model.AgentCursor:        "sha256:acb6f0092917d40ee12ca88b2661f623f317f0c7436b8b4205fccabf6dd9a9ca",
		model.AgentGeminiCLI:     "sha256:98095b61c7598a2b36088a0d28309ca95d27944298e20d09159375874b09d9fa",
		model.AgentHermes:        "sha256:9f3a958ce7ca8d5b667f6af894eb8085535cd10b81d5ff238ffd358de12f7a5f",
		model.AgentKilocode:      "sha256:13b99f2e8461693bd696ab8c9cea79dfcf37d1859a99f50a74396ff425004d99",
		model.AgentKimi:          "sha256:57b074845e1fe0c98a6bdbb03486d6768a9fa4b48a87c0d4ad68cfdab972f21d",
		model.AgentKiroIDE:       "sha256:e453ea65b26eca021f460b0d833c49eb3047e100aa3f81cb9fb77b77832c0da3",
		model.AgentOpenClaw:      "sha256:5c14949d82e9bce1f283e77733cd7fab50116b5b37fad7fefa0d8fc8dee68299",
		model.AgentOpenCode:      "sha256:c4889e14ddd1f82160d8d5bb49bcdf29bdac5fe367218adf6458a80bc9d5af9d",
		model.AgentPi:            "sha256:cf2edc78e304c31a4fb0fe7ffb034128c008801499200eb6da97303f3a3b719d",
		model.AgentQwenCode:      "sha256:490e77af62787bd18dd47145b673fdd222de199815ad1bed9fa33042eba80387",
		model.AgentTrae:          "sha256:6ee937b8a1a35f51dcb8db6d90b9f05a52b651cedff9d323dede48d9c4a3fde8",
		model.AgentVSCodeCopilot: "sha256:cc800f1eca6d5ea36ae83ae4fd43b59223093196970bdfee59096c60fe22fffe",
		model.AgentWindsurf:      "sha256:2ccb52ebf0926b16f39f59e3df4bbf392c7bdd7fcc76dfb38ee758574be3ad00",
	}

	for agent, wantDigest := range wantManifestDigests {
		agent := agent
		wantDigest := wantDigest
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			manifest := MustForAgent(agent)
			if err := manifest.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if manifest.Contracts.WorkRoutingV1.Exposure != ContractExposureDormant {
				t.Fatalf("work-routing exposure = %q, want %q", manifest.Contracts.WorkRoutingV1.Exposure, ContractExposureDormant)
			}
			if manifest.Advertises(ContractWorkRoutingV1) {
				t.Fatal("work-routing must remain unadvertised before final activation")
			}
			wantImmutableExecutor := agent == model.AgentClaudeCode || agent == model.AgentOpenCode || agent == model.AgentCodex || agent == model.AgentPi
			if got := manifest.Advertises(ContractImmutableReviewExecutorV1); got != wantImmutableExecutor {
				t.Fatalf("immutable reviewer execution advertised = %t, want %t", got, wantImmutableExecutor)
			}
			wantExposure := ContractExposureDormant
			if wantImmutableExecutor {
				wantExposure = ContractExposureAdvertised
			}
			if got := manifest.Contracts.ImmutableReviewExecutorV1.Exposure; got != wantExposure {
				t.Fatalf("immutable reviewer execution exposure = %q, want %q", got, wantExposure)
			}

			payload, err := manifest.CanonicalJSON()
			if err != nil {
				t.Fatalf("CanonicalJSON() error = %v", err)
			}
			// Keep the v1 capability envelope and adapter claims, but never
			// serialize the retired file-count policy as active routing facts.
			for _, retired := range []string{"minUnderstandingFiles", "maxUnderstandingFiles", "mappingMinUnderstandingFiles"} {
				if strings.Contains(string(payload), retired) {
					t.Errorf("canonical JSON retains %s", retired)
				}
			}
			if manifest.SchemaVersion != SchemaV1 {
				t.Error("routing update changed capability schema")
			}
			var roundTrip AgentCapabilityManifest
			if err := json.Unmarshal(payload, &roundTrip); err != nil {
				t.Fatalf("Unmarshal(CanonicalJSON()) error = %v", err)
			}
			if roundTrip != manifest {
				t.Fatalf("canonical JSON round trip = %#v, want %#v", roundTrip, manifest)
			}

			gotDigest, err := roundTrip.Digest()
			if err != nil {
				t.Fatalf("Digest() error = %v", err)
			}
			if gotDigest != wantDigest {
				t.Fatalf("Digest() = %q, want %q", gotDigest, wantDigest)
			}

			gotRoutingDigest, err := manifest.RoutingDigest()
			if err != nil {
				t.Fatalf("RoutingDigest() error = %v", err)
			}
			if gotRoutingDigest != wantRoutingDigest {
				t.Fatalf("RoutingDigest() = %q, want %q", gotRoutingDigest, wantRoutingDigest)
			}
		})
	}
}

// TestEveryManifestDigestStaysByteStable pins every non-Pi row at the
// evidence-budget routing baseline without changing review transport claims.
func TestEveryManifestDigestStaysByteStable(t *testing.T) {
	t.Parallel()

	wantNonPiDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:4666df6712fc63b0aacf1227cb28d0afdace1f98cbdd611aa2d5e8d4048b87ce",
		model.AgentClaudeCode:    "sha256:0644de1b6539cffee24ed3d673b450bf1f460fe5db7e8f05c5a0911aace8b280",
		model.AgentCodex:         "sha256:b47855dc0acdae65aa2215eba09135087d32a890814dc73baab13595ecb6602b",
		model.AgentCommandCode:   "sha256:530d634cfc12ace0bcfa895fba6c5380ed2b6b93fa3dbd36fc1ef462594b728c",
		model.AgentConductor:     "sha256:f06b2f6250b4fe2795a4f3c5dba242f4d19933ba2601311f1e8ed1b9fc95e48f",
		model.AgentCursor:        "sha256:acb6f0092917d40ee12ca88b2661f623f317f0c7436b8b4205fccabf6dd9a9ca",
		model.AgentGeminiCLI:     "sha256:98095b61c7598a2b36088a0d28309ca95d27944298e20d09159375874b09d9fa",
		model.AgentHermes:        "sha256:9f3a958ce7ca8d5b667f6af894eb8085535cd10b81d5ff238ffd358de12f7a5f",
		model.AgentKilocode:      "sha256:13b99f2e8461693bd696ab8c9cea79dfcf37d1859a99f50a74396ff425004d99",
		model.AgentKimi:          "sha256:57b074845e1fe0c98a6bdbb03486d6768a9fa4b48a87c0d4ad68cfdab972f21d",
		model.AgentKiroIDE:       "sha256:e453ea65b26eca021f460b0d833c49eb3047e100aa3f81cb9fb77b77832c0da3",
		model.AgentOpenClaw:      "sha256:5c14949d82e9bce1f283e77733cd7fab50116b5b37fad7fefa0d8fc8dee68299",
		model.AgentOpenCode:      "sha256:c4889e14ddd1f82160d8d5bb49bcdf29bdac5fe367218adf6458a80bc9d5af9d",
		model.AgentQwenCode:      "sha256:490e77af62787bd18dd47145b673fdd222de199815ad1bed9fa33042eba80387",
		model.AgentTrae:          "sha256:6ee937b8a1a35f51dcb8db6d90b9f05a52b651cedff9d323dede48d9c4a3fde8",
		model.AgentVSCodeCopilot: "sha256:cc800f1eca6d5ea36ae83ae4fd43b59223093196970bdfee59096c60fe22fffe",
		model.AgentWindsurf:      "sha256:2ccb52ebf0926b16f39f59e3df4bbf392c7bdd7fcc76dfb38ee758574be3ad00",
	}

	nonPiAgents := make([]model.AgentID, 0, len(wantNonPiDigests))
	for agent := range wantNonPiDigests {
		nonPiAgents = append(nonPiAgents, agent)
	}

	if got := len(nonPiAgents); got != 17 {
		t.Fatalf("want 17 non-Pi agents, got %d", got)
	}

	for _, agent := range nonPiAgents {
		agent := agent
		wantDigest := wantNonPiDigests[agent]
		t.Run(string(agent), func(t *testing.T) {
			t.Parallel()

			manifest := MustForAgent(agent)
			gotDigest, err := manifest.Digest()
			if err != nil {
				t.Fatalf("Digest() error = %v", err)
			}
			if gotDigest != wantDigest {
				t.Fatalf("Digest() = %q, want %q (byte-stable contract)", gotDigest, wantDigest)
			}
		})
	}
}

func TestReviewTransportAdvertisementIsClosedCatalogSet(t *testing.T) {
	const wantExposed = 4

	exposed := 0
	for _, agent := range catalog.AllAgents() {
		t.Run(string(agent.ID), func(t *testing.T) {
			manifest := MustForAgent(agent.ID)
			want := agent.ID == model.AgentClaudeCode ||
				agent.ID == model.AgentOpenCode ||
				agent.ID == model.AgentCodex ||
				agent.ID == model.AgentPi
			if got := manifest.Advertises(ContractReviewTransportV1); got != want {
				t.Fatalf("review transport advertised = %t, want %t", got, want)
			}
			if want {
				exposed++
			}
		})
	}
	if exposed != wantExposed {
		t.Fatalf("advertised review transport runtimes = %d, want %d", exposed, wantExposed)
	}
}

func TestForAgentRejectsUnknownAgent(t *testing.T) {
	t.Parallel()

	_, err := ForAgent(model.AgentID("unknown"))
	if !errors.Is(err, ErrUnsupportedAgent) {
		t.Fatalf("ForAgent() error = %v, want ErrUnsupportedAgent", err)
	}
}
