package sddstatus

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveEngramEarlyArtifactsDiscoverAndRouteToPropose(t *testing.T) {
	cases := []struct {
		name   string
		titles []string
	}{
		{name: "explore only", titles: []string{"explore"}},
		{name: "pre-proposal only", titles: []string{"pre-proposal"}},
		{name: "combined early artifacts", titles: []string{"explore", "pre-proposal"}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
			t.Setenv("ENGRAM_PROJECT", "gentle-ai")

			observations := make([]engramObservation, 0, len(tt.titles))
			for _, title := range tt.titles {
				observations = append(observations, engramObservation{
					Title:   "sdd/early-artifacts/" + title,
					Content: "# Early artifact\n",
					Project: "gentle-ai",
					Scope:   "project",
				})
			}
			t.Cleanup(stubEngramExport(t, observations))

			for _, requestedChange := range []string{"", "early-artifacts"} {
				status, err := Resolve(ResolveOptions{CWD: root, ChangeName: requestedChange})
				if err != nil {
					t.Fatalf("Resolve(change=%q): %v", requestedChange, err)
				}
				if got := ptrValue(status.ChangeName); got != "early-artifacts" {
					t.Fatalf("Resolve(change=%q) ChangeName = %q, want early-artifacts", requestedChange, got)
				}
				if status.NextRecommended != string(PhasePropose) {
					t.Fatalf("Resolve(change=%q) NextRecommended = %q, want propose", requestedChange, status.NextRecommended)
				}
				if status.Artifacts["proposal"] != ArtifactMissing || status.Dependencies.Proposal != DependencyBlocked {
					t.Fatalf("Resolve(change=%q) treated early artifacts as a proposal: artifacts=%v dependencies=%#v", requestedChange, status.Artifacts, status.Dependencies)
				}
			}
		})
	}
}

func TestResolveEngramEarlyArtifactsWithProposalRoutesToSpec(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
	t.Setenv("ENGRAM_PROJECT", "gentle-ai")
	t.Cleanup(stubEngramExport(t, []engramObservation{
		{Title: "sdd/early-artifacts/explore", Content: "# Explore\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/early-artifacts/pre-proposal", Content: "# Pre-proposal\n", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/early-artifacts/proposal", Content: "# Proposal\n", Project: "gentle-ai", Scope: "project"},
	}))

	for _, requestedChange := range []string{"", "early-artifacts"} {
		status, err := Resolve(ResolveOptions{CWD: root, ChangeName: requestedChange})
		if err != nil {
			t.Fatalf("Resolve(change=%q): %v", requestedChange, err)
		}
		if got := ptrValue(status.ChangeName); got != "early-artifacts" || status.NextRecommended != string(PhaseSpec) {
			t.Fatalf("Resolve(change=%q) = change %q next %q, want early-artifacts/spec", requestedChange, got, status.NextRecommended)
		}
		if status.Artifacts["proposal"] != ArtifactDone || status.Dependencies.Proposal != DependencyAllDone {
			t.Fatalf("proposal completion = artifacts=%v dependencies=%#v, want proposal done only after its titled artifact", status.Artifacts, status.Dependencies)
		}
	}
}

func TestEngramEarlyArtifactsPreserveProjectArchiveAndSelectionRules(t *testing.T) {
	observations := []engramObservation{
		{Title: "sdd/early/explore", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/current/pre-proposal", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/foreign/explore", Project: "another-project", Scope: "project"},
		{Title: "sdd/personal/pre-proposal", Project: "gentle-ai", Scope: "personal"},
		{Title: "sdd/archived/explore", Project: "gentle-ai", Scope: "project"},
		{Title: "sdd/archived/archive-report", Project: "gentle-ai", Scope: "project"},
	}
	if got, want := collectEngramChanges(observations, "gentle-ai"), []string{"current", "early"}; !slices.Equal(got, want) {
		t.Fatalf("collectEngramChanges() = %v, want %v; foreign, personal, and archived changes must remain excluded", got, want)
	}

	root := t.TempDir()
	write(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
	t.Setenv("ENGRAM_PROJECT", "gentle-ai")
	t.Cleanup(stubEngramExport(t, observations))

	status, err := Resolve(ResolveOptions{CWD: root})
	if err != nil {
		t.Fatalf("Resolve(auto): %v", err)
	}
	if status.NextRecommended != "select-change" || status.ChangeName != nil {
		t.Fatalf("Resolve(auto) = change %q next %q, want ambiguous selection between the two active project changes", ptrValue(status.ChangeName), status.NextRecommended)
	}

	archived, err := Resolve(ResolveOptions{CWD: root, ChangeName: "archived"})
	if err != nil {
		t.Fatalf("Resolve(archived): %v", err)
	}
	if archived.Archived == nil || archived.NextRecommended != "archived" {
		t.Fatalf("Resolve(archived) = archived %#v next %q, want existing archive projection", archived.Archived, archived.NextRecommended)
	}
}
