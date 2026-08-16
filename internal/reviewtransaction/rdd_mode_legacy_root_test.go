package reviewtransaction

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestLegacyCloneOverrideStillDecides is the migration guard for #2882.
//
// Moving the switch out of the review authority tree must never re-enable
// reviews for someone who already disabled them. An override written at the
// pre-#2882 location keeps deciding, because the read path falls back to it
// whenever the switch's own root holds no generation.
func TestLegacyCloneOverrideStillDecides(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)

	// Write through the current writer, then relocate the record to the
	// legacy path so the only thing under test is where reads look.
	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{}); err != nil {
		t.Fatalf("SetCloneLocalRDDMode() error = %v", err)
	}
	current, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := cloneLocalRDDModeLegacyRoot(ctx, repo)
	if err != nil {
		// The legacy tree is created lazily; build it with the same helpers.
		identity, idErr := cloneLocalRDDModeIdentity(ctx, repo)
		if idErr != nil {
			t.Fatal(idErr)
		}
		base := filepath.Join(identity.GitCommonDir, "gentle-ai", "review-transactions", rarAuthorityDirectory, rarAuthorityVersion)
		if err := ensureRARRepositoryRoot(identity.GitCommonDir, base, true); err != nil {
			t.Fatal(err)
		}
		legacy = filepath.Join(base, rddModeDirectory)
		if err := ensurePrivateRARDirectoryTree(base, legacy, true); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(current)
	if err != nil {
		t.Fatal(err)
	}
	moved := 0
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == rddModeLockName {
			continue
		}
		if renameErr := os.Rename(filepath.Join(current, entry.Name()), filepath.Join(legacy, entry.Name())); renameErr != nil {
			t.Fatal(renameErr)
		}
		moved++
	}
	if moved == 0 {
		t.Fatal("no override record to relocate; the writer stored nothing")
	}

	status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{})
	if err != nil {
		t.Fatalf("ResolveRDDMode() error = %v", err)
	}
	if status.Effective != RDDModeOff || status.Source != RDDModeSourceCloneLocal {
		t.Fatalf("a pre-#2882 clone override stopped deciding: %#v; the move silently re-enabled reviews", status)
	}
}

func TestLegacyCloneOverrideMigratesExactRevision(t *testing.T) {
	for _, test := range []struct {
		name      string
		legacy    RDDMode
		requested RDDMode
		wantMode  string
		enabled   bool
	}{
		{name: "clear legacy off", legacy: RDDModeOff, requested: RDDModeUnset, wantMode: rddModeOverrideInherit, enabled: true},
		{name: "disable legacy inherit", legacy: RDDModeUnset, requested: RDDModeOff, wantMode: string(RDDModeOff)},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := initSnapshotRepo(t)
			legacy, legacyPath, legacyBytes, current := legacyCloneOverrideForTest(t, ctx, repo, test.legacy)
			global := RDDGlobalMode{Value: "on"}

			status, err := ResolveRDDMode(ctx, repo, global)
			if err != nil || status.Revision != legacy.Revision {
				t.Fatalf("legacy status = %#v, %v", status, err)
			}
			if _, err := SetCloneLocalRDDMode(ctx, repo, test.requested, "stale", global); !errors.Is(err, ErrRDDModeRevisionMismatch) {
				t.Fatalf("stale legacy revision error = %v, want ErrRDDModeRevisionMismatch", err)
			}
			if head, err := cloneLocalRDDOverrideHeadGeneration(current); err != nil || head != 0 {
				t.Fatalf("stale revision published current generation %d, %v", head, err)
			}

			migrated, err := SetCloneLocalRDDMode(ctx, repo, test.requested, status.Revision, global)
			if err != nil {
				t.Fatalf("migrate legacy record: %v", err)
			}
			currentRecord, present, err := readCloneLocalRDDOverrideHead(current)
			if err != nil || !present || currentRecord.Generation != legacy.Generation+1 ||
				currentRecord.PreviousRevision != legacy.Revision || currentRecord.Mode != test.wantMode ||
				currentRecord.Revision != migrated.Revision {
				t.Fatalf("migrated current record = %#v, present=%v, err=%v", currentRecord, present, err)
			}
			if after, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(after, legacyBytes) {
				t.Fatalf("legacy forensic bytes changed: err=%v before=%q after=%q", err, legacyBytes, after)
			}
			if path, err := CloneLocalRDDModeRecordPath(ctx, repo); err != nil || path != filepath.Join(current, rddModeGenerationName(currentRecord.Generation)) {
				t.Fatalf("authoritative record path = %q, %v", path, err)
			}
			resolved, err := ResolveRDDMode(ctx, repo, global)
			if err != nil || resolved.Revision != migrated.Revision || resolved.Enabled() != test.enabled {
				t.Fatalf("post-migration status = %#v, %v", resolved, err)
			}
		})
	}
}

