package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	RARVerificationAuthoritySchema = "gentle-ai.rar-verification-authority/v1"
	RARNativeReceiptSchema         = "gentle-ai.rar-native-review-receipt/v1"

	rarAuthorityIndexSchema  = "gentle-ai.rar-authority-index/v1"
	rarAuthorityDigestDomain = "gentle-ai.rar-verification-authority-digest/v1"
	rarAuthorityDirectory    = "rar-authority"
	rarAuthorityVersion      = "v1"
	rarAuthorityMaxBytes     = 4 << 20
)

var (
	ErrRARAuthorityConflict = errors.New("RAR authority slot conflicts with an existing publication")
	ErrRARAuthorityCorrupt  = errors.New("RAR authority repository is corrupt")
	ErrRARAuthorityStale    = errors.New("RAR authority no longer matches native review authority")
)

// RARNativeReceiptVersion identifies the real review-kernel receipt preserved
// by a RAR authority object. It is not an outcome projection.
type RARNativeReceiptVersion string

const (
	RARReceiptCompactV2    RARNativeReceiptVersion = "compact_v2"
	RARReceiptHistoricalV1 RARNativeReceiptVersion = "historical_v1"
)

// RARNativeReceiptAuthority preserves one exact, canonical receipt issued by
// the native review kernel and the revision of the authority that issued it.
// Exactly one receipt preimage is present. ReceiptRef is the SHA-256 identity
// of the canonical receipt bytes already published by the kernel.
type RARNativeReceiptAuthority struct {
	Schema            string                  `json:"schema"`
	Version           RARNativeReceiptVersion `json:"version"`
	AuthorityRevision string                  `json:"authority_revision"`
	ReceiptRef        string                  `json:"receipt_ref"`
	Compact           *CompactReceipt         `json:"compact,omitempty"`
	Historical        *Receipt                `json:"historical,omitempty"`
}

// Validate verifies the preserved receipt preimage and its content identity.
// Repository resolution additionally verifies that this exact receipt remains
// the live native authority in the selected Git repository.
func (authority RARNativeReceiptAuthority) Validate() error {
	if authority.Schema != RARNativeReceiptSchema ||
		!validSHA256(authority.AuthorityRevision) ||
		!validSHA256(authority.ReceiptRef) {
		return errors.New("invalid RAR native receipt identity")
	}
	if (authority.Compact == nil) == (authority.Historical == nil) {
		return errors.New("RAR native receipt must preserve exactly one receipt preimage")
	}
	var (
		payload  []byte
		terminal TerminalState
		err      error
	)
	switch authority.Version {
	case RARReceiptCompactV2:
		if authority.Compact == nil || authority.Historical != nil {
			return errors.New("compact RAR receipt version requires the exact compact receipt")
		}
		if err = authority.Compact.Validate(); err != nil {
			return fmt.Errorf("validate compact RAR receipt: %w", err)
		}
		terminal = authority.Compact.TerminalState
		payload, err = canonicalRARReceiptPayload(*authority.Compact)
	case RARReceiptHistoricalV1:
		if authority.Historical == nil || authority.Compact != nil {
			return errors.New("historical RAR receipt version requires the exact v1 receipt")
		}
		if err = validateReceiptStructure(*authority.Historical); err != nil {
			return fmt.Errorf("validate historical RAR receipt: %w", err)
		}
		terminal = authority.Historical.TerminalState
		payload, err = canonicalRARReceiptPayload(*authority.Historical)
	default:
		return fmt.Errorf("unsupported RAR native receipt version %q", authority.Version)
	}
	if err != nil {
		return err
	}
	if terminal != TerminalApproved {
		return errors.New("RAR authority requires a terminal approved native receipt")
	}
	receiptDigest := sha256.Sum256(payload)
	if authority.ReceiptRef != "sha256:"+hex.EncodeToString(receiptDigest[:]) {
		return errors.New("RAR native receipt ref does not match its exact canonical preimage")
	}
	return nil
}

