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
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const GateRequestSchema = "gentle-ai.review-gate-request/v1"

type GateRequest struct {
	Schema           string                      `json:"schema"`
	Gate             GateKind                    `json:"gate"`
	Target           Target                      `json:"target"`
	StoreDir         string                      `json:"store_dir,omitempty"`
	StoreRevision    string                      `json:"store_revision"`
	GenesisRevision  string                      `json:"genesis_revision"`
	ChainIdentity    string                      `json:"chain_identity"`
	BundleDigest     string                      `json:"bundle_digest"`
	PolicyArtifact   string                      `json:"policy_artifact"`
	PolicyContent    string                      `json:"policy_content,omitempty"`
	LedgerArtifact   string                      `json:"ledger_artifact"`
	LedgerContent    string                      `json:"ledger_content,omitempty"`
	FixDeltaArtifact string                      `json:"fix_delta_artifact,omitempty"`
	FixDeltaContent  string                      `json:"fix_delta_content,omitempty"`
	EvidenceArtifact string                      `json:"evidence_artifact"`
	EvidenceContent  string                      `json:"evidence_content,omitempty"`
	ExternalEvidence ExternalEvidenceDisposition `json:"external_evidence,omitempty"`
	Push             *PushRequest                `json:"push,omitempty"`
	PrePR            *PrePRRequest               `json:"pre_pr,omitempty"`
	Release          *ReleaseRequest             `json:"release,omitempty"`
	preimages        *gateArtifactPreimages
}

type PrePRRequest struct {
	CIAttestationArtifact string                  `json:"ci_attestation_artifact"`
	Boundary              *PrePRBoundarySelection `json:"boundary,omitempty"`
	PushRemote            string                  `json:"push_remote,omitempty"`
	PushRemoteIdentity    string                  `json:"push_remote_identity,omitempty"`
}

type PushRequest struct {
	Boundary           PrePRBoundarySelection `json:"boundary"`
	MergeBase          string                 `json:"merge_base"`
	PushRemote         string                 `json:"push_remote"`
	PushRemoteIdentity string                 `json:"push_remote_identity"`
	DeliveryBaseTree   string                 `json:"delivery_base_tree,omitempty"`
	// ReviewedBaseTree is set only for empty-remote bootstrap boundaries and
	// carries the authoritative reviewed base tree so re-derivation can locate
	// the same delivery base without a remote publication boundary.
	ReviewedBaseTree string `json:"reviewed_base_tree,omitempty"`
}

type PrePRBoundarySource string

const (
	PrePRBoundaryExplicit           PrePRBoundarySource = "explicit"
	PrePRBoundaryPublicationDefault PrePRBoundarySource = "publication-default"
	// PrePRBoundaryEmptyRemoteBootstrap marks a pre-push first-publication
	// boundary: the configured upstream remote advertises no refs at all, so
	// the publication base is the explicit zero OID instead of an advertised
	// remote commit.
	PrePRBoundaryEmptyRemoteBootstrap PrePRBoundarySource = "empty-remote-bootstrap"
)

// PrePRBoundarySelection records how the immutable pre-PR range boundary was
// selected. It is evidence only; receipt binding still authorizes the range.
type PrePRBoundarySelection struct {
	Source         PrePRBoundarySource `json:"source"`
	Selector       string              `json:"selector"`
	Commit         string              `json:"commit"`
	MergeBase      string              `json:"merge_base"`
	Remote         string              `json:"remote,omitempty"`
	RemoteRef      string              `json:"remote_ref,omitempty"`
	RemoteIdentity string              `json:"remote_identity,omitempty"`
}

type ReleaseRequest struct {
	Revision                    string                 `json:"revision"`
	ConfigurationArtifact       string                 `json:"configuration_artifact"`
	ConfigurationContent        string                 `json:"configuration_content,omitempty"`
	GeneratedArtifact           string                 `json:"generated_artifact"`
	GeneratedContent            string                 `json:"generated_content,omitempty"`
	ProvenanceArtifact          string                 `json:"provenance_artifact"`
	ProvenanceContent           string                 `json:"provenance_content,omitempty"`
	PublicationBoundaryArtifact string                 `json:"publication_boundary_artifact"`
	PublicationBoundaryContent  string                 `json:"publication_boundary_content,omitempty"`
	PublicationState            PublicationState       `json:"publication_state"`
	EvidenceFreshnessArtifact   string                 `json:"evidence_freshness_artifact"`
	EvidenceFreshnessContent    string                 `json:"evidence_freshness_content,omitempty"`
	EvidenceFreshnessState      EvidenceFreshnessState `json:"evidence_freshness_state"`
}

