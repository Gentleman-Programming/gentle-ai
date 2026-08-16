package sddstatus

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveSpecDiscoveryPrecedenceAndFallback(t *testing.T) {
	flatSpec := "### Requirement: flat\n#### Scenario: present\n"
	emptyFlat := ""
	tests := []struct {
		name           string
		nested         string
		flat           *string
		wantPaths      []string
		wantArtifact   ArtifactState
		wantDependency DependencyState
		wantNext       string
	}{
		{
			name:           "nested only",
			nested:         "### Requirement: nested\n#### Scenario: present\n",
			wantPaths:      []string{"specs/auth/spec.md"},
			wantArtifact:   ArtifactDone,
			wantDependency: DependencyAllDone,
			wantNext:       "apply",
		},
		{
			name:           "flat only",
			flat:           &flatSpec,
			wantPaths:      []string{"spec.md"},
			wantArtifact:   ArtifactDone,
			wantDependency: DependencyAllDone,
			wantNext:       "apply",
		},
		{
			name:           "both present keeps nested authoritative",
			nested:         "### Requirement: nested\n#### Scenario: present\n",
			flat:           &flatSpec,
			wantPaths:      []string{"specs/auth/spec.md"},
			wantArtifact:   ArtifactDone,
			wantDependency: DependencyAllDone,
			wantNext:       "apply",
		},
		{
			name:           "empty flat is not an artifact",
			flat:           &emptyFlat,
			wantPaths:      []string{},
			wantArtifact:   ArtifactMissing,
			wantDependency: DependencyBlocked,
			wantNext:       "spec",
		},
		{
			name:           "neither present",
			wantPaths:      []string{},
			wantArtifact:   ArtifactMissing,
			wantDependency: DependencyBlocked,
			wantNext:       "spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			changeRoot := filepath.Join(root, "openspec", "changes", "flat-spec")
			write(t, filepath.Join(changeRoot, "proposal.md"), "# Proposal\n")
			write(t, filepath.Join(changeRoot, "design.md"), "# Design\n")
			write(t, filepath.Join(changeRoot, "tasks.md"), "- [ ] 1.1 Work\n")
			if tt.nested != "" {
				write(t, filepath.Join(changeRoot, "specs", "auth", "spec.md"), tt.nested)
			}
			if tt.flat != nil {
				write(t, filepath.Join(changeRoot, "spec.md"), *tt.flat)
			}

			status, err := Resolve(ResolveOptions{CWD: root, ChangeName: "flat-spec"})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			wantPaths := make([]string, 0, len(tt.wantPaths))
			for _, path := range tt.wantPaths {
				wantPaths = append(wantPaths, filepath.Join(changeRoot, path))
			}
			if !reflect.DeepEqual(status.ArtifactPaths.Specs, wantPaths) {
				t.Fatalf("ArtifactPaths.Specs = %v, want %v", status.ArtifactPaths.Specs, wantPaths)
			}
			if status.Artifacts["specs"] != tt.wantArtifact {
				t.Fatalf("Artifacts[specs] = %q, want %q", status.Artifacts["specs"], tt.wantArtifact)
			}
			if status.Dependencies.Specs != tt.wantDependency {
				t.Fatalf("Dependencies.Specs = %q, want %q", status.Dependencies.Specs, tt.wantDependency)
			}
			if status.NextRecommended != tt.wantNext {
				t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.wantNext)
			}
		})
	}
}

func TestPlanningSpecInstructionsDescribeBothLayouts(t *testing.T) {
	instructions := strings.Join(planningInstructionsForPhase(Status{}, PhaseSpec), "\n")
	if !strings.Contains(instructions, "Create spec.md or specs/<domain>/spec.md with requirements and scenarios.") {
		t.Fatalf("spec planning instructions = %q, want both spec layouts", instructions)
	}
}
