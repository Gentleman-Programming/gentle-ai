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
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pathidentity"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewBindingSchema = "gentle-ai.sdd-review-binding/v1"

// A change identity is whatever sdd-status already resolves: an OpenSpec
// directory name or an Engram change ID such as DEC-EXAMPLE-CHANGE. It stays
// path-safe by construction, since alphanumeric segments joined by single
// hyphens or underscores can express neither a separator nor a dot segment.
var reviewBindingChange = regexp.MustCompile(`^[A-Za-z0-9]+(?:[-_][A-Za-z0-9]+)*$`)

// legacyRuntimeChange is the shape the runtime ledger stored directly at
// v1/<change> before identities widened. It must never change.
var legacyRuntimeChange = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var reviewBindingLineage = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var reviewBindingHash = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type ReviewBinding struct {
	Schema            string                        `json:"schema"`
	Revision          string                        `json:"revision"`
	Change            string                        `json:"change"`
	Lineage           string                        `json:"lineage"`
	AuthorityRevision string                        `json:"authority_revision"`
	ReceiptHash       string                        `json:"receipt_hash"`
	GateContext       reviewtransaction.GateContext `json:"gate_context"`
}

func resolveBindingChangeRoot(ctx context.Context, root, workspace, change string) (string, error) {
	// Both operands are canonicalized the same way before any containment or
	// equality decision below. Resolving only the workspace was 1773 boundary
	// 1: on macOS the same repository spelled through /var and through
	// /private/var compared unequal, and the planning workspace was reported
	// outside its own repository.
	workspace, err := canonicalBindingPath(workspace)
	if err != nil {
		return "", err
	}
	root, err = canonicalBindingPath(root)
	if err != nil {
		return "", err
	}
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
		if pathidentity.SameDirectory(current, root) {
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

	matches, err := bindingChangeRoots(ctx, root, change)
	if err != nil {
		return "", err
	}
	if len(matches) != 1 || matches[0] != selected {
		return "", errors.New("selected OpenSpec change is ambiguous within repository")
	}
	return selected, nil
}

func bindingChangeRoots(ctx context.Context, root, change string) ([]string, error) {
	paths, err := (reviewtransaction.SnapshotBuilder{Repo: root}).DiscoverTrackedAndUnignoredPaths(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	matches := []string{}
	for _, logicalPath := range paths {
		parts := strings.Split(logicalPath, "/")
		if parts[len(parts)-1] == "openspec" {
			parts = append(parts, "changes", change)
		}
		for index := 0; index+2 < len(parts); index++ {
			if parts[index] != "openspec" || parts[index+1] != "changes" || parts[index+2] != change {
				continue
			}
			rootPath := strings.Join(parts[:index+3], "/")
			if _, duplicate := seen[rootPath]; duplicate {
				break
			}
			seen[rootPath] = struct{}{}
			candidate := filepath.Join(root, filepath.FromSlash(rootPath))
			info, statErr := os.Lstat(candidate)
			if os.IsNotExist(statErr) {
				break
			}
			if statErr != nil {
				return nil, statErr
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				matches = append(matches, filepath.Clean(candidate))
			}
			break
		}
	}
	return matches, nil
}

// canonicalBindingPath is the single canonicalization every binding path goes
// through before it is compared with another. Having one of these, used on
// both operands, is what keeps a second spelling of one repository from
// looking like a different repository.
func canonicalBindingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// pathWithinBindingRoot defers containment to the filesystem identity policy
// in internal/pathidentity, so alternate spellings that the operating system
// resolves to one directory -- symlinked ancestors, case-insensitive volumes,
// Unicode-equivalent names -- are one directory here too. Callers still
// resolve a candidate with filepath.EvalSymlinks before asking, because this
// answers "is it inside", never "did it get there through a symlink".
func pathWithinBindingRoot(root, path string) bool {
	return pathidentity.Contains(root, path)
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

// legacyRuntimeChangeDir reports whether a change identity is one the runtime
// ledger has always stored directly at v1/<change>.
func legacyRuntimeChangeDir(change string) bool {
	return len(change) <= 96 && legacyRuntimeChange.MatchString(change)
}

func validReviewBindingLineage(lineage string) bool {
	return len(lineage) <= 128 && reviewBindingLineage.MatchString(lineage)
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
	if violation := reviewBindingViolation(payload, binding); violation != "" {
		return ReviewBinding{}, fmt.Errorf("invalid binding: %s", violation) // refusal:by-design world-action: this binding is written and digested by this package itself, so a violation is a mutated or corrupted record and the exit is restoring the Git-common-dir store, not a command
	}
	return binding, nil
}

// reviewBindingViolation names WHICH of the twelve integrity conditions a
// binding failed, or "" when it satisfies all of them. Root 8 (#2471): these
// twelve used to answer with one shared `errors.New("invalid binding")`, so a
// caller holding the error learned only that something was wrong with bytes
// this package wrote itself.
//
// Unlike the reviewer-input surfaces, this validates OUR OWN persisted ledger
// bytes, so a violation is tamper or corruption rather than user error. That
// is exactly why naming the condition matters: the reader is an operator
// diagnosing a damaged store, and "invalid binding" tells them to escalate
// while "gate context lineage does not match" tells them what broke. The
// values themselves are never echoed, only the condition that failed, so a
// damaged binding cannot use this to reflect arbitrary bytes into a message.
func reviewBindingViolation(payload []byte, binding ReviewBinding) string {
	canonical, err := bindingBytes(binding)
	switch {
	case err != nil:
		return "binding could not be re-encoded for canonical comparison"
	case !bytes.Equal(payload, canonical):
		return "payload is not the canonical encoding of its own fields"
	case binding.Schema != reviewBindingSchema:
		return "schema is not " + reviewBindingSchema
	case !validReviewBindingChange(binding.Change):
		return "change name is not a valid change identifier"
	case !validReviewBindingLineage(binding.Lineage):
		return "lineage is not a valid lineage identifier"
	case !reviewBindingHash.MatchString(binding.Revision):
		return "revision is not a sha256 digest"
	case !reviewBindingHash.MatchString(binding.AuthorityRevision):
		return "authority_revision is not a sha256 digest"
	case !reviewBindingHash.MatchString(binding.ReceiptHash):
		return "receipt_hash is not a sha256 digest"
	case binding.Revision != bindingDigest(binding):
		return "revision does not match the digest of its own fields"
	case binding.GateContext.Gate != reviewtransaction.GatePostApply:
		return "gate_context.gate is not " + string(reviewtransaction.GatePostApply)
	case binding.GateContext.LineageID != binding.Lineage:
		return "gate_context.lineage_id does not match the binding lineage"
	case binding.GateContext.StoreRevision != binding.AuthorityRevision:
		return "gate_context.store_revision does not match authority_revision"
	}
	return ""
}