type gateArtifactPreimages struct {
	policy, ledger, fixDelta, evidence                    []byte
	configuration, generated, provenance                  []byte
	publicationBoundary, evidenceFreshness, ciAttestation []byte
}

type NativeGateEvaluation struct {
	Result  GateResult
	Reason  string
	Context GateContext
	Cause   error `json:"-"`
	// Contended reports that this evaluation reached no verdict at all: the
	// read-only final-authorization window could not obtain the advisory
	// authority lock within its bounded wait, so nothing about the candidate
	// was decided (1861). A gate that lost a lock has not found damage, so the
	// caller must surface Cause — an *AuthorityLockTimeoutError — instead of
	// publishing Result, which stays `invalidated` only so an emitter that has
	// not been taught about contention degrades to the previous behavior
	// rather than to an unpublished enum value.
	Contended bool `json:"-"`
	// Relation and Next are Wave 5 (Gate Cutover) Slice 3's additive fields
	// (design decision 3, gate.go composite literals stay keyed and compile
	// untouched). Relation is the CandidateRelation gateVerdict classified
	// this evaluation's denial as; Next is gateVerdict's own executable
	// continuation. Both are populated only where Slice 3's wiring
	// (attachGateVerdictRelation, compact_gate.go) can prove the
	// classification through the real evaluation path today — the
	// "changed" relation denials (candidate-or-paths-mismatch,
	// base-mismatch). Every other outcome leaves these at their zero value
	// this slice: gateVerdict is total and already defines an answer for
	// every (gate, relation) pair (TestGateVerdict_TotalFunction_35Cells),
	// but classifying every OTHER live outcome into a relation requires the
	// legacy-through-algebra projection Slice 4 delivers and the
	// composition/decline removals Slices 5-6 deliver; wiring more now
	// would mean guessing at a classification this slice cannot yet prove
	// correct through production code, which is exactly what the matrix
	// harness's "never a fabricated pass" rule (Slice 1) forbids.
	Relation CandidateRelation `json:"relation,omitempty"`
	Next     *GateNextStep     `json:"next,omitempty"`
}

// GateNextStep is a denial's executable continuation (design's Interfaces /
// Contracts sketch): either a named Transition the caller can run next, or —
// when no single operation resolves the denial (ambiguous authority, an
// unresolvable target) — Transition stays empty and ReasonCode alone
// explains why, which is still "never a bare denial" (task 4.1) because a
// caller always has SOMETHING to read, never nothing. Transition names a
// real top-level `review` operation (see runReviewCommand's dispatch table,
// internal/cli/review_facade.go) rather than a fully-rendered command line
// with dynamic flag values (lineage, revision, etc.) baked in — exactly the
// Wave 4 CRITICAL-A livelock lesson: a printed command whose flags were
// wrong or incomplete was worse than none, so this field names an operation
// the caller is proven able to invoke (verified against the CLI's own flag
// validation in the deny-golden tests), never a guessed invocation string.
type GateNextStep struct {
	Transition string `json:"transition,omitempty"`
	ReasonCode string `json:"reason_code"`
}

var finalGateAuthorizationHook = func() {}
var artifactPreimagesReadHook = func() {}

func ParseGateRequest(payload []byte) (GateRequest, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var request GateRequest
	if err := decoder.Decode(&request); err != nil {
		return GateRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return GateRequest{}, errors.New("multiple JSON values in review gate request")
	}
	if err := validateGateRequest(request); err != nil {
		return GateRequest{}, err
	}
	return request, nil
}

