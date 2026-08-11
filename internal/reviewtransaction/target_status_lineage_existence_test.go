package reviewtransaction

import (
	"context"
	"strings"
	"testing"
)

// TestExplicitStatusLineageMustExist proves that an explicit --lineage selector
// names authority that actually exists in the repository. Issue #1997: a
// well-formed but nonexistent lineage fell through the empty candidate set into
// the same fresh-target envelope as selector-free discovery and exited 0, so
// `review status --lineage <id>` could never answer the question its own
// refusal continuation asked. The same veto must hold for every authority
// format the existence check can discover: compact, legacy, and v3.
func TestExplicitStatusLineageMustExist(t *testing.T) {
	formats := []struct {
		name string
		seed func(t *testing.T, repo, lineage string)
	}{
		{
			name: "compact authority",
			seed: func(t *testing.T, repo, lineage string) {
				state := newCompactTestState(t, repo, lineage)
				if _, err := StartCompactAuthority(context.Background(), repo, CompactStartRequest{State: state}); err != nil {
					t.Fatalf("start compact authority: %v", err)
				}
			},
		},
		{
			name: "legacy authority",
			seed: func(t *testing.T, repo, lineage string) {
				requireSnapshotGit(t)
				head := strings.TrimSpace(gitSnapshot(t, repo, "rev-parse", "HEAD"))
				snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{Kind: TargetExactRevision, Revision: head})
				if err != nil {
					t.Fatal(err)
				}
				storeLegacyReviewingStatus(t, repo, lineage, snapshot)
			},
		},
		{
			name: "v3 authority",
			seed: func(t *testing.T, repo, lineage string) {
				store, err := NewLineageAuthorityStore(context.Background(), repo, lineage)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
					*next = fixtureNewLineageAuthority(lineage, NewLineageStateReviewing)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, format := range formats {
		format := format
		t.Run(format.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
			lineage := "existing-review-lineage"
			format.seed(t, repo, lineage)

			tests := []struct {
				name     string
				lineage  string
				wantErr  bool
				wantText string
			}{
				{
					name:    "existing lineage is honored",
					lineage: lineage,
					wantErr: false,
				},
				{
					name:     "well-formed nonexistent lineage refused",
					lineage:  "review-ffffffffffffffff",
					wantErr:  true,
					wantText: `review lineage "review-ffffffffffffffff" does not exist`,
				},
				{
					name:     "kebab-shaped junk refused",
					lineage:  "not-even-a-lineage-shape",
					wantErr:  true,
					wantText: `review lineage "not-even-a-lineage-shape" does not exist`,
				},
			}
			for _, tt := range tests {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					_, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
						Target:    Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
						LineageID: tt.lineage,
						// The pure status surface sets the opt-in existence
						// demand; a negotiated next-transition must keep
						// accepting a proposed lineage that has not been
						// created yet.
						RequireLineageExists: true,
					})
					if tt.wantErr {
						if err == nil {
							t.Fatalf("AssessTargetStatus(%q) = nil; want error", tt.lineage)
						}
						if !strings.Contains(err.Error(), tt.wantText) {
							t.Fatalf("AssessTargetStatus(%q) error = %q; want substring %q", tt.lineage, err, tt.wantText)
						}
						return
					}
					if err != nil {
						t.Fatalf("AssessTargetStatus(%q) error: %v", tt.lineage, err)
					}
				})
			}
		})
	}

	// Shape validation runs before existence discovery, so it is independent
	// of which authority format (if any) is stored.
	t.Run("malformed shape still refused by shape validation", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		writeSnapshotFile(t, repo, "tracked.txt", "candidate\n")
		_, err := AssessTargetStatus(context.Background(), repo, TargetStatusRequest{
			Target:               Target{Kind: TargetCurrentChanges, IntendedUntracked: []string{}},
			LineageID:            "Not-Valid/Lineage",
			RequireLineageExists: true,
		})
		if err == nil || !strings.Contains(err.Error(), "lineage_id must be") {
			t.Fatalf("AssessTargetStatus(malformed) error = %v; want shape-validation refusal", err)
		}
	})
}
