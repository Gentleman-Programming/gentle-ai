//go:build linux || darwin

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Materialize only unpublished source beside retained custody. This never
// replaces the stock prefix, dependency graph or authenticated native runtime.
func userStageMainSource(ctx context.Context, root string, artifact userSnapshotArtifact) (resultErr error) {
	data, err := userReadMainArchive(ctx, root, artifact)
	if err != nil {
		return err
	}
	files, err := userMainSourceFiles(ctx, data, artifact)
	if err != nil {
		return err
	}
	runtime := filepath.Join(root, "runtime")
	identity, err := privateDirectory(runtime)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(runtime)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	if err := directory.Mkdir("main-source", 0700); err != nil {
		return err
	}
	sourceIdentity, err := privateDirectory(filepath.Join(runtime, "main-source"))
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // Parents precede their children.
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		file := files[name]
		if file == nil || file.FileInfo().IsDir() {
			if err := directory.Mkdir("main-source/"+name, 0700); err != nil {
				return err
			}
			continue
		}
		content, err := userMainMemberBytes(ctx, file)
		if err != nil {
			return err
		}
		if err := userStageConfigFiles(ctx, root, "runtime/main-source", []userExperienceStageFile{{name, content}}); err != nil {
			return err
		}
	}
	fresh, err := privateDirectory(runtime)
	if err != nil || !os.SameFile(identity, fresh) {
		return privateError("preimage", err)
	}
	sourceFresh, err := privateDirectory(filepath.Join(runtime, "main-source"))
	if err != nil || !os.SameFile(sourceIdentity, sourceFresh) {
		return privateError("preimage", err)
	}
	return userCompareMainSource(ctx, root, artifact)
}

func userMainMemberBytes(ctx context.Context, file *zip.File) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stream, err := file.Open()
	if err != nil {
		return nil, err
	}
	content, readErr := io.ReadAll(io.LimitReader(stream, int64(file.UncompressedSize64)+1))
	if err := errors.Join(readErr, stream.Close(), ctx.Err()); err != nil || uint64(len(content)) != file.UncompressedSize64 {
		return nil, errors.Join(err, errors.New("Main member bytes differ"))
	}
	return content, nil
}

// Read-only comparison is still source custody, not API compatibility or
// permission to activate it over the existing authenticated distribution.
func userCompareMainSource(ctx context.Context, root string, artifact userSnapshotArtifact) error {
	data, err := userReadMainArchive(ctx, root, artifact)
	if err != nil {
		return err
	}
	files, err := userMainSourceFiles(ctx, data, artifact)
	if err != nil {
		return err
	}
	base := filepath.Join(root, "runtime/main-source")
	identity, err := privateDirectory(base)
	if err != nil {
		return err
	}
	err = filepath.WalkDir(base, func(path string, entry os.DirEntry, cause error) error {
		if cause != nil || ctx.Err() != nil {
			return errors.Join(cause, ctx.Err())
		}
		relative, err := filepath.Rel(base, path)
		if err != nil || relative == "." {
			return err
		}
		name := filepath.ToSlash(relative)
		file, exists := files[name]
		if !exists || entry.IsDir() != (file == nil || file.FileInfo().IsDir()) {
			return errors.New("Main source exact set/type differs")
		}
		delete(files, name)
		if entry.IsDir() {
			_, err := privateDirectory(path)
			return err
		}
		expected, err := userMainMemberBytes(ctx, file)
		if err != nil {
			return err
		}
		actual, err := userRetainedSource(ctx, path, int64(len(expected)))
		if err != nil || !bytes.Equal(actual, expected) {
			return errors.Join(err, errors.New("Main source bytes differ"))
		}
		return nil
	})
	if err != nil || len(files) != 0 {
		return errors.Join(err, errors.New("Main source missing/extra members"))
	}
	fresh, err := privateDirectory(base)
	if err != nil || !os.SameFile(identity, fresh) {
		return privateError("preimage", err)
	}
	return ctx.Err()
}
