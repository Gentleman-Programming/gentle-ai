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
	"reflect"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const reviewBindingSchema = "gentle-ai.sdd-review-binding/v1"

var reviewBindingChange = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var reviewBindingHash = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var bindingFinalAuthorizationHook = func() {}
var bindingRename = reviewtransaction.ReplaceFileAtomic
var syncBindingDirectory = reviewtransaction.SyncReviewDirectory

type ReviewBindingPublicationError struct{ Cause error }

func (err *ReviewBindingPublicationError) Error() string {
	return fmt.Sprintf("SDD review binding publication requires exact replay: %v", err.Cause)
}

func (err *ReviewBindingPublicationError) Unwrap() error { return err.Cause }

type ReviewBinding struct {
	Schema            string                        `json:"schema"`
	Revision          string                        `json:"revision"`
	Change            string                        `json:"change"`
	Lineage           string                        `json:"lineage"`
	AuthorityRevision string                        `json:"authority_revision"`
	ReceiptHash       string                        `json:"receipt_hash"`
	GateContext       reviewtransaction.GateContext `json:"gate_context"`
}

func BindApprovedReview(ctx context.Context, repo, change, lineage, expected string) (ReviewBinding, error) {
	if !validReviewBindingChange(change) {
		return ReviewBinding{}, errors.New("invalid OpenSpec change name")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return ReviewBinding{}, err
	}
	changeRoot, err := resolveBindingChangeRoot(root, repo, change)
	if err != nil {
		return ReviewBinding{}, err
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, lineage)
	if err != nil {
		return ReviewBinding{}, err
	}
	record, err := store.Load()
	if errors.Is(err, os.ErrNotExist) {
		legacy, legacyErr := reviewtransaction.AuthoritativeStore(ctx, root, lineage)
		if legacyErr == nil {
			if _, legacyLoadErr := legacy.LoadChain(); legacyLoadErr == nil {
				return ReviewBinding{}, reviewtransaction.NewLegacyReadOnlyError("review/bind-sdd", lineage)
			}
		}
	}
	if err != nil || record.State.State != reviewtransaction.StateApproved {
		return ReviewBinding{}, errors.New("explicit compact authority is not approved")
	}
	payload, err := os.ReadFile(store.ReceiptPath())
	if err != nil {
		return ReviewBinding{}, err
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(payload)
	authoritative, receiptErr := record.State.Receipt()
	if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
		return ReviewBinding{}, errors.New("compact receipt does not match approved authority")
	}
	if err := verifyBindingLedger(changeRoot, record.State.Findings); err != nil {
		return ReviewBinding{}, err
	}
	input := reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: lineage}
	gate := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
	if gate.Result != reviewtransaction.GateAllow {
		return ReviewBinding{}, errors.New("compact post-apply gate is not allow")
	}
	binding := ReviewBinding{Schema: reviewBindingSchema, Change: change, Lineage: lineage, AuthorityRevision: record.Revision, ReceiptHash: bindingHash(payload), GateContext: gate.Context}
	binding.Revision = bindingDigest(binding)
	final, finalErr := store.Load()
	finalPayload, readErr := os.ReadFile(store.ReceiptPath())
	finalGate := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, input)
	finalChangeRoot, changeErr := resolveBindingChangeRoot(root, repo, change)
	if finalErr != nil || readErr != nil || changeErr != nil || finalChangeRoot != changeRoot || final.Revision != record.Revision || !bytes.Equal(payload, finalPayload) || finalGate.Result != reviewtransaction.GateAllow || !reflect.DeepEqual(gate.Context, finalGate.Context) {
		return ReviewBinding{}, errors.New("authority or live gate changed before binding publish")
	}
	return binding, writeBinding(bindingPath(store, change), expected, binding)
}

