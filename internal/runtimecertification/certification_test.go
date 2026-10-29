package runtimecertification

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvaluateOpenCodeDebugAgentCertification(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	expected := OpenCodeDebugAgentTuple("1.18.5", "sha256:binary", "darwin", "arm64", "gentle-orchestrator", []string{"sha256:agent"})
	base := recordFor(expected, now.Add(time.Hour), OracleStatusPass, `{"name":"gentle-orchestrator"}`)

	tests := map[string]struct {
		record Record
		policy Policy
		want   State
	}{
		"matching pass certifies current identity":      {base, Policy{Expected: expected, Now: now}, StateCertifiedCurrent},
		"written evidence remains lower evidence":       {withEvidenceKind(base, EvidenceWritten), Policy{Expected: expected, Now: now}, StateWritten},
		"version range mismatch is stale":               {withClientVersionRange(base, "1.18.x"), Policy{Expected: expected, Now: now}, StateStale},
		"missing input digest fails closed":             {withInputDigests(base, nil), Policy{Expected: expected, Now: now}, StateFailed},
		"blank input digest fails closed":               {withInputDigests(base, []string{" \t "}), Policy{Expected: expected, Now: now}, StateFailed},
		"missing evidence boundary digest fails closed": {withEvidenceBoundaryDigests(base, nil), Policy{Expected: expected, Now: now}, StateFailed},
		"blank evidence boundary digest fails closed":   {withEvidenceBoundaryDigests(base, []string{"\n"}), Policy{Expected: expected, Now: now}, StateFailed},
		"expired record has explicit stale detail":      {recordFor(expected, now, OracleStatusPass, `{"name":"gentle-orchestrator"}`), Policy{Expected: expected, Now: now}, StateStale},
		"revoked oracle fails closed":                   {base, Policy{Expected: expected, Now: now, Revocations: []Revocation{{OracleID: OracleOpenCodeDebugAgent}}}, StateRevoked},
		"revoked expired oracle returns revoked":        {recordFor(expected, now, OracleStatusPass, `{"name":"gentle-orchestrator"}`), Policy{Expected: expected, Now: now, Revocations: []Revocation{{OracleID: OracleOpenCodeDebugAgent}}}, StateRevoked},
		"inconclusive oracle is not pass":               {recordFor(expected, now.Add(time.Hour), OracleStatusInconclusive, `{"name":"gentle-orchestrator"}`), Policy{Expected: expected, Now: now}, StateInconclusive},
		"malformed oracle JSON is failed":               {recordFor(expected, now.Add(time.Hour), OracleStatusPass, `{`), Policy{Expected: expected, Now: now}, StateFailed},
		"subject mismatch is failed":                    {recordFor(expected, now.Add(time.Hour), OracleStatusPass, `{"name":"other"}`), Policy{Expected: expected, Now: now}, StateFailed},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := Evaluate(tt.record, tt.policy)
			if got.State != tt.want {
				t.Fatalf("state = %s, want %s; detail: %s", got.State, tt.want, got.Detail)
			}
			if name == "expired record has explicit stale detail" && got.Detail != "runtime certification expired for the current identity" {
				t.Fatalf("detail = %q", got.Detail)
			}
		})
	}
}

func recordFor(tuple Tuple, expiresAt time.Time, status OracleStatus, payload string) Record {
	tuple.InputDigests = append([]string(nil), tuple.InputDigests...)
	tuple.EvidenceBoundaryDigests = append([]string(nil), tuple.EvidenceBoundaryDigests...)
	return Record{SchemaVersion: SchemaV1, EvidenceKind: EvidenceOracle, Tuple: tuple, Limitations: []string{"loaded evidence only"}, ExpiresAt: expiresAt, OracleResult: OracleResult{Status: status, Payload: json.RawMessage(payload)}}
}

func withEvidenceKind(record Record, kind EvidenceKind) Record {
	record.EvidenceKind = kind
	return record
}
func withClientVersionRange(record Record, versionRange string) Record {
	record.ClientVersionRange = versionRange
	return record
}
func withInputDigests(record Record, digests []string) Record {
	record.InputDigests = digests
	return record
}
func withEvidenceBoundaryDigests(record Record, digests []string) Record {
	record.EvidenceBoundaryDigests = digests
	return record
}