func (authority RARNativeReceiptAuthority) lineageID() string {
	if authority.Compact != nil {
		return authority.Compact.LineageID
	}
	if authority.Historical != nil {
		return authority.Historical.LineageID
	}
	return ""
}

// LineageID returns the lineage encoded by the preserved native receipt.
func (authority RARNativeReceiptAuthority) LineageID() string {
	return authority.lineageID()
}

func (authority RARNativeReceiptAuthority) candidateTree() string {
	if authority.Compact != nil {
		return authority.Compact.FinalCandidateTree
	}
	if authority.Historical != nil {
		return authority.Historical.FinalCandidateTree
	}
	return ""
}

// CandidateTree returns the terminal candidate encoded by the preserved
// receipt. It is derived from the real receipt, never copied into the wrapper.
func (authority RARNativeReceiptAuthority) CandidateTree() string {
	return authority.candidateTree()
}

func (authority RARNativeReceiptAuthority) policyHash() string {
	if authority.Compact != nil {
		return authority.Compact.PolicyHash
	}
	if authority.Historical != nil {
		return authority.Historical.PolicyHash
	}
	return ""
}

func (authority RARNativeReceiptAuthority) pathsDigest() string {
	if authority.Compact != nil {
		return authority.Compact.PathsDigest
	}
	if authority.Historical != nil {
		return authority.Historical.PathsDigest
	}
	return ""
}

// PathsDigest returns the reviewed path-set identity encoded by the receipt.
func (authority RARNativeReceiptAuthority) PathsDigest() string {
	return authority.pathsDigest()
}

// PolicyHash returns the policy encoded by the preserved native receipt.
func (authority RARNativeReceiptAuthority) PolicyHash() string {
	return authority.policyHash()
}

// RARVerificationAuthority is the exact owner preimage resolved by WorkRun and
// PAD adapters. AuthorityRef content-addresses the receipt plus all four RAR
// verification contracts; no copied review outcome is stored.
type RARVerificationAuthority struct {
	Schema        string                    `json:"schema"`
	AuthorityRef  string                    `json:"authority_ref"`
	Receipt       RARNativeReceiptAuthority `json:"receipt"`
	Applicability VerificationApplicability `json:"applicability"`
	Registry      VerificationPlanRegistry  `json:"registry"`
	Plan          VerificationPlan          `json:"plan"`
	Result        VerificationResultRef     `json:"result"`
}

// Validate verifies canonical self-consistency. Only repository Resolve
// methods prove that the preserved receipt is still native and live.
func (authority RARVerificationAuthority) Validate() error {
	if authority.Schema != RARVerificationAuthoritySchema ||
		!validSHA256(authority.AuthorityRef) {
		return errors.New("invalid RAR verification authority identity")
	}
	if err := authority.Receipt.Validate(); err != nil {
		return err
	}
	if err := ValidateVerificationResultRef(
		authority.Applicability,
		authority.Registry,
		authority.Plan,
		authority.Result,
	); err != nil {
		return err
	}
	if authority.Receipt.candidateTree() != authority.Result.Subject.CandidateTree ||
		authority.Receipt.policyHash() != authority.Result.PolicyHash ||
		authority.Receipt.pathsDigest() != authority.Result.Subject.PathsDigest {
		return errors.New("RAR result does not bind the native receipt candidate, paths, and policy")
	}
	want, err := rarVerificationAuthorityDigest(authority)
	if err != nil {
		return err
	}
	if authority.AuthorityRef != want {
		return errors.New("RAR verification authority ref does not match its canonical content")
	}
	return nil
}

// RARAuthorityPublication carries only exact owner contracts plus the expected
// native receipt identity. LineageID selects native review authority; it never
// selects a filesystem directory.
type RARAuthorityPublication struct {
	LineageID     string
	ReceiptRef    string
	Applicability VerificationApplicability
	Registry      VerificationPlanRegistry
	Plan          VerificationPlan
	Result        VerificationResultRef
}

