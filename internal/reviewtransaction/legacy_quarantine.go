package reviewtransaction

// This file used to also hold QuarantineMalformedLegacyFreeze -- the
// provider behind the retired `review quarantine-legacy` CLI verb -- and its
// whole legacy-freeze-event inspection/replay machinery. Wave 7 S5a retired
// the verb (WU14); S5c retires the provider itself, since nothing else
// called it once the verb was gone (confirmed by direct grep of every
// exported symbol for external callers, and independently corroborated by
// WU14's own deadcode ratchet run, which listed all 5 of this file's
// functions, and nothing outside it, as newly unreachable).
//
// What remains is the retained JSON proof shape, LIVE and forensic-safety
// required (D5): CompactReclaimRecord (compact_reclaim.go) carries an
// optional `MalformedLegacyFreeze *LegacyMalformedFreezeProof` field on the
// shared reclaim-record schema. A historical quarantine record written by
// the now-retired provider, possibly from before this wave, still needs to
// decode into this exact shape when read back -- deleting the type would
// break JSON decoding of any such record still on disk and silently drop
// its audited proof fields.
type LegacyMalformedFreezeProof struct {
	LineageID        string   `json:"lineage_id"`
	HeadRevision     string   `json:"head_revision"`
	ChainIdentity    string   `json:"chain_identity"`
	EventRevision    string   `json:"event_revision"`
	PreviousRevision string   `json:"previous_revision"`
	Operation        string   `json:"operation"`
	Diagnostic       string   `json:"diagnostic"`
	Disposition      string   `json:"disposition"`
	ChangedFields    []string `json:"changed_fields"`
}
