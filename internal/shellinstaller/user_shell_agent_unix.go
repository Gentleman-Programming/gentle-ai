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
	"strings"
	"syscall"
)

// An operator-owned Pi agent may be group-readable but must never be writable
// by another user. The prefix is not reused in this mode, so Homebrew/system
// prefixes stay untouched. The darwin ACL and file-flag guard privateDirectory
// applies to selected roots applies here too.
func userOwnedIdentity(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) {
		return "", privateError("filesystem", err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return "", privateError("filesystem", err)
	}
	if err := privateExtendedMetadata(path, info, false); err != nil {
		return "", privateError("filesystem", err)
	}
	st := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%d:%d:%d:%v", st.Dev, st.Ino, st.Uid, info.Mode()), nil
}

// userAgentPreflight checks, before the first write to the operator's Pi
// agent, every precondition a fresh agent-reuse install would otherwise
// refuse on after it had already changed the agent. Each refusal names the
// path and the exact fix.
func userAgentPreflight(agent, destination string, profile *UserExperience) error {
	refuse := func(format string, args ...any) error {
		return privateError("refused", fmt.Errorf(format, args...))
	}
	if _, err := userOwnedIdentity(agent); err != nil {
		return privateError("refused", userAgentCustodyProblem(agent, err))
	}
	if profile != nil && profile.SDKSource == "main-preview" {
		// The Main SDK selection binds the agent as a private 0700 directory.
		if info, err := os.Lstat(agent); err != nil || info.Mode().Perm() != 0700 {
			return refuse("Main preview on your existing Pi needs the agent %q to be private: run chmod 700 %q, or choose Stable or Separate", agent, agent)
		}
	}
	// Pi's own regular fd/rg are set aside at install, not refused.
	if _, err := userAgentToolsAdmit(agent); err != nil {
		return err
	}
	settingsPath := filepath.Join(agent, "settings.json")
	data, err := userAgentPreflightFile(settingsPath)
	if err != nil {
		return err
	}
	if data != nil {
		settings := map[string]json.RawMessage{}
		if json.Unmarshal(data, &settings) != nil || settings == nil {
			return refuse("%q is not a JSON object; fix or move it, then retry", settingsPath)
		}
		if _, present := settings["npmCommand"]; present {
			return refuse("%q already sets npmCommand (left by an earlier Gentle Shell attempt or another tool); remove the npmCommand key, then retry", settingsPath)
		}
		if raw, present := settings["packages"]; present {
			var packages []json.RawMessage
			if json.Unmarshal(raw, &packages) != nil || packages == nil {
				return refuse("%q packages must be a JSON array; fix it, then retry", settingsPath)
			}
			packageRoot := filepath.Join(destination, "prefix/lib/node_modules/gentle-pi")
			for _, entry := range packages {
				var source string
				if strings.TrimSpace(string(entry)) == "null" || json.Unmarshal(entry, &source) != nil {
					var filtered struct {
						Source *string `json:"source"`
					}
					if json.Unmarshal(entry, &filtered) != nil || filtered.Source == nil {
						return refuse("%q has a packages entry without a source string (%s); fix or remove it, then retry", settingsPath, entry)
					}
					source = *filtered.Source
				}
				if strings.HasPrefix(source, "npm:gentle-pi") || source == packageRoot {
					return refuse("%q already declares gentle-pi (%s); remove that packages entry, then retry", settingsPath, source)
				}
			}
		}
	}
	if profile != nil && profile.Context7 != "" {
		mcpPath := filepath.Join(agent, "mcp.json")
		if _, err := os.Lstat(mcpPath + ".new"); !os.IsNotExist(err) {
			return refuse("%q is left over from an earlier attempt; remove it, then retry", mcpPath+".new")
		}
		data, err := userAgentPreflightFile(mcpPath)
		if err != nil {
			return err
		}
		if data != nil {
			fields := map[string]json.RawMessage{}
			if json.Unmarshal(data, &fields) != nil || fields == nil {
				return refuse("%q is not a JSON object; fix or move it, then retry", mcpPath)
			}
			// Dry-run the exact merge the install applies later.
			if err := userContext7Update(profile.Context7)(fields); err != nil {
				return refuse("%q cannot take the Context7 entry (%v); fix it, then retry", mcpPath, err)
			}
		}
	}
	return nil
}

