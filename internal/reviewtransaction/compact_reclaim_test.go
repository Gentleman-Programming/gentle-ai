package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInventoryClassifiesIncompleteCompactStoreEntry(t *testing.T) {
	for _, tt := range []struct {
		name     string
		populate func(t *testing.T, dir string)
		status   AuthorityStatus
	}{
		{name: "empty entry", populate: func(*testing.T, string) {}, status: AuthorityStatusIncomplete},
		{name: "non-authoritative residue", populate: func(t *testing.T, dir string) {
			writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")
		}, status: AuthorityStatusIncomplete},
		{name: "orphan receipt", populate: func(t *testing.T, dir string) {
			writeReclaimFixtureFile(t, filepath.Join(dir, "review-receipt.json"), "{}\n")
		}, status: AuthorityStatusInvalid},
		{name: "orphan finalize journal", populate: func(t *testing.T, dir string) {
			writeReclaimFixtureFile(t, filepath.Join(dir, "finalize-attempt-journal.json"), "{}\n")
		}, status: AuthorityStatusInvalid},
		{name: "orphan reviewer results", populate: func(t *testing.T, dir string) {
			writeReclaimFixtureFile(t, filepath.Join(dir, "reviewer-results", "01-review-readability.json"), "{}\n")
		}, status: AuthorityStatusInvalid},
		{name: "interrupted atomic write", populate: func(t *testing.T, dir string) {
			writeReclaimFixtureFile(t, filepath.Join(dir, ".atomic-partial"), "partial")
		}, status: AuthorityStatusReset},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			root, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "v2", "reclaim-audit")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			tt.populate(t, dir)

			report, err := InventoryAuthority(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			if report.Complete || report.Authoritative {
				t.Fatalf("incomplete store entry left inventory authoritative: %#v", report)
			}
			if !hasAuthorityInventoryStatus(report.Entries, "reclaim-audit", tt.status) {
				t.Fatalf("inventory entries = %#v", report.Entries)
			}
		})
	}
}

func TestReclaimIncompleteCompactStoreQuarantinesResidueWithAuditRecord(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "v2", "reclaim-audit")
	writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")

	record, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "reclaim-audit", Reason: "interrupted store write residue", Actor: "maintainer@example.com",
	})
	if err != nil {
		t.Fatalf("reclaim incomplete store entry: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("reclaimed store entry still present: %v", statErr)
	}
	if record.Schema != CompactReclaimRecordSchema || record.Status != CompactReclaimCommitted ||
		record.LineageID != "reclaim-audit" ||
		record.Reason != "interrupted store write residue" || record.Actor != "maintainer@example.com" ||
		record.ReclaimedAt.IsZero() || record.SourcePath != dir {
		t.Fatalf("reclaim record = %#v", record)
	}
	quarantineRoot := filepath.Join(root, "quarantine")
	if !strings.HasPrefix(record.QuarantinePath, quarantineRoot+string(os.PathSeparator)) {
		t.Fatalf("quarantine path %q is outside %q", record.QuarantinePath, quarantineRoot)
	}
	moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "stray.tmp"))
	if err != nil || !bytes.Equal(moved, []byte("stray\n")) {
		t.Fatalf("quarantined residue bytes = %q, %v", moved, err)
	}
	if len(record.Residue) != 1 || record.Residue[0] != "stray.tmp" {
		t.Fatalf("reclaim residue manifest = %#v", record.Residue)
	}
	payload, err := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatalf("read persisted reclaim audit record: %v", err)
	}
	var persisted CompactReclaimRecord
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatalf("parse persisted reclaim audit record: %v", err)
	}
	if persisted.Schema != CompactReclaimRecordSchema || persisted.Status != CompactReclaimCommitted ||
		persisted.LineageID != record.LineageID ||
		persisted.Actor != record.Actor || persisted.Reason != record.Reason || persisted.ReclaimedAt.IsZero() {
		t.Fatalf("persisted reclaim record = %#v", persisted)
	}

	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || !report.Authoritative || hasAuthorityInventoryStatus(report.Entries, "reclaim-audit", AuthorityStatusIncomplete) {
		t.Fatalf("post-reclaim inventory = %#v", report)
	}
}

