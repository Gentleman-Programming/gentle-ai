//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var userPNPMRoots = []string{"pnpm", "home", "agent", "config", "state", "data", "project"}

// PNPMCodeSeal covers the complete PNPM entry set, bytes, modes and link text.
// Compare against a trusted staging seal; a freshly computed seal is not provenance.
func PNPMCodeSeal(ctx context.Context, root string) (string, error) {
	entries, err := userPNPMPayloadInventory(ctx, root, []string{"pnpm"})
	if err != nil {
		return "", err
	}
	return userPNPMSeal(entries), nil
}

func userPNPMSeal(entries map[string]string) string {
	code := map[string]string{}
	for path, identity := range entries {
		if path == "pnpm" || strings.HasPrefix(path, "pnpm/") {
			code[path] = identity
		}
	}
	data, _ := json.Marshal(code)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func userPNPMPayloadInventory(ctx context.Context, root string, roots []string) (result map[string]string, resultErr error) {
	reader, err := newUserPNPMInventoryReader(root)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, reader.close()) }()
	entries := map[string]string{}
	for _, name := range roots {
		err := filepath.WalkDir(filepath.Join(root, name), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			owner := info.Sys().(*syscall.Stat_t)
			if owner.Uid != uint32(os.Getuid()) || owner.Gid != uint32(os.Getgid()) {
				return privateError("source", nil)
			}
			rel, _ := filepath.Rel(root, path)
			value := "directory"
			switch {
			case entry.IsDir():
				err = privateNativeDirectory(path)
			case info.Mode().IsRegular():
				var stamp string
				stamp, err = reader.file(ctx, path)
				if err == nil {
					value = "file:" + stamp[len(stamp)-64:]
				}
			case info.Mode()&os.ModeSymlink != 0:
				value, err = os.Readlink(path)
				value = "link:" + value
			default:
				err = privateError("source", nil)
			}
			entries[rel] = fmt.Sprintf("%o:%s", info.Mode().Perm(), value)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	if err := reader.verify(); err != nil {
		return nil, err
	}
	return entries, nil
}
