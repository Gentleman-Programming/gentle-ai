package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A reclaim record committed by a retired legacy-v1 repair provider is closed
// history: it still decodes, survives a re-encode with its proof bytes intact,
// and never gets in the way of live authority -- nothing re-derives it.
func TestCommittedLegacyRepairRecordsStayOpaqueReadableHistory(t *testing.T) {
	repo := initSnapshotRepo(t)
	base, _, err := reviewAuthorityRoot(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	quarantine := filepath.Join(base, "quarantine", "legacy-alias-old-20260101T000000Z")
	if err := os.MkdirAll(filepath.Join(quarantine, "residue"), 0o755); err != nil {
		t.Fatal(err)
	}
	const proofs = `"malformed_legacy_freeze":{"lineage_id":"legacy-old","operation":"review/freeze-findings"},` +
		`"legacy_alias_repair":{"lineage_id":"legacy-old","head_revision":"sha256:` +
		`aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","disposition":"quarantine-approved-historical-alias","operations":["review/validate-fix"]},` +
		`"legacy_fix_scope_quarantine":{"lineage_id":"legacy-old","anomalies":[{"code":"complete_fix_scope_expansion"}]}`
	payload := []byte(`{"schema":"gentle-ai.review-reclaim-record/v1","status":"committed","lineage_id":"legacy-old",` +
		`"reason":"retire","actor":"maintainer@example.com","reclaimed_at":"2026-01-01T00:00:00Z",` +
		`"source_path":"/gone","quarantine_path":"` + filepath.ToSlash(quarantine) + `","residue":["HEAD"],` + proofs + "}\n")
	recordPath := filepath.Join(quarantine, "reclaim-record.json")
	if err := os.WriteFile(recordPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	var record CompactReclaimRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		t.Fatalf("committed legacy repair record no longer decodes: %v", err)
	}
	if record.Status != CompactReclaimCommitted || record.LineageID != "legacy-old" ||
		len(record.LegacyAliasRepair) == 0 || len(record.MalformedLegacyFreeze) == 0 || len(record.LegacyFixScopeQuarantine) == 0 {
		t.Fatalf("decoded record lost its legacy proofs: %#v", record)
	}
	reencoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, proof := range [][]byte{record.LegacyAliasRepair, record.MalformedLegacyFreeze, record.LegacyFixScopeQuarantine} {
		if !bytes.Contains(reencoded, proof) {
			t.Fatalf("re-encoding changed an opaque proof %s in %s", proof, reencoded)
		}
	}
	if !quarantineDirectoryLineageMatches(filepath.Join(base, "quarantine"), filepath.Base(quarantine), "legacy-old") {
		t.Fatal("quarantine discovery no longer reads the committed legacy record")
	}

	// Live authority is unaffected: inventory is clean of it and a different
	// lineage still abandons beside it, leaving the record byte-identical.
	approvedCompactFixture(t, repo, "legacy-history-live")
	record2, _ := pristineReviewingFixture(t, repo, "legacy-history-abandon")
	if _, err := abandonAuthorized(t, context.Background(), repo, abandonFixtureRequest(record2)); err != nil {
		t.Fatalf("abandon beside committed legacy history: %v", err)
	}
	report, err := InventoryAuthority(context.Background(), repo)
	if err != nil || !report.Complete || !report.Authoritative {
		t.Fatalf("inventory beside committed legacy history = %#v, %v", report, err)
	}
	after, err := os.ReadFile(recordPath)
	if err != nil || !bytes.Equal(after, payload) {
		t.Fatalf("committed legacy record changed: %v", err)
	}
}
