// Package runtimecertification evaluates runtime oracle evidence without
// launching external providers or treating rendered files as proof.
package runtimecertification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

type SchemaVersion string

const SchemaV1 SchemaVersion = "gentle-ai.runtime-certification/v1"

type State string

const (
	StateDeclared         State = "declared"
	StateWritten          State = "written"
	StateCertifiedCurrent State = "certified_current"
	StateStale            State = "stale"
	StateRevoked          State = "revoked"
	StateInconclusive     State = "inconclusive"
	StateFailed           State = "failed"
)

type EvidenceKind string

const (
	EvidenceDeclared EvidenceKind = "declared"
	EvidenceWritten  EvidenceKind = "written"
	EvidenceOracle   EvidenceKind = "oracle"
)

type OracleStatus string

const (
	OracleStatusPass         OracleStatus = "pass"
	OracleStatusFail         OracleStatus = "fail"
	OracleStatusInconclusive OracleStatus = "inconclusive"
)

const (
	ClientOpenCode           = "opencode"
	OracleOpenCodeDebugAgent = "opencode.debug-agent/v1"
	CapabilityResolvedAgent  = "opencode.agent-config.resolved/v1"
)

type Tuple struct {
	ClientName              string   `json:"client_name"`
	ClientVersion           string   `json:"client_version"`
	ClientVersionRange      string   `json:"client_version_range"`
	BinarySHA256            string   `json:"binary_sha256"`
	OS                      string   `json:"os"`
	Arch                    string   `json:"arch"`
	Capability              string   `json:"capability_id"`
	Subject                 string   `json:"subject"`
	OracleID                string   `json:"oracle_id"`
	CommandDigest           string   `json:"command_digest"`
	InputDigests            []string `json:"input_digests"`
	EvidenceBoundaryDigests []string `json:"evidence_boundary_digests"`
}

type Record struct {
	SchemaVersion SchemaVersion `json:"schema"`
	EvidenceKind  EvidenceKind  `json:"evidence_kind"`
	Tuple
	Limitations  []string     `json:"limitations,omitempty"`
	ExpiresAt    time.Time    `json:"expires_at"`
	OracleResult OracleResult `json:"oracle_result,omitempty"`
}

