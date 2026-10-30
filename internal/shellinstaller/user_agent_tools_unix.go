//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// A stock Pi agent downloads its own fd and rg into AGENT/bin. An agent-reuse
// install sets exactly those regular files aside inside the installation,
// binds the pinned tools in their place and puts Pi's copies back on
// uninstall. A failure before the publication commit restores them through
// the agent rollback image. Shared keeps its refusal: it has no rollback image
// and no uninstall that could put them back.
const (
	userAgentToolsSchema   = "gentle-shell-agent-tools/v1"
	userAgentToolsBackup   = "backup/agent-tools"
	userAgentToolsEvidence = "state/agent-tools.json"
	// A tool uninstall cannot restore is kept beside the uninstall backup.
	userAgentToolsKeptSuffix   = ".pi-tools"
	userAgentToolsKeptEvidence = "agent-tools.json"
)

type userAgentToolImage struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
}

type userAgentToolsRecord struct {
	Schema string                        `json:"schema"`
	Agent  string                        `json:"agent"`
	Files  map[string]userAgentToolImage `json:"files"`
}

// userAgentToolsAdmit is the agent target's AGENT/bin admission: absent, or
// an owned physical 0700/0755 directory whose fd and rg, when present, are
// regular files the install can set aside and restore exactly. It returns
// the present tool names and never writes.
func userAgentToolsAdmit(agent string) ([]string, error) {
	bin := filepath.Join(agent, "bin")
	if _, err := os.Lstat(bin); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, privateError("refused", fmt.Errorf("inspect agent bin %q: %w", bin, err))
	}
	if err := privateNativeDirectory(bin); err != nil {
		return nil, privateError("refused", fmt.Errorf("agent bin %q must be absent or an owned physical 0700/0755 directory: %w", bin, err))
	}
	var present []string
	for _, source := range userToolSources {
		path := filepath.Join(bin, source.name)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := userAgentToolCustody(path, info, err); err != nil {
			return nil, err
		}
		present = append(present, source.name)
	}
	return present, nil
}

// userAgentToolCustody admits one of Pi's own tools for the set-aside: a
// single-link regular file the operator owns, that no other user can write,
// within the agent image bound. Every refusal names the path and the fix.
func userAgentToolCustody(path string, info os.FileInfo, err error) error {
	refuse := func(problem, fix string) error {
		return privateError("refused", errors.Join(fmt.Errorf("agent tool %q %s; Gentle Shell sets aside only Pi's own regular fd/rg files: %s, then retry", path, problem, fix), err))
	}
	switch {
	case err != nil:
		return refuse("cannot be inspected", "fix its access")
	case info.Mode()&os.ModeSymlink != 0:
		return refuse("is a symbolic link, not a regular file", "move it out of that directory")
	case !info.Mode().IsRegular():
		return refuse("is not a regular file", "move it out of that directory")
	case info.Sys().(*syscall.Stat_t).Uid != uint32(os.Getuid()):
		return refuse("is owned by another user", "take ownership of it or move it out of that directory")
	case info.Mode().Perm()&0022 != 0:
		return refuse("is writable by other users", fmt.Sprintf("run chmod go-w %q", path))
	case info.Sys().(*syscall.Stat_t).Nlink != 1:
		return refuse("has other hard links", "replace it with a single copy")
	case info.Size() > userAgentImageBound:
		return refuse("is larger than 32 MiB", "move it out of that directory")
	}
	if metadata := privateExtendedMetadata(path, info, false); metadata != nil {
		return refuse("has an ACL or BSD file flags", "clear them (chmod -N, chflags nouchg,nouappnd)")
	}
	return nil
}

// userAgentToolRead reads one admitted tool and proves it did not change
// while it was read.
func userAgentToolRead(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err := userAgentToolCustody(path, info, err); err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	fresh, freshErr := os.Lstat(path)
	if err != nil || freshErr != nil || int64(len(data)) != info.Size() || !os.SameFile(info, fresh) || fresh.Size() != info.Size() || !fresh.ModTime().Equal(info.ModTime()) {
		return nil, nil, privateError("preimage", errors.Join(err, freshErr, fmt.Errorf("agent tool %q changed while it was saved", path)))
	}
	return data, info, nil
}

