package sddstatus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
)

// Keep the existing on-disk chain and schema so historical grants remain readable.
const (
	RuntimeStatusSchema             = "gentle-ai.sdd-runtime-status/v1"
	runtimeRecordSchema             = "gentle-ai.sdd-runtime-record/v1"
	runtimeOperationGrant           = "authority/grant"
	maximumRuntimeGrantRoots        = 32
	maximumRuntimeRecordBytes       = 1 << 20
	maximumRuntimeChainRecords      = 10_000
	runtimeLockAcquireAttempts      = 3
	encodedRuntimeChangeNamespace   = "_encoded"
	encodedRuntimeChangeDigestWidth = 32
	runtimeLedgerStatusPointer      = "preserve the historical records and ask a maintainer to inspect edit authority; re-enter with `gentle-ai sdd-status --cwd <repo> --json`"
)

var (
	ErrRuntimeRevisionConflict      = errors.New("SDD runtime ledger revision conflict")
	ErrRuntimeConcurrentUpdate      = errors.New("SDD runtime ledger is concurrently updated")
	ErrRuntimeRequestConflict       = errors.New("SDD runtime request identifier was reused with different inputs")
	runtimePublishRecord            = reviewtransaction.PublishFileNoReplace
	runtimeReplaceHead              = reviewtransaction.ReplaceFileAtomic
	runtimeSyncDirectory            = reviewtransaction.SyncReviewDirectory
	runtimeAcquireAuthorityFileLock = reviewtransaction.AcquireAuthorityFileLock
	runtimeGrantClock               = func() string { return time.Now().UTC().Format(time.RFC3339Nano) }
	runtimeRevisionPattern          = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	runtimeRequestIDPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	legacyRuntimeChangePattern      = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	runtimeChangeUnsafeBytePattern  = regexp.MustCompile(`[\x00-\x1f\x7f\\:]`)
)

type RuntimeRevisionConflictError struct {
	Expected string
	Current  string
}

type RuntimePublicationError struct {
	Revision   string
	Committed  bool
	Cause      error
	staleGrant bool
}

type GrantRootsRequest struct {
	ExpectedRevision string   `json:"expected_revision"`
	RequestID        string   `json:"request_id"`
	Roots            []string `json:"roots"`
	Reason           string   `json:"reason"`
	Actor            string   `json:"actor"`
	// ChangeInstance is filled by Grant from the store's own ForInstance
	// identity (#2540 S5) so the request digest binds the exact instance the
	// grant belongs to. A caller-supplied value must equal the store's; Grant
	// refuses a mismatch rather than silently rebinding authority.
	ChangeInstance string `json:"change_instance"`
}

// RuntimeStore retains only per-instance edit grants in the existing immutable chain.
type RuntimeStore struct {
	Dir                string
	Repo               string
	Workspace          string
	Change             string
	commonDir          string
	instance           string
	grantInstanceCheck func() error
	grantMutation      bool
}

type runtimeRecord struct {
	Schema           string             `json:"schema"`
	Change           string             `json:"change"`
	PreviousRevision string             `json:"previous_revision"`
	Operation        string             `json:"operation"`
	RequestID        string             `json:"request_id"`
	RequestDigest    string             `json:"request_digest"`
	Begin            json.RawMessage    `json:"begin,omitempty"`
	Finish           json.RawMessage    `json:"finish,omitempty"`
	Reset            json.RawMessage    `json:"reset,omitempty"`
	Rescope          json.RawMessage    `json:"rescope,omitempty"`
	Supersede        json.RawMessage    `json:"supersede,omitempty"`
	Repair           json.RawMessage    `json:"repair,omitempty"`
	Advance          json.RawMessage    `json:"advance,omitempty"`
	Handoff          json.RawMessage    `json:"handoff,omitempty"`
	Grant            *runtimeGrantEvent `json:"grant,omitempty"`
}

type runtimeGrantEvent struct {
	Roots     []string `json:"roots"`
	Actor     string   `json:"actor"`
	Reason    string   `json:"reason"`
	GrantedAt string   `json:"granted_at"`
	// Instance is the change-instance identity this grant belongs to (#2540
	// S5), digest-bound like Roots/Actor/Reason: a record whose identity was
	// altered, stripped, or forged after publication no longer matches the
	// bound RequestDigest, so replay refuses the chain. Replay projects the
	// grant into GrantedRoots only for a store bound to the same identity,
	// which is what makes an archived name's reuse unable to resurrect the
	// archived change's authority. Required: no released writer ever emitted
	// an instance-less grant record (RuntimeStore.Grant was unreachable
	// between #2553 and this slice), so an empty value is a mutated record,
	// not a legacy one.
	Instance string `json:"instance"`
}