func TestReclaimRefusesAuthoritativeArtifactsAndInvalidRequests(t *testing.T) {
	for _, tt := range []struct {
		name     string
		artifact string
	}{
		{name: "state file", artifact: "review-state.json"},
		{name: "receipt", artifact: "review-receipt.json"},
		{name: "finalize journal", artifact: "finalize-attempt-journal.json"},
		{name: "reviewer results", artifact: filepath.Join("reviewer-results", "01-review-readability.json")},
		{name: "interrupted atomic write", artifact: ".atomic-partial"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			root, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "v2", "reclaim-audit")
			path := filepath.Join(dir, tt.artifact)
			writeReclaimFixtureFile(t, path, "artifact\n")

			_, err = ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
				LineageID: "reclaim-audit", Reason: "residue", Actor: "maintainer",
			})
			if err == nil {
				t.Fatal("reclaim touched a store entry holding authority artifacts")
			}
			payload, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(payload, []byte("artifact\n")) {
				t.Fatalf("refused reclaim mutated the store entry: %q, %v", payload, readErr)
			}
		})
	}

	t.Run("missing entry", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
			LineageID: "reclaim-absent", Reason: "residue", Actor: "maintainer",
		}); err == nil {
			t.Fatal("reclaim accepted a missing store entry")
		}
	})

	t.Run("missing reason and actor", func(t *testing.T) {
		repo := initSnapshotRepo(t)
		root, _, err := reviewAuthorityRoot(context.Background(), repo)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "v2", "reclaim-audit"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, request := range []CompactReclaimRequest{
			{LineageID: "reclaim-audit", Actor: "maintainer"},
			{LineageID: "reclaim-audit", Reason: "residue"},
			{LineageID: "Not A Lineage", Reason: "residue", Actor: "maintainer"},
		} {
			if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, request); err == nil {
				t.Fatalf("reclaim accepted invalid request %#v", request)
			}
		}
	})
}

func TestReclaimCrashBeforeRenameLeavesPreparedOrphanAndRetrySucceeds(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "v2", "reclaim-audit")
	writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")

	original := reclaimQuarantineResidue
	reclaimQuarantineResidue = func(string, string) error {
		return errors.New("simulated crash before residue rename")
	}
	t.Cleanup(func() { reclaimQuarantineResidue = original })
	request := CompactReclaimRequest{LineageID: "reclaim-audit", Reason: "interrupted store write residue", Actor: "maintainer@example.com"}
	prepared, err := ReclaimIncompleteCompactStore(context.Background(), repo, request)
	if err == nil {
		t.Fatal("interrupted reclaim reported success")
	}
	if prepared.Status != CompactReclaimPrepared || prepared.QuarantinePath == "" {
		t.Fatalf("rename-failure record = %#v", prepared)
	}
	if !strings.Contains(err.Error(), prepared.QuarantinePath) {
		t.Fatalf("rename failure hides the prepared quarantine location: %v", err)
	}
	payload, err := os.ReadFile(filepath.Join(dir, "stray.tmp"))
	if err != nil || !bytes.Equal(payload, []byte("stray\n")) {
		t.Fatalf("interrupted reclaim mutated the source entry: %q, %v", payload, err)
	}
	orphans, err := os.ReadDir(filepath.Join(root, "quarantine"))
	if err != nil || len(orphans) != 1 {
		t.Fatalf("orphan quarantine directories = %#v, %v", orphans, err)
	}
	var orphan CompactReclaimRecord
	orphanPayload, err := os.ReadFile(filepath.Join(root, "quarantine", orphans[0].Name(), "reclaim-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(orphanPayload, &orphan); err != nil {
		t.Fatal(err)
	}
	if orphan.Status != CompactReclaimPrepared {
		t.Fatalf("orphan reclaim record status = %q", orphan.Status)
	}
	if _, err := os.Stat(filepath.Join(root, "quarantine", orphans[0].Name(), "residue")); !os.IsNotExist(err) {
		t.Fatalf("orphan quarantine claims residue it never received: %v", err)
	}

	reclaimQuarantineResidue = original
	record, err := ReclaimIncompleteCompactStore(context.Background(), repo, request)
	if err != nil {
		t.Fatalf("retry after interrupted reclaim: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.QuarantinePath == filepath.Join(root, "quarantine", orphans[0].Name()) {
		t.Fatalf("retry reclaim record = %#v", record)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("retried reclaim left the source entry behind: %v", err)
	}
	moved, err := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "stray.tmp"))
	if err != nil || !bytes.Equal(moved, []byte("stray\n")) {
		t.Fatalf("retried quarantine residue bytes = %q, %v", moved, err)
	}
	var committed CompactReclaimRecord
	committedPayload, err := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(committedPayload, &committed); err != nil {
		t.Fatal(err)
	}
	if committed.Status != CompactReclaimCommitted {
		t.Fatalf("persisted retry reclaim record status = %q", committed.Status)
	}
}

