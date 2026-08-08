package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// RDDModeStatusSchema identifies the observable projection of the kill
	// switch. It reports both sources plus the effective mode; it is never an
	// authorization and never carries a review outcome.
	RDDModeStatusSchema = "gentle-ai.rdd-mode-status/v1"

	rddModeOverrideSchema   = "gentle-ai.rdd-mode-override/v1"
	rddModeDigestDomain     = "gentle-ai.rdd-mode-override-digest/v1"
	rddModeDirectory        = "rdd-mode"
	rddModeLockName         = "LOCK"
	rddModeGenerationPrefix = "gen-"
	rddModeGenerationSuffix = ".json"
	rddModeGenerationDigits = 10
	rddModeMaxGeneration    = 999_999_999

	// rddModeOverrideInherit is the persisted "no clone-local opinion" value.
	// The override is off-only, so clearing it records an explicit inherit
	// generation instead of deleting history: CAS needs a head to compare
	// against, and re-enabling needs a cutoff timestamp.
	rddModeOverrideInherit = "inherit"

	// rddConsentSchema identifies the one-shot latch recording that the user has
	// already been asked whether receipt-driven development may run.
	rddConsentSchema = "gentle-ai.rdd-consent-asked/v1"
	// rddConsentName never matches the gen-%010d.json generation pattern, so the
	// override head scan ignores it instead of mistaking it for a generation.
	rddConsentName = "asked.json"
)

var (
	// ErrRDDDisabled reports that the user kill switch keeps receipt-driven
	// development off. It is a stop, never a fallback signal.
	//
	// refusal:by-design human-authority: a sentinel, not a user-facing message. Callers wrap it with the deciding scope and the exact `gentle-ai review mode enable` invocation; naming a command here would offer to undo a choice only the operator may reverse.
	ErrRDDDisabled = errors.New("receipt-driven development is disabled")

	// ErrRDDModeUnknown reports an unrecognised mode value. Callers that ignore
	// it still receive a disabled projection.
	ErrRDDModeUnknown = errors.New("unknown review mode value")

	// ErrRDDModeCorrupt reports an unreadable clone-local override record.
	ErrRDDModeCorrupt = errors.New("clone-local review mode override is corrupt")

	// ErrRDDModeRevisionMismatch reports a lost compare-and-set race.
	ErrRDDModeRevisionMismatch = errors.New("clone-local review mode revision mismatch")

	// ErrRDDModeRepositoryForcedOn reports an attempt to make a repository
	// impose receipt-driven development on every clone that checks it out.
	ErrRDDModeRepositoryForcedOn = errors.New("clone-local review mode override may only disable")

	// ErrRDDConsentCorrupt reports an unreadable one-shot consent latch.
	ErrRDDConsentCorrupt = errors.New("clone-local review consent latch is corrupt")

	// rddConsentPayload is the exact latch content. It deliberately carries no
	// timestamp: identical bytes keep the immutable no-replace publish idempotent,
	// so recording the same answer twice can never raise a slot conflict.
	rddConsentPayload = []byte(`{"schema":"` + rddConsentSchema + `"}` + "\n")
)

// RDDMode is the receipt-driven-development kill-switch value.
type RDDMode string

const (
	// RDDModeUnset means the source expressed no opinion.
	RDDModeUnset RDDMode = ""
	// RDDModeOn means the source permits receipt-driven development.
	RDDModeOn RDDMode = "on"
	// RDDModeOff means the source disables receipt-driven development.
	RDDModeOff RDDMode = "off"
)

// RDDModeSource names which of the two independent sources decided the
// effective mode.
type RDDModeSource string

const (
	// RDDModeSourceDefault means no source expressed an opinion.
	RDDModeSourceDefault RDDModeSource = "default"
	// RDDModeSourceGlobal means the user's uncommitted global mode decided.
	RDDModeSourceGlobal RDDModeSource = "global"
	// RDDModeSourceCloneLocal means this clone's Git-common-dir override decided.
	RDDModeSourceCloneLocal RDDModeSource = "clone_local"
)

// RDDOperation classifies what an actor wants to do, so that disabling freezes
// authority read-only instead of destroying it.
type RDDOperation string

