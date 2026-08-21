package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const piTargetPackageName = "gentle-pi"

var (
	ErrPackageNotConfigured = errors.New("gentle-pi is not declared in Pi settings")
	ErrInvalidPiRoot        = errors.New("Pi settings root must be absolute")
)

type PackageSourceScope string

const (
	SettingsScopeProject PackageSourceScope = "project"
	SettingsScopeUser    PackageSourceScope = "user"
	SettingsScopeNone    PackageSourceScope = "none"
)

type PackageSourceSelection struct {
	Source                      string
	Scope                       PackageSourceScope
	SettingsPath, CWD, AgentDir string
}
type SettingsError struct {
	Path, Kind string
	Scope      PackageSourceScope
	Cause      error
}
type NotConfiguredError struct {
	Path, ProjectPath, UserPath, CWD, AgentDir string
	Scope                                      PackageSourceScope
	Cause                                      error
}

func (e *SettingsError) Error() string {
	return fmt.Sprintf("Pi settings error (%s, scope=%s) at %q: %v", e.Kind, e.Scope, e.Path, e.Cause)
}
func (e *SettingsError) Unwrap() error { return e.Cause }
func (e *NotConfiguredError) Error() string {
	return fmt.Sprintf("Pi package is not configured (scope=%s) at %q", e.Scope, e.Path)
}
func (e *NotConfiguredError) Unwrap() error { return e.Cause }
func SelectPackageSource(cwd, agentDir string) (PackageSourceSelection, error) {
	resolvedCWD, err := absolutePiRoot(cwd, "cwd")
	if err != nil {
		return PackageSourceSelection{}, settingsFailure("cwd", SettingsScopeNone, "invalid-root", err)
	}
	resolvedAgentDir, err := resolvePiAgentDir(agentDir)
	if err != nil {
		return PackageSourceSelection{}, settingsFailure("agentDir", SettingsScopeNone, "invalid-root", err)
	}
	projectPath, userPath := filepath.Join(resolvedCWD, ".pi", "settings.json"), filepath.Join(resolvedAgentDir, "settings.json")
	for _, lookup := range []struct {
		path  string
		scope PackageSourceScope
	}{{projectPath, SettingsScopeProject}, {userPath, SettingsScopeUser}} {
		declarations, present, err := readPiPackageSettings(lookup.path, lookup.scope)
		if err != nil {
			return PackageSourceSelection{}, err
		}
		if !present {
			continue
		}
		for _, declaration := range declarations {
			if isGentlePiDeclaration(declaration) {
				return PackageSourceSelection{declaration.Source, lookup.scope, lookup.path, resolvedCWD, resolvedAgentDir}, nil
			}
		}
	}
	return PackageSourceSelection{Scope: SettingsScopeNone, CWD: resolvedCWD, AgentDir: resolvedAgentDir}, &NotConfiguredError{
		Path: userPath, Scope: SettingsScopeNone, ProjectPath: projectPath, UserPath: userPath,
		CWD: resolvedCWD, AgentDir: resolvedAgentDir, Cause: ErrPackageNotConfigured,
	}
}
func resolvePiAgentDir(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return absolutePiRoot(explicit, "agentDir")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return absolutePiRoot(CodeGraphPaths(home).AgentDir, "agentDir")
}
func absolutePiRoot(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) || !filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: %s=%q", ErrInvalidPiRoot, label, value)
	}
	return filepath.Clean(value), nil
}

type piPackageDeclaration struct{ Source, Name string }

func readPiPackageSettings(path string, scope PackageSourceScope) ([]piPackageDeclaration, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, settingsFailure(path, scope, "unreadable-settings", err)
	}
	object, err := decodePiSettings(data)
	if err != nil {
		return nil, true, settingsFailure(path, scope, "malformed-settings", err)
	}
	raw, ok := object["packages"]
	if !ok {
		return nil, true, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		if err == nil {
			err = errors.New("packages must be an array")
		}
		return nil, true, settingsFailure(path, scope, "malformed-packages", err)
	}
	declarations := make([]piPackageDeclaration, 0, len(entries))
	for _, entry := range entries {
		declaration, err := parsePiPackageDeclaration(entry)
		if err != nil {
			return nil, true, settingsFailure(path, scope, "malformed-package", err)
		}
		declarations = append(declarations, declaration)
	}
	return declarations, true, nil
}
func decodePiSettings(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("settings must be a JSON object")
	}
	return object, scanJSONValue(json.NewDecoder(bytes.NewReader(data)), otherObject)
}
func parsePiPackageDeclaration(raw json.RawMessage) (piPackageDeclaration, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return piPackageDeclaration{}, errors.New("empty package declaration")
	}
	if raw[0] == '"' {
		var source string
		if err := json.Unmarshal(raw, &source); err != nil {
			return piPackageDeclaration{}, err
		}
		if strings.TrimSpace(source) == "" {
			return piPackageDeclaration{}, errors.New("package source must be non-empty")
		}
		return piPackageDeclaration{Source: source}, nil
	}
	if raw[0] != '{' {
		return piPackageDeclaration{}, errors.New("package must be a string or object")
	}
	object, err := decodePiSettings(raw)
	if err != nil {
		return piPackageDeclaration{}, err
	}
	for key := range object {
		if key != "source" && key != "name" {
			return piPackageDeclaration{}, fmt.Errorf("unknown package object field %q", key)
		}
	}
	rawSource, ok := object["source"]
	if !ok {
		return piPackageDeclaration{}, errors.New("package object requires source")
	}
	var declaration piPackageDeclaration
	if err := json.Unmarshal(rawSource, &declaration.Source); err != nil {
		return piPackageDeclaration{}, fmt.Errorf("package source: %w", err)
	}
	if strings.TrimSpace(declaration.Source) == "" {
		return piPackageDeclaration{}, errors.New("package source must be non-empty")
	}
	if rawName, ok := object["name"]; ok {
		if bytes.Equal(bytes.TrimSpace(rawName), []byte("null")) {
			return piPackageDeclaration{}, errors.New("package name must be a string")
		}
		if err := json.Unmarshal(rawName, &declaration.Name); err != nil {
			return piPackageDeclaration{}, fmt.Errorf("package name: %w", err)
		}
	}
	return declaration, nil
}
func isGentlePiDeclaration(declaration piPackageDeclaration) bool {
	if declaration.Name == piTargetPackageName {
		return true
	}
	source := strings.TrimSpace(declaration.Source)
	return source == piTargetPackageName || source == "npm:"+piTargetPackageName || strings.HasPrefix(source, "npm:"+piTargetPackageName+"@")
}
func settingsFailure(path string, scope PackageSourceScope, kind string, cause error) error {
	return &SettingsError{Path: path, Scope: scope, Kind: kind, Cause: cause}
}