func HashArtifact(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("artifact path is required")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type resolvedPrePRRefs struct {
	Selection            PrePRBoundarySelection
	TrackingBoundary     PrePRBoundarySelection
	TrackingPresent      bool
	HeadCommit           string
	BaseCommit           string
	DeliveredCommitCount int
	PushRemote           string
}

// shellUnsafeBoundaryRefCharacters lists every POSIX shell metacharacter plus
// the glob and brace-expansion characters. The gate binds boundary selectors as
// structured fields and launches Git only through an argv, but a bound selector
// is replayed verbatim into whatever process consumes it (`check-ref-format`,
// `ls-remote`, `merge-base`, `rev-list`), so a selector that carries `;`,
// `&&`, `$(...)`, a backtick, a redirection, or a glob would become a composed
// command the moment any consumer is less careful than the gate. Git ref names
// never need these characters, so rejecting the whole set costs nothing and
// fails closed.
const shellUnsafeBoundaryRefCharacters = "|&;<>()$`\\\"'*?[]{}!"

// errCommandLikeBoundaryRef marks a publication-boundary selector that only
// means something to a shell or an argument parser, never to Git.
var errCommandLikeBoundaryRef = errors.New("boundary ref must be a structured ref, not a command")

// validateBoundaryRefSelector hardens the refs that name the PR head and its
// destination — the "PR commands" threat-matrix boundary. Beyond the ordinary
// token rules (printable ASCII, no surrounding whitespace, bounded length) it
// rejects three families that only mean something to a shell or an argument
// parser:
//
//  1. Composed-shell forms, via shellUnsafeBoundaryRefCharacters.
//     Environment-prefix forms (`env VAR=x refs/heads/main`,
//     `GIT_DIR=/tmp refs/heads/main`) are already rejected by the token rule
//     because they contain whitespace.
//  2. Argument injection: a leading `-` would still be read as an option by a
//     structured argument list, so `--upload-pack=id` can never be a ref.
//  3. Selector and traversal forms Git itself forbids in ref names: `..`
//     escapes the ref namespace and `@{` is a reflog/upstream selector.
//
// The rule deliberately keeps `:`, `/`, `.`, `-`, `_`, `+`, `,`, `%` and `@`
// legal, because it is a shell/argument boundary, not a ref-name policy;
// `git check-ref-format` still governs branch resolution downstream.
func validateBoundaryRefSelector(name, value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 512 {
		return fmt.Errorf("%w: %s", errCommandLikeBoundaryRef, name)
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("%w: %s", errCommandLikeBoundaryRef, name)
		}
	}
	if strings.ContainsAny(value, shellUnsafeBoundaryRefCharacters) ||
		strings.HasPrefix(value, "-") ||
		strings.Contains(value, "..") || strings.Contains(value, "@{") {
		return fmt.Errorf("%w: %s", errCommandLikeBoundaryRef, name)
	}
	return nil
}

func selectPrePRBoundary(ctx context.Context, repo, selector string) (PrePRBoundarySelection, error) {
	selector = strings.TrimSpace(selector)
	var selection PrePRBoundarySelection
	var err error
	if selector != "" {
		if filepath.IsAbs(selector) {
			return PrePRBoundarySelection{}, errors.New("explicit pre-PR base selector must not be an absolute path")
		}
		if err := validateBoundaryRefSelector("pre-PR base selector", selector); err != nil {
			return PrePRBoundarySelection{}, err
		}
		selection, err = resolveAdvertisedSelector(ctx, repo, selector, PrePRBoundaryExplicit)
	} else {
		var ref, remote, commit string
		ref, remote, commit, err = resolveAuthoritativePublicationBase(ctx, repo)
		identity, identityErr := remoteRepositoryIdentity(ctx, repo, remote)
		if err == nil {
			err = identityErr
		}
		selection = PrePRBoundarySelection{Source: PrePRBoundaryPublicationDefault, Selector: ref, Commit: commit, Remote: remote, RemoteRef: ref, RemoteIdentity: identity}
	}
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	bases, mergeErr := runGit(ctx, repo, nil, nil, "merge-base", "--all", head, selection.Commit)
	if mergeBases := strings.Fields(string(bases)); mergeErr != nil || len(mergeBases) != 1 {
		message := fmt.Sprintf("publication base has %d merge bases; pass --base-ref <remote>/<branch>", len(mergeBases))
		if mergeErr != nil {
			// A Git fault says nothing about the selector, so it keeps failing
			// closed instead of promising that a flag resolves it.
			return PrePRBoundarySelection{}, errors.New(message)
		}
		return PrePRBoundarySelection{}, baseRefTargetResolutionError(message)
	}
	selection.MergeBase = strings.TrimSpace(string(bases))
	return selection, nil
}