// RARAuthorityRepository is bound to one exact Git repository/common-dir
// identity. Its filesystem roots are private and never caller-selected.
type RARAuthorityRepository struct {
	identity reviewRepositoryIdentityRecord
	root     string
	lease    *RepositoryIdentityLease
}

// OpenRARAuthorityRepository binds a repository handle without creating any
// RAR storage. Publish creates the private root; Resolve methods are read-only.
func OpenRARAuthorityRepository(ctx context.Context, repo string) (*RARAuthorityRepository, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("resolve RAR repository identity: %w", err)
	}
	return OpenRARAuthorityRepositoryWithRepositoryIdentityLease(ctx, lease)
}

// OpenRARAuthorityRepositoryWithRepositoryIdentityLease lets the production
// composition share one exact Git identity across every owner repository.
func OpenRARAuthorityRepositoryWithRepositoryIdentityLease(
	ctx context.Context,
	lease *RepositoryIdentityLease,
) (*RARAuthorityRepository, error) {
	if lease == nil {
		return nil, errors.New("RAR authority repository requires a repository identity lease")
	}
	if err := lease.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate RAR repository identity: %w", err)
	}
	identity := reviewRepositoryIdentityRecordFromLease(lease)
	root := filepath.Join(
		identity.GitCommonDir,
		"gentle-ai",
		"review-transactions",
		rarAuthorityDirectory,
		rarAuthorityVersion,
	)
	return &RARAuthorityRepository{identity: identity, root: root, lease: lease}, nil
}

// RepositoryRef returns the exact path-free Git identity retained by RAR.
func (repository *RARAuthorityRepository) RepositoryRef() string {
	if repository == nil || repository.lease == nil {
		return ""
	}
	return repository.lease.Identity().RepositoryRef
}

// Publish validates the exact result tuple, locks and revalidates the native
// approved receipt, then durably publishes one immutable content-addressed
// object and its result/exact-pair indexes. Exact replay is idempotent.
func (repository *RARAuthorityRepository) Publish(
	ctx context.Context,
	request RARAuthorityPublication,
) (RARVerificationAuthority, error) {
	if err := ctx.Err(); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := validateLineageID(request.LineageID); err != nil {
		return RARVerificationAuthority{}, err
	}
	if !validSHA256(request.ReceiptRef) {
		return RARVerificationAuthority{}, errors.New("RAR publication requires an exact native receipt ref")
	}
	if err := ValidateVerificationResultRef(
		request.Applicability,
		request.Registry,
		request.Plan,
		request.Result,
	); err != nil {
		return RARVerificationAuthority{}, fmt.Errorf("validate RAR publication result: %w", err)
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}

	native, subject, release, err := repository.lockNativeReceipt(
		ctx,
		request.LineageID,
		request.ReceiptRef,
	)
	if err != nil {
		// Bounded lock exhaustion converges only when a winner published this
		// caller's exact immutable pair and it still matches live native authority.
		if errors.Is(err, ErrAuthorityLockTimeout) {
			if _, readErr := readPrivateRARFile(repository.pairIndexPath(request.ReceiptRef, request.Result.ResultRef)); readErr == nil {
				replay, resolveErr := repository.ResolveReceiptResult(ctx, request.ReceiptRef, request.Result.ResultRef)
				if resolveErr == nil && replay.Receipt.lineageID() == request.LineageID &&
					reflect.DeepEqual(replay.Applicability, request.Applicability) &&
					reflect.DeepEqual(replay.Registry, request.Registry) &&
					reflect.DeepEqual(replay.Plan, request.Plan) &&
					reflect.DeepEqual(replay.Result, request.Result) {
					return replay, nil
				}
			}
		}
		return RARVerificationAuthority{}, err
	}
	defer release()
	if request.Result.Subject != subject ||
		request.Applicability.Subject != subject ||
		request.Plan.Subject != subject ||
		request.Result.PolicyHash != native.policyHash() {
		return RARVerificationAuthority{}, errors.New("RAR publication does not bind the exact native review subject and policy")
	}

	authority := RARVerificationAuthority{
		Schema:        RARVerificationAuthoritySchema,
		Receipt:       native,
		Applicability: request.Applicability,
		Registry:      request.Registry,
		Plan:          request.Plan,
		Result:        request.Result,
	}
	authorityRef, err := rarVerificationAuthorityDigest(authority)
	if err != nil {
		return RARVerificationAuthority{}, err
	}
	authority.AuthorityRef = authorityRef
	if err := authority.Validate(); err != nil {
		return RARVerificationAuthority{}, err
	}
	payload, err := canonicalRARAuthorityPayload(authority)
	if err != nil {
		return RARVerificationAuthority{}, err
	}

	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := ensureRARRepositoryRoot(repository.identity.GitCommonDir, repository.root, true); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}
	lock, err := acquireRARAuthorityLock(ctx, filepath.Join(repository.root, "LOCK"))
	if err != nil {
		return RARVerificationAuthority{}, err
	}
	defer lock.release()
	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := repository.publishLocked(authority, payload); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}
	return authority, nil
}

