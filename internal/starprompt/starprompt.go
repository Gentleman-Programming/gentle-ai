// Package starprompt holds the pure logic and persistence for the non-intrusive
// GitHub star offer (issue #5969). The TUI layer (internal/tui) owns the
// rendering and key routing; everything here is testable without Bubbletea.
package starprompt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Schema identifies the star-prompt state file version.
const Schema = "gentle-ai.star-prompt/v1"

// FileName is the state file, kept next to state.json inside the gentle-ai
// state directory but separate from InstallState so the two schemas never
// couple.
const FileName = "star-prompt.json"

// Repository is the GitHub repository the offer points at.
const Repository = "Gentleman-Programming/gentle-ai"

// RepositoryURL is the browser fallback target.
const RepositoryURL = "https://github.com/" + Repository

// Threshold is how many successful completed operations stay silent; the offer
// may appear from Threshold+1 on (t3code parity).
const Threshold = 5

// IdleDelay is how long the TUI must see no keypress before offering.
const IdleDelay = 30 * time.Second

// VisibleTimeout is how long the offer stays up without an answer.
const VisibleTimeout = 5 * time.Minute

// State is the persisted star-prompt state.
type State struct {
	Schema       string `json:"schema"`
	CompletedOps int    `json:"completed_ops"`
	Dismissed    bool   `json:"dismissed"`
	// DirectStar opts in to starring through the authenticated gh account after
	// the explicit Ctrl+G keypress. Default false: the chord opens the browser.
	DirectStar bool `json:"direct_star"`
}

// Default returns the fresh-install state.
func Default() State {
	return State{Schema: Schema}
}

// ReadResult carries a state read plus the fail-closed verdict: when Unusable
// is true the TUI must never ask, because a dismissal could not have been
// saved either.
type ReadResult struct {
	State     State
	Unusable  bool
	Malformed bool
	ReadError bool
	Missing   bool
}

// Read loads the state file from dir. A missing file is a fresh install, not
// an error; malformed or unreadable content fails closed.
func Read(dir string) ReadResult {
	res := ReadResult{State: Default()}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			res.Missing = true
			return res
		}
		res.ReadError = true
		res.Unusable = true
		return res
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		res.Malformed = true
		res.Unusable = true
		return res
	}
	if s.Schema != Schema || s.CompletedOps < 0 {
		res.Malformed = true
		res.Unusable = true
		return res
	}
	res.State = s
	return res
}

// Write persists the state; the caller must treat an error as Unusable.
func Write(dir string, s State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, FileName), append(data, '\n'), 0o644)
}

// ShouldOffer reports whether the offer may appear right now. Callers pass
// unusable=true when state persistence is broken: then the answer is always no
// (t3code courtesy: a dismissal that cannot be saved must disable asking).
func ShouldOffer(s State, unusable, offeredThisSession, operationRunning bool) bool {
	if unusable || s.Dismissed || offeredThisSession || operationRunning {
		return false
	}
	return s.CompletedOps > Threshold
}

// Outcome classifies a gh result. Unknown means ambiguity: the TUI must fall
// back to the browser instead of claiming success or failure.
type Outcome int

const (
	OutcomeYes Outcome = iota
	OutcomeNo
	OutcomeUnknown
)

// GhRunner executes one gh invocation and reports exit code plus stderr.
type GhRunner func(ctx context.Context, ghPath string, args ...string) (code int, stderr string, err error)

// DefaultGhRunner runs gh with a bounded timeout.
func DefaultGhRunner(ctx context.Context, ghPath string, args ...string) (int, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ghPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}
	return code, stderr.String(), err
}

// GhStarred resolves whether the authenticated gh account already starred the
// repository. Unknown on any ambiguity (network failure, auth error).
func GhStarred(ctx context.Context, run GhRunner, ghPath, repository string) Outcome {
	code, stderr, err := run(ctx, ghPath, "api", "user/starred/"+repository)
	if err != nil && code == -1 {
		return OutcomeUnknown
	}
	if code == 0 {
		return OutcomeYes
	}
	if strings.Contains(stderr, "HTTP 404") {
		return OutcomeNo
	}
	return OutcomeUnknown
}

// GhStar stars the repository with the authenticated gh account.
func GhStar(ctx context.Context, run GhRunner, ghPath, repository string) Outcome {
	code, stderr, err := run(ctx, ghPath, "api", "--method", "PUT", "user/starred/"+repository)
	if err != nil && code == -1 {
		return OutcomeUnknown
	}
	if code == 0 {
		return OutcomeYes
	}
	if strings.Contains(stderr, "HTTP 404") {
		return OutcomeNo
	}
	return OutcomeUnknown
}
