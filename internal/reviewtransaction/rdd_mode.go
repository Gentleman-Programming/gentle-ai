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

	// ErrRDDModePartiallyApplied reports a clone-scope decision this build
	// recorded but could not publish at the readable location a coexisting
	// gentle-ai reads. It is the opposite of a fallback: it exists so a
	// half-applied kill switch can never be reported as a working one.
	ErrRDDModePartiallyApplied = errors.New("clone-local review mode was not applied for every gentle-ai on this machine")

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

// RDDModeReach reports how far a clone-scope write actually reached.
//
// The kill switch is a decision about this machine, not about one build, and
// #3284 is what happens when a write reports plain success for less than that:
// a modern binary published the decision only under the relocated switch root,
// every gentle-ai installed before that relocation kept reading its own
// location, and those builds went on enforcing review while the operator had
// been told the switch was off.
type RDDModeReach string

const (
	// RDDModeReachUnreported is the read-only projection's answer. Resolving a
	// mode never writes and never probes the other location, so it asserts
	// nothing about reach instead of guessing.
	RDDModeReachUnreported RDDModeReach = ""
	// RDDModeReachMachine means every gentle-ai installed on this machine
	// observes the decision.
	RDDModeReachMachine RDDModeReach = "machine"
	// RDDModeReachThisBuild means only builds that read the relocated switch
	// root observe it, because the pre-relocation location could not be
	// reached. A gentle-ai installed before the relocation keeps reading the
	// value it already has there -- which is also the value it fails closed to
	// when that location is unreadable, so nothing is relaxed and nothing is
	// silently claimed.
	RDDModeReachThisBuild RDDModeReach = "this_build"
)

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
	Reach      RDDModeReach  `json:"reach,omitempty"`
}

// Enabled reports whether new receipt-driven development may start.
func (status RDDModeStatus) Enabled() bool { return status.Effective == RDDModeOn }

// RDDDisabledError is the typed rejection returned while the kill switch is
// off. No agent may retry past it, reactivate it, or fall back around it.
type RDDDisabledError struct {
	Operation RDDOperation
	Source    RDDModeSource
}