const (
	// RDDOperationStart is a new review start. Disabled mode rejects it.
	RDDOperationStart RDDOperation = "start"
	// RDDOperationMutate advances existing authority. Disabled mode rejects it.
	RDDOperationMutate RDDOperation = "mutate"
	// RDDOperationAbandon delegates eligibility to the abandon storage gate,
	// which admits only its explicitly sanctioned cleanup classes. Disabled mode
	// permits that cleanup; it cannot issue or advance a terminal receipt.
	RDDOperationAbandon RDDOperation = "abandon"
	// RDDOperationRead covers status, exact replay, receipt validation, and
	// diagnostics. Disabled mode never rejects it.
	RDDOperationRead RDDOperation = "read"
)

// RDDDelivery is the delivery projection reported under ordinary repository
// policy. None of its values is an approval or a PASS.
type RDDDelivery string

const (
	// RDDDeliveryReceiptGoverned means an existing receipt governs delivery.
	RDDDeliveryReceiptGoverned RDDDelivery = "receipt_governed"
	// RDDDeliveryDisabledUnmanaged is delivery of work produced with the kill
	// switch off and no receipt.
	RDDDeliveryDisabledUnmanaged RDDDelivery = "disabled/unmanaged"
	// RDDDeliveryUnmanaged is delivery with the switch on but no receipt yet.
	RDDDeliveryUnmanaged RDDDelivery = "unmanaged"
	// RDDDeliveryCandidateDeclinedUnmanaged is delivery the operator explicitly
	// chose to leave outside RDD for one exact candidate. It is not a receipt,
	// approval, or global mode change.
	RDDDeliveryCandidateDeclinedUnmanaged RDDDelivery = "candidate_declined/unmanaged"
)

// RDDGlobalMode is the raw global user mode read from uncommitted user state.
// Value is deliberately untyped here so that a hand-edited or future value
// fails closed inside this package instead of at its persistence boundary.
// RecordedAt is provenance only: it says when the user last recorded the global
// mode, and it is deliberately not an approval cutoff, because approval is
// bound to candidate content rather than to any wall-clock moment.
type RDDGlobalMode struct {
	Value      string
	RecordedAt time.Time
}

// RDDModeStatus is the read-only projection of both sources. Revision is the
// clone-local compare-and-set token. The projection carries no time cutoff: it
// answers "may review start now", never "which bytes are approved".
type RDDModeStatus struct {
	Schema     string        `json:"schema"`
	Global     RDDMode       `json:"global"`
	CloneLocal RDDMode       `json:"clone_local"`
	Effective  RDDMode       `json:"effective"`
	Source     RDDModeSource `json:"source"`
	Revision   string        `json:"revision,omitempty"`
}

// Enabled reports whether new receipt-driven development may start.
func (status RDDModeStatus) Enabled() bool { return status.Effective == RDDModeOn }

// RDDDisabledError is the typed rejection returned while the kill switch is
// off. No agent may retry past it, reactivate it, or fall back around it.
type RDDDisabledError struct {
	Operation RDDOperation
	Source    RDDModeSource
}

// Error names the exact command that turns reviews back on, scoped to the
// source that actually decided. Refusing here is correct -- the operator asked
// for reviews to be off -- but a refusal that exits non-zero and names no
// runnable continuation is the one shape this project does not ship. The scope
// is derived rather than generic so the operator does not have to work out
// which of the two independent sources they need to change.
func (err *RDDDisabledError) Error() string {
	message := fmt.Sprintf("%v: %s is rejected because the %s mode source keeps it off",
		ErrRDDDisabled, rddOperationSubject(err.Operation), err.Source)
	// A mutation refuses against authority that already exists, so the operator
	// needs one fact a start never has to carry: their in-flight review survived
	// the refusal. It is stated before the continuation because it is true even
	// when no source can be named and no command may be offered.
	if err.Operation == RDDOperationMutate {
		message += "; the review is frozen, not discarded"
	}
	scope := reviewModeScopeForSource(err.Source)
	if scope == "" {
		return message
	}
	if err.Operation == RDDOperationMutate {
		return fmt.Sprintf("%s; turn reviews back on with gentle-ai review mode enable --scope=%s to continue it from where it stopped", message, scope)
	}
	return fmt.Sprintf("%s; turn it back on with gentle-ai review mode enable --scope=%s", message, scope)
}

// rddOperationSubject names the refused operation the way an operator would say
// it. "mutate" is an internal classification, not something anybody typed; the
// operator ran a verb that advances a review they already started.
func rddOperationSubject(operation RDDOperation) string {
	if operation == RDDOperationMutate {
		return "advancing an existing review"
	}
	return string(operation)
}