// ResolveResult resolves the unique immutable authority occupying resultRef
// and then revalidates its exact native receipt against the live review store.
func (repository *RARAuthorityRepository) ResolveResult(
	ctx context.Context,
	resultRef string,
) (RARVerificationAuthority, error) {
	return repository.resolve(ctx, "", resultRef, false)
}

// ResolveReceiptResult resolves only the exact receipt+result pair. It is the
// direct production seam for PAD's terminal-review authority port.
func (repository *RARAuthorityRepository) ResolveReceiptResult(
	ctx context.Context,
	receiptRef string,
	resultRef string,
) (RARVerificationAuthority, error) {
	return repository.resolve(ctx, receiptRef, resultRef, true)
}

// ResolveTerminalReview is the production PAD seam. It resolves the exact pair
// and returns the complete owner preimage, including the preserved native
// receipt abstraction. It never returns a projected outcome or a lossy
// conversion from CompactReceipt v2 to Receipt v1.
func (repository *RARAuthorityRepository) ResolveTerminalReview(
	ctx context.Context,
	receiptRef string,
	resultRef string,
) (RARVerificationAuthority, error) {
	return repository.ResolveReceiptResult(ctx, receiptRef, resultRef)
}

func (repository *RARAuthorityRepository) publishLocked(
	authority RARVerificationAuthority,
	payload []byte,
) error {
	for _, dir := range []string{
		filepath.Join(repository.root, "objects"),
		filepath.Join(repository.root, "by-result"),
		filepath.Join(repository.root, "by-receipt-result"),
		filepath.Join(repository.root, "by-receipt-result", hashPathComponent(authority.Receipt.ReceiptRef)),
	} {
		if err := ensurePrivateRARDirectoryTree(repository.root, dir, true); err != nil {
			return err
		}
	}

	index := rarAuthorityIndex{
		Schema:       rarAuthorityIndexSchema,
		ResultRef:    authority.Result.ResultRef,
		ReceiptRef:   authority.Receipt.ReceiptRef,
		AuthorityRef: authority.AuthorityRef,
	}
	indexPayload, err := canonicalRARIndexPayload(index)
	if err != nil {
		return err
	}
	objectPath := repository.objectPath(authority.AuthorityRef)
	resultPath := repository.resultIndexPath(authority.Result.ResultRef)
	pairPath := repository.pairIndexPath(authority.Receipt.ReceiptRef, authority.Result.ResultRef)

	// Preflight both occupied slots before creating any new index. Orphaned
	// content-addressed objects are harmless and make crash retry convergent.
	for _, slot := range []string{resultPath, pairPath} {
		existing, readErr := readPrivateRARFile(slot)
		if readErr == nil {
			if !bytes.Equal(existing, indexPayload) {
				return &RARAuthorityConflictError{Slot: slot}
			}
		} else if !errors.Is(readErr, fs.ErrNotExist) {
			return readErr
		}
	}
	if err := publishPrivateRARImmutable(objectPath, payload); err != nil {
		return err
	}
	if err := publishPrivateRARImmutable(resultPath, indexPayload); err != nil {
		return err
	}
	if err := publishPrivateRARImmutable(pairPath, indexPayload); err != nil {
		return err
	}
	return nil
}

