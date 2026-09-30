package managedstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/filecoord"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

// ---- fixtures -------------------------------------------------------------

func desiredBytes() []byte { return []byte("# Gentleman output style\nmanaged desired content\n") }

func oldBytes() []byte { return []byte("# Gentleman output style\nold committed content\n") }

func foreignBytes() []byte { return []byte("USER EDITED WHILE WE WERE NOT LOOKING\n") }

func digestHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func onePlan(home string) Plan {
	target := filepath.Join(home, "claude", "gentleman.md")
	return Plan{
		Producer: state.Producer{BinaryVersion: "test-binary", Commit: "testcommit"},
		Resources: []PlannedResource{{
			ID:      "persona/output-style/gentleman",
			Adapter: "claude",
			Target:  target,
			Desired: desiredBytes(),
			Mode:    0o644,
			Extent:  state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipManaged},
		}},
	}
}

func planTarget(p Plan) string { return p.Resources[0].Target }

// writeManagedManifest records the target as managed-full with the given
// committed desired/observed digests, mimicking a contract-aware install.
func writeManagedManifest(t *testing.T, home, target, desiredHex string) state.Manifest {
	t.Helper()
	m := state.Manifest{
		Schema:   state.ManifestSchema,
		Producer: state.Producer{BinaryVersion: "predecessor", Commit: "predcommit"},
		Resources: []state.ManifestResource{{
			ID:          "persona/output-style/gentleman",
			Adapter:     "claude",
			Target:      target,
			OwnedExtent: state.OwnedExtent{Kind: state.ExtentFull, Ownership: state.OwnershipManaged},
			Desired:     desiredHex,
			Observed:    desiredHex,
		}},
	}.WithBundleDigest()
	if err := state.WriteManifestAtomic(home, m); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return m
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func mustTargetMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// ---- T1: success publishes one record; second run is a semantic no-op ------

func TestRunSuccessThenSecondRunIsSemanticNoOp(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)

	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Status != "" || res.Phase != PhaseCompleted || res.NoOp {
		t.Fatalf("unexpected result: %+v", res)
	}
	if got := mustReadFile(t, target); string(got) != string(desiredBytes()) {
		t.Fatalf("target not exact desired: %q", got)
	}
	m, err := state.ReadManifest(home)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(m.Resources) != 1 {
		t.Fatalf("expected exactly one committed record, got %d", len(m.Resources))
	}
	r := m.Resources[0]
	if r.Desired != digestHex(desiredBytes()) || r.Observed != r.Desired {
		t.Fatalf("committed record wrong: %+v", r)
	}
	if m.Producer.BinaryVersion != "test-binary" {
		t.Fatalf("producer must be the migrating binary: %+v", m.Producer)
	}

	manifestBefore := mustReadFile(t, state.ManifestPath(home))
	res2, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !res2.NoOp || res2.TargetWrites != 0 || res2.ManifestWrites != 0 {
		t.Fatalf("second run must be a semantic no-op: %+v", res2)
	}
	if string(mustReadFile(t, state.ManifestPath(home))) != string(manifestBefore) {
		t.Fatalf("second run rewrote the manifest")
	}
}

// ---- T3: unknown ownership blocks before backup/apply, zero mutation -------

func TestRunBlocksUnknownOwnershipZeroMutation(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, foreignBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("blocked ownership is a recorded outcome, not an error: %v", err)
	}
	if res.Status != StatusBlockedUnknownOwnership {
		t.Fatalf("status: %+v", res)
	}
	if string(mustReadFile(t, target)) != string(foreignBytes()) {
		t.Fatalf("target must stay byte-identical")
	}
	if _, err := os.Stat(state.ManifestPath(home)); !os.IsNotExist(err) {
		t.Fatalf("no manifest may be created on blocked ownership")
	}
	j, err := LoadActiveJournal(home)
	if err != nil || j.Status != StatusBlockedUnknownOwnership {
		t.Fatalf("block must be durably recorded, journal=%+v err=%v", j, err)
	}
}

// ---- T4: crash before target replace resumes from exact before state -------

