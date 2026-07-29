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
	wantManifestDigests := map[model.AgentID]string{
		model.AgentAntigravity:   "sha256:2f72974f6abdce68ca28a585705227d37d1c64c965120281727455223d678394",
		model.AgentClaudeCode:    "sha256:1954836303597cd9efc3e9736f2eb7c72d2c3b5107f6f36e0ca63d82c561c005",
		model.AgentCodex:         "sha256:fd1ed4fc30881c9ce53550f4c57ad7bd007e1b76bba943510ef53840b2e43a16",
		model.AgentCursor:        "sha256:5ec0323fd33720a5a99ec2ff8b876312f52aa6a588871a71920516f748f23f80",
		model.AgentGeminiCLI:     "sha256:3d51601fb11f71e2cc22daba09fd19dfdf473fbcbf3b16d732d4100f0a09cbbb",
		model.AgentHermes:        "sha256:00d1f1d2db659d33a032a97dae373a0f5e4a676921ef65d0f1162923e4758aa1",
		model.AgentKilocode:      "sha256:5472e4fb098caa868c650cf0d065bf277e079ddb8d5b8996b0cd9e8faa72d381",
		model.AgentKimi:          "sha256:20da639dbb4c852aef56c81e417641bc0b817a7fa41fd6aa2eecfe42aad9fafa",
		model.AgentKiroIDE:       "sha256:00cec3beeaa3506476151a6aa19966a6b3bfe52c3648ecf2e9d1804adafb86c2",
		model.AgentOpenClaw:      "sha256:e3dadd12614a5d27daf1c3fbdde875df5cb52888b7987ded87e5abd7bca8d49f",
		model.AgentOpenCode:      "sha256:77b30ecfac3cd3a6d54db33328b9ddddd8db3f11fa19ddd5e3829c5a0a506b80",
		model.AgentPi:            "sha256:f1f8f67171ef2ea40f5690b4c2f20f7e0073e6292092b218c81a69b31281d5ae",
		model.AgentQwenCode:      "sha256:def191f9b6ec065eda9fdd490817f27bbc89634b393db4bbbe9e81dffe1d9fba",
		model.AgentTrae:          "sha256:aada07d8d187a2649cf18613c2ba4be6eefd1632c39a7e792c6ec23dcc8e803d",
		model.AgentVSCodeCopilot: "sha256:3be9f31260509c10c8f4b2866c76f950ab4d2fabd8bf8684fd3af7a9b2391657",
		model.AgentWindsurf:      "sha256:a8c46fa07497092005ce74cd3b71ef230704408ea035b08229ebd054f42794ae",
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
