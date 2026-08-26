package sddstatus

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// NativeAttemptIdentity binds a repository and immutable input snapshots before authority opens.
type NativeAttemptIdentity struct {
	Root, RepositoryRef string
	lease               *reviewtransaction.RepositoryIdentityLease
	inputs              []nativeAttemptFileIdentity
}

type nativeAttemptFileIdentity struct {
	path     string
	info     fs.FileInfo
	mode     fs.FileMode
	size     int64
	mtime    int64
	digest   [sha256.Size]byte
	children []nativeAttemptFileIdentity
}

func ResolveNativeAttemptIdentity(ctx context.Context, repo string) (NativeAttemptIdentity, error) {
	lease, err := reviewtransaction.OpenRepositoryIdentityLease(ctx, repo)
	if err != nil {
		return NativeAttemptIdentity{}, errors.New("native attempt repository is unavailable")
	}
	value := lease.Identity()
	return NativeAttemptIdentity{Root: value.RepositoryRoot, RepositoryRef: value.RepositoryRef, lease: lease}, nil
}

func (identity NativeAttemptIdentity) Validate(ctx context.Context) error {
	if identity.lease == nil || identity.Root == "" || identity.RepositoryRef == "" || identity.lease.Validate(ctx) != nil {
		return errors.New("native attempt repository identity changed")
	}
	for _, input := range identity.inputs {
		live, err := captureNativeAttemptFileIdentity(identity.Root, input.path)
		if err != nil || !nativeAttemptSameFileIdentity(input, live) {
			return errors.New("native attempt input identity changed")
		}
	}
	return nil
}

func nativeAttemptInputIdentities(ctx context.Context, identity NativeAttemptIdentity, paths ...string) (NativeAttemptIdentity, error) {
	if identity.lease == nil || identity.Validate(ctx) != nil {
		return NativeAttemptIdentity{}, errors.New("native attempt repository identity changed")
	}
	identity.inputs = nil
	for _, path := range paths {
		input, err := captureNativeAttemptFileIdentity(identity.Root, path)
		if err != nil {
			return NativeAttemptIdentity{}, errors.New("native attempt input is invalid")
		}
		identity.inputs = append(identity.inputs, input)
	}
	return identity, nil
}

func captureNativeAttemptFileIdentity(root, path string) (nativeAttemptFileIdentity, error) {
	path, err := nativeAttemptCanonicalPath(root, path)
	if err != nil {
		return nativeAttemptFileIdentity{}, err
	}
	info, err := nativeAttemptLstat(root, path)
	if err != nil {
		return nativeAttemptFileIdentity{}, err
	}
	value := nativeAttemptFileIdentity{path: path, info: info, mode: info.Mode(), size: info.Size(), mtime: info.ModTime().UnixNano()}
	if info.IsDir() {
		entries, err := os.ReadDir(nativeAttemptPath(root, path))
		if err != nil {
			return nativeAttemptFileIdentity{}, err
		}
		for _, entry := range entries {
			child, err := captureNativeAttemptFileIdentity(root, filepath.Join(path, entry.Name()))
			if err != nil {
				return nativeAttemptFileIdentity{}, err
			}
			value.children = append(value.children, child)
		}
		return value, nil
	}
	if !info.Mode().IsRegular() {
		return nativeAttemptFileIdentity{}, errors.New("unsafe input type")
	}
	file, err := os.Open(nativeAttemptPath(root, path))
	if err != nil {
		return nativeAttemptFileIdentity{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nativeAttemptFileIdentity{}, errors.New("input changed while opening")
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return nativeAttemptFileIdentity{}, err
	}
	after, err := file.Stat()
	latest, latestErr := nativeAttemptLstat(root, path)
	if err != nil || latestErr != nil || !os.SameFile(info, after) || !os.SameFile(info, latest) || !nativeAttemptMetadataEqual(info, after) || !nativeAttemptMetadataEqual(info, latest) {
		return nativeAttemptFileIdentity{}, errors.New("input changed while reading")
	}
	copy(value.digest[:], hash.Sum(nil))
	return value, nil
}

func nativeAttemptCanonicalPath(root, path string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe input path")
	}
	if relative == ".git" || strings.HasPrefix(relative, ".git"+string(filepath.Separator)) {
		return "", errors.New("unsafe input path")
	}
	return relative, nil
}

func nativeAttemptLstat(root, path string) (fs.FileInfo, error) {
	current := filepath.VolumeName(nativeAttemptPath(root, path)) + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(nativeAttemptPath(root, path), current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("unsafe input path")
		}
	}
	return os.Lstat(current)
}

func nativeAttemptPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func nativeAttemptSameFileIdentity(left, right nativeAttemptFileIdentity) bool {
	if left.path != right.path || !os.SameFile(left.info, right.info) || left.mode != right.mode || left.size != right.size || left.mtime != right.mtime || left.digest != right.digest || len(left.children) != len(right.children) {
		return false
	}
	for index := range left.children {
		if !nativeAttemptSameFileIdentity(left.children[index], right.children[index]) {
			return false
		}
	}
	return true
}

func nativeAttemptMetadataEqual(left, right fs.FileInfo) bool {
	return left.Mode() == right.Mode() && left.Size() == right.Size() && left.ModTime().UnixNano() == right.ModTime().UnixNano()
}
