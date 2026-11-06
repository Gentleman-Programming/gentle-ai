//go:build linux || darwin

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// The launch path fully hashes two pinned binaries on every start: the
// supervisor itself and the bundled node runtime. Both are large (tens to
// hundreds of megabytes), so the hash dominates cold starts and costs hundreds
// of milliseconds even warm. This cache records, per pinned file, the
// privateStamp observed when the bytes were last hashed against their pin.
//
// A stamp match still refuses tampered bytes: privateStamp binds
// dev:ino:uid:gid:mode:size:mtime:ctime, and ctime cannot be forged from
// userspace. Any content rewrite changes ctime, misses the cache, and forces
// the full privateNativeFile hash again. A changed pin (update flow) also
// misses, because the recorded pin is part of the entry.
const userPinCacheSchema = "gentle-shell-launch-pin-cache/v1"

type userPinCacheEntry struct {
	Stamp string `json:"stamp"`
	Pin   string `json:"pin"`
}

type userPinCache struct {
	Schema  string                       `json:"schema"`
	Entries map[string]userPinCacheEntry `json:"entries"`
}

// userPinCacheMu serializes cache writers within one process; concurrent
// supervisor launches still race across processes, where the last atomic
// rename wins and both records are valid.
var userPinCacheMu sync.Mutex

// userPinnedFileCached verifies path against pin like privateNativeFile does,
// first through the recorded stamp (fast) and otherwise through the full
// hash, which then refreshes the record. The accepted metadata (owner, mode,
// size bound, canonical physical path) matches privateNativeFile exactly.
func userPinnedFileCached(ctx context.Context, root, relative string, mode os.FileMode, pin string) (string, error) {
	path := filepath.Join(root, relative)
	info, err := privatePhysical(path)
	if err != nil || ctx.Err() != nil {
		return "", privateError("source", errors.Join(err, ctx.Err()))
	}
	if info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || (mode != 0 && info.Mode() != mode) || info.Size() > 268435456 {
		return "", privateError("source", fmt.Errorf("native file refused: %s metadata=%v", path, info))
	}
	stamp := privateStamp(info)
	if entry, ok := userReadPinCacheEntry(root, relative); ok && entry.Pin == pin && entry.Stamp == stamp {
		return stamp + ":cached", nil
	}
	digest, err := privateNativeFile(ctx, path, mode, -1, pin)
	if err != nil {
		return "", err
	}
	userWritePinCacheEntry(root, relative, userPinCacheEntry{Stamp: stamp, Pin: pin})
	return digest, nil
}

func userPinCachePath(root string) string {
	return filepath.Join(root, "state", "launch-pin-cache.json")
}

func userReadPinCacheEntry(root, relative string) (userPinCacheEntry, bool) {
	data, err := os.ReadFile(userPinCachePath(root))
	if err != nil || len(data) > 1<<20 {
		return userPinCacheEntry{}, false
	}
	var cache userPinCache
	if json.Unmarshal(data, &cache) != nil || cache.Schema != userPinCacheSchema || cache.Entries == nil {
		return userPinCacheEntry{}, false
	}
	entry, ok := cache.Entries[relative]
	return entry, ok
}

func userWritePinCacheEntry(root, relative string, entry userPinCacheEntry) {
	userPinCacheMu.Lock()
	defer userPinCacheMu.Unlock()
	cache := userPinCache{Schema: userPinCacheSchema, Entries: map[string]userPinCacheEntry{}}
	if data, err := os.ReadFile(userPinCachePath(root)); err == nil && len(data) <= 1<<20 {
		var parsed userPinCache
		if json.Unmarshal(data, &parsed) == nil && parsed.Schema == userPinCacheSchema && parsed.Entries != nil {
			cache.Entries = parsed.Entries
		}
	}
	cache.Entries[relative] = entry
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	if _, err := privateDirectory(filepath.Join(root, "state")); err != nil {
		return
	}
	if err := userAtomicWrite(userPinCachePath(root), data, 0600); err != nil {
		return
	}
}

// userAtomicWrite replaces path with bytes through a temp file, an fsync, a
// rename, and a directory fsync, so readers never observe a partial record.
func userAtomicWrite(path string, bytes []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".pin-cache-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer func() {
		_ = os.Remove(name)
	}()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(bytes); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
