package skills

import (
	"errors"
	"os"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/agents"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/antigravity"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/capabilitymanifest"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/claude"
	"github.com/gentleman-programming/gentle-ai/v2/internal/agents/kimi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/catalog"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestInjectFiltersSkillsByRequiredCapabilities(t *testing.T) {
	tests := []struct {
		name         string
		adapter      agents.Adapter
		requirements capabilitymanifest.AgentFeatureClaims
		wantSkipped  bool
	}{
		{
			name:         "declared compatible skill installs",
			adapter:      kimi.NewAdapter(),
			requirements: capabilitymanifest.AgentFeatureClaims{FileSubAgents: true},
		},
		{
			name:         "declared incompatible skill is skipped",
			adapter:      antigravity.NewAdapter(),
			requirements: capabilitymanifest.AgentFeatureClaims{FileSubAgents: true},
			wantSkipped:  true,
		},
		{
			name:    "unannotated skill remains fail open",
			adapter: antigravity.NewAdapter(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			available := []catalog.Skill{{
				ID:                   model.SkillCreator,
				Name:                 "skill-creator",
				RequiredCapabilities: tt.requirements,
			}}

			result, err := injectWithCatalog(home, tt.adapter, []model.SkillID{model.SkillCreator}, "", available)
			if err != nil {
				t.Fatalf("injectWithCatalog() error = %v", err)
			}

			path := SkillPathForAgent(home, tt.adapter, model.SkillCreator)
			if tt.wantSkipped {
				if len(result.Skipped) != 1 || result.Skipped[0] != model.SkillCreator {
					t.Fatalf("injectWithCatalog() skipped = %v, want [%s]", result.Skipped, model.SkillCreator)
				}
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("incompatible skill path stat error = %v, want not exist", statErr)
				}
				return
			}

			if len(result.Skipped) != 0 {
				t.Fatalf("injectWithCatalog() skipped = %v, want none", result.Skipped)
			}
			assertNonEmptyFile(t, path)
		})
	}
}

func TestInjectPreflightsCatalogAndManifestBeforeWritingSkills(t *testing.T) {
	tests := []struct {
		name      string
		adapter   agents.Adapter
		available []catalog.Skill
		wantErr   error
	}{
		{
			name:    "invalid manifest",
			adapter: invalidManifestAdapter{Adapter: claude.NewAdapter()},
			available: []catalog.Skill{
				{ID: model.SkillCreator, Name: "skill-creator"},
				{ID: model.SkillGoTesting, Name: "go-testing", RequiredCapabilities: capabilitymanifest.AgentFeatureClaims{FileSubAgents: true}},
			},
			wantErr: agents.ErrCapabilityManifestMismatch,
		},
		{
			name:    "duplicate catalog ID",
			adapter: claude.NewAdapter(),
			available: []catalog.Skill{
				{ID: model.SkillCreator, Name: "skill-creator"},
				{ID: model.SkillCreator, Name: "duplicate-skill-creator"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			_, err := injectWithCatalog(home, tt.adapter, []model.SkillID{model.SkillCreator, model.SkillGoTesting}, "", tt.available)
			if err == nil {
				t.Fatal("injectWithCatalog() error = nil, want preflight failure")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("injectWithCatalog() error = %v, want %v", err, tt.wantErr)
			}

			earlierPath := SkillPathForAgent(home, tt.adapter, model.SkillCreator)
			if _, statErr := os.Stat(earlierPath); !os.IsNotExist(statErr) {
				t.Fatalf("earlier skill path stat error = %v, want not exist", statErr)
			}
		})
	}
}

type invalidManifestAdapter struct {
	agents.Adapter
}

func (a invalidManifestAdapter) CapabilityManifest() capabilitymanifest.AgentCapabilityManifest {
	return capabilitymanifest.AgentCapabilityManifest{}
}

func TestRequiredCapabilitiesAreSubsetOfProvided(t *testing.T) {
	all := capabilitymanifest.AgentFeatureClaims{
		OutputStyles: true, SlashCommands: true, FileSubAgents: true, Skills: true,
		SystemPrompt: true, MCP: true, Workflows: true,
	}
	tests := []struct {
		name     string
		required capabilitymanifest.AgentFeatureClaims
		provided capabilitymanifest.AgentFeatureClaims
		want     bool
	}{
		{name: "empty requirement", provided: capabilitymanifest.AgentFeatureClaims{}, want: true},
		{name: "all dimensions provided", required: all, provided: all, want: true},
		{name: "output styles absent", required: capabilitymanifest.AgentFeatureClaims{OutputStyles: true}, want: false},
		{name: "slash commands absent", required: capabilitymanifest.AgentFeatureClaims{SlashCommands: true}, want: false},
		{name: "file subagents absent", required: capabilitymanifest.AgentFeatureClaims{FileSubAgents: true}, want: false},
		{name: "skills absent", required: capabilitymanifest.AgentFeatureClaims{Skills: true}, want: false},
		{name: "system prompt absent", required: capabilitymanifest.AgentFeatureClaims{SystemPrompt: true}, want: false},
		{name: "MCP absent", required: capabilitymanifest.AgentFeatureClaims{MCP: true}, want: false},
		{name: "workflows absent", required: capabilitymanifest.AgentFeatureClaims{Workflows: true}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredCapabilitiesCompatible(tt.required, tt.provided); got != tt.want {
				t.Fatalf("requiredCapabilitiesCompatible() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestFileSubAgentsManifestClaims(t *testing.T) {
	tests := []struct {
		name    string
		adapter agents.Adapter
		want    bool
	}{
		{name: "Antigravity is incompatible", adapter: antigravity.NewAdapter(), want: false},
		{name: "Kimi is compatible", adapter: kimi.NewAdapter(), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := agents.ResolveCapabilityManifest(tt.adapter)
			if err != nil {
				t.Fatalf("ResolveCapabilityManifest() error = %v", err)
			}
			if manifest.Features.FileSubAgents != tt.want {
				t.Fatalf("FileSubAgents = %t, want %t", manifest.Features.FileSubAgents, tt.want)
			}
		})
	}
}