func TestLegacyCloneOverridePreservesNoopAndGlobalOffTruthfulness(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	legacy, legacyPath, legacyBytes, current := legacyCloneOverrideForTest(t, ctx, repo, RDDModeOff)

	status, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, legacy.Revision, RDDGlobalMode{Value: "on"})
	if err != nil || status.Revision != legacy.Revision {
		t.Fatalf("legacy no-op = %#v, %v", status, err)
	}
	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, "stale", RDDGlobalMode{Value: "off"}); !errors.Is(err, ErrRDDModeRevisionMismatch) {
		t.Fatalf("stale revision before global-off error = %v, want ErrRDDModeRevisionMismatch", err)
	}
	status, err = SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, legacy.Revision, RDDGlobalMode{Value: "off"})
	var disabled *RDDDisabledError
	if !errors.As(err, &disabled) || disabled.Source != RDDModeSourceGlobal || status.Revision != legacy.Revision {
		t.Fatalf("current revision under global-off = %#v, %v", status, err)
	}
	if head, err := cloneLocalRDDOverrideHeadGeneration(current); err != nil || head != 0 {
		t.Fatalf("no-op/global-off published current generation %d, %v", head, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(after, legacyBytes) {
		t.Fatalf("no-op/global-off changed legacy bytes: err=%v before=%q after=%q", err, legacyBytes, after)
	}
}

func TestCorruptLegacyCloneOverrideFailsClosedWithoutMigration(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	_, legacyPath, _, current := legacyCloneOverrideForTest(t, ctx, repo, RDDModeOff)
	corrupt := []byte("{not json}\n")
	if err := os.WriteFile(legacyPath, corrupt, 0o600); err != nil {
		t.Fatalf("corrupt legacy record: %v", err)
	}

	if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, "", RDDGlobalMode{Value: "on"}); !errors.Is(err, ErrRDDModeCorrupt) {
		t.Fatalf("corrupt legacy mutation error = %v, want ErrRDDModeCorrupt", err)
	}
	if head, err := cloneLocalRDDOverrideHeadGeneration(current); err != nil || head != 0 {
		t.Fatalf("corrupt legacy migration published current generation %d, %v", head, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(after, corrupt) {
		t.Fatalf("corrupt legacy bytes changed: err=%v before=%q after=%q", err, corrupt, after)
	}
}

func TestCurrentOverrideWinsOverLegacyRecords(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{string(RDDModeOff), rddModeOverrideInherit} {
		t.Run(mode, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			legacy, legacyPath, legacyBytes, current := legacyCloneOverrideForTest(t, ctx, repo, RDDModeOff)
			currentRecord := publishCurrentModeRecordForTest(t, current, legacy, mode)
			currentPath := filepath.Join(current, rddModeGenerationName(currentRecord.Generation))
			currentBytes, err := os.ReadFile(currentPath)
			if err != nil {
				t.Fatalf("read current record: %v", err)
			}

			status, err := ResolveRDDMode(ctx, repo, RDDGlobalMode{Value: "on"})
			if err != nil || status.Revision != currentRecord.Revision {
				t.Fatalf("current root was not authoritative: %#v, %v", status, err)
			}
			if _, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, legacy.Revision, RDDGlobalMode{Value: "on"}); !errors.Is(err, ErrRDDModeRevisionMismatch) {
				t.Fatalf("legacy revision replaced current authority: %v", err)
			}
			if after, err := os.ReadFile(currentPath); err != nil || !bytes.Equal(after, currentBytes) {
				t.Fatalf("stale legacy request changed current bytes: err=%v", err)
			}
			if after, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(after, legacyBytes) {
				t.Fatalf("stale legacy request changed legacy bytes: err=%v", err)
			}
		})
	}
}

