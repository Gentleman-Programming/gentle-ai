package kickoff

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/reviewtransaction"
	"gopkg.in/yaml.v3"
)

// KickoffFileName is the sealed document's file name inside a change root.
const KickoffFileName = "kickoff.yaml"

// nowFunc is the injectable clock Seal uses for SealedAt. Production always
// resolves to time.Now().UTC(); tests substitute it to assert on a fixed
// timestamp without depending on wall-clock time.
var nowFunc = func() time.Time { return time.Now().UTC() }

// publishKickoff is a test seam over reviewtransaction.PublishFileNoReplace,
// mirroring the seam sddstatus.ensureChangeInstanceMarker already uses for
// its own single-write publication (edit_authority_consent.go:91).
var publishKickoff = reviewtransaction.PublishFileNoReplace

func kickoffPath(changeRoot string) string {
	return filepath.Join(changeRoot, KickoffFileName)
}

// Load reads the sealed kickoff document for a change. Absence of the file
// is not an error: it means "not sealed yet", so Load returns (nil, nil).
// Any other read or parse failure is a named error — a corrupt document is
// never treated as an unsealed change (T-10).
func Load(changeRoot string) (*Kickoff, error) {
	path := kickoffPath(changeRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("leer kickoff sellado en %q: %w", path, err)
	}
	parsed, err := ParseKickoff(data)
	if err != nil {
		return nil, fmt.Errorf("kickoff sellado invalido en %q: %w", path, err)
	}
	return &parsed, nil
}

// Seal writes k as the sealed kickoff.yaml for changeRoot exactly once.
// The first successful publication wins for the life of the change
// instance (D-01, D-02): a later Seal call with different content never
// overwrites it. Instead, Seal re-reads and returns the winner already on
// disk with ok=false, so the caller knows its own value was not persisted.
//
// The implementation mirrors sddstatus.ensureChangeInstanceMarker: publish
// a temporary file in changeRoot via PublishFileNoReplace, and on
// os.ErrExist, re-read the document that won the race instead of failing.
func Seal(changeRoot string, k Kickoff) (Kickoff, bool, error) {
	if k.SealedAt.IsZero() {
		k.SealedAt = nowFunc()
	}
	if err := k.Validate(); err != nil {
		return Kickoff{}, false, err
	}

	payload, err := yaml.Marshal(k)
	if err != nil {
		return Kickoff{}, false, fmt.Errorf("serializar kickoff.yaml: %w", err)
	}

	path := kickoffPath(changeRoot)
	temporary, err := os.CreateTemp(changeRoot, ".kickoff-*.yaml")
	if err != nil {
		return Kickoff{}, false, fmt.Errorf("crear publicacion temporal de kickoff.yaml: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return Kickoff{}, false, fmt.Errorf("escribir publicacion temporal de kickoff.yaml: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Kickoff{}, false, fmt.Errorf("cerrar publicacion temporal de kickoff.yaml: %w", err)
	}

	if err := publishKickoff(temporaryPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			winner, readErr := Load(changeRoot)
			if readErr != nil {
				return Kickoff{}, false, readErr
			}
			if winner == nil {
				return Kickoff{}, false, fmt.Errorf("kickoff.yaml existe pero no se pudo releer en %q", path)
			}
			return *winner, false, nil
		}
		return Kickoff{}, false, fmt.Errorf("publicar kickoff.yaml: %w", err)
	}

	winner, err := Load(changeRoot)
	if err != nil {
		return Kickoff{}, false, err
	}
	if winner == nil {
		return Kickoff{}, false, fmt.Errorf("kickoff.yaml publicado pero ausente en la relectura de %q", path)
	}
	return *winner, true, nil
}
