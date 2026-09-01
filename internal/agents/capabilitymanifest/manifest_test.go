package capabilitymanifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestCanonicalImplementationRoutingBoundaries(t *testing.T) {
	t.Parallel()

	got := CanonicalImplementationRouting()
	want := ImplementationRoutingFacts{
		DirectInline: DirectInlineFacts{
			MinUnderstandingFiles:                    1,
			MaxUnderstandingFiles:                    3,
			MaxMechanicalWriteFiles:                  1,
			MechanicalWriteMustBeAlreadyUnderstood:   true,
			MechanicalWriteMustNotRequireResearch:    true,
			MechanicalWriteMustNotHaveOpenDesignWork: true,
		},
		DelegatedDirect: DelegatedDirectFacts{
			MappingMinUnderstandingFiles:  4,
			WriterMinNonTrivialFiles:      2,
			DelegateWhenReadPreparesWrite: true,
			DelegateWhenBroadResearch:     true,
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
			name: "direct understanding starts below one file",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DirectInline.MinUnderstandingFiles = 0
			},
		},
		{
			name: "direct understanding exceeds three files",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DirectInline.MaxUnderstandingFiles = 4
			},
		},
		{
			name: "mapping starts after four files",
			weaken: func(manifest *AgentCapabilityManifest) {
				manifest.ImplementationRouting.DelegatedDirect.MappingMinUnderstandingFiles = 5
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

	const wantRoutingDigest = "sha256:ed03b86f20c9449a6e4c018f51d1e05619e1070b1076287a0792a74c458762b2"
	// Digests pin the four providers with an enforceable fresh-reviewer
	// boundary: Claude Code's generated reviewer has no live tools, OpenCode
	// relays one ordinary task through Go-owned admission, Codex's provider
	// subprocess reaches the same contract, and gentle-pi's host relay
	// forwards the Go-issued opaque task to a fresh locked-down pi
	// subprocess (gentle-pi#311, gentle-ai#3249).
	wantManifestDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:8e09945cd860b793c59f73db19827bcb4dcfd75c9ecc7f876167ab52fe77ccc2",
		model.AgentClaudeCode:    "sha256:132b9219b222d35b0e4eafce3dae965c56eb8d79f07dff6d45c42c137e36fd9b",
		model.AgentCodex:         "sha256:dbf94a3b7815cf68ccd6299c634f3e17be9abc305b3849adee382c65055c5ed9",
		model.AgentCursor:        "sha256:08e32b28b4cde7ffaf67210354fb95df2aaf424016ec6093190fb38c5f7226cb",
		model.AgentGeminiCLI:     "sha256:5738280648925ebc011e6564b59bd6108bb573b5615771286fbcba97876a61dc",
		model.AgentHermes:        "sha256:25a9583f4b1fe58dbc64a33a016e9a1d88acb545d3d6f6648bbf08dc29cb5656",
		model.AgentKilocode:      "sha256:9cd93b70fee7da43b7dffdd4f0a7a949886b24c97bef163ef99f4d04d21dc06d",
		model.AgentKimi:          "sha256:6cc52c4b6e00d15a91b76259f2e594001904b8dd0eabb0b41c4d3b72669d9964",
		model.AgentKiroIDE:       "sha256:ac77662bea712a283a44e7985257ec68f4d1217cf311dbb9322966f9e5c8423a",
		model.AgentOpenClaw:      "sha256:f83aee743181528688a9555639f1b573c8273d0cfc28b7b499bffa21c406deb2",
		model.AgentOpenCode:      "sha256:3df2c0ee0a61774b7b7f0d547abed55721cc37ecc332c131935ce72fb142103f",
		model.AgentPi:            "sha256:0332851d2286a97ab824a1d656b94f02651bfbf85bdf0f6cc47fe8f7d09765ad",
		model.AgentQwenCode:      "sha256:11e49bee9741be99ae23257471e78258b1429d74ac491e79395bcfe46774614c",
		model.AgentTrae:          "sha256:fbbc5ae0a54d31aee4322a89a4d95f854a564d6ea16438c30d0de01b34f7bb8e",
		model.AgentVSCodeCopilot: "sha256:d982315762ac70ed1a855aec32bc75547b02ba16ae87a330af14366e0e8facee",
		model.AgentWindsurf:      "sha256:0b70f983ef8d5154f1d13c9d377d70d04cae1eaa460f280ee96db6a43f84fa28",
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

// TestEveryManifestDigestStaysByteStable pins every non-Pi row at the closed
// review-transport baseline. The 12 non-RDD rows change only because their
// transport claim becomes dormant; the three non-Pi RDD rows remain unchanged.
func TestEveryManifestDigestStaysByteStable(t *testing.T) {
	t.Parallel()

	wantNonPiDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:8e09945cd860b793c59f73db19827bcb4dcfd75c9ecc7f876167ab52fe77ccc2",
		model.AgentClaudeCode:    "sha256:132b9219b222d35b0e4eafce3dae965c56eb8d79f07dff6d45c42c137e36fd9b",
		model.AgentCodex:         "sha256:dbf94a3b7815cf68ccd6299c634f3e17be9abc305b3849adee382c65055c5ed9",
		model.AgentCursor:        "sha256:08e32b28b4cde7ffaf67210354fb95df2aaf424016ec6093190fb38c5f7226cb",
		model.AgentGeminiCLI:     "sha256:5738280648925ebc011e6564b59bd6108bb573b5615771286fbcba97876a61dc",
		model.AgentHermes:        "sha256:25a9583f4b1fe58dbc64a33a016e9a1d88acb545d3d6f6648bbf08dc29cb5656",
		model.AgentKilocode:      "sha256:9cd93b70fee7da43b7dffdd4f0a7a949886b24c97bef163ef99f4d04d21dc06d",
		model.AgentKimi:          "sha256:6cc52c4b6e00d15a91b76259f2e594001904b8dd0eabb0b41c4d3b72669d9964",
		model.AgentKiroIDE:       "sha256:ac77662bea712a283a44e7985257ec68f4d1217cf311dbb9322966f9e5c8423a",
		model.AgentOpenClaw:      "sha256:f83aee743181528688a9555639f1b573c8273d0cfc28b7b499bffa21c406deb2",
		model.AgentOpenCode:      "sha256:3df2c0ee0a61774b7b7f0d547abed55721cc37ecc332c131935ce72fb142103f",
		model.AgentQwenCode:      "sha256:11e49bee9741be99ae23257471e78258b1429d74ac491e79395bcfe46774614c",
		model.AgentTrae:          "sha256:fbbc5ae0a54d31aee4322a89a4d95f854a564d6ea16438c30d0de01b34f7bb8e",
		model.AgentVSCodeCopilot: "sha256:d982315762ac70ed1a855aec32bc75547b02ba16ae87a330af14366e0e8facee",
		model.AgentWindsurf:      "sha256:0b70f983ef8d5154f1d13c9d377d70d04cae1eaa460f280ee96db6a43f84fa28",
	}

	nonPiAgents := make([]model.AgentID, 0, len(wantNonPiDigests))
	for agent := range wantNonPiDigests {
		nonPiAgents = append(nonPiAgents, agent)
	}

	if got := len(nonPiAgents); got != 15 {
		t.Fatalf("want 15 non-Pi agents, got %d", got)
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
