//go:build windows

package shellinstaller

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// userWindowsReuseTree copies one owned installation subtree of the retired
// previous installation into the private stage for a component-scoped
// update. Physical directories and regular files only; the stage's inherited
// DACLs govern the copied bytes exactly like the unpacked archives. The
// target may pre-exist as one of the stage's just-created directories.
func userWindowsReuseTree(ctx context.Context, from, to string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if !filepath.IsAbs(from) || !filepath.IsAbs(to) {
		return privateError("refused", fmt.Errorf("component reuse requires absolute paths: %q -> %q", from, to))
	}
	info, err := os.Lstat(from)
	if err != nil || !info.Mode().IsDir() {
		return privateError("refused", fmt.Errorf("reusable component tree %q is absent or not a directory: %w", from, err))
	}
	if err := os.MkdirAll(to, 0700); err != nil {
		return fmt.Errorf("reuse target %q: %w", to, err)
	}
	return userWindowsReuseWalk(from, to)
}

func userWindowsReuseWalk(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return fmt.Errorf("reuse read %q: %w", from, err)
	}
	for _, entry := range entries {
		source, target := filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("reuse stat %q: %w", source, err)
		}
		switch kind := info.Mode().Type(); {
		case kind.IsDir():
			if err := os.MkdirAll(target, 0700); err != nil {
				return fmt.Errorf("reuse create %q: %w", target, err)
			}
			if err := userWindowsReuseWalk(source, target); err != nil {
				return err
			}
		case kind.IsRegular():
			if err := userWindowsReuseFile(source, target); err != nil {
				return err
			}
		default:
			return privateError("refused", fmt.Errorf("reusable tree contains non-regular entry %q: %#x", source, kind))
		}
	}
	return nil
}

func userWindowsReuseFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("reuse read %q: %w", from, err)
	}
	if err := os.WriteFile(to, data, infoMode(from)); err != nil {
		return fmt.Errorf("reuse write %q: %w", to, err)
	}
	return nil
}

func infoMode(from string) fs.FileMode {
	info, err := os.Lstat(from)
	if err != nil {
		return 0600
	}
	return info.Mode().Perm()
}

// userWindowsReuseComponents copies the out-of-scope component trees of the
// retired installation into the private stage before any acquisition runs.
// backup is the retired previous installation tree; goReuse adds the Go
// runtime to the Node reuse when its recorded pin matches this build.
func userWindowsReuseComponents(ctx context.Context, stage, backup string, installed *userWindowsManifest, req UserInstallRequest, scope userInstallScope, goReuse bool) error {
	if scope.reused[UserComponentNode] {
		if err := userWindowsReuseTree(ctx, filepath.Join(backup, "runtime", "node"), filepath.Join(stage, "runtime", "node")); err != nil {
			return fmt.Errorf("Node runtime reuse: %w", err)
		}
		if goReuse {
			if err := userWindowsReuseTree(ctx, filepath.Join(backup, "runtime", "go"), filepath.Join(stage, "runtime", "go")); err != nil {
				return fmt.Errorf("Go runtime reuse: %w", err)
			}
		}
	}
	if scope.reused[UserComponentCodeGraph] && req.Experience != nil && req.Experience.CodeGraph == "on" {
		if err := userWindowsReuseCodeGraphTree(ctx, filepath.Join(backup, "runtime", "codegraph"), filepath.Join(stage, "runtime", "codegraph")); err != nil {
			return fmt.Errorf("CodeGraph reuse: %w", err)
		}
	}
	if scope.reused[UserComponentSDK] {
		for _, name := range []string{"prefix", "agent", "source"} {
			if err := userWindowsReuseTree(ctx, filepath.Join(backup, name), filepath.Join(stage, name)); err != nil {
				return fmt.Errorf("SDK %s reuse: %w", name, err)
			}
		}
		if installed.MainArchiveSHA != "" {
			if _, statErr := os.Lstat(filepath.Join(backup, "stock-gentle-pi")); statErr == nil {
				if err := userWindowsReuseTree(ctx, filepath.Join(backup, "stock-gentle-pi"), filepath.Join(stage, "stock-gentle-pi")); err != nil {
					return fmt.Errorf("SDK stock package reuse: %w", err)
				}
			} else if !os.IsNotExist(statErr) {
				return fmt.Errorf("SDK stock package inspect: %w", statErr)
			}
			if err := userWindowsCopyFile(filepath.Join(backup, "main.json"), filepath.Join(stage, "main.json")); err != nil {
				return fmt.Errorf("SDK Main metadata reuse: %w", err)
			}
			if err := userWindowsCopyFile(filepath.Join(backup, "runtime", "archives", "main.zip"), filepath.Join(stage, "runtime", "archives", "main.zip")); err != nil {
				return fmt.Errorf("SDK Main archive reuse: %w", err)
			}
		}
	}
	return nil
}

func userWindowsCopyFile(from, to string) error {
	if _, err := os.Lstat(from); err != nil {
		return err
	}
	return userWindowsReuseFile(from, to)
}

// userWindowsReuseCodeGraphTree reuses the installed CodeGraph runtime into
// the stage without its generated command: userWindowsCodeGraphCommand
// regenerates bin\codegraph.cmd with an exclusive create, so a reused copy
// would fail the update with "file exists".
func userWindowsReuseCodeGraphTree(ctx context.Context, from, to string) error {
	if err := userWindowsReuseTree(ctx, from, to); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(to, "bin"))
}