func TestReclaimPreparedRecordFailureRemovesUnidentifiedQuarantine(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "v2", "reclaim-audit")
	writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")

	original := persistReclaimRecord
	persistReclaimRecord = func(CompactReclaimRecord) error {
		return errors.New("simulated prepared record failure")
	}
	t.Cleanup(func() { persistReclaimRecord = original })

	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "reclaim-audit", Reason: "residue", Actor: "maintainer",
	}); err == nil {
		t.Fatal("failed prepared record write reported success")
	}
	entries, err := os.ReadDir(filepath.Join(root, "quarantine"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("prepared record failure left unidentified quarantine entries: %#v", entries)
	}
	if payload, err := os.ReadFile(filepath.Join(dir, "stray.tmp")); err != nil || !bytes.Equal(payload, []byte("stray\n")) {
		t.Fatalf("prepared record failure mutated source residue: %q, %v", payload, err)
	}
}

func TestReclaimRefusesSymlinkedQuarantineRoot(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "v2", "reclaim-audit")
	writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "quarantine")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
		LineageID: "reclaim-audit", Reason: "residue", Actor: "maintainer",
	}); err == nil {
		t.Fatal("reclaim followed a symlinked quarantine root")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("reclaim escaped through quarantine symlink: %#v", entries)
	}
}

func TestReclaimQuarantineCreationSyncFailureLeavesNoUnidentifiedDirectory(t *testing.T) {
	for _, failedDirectory := range []string{"authority root", "quarantine root"} {
		t.Run(failedDirectory, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			root, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "v2", "reclaim-audit")
			writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")

			originalSync := syncReviewDirectory
			quarantineRootSyncs := 0
			syncReviewDirectory = func(path string) error {
				if path == filepath.Join(root, "quarantine") {
					quarantineRootSyncs++
				}
				if failedDirectory == "authority root" && path == root ||
					failedDirectory == "quarantine root" && path == filepath.Join(root, "quarantine") && quarantineRootSyncs == 1 {
					return errors.New("simulated directory sync failure")
				}
				return nil
			}
			t.Cleanup(func() { syncReviewDirectory = originalSync })

			if _, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
				LineageID: "reclaim-audit", Reason: "residue", Actor: "maintainer",
			}); err == nil {
				t.Fatal("quarantine creation sync failure reported success")
			}
			entries, err := os.ReadDir(filepath.Join(root, "quarantine"))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("quarantine creation sync failure left unidentified entries: %#v", entries)
			}
		})
	}
}