func buildPrePRTarget(ctx context.Context, repo, selector, ciAttestation string, intendedUntracked []string) (Target, *PrePRRequest, error) {
	selection, err := selectPrePRBoundary(ctx, repo, selector)
	if err != nil {
		return Target{}, nil, err
	}
	pushRemote, _, _ := publicationRemote(ctx, repo)
	pushIdentity, err := pushRepositoryIdentity(ctx, repo, pushRemote)
	if err != nil {
		return Target{}, nil, err
	}
	return Target{Kind: TargetBaseDiff, BaseRef: selection.MergeBase, IntendedUntracked: append([]string(nil), intendedUntracked...)},
		&PrePRRequest{CIAttestationArtifact: ciAttestation, Boundary: &selection, PushRemote: pushRemote, PushRemoteIdentity: pushIdentity}, nil
}

// BuildPrePRTarget binds an advertised publication boundary to its unique merge-base.
func BuildPrePRTarget(ctx context.Context, repo, selector, ciAttestation string, intendedUntracked []string) (Target, *PrePRRequest, error) {
	return buildPrePRTarget(ctx, repo, selector, ciAttestation, intendedUntracked)
}

func publicationRemoteConfigured(ctx context.Context, repo string) (bool, error) {
	_, configured, err := publicationRemote(ctx, repo)
	return configured, err
}

func resolveAuthoritativePublicationBase(ctx context.Context, repo string) (string, string, string, error) {
	remote, configured, err := publicationRemote(ctx, repo)
	if err != nil || !configured {
		return "", "", "", baseRefTargetResolutionError("publication target remote is not configured; pass --base-ref <remote>/<branch>")
	}
	output, err := runGit(ctx, repo, nil, nil, "ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", "", "", fmt.Errorf("query publication target default branch: %w", err)
	}
	ref := ""
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" && strings.HasPrefix(fields[1], "refs/heads/") {
			ref = fields[1]
		}
	}
	if ref == "" {
		return "", "", "", baseRefTargetResolutionError("publication target default branch is unavailable; pass --base-ref <remote>/<branch>")
	}
	selection, err := advertisedRemoteRef(ctx, repo, remote, ref, remote+"/"+strings.TrimPrefix(ref, "refs/heads/"), PrePRBoundaryPublicationDefault)
	if err != nil {
		return "", "", "", err
	}
	return selection.RemoteRef, selection.Remote, selection.Commit, nil
}

// GateTargetResolutionError reports a semantic target-selection failure that
// the caller can correct by supplying the named input. It does not represent
// receipt-authority corruption or a Git process failure.
type GateTargetResolutionError struct {
	RequiredInput string
	Err           error
}

func (err *GateTargetResolutionError) Error() string {
	if err == nil || err.Err == nil {
		// refusal:by-design world-action: this sentence renders only for a degenerately constructed error with no cause, which no operator command can reach; the exit is a code fix at the construction site.
		return "review gate target resolution failed"
	}
	return err.Err.Error()
}