// userSetAsideAgentTools moves Pi's own fd/rg out of AGENT/bin into the
// stage's backup, after recording their digests, sizes and modes as the
// installation's evidence. It runs after the agent rollback image is taken,
// so a later failure before the publication commit restores them.
func userSetAsideAgentTools(ctx context.Context, agent, root string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	present, err := userAgentToolsAdmit(agent)
	if err != nil || len(present) == 0 {
		return err
	}
	bin := filepath.Join(agent, "bin")
	backup := filepath.Join(root, userAgentToolsBackup)
	if err := os.MkdirAll(backup, 0700); err != nil {
		return err
	}
	record := userAgentToolsRecord{Schema: userAgentToolsSchema, Agent: agent, Files: map[string]userAgentToolImage{}}
	for _, name := range present {
		data, info, err := userAgentToolRead(filepath.Join(bin, name))
		if err != nil {
			return err
		}
		// The copy is inert data: the recorded mode is applied on restore.
		if err := userToolWrite(filepath.Join(backup, name), data, 0600); err != nil {
			return err
		}
		record.Files[name] = userAgentToolImage{SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Size: info.Size(), Mode: fmt.Sprintf("%04o", info.Mode().Perm())}
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
		return err
	}
	if err := userToolWrite(filepath.Join(root, userAgentToolsEvidence), data, 0600); err != nil {
		return err
	}
	for _, name := range present {
		path := filepath.Join(bin, name)
		current, _, err := userAgentToolRead(path)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(current)) != record.Files[name].SHA256 {
			return privateError("preimage", fmt.Errorf("agent tool %q changed while it was set aside", path))
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return userDirectorySync(bin)
}

// userReadAgentToolsRecord loads the set-aside evidence of an agent install.
// An install that set nothing aside has no record.
func userReadAgentToolsRecord(target, agent string) (*userAgentToolsRecord, error) {
	data, err := os.ReadFile(filepath.Join(target, userAgentToolsEvidence))
	if os.IsNotExist(err) {
		return nil, nil
	}
	var record userAgentToolsRecord
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &record) != nil || record.Schema != userAgentToolsSchema || record.Agent != agent || len(record.Files) == 0 {
		return nil, errors.Join(err, errors.New("the set-aside Pi tools evidence is malformed; uninstall aborted without touching the agent"))
	}
	for name, image := range record.Files {
		mode, modeErr := strconv.ParseUint(image.Mode, 8, 32)
		if !userAgentToolName(name) || len(image.SHA256) != 64 || strings.Trim(image.SHA256, "0123456789abcdef") != "" || image.Size < 0 || image.Size > userAgentImageBound || modeErr != nil || mode > 0777 || mode&0022 != 0 {
			return nil, fmt.Errorf("the set-aside Pi tools evidence for %q is malformed; uninstall aborted without touching the agent", name)
		}
	}
	return &record, nil
}

func userAgentToolName(name string) bool {
	for _, source := range userToolSources {
		if source.name == name {
			return true
		}
	}
	return false
}

// userAgentToolsRestore is the uninstall plan for the set-aside tools,
// decided before any write: restore Pi's copy into a slot that still holds
// the pinned tool or is empty, skip one already restored (same bytes and
// mode; Pi may download the very release the installer pins), and keep the
// backup of one the operator or Pi replaced.
type userAgentToolsRestore struct {
	target, agent string
	record        *userAgentToolsRecord
	restore, keep []string
	data          map[string][]byte
}

