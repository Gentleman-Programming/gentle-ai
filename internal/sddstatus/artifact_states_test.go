package sddstatus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// engramWorkspaceWithArchivedChangesOnly builds the shape issue #2346 reports:
// a hybrid workspace whose openspec/changes holds only archive/, while the
// Engram store still carries observations from previously finished changes.
func engramWorkspaceWithArchivedChangesOnly(t *testing.T, changes ...string) string {
	t.Helper()
	t.Setenv("ENGRAM_PROJECT", "gentle-ai")
	workspace := t.TempDir()
	mkdir(t, filepath.Join(workspace, "openspec", "changes", "archive"))
	write(t, filepath.Join(workspace, "openspec", "config.yaml"), "artifact_store: hybrid\n")

	observations := []engramObservation{}
	for _, change := range changes {
		observations = append(observations, engramPlanningRoute(change, "tasks")...)
	}
	t.Cleanup(stubEngramExport(t, observations))
	return workspace
}

// TestEmptyWorkspaceStatusProjectsV2 pins issue #2346 property 1: a workspace
// with nothing but archive/ must return a usable status on both the JSON and
// the markdown path, for every artifact store the resolver can land on.
func TestEmptyWorkspaceStatusProjectsV2(t *testing.T) {
	tests := []struct {
		name      string
		workspace func(t *testing.T) string
		wantStore ArtifactStore
		wantNext  string
	}{
		{
			name: "openspec store with no engram changes",
			workspace: func(t *testing.T) string {
				return engramWorkspaceWithArchivedChangesOnly(t)
			},
			wantStore: ArtifactStoreOpenSpec,
			wantNext:  "sdd-new",
		},
		{
			name: "engram store with only archived engram changes",
			workspace: func(t *testing.T) string {
				return engramWorkspaceWithArchivedChangesOnly(t, "alpha-change", "beta-change")
			},
			wantStore: ArtifactStoreEngram,
			wantNext:  "select-change",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := tt.workspace(t)
			before := snapshotWorkspace(t, workspace)

			status, err := Resolve(ResolveOptions{WorkspaceRoot: workspace, IncludeInstructions: true})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if status.ArtifactStore != tt.wantStore {
				t.Fatalf("ArtifactStore = %q, want %q", status.ArtifactStore, tt.wantStore)
			}
			if status.NextRecommended != tt.wantNext {
				t.Fatalf("NextRecommended = %q, want %q", status.NextRecommended, tt.wantNext)
			}

			projected, err := ProjectStatusV2(status)
			if err != nil {
				t.Fatalf("ProjectStatusV2() error = %v", err)
			}
			payload, err := json.Marshal(projected)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if !strings.Contains(string(payload), `"schemaName":"gentle-ai.sdd-status"`) {
				t.Fatalf("projected JSON lost its identity: %s", payload)
			}

			markdown := RenderMarkdown(status)
			if strings.Contains(markdown, "\n{}\n") {
				t.Fatalf("markdown fell back to the empty projection:\n%s", markdown)
			}
			if !strings.Contains(markdown, `"schemaName": "gentle-ai.sdd-status"`) {
				t.Fatalf("markdown omitted the projected status:\n%s", markdown)
			}

			// Property 3: sdd-status is read-only.
			assertWorkspaceUnchanged(t, workspace, before)
		})
	}
}

// TestEveryArtifactMapBuilderSatisfiesTheV2Projection pins issue #2346
// property 2. It names no artifact key: each case hands the projection the map
// a production builder actually produced for the store that same builder was
// asked for, and then proves the projection genuinely demands every key in it
// by deleting one at a time. A builder that forgets a key its store declares,
// or a status whose store no longer matches the map it carries, fails here
// rather than at a user's terminal.
func TestEveryArtifactMapBuilderSatisfiesTheV2Projection(t *testing.T) {
	for _, built := range artifactMapsFromEveryBuilder(t) {
		t.Run(built.builder, func(t *testing.T) {
			if len(built.artifacts) == 0 {
				t.Fatal("builder produced no artifact states")
			}
			if _, err := projectArtifactsV2(built.store, built.artifacts); err != nil {
				t.Fatalf("projectArtifactsV2(%q, <%s output>) error = %v", built.store, built.builder, err)
			}
			for key := range built.artifacts {
				withhold := make(map[string]ArtifactState, len(built.artifacts))
				for existing, state := range built.artifacts {
					if existing != key {
						withhold[existing] = state
					}
				}
				if _, err := projectArtifactsV2(built.store, withhold); err == nil {
					t.Fatalf("projectArtifactsV2(%q) accepted a map missing %q, so the builder and the projection do not agree on the key set", built.store, key)
				}
			}
		})
	}
}

