package reviewtransaction

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

const (
	LedgerSchema         = "gentle-ai.review-ledger/v1"
	CanonicalEmptyLedger = `{"schema":"gentle-ai.review-ledger/v1","findings":[]}`
)

type ledgerEnvelope struct {
	Schema   string    `json:"schema"`
	Findings []Finding `json:"findings"`
}

// ledgerFinding deliberately excludes admission-only causal fields. The v1
// ledger is a stable projection of the frozen finding text; evidence routing
// remains bound by the native lens-result and compact-authority identities.
type ledgerFinding struct {
	ID        string   `json:"id"`
	Lens      string   `json:"lens,omitempty"`
	Location  string   `json:"location,omitempty"`
	Severity  string   `json:"severity,omitempty"`
	Claim     string   `json:"claim,omitempty"`
	ProofRefs []string `json:"proof_refs,omitempty"`
}

type strictLedgerEnvelope struct {
	Schema   string          `json:"schema"`
	Findings []ledgerFinding `json:"findings"`
}

func ledgerProjection(findings []Finding) []ledgerFinding {
	if findings == nil {
		return nil
	}
	projected := make([]ledgerFinding, len(findings))
	for index, finding := range findings {
		projected[index] = ledgerFinding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location, Severity: finding.Severity,
			Claim: finding.Claim, ProofRefs: finding.ProofRefs,
		}
	}
	return projected
}

// CanonicalLedger returns the exact bytes accepted by the native freeze boundary.
func CanonicalLedger(findings []Finding) ([]byte, error) {
	if findings == nil {
		return nil, errors.New("ledger requires an explicit findings array")
	}
	return json.Marshal(strictLedgerEnvelope{Schema: LedgerSchema, Findings: ledgerProjection(findings)})
}

func validateCanonicalLedger(payload []byte, expectedFindings []Finding, suppliedHash string) (string, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope strictLedgerEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return "", "", fmt.Errorf("parse canonical ledger: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", "", errors.New("canonical ledger must contain exactly one JSON value")
	}
	if envelope.Schema != LedgerSchema || envelope.Findings == nil {
		return "", "", errors.New("ledger requires gentle-ai.review-ledger/v1 and an explicit findings array")
	}
	canonical, err := json.Marshal(envelope)
	if err != nil {
		return "", "", err
	}
	if !bytes.Equal(payload, canonical) {
		return "", "", errors.New("ledger bytes are not the canonical compact JSON representation")
	}
	if expectedFindings != nil && !reflect.DeepEqual(envelope.Findings, ledgerProjection(expectedFindings)) {
		return "", "", errors.New("ledger findings do not exactly match the native findings input")
	}
	sum := sha256.Sum256(payload)
	ledgerHash := "sha256:" + hex.EncodeToString(sum[:])
	if suppliedHash != "" && suppliedHash != ledgerHash {
		return "", "", errors.New("supplied ledger_hash does not match canonical ledger bytes")
	}
	findings := make([]Finding, len(envelope.Findings))
	for index, finding := range envelope.Findings {
		findings[index] = Finding{
			ID: finding.ID, Lens: finding.Lens, Location: finding.Location, Severity: finding.Severity,
			Claim: finding.Claim, ProofRefs: finding.ProofRefs,
		}
	}
	return ledgerHash, findingsHash(findings), nil
}