func validateBoundReview(ctx context.Context, repo, change string) (ReviewBinding, reviewtransaction.NativeGateEvaluation, error) {
	if !validReviewBindingChange(change) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("invalid OpenSpec change name")
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	changeRoot, err := resolveBindingChangeRoot(root, repo, change)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	probe, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, "binding-probe")
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	payload, err := os.ReadFile(bindingPath(probe, change))
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, fmt.Errorf("approved review binding is missing: %w", err)
	}
	binding, err := parseBinding(payload)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, fmt.Errorf("approved review binding is invalid: %w", err)
	}
	if binding.Change != change {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("approved review binding change does not match selected change")
	}
	store, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, binding.Lineage)
	if err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	record, err := store.Load()
	if err != nil || record.Revision != binding.AuthorityRevision || record.State.State != reviewtransaction.StateApproved {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact authority is stale or not approved")
	}
	receiptPayload, err := os.ReadFile(store.ReceiptPath())
	if err != nil || bindingHash(receiptPayload) != binding.ReceiptHash {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact receipt changed")
	}
	receipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
	authoritative, receiptErr := record.State.Receipt()
	if err != nil || receiptErr != nil || !reflect.DeepEqual(receipt, authoritative) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact receipt does not match approved authority")
	}
	if err := verifyBindingLedger(changeRoot, record.State.Findings); err != nil {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, err
	}
	evaluation := reviewtransaction.EvaluateCompactGate(ctx, root, receipt, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: binding.Lineage})
	if evaluation.Result != reviewtransaction.GateAllow || !boundGateContextMatches(binding.GateContext, evaluation.Context) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact post-apply gate context changed")
	}
	bindingFinalAuthorizationHook()
	finalBinding, bindingErr := os.ReadFile(bindingPath(probe, change))
	finalRecord, recordErr := store.Load()
	finalReceipt, receiptErr := os.ReadFile(store.ReceiptPath())
	finalChangeRoot, changeErr := resolveBindingChangeRoot(root, repo, change)
	if bindingErr != nil || recordErr != nil || receiptErr != nil || changeErr != nil || finalChangeRoot != changeRoot || !bytes.Equal(finalBinding, payload) || finalRecord.Revision != record.Revision || !reflect.DeepEqual(finalRecord.State, record.State) || finalRecord.State.State != reviewtransaction.StateApproved || !bytes.Equal(finalReceipt, receiptPayload) || bindingHash(finalReceipt) != binding.ReceiptHash {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound authority, receipt, or binding changed during final read")
	}
	finalReceiptValue, parseErr := reviewtransaction.ParseCompactReceipt(finalReceipt)
	finalAuthoritative, authorityErr := finalRecord.State.Receipt()
	if parseErr != nil || authorityErr != nil || !reflect.DeepEqual(finalReceiptValue, finalAuthoritative) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact receipt does not match final authority")
	}
	finalGate := reviewtransaction.EvaluateCompactGate(ctx, root, finalReceiptValue, reviewtransaction.NativeGateRequestInput{Gate: reviewtransaction.GatePostApply, LineageID: binding.Lineage})
	if finalGate.Result != reviewtransaction.GateAllow || !boundGateContextMatches(binding.GateContext, finalGate.Context) {
		return ReviewBinding{}, reviewtransaction.NativeGateEvaluation{}, errors.New("bound compact post-apply gate changed during final authorization")
	}
	return binding, finalGate, nil
}

// boundGateContextMatches compares a persisted binding gate context against
// the live post-apply evaluation. Bindings persisted before compact gate
// contexts bound the frozen findings ledger recorded the empty-input hash in
// ledger_hash; they stay valid against a now-populated live context because
// the findings ledger itself is still pinned by the authority state through
// AuthorityRevision and ReceiptHash. Every other divergence — including any
// non-empty ledger hash that does not match the live binding — still fails.
// The reverse mix is a known residual this side cannot repair: a binding
// written by a ledger-binding binary but validated by an older binary
// deterministically fails closed here ("bound compact post-apply gate context
// changed"), because the older binary hardcodes the empty hash in its live
// context; the remedy is upgrading that older binary.
func boundGateContextMatches(bound, live reviewtransaction.GateContext) bool {
	if reflect.DeepEqual(bound, live) {
		return true
	}
	if bound.LedgerHash != reviewtransaction.EmptyFixDeltaHash {
		return false
	}
	bound.LedgerHash = live.LedgerHash
	return reflect.DeepEqual(bound, live)
}

func bindingExists(ctx context.Context, repo, change string) (bool, error) {
	root, err := (reviewtransaction.SnapshotBuilder{Repo: repo}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return false, nil
	}
	probe, err := reviewtransaction.CompactAuthoritativeStore(ctx, root, "binding-probe")
	if err != nil {
		return false, err
	}
	_, err = os.Stat(bindingPath(probe, change))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func resolveBindingChangeRoot(root, workspace, change string) (string, error) {
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	workspace = filepath.Clean(workspace)
	if !pathWithinBindingRoot(root, workspace) {
		return "", errors.New("planning workspace is outside selected repository")
	}

	planningRoot := ""
	for current := workspace; pathWithinBindingRoot(root, current); current = filepath.Dir(current) {
		openspecRoot := filepath.Join(current, "openspec")
		info, statErr := os.Stat(openspecRoot)
		if statErr == nil {
			if !info.IsDir() {
				return "", errors.New("selected OpenSpec root is not a directory")
			}
			resolved, resolveErr := filepath.EvalSymlinks(openspecRoot)
			if resolveErr != nil {
				return "", resolveErr
			}
			resolved = filepath.Clean(resolved)
			if !pathWithinBindingRoot(root, resolved) {
				return "", errors.New("selected OpenSpec root resolves outside repository")
			}
			if resolved != filepath.Clean(openspecRoot) {
				return "", errors.New("selected OpenSpec root uses a symlinked path")
			}
			planningRoot = current
			break
		} else if !os.IsNotExist(statErr) {
			return "", statErr
		}
		if current == root {
			break
		}
	}
	if planningRoot == "" {
		return "", errors.New("selected OpenSpec change does not exist")
	}
	candidate := filepath.Join(planningRoot, "openspec", "changes", change)
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("selected OpenSpec change does not exist")
		}
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("selected OpenSpec change is not a directory")
	}
	selected, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	selected = filepath.Clean(selected)
	if !pathWithinBindingRoot(root, selected) {
		return "", errors.New("selected OpenSpec change resolves outside repository")
	}
	if selected != filepath.Clean(candidate) {
		return "", errors.New("selected OpenSpec change uses a symlinked path")
	}

	matches, err := bindingChangeRoots(root, change)
	if err != nil {
		return "", err
	}
	if len(matches) != 1 || matches[0] != selected {
		return "", errors.New("selected OpenSpec change is ambiguous within repository")
	}
	return selected, nil
}

