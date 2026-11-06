package reviewtransaction

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// An rctx3 handle seals the canonical repository root it was derived for,
// together with the identity digest that commits to that root, its Git
// directories, and the capture binding. Sealing keeps the #3797 contract --
// a reader holding the handle on a command line or in a host transcript
// learns nothing about the filesystem -- while letting the resolver open the
// repository directly instead of discovering it from the caller's cwd.
//
// Only OpenCode hosts receive rctx3: their relay runs in the host session
// directory, which says nothing about the review (#5136, #4516). Every other
// runtime keeps the rctx2 digest, verified against the caller repository.
//
// The seal is AES-256-GCM with a synthetic nonce, HMAC-SHA256 over the
// associated data and plaintext. The same tuple always yields the same handle,
// so STATUS can reissue it byte-identically across processes, and distinct
// tuples collide on a nonce only with negligible probability.
const (
	reviewRepositoryContextV3Schema       = "gentle-ai.review-repository-context/v3"
	reviewRepositoryContextV3HandlePrefix = "rctx3_"

	reviewRepositoryContextKeyFile  = "review-context.key"
	reviewRepositoryContextKeyBytes = 32

	reviewRepositoryContextV3NonceBytes   = 12
	reviewRepositoryContextV3TagBytes     = 16
	reviewRepositoryContextV3MaxRootBytes = 4096
	reviewRepositoryContextV3MinSealed    = reviewRepositoryContextV3NonceBytes + sha256.Size + 1 + reviewRepositoryContextV3TagBytes
	reviewRepositoryContextV3MaxSealed    = reviewRepositoryContextV3NonceBytes + sha256.Size + reviewRepositoryContextV3MaxRootBytes + reviewRepositoryContextV3TagBytes
)

var reviewRepositoryContextV3Encoding = base64.RawURLEncoding.Strict()

// ErrReviewRepositoryContextKeyUnsafe never names key bytes; it only names the
// file an operator has to repair. The wording carries no path separator, so
// the path-scrubbing failure envelope delivers it intact.
var ErrReviewRepositoryContextKeyUnsafe = errReviewRepositoryContextKeyUnsafe

var errReviewRepositoryContextKeyUnsafe = errors.New("the review context key file review-context.key in the .gentle-ai directory of your home directory is not a private 32-byte regular file; run `chmod 600 review-context.key` in that directory, or move the file aside so a new key is created, then re-run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent <agent> --lineage <lineage> --next-transition` to obtain a fresh repository context")

var errInvalidReviewRepositoryContextV3 = errors.New("invalid rctx3 repository context") // refusal:by-design operator-knowledge: callers must refresh the provider-issued repository context instead of attempting to repair an untrusted token

type reviewRepositoryContextV3ResolutionError struct{ cause error }

func (err *reviewRepositoryContextV3ResolutionError) Error() string {
	return errInvalidReviewRepositoryContextV3.Error()
}

func (err *reviewRepositoryContextV3ResolutionError) Unwrap() error { return err.cause }

func invalidReviewRepositoryContextV3Resolution(cause error) error {
	if cause == nil {
		return errInvalidReviewRepositoryContextV3
	}
	return &reviewRepositoryContextV3ResolutionError{cause: cause}
}

// reviewRepositoryContextUnsealedError refuses, at the OpenCode relay, any
// handle that is not the sealed rctx3 shape the relay's own STATUS issues. It
// carries the already-validated lineage so the remedy names the bound STATUS
// that reissues the Task, not a selectorless one that offers a fresh START.
type reviewRepositoryContextUnsealedError struct{ lineageID string }

func (err reviewRepositoryContextUnsealedError) Error() string {
	return "the OpenCode review relay accepts only the sealed rctx3 repository context its own STATUS issues; run `gentle-ai review status --cwd <repo> --contract gentle-ai.review-integration/v2 --agent <agent> --lineage " + err.lineageID + " --next-transition` to obtain the current provider-issued Task"
}

// Is matches every unsealed-handle refusal regardless of its lineage.
func (reviewRepositoryContextUnsealedError) Is(target error) bool {
	_, ok := target.(reviewRepositoryContextUnsealedError)
	return ok
}

// UnsealedReviewRepositoryContextLineage returns the validated lineage a relay
// refusal of an unsealed handle was bound to, so the caller can name the exact
// STATUS continuation for its own runtime.
func UnsealedReviewRepositoryContextLineage(err error) (string, bool) {
	var unsealed reviewRepositoryContextUnsealedError
	if !errors.As(err, &unsealed) || unsealed.lineageID == "" {
		return "", false
	}
	return unsealed.lineageID, true
}

