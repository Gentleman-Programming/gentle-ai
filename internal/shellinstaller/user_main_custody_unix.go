//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Retain only in an unpublished private runtime. The caller owns cleanup;
// this does not apply source, provision dependencies or enable a Main selector.
func userStageMainArchive(ctx context.Context, root string, artifact userSnapshotArtifact, data []byte) (resultErr error) {
	if _, err := userMainSourceFiles(ctx, data, artifact); err != nil {
		return err
	}
	if !privateHierarchyPath(root) {
		return privateError("refused", errors.New("Main requires an absolute private stage"))
	}
	identities := map[string]os.FileInfo{}
	runtime := filepath.Join(root, "runtime")
	for _, name := range []string{root, runtime} {
		info, err := privateDirectory(name)
		if err != nil {
			return err
		}
		identities[name] = info
	}
	directory, err := os.OpenRoot(runtime)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identities[runtime], opened) {
		return privateError("preimage", err)
	}
	if err := directory.Mkdir("main", 0700); err != nil {
		return err
	}
	descriptor, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	files := []userExperienceStageFile{{"main.json", descriptor}, {"main.zip", data}}
	if err := userStageConfigFiles(ctx, root, "runtime/main", files); err != nil {
		return err
	}
	for name, before := range identities {
		after, err := privateDirectory(name)
		if err != nil || !os.SameFile(before, after) {
			return privateError("preimage", err)
		}
	}
	_, err = userReadMainArchive(ctx, root, artifact)
	return err
}

// The expected descriptor remains caller-bound, never derived from the mutable
// retained JSON. Reading is not publisher authentication or source compatibility.
func userReadMainArchive(ctx context.Context, root string, artifact userSnapshotArtifact) ([]byte, error) {
	if ctx == nil {
		return nil, privateError("canceled", context.Canceled)
	}
	if err := ctx.Err(); err != nil {
		return nil, privateError("canceled", err)
	}
	canonical, err := userMainDescriptor(strings.TrimPrefix(artifact.Prefix, "gentle-shell-"), artifact.SHA)
	if err != nil || canonical != artifact {
		return nil, errors.Join(err, errors.New("Main retained descriptor authority differs"))
	}
	base := filepath.Join(root, "runtime/main")
	identities := map[string]os.FileInfo{}
	for _, name := range []string{root, filepath.Join(root, "runtime"), base} {
		info, err := privateDirectory(name)
		if err != nil {
			return nil, err
		}
		identities[name] = info
	}
	entries, err := privateNativeEntries(base)
	if err != nil || len(entries) != 2 {
		return nil, errors.Join(err, errors.New("Main retained layout differs"))
	}
	expected, err := json.Marshal(artifact)
	if err != nil {
		return nil, err
	}
	descriptor, err := userRetainedSource(ctx, filepath.Join(base, "main.json"), int64(len(expected)))
	if err != nil || !bytes.Equal(descriptor, expected) {
		return nil, errors.Join(err, errors.New("Main retained descriptor differs"))
	}
	archive := filepath.Join(base, "main.zip")
	info, err := privatePhysical(archive)
	if err != nil {
		return nil, err
	}
	data, err := userRetainedSource(ctx, archive, info.Size())
	if err != nil {
		return nil, err
	}
	if _, err := userMainSourceFiles(ctx, data, artifact); err != nil {
		return nil, err
	}
	for name, before := range identities {
		after, err := privateDirectory(name)
		if err != nil || !os.SameFile(before, after) {
			return nil, privateError("preimage", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