func (repository *RARAuthorityRepository) resolve(
	ctx context.Context,
	receiptRef string,
	resultRef string,
	exactPair bool,
) (RARVerificationAuthority, error) {
	if err := ctx.Err(); err != nil {
		return RARVerificationAuthority{}, err
	}
	if !validSHA256(resultRef) || exactPair && !validSHA256(receiptRef) {
		return RARVerificationAuthority{}, errors.New("invalid RAR authority lookup ref")
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := ensureRARRepositoryRoot(repository.identity.GitCommonDir, repository.root, false); err != nil {
		return RARVerificationAuthority{}, err
	}

	indexPath := repository.resultIndexPath(resultRef)
	if exactPair {
		indexPath = repository.pairIndexPath(receiptRef, resultRef)
	}
	indexPayload, err := readPrivateRARFile(indexPath)
	if err != nil {
		return RARVerificationAuthority{}, err
	}
	index, err := parseRARAuthorityIndex(indexPayload)
	if err != nil {
		return RARVerificationAuthority{}, fmt.Errorf("%w: %v", ErrRARAuthorityCorrupt, err)
	}
	if index.ResultRef != resultRef || exactPair && index.ReceiptRef != receiptRef {
		return RARVerificationAuthority{}, fmt.Errorf("%w: lookup index binding mismatch", ErrRARAuthorityCorrupt)
	}

	payload, err := readPrivateRARFile(repository.objectPath(index.AuthorityRef))
	if err != nil {
		return RARVerificationAuthority{}, fmt.Errorf("%w: read authority object: %v", ErrRARAuthorityCorrupt, err)
	}
	authority, err := parseRARVerificationAuthority(payload)
	if err != nil {
		return RARVerificationAuthority{}, fmt.Errorf("%w: %v", ErrRARAuthorityCorrupt, err)
	}
	if authority.AuthorityRef != index.AuthorityRef ||
		authority.Result.ResultRef != index.ResultRef ||
		authority.Receipt.ReceiptRef != index.ReceiptRef {
		return RARVerificationAuthority{}, fmt.Errorf("%w: authority index does not bind its exact object", ErrRARAuthorityCorrupt)
	}
	if exactPair && authority.Receipt.ReceiptRef != receiptRef {
		return RARVerificationAuthority{}, fmt.Errorf("%w: receipt-result lookup mismatch", ErrRARAuthorityCorrupt)
	}
	if err := repository.validateLiveAuthority(ctx, authority); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARVerificationAuthority{}, err
	}
	return authority, nil
}

func (repository *RARAuthorityRepository) validateLiveAuthority(
	ctx context.Context,
	authority RARVerificationAuthority,
) error {
	native, subject, release, err := repository.lockNativeReceipt(
		ctx,
		authority.Receipt.lineageID(),
		authority.Receipt.ReceiptRef,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRARAuthorityStale, err)
	}
	defer release()
	if !rarNativeReceiptEqual(native, authority.Receipt) ||
		subject != authority.Result.Subject ||
		native.policyHash() != authority.Result.PolicyHash {
		return fmt.Errorf("%w: exact receipt, subject, or policy changed", ErrRARAuthorityStale)
	}
	return nil
}

func (repository *RARAuthorityRepository) validateIdentity(ctx context.Context) error {
	if repository == nil || repository.lease == nil ||
		strings.TrimSpace(repository.root) == "" {
		return errors.New("RAR authority repository is not initialized")
	}
	if err := repository.lease.Validate(ctx); err != nil {
		return fmt.Errorf("RAR authority repository identity changed: %w", err)
	}
	live := reviewRepositoryIdentityRecordFromLease(repository.lease)
	if live != repository.identity {
		return errors.New("RAR authority repository identity changed")
	}
	wantRoot := filepath.Join(
		live.GitCommonDir,
		"gentle-ai",
		"review-transactions",
		rarAuthorityDirectory,
		rarAuthorityVersion,
	)
	if filepath.Clean(repository.root) != filepath.Clean(wantRoot) {
		return errors.New("RAR authority root is not derived from the repository Git common directory")
	}
	return nil
}