func (reviewRepositoryContextUnsealedError) Unwrap() error {
	return errInvalidReviewRepositoryContextV3
}

// ErrUnsealedReviewRepositoryContext matches, through errors.Is, the relay's
// refusal of a handle that is not rctx3, such as an rctx2 digest.
var ErrUnsealedReviewRepositoryContext error = reviewRepositoryContextUnsealedError{}

// DeriveOpenCodeReviewRepositoryContextHandle derives the sealed rctx3 handle
// issued to OpenCode hosts. It is deterministic, so START and every later
// STATUS render the same handle; its only side effect is creating this user's
// private sealing key on first use.
func DeriveOpenCodeReviewRepositoryContextHandle(ctx context.Context, repo string, binding ReviewRepositoryContextBinding) (string, error) {
	if ctx == nil || ctx.Err() != nil || validateReviewRepositoryContextBinding(binding) != nil {
		return "", errInvalidReviewRepositoryContextV3
	}
	lease, err := OpenRepositoryIdentityLease(ctx, repo)
	if err != nil || lease.Validate(ctx) != nil {
		return "", errInvalidReviewRepositoryContextV3
	}
	identity := lease.Identity()
	digest, err := reviewRepositoryContextV3Digest(identity, binding)
	if err != nil {
		return "", errInvalidReviewRepositoryContextV3
	}
	key, err := loadReviewRepositoryContextKey(true)
	if err != nil {
		if errors.Is(err, errReviewRepositoryContextKeyUnsafe) {
			return "", errReviewRepositoryContextKeyUnsafe
		}
		return "", invalidReviewRepositoryContextV3Resolution(err)
	}
	return sealReviewRepositoryContextV3(key, identity.RepositoryRoot, digest, binding)
}

// ResolveOpenCodeReviewRepositoryContextBinding is the OpenCode relay's only
// resolver. It accepts the sealed rctx3 handle alone; an rctx2 digest names no
// root, so the relay refuses it with the STATUS that reissues the Task instead
// of searching the host session for a candidate.
func ResolveOpenCodeReviewRepositoryContextBinding(ctx context.Context, handle string, binding ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, error) {
	// The binding is validated first: the refusal below echoes its lineage
	// into an operator-facing command, so it must be canonical.
	if validateReviewRepositoryContextBinding(binding) != nil {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV3
	}
	if !strings.HasPrefix(handle, reviewRepositoryContextV3HandlePrefix) {
		return "", ReviewRepositoryContextBinding{}, reviewRepositoryContextUnsealedError{lineageID: binding.LineageID}
	}
	return resolveSealedReviewRepositoryContext(ctx, handle, binding)
}

// resolveSealedReviewRepositoryContext authenticates an rctx3 handle under
// this user's key and the caller's binding, opens the identity lease at
// exactly the sealed root -- no caller cwd, host discovery, or candidate
// enumeration participates -- requires the digest re-derived from that live
// identity to equal the sealed one, and then requires live compact authority.
// A handle sealed under another key or binding fails authentication, a moved
// root fails the lease, a root naming another repository fails the digest, and
// a repository replaced in place holds no live authority for the binding.
func resolveSealedReviewRepositoryContext(ctx context.Context, handle string, binding ReviewRepositoryContextBinding) (string, ReviewRepositoryContextBinding, error) {
	if ctx == nil || ctx.Err() != nil || validateReviewRepositoryContextBinding(binding) != nil || !validReviewRepositoryContextV3Handle(handle) {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV3
	}
	key, err := loadReviewRepositoryContextKey(false)
	if err != nil {
		if errors.Is(err, errReviewRepositoryContextKeyUnsafe) {
			return "", ReviewRepositoryContextBinding{}, errReviewRepositoryContextKeyUnsafe
		}
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV3Resolution(err)
	}
	root, sealedDigest, err := openReviewRepositoryContextV3(key, handle, binding)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV3
	}
	lease, err := openRepositoryIdentityLeaseAtRoot(ctx, root)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV3Resolution(err)
	}
	if err := lease.Validate(ctx); err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV3Resolution(err)
	}
	identity := lease.Identity()
	liveDigest, err := reviewRepositoryContextV3Digest(identity, binding)
	if err != nil || identity.RepositoryRoot != root || subtle.ConstantTimeCompare(liveDigest, sealedDigest) != 1 {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV3
	}
	store, err := CompactAuthoritativeStore(ctx, identity.RepositoryRoot, binding.LineageID)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV3Resolution(err)
	}
	record, err := store.LoadContext(ctx)
	if err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV3Resolution(err)
	}
	if err := validateReviewRepositoryContextRecord(ctx, identity.RepositoryRoot, binding, record); err != nil {
		return "", ReviewRepositoryContextBinding{}, errInvalidReviewRepositoryContextV3
	}
	if err := lease.Validate(ctx); err != nil {
		return "", ReviewRepositoryContextBinding{}, invalidReviewRepositoryContextV3Resolution(err)
	}
	return identity.RepositoryRoot, binding, nil
}