func (err *GateTargetResolutionError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

// GateRemoteFetchRequiredError reports that boundary selection resolved an
// advertised remote tip whose commit is absent from the local object store
// (issue #3342). The review authority store is untouched by this condition:
// the local clone is merely behind the remote it publishes to, and
// `git fetch <remote>` followed by re-running the identical gate resolves it.
// It is typed so receipt discovery can classify the denial as retry-safe
// instead of collapsing it into authority corruption.
type GateRemoteFetchRequiredError struct {
	Remote string
}

func (err *GateRemoteFetchRequiredError) Error() string {
	return "advertised base commit is not available locally; fetch before validation"
}

// baseRefTargetResolutionError types a publication-boundary failure the caller
// resolves by supplying --base-ref <remote>/<branch>. Every producer of that
// sentence types itself here: an untyped one is classified downstream as an
// unexplained assessment failure and reported as a damaged authority store,
// which is a repair instruction for a repository that needs a flag (#1861
// sibling).
func baseRefTargetResolutionError(message string) error {
	return &GateTargetResolutionError{RequiredInput: "base_ref", Err: errors.New(message)}
}

// refusal:by-design world-action: malformed remote output cannot be repaired by a review command; the operator must correct the remote boundary.
var ErrMalformedAdvertisedRemoteOutput = errors.New("malformed advertised remote output")

type GitAdvertisedRemoteOutputError struct {
	Remote string
	Ref    string
	Output string
}

func (err *GitAdvertisedRemoteOutputError) Error() string {
	return fmt.Sprintf("git ls-remote --heads %s %s returned malformed advertised remote output: %q", err.Remote, err.Ref, err.Output)
}

func (err *GitAdvertisedRemoteOutputError) Unwrap() error { return ErrMalformedAdvertisedRemoteOutput }

func resolveAdvertisedSelector(ctx context.Context, repo, selector string, source PrePRBoundarySource) (PrePRBoundarySelection, error) {
	if validGitTree(selector) {
		return PrePRBoundarySelection{}, baseRefTargetResolutionError(fmt.Sprintf("explicit pre-PR base %q must name an advertised remote branch; pass --base-ref <remote>/<branch>", selector))
	}
	output, err := runGit(ctx, repo, nil, nil, "remote")
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	remotes := strings.Fields(string(output))
	matches := []PrePRBoundarySelection{}
	operationalErrors := []error{}
	for _, remote := range remotes {
		branch := selector
		if strings.HasPrefix(selector, remote+"/") {
			branch = strings.TrimPrefix(selector, remote+"/")
		} else if strings.Contains(selector, "/") {
			continue
		}
		if _, err := runGit(ctx, repo, nil, nil, "check-ref-format", "--branch", branch); err != nil {
			continue
		}
		identity, identityErr := remoteRepositoryIdentity(ctx, repo, remote)
		if identityErr != nil {
			if strings.Contains(selector, "/") {
				return PrePRBoundarySelection{}, identityErr
			}
			operationalErrors = append(operationalErrors, identityErr)
			continue
		}
		remoteOutput, queryErr := runGit(ctx, repo, nil, nil, "ls-remote", "--heads", remote, branch)
		if queryErr != nil {
			if strings.Contains(selector, "/") {
				return PrePRBoundarySelection{}, queryErr
			}
			operationalErrors = append(operationalErrors, queryErr)
			continue
		}
		advertisedOutput := strings.TrimSpace(string(remoteOutput))
		if advertisedOutput == "" {
			continue
		}
		for _, line := range strings.Split(advertisedOutput, "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 || !validGitTree(fields[0]) || fields[1] != "refs/heads/"+branch {
				return PrePRBoundarySelection{}, &GitAdvertisedRemoteOutputError{Remote: remote, Ref: branch, Output: strings.TrimSpace(line)}
			}
			matches = append(matches, PrePRBoundarySelection{Source: source, Selector: selector, Commit: fields[0], Remote: remote, RemoteRef: fields[1], RemoteIdentity: identity})
		}
	}
	if len(matches) == 0 && len(operationalErrors) > 0 {
		return PrePRBoundarySelection{}, errors.Join(operationalErrors...)
	}
	if len(matches) != 1 {
		return PrePRBoundarySelection{}, baseRefTargetResolutionError(fmt.Sprintf("explicit pre-PR base %q is missing or ambiguous on advertised remote branches; pass --base-ref <remote>/<branch>", selector))
	}
	local, err := resolveCommit(ctx, repo, matches[0].Commit)
	if err != nil || local != matches[0].Commit {
		return PrePRBoundarySelection{}, &GateRemoteFetchRequiredError{Remote: matches[0].Remote}
	}
	return matches[0], nil
}

