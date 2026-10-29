//go:build linux || darwin

package shellinstaller

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// userReuseTree copies one owned installation subtree into the private stage
// for a component-scoped update. The source is the recorded installation
// itself: physical directories, regular files and symlinks only, with modes
// preserved. The target must be absent or an empty physical directory (the
// stage preparation pre-creates some component directories); a non-empty or
// foreign target is refused and a partial reuse never publishes.
func userReuseTree(ctx context.Context, from, to string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if !privateHierarchyPath(from) || !privateHierarchyPath(to) {
		return privateError("refused", fmt.Errorf("component reuse requires owned absolute paths: %q -> %q", from, to))
	}
	if err := privateNativeDirectory(from); err != nil {
		return privateError("refused", fmt.Errorf("reusable component tree %q is absent or not owned: %w", from, err))
	}
	info, err := os.Lstat(to)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
			return fmt.Errorf("reuse parents %q: %w", to, err)
		}
	case err != nil:
		return fmt.Errorf("reuse target %q: %w", to, err)
	case !info.Mode().IsDir():
		return privateError("refused", fmt.Errorf("reuse target %q is not a physical directory", to))
	default:
		entries, readErr := os.ReadDir(to)
		if readErr != nil {
			return fmt.Errorf("reuse target %q: %w", to, readErr)
		}
		if len(entries) != 0 {
			return privateError("refused", fmt.Errorf("reuse target %q is not empty", to))
		}
	}
	return userReuseWalk(from, to)
}

func userReuseWalk(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return fmt.Errorf("reuse read %q: %w", from, err)
	}
	if err := os.Mkdir(to, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("reuse create %q: %w", to, err)
	}
	for _, entry := range entries {
		source, target := filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("reuse stat %q: %w", source, err)
		}
		switch kind := info.Mode().Type(); {
		case kind.IsDir():
			if err := userReuseWalk(source, target); err != nil {
				return err
			}
		case kind.IsRegular():
			if err := userReuseFile(source, target, info.Mode().Perm()); err != nil {
				return err
			}
		case kind&fs.ModeSymlink != 0:
			link, err := os.Readlink(source)
			if err != nil {
				return fmt.Errorf("reuse link %q: %w", source, err)
			}
			if err := os.Symlink(link, target); err != nil {
				return fmt.Errorf("reuse link %q: %w", target, err)
			}
		default:
			return privateError("refused", fmt.Errorf("reusable tree contains non-regular entry %q: %#x", source, kind))
		}
	}
	return nil
}

func userReuseFile(from, to string, mode fs.FileMode) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("reuse read %q: %w", from, err)
	}
	if err := os.WriteFile(to, data, mode); err != nil {
		return fmt.Errorf("reuse write %q: %w", to, err)
	}
	return os.Chmod(to, mode)
}

// userReuseComponents copies the out-of-scope component trees of the recorded
// installation into the private stage before any provisioning runs. The
// reused SDK prefix/agent (separate mode) carries the installed mcp.json, so
// the deterministic Context7 entry is restaged afterwards to honor a Context7
// choice that changed without re-provisioning the SDK.
func userReuseComponents(ctx context.Context, root string, req UserInstallRequest, existing *userManifest, scope userInstallScope) error {
	if scope.reused[UserComponentNode] {
		if err := userReuseTree(ctx, filepath.Join(req.Destination, "runtime/node"), filepath.Join(root, "runtime/node")); err != nil {
			return fmt.Errorf("Node runtime reuse: %w", err)
		}
	}
	if scope.reused[UserComponentCodeGraph] && req.Experience != nil && req.Experience.CodeGraph == "on" {
		if err := userReuseCodeGraphTree(ctx, filepath.Join(req.Destination, "runtime/codegraph"), filepath.Join(root, "runtime/codegraph")); err != nil {
			return fmt.Errorf("CodeGraph reuse: %w", err)
		}
	}
	if scope.reused[UserComponentSDK] && req.Mode == "separate" {
		if err := userReuseTree(ctx, existing.Prefix, filepath.Join(root, "prefix")); err != nil {
			return fmt.Errorf("SDK prefix reuse: %w", err)
		}
		if err := userReuseTree(ctx, existing.Agent, filepath.Join(root, "agent")); err != nil {
			return fmt.Errorf("SDK agent reuse: %w", err)
		}
		if err := userReuseSDKState(ctx, root, req); err != nil {
			return err
		}
		if req.Experience != nil && req.Experience.Context7 != "" {
			if err := userStageJSON(ctx, root, "agent", "mcp.json", userContext7Update(req.Experience.Context7)); err != nil {
				return fmt.Errorf("Context7 restage: %w", err)
			}
		}
	}
	if scope.reused[UserComponentSDK] && req.Mode == "agent" {
		if err := userReuseTree(ctx, existing.Prefix, filepath.Join(root, "prefix")); err != nil {
			return fmt.Errorf("SDK prefix reuse: %w", err)
		}
		if err := userReuseSDKState(ctx, root, req); err != nil {
			return err
		}
	}
	return nil
}

