package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// PiBackgroundSubagentsEnv is the environment source for the managed Pi
// background-subagent preference. It mirrors the OpenCode contract; there is
// no launcher or activation plumbing behind it because the primitive is the
// already-installed pi-subagents extension reading a projected policy file.
const PiBackgroundSubagentsEnv = "GENTLE_AI_PI_BACKGROUND_SUBAGENTS"

// PiConfigHomeEnv overrides gentle-pi's config base directory (default
// ~/.pi), matching gentle-pi's own configuration precedent.
const PiConfigHomeEnv = "GENTLE_PI_CONFIG_HOME"

// piBackgroundPolicySchema marks the projected policy file as managed by this
// contract. A file at the managed path without this schema field is foreign
// and is never overwritten.
const piBackgroundPolicySchema = "gentle-pi.background-subagents/v1"

// PiBackgroundResolveInput contains already-discovered sources. The resolver
// is pure so loading state and reading the process environment remain caller
// responsibilities.
type PiBackgroundResolveInput struct {
	CLISet       bool
	CLIValue     model.PiBackgroundIntent
	EnvSet       bool
	EnvValue     string
	PriorManaged model.PiBackgroundIntent
	Interactive  bool
}

// PiBackgroundResolution separates the winning intent from the concrete
// runtime outcome. Persist is non-empty only for a newly supplied on/off
// choice; prior state is consulted by auto but is never republished here.
type PiBackgroundResolution struct {
	Intent         model.PiBackgroundIntent
	Effective      model.PiBackgroundIntent
	Persist        model.PiBackgroundIntent
	NeedsPrompt    bool
	projectionPlan *piBackgroundProjectionPlan

	// managed marks an explicit or prior-managed decision; implicit auto never projects.
	managed bool
}

// ResolvePiBackground applies CLI > non-empty env > prior managed state >
// auto. Auto consults prior on/off state; only implicit unresolved auto
// prompts interactively, while explicit auto stays auto without enabling.
// Unresolved non-interactive auto resolves to off.
func ResolvePiBackground(input PiBackgroundResolveInput) (PiBackgroundResolution, error) {
	cliValue, err := parsePiBackgroundIntent("explicit CLI", string(input.CLIValue), input.CLISet)
	if err != nil {
		return PiBackgroundResolution{}, err
	}
	envValue, err := parsePiBackgroundIntent(PiBackgroundSubagentsEnv, input.EnvValue, input.EnvSet && input.EnvValue != "")
	if err != nil {
		return PiBackgroundResolution{}, err
	}
	prior, err := parsePiBackgroundIntent("prior managed state", string(input.PriorManaged), input.PriorManaged != "")
	if err != nil {
		return PiBackgroundResolution{}, err
	}

	selected := model.PiBackgroundAuto
	explicit := false
	switch {
	case input.CLISet:
		selected = cliValue
		explicit = true
	case input.EnvSet && input.EnvValue != "":
		selected = envValue
		explicit = true
	case prior == model.PiBackgroundOn || prior == model.PiBackgroundOff:
		selected = prior
	}

	result := PiBackgroundResolution{Intent: selected}
	switch selected {
	case model.PiBackgroundOn, model.PiBackgroundOff:
		result.Effective = selected
		result.managed = true
		if explicit {
			result.Persist = selected
		}
	case model.PiBackgroundAuto:
		// Explicit/env auto is still auto policy: an existing managed choice is
		// safe to reuse, but auto never creates a managed choice by itself.
		if prior == model.PiBackgroundOn || prior == model.PiBackgroundOff {
			result.Effective = prior
			result.managed = true
		} else if explicit && input.Interactive {
			result.Effective = model.PiBackgroundAuto
		} else if input.Interactive {
			result.Effective = model.PiBackgroundAuto
			result.NeedsPrompt = true
		} else {
			result.Effective = model.PiBackgroundOff
		}
	}
	return result, nil
}

func parsePiBackgroundIntent(source, raw string, present bool) (model.PiBackgroundIntent, error) {
	if !present {
		return "", nil
	}
	parsed, err := model.ParsePiBackgroundIntent(raw)
	if err != nil {
		return "", fmt.Errorf("%s=%q: %w; use auto, on, or off", source, raw, err)
	}
	return parsed, nil
}

func resolvePiBackgroundCLI(set bool, raw string, persisted state.InstallState) (PiBackgroundResolution, error) {
	envValue, envSet := os.LookupEnv(PiBackgroundSubagentsEnv)
	return ResolvePiBackground(PiBackgroundResolveInput{
		CLISet:       set,
		CLIValue:     model.PiBackgroundIntent(raw),
		EnvSet:       envSet,
		EnvValue:     envValue,
		PriorManaged: persisted.PiBackgroundIntent,
		Interactive:  false,
	})
}

