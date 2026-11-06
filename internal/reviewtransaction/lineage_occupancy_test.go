package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExactReviewLineageOccupiedChecksOnlyRequestedVersionedPaths(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		lineage string
		want    bool
	}{
		{name: "free despite unrelated sibling", lineage: "requested-free", want: false},
		{name: "v1", version: "v1", lineage: "requested-v1", want: true},
		{name: "v2", version: "v2", lineage: "requested-v2", want: true},
		{name: "v3", version: "v3", lineage: "requested-v3", want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			root, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "v2", "unrelated-sibling"), 0o755); err != nil {
				t.Fatal(err)
			}
			if test.version != "" {
				if err := os.MkdirAll(filepath.Join(root, test.version, test.lineage), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ExactReviewLineageOccupied(context.Background(), repo, test.lineage)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("ExactReviewLineageOccupied(%q) = %t, want %t", test.lineage, got, test.want)
			}
		})
	}
}