// reviewModeScopeForSource maps the deciding source onto the --scope value of
// `gentle-ai review mode enable`. The default source expresses no opinion, so
// it can never be what keeps reviews off and gets no continuation rather than
// a guessed one.
func reviewModeScopeForSource(source RDDModeSource) string {
	switch source {
	case RDDModeSourceGlobal:
		return "global"
	case RDDModeSourceCloneLocal:
		return "clone"
	default:
		return ""
	}
}

func (err *RDDDisabledError) Unwrap() error { return ErrRDDDisabled }

// ResolveRDDMode combines the global user mode with this clone's off-only
// override. Any off wins, a repository can never force on, and every failure
// projects a disabled status so a caller that drops the error still fails safe.
func ResolveRDDMode(ctx context.Context, repo string, global RDDGlobalMode) (RDDModeStatus, error) {
	if err := ctx.Err(); err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceDefault), err
	}
	globalMode, globalErr := normalizeRDDMode(global.Value)
	if globalErr != nil {
		return failedClosedRDDModeStatus(RDDModeSourceGlobal), globalErr
	}
	override, present, overrideErr := readCloneLocalRDDOverride(ctx, repo)
	if overrideErr != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), overrideErr
	}
	return rddModeStatus(globalMode, override, present), nil
}

// SetCloneLocalRDDMode records this clone's off-only override under the Git
// common directory. It is never committed, never shared with another clone, and
// accepts only RDDModeOff or RDDModeUnset. expectedRevision is the exact
// compare-and-set token returned by the previous read; "" expects no record.
func SetCloneLocalRDDMode(
	ctx context.Context,
	repo string,
	mode RDDMode,
	expectedRevision string,
	global RDDGlobalMode,
) (RDDModeStatus, error) {
	if err := ctx.Err(); err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	persisted, err := cloneLocalRDDOverrideValue(mode)
	if err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	dir, err := cloneLocalRDDModeRoot(ctx, repo, true)
	if err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	lock, err := acquireRARAuthorityLock(ctx, filepath.Join(dir, rddModeLockName))
	if err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	defer func() { _ = lock.release() }()

	head, present, err := readCloneLocalRDDOverrideHead(dir)
	if err != nil {
		// An unreadable head is precisely what this command exists to replace,
		// so it must not be able to block its own repair -- that left the
		// operator with a refusal and no runnable way out of it. It expresses
		// no readable opinion and therefore carries no compare-and-set token,
		// which is the same position as holding no record at all.
		//
		// Nothing is weakened. The lock still serialises writers, and the
		// immutable no-replace publish still refuses to overwrite the
		// unreadable generation: the repair writes the generation that
		// supersedes it, so a lost race still cannot corrupt the head.
		if !errors.Is(err, ErrRDDModeCorrupt) {
			return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
		}
		generation, generationErr := cloneLocalRDDOverrideHeadGeneration(dir)
		if generationErr != nil {
			return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), generationErr
		}
		head, present = rddModeOverrideRecord{Generation: generation}, false
	}
	current := ""
	if present {
		current = head.Revision
	}
	if strings.TrimSpace(expectedRevision) != current {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), fmt.Errorf(
			"%w: expected %q but the clone-local head is %q", ErrRDDModeRevisionMismatch, expectedRevision, current)
	}
	if head.Generation >= rddModeMaxGeneration {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), errors.New("clone-local review mode generation space is exhausted")
	}

	record := rddModeOverrideRecord{
		Schema:           rddModeOverrideSchema,
		Generation:       head.Generation + 1,
		PreviousRevision: current,
		Mode:             persisted,
		RecordedAt:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if record.Revision, err = rddModeOverrideDigest(record); err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	payload, err := canonicalRDDModeOverridePayload(record)
	if err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	// The immutable no-replace publish is the fail-closed backstop: a writer
	// that somehow bypassed the lock still cannot overwrite a published
	// generation, so a lost race can never corrupt the head record.
	if err := publishPrivateRARImmutable(filepath.Join(dir, rddModeGenerationName(record.Generation)), payload); err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	globalMode, globalErr := normalizeRDDMode(global.Value)
	if globalErr != nil {
		return failedClosedRDDModeStatus(RDDModeSourceGlobal), globalErr
	}
	return rddModeStatus(globalMode, record, true), nil
}

