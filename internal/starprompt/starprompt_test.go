package starprompt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadMissingFileIsFreshDefaults(t *testing.T) {
	res := Read(t.TempDir())
	if res.Unusable || !res.Missing {
		t.Fatalf("missing file must be fresh and usable: %+v", res)
	}
	if res.State.CompletedOps != 0 || res.State.Dismissed || res.State.DirectStar {
		t.Fatalf("unexpected defaults: %+v", res.State)
	}
}

func TestReadMalformedFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Read(dir)
	if !res.Malformed || !res.Unusable {
		t.Fatalf("malformed must fail closed: %+v", res)
	}
}

func TestReadWrongSchemaFailsClosed(t *testing.T) {
	dir := t.TempDir()
	data, _ := json.Marshal(State{Schema: "other/v9", CompletedOps: 3})
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if res := Read(dir); !res.Malformed || !res.Unusable {
		t.Fatalf("wrong schema must fail closed: %+v", res)
	}
}

func TestReadValidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := State{Schema: Schema, CompletedOps: 7, Dismissed: true, DirectStar: true}
	if err := Write(dir, want); err != nil {
		t.Fatal(err)
	}
	res := Read(dir)
	if res.Unusable || res.State != want {
		t.Fatalf("round trip mismatch: %+v vs %+v", res.State, want)
	}
}

func TestShouldOffer(t *testing.T) {
	base := State{Schema: Schema, CompletedOps: Threshold}
	cases := []struct {
		name                       string
		s                          State
		unusable, offered, running bool
		want                       bool
	}{
		{"silent through threshold", base, false, false, false, false},
		{"eligible after threshold", func() State { s := base; s.CompletedOps = Threshold + 1; return s }(), false, false, false, true},
		{"dismissed never", func() State { s := base; s.CompletedOps = 50; s.Dismissed = true; return s }(), false, false, false, false},
		{"unusable never", func() State { s := base; s.CompletedOps = 50; return s }(), true, false, false, false},
		{"already offered", func() State { s := base; s.CompletedOps = 50; return s }(), false, true, false, false},
		{"operation running", func() State { s := base; s.CompletedOps = 50; return s }(), false, false, true, false},
	}
	for _, tc := range cases {
		if got := ShouldOffer(tc.s, tc.unusable, tc.offered, tc.running); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestGhClassification(t *testing.T) {
	ctx := context.Background()
	yes := func(ctx context.Context, ghPath string, args ...string) (int, string, error) { return 0, "", nil }
	if got := GhStarred(ctx, yes, "gh", []string{"x/y"}); got != OutcomeYes {
		t.Errorf("exit 0 must be yes, got %v", got)
	}
	notFound := func(ctx context.Context, ghPath string, args ...string) (int, string, error) {
		return 1, "gh: Not Found (HTTP 404)", assertErr{}
	}
	if got := GhStarred(ctx, notFound, "gh", []string{"x/y"}); got != OutcomeNo {
		t.Errorf("404 must be no, got %v", got)
	}
	broken := func(ctx context.Context, ghPath string, args ...string) (int, string, error) {
		return -1, "", context.DeadlineExceeded
	}
	if got := GhStarred(ctx, broken, "gh", []string{"x/y"}); got != OutcomeUnknown {
		t.Errorf("spawn failure must be unknown, got %v", got)
	}
	other := func(ctx context.Context, ghPath string, args ...string) (int, string, error) {
		return 1, "HTTP 403", assertErr{}
	}
	if got := GhStarred(ctx, other, "gh", []string{"x/y"}); got != OutcomeUnknown {
		t.Errorf("403 must be unknown, got %v", got)
	}
	if got := GhStar(ctx, yes, "gh", []string{"x/y"}); got != OutcomeYes {
		t.Errorf("star exit 0 must be yes, got %v", got)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "failed" }

// Compile-time guard: DefaultGhRunner must stay assignable to GhRunner.
var _ GhRunner = DefaultGhRunner

func TestIdleAndVisibleConstantsMatchContract(t *testing.T) {
	if IdleDelay != 30*time.Second || VisibleTimeout != 5*time.Minute || Threshold != 5 {
		t.Fatalf("constants drifted from the documented courtesies")
	}
}