// Error names the exact command that turns reviews on, scoped to the source
// that actually decided. Refusing here is correct -- either the operator asked
// for reviews to be off, or nobody ever opted in -- but a refusal that exits
// non-zero and names no runnable continuation is the one shape this project
// does not ship. The scope is derived rather than generic so the operator does
// not have to work out which source they need to change. The wording says "on"
// rather than "back on" because receipt-driven development is opt-in: the most
// common refusal is a fresh install where reviews were never on to begin with.
func (err *RDDDisabledError) Error() string {
	message := fmt.Sprintf("%v: %s is rejected because the %s mode source keeps it off",
		ErrRDDDisabled, rddOperationSubject(err.Operation), err.Source)
	// A mutation refuses against authority that already exists, so the operator
	// needs one fact a start never has to carry: their in-flight review survived
	// the refusal. It is stated before the continuation because it answers a
	// different question than "how do I proceed".
	if err.Operation == RDDOperationMutate {
		message += "; the review is frozen, not discarded"
	}
	enable := reviewModeEnableForSource(err.Source)
	if err.Operation == RDDOperationMutate {
		return fmt.Sprintf("%s; turn reviews on with %s to continue it from where it stopped", message, enable)
	}
	return fmt.Sprintf("%s; turn reviews on with %s", message, enable)
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

// reviewModeEnableForSource names the exact `gentle-ai review mode enable`
// commands that turn reviews on. Receipt-driven development is opt-in, so the
// default source is not an absence of a decision the operator can act on: it is
// the ordinary state of an install nobody configured, and it resolves the same
// way a global opinion does. It answers "global" for that reason, and because
// global is the only scope that can turn reviews on at all -- a clone may
// disable for itself but may never require review for the user, so pointing a
// never-configured operator at --scope=clone would name a command that cannot
// do what the refusal just asked them to do. A clone-local off is the one
// source needing two commands: --scope=clone clears the override, but clearing
// it only lands on the global source, which an opt-in install has no reason to
// have turned on -- so naming that scope alone was a dead end.
func reviewModeEnableForSource(source RDDModeSource) string {
	const enable = "gentle-ai review mode enable --scope="
	if source == RDDModeSourceCloneLocal {
		return enable + "global then " + enable + "clone"
	}
	return enable + "global"
}

func (err *RDDDisabledError) Unwrap() error { return ErrRDDDisabled }

// RDDModePartialApplyError reports a clone-scope write that applied for this
// build and did not apply at the readable pre-relocation location, so two
// gentle-ai installations on this machine now disagree about whether reviews
// run. It is raised only when that location was reachable and its publish
// failed: an unreachable one is reported as reach instead, because refusing
// there would re-close the exit #2882 opened.
//
// It deliberately does not unwrap to its cause. A caller that classified this
// by the cause would report the other location's local problem and lose the
// one fact the operator has to act on. The sentinel is enough to recognise it.
type RDDModePartialApplyError struct {
	Mode  RDDMode
	Cause error
}

func (err *RDDModePartialApplyError) Error() string {
	decision, verb := "no longer disables", "enable"
	if err.Mode == RDDModeOff {
		decision, verb = "disables", "disable"
	}
	return fmt.Sprintf(
		"%v: this clone %s receipt-driven development for this gentle-ai, but publishing the same decision under gentle-ai/%s/%s/%s/%s failed, so a gentle-ai installed before the switch moved still reads the value it already has there and keeps enforcing it: %v; rerun `gentle-ai review mode %s --scope clone` to publish it in both places",
		ErrRDDModePartiallyApplied,
		decision,
		rddModeLegacySwitchDirectory,
		rarAuthorityDirectory,
		rarAuthorityVersion,
		rddModeDirectory,
		err.Cause,
		verb,
	)
}

func (err *RDDModePartialApplyError) Unwrap() error { return ErrRDDModePartiallyApplied }

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
	globalMode, globalErr := normalizeRDDMode(global.Value)
	if globalErr != nil {
		return failedClosedRDDModeStatus(RDDModeSourceGlobal), globalErr
	}
	currentStatus, currentErr := ResolveRDDMode(ctx, repo, global)
	if currentErr == nil && strings.TrimSpace(expectedRevision) != currentStatus.Revision {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), fmt.Errorf(
			"%w: expected %q but the clone-local head is %q", ErrRDDModeRevisionMismatch, expectedRevision, currentStatus.Revision)
	}
	if currentErr == nil && mode == RDDModeUnset && globalMode == RDDModeOff {
		// Clearing this clone's off-only override cannot enable review while the
		// global source remains off, so refuse without publishing a generation.
		return currentStatus, &RDDDisabledError{Operation: RDDOperationStart, Source: RDDModeSourceGlobal}
	}
	// A request that matches the mode this clone already carries publishes no
	// new generation of its own -- but it is not finished until every gentle-ai
	// on this machine carries it too, so it goes on to the mirror below rather
	// than returning here.
	alreadyDecided := currentErr == nil && ((mode == RDDModeOff && currentStatus.CloneLocal == RDDModeOff) ||
		(mode == RDDModeUnset && currentStatus.CloneLocal == RDDModeUnset))
	if alreadyDecided && currentStatus.Revision == "" {
		// This clone holds no override in either location and is not being asked
		// for one. There is no decision to record and none to mirror, so this
		// stays the one path that creates nothing at all.
		return currentStatus, nil
	}
	if currentErr != nil && !errors.Is(currentErr, ErrRDDModeCorrupt) {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), currentErr
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

	// Every clone-scope write also publishes into the pre-relocation location,
	// because that is where the other gentle-ai installations on this machine
	// read the switch. Writers from before the relocation lock that path;
	// taking this build's root first and that one second serialises either
	// version of gentle-ai without a lock-order cycle.
	mirror := openCloneLocalRDDModeMirror(ctx, repo)
	defer mirror.release()

	head, present, err := readCloneLocalRDDOverrideHead(dir)
	repairingCurrentHead := false
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
		repairingCurrentHead = true
	}
	if !present && !repairingCurrentHead && mirror.available {
		// Legacy records are immutable forensic evidence. A valid one may advance
		// only by publishing its successor in the switch-owned authority root;
		// a damaged legacy record must never be shadowed by a fresh root.
		if mirror.readErr != nil {
			return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), mirror.readErr
		}
		if mirror.present {
			head, present = mirror.record, true
		}
	}
	current := ""
	if present {
		current = head.Revision
	}
	if strings.TrimSpace(expectedRevision) != current {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), fmt.Errorf(
			"%w: expected %q but the clone-local head is %q", ErrRDDModeRevisionMismatch, expectedRevision, current)
	}
	if alreadyDecided && (!mirror.available || mirror.decides(persisted)) {
		// Both readable locations already carry this decision, or the other one
		// cannot be reached at all and no write would change what it reports.
		// Publishing another generation would move the compare-and-set token
		// for nothing.
		return rddModeWriteStatus(globalMode, head, present, mirror.reach()), nil
	}
	// The generation is a slot number in both locations, so it clears whichever
	// of them has published further. A pre-relocation gentle-ai that wrote its
	// own generations must not have one of them overwritten, and this build's
	// own head must still advance.
	generation := head.Generation + 1
	if mirror.available && mirror.head >= generation {
		generation = mirror.head + 1
	}
	if generation > rddModeMaxGeneration {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), errors.New("clone-local review mode generation space is exhausted")
	}

	record := rddModeOverrideRecord{
		Schema:           rddModeOverrideSchema,
		Generation:       generation,
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
	//
	// This build's own root is published first and never waits on the other
	// location's health. #2882 is exactly what happens when the switch can be
	// held hostage by the review authority tree, and ordering the mirror ahead
	// of it would reintroduce that through the back door.
	if err := publishPrivateRARImmutable(filepath.Join(dir, rddModeGenerationName(record.Generation)), payload); err != nil {
		return failedClosedRDDModeStatus(RDDModeSourceCloneLocal), err
	}
	if !mirror.available {
		return rddModeWriteStatus(globalMode, record, true, RDDModeReachThisBuild), nil
	}
	// The same exact bytes at the same slot: a pre-relocation gentle-ai parses,
	// digests, and canonicalises this record identically, so the two locations
	// hold one decision rather than two that have to be reconciled.
	if err := publishPrivateRARImmutable(filepath.Join(mirror.dir, rddModeGenerationName(record.Generation)), payload); err != nil {
		return rddModeWriteStatus(globalMode, record, true, RDDModeReachThisBuild),
			&RDDModePartialApplyError{Mode: mode, Cause: err}
	}
	return rddModeWriteStatus(globalMode, record, true, RDDModeReachMachine), nil
}

