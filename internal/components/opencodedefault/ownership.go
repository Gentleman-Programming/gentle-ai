package opencodedefault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/mutationjournal"
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
	return filepath.Join(filepath.Dir(settingsPath), ".gentle-ai-default-agent.json")
}
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
func (p *InstallPlan) Apply() (bool, error) {
	root, raw, _, current, err := readSettings(p.settingsPath)
	if err != nil {
		return false, err
	}
	owned := p.owned
	if owned == nil || p.recapture || !current.present || current.value != ManagedAgent {
		owned = newOwnership(current)
	}
	root["default_agent"] = ManagedAgent
	settings := encode(root)
	metadata := encode(owned)
	ownerPath := OwnershipPath(p.settingsPath)
	ownerRaw, _ := os.ReadFile(ownerPath)
	changed := !bytes.Equal(raw, settings) || !bytes.Equal(ownerRaw, metadata)
	if err := writePair(p.settingsPath, settings, true, ownerPath, metadata, true); err != nil {
		return false, err
	}
	return changed, nil
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
	root := map[string]any{}
	if settingsExist {
		if root, err = filemerge.UnmarshalJSONObject(cleaned); err != nil {
			return false, false, fmt.Errorf("parse cleaned OpenCode settings: %w", err)
		}
	}
	if p.settingsExist && p.current.present && p.current.value == ManagedAgent {
		if p.owned == nil || p.owned.PreviousState == "absent" {
			delete(root, "default_agent")
		} else {
			root["default_agent"] = p.owned.PreviousDefault
		}
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
			_, err := journal.WriteWithMode(path, data, 0o644)
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
