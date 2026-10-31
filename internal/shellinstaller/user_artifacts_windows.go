//go:build windows

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Preserve the Windows artifact/retained JSON ABI while sharing the fixed
// canonical Main producer. ZIP/custody/provision/activation stay platform-owned.
type userWindowsArtifact = userSnapshotArtifact

func userWindowsMainArtifact(commit, archiveSHA string) (userWindowsArtifact, error) {
	return userMainDescriptor(commit, archiveSHA)
}

func userWindowsLowerHex(value string, length int) bool {
	return userSnapshotLowerHex(value, length)
}

func userWindowsMainSnapshot(ctx context.Context) (userWindowsArtifact, []byte, error) {
	return userMainSnapshot(ctx)
}

func userWindowsMainFiles(ctx context.Context, data []byte, artifact userWindowsArtifact) (map[string]*zip.File, error) {
	return userMainSourceFiles(ctx, data, artifact)
}

func userWindowsCompareMain(ctx context.Context, data []byte, artifact userWindowsArtifact, destination string) error {
	files, err := userWindowsMainFiles(ctx, data, artifact)
	if err != nil {
		return err
	}
	err = filepath.WalkDir(destination, func(path string, entry os.DirEntry, cause error) error {
		if cause != nil || ctx.Err() != nil {
			return errors.Join(cause, ctx.Err())
		}
		relative, err := filepath.Rel(destination, path)
		if err != nil || relative == "." {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, err := userWindowsIdentity(path, true); err != nil {
			return err
		}
		if (relative == "node_modules" || relative == ".gentle-ai") && entry.IsDir() {
			return filepath.SkipDir // Verified separately against npm and SumDB custody.
		}
		file, exists := files[relative]
		if !exists || entry.IsDir() != (file == nil || file.FileInfo().IsDir()) {
			return errors.New("Main source exact set/type differs")
		}
		delete(files, relative)
		if entry.IsDir() {
			return nil
		}
		stream, err := file.Open()
		if err != nil {
			return err
		}
		original, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		closeErr := stream.Close()
		actual, actualErr := userWindowsRead(path, int64(file.UncompressedSize64))
		if cause := errors.Join(readErr, closeErr, actualErr); cause != nil || !bytes.Equal(original, actual) {
			return errors.Join(cause, errors.New("Main source bytes differ from retained archive"))
		}
		return nil
	})
	if err != nil || len(files) != 0 {
		return errors.Join(err, errors.New("Main source missing/extra members"))
	}
	return nil
}

func userWindowsMainRead(root string, manifest userWindowsManifest) (userWindowsArtifact, error) {
	artifact, err := userWindowsMainArtifact(manifest.MainCommit, manifest.MainArchiveSHA)
	if err != nil {
		return userWindowsArtifact{}, err
	}
	data, err := userWindowsRead(filepath.Join(root, "main.json"), 4096)
	canonical, _ := json.Marshal(artifact)
	if err != nil || string(data) != string(canonical) {
		return userWindowsArtifact{}, errors.Join(err, errors.New("retained Main descriptor differs"))
	}
	return artifact, nil
}

var userWindowsArtifacts = []userWindowsArtifact{
	{"https://nodejs.org/dist/v24.18.0/node-v24.18.0-win-x64.zip", "0ae68406b42d7725661da979b1403ec9926da205c6770827f33aac9d8f26e821", "node.zip", "node-v24.18.0-win-x64", 64 << 20, 0},
	{"https://go.dev/dl/go1.27.1.windows-amd64.zip", "a3911b5e0e1b1053f25ed0675f4c1c6aad1e2bfcf253df2b9be4caabd2edd95d", "go.zip", "go", 128 << 20, 78931360},
	{"https://github.com/sharkdp/fd/releases/download/v10.5.0/fd-v10.5.0-x86_64-pc-windows-msvc.zip", "a227701b8551c35a9931d9f6da75503cf86d88e182d71fb849a70864c5d57cd7", "fd.zip", "fd-v10.5.0-x86_64-pc-windows-msvc", 32 << 20, 1535063},
	{"https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/ripgrep-15.2.0-x86_64-pc-windows-msvc.zip", "71b2fef860abe467217a538ff31de02f5258807c0129f771846f87bd029aafc5", "rg.zip", "ripgrep-15.2.0-x86_64-pc-windows-msvc", 32 << 20, 1789611},
}

// userWindowsDownloadCacheDirectory resolves, creates and owns the private
// per-user download cache directory below the real LOCALAPPDATA, which is
// resolved from the user token even when the sanitized worker environment
// omits LOCALAPPDATA. Only the fixed supplier archives in
// userWindowsArtifacts are cached, keyed by their exact archive name; the
// Main snapshot descriptor and every mutable/user artifact are acquired
// through their own path and are never routed through userWindowsAcquire.
// Every failure — an unresolvable LOCALAPPDATA, a creation error or a
// foreign/aliased cache identity — is a bypass, never an install failure:
// caching is a pure optimization.
func userWindowsDownloadCacheDirectory() (string, bool) {
	local, err := userWindowsLocalAppData()
	if err != nil {
		return "", false
	}
	cache := filepath.Join(local, "gentle-shell", "cache", "downloads")
	if _, err := os.Lstat(cache); err != nil {
		if !errors.Is(err, os.ErrNotExist) || os.MkdirAll(cache, 0700) != nil {
			return "", false
		}
	}
	if err := userWindowsPrivate(cache); err != nil {
		return "", false // Foreign, aliased or otherwise uncertain cache custody.
	}
	return cache, true
}

// userWindowsDownloadCacheHit returns cached archive bytes only when the
// exact-name entry is owned (no reparse point, current-SID owner, private
// DACL, NTFS, canonical path), stays within the artifact byte bound and
// matches the artifact descriptor's exact size and SHA-256. Anything else is
// a miss; unverified cache bytes are never returned.
func userWindowsDownloadCacheHit(ctx context.Context, artifact userWindowsArtifact) ([]byte, bool) {
	if ctx.Err() != nil {
		return nil, false
	}
	cache, ok := userWindowsDownloadCacheDirectory()
	if !ok {
		return nil, false
	}
	data, err := userWindowsRead(filepath.Join(cache, artifact.Archive), artifact.Bound)
	if err != nil ||
		(artifact.Size != 0 && int64(len(data)) != artifact.Size) ||
		userWindowsSHA(data) != artifact.SHA {
		return nil, false
	}
	return data, true
}

// userWindowsDownloadCacheStore best-effort retains already verified supplier
// bytes under their exact archive name. Any failure is dropped: a partial or
// unverified entry is removed rather than left behind, and an entry another
// concurrent install already stored is kept as found (it is re-verified
// before any later read).
func userWindowsDownloadCacheStore(artifact userWindowsArtifact, data []byte) {
	cache, ok := userWindowsDownloadCacheDirectory()
	if !ok {
		return
	}
	entry := filepath.Join(cache, artifact.Archive)
	if err := userWindowsWrite(entry, data); err != nil && !errors.Is(err, os.ErrExist) {
		_ = os.Remove(entry)
	}
}

func userWindowsAcquire(ctx context.Context, artifact userWindowsArtifact, target string) ([]byte, error) {
	if data, hit := userWindowsDownloadCacheHit(ctx, artifact); hit {
		if err := userWindowsWrite(target, data); err != nil {
			return nil, err
		}
		return data, nil
	}
	data, err := userWindowsAcquireNetwork(ctx, artifact)
	if err != nil {
		return nil, err // A failed download never writes the cache.
	}
	if err := userWindowsWrite(target, data); err != nil {
		return nil, err
	}
	userWindowsDownloadCacheStore(artifact, data)
	return data, nil
}

func userWindowsAcquireNetwork(ctx context.Context, artifact userWindowsArtifact) ([]byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second}
	defer transport.CloseIdleConnections()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errors.New("Windows supplier redirect refused")
		}
		host := req.URL.Hostname()
		if host != "nodejs.org" && host != "dl.google.com" && host != "release-assets.githubusercontent.com" {
			return errors.New("Windows supplier redirect host refused")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > artifact.Bound {
		return nil, errors.New("Windows supplier status/byte bound refused")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, artifact.Bound+1))
	if err != nil || int64(len(data)) > artifact.Bound || (artifact.Size != 0 && int64(len(data)) != artifact.Size) || userWindowsSHA(data) != artifact.SHA {
		return nil, errors.Join(err, errors.New("Windows supplier digest/size differs"))
	}
	return data, nil
}

// Parsing/extraction is reached only inside the qualified Windows worker.
// ZIP names, link modes, duplicates and aggregate inflation are checked before
// publishing any extracted object. Archives remain retained for later readback.
func userWindowsZIP(ctx context.Context, data []byte, artifact userWindowsArtifact, destination, member string) ([]byte, error) {
	reader, err := userSnapshotZIP(ctx, data, artifact)
	if err != nil {
		return nil, err
	}
	var selected []byte
	for _, file := range reader.File {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if file.FileInfo().IsDir() || (member != "" && file.Name != member) {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
		closeErr := stream.Close()
		if err := errors.Join(readErr, closeErr); err != nil || uint64(len(content)) != file.UncompressedSize64 {
			return nil, errors.Join(err, errors.New("Windows archive physical member differs"))
		}
		if member != "" {
			selected = content
			continue
		}
		relative := strings.TrimPrefix(file.Name, artifact.Prefix+"/")
		if relative == file.Name {
			return nil, errors.New("Windows archive root is not a regular member")
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		if err := userWindowsWrite(target, content); err != nil {
			return nil, err
		}
	}
	if member != "" && selected == nil {
		return nil, fmt.Errorf("Windows archive member missing: %s", member)
	}
	return selected, nil
}