func TestCrashBeforeTargetReplaceResumesToCommitted(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	faults := Faults{BeforeTargetWrite: func(string) error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, faults); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected injected crash, got %v", err)
	}
	j, err := LoadActiveJournal(home)
	if err != nil {
		t.Fatalf("journal must survive the crash: %v", err)
	}
	if j.LastPhase != PhaseApplying {
		t.Fatalf("journal must be durable at applying before the write: %q", j.LastPhase)
	}
	if string(mustReadFile(t, target)) != string(oldBytes()) {
		t.Fatalf("target must still hold exact before bytes")
	}

	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Phase != PhaseCompleted || !res.Resumed {
		t.Fatalf("resume result: %+v", res)
	}
	if string(mustReadFile(t, target)) != string(desiredBytes()) {
		t.Fatalf("target must converge to desired")
	}
	if _, err := LoadActiveJournal(home); err != ErrNoActiveJournal {
		t.Fatalf("completed transaction must not survive as active journal: %v", err)
	}
}

// ---- T5: crash after replace commits without a second mutation -------------

func TestCrashAfterTargetReplaceCommitsWithoutRewrite(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	faults := Faults{AfterTargetWrite: func(string) error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, faults); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected injected crash, got %v", err)
	}
	if string(mustReadFile(t, target)) != string(desiredBytes()) {
		t.Fatalf("target already replaced")
	}

	writes := 0
	resume := Faults{AfterTargetWrite: func(string) error { writes++; return nil }}
	res, err := Run(context.Background(), home, plan, resume)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Phase != PhaseCompleted || !res.Resumed {
		t.Fatalf("resume result: %+v", res)
	}
	if res.TargetWrites != 0 || writes != 0 {
		t.Fatalf("restart must recognize exact desired bytes without rewriting: %+v writes=%d", res, writes)
	}
	if mustTargetMode(t, target) != plan.Resources[0].Mode {
		t.Fatalf("mode must converge to desired perm")
	}
}

// ---- T6a: foreign bytes during live run -> rollback_required ---------------

func TestForeignBytesDuringRunBlocksWithoutOverwrite(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestBefore := mustReadFile(t, state.ManifestPath(home))
	faults := Faults{AfterJournal: func(p Phase) error {
		if p == PhaseSnapshotted {
			if err := os.WriteFile(target, foreignBytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}}
	res, err := Run(context.Background(), home, plan, faults)
	if err != nil {
		t.Fatalf("foreign bytes are a recorded outcome: %v", err)
	}
	if res.Status != StatusRollbackRequired {
		t.Fatalf("status: %+v", res)
	}
	if string(mustReadFile(t, target)) != string(foreignBytes()) {
		t.Fatalf("never guess-overwrite foreign bytes")
	}
	if string(mustReadFile(t, state.ManifestPath(home))) != string(manifestBefore) {
		t.Fatalf("no manifest write on rollback_required")
	}
}

// ---- T6b: foreign bytes found on restart -> blocked_conflict ---------------

func TestForeignBytesOnRestartBlockedConflictZeroWrites(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	crash := Faults{BeforeTargetWrite: func(string) error {
		if err := os.WriteFile(target, foreignBytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return ErrInjected
	}}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash, got %v", err)
	}

	targetBefore := mustReadFile(t, target)
	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("restart outcome: %v", err)
	}
	if res.Status != StatusBlockedConflict {
		t.Fatalf("status: %+v", res)
	}
	if string(mustReadFile(t, target)) != string(targetBefore) {
		t.Fatalf("zero further writes on blocked_conflict")
	}
}

// ---- T7: verification failure restores exact before identity ---------------

func TestVerifyFailureRestoresExactBeforeAndRecordsRolledBack(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o640); err != nil {
		t.Fatal(err)
	}
	manifestBefore := mustReadFile(t, state.ManifestPath(home))

	faults := Faults{Verify: func(string) error { return ErrInjected }}
	res, err := Run(context.Background(), home, plan, faults)
	if err != nil {
		t.Fatalf("rolled back is a recorded outcome: %v", err)
	}
	if res.Status != StatusRolledBack {
		t.Fatalf("status: %+v", res)
	}
	if string(mustReadFile(t, target)) != string(oldBytes()) {
		t.Fatalf("restore must be byte-exact")
	}
	if mustTargetMode(t, target) != 0o640 {
		t.Fatalf("restore must recover the exact mode")
	}
	if string(mustReadFile(t, state.ManifestPath(home))) != string(manifestBefore) {
		t.Fatalf("manifest untouched by rollback")
	}

	// A terminal journal stays read-only: rerun reports it without writes.
	res2, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("terminal rerun: %v", err)
	}
	if res2.Status != StatusRolledBack || res2.TargetWrites != 0 || res2.ManifestWrites != 0 {
		t.Fatalf("terminal journal is read-only truth: %+v", res2)
	}
}

