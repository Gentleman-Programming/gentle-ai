package reviewtransaction

// This file used to also hold QuarantineHistoricalLegacyFixScope -- the
// provider behind the retired `review quarantine-legacy-fix-scope` CLI verb
// -- and its whole physical-store inspection/replay machinery. Wave 7 S5a
// retired the verb (WU14); S5b retires the provider itself, since nothing
// else called it once the verb was gone (confirmed: zero production callers
// outside the retired CLI handler, and the deadcode ratchet independently
// listed every other symbol in this file, and only those symbols, as newly
// unreachable in WU14).
//
// What remains is the retained JSON proof shape, LIVE and forensic-safety
// required (D5): CompactReclaimRecord (compact_reclaim.go) carries an
// optional `LegacyFixScopeQuarantine *LegacyFixScopeQuarantineProof` field
// on the shared reclaim-record schema. A historical quarantine record
// written by the now-retired provider, possibly from before this wave,
// still needs to decode into this exact shape when read back -- deleting
// the type would break JSON decoding of any such record still on disk and
// silently drop its audited proof fields.
type LegacyFixScopeQuarantineProof struct {
	LineageID               string                  `json:"lineage_id"`
	HeadRevision            string                  `json:"head_revision"`
	ChainIdentity           string                  `json:"chain_identity"`
	EventRevision           string                  `json:"event_revision"`
	PreviousRevision        string                  `json:"previous_revision"`
	Operation               string                  `json:"operation"`
	FrozenGenesisPaths      []string                `json:"frozen_genesis_paths"`
	ExpandedCorrectionPaths []string                `json:"expanded_correction_paths"`
	FixDeltaHash            string                  `json:"fix_delta_hash"`
	Diagnostic              string                  `json:"diagnostic"`
	Disposition             string                  `json:"disposition"`
	AnomalySet              string                  `json:"anomaly_set"`
	Anomalies               []LegacyFixScopeAnomaly `json:"anomalies"`
}

// LegacyFixScopeAnomaly is LegacyFixScopeQuarantineProof's own nested proof
// shape; retained for the same D5 reason as its parent type.
type LegacyFixScopeAnomaly struct {
	Kind               string `json:"kind"`
	EventRevision      string `json:"event_revision"`
	Operation          string `json:"operation"`
	CanonicalOperation string `json:"canonical_operation"`
}