// userSDKReusePaths is every SDK-owned path of a recorded installation that
// the post-native readback re-reads when the SDK is reused: the stock
// acquisition and its prior cohort (provision-gentle-shell-private-global.mjs
// :77,:221,:224-225), both global-graph seals (:83,:222,:631), and the Main
// preview composition - the retained archive is re-read by Go's own Main
// custody readback (userReadMainArchive in user_main_custody_unix.go) and the
// source, composed package and runtime/node_modules dependency link by the
// provisioner's verify and Main actions (:590,:596,:347,:597,:665,:683-686).
// state/selection.json is regenerated fresh by every provisioning run and is
// never carried.
var userSDKReusePaths = []string{
	"runtime/project",
	"runtime/prior",
	"state/global-graph.json",
	"state/prior-global-graph.json",
	"runtime/main",
	"runtime/main-source",
	"runtime/main-sdk",
	"runtime/node_modules",
}

// userReuseStateFileBound matches the provisioner's own per-file read bound
// ("file byte bound" in provision-gentle-shell-private-global.mjs).
const userReuseStateFileBound = 33554432

// userReuseSDKState carries every path of userSDKReusePaths that exists in the
// recorded installation into the stage: directories through userReuseTree,
// regular files through a bounded owned copy that keeps their private modes,
// and symlinks recreated with the identical target and never followed. An
// unexpected entry type is refused exactly like an unexpected entry inside
// userReuseTree: a partial reuse never publishes. Installations recorded
// before a path existed have nothing to carry.
func userReuseSDKState(ctx context.Context, root string, req UserInstallRequest) error {
	for _, name := range userSDKReusePaths {
		from := filepath.Join(req.Destination, filepath.FromSlash(name))
		to := filepath.Join(root, filepath.FromSlash(name))
		if !privateHierarchyPath(from) || !privateHierarchyPath(to) {
			return privateError("refused", fmt.Errorf("SDK state reuse requires owned absolute paths: %q -> %q", from, to))
		}
		info, err := os.Lstat(from)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("SDK state reuse %s: %w", name, err)
		}
		switch kind := info.Mode().Type(); {
		case kind.IsDir():
			if err := userReuseTree(ctx, from, to); err != nil {
				return fmt.Errorf("SDK state reuse %s: %w", name, err)
			}
		case kind.IsRegular():
			if info.Size() > userReuseStateFileBound {
				return privateError("refused", fmt.Errorf("SDK state file %q exceeds the %d byte reuse bound", from, int64(userReuseStateFileBound)))
			}
			if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
				return fmt.Errorf("SDK state reuse %s: %w", name, err)
			}
			if err := userReuseFile(from, to, info.Mode().Perm()); err != nil {
				return fmt.Errorf("SDK state reuse %s: %w", name, err)
			}
		case kind&fs.ModeSymlink != 0:
			link, err := os.Readlink(from)
			if err != nil {
				return fmt.Errorf("SDK state reuse %s: %w", name, err)
			}
			if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
				return fmt.Errorf("SDK state reuse %s: %w", name, err)
			}
			if err := os.Symlink(link, to); err != nil {
				return fmt.Errorf("SDK state reuse %s: %w", name, err)
			}
		default:
			return privateError("refused", fmt.Errorf("SDK state %q contains a non-regular entry: %#x", from, kind))
		}
	}
	return nil
}

// userReuseCodeGraphTree reuses the installed CodeGraph runtime into the stage
// without its generated command: userStageCodeGraphExecutable regenerates
// bin/codegraph for the destination and creates bin exclusively, so a reused
// copy would fail the update with "mkdirat bin: file exists".
func userReuseCodeGraphTree(ctx context.Context, from, to string) error {
	if err := userReuseTree(ctx, from, to); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(to, "bin"))
}
