package opencodedefault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v3/internal/components/mutationjournal"
)

const (
	ManagedAgent = "gentle-orchestrator"
	schema       = "gentle-ai.opencode-default-agent"
	version      = 1
)

type ownership struct {
	Schema          string `json:"schema"`
	Version         int    `json:"version"`
	State           string `json:"state"`
	PreviousState   string `json:"previous_state"`
	PreviousDefault string `json:"previous_default,omitempty"`
}
type fieldValue struct {
	present bool
	value   string
}

// InstallPlan sets default_agent to the managed orchestrator and records the
// value it replaced, so uninstall can hand the field back (v3.7.0 semantics).
type InstallPlan struct {
	settingsPath string
	owned        *ownership
	recapture    bool
}
type UninstallPlan struct {
	settingsPath  string
	settingsExist bool
	current       fieldValue
	owned         *ownership
}

func OwnershipPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), ".gentle-ai-default-agent.json")
}

// PrepareInstall must run before the managed orchestrator is merged. When the
// settings or the orchestrator did not exist yet, any earlier record is stale
// and the current default is captured again.
func PrepareInstall(settingsPath string) (*InstallPlan, error) {
	root, _, exists, _, err := readSettings(settingsPath)
	if err != nil {
		return nil, err
	}
	owned, err := readOwnership(OwnershipPath(settingsPath))
	if err != nil {
		return nil, err
	}
	agents, _ := root["agent"].(map[string]any)
	_, managedAgentPresent := agents[ManagedAgent]
	return &InstallPlan{settingsPath: settingsPath, owned: owned, recapture: !exists || !managedAgentPresent}, nil
}

// Apply writes default_agent and its ownership record together. A record whose
// managed value is still in place is kept, so a repeated install never
// captures its own value as the user's previous default.
func (p *InstallPlan) Apply() (bool, error) {
	_, raw, _, current, err := readSettings(p.settingsPath)
	if err != nil {
		return false, err
	}
	owned := p.owned
	if owned == nil || p.recapture || !current.present || current.value != ManagedAgent {
		owned = newOwnership(current)
	}
	// Merge instead of re-encoding so permission rule order and every
	// unrelated user value are preserved.
	settings, err := filemerge.MergeJSONObjects(raw, []byte(`{"default_agent":"`+ManagedAgent+`"}`))
	if err != nil {
		return false, err
	}
	metadata := encode(owned)
	ownerPath := OwnershipPath(p.settingsPath)
	ownerRaw, _ := os.ReadFile(ownerPath)
	changed := !bytes.Equal(raw, settings) || !bytes.Equal(ownerRaw, metadata)
	if !changed {
		return false, nil
	}
	if err := writePair(p.settingsPath, settings, true, ownerPath, metadata, true); err != nil {
		return false, err
	}
	return true, nil
}

