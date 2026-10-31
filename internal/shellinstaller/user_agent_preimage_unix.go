//go:build linux || darwin

package shellinstaller

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// The agent preimage evidence records, for each agent file an install
// rewrites (settings.json, mcp.json), whether it existed before the install
// and its mode, so uninstall restores presence, bytes and mode exactly.
// Installs made before this evidence existed restore owner-only (0600), which
// never widens access.
const (
	userAgentPreimageSchema   = "gentle-shell-agent-preimage/v1"
	userAgentPreimageEvidence = "state/agent.preimage.json"
	userAgentPreimageLegacy   = os.FileMode(0600)
)

type userAgentPreimageFile struct {
	Exists bool   `json:"exists"`
	Mode   string `json:"mode,omitempty"`
}

type userAgentPreimageRecord struct {
	Schema string                           `json:"schema"`
	Files  map[string]userAgentPreimageFile `json:"files"`
}

// userRecordAgentPreimage records the preimage presence and mode of one agent
// file; info is nil when the file did not exist before the install.
func userRecordAgentPreimage(root, name string, info os.FileInfo) error {
	record, err := userReadAgentPreimageRecord(root)
	if err != nil {
		return err
	}
	if record == nil {
		record = &userAgentPreimageRecord{Schema: userAgentPreimageSchema, Files: map[string]userAgentPreimageFile{}}
	}
	entry := userAgentPreimageFile{}
	if info != nil {
		entry = userAgentPreimageFile{Exists: true, Mode: fmt.Sprintf("%04o", info.Mode().Perm())}
	}
	record.Files[name] = entry
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "state"), 0700); err != nil {
		return err
	}
	return userAgentReplaceFile(filepath.Join(root, userAgentPreimageEvidence), data, 0600)
}

// userReadAgentPreimageRecord loads and validates the preimage evidence. An
// install without it has no record; a recorded mode never grants group or
// other write access.
func userReadAgentPreimageRecord(root string) (*userAgentPreimageRecord, error) {
	data, err := os.ReadFile(filepath.Join(root, userAgentPreimageEvidence))
	if os.IsNotExist(err) {
		return nil, nil
	}
	var record userAgentPreimageRecord
	if err != nil || len(data) > 4096 || json.Unmarshal(data, &record) != nil || record.Schema != userAgentPreimageSchema || record.Files == nil {
		return nil, errors.Join(err, errors.New("the agent preimage evidence is malformed; uninstall aborted without touching the agent"))
	}
	for name, entry := range record.Files {
		if name != "settings.json" && name != "mcp.json" {
			return nil, fmt.Errorf("the agent preimage evidence names %q; uninstall aborted without touching the agent", name)
		}
		if _, err := userAgentPreimageMode(entry); err != nil {
			return nil, fmt.Errorf("the agent preimage evidence for %q is malformed (%v); uninstall aborted without touching the agent", name, err)
		}
	}
	return &record, nil
}

// userAgentPreimageMode returns the mode a present preimage is restored with.
func userAgentPreimageMode(entry userAgentPreimageFile) (os.FileMode, error) {
	if !entry.Exists {
		if entry.Mode != "" {
			return 0, errors.New("an absent file has a mode")
		}
		return 0, nil
	}
	mode, err := strconv.ParseUint(entry.Mode, 8, 32)
	if err != nil || len(entry.Mode) != 4 || mode > 0777 || mode&0022 != 0 {
		return 0, fmt.Errorf("mode %q is not a file mode without group or other write access", entry.Mode)
	}
	return os.FileMode(mode), nil
}

// userAgentReplaceFile replaces path with data and mode in one rename, after
// syncing the new bytes; the containing directory is synced too.
func userAgentReplaceFile(path string, data []byte, mode os.FileMode) error {
	temporary := path + ".gentle-restore"
	if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
		return fmt.Errorf("restore path %q is occupied", temporary)
	}
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, userSync(file), file.Close(), os.Chmod(temporary, mode))
	if err == nil {
		err = os.Rename(temporary, path)
	}
	if err != nil {
		return errors.Join(err, os.Remove(temporary))
	}
	return userDirectorySync(filepath.Dir(path))
}
