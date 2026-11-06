//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// userAgentImageBound bounds every operator file the agent rollback and
// post-image evidence read. It matches the per-file bound of the selection
// inventory (userTreeStamp) and the provision helper, so the pinned fd/rg the
// installer binds into a stock agent stay within it.
const userAgentImageBound = 32 << 20

// userSelectedAgentIdentity binds the agent role of a manifest. An agent-reuse
// install points at the operator's own Pi agent, which Pi creates 0755: it
// must be owned, physical and not writable by another user, and it never
// gains broader access from the installer. Every other agent (Separate's
// private agent, Shared's selected agent) keeps the private 0700 rule.
func userSelectedAgentIdentity(mode, agent string) (string, error) {
	if mode == "agent" {
		return userOwnedIdentity(agent)
	}
	return userIdentity(agent)
}

type userAgentFileImage struct {
	exists bool
	mode   os.FileMode
	data   []byte
}

// userAgentRollback is the pre-install image of everything an agent-reuse
// install writes in the operator's Pi agent: the touched files, the bin
// directory and the installer's own temporaries. Restoring it returns the
// agent to its exact contents, modes and presence; files the installer never
// writes (sessions, auth) are never read or touched.
type userAgentRollback struct {
	agent     string
	identity  string
	entries   map[string]bool
	binExists bool
	files     map[string]userAgentFileImage
}

// userBeginAgentRollback records the image immediately before the first
// write to the agent.
func userBeginAgentRollback(agent string) (*userAgentRollback, error) {
	identity, err := userOwnedIdentity(agent)
	if err != nil {
		return nil, err
	}
	guard := &userAgentRollback{agent: agent, identity: identity, entries: map[string]bool{}, files: map[string]userAgentFileImage{}}
	entries, err := os.ReadDir(agent)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		guard.entries[entry.Name()] = true
	}
	if _, err := os.Lstat(filepath.Join(agent, "bin")); err == nil {
		guard.binExists = true
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for _, name := range userAgentTouchFiles {
		path := filepath.Join(agent, filepath.FromSlash(name))
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > userAgentImageBound {
			return nil, privateError("preimage", fmt.Errorf("%q is not a bounded regular file", path))
		}
		data, err := os.ReadFile(path)
		if err != nil || int64(len(data)) != info.Size() {
			return nil, privateError("preimage", errors.Join(err, fmt.Errorf("%q changed while it was saved", path)))
		}
		guard.files[name] = userAgentFileImage{exists: true, mode: info.Mode().Perm(), data: data}
	}
	return guard, nil
}

// rollback restores the agent after a failed install and reports the outcome
// with the original cause. When the agent cannot be restored, the saved
// pre-install files are kept beside the target for the operator.
func (g *userAgentRollback) rollback(cause error, parent string) error {
	if err := g.restore(); err != nil {
		return fmt.Errorf("%w; the Pi agent %q was not restored (%v); %s", cause, g.agent, err, g.keep(parent))
	}
	return fmt.Errorf("%w; the Pi agent %q was restored to its pre-install state", cause, g.agent)
}

func (g *userAgentRollback) restore() error {
	if current, err := userOwnedIdentity(g.agent); err != nil || current != g.identity {
		return errors.Join(errors.New("the agent directory was replaced or changed custody during installation"), err)
	}
	var failures []error
	entries, err := os.ReadDir(g.agent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if g.entries[name] || (!strings.HasPrefix(name, ".gentle-shell-settings-") && name != "mcp.json.new") {
			continue
		}
		failures = append(failures, userAgentRemoveRegular(filepath.Join(g.agent, name)))
	}
	for _, name := range userAgentTouchFiles {
		failures = append(failures, g.restoreFile(name))
	}
	bin := filepath.Join(g.agent, "bin")
	if !g.binExists {
		if err := os.Remove(bin); err != nil && !os.IsNotExist(err) {
			failures = append(failures, fmt.Errorf("remove %q: %w", bin, err))
		}
	} else {
		failures = append(failures, userDirectorySync(bin))
	}
	failures = append(failures, userDirectorySync(g.agent))
	return errors.Join(failures...)
}

func (g *userAgentRollback) restoreFile(name string) error {
	path := filepath.Join(g.agent, filepath.FromSlash(name))
	image := g.files[name]
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !image.exists {
		if err != nil {
			return nil
		}
		return userAgentRemoveRegular(path)
	}
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%q is no longer a regular file; left in place", path)
		}
		if data, readErr := os.ReadFile(path); readErr == nil && info.Mode().Perm() == image.mode && bytes.Equal(data, image.data) {
			return nil
		}
	}
	temporary := path + ".gentle-restore"
	if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
		return fmt.Errorf("restore path %q is occupied", temporary)
	}
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(image.data)
	err = errors.Join(writeErr, userSync(file), file.Close(), os.Chmod(temporary, image.mode))
	if err == nil {
		err = os.Rename(temporary, path)
	}
	if err != nil {
		return errors.Join(err, os.Remove(temporary))
	}
	return nil
}

func userAgentRemoveRegular(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file; left in place", path)
	}
	return os.Remove(path)
}

// keep saves the pre-install files in a private directory beside the target
// when the agent itself could not be restored.
func (g *userAgentRollback) keep(parent string) string {
	saved, err := os.MkdirTemp(parent, ".gentle-agent-preimage-")
	for _, name := range userAgentTouchFiles {
		image := g.files[name]
		if err != nil || !image.exists {
			continue
		}
		path := filepath.Join(saved, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err == nil {
			err = userToolWrite(path, image.data, 0600)
		}
	}
	if err != nil {
		return fmt.Sprintf("its pre-install files could not be saved (%v); preserve the agent as it is", err)
	}
	return fmt.Sprintf("its pre-install files are saved in %q; restore them by hand", saved)
}

type userPublicationCommitKey struct{}

// userWithPublicationCommit returns a context whose publication marks the
// returned flag once its rename has committed the destination.
func userWithPublicationCommit(ctx context.Context) (context.Context, *bool) {
	committed := new(bool)
	return context.WithValue(ctx, userPublicationCommitKey{}, committed), committed
}

// userMarkPublicationCommitted records that the destination is published with
// its manifest bound to the selected agent.
func userMarkPublicationCommitted(ctx context.Context) {
	if committed, ok := ctx.Value(userPublicationCommitKey{}).(*bool); ok {
		*committed = true
	}
}

// settle decides the agent outcome of a failed install. Before the
// publication commit the agent is restored. After it, the published manifest
// and its uninstall evidence are bound to the agent's current bindings, so a
// restore would orphan them: the failure is reported and the agent is kept.
func (g *userAgentRollback) settle(cause error, committed bool, parent string) error {
	if committed {
		return fmt.Errorf("%w; the installation was already published, so the Pi agent %q keeps the published bindings; uninstall restores it", cause, g.agent)
	}
	return g.rollback(cause, parent)
}