type runtimeRequestReceipt struct {
	Digest   string
	Revision string
}

type RuntimeRepositoryRequiredError struct {
	Workspace string
	Cause     error
}

type RuntimeRecordRejectedError struct {
	Condition string
	Revision  string
	Path      string
}

type RuntimeRecordSchemaUnsupportedError struct {
	Revision string
	Schema   string
}

// RuntimeStatus projects edit authority only; old attempt records never govern execution.
type RuntimeStatus struct {
	Schema       string   `json:"schema"`
	Change       string   `json:"change"`
	Revision     string   `json:"revision"`
	GrantedRoots []string `json:"granted_roots,omitempty"`
}
type runtimeReplay struct {
	Status   RuntimeStatus
	Requests map[string]runtimeRequestReceipt
	Instance string
}

func (err *RuntimeRevisionConflictError) Error() string {
	return fmt.Sprintf("%v: expected %q, current %q; retry with --expected-revision %q", ErrRuntimeRevisionConflict, err.Expected, err.Current, err.Current)
}

func (err *RuntimeRevisionConflictError) Unwrap() error { return ErrRuntimeRevisionConflict }

func (err *RuntimePublicationError) Error() string {
	if err.staleGrant {
		return fmt.Sprintf("SDD historical grant %s is committed but not current authority; re-read status before granting: %v", err.Revision, err.Cause)
	}
	return fmt.Sprintf("SDD runtime ledger publication for %s requires exact replay: %v", err.Revision, err.Cause)
}

func (err *RuntimePublicationError) Unwrap() error { return err.Cause }

func (err *RuntimeRepositoryRequiredError) Error() string {
	return fmt.Sprintf("SDD edit authority needs a Git repository because its authority lives in the Git common directory, and %s is not inside one; run `git init` in that workspace (or run from the repository that contains it), then rerun the same `gentle-ai sdd-attempt` command", err.Workspace)
}

func (err *RuntimeRepositoryRequiredError) Unwrap() error { return err.Cause }

func (err *RuntimeRecordRejectedError) Error() string {
	if err.Revision == "" {
		return fmt.Sprintf("SDD runtime record rejected (condition %s); %s", err.Condition, runtimeLedgerStatusPointer)
	}
	if err.Path == "" {
		return fmt.Sprintf("SDD runtime record rejected (condition %s, revision %s); %s", err.Condition, err.Revision, runtimeLedgerStatusPointer)
	}
	// No repair command exists here by design (human authority): nothing may
	// rewrite or drop a published chain record; a maintainer inspects or
	// removes the named file, then the status pointer is the read-only re-entry.
	return fmt.Sprintf("SDD runtime record rejected (condition %s, revision %s); the record file is %s; a maintainer must inspect that record without rewriting history, then %s", err.Condition, err.Revision, err.Path, runtimeLedgerStatusPointer)
}

func (err *RuntimeRecordSchemaUnsupportedError) Error() string {
	return fmt.Sprintf("SDD runtime revision %s declares \"schema\" %s, newer than this binary supports (%s); run `gentle-ai update` to install a build that reads it, then rerun the same `gentle-ai sdd-attempt` command", err.Revision, err.Schema, runtimeRecordSchema)
}

func (store RuntimeStore) ForInstance(instance string) (RuntimeStore, error) {
	if instance == "" {
		return RuntimeStore{}, errors.New("change-instance identity must not be empty") // refusal:by-design operator-knowledge: only the caller knows the change instance this session serves; retry with the instance identity the status layer derived
	}
	if err := validateRuntimeText(instance, 128); err != nil {
		return RuntimeStore{}, fmt.Errorf("invalid change-instance identity: %w", err)
	}
	store.instance = instance
	return store, nil
}