// AuthorizeRDDOperation is the single kill-switch gate. Reads and the
// independently-gated abandon cleanup pass; starts and mutations stop
// with a typed error while the switch is off.
func AuthorizeRDDOperation(
	ctx context.Context,
	repo string,
	global RDDGlobalMode,
	operation RDDOperation,
) (RDDModeStatus, error) {
	status, err := ResolveRDDMode(ctx, repo, global)
	if err != nil {
		return status, err
	}
	switch operation {
	case RDDOperationRead, RDDOperationAbandon:
		return status, nil
	case RDDOperationStart, RDDOperationMutate:
		if !status.Enabled() {
			return status, &RDDDisabledError{Operation: operation, Source: status.Source}
		}
		return status, nil
	default:
		// refusal:by-design world-action: the operation set is a compile-time constant of this package, so an unknown value is a caller bug and the exit is a code fix, not a command the operator could run.
		return failedClosedRDDModeStatus(status.Source), fmt.Errorf("unknown receipt-driven development operation %q", operation)
	}
}

// AuthorizeRDDCandidate reports whether the current candidate may start a fresh
// review. Only the effective mode decides: while the switch is off this is the
// same typed start stop as any other, and once it is back on the candidate is
// reviewable whatever its authorship time.
//
// Authorship time is deliberately not a gate. Receipt-driven development is
// post-candidate by design: the review freezes a snapshot at review time,
// inspects exactly those bytes, and issues a receipt content-bound to them, so
// reviewing pre-existing bytes is the normal case rather than an exception.
// Gating on creation time would strand every candidate authored during a
// disabled window with no recovery other than discarding the work.
//
// The property that must survive a disabled window is that no approval is ever
// inherited, and that is enforced structurally elsewhere: a receipt binds its
// candidate tree and policy, so lockNativeReceipt refuses any receipt that does
// not match the bytes currently under review. Duplicating a weaker time-based
// approximation of that rule here is what conflated the two concerns.
func AuthorizeRDDCandidate(status RDDModeStatus) error {
	if !status.Enabled() {
		return &RDDDisabledError{Operation: RDDOperationStart, Source: status.Source}
	}
	return nil
}