// userPlanAgentToolsRestore classifies every set-aside tool against the
// install's post-image evidence and verifies each backup it will restore.
func userPlanAgentToolsRestore(target, agent string, post map[string]userAgentPostFile) (*userAgentToolsRestore, error) {
	record, err := userReadAgentToolsRecord(target, agent)
	if err != nil || record == nil {
		return nil, err
	}
	plan := &userAgentToolsRestore{target: target, agent: agent, record: record, data: map[string][]byte{}}
	bin := filepath.Join(agent, "bin")
	binInfo, binErr := os.Lstat(bin)
	for _, source := range userToolSources {
		image, ok := record.Files[source.name]
		if !ok {
			continue
		}
		path := filepath.Join(bin, source.name)
		info, err := os.Lstat(path)
		current := ""
		if err == nil && info.Mode().IsRegular() {
			if data, readErr := os.ReadFile(path); readErr == nil {
				current = fmt.Sprintf("%x", sha256.Sum256(data))
			}
		}
		pinned := post["bin/"+source.name]
		switch {
		case binErr != nil || !binInfo.IsDir():
			plan.keep = append(plan.keep, source.name)
			continue
		case current != "" && current == image.SHA256 && fmt.Sprintf("%04o", info.Mode().Perm()) == image.Mode:
			continue // Already restored by an earlier attempt.
		case os.IsNotExist(err), current != "" && pinned.Exists && current == pinned.SHA256:
		default:
			plan.keep = append(plan.keep, source.name)
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(target, userAgentToolsBackup, source.name))
		if readErr != nil || int64(len(data)) != image.Size || fmt.Sprintf("%x", sha256.Sum256(data)) != image.SHA256 {
			return nil, errors.Join(readErr, fmt.Errorf("the set-aside Pi %s in %q is damaged or missing; uninstall aborted without touching the agent", source.name, filepath.Join(target, userAgentToolsBackup)))
		}
		plan.restore, plan.data[source.name] = append(plan.restore, source.name), data
	}
	return plan, nil
}

// skips reports whether a touched agent file is handled by this plan instead
// of the strict post-image restore.
func (p *userAgentToolsRestore) skips(name string) bool {
	if p == nil {
		return false
	}
	_, ok := p.record.Files[strings.TrimPrefix(name, "bin/")]
	return ok && strings.HasPrefix(name, "bin/")
}

// apply puts each restorable tool back with its recorded bytes and mode,
// replacing the pinned copy in one rename, and drops its backup copy. It
// returns the tools whose backup is kept.
func (p *userAgentToolsRestore) apply() ([]string, error) {
	if p == nil {
		return nil, nil
	}
	bin := filepath.Join(p.agent, "bin")
	for _, name := range p.restore {
		mode, _ := strconv.ParseUint(p.record.Files[name].Mode, 8, 32)
		path := filepath.Join(bin, name)
		if err := userAgentReplaceFile(path, p.data[name], os.FileMode(mode)); err != nil {
			return nil, err
		}
		if restored, err := os.ReadFile(path); err != nil || !bytes.Equal(restored, p.data[name]) {
			return nil, errors.Join(err, fmt.Errorf("restored Pi %s in %q did not read back", name, path))
		}
		if err := os.Remove(filepath.Join(p.target, userAgentToolsBackup, name)); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return p.keep, nil
}

// userKeepAgentTools moves the kept tool backups out of an uninstall backup
// before it is pruned, with their evidence, and words the outcome.
func userKeepAgentTools(backup, agent string, kept []string) (string, error) {
	keptDir := backup + userAgentToolsKeptSuffix
	if _, err := os.Lstat(keptDir); !os.IsNotExist(err) {
		return "", fmt.Errorf("kept Pi tools path %q is occupied; the originals stay in %q", keptDir, filepath.Join(backup, userAgentToolsBackup))
	}
	if err := os.Rename(filepath.Join(backup, userAgentToolsBackup), keptDir); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(backup, userAgentToolsEvidence))
	if err == nil {
		err = userToolWrite(filepath.Join(keptDir, userAgentToolsKeptEvidence), data, 0600)
	}
	if err != nil {
		return "", fmt.Errorf("Pi's original tools are kept in %q but their evidence could not be copied: %w", keptDir, err)
	}
	var paths []string
	for _, name := range kept {
		paths = append(paths, strconv.Quote(filepath.Join(agent, "bin", name)))
	}
	return fmt.Sprintf("Pi's %s in %s changed after installation (Pi may have downloaded a newer copy), so uninstall left it as it is; Pi's original is kept in %q with its digest and mode in %s",
		strings.Join(kept, " and "), strings.Join(paths, " and "), keptDir, userAgentToolsKeptEvidence), nil
}