// validReviewRepositoryContextV3Handle validates only the opaque transport
// shape: the v3 prefix followed by canonical unpadded base64url whose decoded
// length lies within the sealed bounds.
func validReviewRepositoryContextV3Handle(handle string) bool {
	encoded, found := strings.CutPrefix(handle, reviewRepositoryContextV3HandlePrefix)
	if !found || len(encoded) < reviewRepositoryContextV3Encoding.EncodedLen(reviewRepositoryContextV3MinSealed) ||
		len(encoded) > reviewRepositoryContextV3Encoding.EncodedLen(reviewRepositoryContextV3MaxSealed) {
		return false
	}
	sealed, err := reviewRepositoryContextV3Encoding.DecodeString(encoded)
	return err == nil && len(sealed) >= reviewRepositoryContextV3MinSealed && len(sealed) <= reviewRepositoryContextV3MaxSealed
}

// reviewRepositoryContextV3Digest commits to the full repository identity and
// capture binding. The sealed root alone proves nothing: resolution re-derives
// this digest from the live lease opened at that root and requires equality.
func reviewRepositoryContextV3Digest(identity RepositoryIdentity, binding ReviewRepositoryContextBinding) ([]byte, error) {
	payload, err := canonicalReviewRepositoryContextV2Payload(reviewRepositoryContextV2Token{
		Schema:               reviewRepositoryContextV3Schema,
		RepositoryRoot:       identity.RepositoryRoot,
		GitCommonDir:         identity.GitCommonDir,
		GitDir:               identity.GitDir,
		RepositoryRef:        identity.RepositoryRef,
		LineageID:            binding.LineageID,
		TargetIdentity:       binding.TargetIdentity,
		CapturePhaseRevision: binding.Revision,
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(payload)
	return digest[:], nil
}

func reviewRepositoryContextV3AssociatedData(binding ReviewRepositoryContextBinding) []byte {
	return []byte(reviewRepositoryContextV3Schema + "\x00" + binding.LineageID + "\x00" + binding.TargetIdentity + "\x00" + binding.Revision)
}

func reviewRepositoryContextV3Subkey(key []byte, purpose string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(reviewRepositoryContextV3Schema + "\x00" + purpose))
	return mac.Sum(nil)
}

func reviewRepositoryContextV3Cipher(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(reviewRepositoryContextV3Subkey(key, "seal"))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func reviewRepositoryContextV3Nonce(key, associated, plaintext []byte) []byte {
	mac := hmac.New(sha256.New, reviewRepositoryContextV3Subkey(key, "nonce"))
	mac.Write(associated)
	mac.Write([]byte{0})
	mac.Write(plaintext)
	return mac.Sum(nil)[:reviewRepositoryContextV3NonceBytes]
}

func sealReviewRepositoryContextV3(key []byte, root string, digest []byte, binding ReviewRepositoryContextBinding) (string, error) {
	if len(key) != reviewRepositoryContextKeyBytes || len(digest) != sha256.Size || !validReviewRepositoryContextV3Root(root) {
		return "", errInvalidReviewRepositoryContextV3
	}
	aead, err := reviewRepositoryContextV3Cipher(key)
	if err != nil {
		return "", errInvalidReviewRepositoryContextV3
	}
	associated := reviewRepositoryContextV3AssociatedData(binding)
	plaintext := append(append([]byte(nil), digest...), root...)
	nonce := reviewRepositoryContextV3Nonce(key, associated, plaintext)
	sealed := aead.Seal(append([]byte(nil), nonce...), nonce, plaintext, associated)
	return reviewRepositoryContextV3HandlePrefix + reviewRepositoryContextV3Encoding.EncodeToString(sealed), nil
}

// openReviewRepositoryContextV3 authenticates a handle under this user's key
// and the caller-supplied binding, and returns the sealed root and digest. A
// handle sealed under another key, for another binding, or altered in any byte
// fails here before any filesystem path is touched.
func openReviewRepositoryContextV3(key []byte, handle string, binding ReviewRepositoryContextBinding) (string, []byte, error) {
	if len(key) != reviewRepositoryContextKeyBytes || !validReviewRepositoryContextV3Handle(handle) {
		return "", nil, errInvalidReviewRepositoryContextV3
	}
	sealed, err := reviewRepositoryContextV3Encoding.DecodeString(strings.TrimPrefix(handle, reviewRepositoryContextV3HandlePrefix))
	if err != nil {
		return "", nil, errInvalidReviewRepositoryContextV3
	}
	aead, err := reviewRepositoryContextV3Cipher(key)
	if err != nil {
		return "", nil, errInvalidReviewRepositoryContextV3
	}
	associated := reviewRepositoryContextV3AssociatedData(binding)
	nonce, ciphertext := sealed[:reviewRepositoryContextV3NonceBytes], sealed[reviewRepositoryContextV3NonceBytes:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, associated)
	if err != nil || len(plaintext) <= sha256.Size {
		return "", nil, errInvalidReviewRepositoryContextV3
	}
	// The synthetic nonce is a function of the plaintext, so only the one
	// canonical encoding of each tuple is accepted.
	if subtle.ConstantTimeCompare(nonce, reviewRepositoryContextV3Nonce(key, associated, plaintext)) != 1 {
		return "", nil, errInvalidReviewRepositoryContextV3
	}
	root := string(plaintext[sha256.Size:])
	if !validReviewRepositoryContextV3Root(root) {
		return "", nil, errInvalidReviewRepositoryContextV3
	}
	return root, plaintext[:sha256.Size], nil
}

func validReviewRepositoryContextV3Root(root string) bool {
	return len(root) <= reviewRepositoryContextV3MaxRootBytes && utf8.ValidString(root) && validReviewRepositoryContextV2Path(root)
}

// ReviewRepositoryContextKeyHealth reports, read-only, whether this user's
// sealing key can be used. An absent key is healthy: the next derivation
// creates it. A present key that is not a private 32-byte regular file
// returns the actionable ErrReviewRepositoryContextKeyUnsafe refusal.
func ReviewRepositoryContextKeyHealth() error {
	_, err := loadReviewRepositoryContextKey(false)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if errors.Is(err, errReviewRepositoryContextKeyUnsafe) {
		return errReviewRepositoryContextKeyUnsafe
	}
	return invalidReviewRepositoryContextV3Resolution(err)
}

// loadReviewRepositoryContextKey returns this user's sealing key. Derivation
// creates it on first use; resolution never does, because a handle sealed
// under a key that no longer exists can only be replaced by a fresh STATUS.
func loadReviewRepositoryContextKey(create bool) ([]byte, error) {
	home, err := reviewRepositoryContextHome()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".gentle-ai")
	path := filepath.Join(dir, reviewRepositoryContextKeyFile)
	key, err := readReviewRepositoryContextKey(path)
	if err == nil || !create || !errors.Is(err, fs.ErrNotExist) {
		return key, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := publishReviewRepositoryContextKey(dir, path); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	// Concurrent creators converge on whichever key was linked first.
	return readReviewRepositoryContextKey(path)
}

// publishReviewRepositoryContextKey writes a complete private key under a
// unique name and links it into place, so the key path either does not exist
// or names all 32 bytes; no reader can observe a partially written key.
func publishReviewRepositoryContextKey(dir, path string) error {
	key := make([]byte, reviewRepositoryContextKeyBytes)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return err
	}
	staged, err := os.CreateTemp(dir, "."+reviewRepositoryContextKeyFile+".*")
	if err != nil {
		return err
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if err := staged.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = staged.Close()
		return err
	}
	_, writeErr := staged.Write(key)
	syncErr := staged.Sync()
	if closeErr := staged.Close(); writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.Join(writeErr, syncErr, closeErr)
	}
	return os.Link(stagedPath, path)
}

func readReviewRepositoryContextKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", errReviewRepositoryContextKeyUnsafe, err)
	}
	if !reviewRepositoryContextKeyInfoSafe(info) {
		return nil, errReviewRepositoryContextKeyUnsafe
	}
	file, err := openReviewRepositoryContext(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errReviewRepositoryContextKeyUnsafe, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !reviewRepositoryContextKeyInfoSafe(opened) || !os.SameFile(info, opened) {
		return nil, errReviewRepositoryContextKeyUnsafe
	}
	key, err := io.ReadAll(io.LimitReader(file, reviewRepositoryContextKeyBytes+1))
	if err != nil || len(key) != reviewRepositoryContextKeyBytes {
		return nil, errReviewRepositoryContextKeyUnsafe
	}
	return key, nil
}

func reviewRepositoryContextKeyInfoSafe(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() && info.Size() == reviewRepositoryContextKeyBytes &&
		(runtime.GOOS == "windows" || info.Mode().Perm()&0o077 == 0)
}