func OpenRuntimeStore(ctx context.Context, repo, change string) (RuntimeStore, error) {
	if !validRuntimeChange(change) {
		return RuntimeStore{}, fmt.Errorf("invalid SDD change name %q; want a non-empty identity of at most 96 characters with no control characters, backslash, colon, or \".\"/\"..\" path segment; run `gentle-ai sdd-status --cwd <repo> --json` to read the resolved changeName", change)
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		if abs, absErr := filepath.Abs(repo); absErr == nil && !workspaceHasGitMetadata(abs) {
			return RuntimeStore{}, &RuntimeRepositoryRequiredError{Workspace: abs, Cause: err}
		}
		return RuntimeStore{}, err
	}
	workspace, err := filepath.Abs(repo)
	if err != nil {
		return RuntimeStore{}, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return RuntimeStore{}, err
	}
	probe, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, "sdd-runtime-probe")
	if err != nil {
		return RuntimeStore{}, err
	}
	commonDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(probe.Dir))))
	dir := runtimeChangeLedgerDir(filepath.Join(commonDir, "gentle-ai", "sdd-runtime"), change)
	return RuntimeStore{Dir: dir, Repo: root, Workspace: workspace, Change: change, commonDir: commonDir}, nil
}

func validRuntimeChange(change string) bool {
	if change == "" || len(change) > 96 || runtimeChangeUnsafeBytePattern.MatchString(change) {
		return false
	}
	for _, segment := range strings.Split(change, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func legacyRuntimeChangeDir(change string) bool {
	return len(change) <= 96 && legacyRuntimeChangePattern.MatchString(change)
}

func runtimeChangeLedgerDir(base, change string) string {
	if legacyRuntimeChangeDir(change) {
		return filepath.Join(base, "v1", change)
	}
	digest := strings.TrimPrefix(runtimeValueHash("gentle-ai.sdd-runtime-change-identity/v1", change), "sha256:")
	return filepath.Join(base, "v1", encodedRuntimeChangeNamespace, runtimeChangeEncodedLabel(change)+"-"+digest[:encodedRuntimeChangeDigestWidth])
}

func runtimeChangeEncodedLabel(change string) string {
	lower := strings.ToLower(change)
	label := make([]byte, len(lower))
	for i := 0; i < len(lower); i++ {
		switch c := lower[i]; {
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_':
			label[i] = c
		default:
			label[i] = '_'
		}
	}
	return string(label)
}

func (store RuntimeStore) Status() (RuntimeStatus, error) {
	replay, err := store.load()
	return replay.Status, err
}

func (store RuntimeStore) Grant(ctx context.Context, request GrantRootsRequest) (RuntimeStatus, error) {
	if store.instance == "" {
		return RuntimeStatus{}, errors.New("grant requires a change-instance identity; derive the store with ForInstance first") // refusal:by-design operator-knowledge: only the caller knows which change instance this grant authorizes; derive the store with ForInstance and retry
	}
	if request.ChangeInstance != "" && request.ChangeInstance != store.instance {
		return RuntimeStatus{}, errors.New("grant request change-instance does not equal the store's instance identity") // refusal:by-design operator-knowledge: the request and the store name different change instances; retry with one coherent instance identity
	}
	request.ChangeInstance = store.instance
	request, err := normalizeGrantRootsRequest(request)
	if err != nil {
		return RuntimeStatus{}, err
	}
	digest := runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", request)
	grantedAt := runtimeGrantClock()
	store.grantMutation = true
	return store.mutate(ctx, request.ExpectedRevision, request.RequestID, digest, func(runtimeReplay) (runtimeRecord, error) {
		return runtimeRecord{Operation: runtimeOperationGrant, Grant: &runtimeGrantEvent{
			Roots: request.Roots, Actor: request.Actor, Reason: request.Reason, GrantedAt: grantedAt,
			Instance: request.ChangeInstance,
		}}, nil
	})
}

func (store RuntimeStore) mutate(
	ctx context.Context,
	expected, requestID, requestDigest string,
	build func(runtimeReplay) (runtimeRecord, error),
) (result RuntimeStatus, err error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.ensureDirectories(); err != nil {
		return RuntimeStatus{}, err
	}
	// acquireLock already maps contention to ErrRuntimeConcurrentUpdate; the
	// re-wrap that used to live here duplicated the prefix and flattened the
	// underlying contention proof back out of the chain (1861).
	lock, err := store.acquireLock()
	if err != nil {
		return RuntimeStatus{}, err
	}
	defer lock.Release()

	receiptRevision := ""
	if store.grantMutation && store.grantInstanceCheck != nil {
		if err := store.grantInstanceCheck(); err != nil {
			return RuntimeStatus{}, err
		}
		// External replacement can leave immutable history; never return detected stale authority.
		defer func() {
			if err == nil {
				if mismatch := store.grantInstanceCheck(); mismatch != nil {
					if receiptRevision == "" {
						receiptRevision = result.Revision
					}
					err = &RuntimePublicationError{Revision: receiptRevision, Committed: true, Cause: mismatch, staleGrant: true}
					result = RuntimeStatus{}
				}
			}
		}()
	}
	replay, err := store.load()
	if err != nil {
		return RuntimeStatus{}, err
	}
	if receipt, ok := replay.Requests[requestID]; ok {
		receiptRevision = receipt.Revision
		if receipt.Digest != requestDigest {
			return RuntimeStatus{}, ErrRuntimeRequestConflict
		}
		if err := store.syncReplay(); err != nil {
			return RuntimeStatus{}, &RuntimePublicationError{Revision: receipt.Revision, Committed: true, Cause: err}
		}
		return replay.Status, nil
	}
	if replay.Status.Revision != expected {
		return RuntimeStatus{}, &RuntimeRevisionConflictError{Expected: expected, Current: replay.Status.Revision}
	}
	record, err := build(replay)
	if err != nil {
		return RuntimeStatus{}, err
	}
	record.Schema = runtimeRecordSchema
	record.Change = store.Change
	record.PreviousRevision = expected
	record.RequestID = requestID
	record.RequestDigest = requestDigest
	if err := validateRuntimeRecordShape(record); err != nil {
		return RuntimeStatus{}, err
	}
	return store.commitRecordLocked(record)
}

func (store RuntimeStore) acquireLock() (*reviewtransaction.AuthorityFileLock, error) {
	lockPath := filepath.Join(store.Dir, "LOCK")
	var lastErr error
	for attempt := 0; attempt < runtimeLockAcquireAttempts; attempt++ {
		lock, err := runtimeAcquireAuthorityFileLock(lockPath)
		if err == nil {
			return lock, nil
		}
		if errors.Is(err, reviewtransaction.ErrConcurrentUpdate) {
			// Wrapped rather than flattened so the contention proof survives
			// (1861). Both callers of acquireLock -- RepairConsecutiveRescope and
			// mutate -- refuse here strictly before commitRecordLocked, so a
			// refusal at acquisition wrote nothing, and a caller must be told
			// that instead of being handed an unknown mutation outcome. The
			// rendered text is identical to the previous %v form.
			return nil, fmt.Errorf("%w: %w", ErrRuntimeConcurrentUpdate, err)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		lastErr = err
		if attempt == runtimeLockAcquireAttempts-1 {
			break
		}
		if err := store.ensureDirectories(); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("acquire SDD runtime ledger lock %s after %d attempts: %w", pathquote.Quote(lockPath), runtimeLockAcquireAttempts, lastErr)
}

func (store RuntimeStore) commitRecordLocked(record runtimeRecord) (RuntimeStatus, error) {
	revision, payload, err := runtimeRecordRevision(record)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := store.publishRecord(revision, payload); err != nil {
		return RuntimeStatus{}, err
	}
	// Verify BEFORE committing (#2833). Replay a candidate chain that ends at
	// this record; HEAD advances only if that replay lands on the expected
	// revision. Previously HEAD moved first and the replay could only report a
	// state it had already made permanent, so a record the store's own
	// validator rejects was on the chain and every later read walked into it.
	// The wedge class disappears by construction rather than by catching it.
	//
	// A record that fails here stays on disk, unreferenced by HEAD. Records are
	// content-addressed and immutable, so an unreachable one is inert: the next
	// attempt at the same record re-publishes identical bytes.
	if candidate, err := store.loadRevision(revision); err != nil {
		return RuntimeStatus{}, fmt.Errorf("replay candidate SDD runtime record: %w", err)
	} else if candidate.Status.Revision != revision {
		// HEAD did not move, so the chain is intact and status is actionable.
		// This deliberately does not say "restore the store": nothing was
		// committed, which is the entire point of verifying first.
		return RuntimeStatus{}, errors.New("candidate SDD runtime record did not replay to its own revision; HEAD was not advanced and the chain is unchanged; " + runtimeLedgerStatusPointer)
	}
	if err := store.publishHead(revision); err != nil {
		return RuntimeStatus{}, err
	}
	if err := runtimeSyncDirectory(store.Dir); err != nil {
		return RuntimeStatus{}, &RuntimePublicationError{Revision: revision, Committed: true, Cause: fmt.Errorf("sync SDD runtime HEAD directory: %w", err)}
	}

	committed, err := store.load()
	if err != nil {
		return RuntimeStatus{}, &RuntimePublicationError{Revision: revision, Committed: true, Cause: fmt.Errorf("replay committed SDD runtime HEAD: %w", err)}
	}
	if committed.Status.Revision != revision {
		return RuntimeStatus{}, &RuntimePublicationError{Revision: revision, Committed: true, Cause: errors.New("committed SDD runtime HEAD did not replay to candidate revision")}
	}
	return committed.Status, nil
}

func (store RuntimeStore) load() (runtimeReplay, error) {
	head, exists, err := readRuntimeHead(filepath.Join(store.Dir, "HEAD"))
	if err != nil {
		return runtimeReplay{}, err
	}
	if !exists {
		head = ""
	}
	replay, err := store.loadRevision(head)
	if err != nil {
		return runtimeReplay{}, err
	}
	return replay, nil
}

func (store RuntimeStore) loadRevision(head string) (runtimeReplay, error) {
	replay := runtimeReplay{
		Status: RuntimeStatus{
			Schema: RuntimeStatusSchema, Change: store.Change,
		},
		Requests: map[string]runtimeRequestReceipt{},
		Instance: store.instance,
	}
	type revisionRecord struct {
		revision string
		record   runtimeRecord
	}
	reverse := make([]revisionRecord, 0, 16)
	seen := map[string]struct{}{}
	for revision := head; revision != ""; {
		if len(reverse) >= maximumRuntimeChainRecords {
			return runtimeReplay{}, errors.New("SDD runtime chain exceeds the bounded record count")
		}
		if _, duplicate := seen[revision]; duplicate {
			return runtimeReplay{}, errors.New("SDD runtime record predecessor cycle detected")
		}
		seen[revision] = struct{}{}
		record, err := store.loadRecord(revision)
		if err != nil {
			return runtimeReplay{}, err
		}
		reverse = append(reverse, revisionRecord{revision: revision, record: record})
		revision = record.PreviousRevision
	}
	for index := len(reverse) - 1; index >= 0; index-- {
		entry := reverse[index]
		if err := applyRuntimeRecord(store, &replay, entry.revision, entry.record); err != nil {
			return runtimeReplay{}, fmt.Errorf("replay SDD runtime revision %s: %w", entry.revision, err)
		}
	}
	if head != "" && replay.Status.Revision != head {
		return runtimeReplay{}, errors.New("SDD runtime HEAD does not equal replayed revision")
	}
	return replay, nil
}

func rejectRuntimeRecord(condition string) error {
	return &RuntimeRecordRejectedError{Condition: condition}
}

func withRuntimeRecordRevision(err error, revision string, path string) error {
	var rejected *RuntimeRecordRejectedError
	if err == nil || !errors.As(err, &rejected) || rejected.Revision != "" {
		return err
	}
	return &RuntimeRecordRejectedError{Condition: rejected.Condition, Revision: revision, Path: path}
}

func applyRuntimeRecord(store RuntimeStore, replay *runtimeReplay, revision string, record runtimeRecord) error {
	return withRuntimeRecordRevision(applyRuntimeRecordLocked(store, replay, revision, record), revision, store.recordPath(revision))
}

func applyRuntimeGrantEvent(replay *runtimeReplay, event *runtimeGrantEvent) {
	if replay.Instance == "" || event.Instance != replay.Instance {
		return
	}
	for _, root := range event.Roots {
		duplicate := false
		for _, granted := range replay.Status.GrantedRoots {
			if granted == root {
				duplicate = true
				break
			}
		}
		if !duplicate {
			replay.Status.GrantedRoots = append(replay.Status.GrantedRoots, root)
		}
	}
}

func normalizeGrantRootsRequest(request GrantRootsRequest) (GrantRootsRequest, error) {
	if request.ExpectedRevision != "" && !runtimeRevisionPattern.MatchString(request.ExpectedRevision) {
		return GrantRootsRequest{}, errors.New("expected runtime revision must be empty or sha256")
	}
	if !runtimeRequestIDPattern.MatchString(request.RequestID) {
		return GrantRootsRequest{}, errors.New("request_id must be a canonical lowercase identifier")
	}
	if len(request.Roots) < 1 || len(request.Roots) > maximumRuntimeGrantRoots {
		return GrantRootsRequest{}, fmt.Errorf("grant requires between 1 and %d roots", maximumRuntimeGrantRoots) // refusal:by-design operator-knowledge: only the caller knows which edit roots this change needs; retry with a bounded non-empty root list
	}
	canonical := make([]string, 0, len(request.Roots))
	seen := make(map[string]struct{}, len(request.Roots))
	for _, root := range request.Roots {
		if err := validateRuntimeText(root, 4096); err != nil {
			return GrantRootsRequest{}, fmt.Errorf("invalid grant root: %w", err)
		}
		resolved, err := filepath.Abs(root)
		if err == nil {
			resolved, err = filepath.EvalSymlinks(resolved)
		}
		if err != nil {
			return GrantRootsRequest{}, fmt.Errorf("resolve grant root %s: %w", pathquote.Quote(root), err)
		}
		if err := validateRuntimeText(resolved, 4096); err != nil {
			return GrantRootsRequest{}, fmt.Errorf("invalid canonical grant root: %w", err)
		}
		if _, duplicate := seen[resolved]; duplicate {
			continue
		}
		seen[resolved] = struct{}{}
		canonical = append(canonical, resolved)
	}
	request.Roots = canonical
	if err := validateRuntimeText(request.Reason, 500); err != nil {
		return GrantRootsRequest{}, fmt.Errorf("invalid grant reason: %w", err)
	}
	if err := validateRuntimeText(request.Actor, 128); err != nil {
		return GrantRootsRequest{}, fmt.Errorf("invalid grant actor: %w", err)
	}
	if request.ChangeInstance == "" {
		return GrantRootsRequest{}, errors.New("grant requires a change-instance identity") // refusal:by-design operator-knowledge: only the caller knows which change instance this grant authorizes; derive the store with ForInstance and retry
	}
	if err := validateRuntimeText(request.ChangeInstance, 128); err != nil {
		return GrantRootsRequest{}, fmt.Errorf("invalid grant change-instance identity: %w", err)
	}
	return request, nil
}

func validateRuntimeText(value string, maximum int) error {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("value must be non-empty, trimmed, single-line, and bounded")
	}
	return nil
}

func runtimeValueHash(domain string, value any) string {
	payload, _ := json.Marshal(value)
	sum := sha256.Sum256(append(append([]byte(domain), '\n'), payload...))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func runtimeRecordRevision(record runtimeRecord) (string, []byte, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return "", nil, err
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), payload, nil
}

func (store RuntimeStore) ensureDirectories() error {
	if filepath.Clean(store.commonDir) == "." || !filepath.IsAbs(store.commonDir) {
		return errors.New("SDD runtime common directory is invalid")
	}
	relative, err := filepath.Rel(store.commonDir, filepath.Join(store.Dir, "records"))
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("SDD runtime authority escapes the Git common directory")
	}
	current := store.commonDir
	segments := strings.Split(relative, string(filepath.Separator))
	created := make([]string, 0, len(segments))
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("SDD runtime authority contains an invalid path segment")
		}
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			mode := os.FileMode(0o700)
			// The shared gentle-ai container predates this private store and may
			// also hold review authority. New SDD runtime descendants remain 0700.
			if index == 0 && segment == "gentle-ai" {
				mode = 0o755
			}
			if err := os.Mkdir(current, mode); err != nil {
				if !os.IsExist(err) {
					return err
				}
			} else {
				created = append(created, current)
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("SDD runtime authority path is not a private directory")
		}
	}
	if filepath.Clean(current) != filepath.Clean(filepath.Join(store.Dir, "records")) {
		return errors.New("SDD runtime authority path resolution is inconsistent")
	}
	for _, path := range created {
		if err := runtimeSyncDirectory(filepath.Dir(path)); err != nil {
			return fmt.Errorf("sync parent of SDD runtime authority directory: %w", err)
		}
	}
	if err := os.Chmod(store.Dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(store.Dir, "records"), 0o700); err != nil {
		return err
	}
	return nil
}