func advertisedRemoteRef(ctx context.Context, repo, remote, ref, selector string, source PrePRBoundarySource) (PrePRBoundarySelection, error) {
	identity, err := remoteRepositoryIdentity(ctx, repo, remote)
	if err != nil {
		return PrePRBoundarySelection{}, err
	}
	output, err := runGit(ctx, repo, nil, nil, "ls-remote", "--heads", remote, ref)
	if err != nil {
		return PrePRBoundarySelection{}, fmt.Errorf("query base remote %q: %w", remote, err)
	}
	advertisedOutput := strings.TrimSpace(string(output))
	if advertisedOutput == "" {
		return PrePRBoundarySelection{}, baseRefTargetResolutionError(fmt.Sprintf("base selector %q is not a current advertised remote branch; pass --base-ref <remote>/<branch>", selector))
	}
	// A direct lookup must receive one complete record; strings.Fields would
	// otherwise merge an OID and ref split across record separators.
	if strings.ContainsAny(advertisedOutput, "\r\n\x00") {
		return PrePRBoundarySelection{}, &GitAdvertisedRemoteOutputError{Remote: remote, Ref: ref, Output: advertisedOutput}
	}
	fields := strings.Fields(advertisedOutput)
	if len(fields) != 2 || fields[1] != ref || !validGitTree(fields[0]) {
		return PrePRBoundarySelection{}, &GitAdvertisedRemoteOutputError{Remote: remote, Ref: ref, Output: advertisedOutput}
	}
	local, err := resolveCommit(ctx, repo, fields[0])
	if err != nil || local != fields[0] {
		return PrePRBoundarySelection{}, &GateRemoteFetchRequiredError{Remote: remote}
	}
	return PrePRBoundarySelection{Source: source, Selector: selector, Commit: fields[0], Remote: remote, RemoteRef: ref, RemoteIdentity: identity}, nil
}

func remoteRepositoryIdentity(ctx context.Context, repo, remote string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "remote."+remote+".url")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(output)) == "" {
		return "", errors.New("publication remote URL is not configured")
	}
	return repositoryLocationIdentity(ctx, repo, strings.TrimSpace(string(output)))
}

func pushRepositoryIdentity(ctx context.Context, repo, remote string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "remote."+remote+".pushurl")
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return remoteRepositoryIdentity(ctx, repo, remote)
	}
	return repositoryLocationIdentity(ctx, repo, strings.TrimSpace(string(output)))
}

func repositoryLocationIdentity(ctx context.Context, repo, value string) (string, error) {
	if parsed, parseErr := url.Parse(value); parseErr == nil && parsed.Scheme != "" && parsed.Opaque == "" {
		value = strings.ToLower(parsed.Host) + "/" + strings.Trim(parsed.Path, "/")
	} else {
		if colon := strings.Index(value, ":"); colon > 0 && !strings.Contains(value[:colon], "/") {
			host := value[:colon]
			if at := strings.LastIndex(host, "@"); at >= 0 {
				host = host[at+1:]
			}
			value = strings.ToLower(host) + "/" + value[colon+1:]
		} else if !filepath.IsAbs(value) {
			root, rootErr := runGit(ctx, repo, nil, nil, "rev-parse", "--show-toplevel")
			if rootErr != nil {
				return "", errors.New("publication remote identity cannot be derived")
			}
			value = filepath.Clean(filepath.Join(strings.TrimSpace(string(root)), value))
		}
	}
	sum := sha256.Sum256([]byte(strings.TrimSuffix(strings.TrimRight(value, "/"), ".git")))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func publicationRemote(ctx context.Context, repo string) (string, bool, error) {
	branchOutput, _ := runGit(ctx, repo, nil, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	branch := strings.TrimSpace(string(branchOutput))
	keys := make([]string, 0, 4)
	if branch != "" {
		keys = append(keys, "branch."+branch+".pushRemote")
	}
	keys = append(keys, "remote.pushDefault")
	if branch != "" {
		keys = append(keys, "branch."+branch+".remote")
	}
	for _, key := range keys {
		output, err := runGit(ctx, repo, nil, nil, "config", "--get", key)
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return strings.TrimSpace(string(output)), true, nil
		}
	}
	output, err := runGit(ctx, repo, nil, nil, "config", "--get", "remote.origin.url")
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return "origin", true, nil
	}
	return "", false, nil
}

// ErrReviewedDeliveryNotOneCommit reports the deterministic pre-push delivery
// shape rule for current-changes receipts: the candidate publishes a range
// that is not exactly one commit beyond the reviewed base the receipt froze.
// It is a typed statement about candidate shape versus the reviewed receipt —
// never about authority integrity — so receipt discovery can classify it as a
// receipt/scope mismatch instead of corruption. Infrastructure or derivation
// failures while computing the shape stay untyped and keep failing closed.
var ErrReviewedDeliveryNotOneCommit = errors.New("reviewed delivery is not exactly one commit from its reviewed base")

