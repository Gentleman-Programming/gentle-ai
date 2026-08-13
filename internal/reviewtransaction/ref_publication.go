package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	RefPublicationRecordSchema         = "gentle-ai.review-ref-publication-record/v1"
	RefPublicationAuthorizationSchema  = "gentle-ai.review-ref-publication-authorization/v1"
	RefPublicationResultSchema         = "gentle-ai.review-ref-publication/v1"
	RefPublicationStatusSchema         = "gentle-ai.review-ref-publication-status/v1"
	RefPublicationReconciliationSchema = "gentle-ai.review-ref-publication-reconciliation/v1"

	refPublicationAuthorizationFieldsCount = 14
	refPublicationRecordMaxBytes           = 4 << 20
)

// RefPublicationState is the lifecycle of one publish-ref attempt (design §3,
// lesson #1433). Only one transition may be taken from a non-terminal state.
type RefPublicationState string

const (
	RefPubPending            RefPublicationState = "pending"
	RefPubPrepared           RefPublicationState = "prepared"
	RefPubPushed             RefPublicationState = "pushed"
	RefPubConfirmed          RefPublicationState = "confirmed"
	RefPubConflict           RefPublicationState = "conflict"
	RefPubNotCreated         RefPublicationState = "not_created"
	RefPubPublicationUnknown RefPublicationState = "publication_unknown"
	RefPubBlocked            RefPublicationState = "blocked"
	RefPubInvalidRequest     RefPublicationState = "invalid_request"
)

// RefPublicationAttribution is the isolated-push attribution verdict required
// to classify a destination publication as confirmed.
type RefPublicationAttribution string

const (
	RefPublicationAttributionProven   RefPublicationAttribution = "proven"
	RefPublicationAttributionUnproven RefPublicationAttribution = "unproven"
)

var refPublicationStateTransitions = map[RefPublicationState]map[RefPublicationState]bool{
	RefPubPending: {
		RefPubPrepared:       true,
		RefPubInvalidRequest: true,
		RefPubBlocked:        true,
	},
	RefPubPrepared: {
		RefPubPushed:         true,
		RefPubBlocked:        true,
		RefPubInvalidRequest: true,
	},
	RefPubPushed: {
		RefPubConfirmed:          true,
		RefPubConflict:           true,
		RefPubNotCreated:         true,
		RefPubPublicationUnknown: true,
		RefPubBlocked:            true,
	},
	RefPubConfirmed:          {},
	RefPubConflict:           {},
	RefPubNotCreated:         {},
	RefPubPublicationUnknown: {},
	RefPubBlocked:            {},
	RefPubInvalidRequest:     {},
}

func isTerminalRefPublicationState(state RefPublicationState) bool {
	switch state {
	case RefPubConfirmed, RefPubConflict, RefPubNotCreated,
		RefPubPublicationUnknown, RefPubBlocked, RefPubInvalidRequest:
		return true
	}
	return false
}

func isLegalRefPublicationStateTransition(from, to RefPublicationState) bool {
	if from == to {
		return isActiveRefPublicationState(from)
	}
	allowed, ok := refPublicationStateTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

func isActiveRefPublicationState(state RefPublicationState) bool {
	return !isTerminalRefPublicationState(state)
}

// refPublicationAuthorizationFields is the canonical alphabetical order for
// the LF-token authorization. The list is the contract; do not reorder.
var refPublicationAuthorizationFields = [refPublicationAuthorizationFieldsCount]string{
	"actor",
	"advertised_source_ref",
	"authority_revision",
	"candidate_tree",
	"destination_ref",
	"endpoint_identity",
	"lineage_id",
	"local_source_ref",
	"path_manifest_digest",
	"reason",
	"receipt_ref",
	"request_digest",
	"request_id",
	"source_commit",
}

// RefPublicationManifestEntry is one exact reviewed path bound to a mode and
// blob. The entries must be sorted by (path, mode, blob) before the digest is
// computed so the manifest is content-addressed.
type RefPublicationManifestEntry struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	BlobSHA string `json:"blob_sha"`
}

// RefPublicationManifest is the sorted, content-addressed set of reviewed
// paths that bind C^{tree} to the authorized authority.
type RefPublicationManifest struct {
	Entries []RefPublicationManifestEntry `json:"entries"`
}

// Validate ensures every entry has a non-empty path, mode, and blob hash.
func (m RefPublicationManifest) Validate() error {
	if len(m.Entries) == 0 {
		return errors.New("ref publication manifest must bind at least one reviewed path")
	}
	for _, entry := range m.Entries {
		if entry.Path == "" {
			return errors.New("ref publication manifest entry path is empty")
		}
		if entry.Mode == "" {
			return errors.New("ref publication manifest entry mode is empty")
		}
		if !validSHA256(entry.BlobSHA) {
			return errors.New("ref publication manifest entry blob sha is invalid")
		}
	}
	return nil
}

