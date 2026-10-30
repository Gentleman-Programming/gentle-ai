//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// NativePNPMStage seals immutable code, not mutable agent sessions or preferences.
type NativePNPMStage struct {
	Installation NativePNPMInstallation
	CodeSeal     string
}

// StageNativePNPMInstallation copies exclusively held original-installer DATA.
// It never executes suppliers or publishes destination. Absolute generated links
// target destination and require physical readback after the caller's rename.
func StageNativePNPMInstallation(ctx context.Context, source, stage, destination string, location NativeInstallationLocation) (result NativePNPMStage, err error) {
	if !privateHierarchyPath(stage) || !privateHierarchyPath(destination) || stage == destination || filepath.Dir(stage) != filepath.Dir(destination) || strings.ContainsAny(destination, "\"$`\\") || strings.HasPrefix(stage, source+"/") || strings.HasPrefix(destination, source+"/") {
		return result, privateError("destination", nil)
	}
	if _, err = privateDirectory(filepath.Dir(stage)); err != nil {
		return result, err
	}
	if err = privateDestination(destination); err != nil {
		return result, err
	}
	if _, err = os.Lstat(stage); !os.IsNotExist(err) {
		return result, privateError("destination", err)
	}
	result.Installation, err = ReadNativePNPMInstallation(ctx, source, location)
	if err != nil {
		return result, err
	}
	before, err := userPNPMPayloadInventory(ctx, source, userPNPMRoots)
	if err != nil {
		return result, err
	}
	if err = os.Mkdir(stage, 0700); err != nil {
		return result, err
	}
	flushes := newUserPNPMFileSyncBatch()
	defer func() {
		err = errors.Join(err, flushes.finish())
		if err != nil {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	reader, err := newUserPNPMInventoryReader(stage)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, reader.close()) }()
	copyFlush := flushes.submit
	if reader != nil {
		// Copy first so durable synchronization can amortize storage latency.
		copyFlush = func(file userPNPMSyncFile) error { return file.Close() }
	}
	expected := make(map[string]string, len(before))
	for _, root := range userPNPMRoots {
		err = filepath.WalkDir(filepath.Join(source, root), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			rel, _ := filepath.Rel(source, path)
			// Acquisition caches are not runtime payload. Preserve the virtual-store links.
			// Full source inventories still include it and must remain unchanged.
			if userPNPMAcquisitionCache(rel) {
				if !entry.IsDir() {
					return privateError("source", nil)
				}
				return filepath.SkipDir
			}
			out := filepath.Join(stage, rel)
			info, err := entry.Info()
			if err != nil {
				return err
			}
			expected[rel] = before[rel]
			switch {
			case entry.IsDir():
				if err := os.Mkdir(out, info.Mode().Perm()); err != nil {
					return err
				}
				return os.Chmod(out, info.Mode().Perm())
			case info.Mode()&os.ModeSymlink != 0:
				target, err := os.Readlink(path)
				physical, resolveErr := filepath.EvalSymlinks(path)
				boundary := source + "/" + root + "/"
				if err != nil || resolveErr != nil || !strings.HasPrefix(physical, boundary) {
					return privateError("source", errors.Join(err, resolveErr))
				}
				physicalRel, err := filepath.Rel(source, physical)
				if err != nil || userPNPMAcquisitionCache(physicalRel) {
					return privateError("source", err)
				}
				if filepath.IsAbs(target) {
					if root != "pnpm" || filepath.Base(path) != "node" || !strings.HasPrefix(target, source+"/pnpm/") {
						return privateError("source", nil)
					}
					target = destination + strings.TrimPrefix(target, source)
				}
				expected[rel] = fmt.Sprintf("%o:link:%s", info.Mode().Perm(), target)
				return os.Symlink(target, out)
			case info.Mode().IsRegular():
				data, changed, err := userPNPMRelocation(ctx, path, rel, source, destination)
				if err != nil {
					return err
				}
				if changed {
					expected[rel] = fmt.Sprintf("%o:file:%x", info.Mode().Perm(), sha256.Sum256(data))
					if err := userToolWrite(out, data, info.Mode().Perm()); err != nil {
						return err
					}
					return os.Chmod(out, info.Mode().Perm())
				}
				return userPNPMCopyFileWithSync(ctx, path, out, info, root == "pnpm", source, copyFlush)
			default:
				return privateError("source", nil)
			}
		})
		if err != nil {
			return result, err
		}
	}
	after, err := userPNPMInventoryWhileSyncing(ctx, func(ctx context.Context) (map[string]string, error) {
		return userPNPMPayloadInventory(ctx, stage, userPNPMRoots)
	}, func(ctx context.Context) error {
		// Always drain accepted files, including on walk failure or cancellation.
		return errors.Join(userPNPMSyncTree(ctx, reader, flushes.submit), flushes.finish())
	})
	if err != nil {
		return result, err
	}
	if !maps.Equal(after, expected) {
		return result, privateError("readback", nil)
	}
	fresh, err := userPNPMPayloadInventory(ctx, source, userPNPMRoots)
	if err != nil {
		return result, err
	}
	if !maps.Equal(before, fresh) {
		return result, privateError("preimage", nil)
	}
	err = filepath.WalkDir(stage, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return userDirectorySync(path)
		}
		return nil
	})
	return NativePNPMStage{result.Installation, userPNPMSeal(after)}, err
}

