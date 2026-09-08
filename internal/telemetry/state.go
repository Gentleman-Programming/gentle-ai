package telemetry

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
)

// stateDir matches internal/state's own stateDir: telemetry state lives next
// to the rest of Gentle AI's uncommitted user state, under the same home
// directory resolution so tests can point both at one temp dir.
const stateDir = ".gentle-ai"
const stateFile = "telemetry.json"

// Counters counts activity since the previous successful send. They are
// reset to zero only after a 2xx response for a heartbeat event; a failed or
// skipped send leaves them untouched so nothing is lost.
type Counters struct {
	Syncs             int `json:"syncs"`
	SDDPhaseRuns      int `json:"sdd_phase_runs"`
	ReviewsApproved   int `json:"reviews_approved"`
	ReviewsCorrection int `json:"reviews_correction"`
	ReviewsEscalated  int `json:"reviews_escalated"`
}

// IsZero reports whether every counter is at its zero value.
func (c Counters) IsZero() bool {
	return c == Counters{}
}

// State is the persisted telemetry state file. InstallID is generated once
// and never changes; Enabled is the user's local opt-out; NoticeShown
// records whether the one-time disclosure line has already been printed.
type State struct {
	InstallID         string     `json:"install_id"`
	Enabled           bool       `json:"enabled"`
	NoticeShown       bool       `json:"notice_shown"`
	LastInstallSentAt *time.Time `json:"last_install_sent_at,omitempty"`
	LastHeartbeatAt   *time.Time `json:"last_heartbeat_at,omitempty"`
	// LastFailureAt records the last time a send attempt failed (network
	// error or non-2xx). Opportunistic refuses to spawn another sender
	// until FailureBackoff has passed since this timestamp, so a blocked or
	// unreachable endpoint does not respawn a failing child on every trigger.
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
	// LastAttemptAt records the last time Opportunistic handed an event to
	// Spawn, persisted before the (detached, possibly never-completing)
	// sender runs. It throttles a burst of triggers from forking a second
	// sender while the first is still in flight or was killed before it
	// could record success or failure. Intentionally not projected by
	// `telemetry status` (see TelemetryStatusResult): it is an internal
	// throttle detail, not part of the pinned status contract.
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	Counters      Counters   `json:"counters"`
}

// UnmarshalJSON preserves the default-enabled contract: a state file written
// before the "enabled" key existed, or one that never mentions it, means the
// user never opted out. Only an explicit `"enabled": false` disables sending.
func (s *State) UnmarshalJSON(data []byte) error {
	type plainState State
	var decoded plainState
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*s = State(decoded)
	if _, present := fields["enabled"]; !present {
		s.Enabled = true
	}
	return nil
}

// Path returns the absolute path to the telemetry state file for the given
// home directory.
func Path(homeDir string) string {
	return filepath.Join(homeDir, stateDir, stateFile)
}

// NewState returns a fresh, enabled state with a newly generated install_id.
func NewState() (State, error) {
	id, err := newInstallID()
	if err != nil {
		return State{}, err
	}
	return State{InstallID: id, Enabled: true}, nil
}

// Load reads the telemetry state file for homeDir and returns it verbatim.
// It never mints or persists an install_id: a missing file returns its
// os.ErrNotExist unchanged. Callers that need a durable, stable install_id
// (anything that reports or sends it) must use EnsureState instead — Load
// exists for read-only checks that must not create a file as a side effect
// (e.g. deciding whether telemetry is disabled before touching disk at all).
func Load(homeDir string) (State, error) {
	data, err := os.ReadFile(Path(homeDir))
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// loadForDecision reads state for a kill-switch decision without ever
// creating anything: a missing file means "nothing has opted out locally
// yet" (State{Enabled: true}), and any other read error is returned as-is so
// callers fail safe instead of guessing.
func loadForDecision(homeDir string) (State, error) {
	s, err := Load(homeDir)
	switch {
	case err == nil:
		return s, nil
	case os.IsNotExist(err):
		return State{Enabled: true}, nil
	default:
		return State{}, err
	}
}

// EnsureState loads the persisted state, or creates and durably persists a
// fresh one (atomically: a temp file synced then renamed into place, via
// filemerge.WriteFileAtomic) when none exists yet, or when an existing file
// somehow carries no install_id (a corrupt or pre-install_id legacy file).
// This is the one function every caller that reports or sends an install_id
// must use: it is what makes the id stable across consecutive calls
// (`status`, `preview`, and the opportunistic sender all resolve to the same
// persisted value instead of each minting their own).
func EnsureState(homeDir string) (State, error) {
	s, err := Load(homeDir)
	switch {
	case err == nil && s.InstallID != "":
		return s, nil
	case err != nil && !os.IsNotExist(err):
		return State{}, err
	}

	fresh, mintErr := NewState()
	if mintErr != nil {
		return State{}, mintErr
	}
	if err == nil {
		// A file existed but carried no install_id: preserve every other
		// field the user may already have set (opt-out, notice history,
		// counters, send timestamps) and only fill in the missing id.
		fresh.Enabled = s.Enabled
		fresh.NoticeShown = s.NoticeShown
		fresh.LastInstallSentAt = s.LastInstallSentAt
		fresh.LastHeartbeatAt = s.LastHeartbeatAt
		fresh.LastFailureAt = s.LastFailureAt
		fresh.Counters = s.Counters
	}
	if err := Save(homeDir, fresh); err != nil {
		return State{}, err
	}
	return fresh, nil
}

// Save persists the telemetry state file atomically, creating the
// containing directory if needed.
func Save(homeDir string, s State) error {
	dir := filepath.Join(homeDir, stateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = filemerge.WriteFileAtomic(Path(homeDir), data, 0o644)
	return err
}

// newInstallID generates a random UUID v4, per RFC 4122.
func newInstallID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate install id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
