package engram

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/filecoord"
)

// antigravityLockTarget returns the stable coordination target for one config
// home: the CLI plugin directory, deliberately independent of the desktop/CLI
// variant that stat selects for the MCP config path. Desktop and CLI users of
// one physical home therefore share a single lock key instead of splitting a
// shared resource across two locks.
func antigravityLockTarget(configHome string) (string, error) {
	canonical, err := canonicalAntigravityConfigHome(configHome)
	if err != nil {
		return "", err
	}
	return filepath.Join(canonical, ".gemini", "antigravity-cli", "plugins", "gentle-ai-engram"), nil
}

// antigravityLockRoot returns the coordination lock root for one canonical
// config home. It holds only coordination metadata; managed Antigravity files
// never live here.
func antigravityLockRoot(configHome string) (string, error) {
	canonical, err := canonicalAntigravityConfigHome(configHome)
	if err != nil {
		return "", err
	}
	return filepath.Join(canonical, ".gentle-ai", "locks"), nil
}

// canonicalAntigravityConfigHome canonicalizes configHome through symlinks so
// that aliases of the same physical home resolve to one lock key, while
// distinct physical paths (workspace vs global scope) stay distinct. When the
// leaf does not exist yet, the deepest existing ancestor is resolved and the
// remaining relative suffix is re-appended (the statecoord pattern); symlinked
// intermediate components are resolved as part of that ancestor.
func canonicalAntigravityConfigHome(configHome string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(configHome); err == nil {
		return resolved, nil
	}
	ancestor, err := filepath.Abs(configHome)
	if err != nil {
		return "", fmt.Errorf("resolve Antigravity coordination home %q: %w", configHome, err)
	}
	var suffix []string
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("resolve Antigravity coordination home %q: no existing ancestor", configHome)
		}
		suffix = append([]string{filepath.Base(ancestor)}, suffix...)
		ancestor = parent
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			return filepath.Join(append([]string{resolved}, suffix...)...), nil
		}
	}
}

// acquireAntigravityCoordinationLock takes the cooperative single-writer lock
// guarding every managed Antigravity mutation for one physical config home.
// One non-blocking attempt on a background context: contention returns a busy
// error wrapping filecoord.ErrBusy with retry advice, and the caller owns
// pacing. The lock is cooperative — it serializes gentle-ai writers, never
// arbitrary external editors — and its lease must be released by the caller,
// which joins release failures into the operation's error.
func acquireAntigravityCoordinationLock(configHomeDir string) (*filecoord.Lease, error) {
	target, err := antigravityLockTarget(configHomeDir)
	if err != nil {
		return nil, err
	}
	root, err := antigravityLockRoot(configHomeDir)
	if err != nil {
		return nil, err
	}
	lease, err := filecoord.Acquire(context.Background(), target, root)
	if err != nil {
		if errors.Is(err, filecoord.ErrBusy) {
			return nil, fmt.Errorf("another gentle-ai writer already holds the Antigravity coordination lock for %q; no Antigravity files were modified — retry once that run completes: %w", target, err)
		}
		return nil, fmt.Errorf("acquire Antigravity coordination lock: %w", err)
	}
	return lease, nil
}
