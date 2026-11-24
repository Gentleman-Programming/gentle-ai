package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v5/internal/reviewtransaction"
)

// A repository that still carries a shipped legacy-v1 review store keeps
// working: status, inspect-authority and start behave as if v1 were absent,
// report nothing about it, and delete nothing.
func TestReviewCommandsIgnoreLegacyV1StoreAndDeleteNothing(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	legacy := addPristineLegacyAuthority(t, repo, "legacy-v1-bystander")
	broken := filepath.Join(filepath.Dir(legacy.Dir), "legacy-v1-broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "HEAD"), []byte("not-a-revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v1Root := filepath.Dir(legacy.Dir)
	before := readLegacyAuthorityTree(t, v1Root)

	var status bytes.Buffer
	if err := RunReview([]string{"status", "--cwd", repo}, &status); err != nil {
		t.Fatalf("status with a v1 store: %v\n%s", err, status.String())
	}
	if strings.Contains(status.String(), "legacy-v1") || strings.Contains(status.String(), "legacy-v1-bystander") {
		t.Fatalf("status mentions the v1 store:\n%s", status.String())
	}

	var inspect bytes.Buffer
	if err := RunReview([]string{"inspect-authority", "--cwd", repo}, &inspect); err != nil {
		t.Fatalf("inspect-authority with a v1 store: %v\n%s", err, inspect.String())
	}
	if strings.Contains(inspect.String(), "legacy-v1") {
		t.Fatalf("inspect-authority mentions the v1 store:\n%s", inspect.String())
	}

	started := startFacadeReview(t, repo)
	if started.LineageID == "" {
		t.Fatalf("start with a v1 store produced no lineage: %#v", started)
	}
	compact, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, started.LineageID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compact.Load(); err != nil {
		t.Fatalf("start did not create compact authority: %v", err)
	}
	if after := readLegacyAuthorityTree(t, v1Root); !reflect.DeepEqual(after, before) {
		t.Fatal("status, inspect or start modified the v1 store")
	}
}