// RDDConsentAsked reports whether this clone has already put the one-time review
// question to the user.
//
// Only acceptance sets the latch. Declining applies to one candidate and records
// nothing, so the next work unit is offered the review again — today's passive
// documentation says nothing about tomorrow's migration. Turning reviews off for
// good stays a deliberate `review mode disable`, never a keystroke in a hurry.
//
// The latch lives beside the clone-local override so both share one never-committed
// scope: a fresh clone is asked once, no clone inherits another clone's answer, and
// there is no second storage to reconcile.
func RDDConsentAsked(ctx context.Context, repo string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	payload, err := readPrivateRARFile(filepath.Join(dir, rddConsentName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", ErrRDDConsentCorrupt, err)
	}
	if !bytes.Equal(payload, rddConsentPayload) {
		return false, fmt.Errorf("%w: unexpected latch content", ErrRDDConsentCorrupt)
	}
	return true, nil
}

// RecordRDDConsentAsked latches the one-time question as asked. It is a one-way
// latch rather than a mode: it records only that the human was given the choice,
// never which choice they made.
func RecordRDDConsentAsked(ctx context.Context, repo string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := cloneLocalRDDModeRoot(ctx, repo, true)
	if err != nil {
		return err
	}
	return publishPrivateRARImmutable(filepath.Join(dir, rddConsentName), rddConsentPayload)
}

// RDDDeliveryDisposition reports delivery under ordinary repository policy. An
// existing receipt still governs delivery because disabling freezes authority
// read-only; without one, disabled work stays explicitly unmanaged. No value
// here fabricates an approval or a PASS.
func RDDDeliveryDisposition(status RDDModeStatus, receiptPresent bool) RDDDelivery {
	switch {
	case receiptPresent:
		return RDDDeliveryReceiptGoverned
	case status.Enabled():
		return RDDDeliveryUnmanaged
	default:
		return RDDDeliveryDisabledUnmanaged
	}
}

func rddModeStatus(
	globalMode RDDMode,
	override rddModeOverrideRecord,
	present bool,
) RDDModeStatus {
	status := RDDModeStatus{
		Schema:     RDDModeStatusSchema,
		Global:     globalMode,
		CloneLocal: RDDModeUnset,
		Effective:  RDDModeOff,
		Source:     RDDModeSourceDefault,
	}
	if present {
		status.Revision = override.Revision
		if override.Mode == string(RDDModeOff) {
			status.CloneLocal = RDDModeOff
		}
	}
	switch {
	case status.CloneLocal == RDDModeOff:
		status.Effective, status.Source = RDDModeOff, RDDModeSourceCloneLocal
	case globalMode == RDDModeOff:
		status.Effective, status.Source = RDDModeOff, RDDModeSourceGlobal
	case globalMode == RDDModeOn:
		status.Effective, status.Source = RDDModeOn, RDDModeSourceGlobal
	default:
		status.Effective, status.Source = RDDModeOn, RDDModeSourceDefault
	}
	return status
}

func failedClosedRDDModeStatus(source RDDModeSource) RDDModeStatus {
	return RDDModeStatus{
		Schema:     RDDModeStatusSchema,
		Global:     RDDModeUnset,
		CloneLocal: RDDModeUnset,
		Effective:  RDDModeOff,
		Source:     source,
	}
}

func normalizeRDDMode(value string) (RDDMode, error) {
	switch strings.TrimSpace(value) {
	case "":
		return RDDModeUnset, nil
	case string(RDDModeOn):
		return RDDModeOn, nil
	case string(RDDModeOff):
		return RDDModeOff, nil
	default:
		return RDDModeOff, fmt.Errorf("%w: %q", ErrRDDModeUnknown, value)
	}
}

// RDDModeValueUnintelligible reports whether a persisted global mode value is
// neither a mode this product understands nor the absence of an opinion. It
// exists so a refusal can name the file holding such a value without
// re-implementing this package's own vocabulary at the boundary.
func RDDModeValueUnintelligible(value string) bool {
	_, err := normalizeRDDMode(value)
	return err != nil
}

func cloneLocalRDDOverrideValue(mode RDDMode) (string, error) {
	switch mode {
	case RDDModeOff:
		return string(RDDModeOff), nil
	case RDDModeUnset:
		return rddModeOverrideInherit, nil
	case RDDModeOn:
		return "", fmt.Errorf("%w: a repository may disable receipt-driven development but never require it", ErrRDDModeRepositoryForcedOn)
	default:
		return "", fmt.Errorf("%w: %q", ErrRDDModeUnknown, mode)
	}
}

// cloneLocalRDDModeRoot derives the override directory from the exact Git
// common directory. It nests inside the already-validated owner-only review
// authority root so that path safety, permissions, and private IO reuse the
// existing helpers instead of inventing a second path policy.
func cloneLocalRDDModeRoot(ctx context.Context, repo string, create bool) (string, error) {
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		// A bare repository already states its own refusal and names its own
		// recovery. Wrapping it in this internal concern would misattribute the
		// failure to a kill switch the operator never touched.
		var bare *BareRepositoryError
		if errors.As(err, &bare) {
			return "", err
		}
		return "", fmt.Errorf("resolve review mode repository identity: %w", err)
	}
	identity := reviewRepositoryIdentityRecordFromLease(lease)
	base := filepath.Join(
		identity.GitCommonDir,
		"gentle-ai",
		"review-transactions",
		rarAuthorityDirectory,
		rarAuthorityVersion,
	)
	if err := ensureRARRepositoryRoot(identity.GitCommonDir, base, create); err != nil {
		return "", err
	}
	dir := filepath.Join(base, rddModeDirectory)
	if err := ensurePrivateRARDirectoryTree(base, dir, create); err != nil {
		return "", err
	}
	return dir, nil
}

func readCloneLocalRDDOverride(ctx context.Context, repo string) (rddModeOverrideRecord, bool, error) {
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return rddModeOverrideRecord{}, false, nil
		}
		return rddModeOverrideRecord{}, false, err
	}
	return readCloneLocalRDDOverrideHead(dir)
}

// CloneLocalRDDModeRecordPath reports the clone-local override file that
// currently decides this clone's mode, so a refusal can name the exact file
// holding an unreadable value instead of merely describing it. It is strictly
// read-only, never creates state, and reports "" when this clone holds no
// override at all.
func CloneLocalRDDModeRecordPath(ctx context.Context, repo string) (string, error) {
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	head, err := cloneLocalRDDOverrideHeadGeneration(dir)
	if err != nil || head == 0 {
		return "", err
	}
	return filepath.Join(dir, rddModeGenerationName(head)), nil
}

// cloneLocalRDDOverrideHeadGeneration reports the highest published generation
// without reading or parsing it. Naming and repairing an unreadable head need
// the slot number and nothing else, and a record that cannot be parsed must not
// be able to hide the slot that supersedes it.
func cloneLocalRDDOverrideHeadGeneration(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	head := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		generation, ok := rddModeGenerationOf(entry.Name())
		if ok && generation > head {
			head = generation
		}
	}
	return head, nil
}