func (store RuntimeStore) publishRecord(revision string, payload []byte) error {
	recordsDir := filepath.Join(store.Dir, "records")
	path := store.recordPath(revision)
	temp, err := os.CreateTemp(recordsDir, ".record-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err == nil {
		_, err = temp.Write(payload)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := runtimePublishRecord(tempPath, path); err != nil {
		if !os.IsExist(err) {
			return err
		}
		existing, readErr := readBoundedRuntimeFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, payload) {
			return errors.New("existing immutable SDD runtime record differs from its revision")
		}
	}
	if err := runtimeSyncDirectory(recordsDir); err != nil {
		return fmt.Errorf("sync immutable SDD runtime record directory: %w", err)
	}
	return nil
}

func (store RuntimeStore) publishHead(revision string) error {
	temp, err := os.CreateTemp(store.Dir, ".head-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err == nil {
		_, err = temp.WriteString(revision + "\n")
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := runtimeReplaceHead(tempPath, filepath.Join(store.Dir, "HEAD")); err != nil {
		return fmt.Errorf("publish SDD runtime HEAD: %w", err)
	}
	return nil
}

func (store RuntimeStore) syncReplay() error {
	if err := runtimeSyncDirectory(filepath.Join(store.Dir, "records")); err != nil {
		return fmt.Errorf("sync immutable SDD runtime record directory: %w", err)
	}
	if err := runtimeSyncDirectory(store.Dir); err != nil {
		return fmt.Errorf("sync SDD runtime HEAD directory: %w", err)
	}
	return nil
}

func readRuntimeHead(path string) (string, bool, error) {
	payload, err := readBoundedRuntimeFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if len(payload) != len("sha256:")+64+1 || payload[len(payload)-1] != '\n' {
		return "", true, errors.New("invalid SDD runtime HEAD encoding")
	}
	revision := strings.TrimSuffix(string(payload), "\n")
	if !runtimeRevisionPattern.MatchString(revision) {
		return "", true, errors.New("invalid SDD runtime HEAD revision")
	}
	return revision, true, nil
}

func (store RuntimeStore) recordPath(revision string) string {
	return filepath.Join(store.Dir, "records", strings.TrimPrefix(revision, "sha256:")+".json")
}

func (store RuntimeStore) loadRecord(revision string) (runtimeRecord, error) {
	if !runtimeRevisionPattern.MatchString(revision) {
		return runtimeRecord{}, errors.New("invalid SDD runtime record revision")
	}
	path := store.recordPath(revision)
	payload, err := readBoundedRuntimeFile(path)
	if err != nil {
		return runtimeRecord{}, fmt.Errorf("load SDD runtime revision %s: %w", revision, err)
	}
	sum := sha256.Sum256(payload)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != revision {
		return runtimeRecord{}, fmt.Errorf("SDD runtime record revision mismatch: expected %s, got %s", revision, actual)
	}
	// #2702: unknown fields are tolerated, not refused. The sha256 revision
	// above already pins the bytes, so a strict decode added no integrity; it
	// only made a record with one additive field from a newer binary
	// unreadable by every operation, reset included. The trade-off is that an
	// older binary ignores fields it does not understand. A record whose
	// schema version is newer than this binary's stays a typed refusal.
	decoder := json.NewDecoder(bytes.NewReader(payload))
	var record runtimeRecord
	if err := decoder.Decode(&record); err != nil {
		return runtimeRecord{}, fmt.Errorf("decode SDD runtime revision %s: %w", revision, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return runtimeRecord{}, errors.New("SDD runtime record contains multiple JSON values")
	}
	if runtimeRecordSchemaIsNewer(record.Schema) {
		return runtimeRecord{}, &RuntimeRecordSchemaUnsupportedError{Revision: revision, Schema: record.Schema}
	}
	_, canonical, err := runtimeRecordRevision(record)
	if err != nil || !runtimeRecordPayloadCanonical(payload, canonical) {
		return runtimeRecord{}, errors.New("SDD runtime record is not canonical")
	}
	if record.Change != store.Change {
		return runtimeRecord{}, errors.New("SDD runtime record change does not match store")
	}
	return record, nil
}

func runtimeRecordPayloadCanonical(payload, canonical []byte) bool {
	if bytes.Equal(payload, canonical) {
		return true
	}
	var actual, expected map[string]json.RawMessage
	if json.Unmarshal(payload, &actual) != nil || json.Unmarshal(canonical, &expected) != nil || len(actual) <= len(expected) {
		return false
	}
	for key, value := range expected {
		if !bytes.Equal(actual[key], value) {
			return false
		}
	}
	var compact bytes.Buffer
	if json.Compact(&compact, payload) != nil {
		return false
	}
	compact.WriteByte('\n')
	return bytes.Equal(compact.Bytes(), payload)
}

func runtimeRecordSchemaIsNewer(schema string) bool {
	prefix := runtimeRecordSchema[:strings.LastIndex(runtimeRecordSchema, "/v")+2]
	supported, err := strconv.Atoi(strings.TrimPrefix(runtimeRecordSchema, prefix))
	if err != nil || !strings.HasPrefix(schema, prefix) {
		return false
	}
	version, err := strconv.Atoi(strings.TrimPrefix(schema, prefix))
	return err == nil && version > supported
}

func readBoundedRuntimeFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximumRuntimeRecordBytes {
		return nil, errors.New("SDD runtime authority artifact is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maximumRuntimeRecordBytes+1))
}

// Old attempt events stay authenticated, immutable history, not executable policy.
func applyRuntimeRecordLocked(store RuntimeStore, replay *runtimeReplay, revision string, record runtimeRecord) error {
	if record.PreviousRevision != replay.Status.Revision {
		return rejectRuntimeRecord("predecessor_equal_replay_state")
	}
	if _, duplicate := replay.Requests[record.RequestID]; duplicate {
		return rejectRuntimeRecord("duplicate_request_identifier")
	}
	if err := validateRuntimeRecordShape(record); err != nil {
		return err
	}
	if record.Operation == runtimeOperationGrant {
		applyRuntimeGrantEvent(replay, record.Grant)
	}
	replay.Status.Revision = revision
	replay.Requests[record.RequestID] = runtimeRequestReceipt{Digest: record.RequestDigest, Revision: revision}
	return nil
}

func validateRuntimeRecordShape(record runtimeRecord) error {
	if record.Schema != runtimeRecordSchema || !validRuntimeChange(record.Change) ||
		(record.PreviousRevision != "" && !runtimeRevisionPattern.MatchString(record.PreviousRevision)) ||
		!runtimeRequestIDPattern.MatchString(record.RequestID) || !runtimeRevisionPattern.MatchString(record.RequestDigest) {
		return rejectRuntimeRecord("invalid_identity")
	}
	switch record.Operation {
	case runtimeOperationGrant:
		if record.Grant == nil || record.Repair != nil || record.Begin != nil || record.Finish != nil || record.Reset != nil || record.Rescope != nil || record.Supersede != nil || record.Advance != nil || record.Handoff != nil {
			return rejectRuntimeRecord("invalid_grant_shape")
		}
		event := record.Grant
		if len(event.Roots) < 1 || len(event.Roots) > maximumRuntimeGrantRoots ||
			validateRuntimeText(event.Reason, 500) != nil || validateRuntimeText(event.Actor, 128) != nil {
			return rejectRuntimeRecord("invalid_grant_event")
		}
		if event.Instance == "" || validateRuntimeText(event.Instance, 128) != nil {
			return rejectRuntimeRecord("invalid_grant_change_instance")
		}
		seen := make(map[string]struct{}, len(event.Roots))
		for _, root := range event.Roots {
			if validateRuntimeText(root, 4096) != nil || !filepath.IsAbs(root) {
				return rejectRuntimeRecord("invalid_grant_root")
			}
			if _, duplicate := seen[root]; duplicate {
				return rejectRuntimeRecord("duplicate_grant_root")
			}
			seen[root] = struct{}{}
		}
		// GrantedAt is the ledger's first wall-clock field: validated for
		// parseability only, never recomputed or compared against a clock, so
		// it stays excluded from determinism-replay expectations.
		if _, err := time.Parse(time.RFC3339Nano, event.GrantedAt); err != nil {
			return rejectRuntimeRecord("invalid_grant_timestamp")
		}
		request := GrantRootsRequest{
			ExpectedRevision: record.PreviousRevision, RequestID: record.RequestID,
			Roots: event.Roots, Reason: event.Reason, Actor: event.Actor,
			ChangeInstance: event.Instance,
		}
		if runtimeValueHash("gentle-ai.sdd-runtime-grant-request/v1", request) != record.RequestDigest {
			return rejectRuntimeRecord("grant_request_digest_match")
		}
	case "attempt/begin", "attempt/finish", "attempt/handoff", "objective/reset", "objective/rescope", "objective/supersede", "objective/repair-consecutive-rescope", "objective/advance":
		if record.Grant != nil {
			return rejectRuntimeRecord("unexpected_grant_event")
		}
	default:
		return rejectRuntimeRecord("invalid_operation")
	}
	return nil
}
