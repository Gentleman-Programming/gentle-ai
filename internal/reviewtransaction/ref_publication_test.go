package reviewtransaction

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRefPublicationAuthorizationEncodesAndDecodesCanonicalLF(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "request-token-1")
	auth.RequestDigest = RefPublicationAuthorizationDigest(auth)

	encoded := auth.Payload()
	if strings.Count(encoded, "\n") != refPublicationAuthorizationFieldsCount {
		t.Fatalf("encoded payload has %d lines; want %d", strings.Count(encoded, "\n"), refPublicationAuthorizationFieldsCount)
	}
	for index, field := range refPublicationAuthorizationFields {
		want := fmt.Sprintf("%s=%s\n", field, auth.ValueOf(field))
		if !strings.HasPrefix(encoded, want) {
			t.Fatalf("encoded payload line %d prefix = %q; want %q", index+1, encoded, want)
		}
		encoded = encoded[len(want):]
	}
	if encoded != "" {
		t.Fatalf("encoded payload has trailing bytes: %q", encoded)
	}

	parsed, err := ParseRefPublicationAuthorization(auth.Payload())
	if err != nil {
		t.Fatalf("ParseRefPublicationAuthorization: %v", err)
	}
	if parsed != auth {
		t.Fatalf("round-trip mismatch:\nencoded=%#v\nparsed=%#v", auth, parsed)
	}
}

func TestRefPublicationAuthorizationRejectsOutOfFieldOrder(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "request-token-2")
	encoded := auth.Payload()
	lines := strings.Split(encoded, "\n")
	lines[0], lines[1] = lines[1], lines[0]
	scrambled := strings.Join(lines, "\n")
	if _, err := ParseRefPublicationAuthorization(scrambled); err == nil {
		t.Fatal("ParseRefPublicationAuthorization accepted an out-of-order payload")
	}
}

func TestRefPublicationAuthorizationRejectsTruncatedPayload(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "request-token-3")
	encoded := auth.Payload()
	truncated := strings.TrimSuffix(encoded, "\n")
	if _, err := ParseRefPublicationAuthorization(truncated); err == nil {
		t.Fatal("ParseRefPublicationAuthorization accepted a truncated payload")
	}
}

func TestRefPublicationAuthorizationDigestIsOverNonSelfFields(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "request-token-4")
	auth.RequestDigest = RefPublicationAuthorizationDigest(auth)
	alternate := auth
	alternate.RequestDigest = "sha256:" + strings.Repeat("a", 64)

	if RefPublicationAuthorizationDigest(auth) != RefPublicationAuthorizationDigest(alternate) {
		t.Fatalf("digest is not independent of request_digest field value")
	}
	identity := sampleRefPublicationAuthorization(t, "request-token-4")
	canonical := RefPublicationAuthorizationDigest(identity)
	identity.RequestDigest = canonical
	if RefPublicationAuthorizationDigest(identity) != canonical {
		t.Fatalf("digest is not stable across request_digest assignment: %s vs %s",
			RefPublicationAuthorizationDigest(identity), canonical)
	}
}

func TestRefPublicationManifestDigestIsStableAndSorted(t *testing.T) {
	m := RefPublicationManifest{Entries: []RefPublicationManifestEntry{
		{Path: "src/b.go", Mode: "100644", BlobSHA: verificationTestHash("blob-b")},
		{Path: "src/a.go", Mode: "100644", BlobSHA: verificationTestHash("blob-a")},
		{Path: "README.md", Mode: "100644", BlobSHA: verificationTestHash("blob-readme")},
	}}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	first := m.Digest()
	reversed := RefPublicationManifest{Entries: []RefPublicationManifestEntry{
		{Path: "README.md", Mode: "100644", BlobSHA: verificationTestHash("blob-readme")},
		{Path: "src/a.go", Mode: "100644", BlobSHA: verificationTestHash("blob-a")},
		{Path: "src/b.go", Mode: "100644", BlobSHA: verificationTestHash("blob-b")},
	}}
	if first != reversed.Digest() {
		t.Fatalf("manifest digest is not order-independent: %s vs %s", first, reversed.Digest())
	}
	if err := (RefPublicationManifest{}).Validate(); err == nil {
		t.Fatal("Validate accepted an empty manifest")
	}
}

func TestRefPublicationDestinationRefAllowlist(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		ok   bool
	}{
		{"feature branch", "refs/heads/feat/example-integration", true},
		{"nested feature branch", "refs/heads/team/feat/example", true},
		{"main branch is rejected", "refs/heads/main", false},
		{"master branch is rejected", "refs/heads/master", false},
		{"tag is rejected", "refs/tags/v1.0.0", false},
		{"symbolic ref is rejected", "HEAD", false},
		{"missing prefix is rejected", "feat/example-integration", false},
		{"wildcard is rejected", "refs/heads/*", false},
		{"empty segment is rejected", "refs/heads//feature", false},
		{"double dot is rejected", "refs/heads/feat/../escape", false},
		{"colon is rejected", "refs/heads/feat:other", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRefPublicationDestinationRef(tc.ref)
			if tc.ok && err != nil {
				t.Fatalf("ValidateRefPublicationDestinationRef(%q) = %v; want nil", tc.ref, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("ValidateRefPublicationDestinationRef(%q) = nil; want error", tc.ref)
			}
		})
	}
}