// ApplyShareDefault disables session sharing in an OpenCode-compatible config
// (OpenCode or Kilo) unless the user already chose a share mode. Child sessions created for subagents can route through
// SessionShare.create when sharing is enabled, which has been observed to fail
// with a SQLite FOREIGN KEY error. An explicit user value is never changed.
func ApplyShareDefault(settingsPath string) (bool, error) {
	root, raw, exists, _, err := readSettings(settingsPath)
	if err != nil || !exists {
		return false, err
	}
	if _, chosen := root["share"]; chosen {
		return false, nil
	}
	settings, err := filemerge.MergeJSONObjects(raw, []byte(`{"share":"disabled"}`))
	if err != nil {
		return false, err
	}
	written, err := filemerge.WriteFileAtomic(settingsPath, settings, filemerge.ExistingFileMode(settingsPath, 0o644))
	return written.Changed, err
}
func PrepareUninstall(settingsPath string) (*UninstallPlan, error) {
	_, _, exists, current, err := readSettings(settingsPath)
	if err != nil {
		return nil, err
	}
	owned, err := readOwnership(OwnershipPath(settingsPath))
	if err != nil {
		return nil, err
	}
	return &UninstallPlan{settingsPath: settingsPath, settingsExist: exists, current: current, owned: owned}, nil
}
func (p *UninstallPlan) Apply(cleaned []byte, settingsExist bool) (changed, removed bool, err error) {
	// Only an ownership record proves that gentle-ai may roll back this
	// field. A user-modified default is not ours to change or release.
	if p.owned == nil || !p.settingsExist || !p.current.present || p.current.value != ManagedAgent {
		return false, false, nil
	}
	root := map[string]any{}
	if settingsExist {
		if root, err = filemerge.UnmarshalJSONObject(cleaned); err != nil {
			return false, false, fmt.Errorf("parse cleaned OpenCode settings: %w", err)
		}
	}
	if p.owned.PreviousState == "absent" {
		delete(root, "default_agent")
	} else {
		root["default_agent"] = p.owned.PreviousDefault
	}
	settingsExist = settingsExist && len(root) > 0
	var settings []byte
	if settingsExist {
		settings = encode(root)
	}
	currentRaw, readErr := os.ReadFile(p.settingsPath)
	currentExists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, false, readErr
	}
	changed = currentExists != settingsExist || (settingsExist && !bytes.Equal(currentRaw, settings)) || p.owned != nil
	if err := writePair(p.settingsPath, settings, settingsExist, OwnershipPath(p.settingsPath), nil, false); err != nil {
		return false, false, err
	}
	return changed, currentExists && !settingsExist, nil
}
func readSettings(path string) (map[string]any, []byte, bool, fieldValue, error) {
	raw, err := readRegular(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil, false, fieldValue{}, nil
	}
	if err != nil {
		return nil, nil, false, fieldValue{}, fmt.Errorf("read OpenCode settings %q: %w", path, err)
	}
	root, err := filemerge.UnmarshalJSONObject(raw)
	if err != nil {
		return nil, nil, false, fieldValue{}, fmt.Errorf("parse OpenCode settings %q: %w", path, err)
	}
	current, err := defaultField(root)
	if err != nil {
		return nil, nil, false, fieldValue{}, err
	}
	return root, raw, true, current, nil
}
func readOwnership(path string) (*ownership, error) {
	raw, err := readRegular(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read OpenCode default ownership %q: %w", path, err)
	}
	var owned ownership
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&owned); err != nil {
		return nil, fmt.Errorf("decode OpenCode default ownership %q: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode OpenCode default ownership %q: trailing data", path)
	}
	validPrevious := owned.PreviousState == "absent" && owned.PreviousDefault == "" || owned.PreviousState == "value"
	if owned.Schema != schema || owned.Version != version || owned.State != "managed" || !validPrevious {
		return nil, fmt.Errorf("invalid OpenCode default ownership %q", path)
	}
	return &owned, nil
}
func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	return os.ReadFile(path)
}
func defaultField(root map[string]any) (fieldValue, error) {
	value, exists := root["default_agent"]
	if !exists {
		return fieldValue{}, nil
	}
	text, ok := value.(string)
	if !ok {
		return fieldValue{}, fmt.Errorf("OpenCode default_agent must be a string")
	}
	return fieldValue{present: true, value: text}, nil
}
func newOwnership(previous fieldValue) *ownership {
	owned := &ownership{Schema: schema, Version: version, State: "managed", PreviousState: "absent"}
	if previous.present {
		owned.PreviousState = "value"
		owned.PreviousDefault = previous.value
	}
	return owned
}
func encode(value any) []byte {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err) // Values are either decoded JSON or the fixed ownership struct.
	}
	return append(raw, '\n')
}
func writePair(settingsPath string, settings []byte, keepSettings bool, ownerPath string, owner []byte, keepOwner bool) error {
	journal := mutationjournal.New(filepath.Dir(settingsPath))
	if err := journal.Capture(settingsPath); err != nil {
		return err
	}
	if err := journal.Capture(ownerPath); err != nil {
		return err
	}
	mutate := func(path string, data []byte, keep bool) error {
		if keep {
			mode := os.FileMode(0o644)
			if info, err := os.Lstat(path); err == nil {
				mode = info.Mode().Perm()
			} else if !os.IsNotExist(err) {
				return err
			}
			_, err := journal.WriteWithMode(path, data, mode)
			return err
		}
		_, err := journal.Remove(path)
		return err
	}
	if err := mutate(settingsPath, settings, keepSettings); err != nil {
		return fmt.Errorf("update OpenCode settings: %w (rollback: %v)", err, journal.Restore())
	}
	if err := mutate(ownerPath, owner, keepOwner); err != nil {
		return fmt.Errorf("update OpenCode default ownership: %w (rollback: %v)", err, journal.Restore())
	}
	return nil
}
