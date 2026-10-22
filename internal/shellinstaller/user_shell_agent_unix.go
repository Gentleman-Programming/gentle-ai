//go:build linux || darwin

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// An operator-owned Pi agent may be group-readable but must never be writable
// by another user. The prefix is not reused in this mode, so Homebrew/system
// prefixes stay untouched.
func userOwnedIdentity(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return "", privateError("filesystem", err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return "", privateError("filesystem", err)
	}
	st := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%d:%d:%d:%v", st.Dev, st.Ino, st.Uid, info.Mode()), nil
}

// userStageOwnedAgentContext7 merges the selected Context7 entry into the
// existing Pi agent, after writing the untouched preimage into the published
// target's recovery evidence. Only the selected fields are replaced.
func userStageOwnedAgentContext7(ctx context.Context, agent, choice, stage string) error {
	if choice == "" {
		return nil
	}
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	before, err := userOwnedIdentity(agent)
	if err != nil {
		return err
	}
	path := filepath.Join(agent, "mcp.json")
	original := []byte("{}\n")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || info.Size() > 1<<20 {
			return privateError("preimage", nil)
		}
		original, err = os.ReadFile(path)
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(original, &fields); err != nil {
		return errors.New("agent mcp.json is not a JSON object")
	}
	if err := userContext7Update(choice)(fields); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	backup := filepath.Join(stage, "state/agent.preimage")
	if err := os.MkdirAll(backup, 0700); err != nil {
		return err
	}
	if err := userToolWrite(filepath.Join(backup, "mcp.json"), original, 0600); err != nil {
		return err
	}
	temporary := path + ".new"
	if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
		return privateError("preimage", nil)
	}
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	directory, err := os.Open(agent)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		directory.Close()
		return err
	}
	if err := directory.Close(); err != nil {
		return err
	}
	after, err := userOwnedIdentity(agent)
	if err != nil || after != before {
		return privateError("preimage", err)
	}
	return nil
}

type userAgentPostFile struct {
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
}

const userAgentPostSchema = "gentle-shell-agent-post/v1"

var userAgentTouchFiles = []string{"settings.json", "mcp.json", "bin/fd", "bin/rg"}

// userCaptureAgentPostImages records the exact external-agent state after all
// managed writes. Uninstall restores preimages only when every file still
// matches this evidence.
func userCaptureAgentPostImages(agent, root string) error {
	files := map[string]userAgentPostFile{}
	for _, name := range userAgentTouchFiles {
		path := filepath.Join(agent, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			files[name] = userAgentPostFile{}
			continue
		}
		if err != nil || len(data) > 1<<20 {
			return privateError("preimage", err)
		}
		files[name] = userAgentPostFile{Exists: true, SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
	}
	data, err := json.Marshal(struct {
		Schema string                       `json:"schema"`
		Files  map[string]userAgentPostFile `json:"files"`
	}{userAgentPostSchema, files})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
		return err
	}
	return userToolWrite(filepath.Join(root, "state/agent.post.json"), data, 0600)
}

func userRestoreAgentImages(target, agent string) error {
	if _, err := userOwnedIdentity(agent); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(target, "state/agent.post.json"))
	var post struct {
		Schema string                       `json:"schema"`
		Files  map[string]userAgentPostFile `json:"files"`
	}
	if err != nil || json.Unmarshal(data, &post) != nil || post.Schema != userAgentPostSchema {
		return errors.New("agent uninstall evidence is missing; reinstall this agent install with the current version first")
	}
	for _, name := range userAgentTouchFiles {
		evidence, ok := post.Files[name]
		if !ok {
			return errors.New("agent uninstall evidence is incomplete")
		}
		path := filepath.Join(agent, filepath.FromSlash(name))
		current, readErr := os.ReadFile(path)
		if evidence.Exists {
			if readErr != nil || fmt.Sprintf("%x", sha256.Sum256(current)) != evidence.SHA256 {
				return errors.New("the Pi agent changed after installation; uninstall aborted without touching it")
			}
		} else if !os.IsNotExist(readErr) {
			return errors.New("the Pi agent changed after installation; uninstall aborted without touching it")
		}
	}
	for _, name := range userAgentTouchFiles {
		evidence := post.Files[name]
		path := filepath.Join(agent, filepath.FromSlash(name))
		preimage, preimageErr := os.ReadFile(filepath.Join(target, "state/agent.preimage", name))
		switch {
		case preimageErr == nil:
			if err := atomicPreferenceWrite(path, preimage); err != nil {
				return err
			}
		case evidence.Exists && os.IsNotExist(preimageErr):
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		case !os.IsNotExist(preimageErr):
			return preimageErr
		}
	}
	return nil
}

// userSnapshotOwnedAgentSettings retains the operator agent's settings inside
// the target before the legacy provisioning writes its stock declaration.
func userSnapshotOwnedAgentSettings(ctx context.Context, agent, root string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	if _, err := userOwnedIdentity(agent); err != nil {
		return err
	}
	path := filepath.Join(agent, "settings.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || len(data) > 1<<20 {
		return privateError("preimage", err)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return privateError("preimage", err)
	}
	backup := filepath.Join(root, "state/agent.preimage")
	if err := os.MkdirAll(backup, 0700); err != nil {
		return err
	}
	return userToolWrite(filepath.Join(backup, "settings.json"), data, 0600)
}