// Digest returns the SHA-256 domain-separated content identity of the sorted
// manifest. Sorting is deterministic so two reviewers with the same set
// produce the same digest.
func (m RefPublicationManifest) Digest() string {
	entries := make([]RefPublicationManifestEntry, len(m.Entries))
	copy(entries, m.Entries)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		if entries[i].Mode != entries[j].Mode {
			return entries[i].Mode < entries[j].Mode
		}
		return entries[i].BlobSHA < entries[j].BlobSHA
	})
	hash := sha256.New()
	_, _ = hash.Write([]byte("gentle-ai.ref-publication-manifest-digest/v1"))
	_, _ = hash.Write([]byte{0})
	for _, entry := range entries {
		_, _ = hash.Write([]byte(entry.Path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(entry.Mode))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(entry.BlobSHA))
		_, _ = hash.Write([]byte{'\n'})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// RefPublicationAuthorization is the durable binding for one publish-ref
// attempt. The 14 fields are serialized in canonical alphabetical order as
// LF tokens so the same intent survives byte-for-byte across replays.
type RefPublicationAuthorization struct {
	RequestID           string
	RequestDigest       string
	LineageID           string
	AuthorityRevision   string
	ReceiptRef          string
	EndpointIdentity    string
	LocalSourceRef      string
	AdvertisedSourceRef string
	DestinationRef      string
	SourceCommit        string
	CandidateTree       string
	PathManifestDigest  string
	Actor               string
	Reason              string
}

// Validate enforces the schema, the destination ref whitelist, and the digest
// identity. It does not validate receipt or authority liveness; that lives
// in the RAR repository.
func (a RefPublicationAuthorization) Validate() error {
	if a.RequestID == "" {
		return errors.New("ref publication authorization request_id is empty")
	}
	if a.LineageID == "" {
		if err := validateLineageID(a.LineageID); err != nil {
			return fmt.Errorf("ref publication authorization lineage_id: %w", err)
		}
	}
	if !validSHA256(a.AuthorityRevision) {
		return errors.New("ref publication authorization authority_revision is not a sha256 digest")
	}
	if !validSHA256(a.ReceiptRef) {
		return errors.New("ref publication authorization receipt_ref is not a sha256 digest")
	}
	if a.EndpointIdentity == "" {
		return errors.New("ref publication authorization endpoint_identity is empty")
	}
	if !strings.HasPrefix(a.LocalSourceRef, "refs/heads/") {
		return errors.New("ref publication authorization local_source_ref must be a refs/heads/* ref")
	}
	if !strings.HasPrefix(a.AdvertisedSourceRef, "refs/heads/") {
		return errors.New("ref publication authorization advertised_source_ref must be a refs/heads/* ref")
	}
	if err := ValidateRefPublicationDestinationRef(a.DestinationRef); err != nil {
		return fmt.Errorf("ref publication authorization destination_ref: %w", err)
	}
	if a.SourceCommit == "" {
		return errors.New("ref publication authorization source_commit is empty")
	}
	if a.CandidateTree == "" {
		return errors.New("ref publication authorization candidate_tree is empty")
	}
	if !validSHA256(a.PathManifestDigest) {
		return errors.New("ref publication authorization path_manifest_digest is not a sha256 digest")
	}
	if a.Actor == "" {
		return errors.New("ref publication authorization actor is empty")
	}
	if strings.ContainsAny(a.Reason, "\r\n") {
		return errors.New("ref publication authorization reason must not contain line breaks")
	}
	return nil
}

// ValueOf returns the canonical-string value of the named LF-token field.
func (a RefPublicationAuthorization) ValueOf(field string) string {
	switch field {
	case "request_id":
		return a.RequestID
	case "request_digest":
		return a.RequestDigest
	case "lineage_id":
		return a.LineageID
	case "authority_revision":
		return a.AuthorityRevision
	case "receipt_ref":
		return a.ReceiptRef
	case "endpoint_identity":
		return a.EndpointIdentity
	case "local_source_ref":
		return a.LocalSourceRef
	case "advertised_source_ref":
		return a.AdvertisedSourceRef
	case "destination_ref":
		return a.DestinationRef
	case "source_commit":
		return a.SourceCommit
	case "candidate_tree":
		return a.CandidateTree
	case "path_manifest_digest":
		return a.PathManifestDigest
	case "actor":
		return a.Actor
	case "reason":
		return a.Reason
	}
	return ""
}

// SetField assigns the parsed value to the matching field. Unknown fields
// return false so the parser can reject them.
func (a *RefPublicationAuthorization) SetField(field, value string) bool {
	switch field {
	case "request_id":
		a.RequestID = value
	case "request_digest":
		a.RequestDigest = value
	case "lineage_id":
		a.LineageID = value
	case "authority_revision":
		a.AuthorityRevision = value
	case "receipt_ref":
		a.ReceiptRef = value
	case "endpoint_identity":
		a.EndpointIdentity = value
	case "local_source_ref":
		a.LocalSourceRef = value
	case "advertised_source_ref":
		a.AdvertisedSourceRef = value
	case "destination_ref":
		a.DestinationRef = value
	case "source_commit":
		a.SourceCommit = value
	case "candidate_tree":
		a.CandidateTree = value
	case "path_manifest_digest":
		a.PathManifestDigest = value
	case "actor":
		a.Actor = value
	case "reason":
		a.Reason = value
	default:
		return false
	}
	return true
}

// Payload renders the canonical LF-token encoding in alphabetical field order.
// The result is byte-identical for the same content, so identity hashes and
// write-ahead replays compare bit-for-bit.
func (a RefPublicationAuthorization) Payload() string {
	return a.encodeLF(nil)
}

func (a RefPublicationAuthorization) encodeLF(omit map[string]struct{}) string {
	var builder strings.Builder
	for _, field := range refPublicationAuthorizationFields {
		if _, skip := omit[field]; skip {
			continue
		}
		builder.WriteString(field)
		builder.WriteByte('=')
		builder.WriteString(a.ValueOf(field))
		builder.WriteByte('\n')
	}
	return builder.String()
}

// RefPublicationAuthorizationDigest computes the SHA-256 identity of the
// 13 non-self authorization fields in canonical order. The request_digest
// field is excluded so the digest is deterministic regardless of the
// expected value supplied by the caller.
func RefPublicationAuthorizationDigest(a RefPublicationAuthorization) string {
	omit := map[string]struct{}{"request_digest": {}}
	hash := sha256.New()
	_, _ = hash.Write([]byte("gentle-ai.ref-publication-authorization-digest/v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(a.encodeLF(omit)))
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// ParseRefPublicationAuthorization parses a canonical LF-token payload back
// into its struct. Field order must match the canonical order; the field
// names themselves are part of the contract.
func ParseRefPublicationAuthorization(payload string) (RefPublicationAuthorization, error) {
	var auth RefPublicationAuthorization
	if payload == "" {
		return auth, errors.New("ref publication authorization payload is empty")
	}
	lines := strings.Split(payload, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) != refPublicationAuthorizationFieldsCount {
		return auth, fmt.Errorf(
			"ref publication authorization payload has %d lines; want %d",
			len(lines), refPublicationAuthorizationFieldsCount,
		)
	}
	for index, line := range lines {
		eq := strings.IndexByte(line, '=')
		if eq <= 0 || eq == len(line)-1 {
			return auth, fmt.Errorf(
				"ref publication authorization line %d is not <field>=<value>: %q",
				index+1, line,
			)
		}
		if strings.Count(line, "=") != 1 {
			return auth, fmt.Errorf(
				"ref publication authorization line %d contains multiple '=': %q",
				index+1, line,
			)
		}
		want := refPublicationAuthorizationFields[index]
		got := line[:eq]
		if got != want {
			return auth, fmt.Errorf(
				"ref publication authorization line %d field %q is not in canonical position %q",
				index+1, got, want,
			)
		}
		if !auth.SetField(got, line[eq+1:]) {
			return auth, fmt.Errorf(
				"ref publication authorization line %d field %q is not a known field",
				index+1, got,
			)
		}
	}
	return auth, nil
}

// ValidateRefPublicationDestinationRef enforces the create-only allowlist:
// `refs/heads/<name>` where <name> is a non-empty, non-reserved branch name.
// main, default branches, symbolic refs, tags, deletes, wildcards, and the
// non-refs/heads/ namespace are rejected.
func ValidateRefPublicationDestinationRef(destination string) error {
	if !strings.HasPrefix(destination, "refs/heads/") {
		return errors.New("ref publication destination must be a refs/heads/* ref")
	}
	name := strings.TrimPrefix(destination, "refs/heads/")
	if name == "" || strings.Contains(name, "/") && (strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/")) {
		return errors.New("ref publication destination has an empty or malformed segment")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "~") ||
		strings.Contains(name, "^") || strings.Contains(name, ":") ||
		strings.Contains(name, "?") || strings.Contains(name, "*") ||
		strings.Contains(name, "[") {
		return errors.New("ref publication destination contains forbidden characters")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." ||
			segment == "@" || strings.HasSuffix(segment, ".lock") {
			return errors.New("ref publication destination has an invalid segment")
		}
		for _, r := range segment {
			if r == 0x7f || r < 0x20 {
				return errors.New("ref publication destination has a control character")
			}
		}
	}
	if strings.EqualFold(name, "main") || strings.EqualFold(name, "master") {
		return errors.New("ref publication destination cannot be the repository default branch")
	}
	return nil
}

// RefPublicationRecord is the durable on-disk artifact for one publish-ref
// attempt. The Payload is the canonical LF-token authorization; the State
// tracks the lifecycle; the Attribution is set only when the result is
// proven or marked unproven after a successful isolated push.
type RefPublicationRecord struct {
	Schema        string                    `json:"schema"`
	RequestID     string                    `json:"request_id"`
	RequestDigest string                    `json:"request_digest"`
	State         RefPublicationState       `json:"state"`
	Payload       []byte                    `json:"payload"`
	Attribution   RefPublicationAttribution `json:"attribution,omitempty"`
	ResultRef     string                    `json:"result_ref,omitempty"`
	UpdatedAt     string                    `json:"updated_at"`
}

// Validate enforces the on-disk schema contract before the record is
// persisted or returned to a caller.
func (record RefPublicationRecord) Validate() error {
	if record.Schema != RefPublicationRecordSchema {
		return errors.New("ref publication record schema is not review-ref-publication-record/v1")
	}
	if record.RequestID == "" {
		return errors.New("ref publication record request_id is empty")
	}
	if !validSHA256(record.RequestDigest) {
		return errors.New("ref publication record request_digest is not a sha256 digest")
	}
	if record.State == "" {
		return errors.New("ref publication record state is empty")
	}
	if len(record.Payload) == 0 || len(record.Payload) > refPublicationRecordMaxBytes {
		return errors.New("ref publication record payload size is invalid")
	}
	auth, err := ParseRefPublicationAuthorization(string(record.Payload))
	if err != nil {
		return fmt.Errorf("ref publication record payload: %w", err)
	}
	if auth.RequestID != record.RequestID {
		return errors.New("ref publication record binds a different request_id than its payload")
	}
	if auth.RequestDigest != record.RequestDigest {
		return errors.New("ref publication record binds a different request_digest than its payload")
	}
	if err := auth.Validate(); err != nil {
		return fmt.Errorf("ref publication record payload authorization: %w", err)
	}
	return nil
}

// RefPublicationResult is the canonical terminal verdict for a publish-ref
// attempt. The output schema is the durable receipt written into the result
// index when the attempt is confirmed or marked in conflict.
type RefPublicationResult struct {
	Schema       string                    `json:"schema"`
	RequestID    string                    `json:"request_id"`
	AttemptState RefPublicationState       `json:"attempt_state"`
	Attribution  RefPublicationAttribution `json:"attribution"`
	RecordedAt   string                    `json:"recorded_at"`
}

// Validate enforces the schema contract for the durable result record.
func (result RefPublicationResult) Validate() error {
	if result.Schema != RefPublicationResultSchema {
		return errors.New("ref publication result schema is not review-ref-publication/v1")
	}
	if result.RequestID == "" {
		return errors.New("ref publication result request_id is empty")
	}
	if result.AttemptState != RefPubConfirmed {
		return errors.New("ref publication result attempt_state must be confirmed")
	}
	if result.Attribution != RefPublicationAttributionProven {
		return errors.New("ref publication result attribution must be proven")
	}
	return nil
}

var (
	// ErrRefPublicationReplayMismatch is returned when the same request_id is
	// presented with a different request_digest than the persisted one.
	ErrRefPublicationReplayMismatch = errors.New("ref publication request_id is already bound to a different request_digest")
	// ErrRefPublicationAlreadyTerminal is returned when a write is attempted
	// against a record that has already reached a terminal state.
	ErrRefPublicationAlreadyTerminal = errors.New("ref publication record is already in a terminal state")
	// ErrRefPublicationNotPrepared is returned when a transition that requires
	// the prepared state is requested from any other state.
	ErrRefPublicationNotPrepared = errors.New("ref publication record is not in prepared state")
	// ErrRefPublicationUnknownRequestID is returned when Load or MarkTerminal
	// is called for a request_id that has no persisted record.
	ErrRefPublicationUnknownRequestID = errors.New("ref publication request_id is unknown")
	// ErrRefPublicationAllocationContested is returned when another active
	// request already occupies the same endpoint/destination pair.
	ErrRefPublicationAllocationContested = errors.New("ref publication endpoint/destination is already occupied by an active request")
	// ErrRefPublicationTransitionIllegal is returned when a state transition
	// is not allowed by the lifecycle table.
	ErrRefPublicationTransitionIllegal = errors.New("ref publication state transition is illegal")
	errRefPublicationRecordMissing     = errors.New("ref publication record is missing")
)