// validateBootstrapAncestryDisclosure enforces the bootstrap publication
// invariant for pre-base history. A push to an empty remote transfers every
// object reachable from HEAD, including ancestry behind the reviewed delivery
// base that no publication gate ever examined. That ancestry is admissible
// only under two disclosure rules bound to the reviewed base tree — exactly
// the immutable context the receipt binds:
//
//  1. Path disclosure: every path pre-base history ever named must exist in
//     the reviewed base tree, so no published tree entry carries a file name
//     review never saw.
//  2. Blob disclosure: every blob object reachable from the reviewed base
//     commit must be byte-identical to some blob the reviewed base tree
//     contains, path-agnostic. Renaming disclosed content is admissible; any
//     historical content revision absent from the base tree — for example a
//     secret later overwritten at the same path — fails closed even though
//     its path is disclosed.
//
// Symlink entries are blobs and participate in both rules. Gitlink
// (submodule) entries disclose only their path: their commit OIDs reference
// external objects that history traversal never transfers, so they are
// excluded from blob disclosure. Failing either rule requires squashing
// pre-publication history (or re-creating an orphan first commit) so the
// first publication contains only reviewed content.
//
// Scope: disclosure covers repository CONTENT — tracked paths and blob bytes
// — and nothing else. Commit and tag metadata (messages, author/committer
// identities, timestamps) is not review-bound anywhere in the gate model: no
// receipt hashes or binds message text, and an ordinary (non-bootstrap)
// pre-push equally publishes the reviewed delivery commit's message without
// any review binding it. A pre-base commit created with `git commit
// --allow-empty` therefore passes both rules even when its message contains a
// secret, and the bootstrap push transfers that commit object; bootstrap does
// not widen a guarantee that never existed. Operators who need message
// hygiene should follow the same remediation the deny messages give — squash
// pre-publication history or re-create an orphan first commit — which reduces
// pre-base history to messages the operator wrote deliberately at publication
// time. TestBootstrapDisclosureScopeExcludesCommitMetadata pins this
// boundary.
// reviewedBaseTreeDisclosure lists the reviewed base tree once and returns
// its canonical path set and its blob OID set. Symlink entries are blobs and
// join both sets; gitlink entries contribute only their path because their
// commit OIDs reference objects outside this repository.
// validateBootstrapReachableBlobs enforces blob-level disclosure: every blob
// object reachable from the reviewed base commit must be a member of the
// reviewed base tree's blob set. The subprocess count stays constant
// regardless of history size: rev-list enumerates each reachable object once
// with a representative path, and one cat-file batch resolves every object
// type; anything unresolvable fails closed.
//
// The cat-file --batch-check pass exists deliberately: do not "simplify" this
// to `git rev-list --objects --filter=object:type=blob`. Filter semantics
// proved version-shaky during design probing — git 2.55 dropped a traversed
// commit's line while keeping the provided one — so type resolution through
// one batch-check invocation is the version-stable approach.
func resolveCommit(ctx context.Context, repo, revision string) (string, error) {
	output, err := runGit(ctx, repo, nil, nil, "rev-parse", "--verify", strings.TrimSpace(revision)+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func validateGateRequest(request GateRequest) error {
	if request.Schema != GateRequestSchema {
		return errors.New("unsupported review gate request schema")
	}
	switch request.Gate {
	case GatePostApply, GatePreCommit, GatePrePush, GatePrePR, GateRelease:
	default:
		return fmt.Errorf("unsupported review gate %q", request.Gate)
	}
	if !validSHA256(request.StoreRevision) || !validSHA256(request.GenesisRevision) || !validSHA256(request.ChainIdentity) || !validSHA256(request.BundleDigest) {
		return errors.New("gate request requires the exact authoritative store revision, genesis, chain identity, and bundle digest")
	}
	// The target base ref is replayed verbatim as a Git argv element before the
	// pre-PR boundary re-resolution runs, so it is held to the same
	// structured-ref rule as an explicit boundary selector.
	if request.Target.BaseRef != "" {
		if err := validateBoundaryRefSelector("target base ref", request.Target.BaseRef); err != nil {
			return err
		}
	}
	if request.Gate == GateRelease && request.Release == nil {
		return errors.New("release gate requires an immutable release request")
	}
	if request.Gate != GateRelease && request.Release != nil {
		return errors.New("release request is only valid at the release gate")
	}
	if request.Gate != GatePrePR && request.PrePR != nil {
		return errors.New("pre-PR evidence is only valid at the pre-PR gate")
	}
	if request.Gate != GatePrePush && request.Push != nil {
		return errors.New("push evidence is only valid at the pre-push gate")
	}
	if request.Gate == GatePrePush && (request.Push == nil || request.Push.PushRemoteIdentity == "" || request.Push.Boundary.RemoteIdentity == "") {
		return errors.New("pre-push gate requires repository-bound push evidence")
	}
	if request.Gate == GatePrePR && (request.PrePR == nil || request.PrePR.Boundary == nil || request.PrePR.PushRemoteIdentity == "" || request.PrePR.Boundary.RemoteIdentity == "") {
		return errors.New("pre-PR gate requires repository-bound publication evidence; pass --base-ref <remote>/<branch>")
	}
	switch request.ExternalEvidence {
	case ExternalEvidenceNone, ExternalEvidenceInvalidating, ExternalEvidenceEscalating:
	default:
		return fmt.Errorf("invalid external evidence disposition %q", request.ExternalEvidence)
	}
	return nil
}

func hashLedgerArtifact(path string) (string, error) {
	hash, _, err := hashLedgerArtifactBinding(path)
	return hash, err
}

func hashArtifactSource(path, content string) (string, error) {
	if strings.TrimSpace(content) != "" {
		sum := sha256.Sum256([]byte(content))
		return "sha256:" + hex.EncodeToString(sum[:]), nil
	}
	return HashArtifact(path)
}

func hashLedgerArtifactSource(path, content string) (string, string, error) {
	if strings.TrimSpace(content) == "" {
		return hashLedgerArtifactBinding(path)
	}
	return hashLedgerPayload([]byte(content))
}

func hashLedgerArtifactBinding(path string) (string, string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return hashLedgerPayload(payload)
}

func hashLedgerPayload(payload []byte) (string, string, error) {
	return validateCanonicalLedger(payload, nil, "")
}

func HashLedgerArtifact(path string) (string, error) {
	return hashLedgerArtifact(path)
}

// EscalationAccountingReasonTemplate is the single source of truth for the
// escalation accounting sentence. The organic gate surface renders it through
// compactEscalatedGateReason, the SDD-bound surface through
// sddstatus.resolveBoundedRemediation (which aliases this constant), and the
// organic-dx narration registry samples it, so the surfaces cannot drift.
const EscalationAccountingReasonTemplate = "compact review authority is escalated (%s): spent %d, remaining %d, total %d correction lines"

// compactEscalatedGateReason names why a compact authority is terminally
// escalated in the numbers the frozen state already carries, instead of the
// bare terminal sentence that told an operator nothing about the budget they
// crossed. A state with no derivable escalation cause keeps that bare sentence
// rather than reporting accounting this cannot prove.
// gateVerdict is Wave 5 (Gate Cutover) Slice 3's total function over the
// 5 gates x 7 CandidateRelation values (task 4.4; design's Interfaces /
// Contracts sketch, extended with a GateContext parameter to carry the
// absorbed N2 per-gate preconditions -- design.md's literal two-argument
// signature cannot express a per-gate boundary precondition at all, so this
// is a disclosed, documented extension of it, not a silent deviation).
//
// Every one of the 35 (gate, relation) pairings resolves (task 4.1,
// TestGateVerdict_TotalFunction_35Cells); an unrecognized relation value
// (impossible from the closed CandidateRelation vocabulary, but the
// function must still be total) fails closed to invalidated rather than
// panicking or falling through to allow -- default deny.
//
// Absorbed N2 (W3 verify, PR0 task 4.7): the per-gate preconditions
// reproduce validateDerivedGate's own contract (receipt.go:279-321) instead
// of newLineageGateEvaluation's uniform continue->allow for every gate
// (review_governing_authority.go:240-261, the exact gap N2 identified) --
// BaseRelationshipValid is gated to pre-pr/release only (receipt.go:304),
// with the identical compatible_base_advance exemption; release evidence is
// gated to release only (receipt.go:307-314).