// ---- T8: schema gates are read-only and write nothing -----------------------

func TestNewerManifestSchemaIsReadOnlyZeroWrites(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	newer := state.Manifest{Schema: "gentle-ai.managed-assets/v2", Producer: state.Producer{BinaryVersion: "future"}}
	if err := state.WriteManifestAtomic(home, newer); err != nil {
		t.Fatal(err)
	}
	before := mustReadFile(t, state.ManifestPath(home))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), home, plan, Faults{})
	if !errors.Is(err, ErrUnsupportedNewerSchema) {
		t.Fatalf("expected ErrUnsupportedNewerSchema, got %v", err)
	}
	if string(mustReadFile(t, state.ManifestPath(home))) != string(before) {
		t.Fatalf("schema gate must not rewrite the manifest")
	}
	if string(mustReadFile(t, target)) != string(oldBytes()) {
		t.Fatalf("schema gate must not touch targets")
	}
	if _, err := os.Stat(TransactionsDir(home)); !os.IsNotExist(err) {
		t.Fatalf("schema gate must not create transactions")
	}
}

func TestNewerJournalSchemaIsReadOnlyZeroWrites(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	j := &Journal{Schema: "gentle-ai.migration-journal/v2", TransactionID: "tx-future", HomeDir: home, LastPhase: PhasePrepared}
	if err := j.persist(home); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), home, plan, Faults{})
	if !errors.Is(err, ErrUnsupportedNewerSchema) {
		t.Fatalf("expected ErrUnsupportedNewerSchema for newer journal, got %v", err)
	}
}

func TestInvalidManifestSchemaIsUnknownAuthority(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	bad := state.Manifest{Schema: "definitely-not-a-schema"}
	if err := state.WriteManifestAtomic(home, bad); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), home, plan, Faults{})
	if !errors.Is(err, ErrUnknownAuthority) {
		t.Fatalf("expected ErrUnknownAuthority, got %v", err)
	}
}

// ---- bounded acceptance 7: two writers yield one lock owner -----------------

func TestSecondWriterGetsTypedBusyAndWritesNothing(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)

	if err := os.MkdirAll(lockRoot(home), 0o755); err != nil {
		t.Fatal(err)
	}
	lease, err := filecoord.Acquire(context.Background(), lockTarget(home), lockRoot(home))
	if err != nil {
		t.Fatalf("hold lease: %v", err)
	}
	defer lease.Release()

	var busy *filecoord.BusyError
	_, err = Run(context.Background(), home, plan, Faults{})
	if !errors.As(err, &busy) {
		t.Fatalf("expected typed BusyError, got %v", err)
	}
	if _, errStat := os.Stat(TransactionsDir(home)); !os.IsNotExist(errStat) {
		t.Fatalf("contended run must not create transactions")
	}
}

// ---- bounded acceptance 9: state.json stays untouched ------------------------

func TestStateJSONIsByteIdenticalAfterSuccess(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	stateDir := filepath.Join(home, state.StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateJSON := []byte(`{"rdd_mode":"managed","model":"keep-me","nested":{"a":1}}` + "\n")
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), stateJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(context.Background(), home, plan, Faults{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(stateDir, "state.json")); string(got) != string(stateJSON) {
		t.Fatalf("state.json must stay byte-identical: %q", got)
	}
}

// ---- matrix 4 bounded: crash after manifest commit is bookkeeping only ------

func TestCrashAfterManifestCommitIsBookkeepingOnly(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)

	faults := Faults{AfterJournal: func(p Phase) error {
		if p == PhaseManifestCommitted {
			return ErrInjected
		}
		return nil
	}}
	if _, err := Run(context.Background(), home, plan, faults); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash after commit, got %v", err)
	}
	manifestAfterCommit := mustReadFile(t, state.ManifestPath(home))

	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("bookkeeping run: %v", err)
	}
	if res.Phase != PhaseCompleted || res.TargetWrites != 0 || res.ManifestWrites != 0 {
		t.Fatalf("bookkeeping only: %+v", res)
	}
	if string(mustReadFile(t, state.ManifestPath(home))) != string(manifestAfterCommit) {
		t.Fatalf("bookkeeping must not rewrite the manifest")
	}
	if _, err := os.Stat(TransactionsDir(home)); !os.IsNotExist(err) {
		t.Fatalf("completed transaction directory must be removed")
	}
	entries, err := state.ReadJournal(home)
	if err != nil || len(entries) == 0 {
		t.Fatalf("completion must append one audit record: %+v err=%v", entries, err)
	}
	if last := entries[len(entries)-1]; last.Op != state.OpComplete || last.RunID != res.TransactionID {
		t.Fatalf("audit record must name the transaction: %+v", last)
	}
}

