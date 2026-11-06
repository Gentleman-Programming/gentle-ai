//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// userCompareMainSource re-reads the retained Main archive (13MB) and every
// retained source byte, twice per verify action, on every launch of a Main SDK
// installation. This cache records the privateStamp of the archive, its
// descriptor, and every retained source entry observed when a full compare
// succeeded. A stamp match refuses tampered bytes exactly like the launch pin
// cache: ctime cannot be forged from userspace, so any content rewrite misses
// the cache and forces the full compare again. A different artifact (update
// flow) misses because the artifact identity is part of the record.
const userMainSourceCacheSchema = "gentle-shell-main-source-compare/v1"

type userMainSourceCache struct {
	Schema      string                     `json:"schema"`
	ArtifactSHA string                     `json:"artifactSHA"`
	Prefix      string                     `json:"prefix"`
	Descriptor  string                     `json:"descriptor"`
	Archive     string                     `json:"archive"`
	SourceRoot  string                     `json:"sourceRoot"`
	Entries     map[string]userStampRecord `json:"entries"`
}

type userStampRecord struct {
	Stamp string `json:"stamp"`
	Dir   bool   `json:"dir,omitempty"`
}

// userCompareMainSourceCached proves the retained Main source still matches
// the retained archive, first through recorded stamps and otherwise through
// the full byte compare, which then refreshes the record. The accepted
// metadata matches userCompareMainSource exactly.
func userCompareMainSourceCached(ctx context.Context, root string, artifact userSnapshotArtifact) error {
	base := filepath.Join(root, "runtime/main-source")
	archivePath := filepath.Join(root, "runtime/main/main.zip")
	descriptorPath := filepath.Join(root, "runtime/main/main.json")
	descriptorInfo, err := privatePhysical(descriptorPath)
	if err != nil || ctx.Err() != nil {
		return privateError("source", errors.Join(err, ctx.Err()))
	}
	archiveInfo, err := privatePhysical(archivePath)
	if err != nil || ctx.Err() != nil {
		return privateError("source", errors.Join(err, ctx.Err()))
	}
	sourceRootInfo, err := privateDirectory(base)
	if err != nil || ctx.Err() != nil {
		return err
	}
	if cached, ok := userReadMainSourceCache(root); ok &&
		cached.ArtifactSHA == artifact.SHA &&
		cached.Prefix == artifact.Prefix &&
		cached.Descriptor == privateStamp(descriptorInfo) &&
		cached.Archive == privateStamp(archiveInfo) &&
		cached.SourceRoot == privateStamp(sourceRootInfo) {
		observed, walkErr := userMainSourceStamps(ctx, base)
		if walkErr == nil && userStampsEqual(cached.Entries, observed) {
			return ctx.Err()
		}
	}
	if err := userCompareMainSource(ctx, root, artifact); err != nil {
		return err
	}
	userWriteMainSourceCache(root, userMainSourceCache{
		Schema:      userMainSourceCacheSchema,
		ArtifactSHA: artifact.SHA,
		Prefix:      artifact.Prefix,
		Descriptor:  privateStamp(descriptorInfo),
		Archive:     privateStamp(archiveInfo),
		SourceRoot:  privateStamp(sourceRootInfo),
		Entries:     userMustMainSourceStamps(ctx, base),
	})
	return ctx.Err()
}

// userMainSourceStamps records one stamp per retained source entry. A symlink
// or special file records an empty stamp with the observed kind, which never
// matches the recorded set and always forces the full compare.
func userMainSourceStamps(ctx context.Context, base string) (map[string]userStampRecord, error) {
	entries := map[string]userStampRecord{}
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, cause error) error {
		if cause != nil || ctx.Err() != nil {
			return errors.Join(cause, ctx.Err())
		}
		relative, err := filepath.Rel(base, path)
		if err != nil || relative == "." {
			return err
		}
		name := filepath.ToSlash(relative)
		if entry.IsDir() {
			info, err := privateDirectory(path)
			if err != nil {
				return err
			}
			entries[name] = userStampRecord{Stamp: privateStamp(info), Dir: true}
			return nil
		}
		info, err := privatePhysical(path)
		if err != nil {
			entries[name] = userStampRecord{}
			return nil
		}
		entries[name] = userStampRecord{Stamp: privateStamp(info)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func userMustMainSourceStamps(ctx context.Context, base string) map[string]userStampRecord {
	entries, err := userMainSourceStamps(ctx, base)
	if err != nil {
		return map[string]userStampRecord{}
	}
	return entries
}

func userStampsEqual(recorded, observed map[string]userStampRecord) bool {
	if len(recorded) != len(observed) {
		return false
	}
	for name, want := range recorded {
		got, ok := observed[name]
		if !ok || want != got {
			return false
		}
		if want.Dir && !got.Dir {
			return false
		}
	}
	return true
}

func userMainSourceCachePath(root string) string {
	return filepath.Join(root, "state", "main-source-compare-cache.json")
}

func userReadMainSourceCache(root string) (userMainSourceCache, bool) {
	data, err := os.ReadFile(userMainSourceCachePath(root))
	if err != nil || len(data) > 8<<20 {
		return userMainSourceCache{}, false
	}
	var cache userMainSourceCache
	if json.Unmarshal(data, &cache) != nil || cache.Schema != userMainSourceCacheSchema || cache.Entries == nil {
		return userMainSourceCache{}, false
	}
	return cache, true
}

func userWriteMainSourceCache(root string, cache userMainSourceCache) {
	userPinCacheMu.Lock()
	defer userPinCacheMu.Unlock()
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		data = append(data, '\n')
	}
	if _, err := privateDirectory(filepath.Join(root, "state")); err != nil {
		return
	}
	_ = userAtomicWrite(userMainSourceCachePath(root), data, 0600)
}