type OracleResult struct {
	Status  OracleStatus    `json:"status"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Revocation struct {
	OracleID      string
	ClientName    string
	ClientVersion string
	BinarySHA256  string
}

type Evaluation struct {
	State  State
	Detail string
}

type Policy struct {
	Expected    Tuple
	Now         time.Time
	Revocations []Revocation
}

func Evaluate(record Record, policy Policy) Evaluation {
	if record.SchemaVersion != SchemaV1 {
		return Evaluation{State: StateFailed, Detail: "unsupported runtime certification schema"}
	}
	if record.EvidenceKind == EvidenceDeclared {
		return Evaluation{State: StateDeclared, Detail: "declared evidence is not runtime certification"}
	}
	if record.EvidenceKind == EvidenceWritten {
		return Evaluation{State: StateWritten, Detail: "written evidence is not runtime certification"}
	}
	if record.EvidenceKind != EvidenceOracle {
		return Evaluation{State: StateFailed, Detail: "unknown evidence kind"}
	}
	if err := validateOracleEvidenceBoundary(record); err != nil {
		return Evaluation{State: StateFailed, Detail: err.Error()}
	}
	if !matchesTuple(record, policy.Expected) {
		return Evaluation{State: StateStale, Detail: "runtime certification tuple does not apply to the current identity"}
	}
	if isRevoked(record, policy.Revocations) {
		return Evaluation{State: StateRevoked, Detail: "runtime certification tuple was revoked"}
	}
	if expired(record.ExpiresAt, policy.Now) {
		return Evaluation{State: StateStale, Detail: "runtime certification expired for the current identity"}
	}

	switch record.OracleResult.Status {
	case OracleStatusPass:
		if err := validateOraclePayload(record.OracleID, record.Subject, record.OracleResult.Payload); err != nil {
			return Evaluation{State: StateFailed, Detail: err.Error()}
		}
		return Evaluation{State: StateCertifiedCurrent, Detail: "runtime oracle passed for current identity"}
	case OracleStatusInconclusive:
		return Evaluation{State: StateInconclusive, Detail: "runtime oracle was inconclusive"}
	case OracleStatusFail:
		return Evaluation{State: StateFailed, Detail: "runtime oracle failed"}
	default:
		return Evaluation{State: StateFailed, Detail: "runtime oracle result is missing or invalid"}
	}
}

func OpenCodeDebugAgentTuple(version, binarySHA256, goos, arch, subject string, inputDigests []string) Tuple {
	return Tuple{
		ClientName:              ClientOpenCode,
		ClientVersion:           version,
		ClientVersionRange:      version,
		BinarySHA256:            binarySHA256,
		OS:                      goos,
		Arch:                    arch,
		Capability:              CapabilityResolvedAgent,
		Subject:                 subject,
		OracleID:                OracleOpenCodeDebugAgent,
		CommandDigest:           CommandDigest("opencode", "debug", "agent", subject),
		InputDigests:            append([]string(nil), inputDigests...),
		EvidenceBoundaryDigests: append([]string(nil), inputDigests...),
	}
}

func CommandDigest(command ...string) string {
	payload, _ := json.Marshal(command)
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func matchesTuple(record Record, expected Tuple) bool {
	return record.ClientName == expected.ClientName &&
		record.ClientVersion == expected.ClientVersion &&
		record.ClientVersionRange == expected.ClientVersionRange &&
		record.BinarySHA256 == expected.BinarySHA256 &&
		record.OS == expected.OS &&
		record.Arch == expected.Arch &&
		record.Capability == expected.Capability &&
		record.Subject == expected.Subject &&
		record.OracleID == expected.OracleID &&
		record.CommandDigest == expected.CommandDigest &&
		slices.Equal(record.InputDigests, expected.InputDigests) &&
		slices.Equal(record.EvidenceBoundaryDigests, expected.EvidenceBoundaryDigests)
}

func validateOracleEvidenceBoundary(record Record) error {
	for name, value := range map[string]string{
		"client_name": record.ClientName, "client_version": record.ClientVersion, "client_version_range": record.ClientVersionRange,
		"binary_sha256": record.BinarySHA256, "os": record.OS, "arch": record.Arch, "capability_id": record.Capability,
		"subject": record.Subject, "oracle_id": record.OracleID, "command_digest": record.CommandDigest,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("runtime certification missing %s", name)
		}
	}
	if !hasUsableDigest(record.InputDigests) {
		return errors.New("runtime certification missing input digests")
	}
	if !hasUsableDigest(record.EvidenceBoundaryDigests) {
		return errors.New("runtime certification missing evidence boundary digests")
	}
	return nil
}

func hasUsableDigest(digests []string) bool {
	for _, digest := range digests {
		if strings.TrimSpace(digest) != "" {
			return true
		}
	}
	return false
}

func expired(expiresAt time.Time, now time.Time) bool {
	if expiresAt.IsZero() {
		return true
	}
	if now.IsZero() {
		now = time.Now()
	}
	return !expiresAt.After(now)
}

func isRevoked(record Record, revocations []Revocation) bool {
	for _, revocation := range revocations {
		if revocation.OracleID != "" && revocation.OracleID != record.OracleID {
			continue
		}
		if revocation.ClientName != "" && revocation.ClientName != record.ClientName {
			continue
		}
		if revocation.ClientVersion != "" && revocation.ClientVersion != record.ClientVersion {
			continue
		}
		if revocation.BinarySHA256 != "" && revocation.BinarySHA256 != record.BinarySHA256 {
			continue
		}
		return true
	}
	return false
}

func validateOraclePayload(oracleID, subject string, payload json.RawMessage) error {
	if oracleID != OracleOpenCodeDebugAgent {
		return errors.New("unsupported runtime oracle")
	}
	var agent map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	if err := decoder.Decode(&agent); err != nil {
		return fmt.Errorf("opencode debug-agent oracle payload is invalid: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("opencode debug-agent oracle payload contains trailing data")
	}
	name, _ := agent["name"].(string)
	if strings.TrimSpace(name) != subject {
		return errors.New("opencode debug-agent oracle payload does not match subject")
	}
	return nil
}
