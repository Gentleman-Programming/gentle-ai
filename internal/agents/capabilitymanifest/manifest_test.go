package capabilitymanifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

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
	// Digests intentionally changed here: AgentFeatureClaims dropped the
	// AutoInstall field (gentle-ai no longer installs agent runtimes on the
	// user's behalf — see agentInstallStep in internal/cli/run.go), which is
	// content the canonical JSON payload — and therefore the digest —
	// legitimately covers for every agent, whether that agent's AutoInstall
	// claim used to be true or false.
	wantManifestDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:6d9ccea52a22a523d90b35d2c9540e2953a0d1dd44b368922546d6e3690180c8",
		model.AgentClaudeCode:    "sha256:a330f5e7d36a83fe98aea15fbf6d81a445f0073e6b0b7bfebe052466b0539e05",
		model.AgentCodex:         "sha256:986cdb1c75d26840217634f720093511369e902c14b068a3245e8ebf98c41a5c",
		model.AgentCursor:        "sha256:d5be74e67fa7b78c6b8aeee602894395ddb9e9f481b797693f16c348c0274b0a",
		model.AgentGeminiCLI:     "sha256:3178d298a261cc1bfae1c96201f022f3d5b81135089130e5c747afb2ae39450f",
		model.AgentHermes:        "sha256:f042913fabdc55ad918cd792dab5566b281fa568d37384bf613a9bef13d00850",
		model.AgentKilocode:      "sha256:ba4d977d58b8974db2dcabc3df80432fded140c4a57a16e7be68167639d768d2",
		model.AgentKimi:          "sha256:4f17a25513153d27df042fa7211af1f1520f635e216b3b00e1ae71aa1390b93b",
		model.AgentKiroIDE:       "sha256:e6ea2bbb0848263e2f75b49d462dd853ff59549b9bb2051a8adea963515f8023",
		model.AgentOpenClaw:      "sha256:0ec685b0076d542e3a9762e1fd5bfaccb9113673e2c2d2af530ec88d216f9c4d",
		model.AgentOpenCode:      "sha256:a2ee7d6528f50f709335326aa5f06123d7223df6092e85198bf7a0d4fbf2aa29",
		model.AgentPi:            "sha256:5e2201bf60bf3943ae43afde1327da5fbcc2eb70701feb3bd1bd06c6e4e3172c",
		model.AgentQwenCode:      "sha256:13b640c6f6c2b9b287d4cbeb3777fee052777433db8551cdd1210ffc11bd6923",
		model.AgentTrae:          "sha256:d20a478bc6bce2fa52fbe9dd6735ba6926d89f2bdc97fd8e096d7025274bfdc3",
		model.AgentVSCodeCopilot: "sha256:995c53449c739c4ceeb18c61ce012f27090a263f5d02080982ffffd1266e5e37",
		model.AgentWindsurf:      "sha256:72479f4114b47d93c4ac2b4facd471ab173c456cd77fee7d3df52152938bac4c",
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

func TestForAgentRejectsUnknownAgent(t *testing.T) {
	t.Parallel()

	_, err := ForAgent(model.AgentID("unknown"))
	if !errors.Is(err, ErrUnsupportedAgent) {
		t.Fatalf("ForAgent() error = %v, want ErrUnsupportedAgent", err)
	}
}