func TestReclaimRenameSyncFailureLeavesPreparedRecord(t *testing.T) {
	for _, tt := range []struct {
		name       string
		failedPath func(root, quarantineDir string) string
	}{
		{name: "source version root", failedPath: func(root, _ string) string { return filepath.Join(root, "v2") }},
		{name: "quarantine directory", failedPath: func(_ string, quarantineDir string) string { return quarantineDir }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			root, _, err := reviewAuthorityRoot(context.Background(), repo)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "v2", "reclaim-audit")
			writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")

			originalSync := syncReviewDirectory
			var quarantineDir string
			quarantineDirSyncs := 0
			syncReviewDirectory = func(path string) error {
				if strings.HasPrefix(path, filepath.Join(root, "quarantine")+string(os.PathSeparator)) {
					quarantineDir = path
					quarantineDirSyncs++
				}
				if quarantineDir != "" && path == tt.failedPath(root, quarantineDir) &&
					(path != quarantineDir || quarantineDirSyncs > 1) {
					return errors.New("simulated rename directory sync failure")
				}
				return nil
			}
			t.Cleanup(func() { syncReviewDirectory = originalSync })

			record, err := ReclaimIncompleteCompactStore(context.Background(), repo, CompactReclaimRequest{
				LineageID: "reclaim-audit", Reason: "residue", Actor: "maintainer",
			})
			if err == nil {
				t.Fatal("rename directory sync failure reported success")
			}
			if record.Status != CompactReclaimPrepared || record.QuarantinePath == "" {
				t.Fatalf("rename sync failure record = %#v", record)
			}
			payload, readErr := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var persisted CompactReclaimRecord
			if err := json.Unmarshal(payload, &persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.Status != CompactReclaimPrepared {
				t.Fatalf("rename sync failure published status %q", persisted.Status)
			}
		})
	}
}

func TestReclaimCommitRewriteFailureReturnsPreparedRecordWithQuarantine(t *testing.T) {
	repo := initSnapshotRepo(t)
	root, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "v2", "reclaim-audit")
	writeReclaimFixtureFile(t, filepath.Join(dir, "stray.tmp"), "stray\n")

	original := persistReclaimRecord
	persistReclaimRecord = func(record CompactReclaimRecord) error {
		if record.Status == CompactReclaimCommitted {
			return errors.New("simulated crash before committed rewrite")
		}
		return original(record)
	}
	t.Cleanup(func() { persistReclaimRecord = original })
	request := CompactReclaimRequest{LineageID: "reclaim-audit", Reason: "interrupted store write residue", Actor: "maintainer@example.com"}
	record, err := ReclaimIncompleteCompactStore(context.Background(), repo, request)
	if err == nil {
		t.Fatal("failed committed rewrite reported success")
	}
	if record.Status != CompactReclaimPrepared || record.QuarantinePath == "" ||
		len(record.Residue) != 1 || record.Residue[0] != "stray.tmp" {
		t.Fatalf("committed-rewrite failure record = %#v", record)
	}
	if !strings.Contains(err.Error(), record.QuarantinePath) {
		t.Fatalf("committed-rewrite failure hides the quarantine location: %v", err)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatalf("committed-rewrite failure left the source entry behind: %v", statErr)
	}
	moved, readErr := os.ReadFile(filepath.Join(record.QuarantinePath, "residue", "stray.tmp"))
	if readErr != nil || !bytes.Equal(moved, []byte("stray\n")) {
		t.Fatalf("quarantined residue bytes = %q, %v", moved, readErr)
	}
	var persisted CompactReclaimRecord
	payload, readErr := os.ReadFile(filepath.Join(record.QuarantinePath, "reclaim-record.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Status != CompactReclaimPrepared {
		t.Fatalf("persisted record after failed committed rewrite = %#v", persisted)
	}

	// The source entry is gone, so the only reconciliation surface is the
	// quarantine itself; a retry correctly refuses with the missing-entry error.
	persistReclaimRecord = original
	if _, retryErr := ReclaimIncompleteCompactStore(context.Background(), repo, request); !errors.Is(retryErr, os.ErrNotExist) {
		t.Fatalf("retry after quarantined residue = %v", retryErr)
	}
}

func writeReclaimFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