// userAgentCustodyProblem names why userOwnedIdentity refused the agent and
// the fix for that cause; mode advice is given only for mode problems.
func userAgentCustodyProblem(agent string, cause error) error {
	info, err := os.Lstat(agent)
	switch {
	case err != nil:
		return fmt.Errorf("the Pi agent %q cannot be inspected (%v); run Pi once to create it, or choose Separate", agent, err)
	case info.Mode()&os.ModeSymlink != 0:
		real, _ := filepath.EvalSymlinks(agent)
		return fmt.Errorf("the Pi agent %q is a symbolic link; select its real path %q instead, or choose Separate", agent, real)
	case !info.IsDir():
		return fmt.Errorf("the Pi agent %q is not a directory; run Pi once to create its agent directory, or choose Separate", agent)
	case info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()):
		return fmt.Errorf("the Pi agent %q is owned by another user; take ownership of it, or choose Separate", agent)
	case info.Mode().Perm()&0022 != 0:
		return fmt.Errorf("the Pi agent %q is writable by other users; run chmod go-w %q, then retry", agent, agent)
	}
	if real, err := filepath.EvalSymlinks(agent); err == nil && real != agent {
		return fmt.Errorf("the Pi agent %q is reached through a symbolic link; select its real path %q instead, or choose Separate", agent, real)
	}
	if metadata := privateExtendedMetadata(agent, info, false); metadata != nil {
		if strings.HasPrefix(metadata.Error(), "BSD file flags refused") {
			return fmt.Errorf("the Pi agent %q has BSD file flags such as immutable or append-only (see ls -lOd %q); clear them with chflags nouchg,nouappnd %q, then retry: %w", agent, agent, agent, metadata)
		}
		return fmt.Errorf("the Pi agent %q has an ACL that grants access beyond its mode bits (see ls -led %q); remove it with chmod -N %q, then retry: %w", agent, agent, agent, metadata)
	}
	return fmt.Errorf("the Pi agent %q must be a physical directory you own that no other user can write, or choose Separate: %w", agent, cause)
}