func bindingChangeRoots(root, change string) ([]string, error) {
	matches := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" && path != root {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.Name() == "openspec" {
				candidate := filepath.Join(path, "changes", change)
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					matches = append(matches, candidate)
				} else if err != nil && !os.IsNotExist(err) {
					return err
				}
			} else if isBindingChangePath(path, change) {
				matches = append(matches, path)
			}
			return nil
		}
		if !entry.IsDir() || !isBindingChangePath(path, change) {
			return nil
		}
		matches = append(matches, filepath.Clean(path))
		return filepath.SkipDir
	})
	return matches, err
}

func isBindingChangePath(path, change string) bool {
	changesRoot := filepath.Dir(path)
	return filepath.Base(path) == change && filepath.Base(changesRoot) == "changes" && filepath.Base(filepath.Dir(changesRoot)) == "openspec"
}

func pathWithinBindingRoot(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func verifyBindingLedger(changeRoot string, findings []reviewtransaction.Finding) error {
	payload, err := os.ReadFile(filepath.Join(changeRoot, "reviews", "ledger.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	want, err := reviewtransaction.CanonicalLedger(findings)
	if err != nil || !bytes.Equal(payload, want) {
		return errors.New("SDD review ledger does not equal compact findings")
	}
	return nil
}
func bindingPath(store reviewtransaction.CompactStore, change string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(store.Dir)))), "gentle-ai", "sdd-review-bindings", "v1", change, "binding.json")
}
func bindingHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func bindingDigest(b ReviewBinding) string {
	b.Revision = ""
	payload, _ := json.Marshal(b)
	return bindingHash(payload)
}

func validReviewBindingChange(change string) bool {
	return len(change) <= 96 && reviewBindingChange.MatchString(change)
}

func bindingBytes(binding ReviewBinding) ([]byte, error) {
	payload, err := json.Marshal(binding)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseBinding(payload []byte) (ReviewBinding, error) {
	var binding ReviewBinding
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&binding); err != nil {
		return ReviewBinding{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ReviewBinding{}, errors.New("multiple binding values")
	}
	canonical, err := bindingBytes(binding)
	if err != nil || !bytes.Equal(payload, canonical) || binding.Schema != reviewBindingSchema || !validReviewBindingChange(binding.Change) || !reviewBindingHash.MatchString(binding.Revision) || !reviewBindingHash.MatchString(binding.AuthorityRevision) || !reviewBindingHash.MatchString(binding.ReceiptHash) || binding.Revision != bindingDigest(binding) || binding.GateContext.Gate != reviewtransaction.GatePostApply || binding.GateContext.LineageID != binding.Lineage || binding.GateContext.StoreRevision != binding.AuthorityRevision {
		return ReviewBinding{}, errors.New("invalid binding")
	}
	return binding, nil
}

func writeBinding(path, expected string, binding ReviewBinding) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := acquireBindingLock(filepath.Join(filepath.Dir(path), "LOCK"))
	if err != nil {
		return err
	}
	defer lock.release()
	current := ""
	if payload, err := os.ReadFile(path); err == nil {
		old, parseErr := parseBinding(payload)
		if parseErr != nil || old.Change != binding.Change {
			return errors.New("invalid existing binding")
		}
		current = old.Revision
		if current == binding.Revision {
			if err := syncBindingDirectory(filepath.Dir(path)); err != nil {
				return &ReviewBindingPublicationError{Cause: fmt.Errorf("sync SDD review binding directory: %w", err)}
			}
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if current != expected {
		return fmt.Errorf("binding revision conflict: expected %q, current %q", expected, current)
	}
	payload, err := bindingBytes(binding)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".binding-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(payload); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temp.Name())
		return err
	}
	if err := bindingRename(temp.Name(), path); err != nil {
		return err
	}
	if err := syncBindingDirectory(filepath.Dir(path)); err != nil {
		return &ReviewBindingPublicationError{Cause: fmt.Errorf("sync SDD review binding directory: %w", err)}
	}
	return nil
}
