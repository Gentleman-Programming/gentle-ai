package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestSDDAttemptRefusesRecreatedMarker(t *testing.T) {
	repo := initReviewCLIRepo(t)
	outsideRoot := filepath.Join(t.TempDir(), "source")
	writeSDDStatusFile(t, filepath.Join(outsideRoot, "main.go"), "package source\n")
	const change = "recreated-marker"
	tasks := "- [ ] 1.1 Update `" + filepath.Join(outsideRoot, "main.go") + "`\n"
	changeRoot := seedSDDStatusReadyChange(t, repo, change, tasks)

	var continued bytes.Buffer
	if err := RunSDDContinue([]string{change, "--cwd", repo, "--json"}, &continued); err != nil {
		t.Fatalf("RunSDDContinue() error = %v", err)
	}
	oldMarker, err := os.ReadFile(filepath.Join(changeRoot, ".gentle-ai-instance"))
	if err != nil {
		t.Fatal(err)
	}

	grantArgs := []string{
		"grant", "--cwd", repo, "--change", change, "--root", outsideRoot,
		"--change-instance", string(oldMarker), "--request-id", "grant-stale-marker",
		"--actor", "maintainer", "--reason", "test stale marker",
	}
	var granted bytes.Buffer
	if err := RunSDDAttempt(grantArgs, &granted); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(changeRoot); err != nil {
		t.Fatal(err)
	}
	changeRoot = seedSDDStatusReadyChange(t, repo, change, tasks)
	store, err := sddstatus.OpenRuntimeStore(t.Context(), repo, change)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	err = RunSDDAttempt(grantArgs, &stdout)
	if err == nil {
		t.Fatal("RunSDDAttempt(grant) error = nil, want stale-marker refusal")
	}
	if _, err := os.Stat(filepath.Join(changeRoot, ".gentle-ai-instance")); !os.IsNotExist(err) {
		t.Fatalf("stale grant initialized recreated marker: %v", err)
	}
	after, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.Attempts) != len(before.Attempts) || len(after.GrantedRoots) != len(before.GrantedRoots) {
		t.Fatalf("stale grant mutated authority: before=%#v after=%#v", before, after)
	}
	if before.Revision == "" {
		t.Fatal("A's grant was not retained")
	}
	continued.Reset()
	if err := RunSDDContinue([]string{change, "--cwd", repo, "--json"}, &continued); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(changeRoot, ".gentle-ai-instance")
	newMarker, err := os.ReadFile(markerPath)
	if err != nil || bytes.Equal(newMarker, oldMarker) {
		t.Fatalf("B did not get distinct persisted marker: %q %v", newMarker, err)
	}
	stdout.Reset()
	if err := RunSDDAttempt(grantArgs, &stdout); err == nil || stdout.Len() != 0 {
		t.Fatalf("A replay accepted against B: %v %s", err, stdout.String())
	}
	unchanged, err := os.ReadFile(markerPath)
	if err != nil || !bytes.Equal(unchanged, newMarker) {
		t.Fatal("stale replay changed B marker")
	}
	after, err = store.Status()
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("stale replay changed retained authority: %v", err)
	}
	bound, err := store.ForInstance(strings.TrimSpace(string(newMarker)))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := bound.Status()
	if err != nil || len(projection.GrantedRoots) != 0 {
		t.Fatal("B inherited A roots")
	}

}