// userAgentPreflightFile admits an absent file or a bounded regular file the
// operator owns, with one link, that no other user can write: the custody
// the settings and mcp.json preimages require.
func userAgentPreflightFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	refuse := func(problem string) error {
		return privateError("refused", errors.Join(fmt.Errorf("%q %s, then retry", path, problem), err))
	}
	switch {
	case err != nil:
		return nil, refuse("cannot be inspected; fix its access")
	case !info.Mode().IsRegular():
		return nil, refuse("must be a regular file (not a link or directory); replace it with a regular file")
	case info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()):
		return nil, refuse("is owned by another user; take ownership of it")
	case info.Mode().Perm()&0022 != 0:
		return nil, refuse(fmt.Sprintf("is writable by other users; run chmod go-w %q", path))
	case info.Sys().(*syscall.Stat_t).Nlink != 1:
		return nil, refuse("has other hard links; replace it with a single copy")
	case info.Size() > 1<<20:
		return nil, refuse("is larger than 1 MiB; shrink or move it")
	}
	data, err := os.ReadFile(path)
	if err != nil || int64(len(data)) != info.Size() {
		return nil, refuse("changed while it was read; retry when it is stable")
	}
	return data, nil
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
	// An absent mcp.json merges into an empty object, but its preimage is
	// recorded as absent so uninstall removes the file again.
	original, existed := []byte("{}\n"), os.FileInfo(nil)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Nlink != 1 || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()) || info.Size() > 1<<20 {
			return privateError("preimage", nil)
		}
		original, err = os.ReadFile(path)
		if err != nil {
			return err
		}
		existed = info
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
	if existed != nil {
		backup := filepath.Join(stage, "state/agent.preimage")
		if err := os.MkdirAll(backup, 0700); err != nil {
			return err
		}
		if err := userToolWrite(filepath.Join(backup, "mcp.json"), original, 0600); err != nil {
			return err
		}
	}
	if err := userRecordAgentPreimage(stage, "mcp.json", existed); err != nil {
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
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			files[name] = userAgentPostFile{}
			continue
		}
		// The touched set includes the pinned multi-megabyte fd/rg, so the
		// bound is the selection inventory's per-file bound, not 1 MiB.
		if err != nil || !info.Mode().IsRegular() || info.Size() > userAgentImageBound {
			return privateError("preimage", err)
		}
		data, err := os.ReadFile(path)
		if err != nil || int64(len(data)) != info.Size() {
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

// userRestoreAgentImages restores the agent preimages and returns the
// set-aside Pi tools whose backup is kept because the agent bin changed.
func userRestoreAgentImages(target, agent string) ([]string, error) {
	if _, err := userOwnedIdentity(agent); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(target, "state/agent.post.json"))
	var post struct {
		Schema string                       `json:"schema"`
		Files  map[string]userAgentPostFile `json:"files"`
	}
	if err != nil || json.Unmarshal(data, &post) != nil || post.Schema != userAgentPostSchema {
		return nil, errors.New("agent uninstall evidence is missing; reinstall this agent install with the current version first")
	}
	// Pi's own fd/rg set aside at install follow their own plan, decided and
	// verified here before any write.
	tools, err := userPlanAgentToolsRestore(target, agent, post.Files)
	if err != nil {
		return nil, err
	}
	// settings.json and mcp.json come back with their recorded presence and
	// mode; installs without that record restore owner-only, never wider.
	preimages, err := userReadAgentPreimageRecord(target)
	if err != nil {
		return nil, err
	}
	recorded := func(name string) (userAgentPreimageFile, bool) {
		if preimages == nil {
			return userAgentPreimageFile{}, false
		}
		entry, ok := preimages.Files[name]
		return entry, ok
	}
	if preimages != nil {
		for name, entry := range preimages.Files {
			if _, err := os.Lstat(filepath.Join(target, "state/agent.preimage", name)); entry.Exists != (err == nil) {
				return nil, fmt.Errorf("the agent preimage of %q does not match its evidence; uninstall aborted without touching the agent", name)
			}
		}
	}
	for _, name := range userAgentTouchFiles {
		evidence, ok := post.Files[name]
		if !ok {
			return nil, errors.New("agent uninstall evidence is incomplete")
		}
		if tools.skips(name) {
			continue
		}
		path := filepath.Join(agent, filepath.FromSlash(name))
		current, readErr := os.ReadFile(path)
		if evidence.Exists {
			if readErr != nil || fmt.Sprintf("%x", sha256.Sum256(current)) != evidence.SHA256 {
				return nil, errors.New("the Pi agent changed after installation; uninstall aborted without touching it")
			}
		} else if !os.IsNotExist(readErr) {
			return nil, errors.New("the Pi agent changed after installation; uninstall aborted without touching it")
		}
	}
	for _, name := range userAgentTouchFiles {
		if tools.skips(name) {
			continue
		}
		evidence := post.Files[name]
		path := filepath.Join(agent, filepath.FromSlash(name))
		entry, hasEntry := recorded(name)
		preimage, preimageErr := os.ReadFile(filepath.Join(target, "state/agent.preimage", name))
		switch {
		case hasEntry && !entry.Exists:
			if err := userAgentRemoveRegular(path); err != nil {
				return nil, err
			}
		case preimageErr == nil:
			mode := userAgentPreimageLegacy
			if hasEntry {
				mode, _ = userAgentPreimageMode(entry)
			}
			if err := userAgentReplaceFile(path, preimage, mode); err != nil {
				return nil, err
			}
		case evidence.Exists && os.IsNotExist(preimageErr):
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		case !os.IsNotExist(preimageErr):
			return nil, preimageErr
		}
	}
	return tools.apply()
}

// userSnapshotOwnedAgentSettings retains the operator agent's settings inside
// the target before the legacy provisioning writes its stock declaration, and
// records its presence and mode for uninstall.
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
		return userRecordAgentPreimage(root, "settings.json", nil)
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
	if err := userToolWrite(filepath.Join(backup, "settings.json"), data, 0600); err != nil {
		return err
	}
	return userRecordAgentPreimage(root, "settings.json", info)
}