type builtArtifactMap struct {
	builder   string
	store     ArtifactStore
	artifacts map[string]ArtifactState
}

// artifactMapsFromEveryBuilder collects the artifact map produced by every
// production path that builds one, through the real entry points rather than
// by restating the literals.
func artifactMapsFromEveryBuilder(t *testing.T) []builtArtifactMap {
	t.Helper()
	t.Setenv("ENGRAM_PROJECT", "gentle-ai")
	collected := []builtArtifactMap{}

	for _, store := range []ArtifactStore{ArtifactStoreOpenSpec, ArtifactStoreEngram, ArtifactStoreNone} {
		status := baseStatus(store, "/repo", nil, nil, nil, "sdd-new", nil)
		if status.ArtifactStore != store {
			t.Fatalf("baseStatus(%q) produced store %q", store, status.ArtifactStore)
		}
		collected = append(collected, builtArtifactMap{
			builder:   "baseStatus/" + string(store),
			store:     status.ArtifactStore,
			artifacts: status.Artifacts,
		})
	}

	openspec := t.TempDir()
	write(t, filepath.Join(openspec, "openspec", "changes", "live-change", "proposal.md"), "# Proposal\n")
	write(t, filepath.Join(openspec, "openspec", "changes", "live-change", "tasks.md"), "- [ ] 1.1 Work\n")
	resolved, err := Resolve(ResolveOptions{WorkspaceRoot: openspec})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	collected = append(collected, builtArtifactMap{
		builder:   "Resolve",
		store:     resolved.ArtifactStore,
		artifacts: resolved.Artifacts,
	})

	engram := t.TempDir()
	mkdir(t, filepath.Join(engram, "openspec", "changes", "archive"))
	mkdir(t, filepath.Join(engram, ".engram"))
	defer stubEngramExport(t, engramPlanningRoute("engram-change", "tasks"))()
	engramStatus, err := Resolve(ResolveOptions{WorkspaceRoot: engram})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if engramStatus.ArtifactStore != ArtifactStoreEngram {
		t.Fatalf("engram fixture resolved to store %q; the case would not exercise the Engram builder", engramStatus.ArtifactStore)
	}
	collected = append(collected, builtArtifactMap{
		builder:   "resolveEngramStatus",
		store:     engramStatus.ArtifactStore,
		artifacts: engramStatus.Artifacts,
	})

	blocked := blockedEngramStatus(engram, nil, "select-change", []string{"ambiguous"}, false)
	if blocked.ArtifactStore != ArtifactStoreEngram {
		t.Fatalf("blockedEngramStatus produced store %q", blocked.ArtifactStore)
	}
	collected = append(collected, builtArtifactMap{
		builder:   "blockedEngramStatus",
		store:     blocked.ArtifactStore,
		artifacts: blocked.Artifacts,
	})

	return collected
}

func snapshotWorkspace(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			snapshot[relative] = "dir"
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[relative] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("walk workspace: %v", err)
	}
	return snapshot
}

func assertWorkspaceUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotWorkspace(t, root)
	for path, content := range after {
		previous, existed := before[path]
		if !existed {
			t.Fatalf("sdd-status created %q; the command must be read-only", path)
		}
		if previous != content {
			t.Fatalf("sdd-status mutated %q; the command must be read-only", path)
		}
	}
	for path := range before {
		if _, still := after[path]; !still {
			t.Fatalf("sdd-status removed %q; the command must be read-only", path)
		}
	}
}
