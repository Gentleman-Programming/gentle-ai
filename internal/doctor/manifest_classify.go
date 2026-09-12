package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// ErrUnknownClassification is returned by RunDoctor when the manifest cannot
// be read or Classify reports "unknown" — the only classification that
// indicates a real failure mode (legacy install, missing data, interrupted
// run). The CLI maps this to a non-zero exit code.
var ErrUnknownClassification = errors.New("managed assets classification is unknown")

// Kind enumerates the five doctor classification outcomes. Exactly one is
// returned by Classify.
type Kind string

const (
	Aligned      Kind = "aligned"
	Stale        Kind = "stale"
	Mixed        Kind = "mixed"
	UserModified Kind = "user_modified"
	Unknown      Kind = "unknown"
)

// Classification is the read-only outcome of classifying the binary-vs-bundle
// state. Hint is non-nil only when classification + ownership make the remedy
// safe to advertise. An overwrite hint MUST NEVER accompany UserModified or
// Unknown.
type Classification struct {
	Kind Kind
	Hint *Remedy
}

// Classify returns the read-only classification of the managed bundle
// against the running binary version. Pure: it never mutates the manifest,
// the on-disk resources, or the journal.
//
// The first two checks return Unknown with a sync hint (the only hint that
// is safe to advertise). Otherwise the kind is derived from the digest and
// version comparison below. UserModified always wins over Mixed/Aligned/Stale
// when any user-owned resource has drifted, and never carries an overwrite
// hint — sync must not rewrite what the user owns.
func Classify(m state.Manifest, journal []state.JournalEntry, binaryVersion string) Classification {
	if isAbsentManifest(m) {
		return Classification{Kind: Unknown, Hint: NewRemedy(RemedySync, "legacy install: no manifest; run `gentle-ai sync`")}
	}
	if hasInterruptedRun(journal) {
		return Classification{Kind: Unknown, Hint: NewRemedy(RemedySync, "interrupted sync; run `gentle-ai sync` to recover")}
	}

	cmp := compareVersions(binaryVersion, m.Producer.BinaryVersion)

	anyUserModified, allMatchDesired := false, true
	for _, r := range m.Resources {
		observed, err := digestOwnedExtent(r.Target, r.OwnedExtent)
		if err != nil {
			// If the file cannot be read, treat the resource as a definite
			// mismatch and let the ownership check decide the final shape.
			allMatchDesired = false
			if r.OwnedExtent.Ownership == state.OwnershipUser {
				anyUserModified = true
			}
			continue
		}
		if observed != r.Desired {
			allMatchDesired = false
			if r.OwnedExtent.Ownership == state.OwnershipUser {
				anyUserModified = true
			}
		}
	}

	if anyUserModified {
		// Never advertise an overwrite for user-owned resources.
		return Classification{Kind: UserModified}
	}

	switch {
	case allMatchDesired && cmp > 0:
		return Classification{Kind: Mixed, Hint: NewRemedy(RemedySync, "binary is newer than managed assets; run `gentle-ai sync`")}
	case allMatchDesired && cmp == 0:
		return Classification{Kind: Aligned}
	default:
		return Classification{Kind: Stale, Hint: NewRemedy(RemedySync, "managed assets are stale; run `gentle-ai sync`")}
	}
}

// isAbsentManifest reports whether m has no producer / no schema — the
// legacy-install shape where the manifest file does not exist on disk.
func isAbsentManifest(m state.Manifest) bool {
	return m.Schema == "" && m.Producer.BinaryVersion == ""
}

// hasInterruptedRun reports whether any run_id in the journal has an
// "intent" entry without a matching "complete" entry, OR an explicit
// "interrupted" marker.
func hasInterruptedRun(journal []state.JournalEntry) bool {
	completed := make(map[string]struct{})
	hasIntent := make(map[string]struct{})
	hasInterrupted := false
	for _, e := range journal {
		switch e.Op {
		case "intent":
			hasIntent[e.RunID] = struct{}{}
		case "complete":
			completed[e.RunID] = struct{}{}
		case "interrupted":
			hasInterrupted = true
		}
	}
	if hasInterrupted {
		return true
	}
	for runID := range hasIntent {
		if _, ok := completed[runID]; !ok {
			return true
		}
	}
	return false
}

// digestOwnedExtent returns the "sha256:<hex>" digest of the region
// described by ext for the file at path. Returns an error if the file is
// unreadable; markers outside the file bounds likewise error.
func digestOwnedExtent(path string, ext state.OwnedExtent) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var region []byte
	switch ext.Kind {
	case state.ExtentFull:
		region = data
	case state.ExtentMarkerBlock:
		if ext.Start < 0 || ext.End > len(data) || ext.Start >= ext.End {
			return "", fmt.Errorf("marker-block extent out of range: start=%d end=%d len=%d", ext.Start, ext.End, len(data))
		}
		region = data[ext.Start:ext.End]
	default:
		return "", fmt.Errorf("unknown extent kind: %q", ext.Kind)
	}

	sum := sha256.Sum256(region)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// compareVersions returns -1 if a<b, 0 if a==b, +1 if a>b. Comparison is
// done on the dot-separated numeric prefix; non-numeric suffixes (rc tags,
// refresh tags, "+build" suffixes) are ignored for ordering.
func compareVersions(a, b string) int {
	return compareVersionParts(parseVersion(a), parseVersion(b))
}

// parseVersion returns the numeric prefix components of v. Suffixes
// starting at the first '-' are stripped. Non-numeric components are
// treated as 0.
func parseVersion(v string) []int {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

func compareVersionParts(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ai := 0
		bi := 0
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}