func userPNPMRelocation(ctx context.Context, path, rel, source, destination string) ([]byte, bool, error) {
	shim := strings.HasPrefix(rel, "pnpm/bin/") || strings.Contains(rel, "/node_modules/.bin/")
	modules := strings.HasSuffix(rel, "/node_modules/.modules.yaml")
	if !shim && !modules && rel != "home/.bashrc" && rel != "config/user.npmrc" {
		return nil, false, nil
	}
	data, _, err := userPNPMSmallFile(ctx, path)
	if err != nil {
		return nil, false, err
	}
	if !bytes.Contains(data, []byte(source+"/")) {
		return nil, false, nil
	}
	old, replacement := source+"/pnpm/", destination+"/pnpm/"
	switch {
	case shim:
		if !bytes.HasPrefix(data, []byte("#!/bin/sh\n")) || !bytes.Contains(data, []byte("\n# cmd-shim-target="+old)) {
			return nil, false, privateError("source", nil)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, source+"/") && !strings.Contains(line, "NODE_PATH=") && !strings.HasPrefix(line, "# cmd-shim-target=") {
				return nil, false, privateError("source", nil)
			}
		}
	case modules:
		var metadata struct {
			StoreDir       string
			PackageManager string
		}
		if json.Unmarshal(data, &metadata) != nil || metadata.StoreDir != source+"/pnpm/store/v11" || metadata.PackageManager != "pnpm@11.1.1" {
			return nil, false, privateError("source", nil)
		}
		old, replacement = `"`+source+`/pnpm/store/v11"`, `"`+destination+`/pnpm/store/v11"`
	case rel == "home/.bashrc":
		old, replacement = `export PNPM_HOME="`+source+`/pnpm"`, `export PNPM_HOME="`+destination+`/pnpm"`
	case rel == "config/user.npmrc":
		old, replacement = "prefix="+source+"/pnpm\n", "prefix="+destination+"/pnpm\n"
	}
	if !strings.Contains(string(data), old) {
		return nil, false, privateError("source", nil)
	}
	data = bytes.ReplaceAll(data, []byte(old), []byte(replacement))
	if bytes.Contains(data, []byte(source+"/")) {
		return nil, false, privateError("source", nil)
	}
	return data, true, nil
}

func userPNPMCopyFileWithSync(ctx context.Context, source, target string, info os.FileInfo, code bool, oldRoot string, flush func(userPNPMSyncFile) error) (resultErr error) {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, in.Close()) }()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return errors.Join(err, out.Close())
	}
	buffer, tail, total := make([]byte, 65536), "", int64(0)
	for err == nil {
		var n int
		n, err = in.Read(buffer)
		total += int64(n)
		if total > info.Size() || ctx.Err() != nil {
			err = privateError("source", ctx.Err())
			break
		}
		if code {
			text := tail + string(buffer[:n])
			if strings.Contains(text, oldRoot+"/pnpm") || strings.Contains(text, oldRoot+"/home") {
				err = privateError("source", nil)
				break
			}
			keep := len(oldRoot) + 6
			if len(text) > keep {
				tail = text[len(text)-keep:]
			} else {
				tail = text
			}
		}
		if _, writeErr := out.Write(buffer[:n]); writeErr != nil {
			err = writeErr
		}
	}
	if errors.Is(err, io.EOF) && total == info.Size() {
		err = nil
	}
	return errors.Join(err, flush(out))
}

// Omit only producer acquisition caches. PNPM links/ holds executable packages:
// preserve that virtual store, its metadata, and every other payload path.
func userPNPMAcquisitionCache(rel string) bool {
	return rel == "home/.npm" || strings.HasPrefix(rel, "home/.npm/") ||
		rel == "pnpm/store/v11/files" || strings.HasPrefix(rel, "pnpm/store/v11/files/")
}