// cloneLocalRDDModeMirror is the pre-relocation copy of this clone's override,
// held open under its own lock for the duration of one write.
//
// It is deliberately best-effort about reachability and strict about writing.
// A location this build cannot open is also a location a pre-relocation
// gentle-ai fails closed on, so refusing there would only re-close the exit
// #2882 opened; a location that opens and then refuses the publish is a real
// half-applied switch and is reported as one.
type cloneLocalRDDModeMirror struct {
	dir       string
	lock      *storeLock
	available bool
	head      int
	record    rddModeOverrideRecord
	present   bool
	readErr   error
}

// cloneLocalRDDModeMirrorSlotScan reads the mirror's published slot numbers.
//
// It is a variable for the same reason rarPrivateDirectoryMkdir is: the
// failure it has to survive cannot be produced by a test on ext4 or tmpfs. A
// directory that fails the owner-only no-follow walk never becomes a mirror at
// all, so the only way to reach an unreadable head is a directory that
// validates and then refuses readdir(2) -- a permission change racing the
// walk, EIO, or ESTALE on a network mount. Production always uses the real
// scan.
var cloneLocalRDDModeMirrorSlotScan = cloneLocalRDDOverrideHeadGeneration

func openCloneLocalRDDModeMirror(ctx context.Context, repo string) *cloneLocalRDDModeMirror {
	mirror := &cloneLocalRDDModeMirror{}
	// An unreachable location is not an error here. It is reported as reach,
	// because a location this build cannot open is one a pre-relocation
	// gentle-ai cannot read either, and refusing over it would re-close the
	// clone-scoped exit #2882 opened.
	dir, err := cloneLocalRDDModeLegacyRoot(ctx, repo, true)
	if err != nil {
		return mirror
	}
	lock, err := acquireRARAuthorityLock(ctx, filepath.Join(dir, rddModeLockName))
	if err != nil {
		return mirror
	}
	mirror.dir, mirror.lock, mirror.available = dir, lock, true
	// The slot scan is kept separate from the record read: an unparseable head
	// still occupies its slot, and the successor that supersedes it has to
	// clear that slot rather than collide with it.
	head, headErr := cloneLocalRDDModeMirrorSlotScan(dir)
	if headErr != nil {
		// A location whose slots cannot be enumerated has an UNKNOWN head, not
		// an empty one, and the difference is the whole switch. Leaving it
		// available with head zero published this build's next slot number
		// into it: below the record a pre-relocation gentle-ai actually reads,
		// invisible to the only reader it was written for, and reported as
		// machine-wide reach -- #3284 with a success message on it.
		//
		// So it joins the location that cannot be opened at all. Not reached
		// is the honest answer, the write still applies here, the operator is
		// told reach=this_build, and nothing is published into a layout this
		// build could not read. The lock stays held and released for the rest
		// of this write, because the location itself is real.
		//
		// The scan error is dropped rather than carried, exactly like the two
		// opens above: readErr is the record's own problem, consulted only
		// while this location is still writable, and an unavailable mirror is
		// never written to or asked what it decides.
		mirror.available = false
		return mirror
	}
	mirror.head = head
	mirror.record, mirror.present, mirror.readErr = readCloneLocalRDDOverrideHead(dir)
	return mirror
}

