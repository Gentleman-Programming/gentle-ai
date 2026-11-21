package reviewtransaction

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Committed classified-repair audits are opaque closed history: they are
// preserved byte for byte and never re-derived, so a malformed historical
// field no longer blocks reading the record (gentle-ai#6203).
func TestCompactReclaimRecordPreservesHistoricalClassifiedAudit(t *testing.T) {
	audit := `{"schema":"gentle-ai.review-classified-authority-repair-audit/v1","route":"classified_authority_repair","assessment":{"schema":"gentle-ai.review-authority-repair-assessment/v1","status":"eligible","counts":{"lineages":1}},"assessment_digest":"sha256:dd","request_digest":"sha256:ee"}`
	for name, field := range map[string]string{"well-formed": audit, "malformed": `"not-an-object"`} {
		payload := []byte(`{"schema":"gentle-ai.review-reclaim-record/v1","status":"committed","lineage_id":"old","classified_authority_repair":` + field + `}`)
		var record CompactReclaimRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(record.ClassifiedAuthorityRepair, []byte(field)) {
			t.Fatalf("%s: audit not byte-preserved: %s", name, record.ClassifiedAuthorityRepair)
		}
		out, err := json.Marshal(record)
		if err != nil || !bytes.Contains(out, []byte(`"classified_authority_repair":`+field)) {
			t.Fatalf("%s: re-encode lost audit: %v %s", name, err, out)
		}
	}
}