func (repository *RARAuthorityRepository) objectPath(ref string) string {
	return filepath.Join(repository.root, "objects", hashPathComponent(ref)+".json")
}

func (repository *RARAuthorityRepository) resultIndexPath(ref string) string {
	return filepath.Join(repository.root, "by-result", hashPathComponent(ref)+".json")
}

func (repository *RARAuthorityRepository) pairIndexPath(receiptRef, resultRef string) string {
	return filepath.Join(
		repository.root,
		"by-receipt-result",
		hashPathComponent(receiptRef),
		hashPathComponent(resultRef)+".json",
	)
}

type RARAuthorityConflictError struct {
	Slot string
}

func (err *RARAuthorityConflictError) Error() string {
	return fmt.Sprintf("%v: %s", ErrRARAuthorityConflict, err.Slot)
}

func (err *RARAuthorityConflictError) Unwrap() error { return ErrRARAuthorityConflict }

type rarAuthorityIndex struct {
	Schema       string `json:"schema"`
	ResultRef    string `json:"result_ref"`
	ReceiptRef   string `json:"receipt_ref"`
	AuthorityRef string `json:"authority_ref"`
}

func (index rarAuthorityIndex) validate() error {
	if index.Schema != rarAuthorityIndexSchema ||
		!validSHA256(index.ResultRef) ||
		!validSHA256(index.ReceiptRef) ||
		!validSHA256(index.AuthorityRef) {
		return errors.New("invalid RAR authority index")
	}
	return nil
}

func canonicalRARIndexPayload(index rarAuthorityIndex) ([]byte, error) {
	if err := index.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(index)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseRARAuthorityIndex(payload []byte) (rarAuthorityIndex, error) {
	var index rarAuthorityIndex
	if err := decodeStrictRARJSON(payload, &index); err != nil {
		return rarAuthorityIndex{}, err
	}
	if err := index.validate(); err != nil {
		return rarAuthorityIndex{}, err
	}
	canonical, err := canonicalRARIndexPayload(index)
	if err != nil || !bytes.Equal(payload, canonical) {
		return rarAuthorityIndex{}, errors.New("RAR authority index is not canonical")
	}
	return index, nil
}

func canonicalRARAuthorityPayload(authority RARVerificationAuthority) ([]byte, error) {
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(authority)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseRARVerificationAuthority(payload []byte) (RARVerificationAuthority, error) {
	var authority RARVerificationAuthority
	if err := decodeStrictRARJSON(payload, &authority); err != nil {
		return RARVerificationAuthority{}, err
	}
	if err := authority.Validate(); err != nil {
		return RARVerificationAuthority{}, err
	}
	canonical, err := canonicalRARAuthorityPayload(authority)
	if err != nil || !bytes.Equal(payload, canonical) {
		return RARVerificationAuthority{}, errors.New("RAR verification authority is not canonical")
	}
	return authority, nil
}

func rarVerificationAuthorityDigest(authority RARVerificationAuthority) (string, error) {
	authority.AuthorityRef = ""
	payload, err := json.Marshal(authority)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(rarAuthorityDigestDomain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func canonicalRARReceiptPayload(receipt any) ([]byte, error) {
	payload, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func hashPathComponent(ref string) string {
	return strings.TrimPrefix(ref, "sha256:")
}

func decodeStrictRARJSON(payload []byte, target any) error {
	if len(payload) == 0 || len(payload) > rarAuthorityMaxBytes {
		return errors.New("RAR authority payload size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("RAR authority payload contains multiple JSON values")
	}
	return nil
}

func rarNativeReceiptEqual(left, right RARNativeReceiptAuthority) bool {
	return reflect.DeepEqual(left, right)
}