// decides reports whether a pre-relocation gentle-ai reading this location
// already reaches the same conclusion as the requested mode. An absent record
// is that conclusion for inherit: those builds spell "this clone holds no
// override" by finding nothing here.
func (mirror *cloneLocalRDDModeMirror) decides(mode string) bool {
	if !mirror.available || mirror.readErr != nil {
		return false
	}
	if !mirror.present {
		return mode == rddModeOverrideInherit
	}
	return mirror.record.Mode == mode
}

func (mirror *cloneLocalRDDModeMirror) reach() RDDModeReach {
	if mirror.available {
		return RDDModeReachMachine
	}
	return RDDModeReachThisBuild
}

func (mirror *cloneLocalRDDModeMirror) release() {
	if mirror.lock != nil {
		_ = mirror.lock.release()
	}
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
		// Receipt-driven development is opt-in. Nobody expressed an opinion
		// here, and an install nobody configured must not start reviewing on
		// its own: the only way to "on" is an explicit global enable, which the
		// case above reads back untouched across an upgrade.
		status.Effective, status.Source = RDDModeOff, RDDModeSourceDefault
	}
	return status
}

// rddModeWriteStatus is the projection a clone-scope write returns. It differs
// from the read-only one by exactly one fact: how far the decision reached.
func rddModeWriteStatus(
	globalMode RDDMode,
	override rddModeOverrideRecord,
	present bool,
	reach RDDModeReach,
) RDDModeStatus {
	status := rddModeStatus(globalMode, override, present)
	status.Reach = reach
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

// rddModeSwitchDirectory is the kill switch's own root, a SIBLING of
// review-transactions rather than a descendant of it.
//
// The override used to nest inside the review authority tree, deliberately, so
// path safety, permissions, and private IO could reuse that tree's helpers
// instead of inventing a second path policy. The reuse was fine; the nesting
// was not. It made the switch inherit every failure mode of the thing the
// switch exists to turn off, and #2882 is the consequence: with authority
// failing RAR path safety, the documented clone-scoped exit refused, persisted
// nothing, and left reviews on. That exit is what the stop-reason table names
// for most unrecoverable review states, so those codes were pointing at a door
// that was locked exactly when they pointed at it.
//
// Sitting beside review-transactions keeps every helper and every permission
// rule, and drops only the dependency that had no reason to exist.
const rddModeSwitchDirectory = "review-mode"

// rddModeLegacySwitchDirectory is where the switch lived before #2882 moved it,
// and therefore where every gentle-ai released before that move still reads it.
// It is not history: those builds coexist with this one on the same machine and
// share the same Git common directory, so a decision absent from this location
// is a decision they never see (#3284).
const rddModeLegacySwitchDirectory = "review-transactions"

// cloneLocalRDDModeRoot derives the override directory from the exact Git
// common directory, under the switch's own root.
func cloneLocalRDDModeRoot(ctx context.Context, repo string, create bool) (string, error) {
	identity, err := cloneLocalRDDModeIdentity(ctx, repo)
	if err != nil {
		return "", err
	}
	base := filepath.Join(identity.GitCommonDir, "gentle-ai", rddModeSwitchDirectory, rarAuthorityDirectory, rarAuthorityVersion)
	if err := ensureRARSwitchRoot(identity.GitCommonDir, base, create); err != nil {
		return "", err
	}
	dir := filepath.Join(base, rddModeDirectory)
	if err := ensurePrivateRARDirectoryTree(base, dir, create); err != nil {
		return "", err
	}
	return dir, nil
}

// cloneLocalRDDModeLegacyRoot is the pre-#2882 location inside the review
// authority tree. Reads pass create=false and are best-effort: overrides
// written before that change must keep deciding, and a clone that never
// disabled has nothing here.
//
// Writes pass create=true. The location is not merely history: it is where
// every gentle-ai installed before the relocation still looks, so a clone-scope
// decision that is never published here is invisible to those builds (#3284).
func cloneLocalRDDModeLegacyRoot(ctx context.Context, repo string, create bool) (string, error) {
	identity, err := cloneLocalRDDModeIdentity(ctx, repo)
	if err != nil {
		return "", err
	}
	base := filepath.Join(
		identity.GitCommonDir,
		"gentle-ai",
		rddModeLegacySwitchDirectory,
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

func cloneLocalRDDModeIdentity(ctx context.Context, repo string) (reviewRepositoryIdentityRecord, error) {
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		// A bare repository already states its own refusal and names its own
		// recovery. Wrapping it in this internal concern would misattribute the
		// failure to a kill switch the operator never touched.
		var bare *BareRepositoryError
		if errors.As(err, &bare) {
			return reviewRepositoryIdentityRecord{}, err
		}
		return reviewRepositoryIdentityRecord{}, fmt.Errorf("resolve review mode repository identity: %w", err)
	}
	return reviewRepositoryIdentityRecordFromLease(lease), nil
}

// cloneLocalRDDModeReadRoot reports the directory that currently decides this
// clone's override: the switch's own root when it holds a generation, and
// otherwise the legacy location so a disable written before #2882 keeps
// deciding.
//
// A legacy root this build cannot reach resolves to "no override" rather than
// an error, and that never relaxes anything. Before this change an unreachable
// authority tree already failed the whole resolution closed, which resolves to
// managed and keeps reviews on; reporting no clone-local override reaches the
// same effective mode by the same default, and unlike the old behavior it
// leaves the operator able to write one.
func cloneLocalRDDModeReadRoot(ctx context.Context, repo string) (string, error) {
	dir, err := cloneLocalRDDModeRoot(ctx, repo, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err == nil {
		head, headErr := cloneLocalRDDOverrideHeadGeneration(dir)
		if headErr != nil {
			return "", headErr
		}
		if head > 0 {
			return dir, nil
		}
	}
	legacy, legacyErr := cloneLocalRDDModeLegacyRoot(ctx, repo, false)
	if legacyErr != nil {
		return "", nil
	}
	if head, headErr := cloneLocalRDDOverrideHeadGeneration(legacy); headErr != nil || head == 0 {
		return "", nil
	}
	return legacy, nil
}

func readCloneLocalRDDOverride(ctx context.Context, repo string) (rddModeOverrideRecord, bool, error) {
	dir, err := cloneLocalRDDModeReadRoot(ctx, repo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return rddModeOverrideRecord{}, false, nil
		}
		return rddModeOverrideRecord{}, false, err
	}
	if dir == "" {
		return rddModeOverrideRecord{}, false, nil
	}
	return readCloneLocalRDDOverrideHead(dir)
}

// CloneLocalRDDModeRecordPath reports the clone-local override file that
// currently decides this clone's mode, so a refusal can name the exact file
// holding an unreadable value instead of merely describing it. It is strictly
// read-only, never creates state, and reports "" when this clone holds no
// override at all.
func CloneLocalRDDModeRecordPath(ctx context.Context, repo string) (string, error) {
	dir, err := cloneLocalRDDModeReadRoot(ctx, repo)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if dir == "" {
		return "", nil
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