// ---- stale CAS: manifest changed under us -> typed error, no write ----------

func TestStaleManifestCASRefusesToWrite(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	crash := Faults{BeforeManifestPublish: func() error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash before publish, got %v", err)
	}

	// A foreign manifest writer wins the generation while we are down.
	writeManagedManifest(t, home, target, digestHex(foreignBytes()))
	foreignManifest := mustReadFile(t, state.ManifestPath(home))

	_, err := Run(context.Background(), home, plan, Faults{})
	if !errors.Is(err, ErrStaleManifest) {
		t.Fatalf("expected ErrStaleManifest, got %v", err)
	}
	if string(mustReadFile(t, state.ManifestPath(home))) != string(foreignManifest) {
		t.Fatalf("stale CAS must not overwrite the newer manifest")
	}
}

// ---- T6c: foreign bytes on an absent-before target also refuse -------------

func TestForeignBytesOnAbsentBeforeTargetRollbackRequired(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)

	// Fresh install (absent target, no manifest); foreign bytes appear after
	// snapshot, before apply.
	faults := Faults{AfterJournal: func(p Phase) error {
		if p == PhaseSnapshotted {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, foreignBytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return nil
	}}
	res, err := Run(context.Background(), home, plan, faults)
	if err != nil {
		t.Fatalf("outcome: %v", err)
	}
	if res.Status != StatusRollbackRequired {
		t.Fatalf("status: %+v", res)
	}
	if string(mustReadFile(t, target)) != string(foreignBytes()) {
		t.Fatalf("never guess-overwrite bytes that appeared on an absent-before target")
	}
}

// ---- bounded acceptance 13 with mode: drifted mode is not a no-op ----------

func TestAlignedSecondRunConvergesDriftedMode(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)

	if _, err := Run(context.Background(), home, plan, Faults{}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.NoOp {
		t.Fatalf("a drifted mode is not a semantic no-op")
	}
	if res.TargetWrites != 1 {
		t.Fatalf("mode drift must be repaired by one write: %+v", res)
	}
	if mustTargetMode(t, target) != plan.Resources[0].Mode {
		t.Fatalf("mode must converge to desired perm")
	}
}

// ---- rule 5 resume branch: crash at verified publishes exactly once ---------

func TestResumeFromVerifiedPublishesManifestOnce(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)

	crash := Faults{AfterJournal: func(p Phase) error {
		if p == PhaseVerified {
			return ErrInjected
		}
		return nil
	}}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash at verified, got %v", err)
	}
	j, err := LoadActiveJournal(home)
	if err != nil || j.LastPhase != PhaseVerified {
		t.Fatalf("journal must sit at verified: %+v err=%v", j, err)
	}

	res, err := Run(context.Background(), home, plan, Faults{})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Phase != PhaseCompleted || res.ManifestWrites != 1 || res.TargetWrites != 0 {
		t.Fatalf("publish exactly once, rewrite nothing: %+v", res)
	}
}

// ---- rule 6/7 stale branch: committed manifest replaced during downtime -----

func TestBookkeepingStaleWhenCommittedManifestReplaced(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)

	crash := Faults{AfterJournal: func(p Phase) error {
		if p == PhaseManifestCommitted {
			return ErrInjected
		}
		return nil
	}}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash after commit, got %v", err)
	}
	// Another writer publishes a different generation while we are down.
	writeManagedManifest(t, home, target, digestHex(foreignBytes()))

	if _, err := Run(context.Background(), home, plan, Faults{}); !errors.Is(err, ErrStaleManifest) {
		t.Fatalf("expected ErrStaleManifest, got %v", err)
	}
}

// ---- rule 8 partial-restore branch stays rollback_required ------------------

func TestPartialRestoreStaysRollbackRequired(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// The before-image is destroyed after snapshot, so the explicit rollback
	// cannot restore and must stay rollback_required, never rolled_back.
	faults := Faults{
		AfterJournal: func(p Phase) error {
			if p == PhaseSnapshotted {
				matches, err := filepath.Glob(filepath.Join(TransactionsDir(home), "*", "snapshot", "*"))
				if err != nil || len(matches) == 0 {
					t.Fatalf("snapshot missing: %v", err)
				}
				os.Remove(matches[0])
			}
			return nil
		},
		Verify: func(string) error { return ErrInjected },
	}
	res, err := Run(context.Background(), home, plan, faults)
	if err != nil {
		t.Fatalf("outcome: %v", err)
	}
	if res.Status != StatusRollbackRequired {
		t.Fatalf("partial restore must stay rollback_required: %+v", res)
	}
}