func TestRefPublicationStateTransitionsAreClosedAndLegal(t *testing.T) {
	legal := []struct {
		from, to RefPublicationState
	}{
		{RefPubPending, RefPubPrepared},
		{RefPubPrepared, RefPubPushed},
		{RefPubPushed, RefPubConfirmed},
		{RefPubPushed, RefPubConflict},
		{RefPubPushed, RefPubNotCreated},
		{RefPubPushed, RefPubPublicationUnknown},
		{RefPubPushed, RefPubBlocked},
		{RefPubPrepared, RefPubBlocked},
		{RefPubPending, RefPubInvalidRequest},
	}
	for _, edge := range legal {
		if !isLegalRefPublicationStateTransition(edge.from, edge.to) {
			t.Fatalf("legal transition %q -> %q rejected", edge.from, edge.to)
		}
	}

	terminal := []RefPublicationState{
		RefPubConfirmed, RefPubConflict, RefPubNotCreated,
		RefPubPublicationUnknown, RefPubBlocked, RefPubInvalidRequest,
	}
	for _, state := range terminal {
		if !isTerminalRefPublicationState(state) {
			t.Fatalf("terminal state %q is not classified as terminal", state)
		}
		for _, other := range terminal {
			if isLegalRefPublicationStateTransition(state, other) {
				t.Fatalf("terminal transition %q -> %q must be rejected", state, other)
			}
		}
	}

	if isActiveRefPublicationState(RefPubPushed) != true {
		t.Fatal("Pushed must be classified as active")
	}
	if isActiveRefPublicationState(RefPubConfirmed) != false {
		t.Fatal("Confirmed must be classified as terminal")
	}
}

func TestRefPublicationAuthorizationValidationRejectsForbiddenFields(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "request-token-5")
	auth.DestinationRef = "refs/heads/main"
	if err := auth.Validate(); err == nil {
		t.Fatal("Validate accepted a destination of refs/heads/main")
	}
	auth = sampleRefPublicationAuthorization(t, "request-token-6")
	auth.AdvertisedSourceRef = "refs/tags/v1"
	if err := auth.Validate(); err == nil {
		t.Fatal("Validate accepted a non-refs/heads/* advertised_source_ref")
	}
	auth = sampleRefPublicationAuthorization(t, "request-token-7")
	auth.Reason = "first line\nsecond line"
	if err := auth.Validate(); err == nil {
		t.Fatal("Validate accepted a reason with line breaks")
	}
}

func TestRefPublicationRecordValidationRequiresMatchingPayloadAndDigest(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "request-token-8")
	auth.RequestDigest = RefPublicationAuthorizationDigest(auth)
	record := RefPublicationRecord{
		Schema:        RefPublicationRecordSchema,
		RequestID:     auth.RequestID,
		RequestDigest: auth.RequestDigest,
		State:         RefPubPrepared,
		Payload:       []byte(auth.Payload()),
		UpdatedAt:     "2026-08-13T19:53:36Z",
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	corrupted := record
	corrupted.RequestID = "other-request-id"
	if err := corrupted.Validate(); err == nil {
		t.Fatal("Validate accepted a record whose request_id does not match its payload")
	}

	corrupted = record
	corrupted.Payload = []byte("")
	if err := corrupted.Validate(); err == nil {
		t.Fatal("Validate accepted an empty payload")
	}

	corrupted = record
	corrupted.Schema = "gentle-ai.review-ref-publication-record/v0"
	if err := corrupted.Validate(); err == nil {
		t.Fatal("Validate accepted an unsupported schema")
	}
}

func TestRefPublicationResultValidationRequiresProvenAttribution(t *testing.T) {
	result := RefPublicationResult{
		Schema:       RefPublicationResultSchema,
		RequestID:    "request-1",
		AttemptState: RefPubConfirmed,
		Attribution:  RefPublicationAttributionProven,
		RecordedAt:   "2026-08-13T19:53:36Z",
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	result.Attribution = RefPublicationAttributionUnproven
	if err := result.Validate(); err == nil {
		t.Fatal("Validate accepted an unproven attribution")
	}

	result.AttemptState = RefPubConflict
	if err := result.Validate(); err == nil {
		t.Fatal("Validate accepted a non-confirmed attempt_state")
	}
}

func TestRefPublicationAuthorizationPayloadIsByteExactAcrossReplay(t *testing.T) {
	auth := sampleRefPublicationAuthorization(t, "replay-token")
	auth.RequestDigest = RefPublicationAuthorizationDigest(auth)
	if auth.Payload() != auth.Payload() {
		t.Fatal("Payload is not byte-identical across immediate replays")
	}
	parsed, err := ParseRefPublicationAuthorization(auth.Payload())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Payload() != auth.Payload() {
		t.Fatal("round-tripped payload is not byte-identical")
	}
}

func sampleRefPublicationAuthorization(t *testing.T, token string) RefPublicationAuthorization {
	t.Helper()
	return RefPublicationAuthorization{
		RequestID:           "req-" + token,
		LineageID:           "1471-lineage",
		AuthorityRevision:   verificationTestHash(token + "-authority"),
		ReceiptRef:          verificationTestHash(token + "-receipt"),
		EndpointIdentity:    verificationTestHash(token + "-endpoint"),
		LocalSourceRef:      "refs/heads/feat/example-integration",
		AdvertisedSourceRef: "refs/heads/main",
		DestinationRef:      "refs/heads/feat/example-integration",
		SourceCommit:        "0123456789abcdef0123456789abcdef01234567",
		CandidateTree:       "fedcba9876543210fedcba9876543210fedcba98",
		PathManifestDigest:  verificationTestHash(token + "-manifest"),
		Actor:               "ardelperal",
		Reason:              "zero-diff tracker bootstrap " + token,
	}
}

func mustParseRefPublicationAuthorization(t *testing.T, payload string) RefPublicationAuthorization {
	t.Helper()
	auth, err := ParseRefPublicationAuthorization(payload)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

var _ = errors.New
