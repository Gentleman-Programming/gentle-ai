// Package statecoord serializes read-modify-write mutations of the shared install
// state. All such operations use one lock identity derived from the resolved
// home directory, so each mutation reads the latest state and writes it before
// releasing the shared lock.
package statecoord

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

// LockPath returns the canonical install-state lock path for homeDir.
func LockPath(homeDir string) (string, error) {
	resolvedHome, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return "", fmt.Errorf("resolve install state home: %w", err)
	}
	return state.Path(resolvedHome) + ".lock", nil
}

// WithLock runs operation while holding the canonical install-state lock shared
// by every install-state read-modify-write operation for the resolved home.
func WithLock(homeDir string, operation func() error) (err error) {
	lockPath, err := LockPath(homeDir)
	if err != nil {
		return fmt.Errorf("acquire install state lock: %w", err)
	}
	lock, err := reviewtransaction.AcquireAuthorityFileLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquire install state lock: %w", err)
	}
	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release install state lock: %w", releaseErr))
		}
	}()
	return operation()
}

// BeginExternalOperation records durable intent before the caller runs an external
// side effect. The caller owns that work after this function returns.
func BeginExternalOperation(homeDir string, intent state.ExternalOperation) error {
	if err := validateExternalOperation(intent); err != nil {
		return err
	}
	if intent.Phase != state.ExternalPhaseIntent {
		// refusal:by-design state-transition: external operations begin only with durable intent.
		return errors.New("external operation must begin in intent phase")
	}
	return WithLock(homeDir, func() error {
		current, err := readInstallState(homeDir, true)
		if err != nil {
			return err
		}
		current.ExternalOperations = state.MergeExternalOperation(current.ExternalOperations, intent)
		if err := state.WriteReconciled(homeDir, current); err != nil {
			return fmt.Errorf("persist external operation intent: %w", err)
		}
		return nil
	})
}

// AdvanceExternalOperation updates a pending operation's transition while retaining
// the ownership facts captured by BeginExternalOperation.
func AdvanceExternalOperation(homeDir string, transition state.ExternalOperation) error {
	if err := validateExternalOperation(transition); err != nil {
		return err
	}
	return WithLock(homeDir, func() error {
		current, err := readInstallState(homeDir, false)
		if err != nil {
			return err
		}
		existing, found := matchingExternalOperation(current.ExternalOperations, transition)
		if !found {
			// refusal:by-design state-transition: an operation can only advance after its durable intent is recorded.
			return errors.New("external operation is not pending; begin it before advancing")
		}
		if !canAdvanceExternalOperation(existing.Phase, transition.Phase) {
			// refusal:by-design state-transition: durable external operations may only move forward through known phases.
			return errors.New("external operation phase transition is invalid")
		}
		existing.Phase = transition.Phase
		existing.Continuation = transition.Continuation
		current.ExternalOperations = state.MergeExternalOperation(current.ExternalOperations, existing)
		if err := state.WriteReconciled(homeDir, current); err != nil {
			return fmt.Errorf("persist external operation transition: %w", err)
		}
		return nil
	})
}

// SettleExternalOperation atomically applies an optional provenance mutation and
// clears the matching durable operation record.
func SettleExternalOperation(homeDir string, operation state.ExternalOperation, mutate func(*state.InstallState) error) error {
	if err := validateExternalOperation(operation); err != nil {
		return err
	}
	return WithLock(homeDir, func() error {
		current, err := readInstallState(homeDir, false)
		if err != nil {
			return err
		}
		if _, found := matchingExternalOperation(current.ExternalOperations, operation); !found {
			// refusal:by-design state-transition: provenance may only settle an operation whose ownership facts remain durable.
			return errors.New("external operation is not pending; begin it before settling")
		}
		if mutate != nil {
			if err := mutate(&current); err != nil {
				return err
			}
		}
		current.ExternalOperations = state.RemoveExternalOperation(current.ExternalOperations, operation)
		if err := state.WriteReconciled(homeDir, current); err != nil {
			return fmt.Errorf("persist settled external operation: %w", err)
		}
		return nil
	})
}

// ClearExternalOperation settles an operation without changing provenance.
func ClearExternalOperation(homeDir string, operation state.ExternalOperation) error {
	return SettleExternalOperation(homeDir, operation, nil)
}

func canAdvanceExternalOperation(from, to state.ExternalPhase) bool {
	switch from {
	case state.ExternalPhaseIntent:
		return to == state.ExternalPhaseIntent || to == state.ExternalPhaseApplied || to == state.ExternalPhaseManual
	case state.ExternalPhaseApplied:
		return to == state.ExternalPhaseApplied || to == state.ExternalPhaseManual
	case state.ExternalPhaseManual:
		return to == state.ExternalPhaseManual
	default:
		return false
	}
}

func validateExternalOperation(operation state.ExternalOperation) error {
	if operation.Tool == "" {
		// refusal:by-design input-validation: durable records need a tool identity for recovery.
		return errors.New("external operation tool is required")
	}
	if operation.Action == "" {
		// refusal:by-design input-validation: durable records need an action identity for recovery.
		return errors.New("external operation action is required")
	}
	if operation.Phase == "" {
		// refusal:by-design input-validation: durable records need a transition phase for recovery.
		return errors.New("external operation phase is required")
	}
	return nil
}

func readInstallState(homeDir string, allowAbsent bool) (state.InstallState, error) {
	current, err := state.Read(homeDir)
	if err == nil {
		return current, nil
	}
	if allowAbsent && errors.Is(err, os.ErrNotExist) {
		return state.InstallState{}, nil
	}
	return state.InstallState{}, fmt.Errorf("read install state: %w", err)
}

func matchingExternalOperation(operations []state.ExternalOperation, operation state.ExternalOperation) (state.ExternalOperation, bool) {
	for _, existing := range operations {
		if len(state.RemoveExternalOperation([]state.ExternalOperation{existing}, operation)) == 0 {
			return existing, true
		}
	}
	return state.ExternalOperation{}, false
}