// ---- rule 10: invalid journal schema is unknown authority -------------------

func TestInvalidJournalSchemaIsUnknownAuthority(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	j := &Journal{Schema: "not-a-journal-schema", TransactionID: "tx-bad", HomeDir: home, LastPhase: PhasePrepared}
	if err := j.persist(home); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), home, plan, Faults{})
	if !errors.Is(err, ErrUnknownAuthority) {
		t.Fatalf("expected ErrUnknownAuthority, got %v", err)
	}
}

// ---- rule 7: plan drift between runs is a stale proposal --------------------

func TestPlanDriftBetweenRunsIsStale(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	target := planTarget(plan)
	writeManagedManifest(t, home, target, digestHex(oldBytes()))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, oldBytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	crash := Faults{BeforeTargetWrite: func(string) error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash, got %v", err)
	}

	drifted := plan
	drifted.Resources[0].Desired = []byte("# drifted plan\n")
	_, err := Run(context.Background(), home, drifted, Faults{})
	if !errors.Is(err, ErrStaleManifest) {
		t.Fatalf("expected ErrStaleManifest on plan drift, got %v", err)
	}
}

// ---- resume alignment: journal identity wins over plan order ---------------

func twoPlan(home string) Plan {
	p := onePlan(home)
	second := p.Resources[0]
	second.ID = "persona/output-style/neutral"
	second.Target = filepath.Join(home, "claude", "neutral.md")
	second.Desired = []byte("# Neutral output style\nmanaged desired content\n")
	p.Resources = append(p.Resources, second)
	return p
}

func TestResumeWithReorderedPlanAlignsByID(t *testing.T) {
	home := t.TempDir()
	plan := twoPlan(home)

	crash := Faults{BeforeTargetWrite: func(string) error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash, got %v", err)
	}

	reversed := Plan{Producer: plan.Producer, Resources: []PlannedResource{plan.Resources[1], plan.Resources[0]}}
	res, err := Run(context.Background(), home, reversed, Faults{})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if res.Phase != PhaseCompleted || res.Status != "" {
		t.Fatalf("resume result: %+v", res)
	}
	for _, r := range plan.Resources {
		if got := mustReadFile(t, r.Target); string(got) != string(r.Desired) {
			t.Fatalf("resource %s must receive ITS desired bytes, got %q", r.ID, got)
		}
	}
	m, err := state.ReadManifest(home)
	if err != nil || len(m.Resources) != 2 {
		t.Fatalf("manifest must commit both resources: %+v err=%v", m.Resources, err)
	}
}

func TestResumeWithMissingResourceIDFailsClosed(t *testing.T) {
	home := t.TempDir()
	plan := twoPlan(home)
	crash := Faults{BeforeTargetWrite: func(string) error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash, got %v", err)
	}

	shrunk := Plan{Producer: plan.Producer, Resources: plan.Resources[:1]}
	_, err := Run(context.Background(), home, shrunk, Faults{})
	if err == nil || !strings.Contains(err.Error(), "absent from the plan") {
		t.Fatalf("missing journal resource must fail closed, got %v", err)
	}
	for _, r := range plan.Resources {
		if _, err := os.Stat(r.Target); !os.IsNotExist(err) {
			t.Fatalf("zero target writes on alignment failure: %s exists", r.Target)
		}
	}
}

func TestResumeWithChangedTargetFailsClosed(t *testing.T) {
	home := t.TempDir()
	plan := onePlan(home)
	crash := Faults{BeforeTargetWrite: func(string) error { return ErrInjected }}
	if _, err := Run(context.Background(), home, plan, crash); !errors.Is(err, ErrInjected) {
		t.Fatalf("expected crash, got %v", err)
	}

	moved := plan
	moved.Resources[0].Target = filepath.Join(home, "claude", "elsewhere.md")
	_, err := Run(context.Background(), home, moved, Faults{})
	if err == nil || !strings.Contains(err.Error(), "target changed") {
		t.Fatalf("target drift must fail closed, got %v", err)
	}
}
