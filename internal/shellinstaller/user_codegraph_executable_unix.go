//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Both launchers activate only an authenticated, selected Separate library.
func userCodeGraphLaunchEnv(root string, manifest userManifest, cmd *exec.Cmd) {
	if !manifest.CodeGraph || manifest.Mode != "separate" {
		return
	}
	if strings.ContainsRune(root, os.PathListSeparator) {
		cmd.Err = privateError("refused", errors.New("private CodeGraph launch root cannot contain a PATH separator"))
		return
	}
	for i, entry := range cmd.Env {
		if path, ok := strings.CutPrefix(entry, "PATH="); ok {
			cmd.Env[i] = "PATH=" + filepath.Join(root, "runtime/codegraph/bin") + ":" + path
		}
	}
	cmd.Env = append(cmd.Env, "CODEGRAPH_NO_DOWNLOAD=1", "CODEGRAPH_TELEMETRY=0", "DO_NOT_TRACK=1")
}

func userCodeGraphCommandWithNode(root, node string) string {
	node = strings.ReplaceAll(filepath.Join(root, node), "'", "'\\''")
	root = strings.ReplaceAll(root, "'", "'\\''")
	return "#!/bin/sh\nexec '" + node + "' '" + root + "/runtime/codegraph/node_modules/@colbymchenry/codegraph-linux-x64/lib/dist/bin/codegraph.js' \"$@\"\n"
}

func userStageCodeGraphExecutable(ctx context.Context, stage, destination string) error {
	return userStageCodeGraphExecutableWithNode(ctx, stage, destination, "runtime/node/bin/node")
}

func userStageCodeGraphExecutableWithNode(ctx context.Context, stage, destination, node string) error {
	if ctx == nil || !privateHierarchyPath(destination) {
		return privateError("refused", errors.New("CodeGraph command requires an absolute selected destination"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	base := filepath.Join(stage, "runtime/codegraph")
	identity, err := privateDirectory(base)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	if err := directory.Mkdir("bin", 0700); err != nil {
		return err
	}
	data := []byte(userCodeGraphCommandWithNode(destination, node))
	if err := userWriteWithSync("bin/codegraph", data, 0700, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
		return directory.OpenFile(path, flags, mode)
	}); err != nil {
		return err
	}
	path := filepath.Join(base, "bin/codegraph")
	if _, err := privateNativeFile(ctx, path, 0700, int64(len(data)), fmt.Sprintf("%x", sha256.Sum256(data))); err != nil {
		return err
	}
	after, err := privateDirectory(base)
	if err != nil || !os.SameFile(identity, after) {
		return privateError("preimage", err)
	}
	return nil
}