func TestLegacyCloneOverrideConcurrentMigrationKeepsOneWinner(t *testing.T) {
	ctx := context.Background()
	repo := initSnapshotRepo(t)
	legacy, legacyPath, legacyBytes, current := legacyCloneOverrideForTest(t, ctx, repo, RDDModeOff)
	var group sync.WaitGroup
	var mutex sync.Mutex
	winners := 0
	for range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := SetCloneLocalRDDMode(ctx, repo, RDDModeUnset, legacy.Revision, RDDGlobalMode{Value: "on"})
			mutex.Lock()
			defer mutex.Unlock()
			if err == nil {
				winners++
				return
			}
			if !errors.Is(err, ErrRDDModeRevisionMismatch) {
				t.Errorf("losing migration error = %v", err)
			}
		}()
	}
	group.Wait()
	if winners != 1 {
		t.Fatalf("concurrent legacy migrations = %d winners, want 1", winners)
	}
	currentRecord, present, err := readCloneLocalRDDOverrideHead(current)
	if err != nil || !present || currentRecord.Generation != legacy.Generation+1 || currentRecord.PreviousRevision != legacy.Revision {
		t.Fatalf("concurrent migration record = %#v, present=%v, err=%v", currentRecord, present, err)
	}
	if after, err := os.ReadFile(legacyPath); err != nil || !bytes.Equal(after, legacyBytes) {
		t.Fatalf("concurrent migration changed legacy bytes: err=%v", err)
	}
}

func legacyCloneOverrideForTest(t *testing.T, ctx context.Context, repo string, mode RDDMode) (rddModeOverrideRecord, string, []byte, string) {
	t.Helper()
	status, err := SetCloneLocalRDDMode(ctx, repo, RDDModeOff, "", RDDGlobalMode{Value: "on"})
	if err != nil {
		t.Fatalf("seed current override: %v", err)
	}
	if mode == RDDModeUnset {
		status, err = SetCloneLocalRDDMode(ctx, repo, mode, status.Revision, RDDGlobalMode{Value: "on"})
		if err != nil {
			t.Fatalf("seed current inherit override: %v", err)
		}
	}
	current, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		t.Fatalf("current root: %v", err)
	}
	legacy, err := cloneLocalRDDModeLegacyRoot(ctx, repo)
	if err != nil {
		identity, identityErr := cloneLocalRDDModeIdentity(ctx, repo)
		if identityErr != nil {
			t.Fatal(identityErr)
		}
		base := filepath.Join(identity.GitCommonDir, "gentle-ai", "review-transactions", rarAuthorityDirectory, rarAuthorityVersion)
		if err := ensureRARRepositoryRoot(identity.GitCommonDir, base, true); err != nil {
			t.Fatal(err)
		}
		legacy = filepath.Join(base, rddModeDirectory)
		if err := ensurePrivateRARDirectoryTree(base, legacy, true); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(current)
	if err != nil {
		t.Fatalf("read current root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == rddModeLockName {
			continue
		}
		if _, ok := rddModeGenerationOf(entry.Name()); !ok {
			continue
		}
		if err := os.Rename(filepath.Join(current, entry.Name()), filepath.Join(legacy, entry.Name())); err != nil {
			t.Fatalf("relocate current record: %v", err)
		}
	}
	record, present, err := readCloneLocalRDDOverrideHead(legacy)
	if err != nil || !present || record.Revision != status.Revision {
		t.Fatalf("legacy record = %#v, present=%v, err=%v", record, present, err)
	}
	legacyPath := filepath.Join(legacy, rddModeGenerationName(record.Generation))
	payload, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy record: %v", err)
	}
	return record, legacyPath, payload, current
}

func publishCurrentModeRecordForTest(t *testing.T, current string, predecessor rddModeOverrideRecord, mode string) rddModeOverrideRecord {
	t.Helper()
	record := rddModeOverrideRecord{
		Schema:           rddModeOverrideSchema,
		Generation:       predecessor.Generation + 1,
		PreviousRevision: predecessor.Revision,
		Mode:             mode,
		RecordedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	var err error
	record.Revision, err = rddModeOverrideDigest(record)
	if err != nil {
		t.Fatalf("current record digest: %v", err)
	}
	payload, err := canonicalRDDModeOverridePayload(record)
	if err != nil {
		t.Fatalf("current record payload: %v", err)
	}
	if err := publishPrivateRARImmutable(filepath.Join(current, rddModeGenerationName(record.Generation)), payload); err != nil {
		t.Fatalf("publish current record: %v", err)
	}
	return record
}