// ResolvePiBackgroundInteractive resolves the TUI's environment and persisted
// sources with interactive auto behavior. CLI flags are absent from the TUI
// flow, so a missing prior and missing environment decision is the only case
// that requests the TUI choice screen.
func ResolvePiBackgroundInteractive(prior model.PiBackgroundIntent) (PiBackgroundResolution, error) {
	envValue, envSet := os.LookupEnv(PiBackgroundSubagentsEnv)
	return ResolvePiBackground(PiBackgroundResolveInput{
		EnvSet:       envSet,
		EnvValue:     envValue,
		PriorManaged: prior,
		Interactive:  true,
	})
}

// piBackgroundPolicyPath resolves the gentle-pi-readable policy location:
// <base>/gentle-ai/background-subagents.json where base is
// GENTLE_PI_CONFIG_HOME when set, otherwise ~/.pi.
func piBackgroundPolicyPath(homeDir string) string {
	base := strings.TrimSpace(os.Getenv(PiConfigHomeEnv))
	if base == "" {
		base = filepath.Join(homeDir, ".pi")
	}
	return filepath.Join(base, "gentle-ai", "background-subagents.json")
}

// piBackgroundProjectionPlan writes the RESOLVED effective on/off policy to
// the gentle-pi-readable location. Auto never reaches projection: it resolves
// to on/off (or stays foreground) before a plan is prepared. The plan is
// file-only and snapshot/restore based, mirroring the shape of the OpenCode
// activation plan minus all launcher plumbing.
type piBackgroundProjectionPlan struct {
	path   string
	policy model.PiBackgroundIntent

	applied     bool
	changed     bool
	priorExists bool
	prior       []byte
	skipReason  string
}

// preparePiBackgroundProjection returns nil when there is nothing to project:
// no Pi component in the run, or an unresolved/auto effective policy.
func preparePiBackgroundProjection(homeDir string, resolution *PiBackgroundResolution, hasPi bool) *piBackgroundProjectionPlan {
	if resolution == nil || !hasPi || !resolution.managed || resolution.Effective == "" || resolution.Effective == model.PiBackgroundAuto {
		return nil
	}
	plan := &piBackgroundProjectionPlan{
		path:   piBackgroundPolicyPath(homeDir),
		policy: resolution.Effective,
	}
	resolution.projectionPlan = plan
	return plan
}

// Apply snapshots any existing managed file and writes the resolved policy.
// A pre-existing file is only replaced when it carries the managed schema
// marker; deactivation writes policy off instead of deleting user files.
func (p *piBackgroundProjectionPlan) Apply() error {
	desired, err := json.MarshalIndent(map[string]string{
		"schema": piBackgroundPolicySchema,
		"policy": string(p.policy),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal managed Pi background policy: %w", err)
	}
	desired = append(desired, '\n')

	existing, readErr := os.ReadFile(p.path)
	if readErr == nil {
		var decoded struct {
			Schema string `json:"schema"`
		}
		if json.Unmarshal(existing, &decoded) != nil || decoded.Schema != piBackgroundPolicySchema {
			// Non-fatal: a policy side-file must never roll back the apply stage.
			p.applied = true
			p.skipReason = fmt.Sprintf("existing file %q does not carry the managed schema %q; refusing to overwrite an unmanaged file", p.path, piBackgroundPolicySchema)
			return nil
		}
		p.prior, p.priorExists = existing, true
		if bytes.Equal(existing, desired) {
			p.applied = true
			return nil
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read managed Pi background policy %q: %w", p.path, readErr)
	}

	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("create managed Pi background policy directory: %w", err)
	}
	if _, err := filemerge.WriteFileAtomic(p.path, desired, 0o644); err != nil {
		return fmt.Errorf("write managed Pi background policy %q: %w", p.path, err)
	}
	p.applied, p.changed = true, true
	return nil
}

// Rollback restores the pre-Apply file bytes, or removes the file when this
// plan created it. A refused or convergent Apply leaves nothing to undo.
func (p *piBackgroundProjectionPlan) Rollback() error {
	if !p.applied || !p.changed {
		return nil
	}
	if !p.priorExists {
		if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove projected Pi background policy %q: %w", p.path, err)
		}
		return nil
	}
	if _, err := filemerge.WriteFileAtomic(p.path, p.prior, 0o644); err != nil {
		return fmt.Errorf("restore prior Pi background policy %q: %w", p.path, err)
	}
	return nil
}

// ChangedPaths reports the policy file when Apply actually rewrote it, so
// sync can surface the change alongside other managed assets.
func (p *piBackgroundProjectionPlan) ChangedPaths() []string {
	if p == nil || !p.changed {
		return nil
	}
	return []string{p.path}
}

// piBackgroundProjectionStep is the pipeline seam for the projection. It
// mirrors openCodeBackgroundActivationStep: Run applies the prepared plan and
// Rollback restores the snapshot.
type piBackgroundProjectionStep struct {
	id   string
	plan *piBackgroundProjectionPlan
}

func (s piBackgroundProjectionStep) ID() string { return s.id }

func (s piBackgroundProjectionStep) Run() error {
	if err := s.plan.Apply(); err != nil {
		return fmt.Errorf("apply managed Pi background policy projection: %w", err)
	}
	return nil
}

func (s piBackgroundProjectionStep) Rollback() error { return s.plan.Rollback() }
