//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// userStageContext7 writes an internal, unpublished Separate stage only.
// Public profile admission stays closed; generic writers keep their guards.
// The caller owns cleanup; this neither discovers Shared paths nor reads persona.
func userStageContext7(ctx context.Context, root, mode, choice string) error {
	if choice == "" {
		return nil
	}
	if choice != "on" && choice != "off" {
		return experienceError("Context7 must be on, off or absent")
	}
	if mode != "separate" {
		return experienceError("explicit Context7 requires a private Separate agent")
	}
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if !privateHierarchyPath(root) {
		return experienceError("canonical MCP stage requires an absolute private root")
	}
	rootIdentity, err := privateDirectory(root)
	if err != nil {
		return err
	}
	agent := filepath.Join(root, "agent")
	identity, err := privateDirectory(agent)
	if err != nil {
		return err
	}
	directory, err := os.OpenRoot(agent)
	if err != nil {
		return err
	}
	defer directory.Close()
	opened, err := directory.Stat(".")
	if err != nil || !os.SameFile(identity, opened) {
		return privateError("preimage", err)
	}
	contents, err := directory.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := contents.ReadDir(1)
	closeErr := contents.Close()
	if len(entries) != 0 || !errors.Is(readErr, io.EOF) || closeErr != nil {
		return privateError("preimage", errors.Join(readErr, closeErr, errors.New("canonical MCP stage requires an empty agent directory")))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data := []byte(fmt.Sprintf("{\n  \"mcpServers\": {\n    \"context7\": {\n      \"url\": \"https://mcp.context7.com/mcp\",\n      \"enabled\": %t,\n      \"exposure\": \"codemode\"\n    }\n  }\n}\n", choice == "on"))
	if err := userWriteWithSync("mcp.json", data, 0600, func(path string, flags int, mode os.FileMode) (userSyncedFile, error) {
		return directory.OpenFile(path, flags, mode)
	}); err != nil {
		return err
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(data))
	if _, err := privateNativeFile(ctx, filepath.Join(agent, "mcp.json"), 0600, int64(len(data)), pin); err != nil {
		return err
	}
	for path, before := range map[string]os.FileInfo{root: rootIdentity, agent: identity} {
		fresh, err := privateDirectory(path)
		if err != nil || !os.SameFile(before, fresh) {
			return privateError("preimage", err)
		}
	}
	return nil
}