func readCloneLocalRDDOverrideHead(dir string) (rddModeOverrideRecord, bool, error) {
	head, err := cloneLocalRDDOverrideHeadGeneration(dir)
	if err != nil {
		return rddModeOverrideRecord{}, false, err
	}
	if head == 0 {
		return rddModeOverrideRecord{}, false, nil
	}
	payload, err := readPrivateRARFile(filepath.Join(dir, rddModeGenerationName(head)))
	if err != nil {
		var unsafePath *UnsafeRARPathError
		if errors.As(err, &unsafePath) {
			return rddModeOverrideRecord{}, false, err
		}
		return rddModeOverrideRecord{}, false, fmt.Errorf("%w: read generation %d: %v", ErrRDDModeCorrupt, head, err)
	}
	record, err := parseRDDModeOverride(payload)
	if err != nil {
		return rddModeOverrideRecord{}, false, fmt.Errorf("%w: %v", ErrRDDModeCorrupt, err)
	}
	if record.Generation != head {
		return rddModeOverrideRecord{}, false, fmt.Errorf("%w: generation %d is stored as %d", ErrRDDModeCorrupt, record.Generation, head)
	}
	return record, true, nil
}

// rddModeOverrideRecord is one immutable generation of the clone-local
// override. Generations are the compare-and-set slots: publishing is
// no-replace, so a stale writer loses without touching the current head.
type rddModeOverrideRecord struct {
	Schema           string `json:"schema"`
	Generation       int    `json:"generation"`
	PreviousRevision string `json:"previous_revision,omitempty"`
	Mode             string `json:"mode"`
	RecordedAt       string `json:"recorded_at"`
	Revision         string `json:"revision"`
}

func (record rddModeOverrideRecord) validate() error {
	if record.Schema != rddModeOverrideSchema {
		return errors.New("invalid clone-local review mode schema")
	}
	if record.Generation < 1 || record.Generation > rddModeMaxGeneration {
		return errors.New("invalid clone-local review mode generation")
	}
	if record.Mode != string(RDDModeOff) && record.Mode != rddModeOverrideInherit {
		return fmt.Errorf("%w: %q", ErrRDDModeUnknown, record.Mode)
	}
	if _, err := time.Parse(time.RFC3339Nano, record.RecordedAt); err != nil {
		return fmt.Errorf("invalid clone-local review mode timestamp: %w", err)
	}
	if !validSHA256(record.Revision) {
		return errors.New("invalid clone-local review mode revision")
	}
	if record.PreviousRevision != "" && !validSHA256(record.PreviousRevision) {
		return errors.New("invalid clone-local review mode predecessor revision")
	}
	want, err := rddModeOverrideDigest(record)
	if err != nil {
		return err
	}
	if record.Revision != want {
		return errors.New("clone-local review mode revision does not match its content")
	}
	return nil
}

func rddModeOverrideDigest(record rddModeOverrideRecord) (string, error) {
	record.Revision = ""
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(rddModeDigestDomain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func canonicalRDDModeOverridePayload(record rddModeOverrideRecord) ([]byte, error) {
	if err := record.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseRDDModeOverride(payload []byte) (rddModeOverrideRecord, error) {
	var record rddModeOverrideRecord
	if err := decodeStrictRARJSON(payload, &record); err != nil {
		return rddModeOverrideRecord{}, err
	}
	if err := record.validate(); err != nil {
		return rddModeOverrideRecord{}, err
	}
	canonical, err := canonicalRDDModeOverridePayload(record)
	if err != nil || !bytes.Equal(payload, canonical) {
		return rddModeOverrideRecord{}, errors.New("clone-local review mode record is not canonical")
	}
	return record, nil
}

func rddModeGenerationName(generation int) string {
	return fmt.Sprintf("%s%0*d%s", rddModeGenerationPrefix, rddModeGenerationDigits, generation, rddModeGenerationSuffix)
}

func rddModeGenerationOf(name string) (int, bool) {
	if !strings.HasPrefix(name, rddModeGenerationPrefix) || !strings.HasSuffix(name, rddModeGenerationSuffix) {
		return 0, false
	}
	digits := strings.TrimSuffix(strings.TrimPrefix(name, rddModeGenerationPrefix), rddModeGenerationSuffix)
	if len(digits) != rddModeGenerationDigits {
		return 0, false
	}
	generation, err := strconv.Atoi(digits)
	if err != nil || generation < 1 || generation > rddModeMaxGeneration {
		return 0, false
	}
	return generation, true
}
