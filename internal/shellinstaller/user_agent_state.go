package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The installer owns exactly four agent entries (userAgentTouchFiles): the
// settings and mcp JSON documents and the two pinned tools under bin/. Every
// other entry in a recorded installation's agent is operator state - provider
// logins, sessions, project state, user models - that must survive every
// update and reconfigure of that installation.
const (
	userAgentCarriedSettings = "settings.json"
	userAgentCarriedMCP      = "mcp.json"
)

// userAgentInstallerKeys are the settings keys the provisioner owns; a carry
// never lets the operator's stale copy replace them.
var userAgentInstallerKeys = []string{"packages", "npmCommand"}

// userCarryAgentStateEntries carries the operator state of a retired
// installation's agent into a freshly provisioned stage agent. Installer-owned
// content is never replaced: bin/ keeps the fresh pinned tools, settings.json
// keeps the fresh declarations (packages, npmCommand) and gains the
// operator's other keys, mcp.json keeps the fresh Context7 entry and gains
// the operator's other servers. An unexpected collision is refused, never
// overwritten.
func userCarryAgentStateEntries(ctx context.Context, from, to string) error {
	if ctx == nil || ctx.Err() != nil {
		return privateError("canceled", context.Canceled)
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return fmt.Errorf("carry read %q: %w", from, err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "bin", userAgentCarriedSettings, userAgentCarriedMCP:
			continue // Installer-owned: the fresh stage content stays.
		}
		source, target := filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("carry stat %q: %w", source, err)
		}
		if info.Mode().Type().IsDir() {
			if err := userCarryTree(source, target); err != nil {
				return err
			}
			continue
		}
		if info.Mode().Type().IsRegular() {
			if err := userCarryFile(source, target, info.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		return privateError("refused", fmt.Errorf("retired agent contains non-regular entry %q: %#x", source, info.Mode().Type()))
	}
	if err := userCarrySettings(from, to); err != nil {
		return err
	}
	return userCarryMCP(from, to)
}

func userCarryTree(from, to string) error {
	entries, err := os.ReadDir(from)
	if err != nil {
		return fmt.Errorf("carry read %q: %w", from, err)
	}
	if err := os.MkdirAll(to, 0700); err != nil {
		return fmt.Errorf("carry create %q: %w", to, err)
	}
	for _, entry := range entries {
		source, target := filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("carry stat %q: %w", source, err)
		}
		switch kind := info.Mode().Type(); {
		case kind.IsDir():
			if err := userCarryTree(source, target); err != nil {
				return err
			}
		case kind.IsRegular():
			if err := userCarryFile(source, target, info.Mode().Perm()); err != nil {
				return err
			}
		case kind&fs.ModeSymlink != 0:
			link, err := os.Readlink(source)
			if err != nil {
				return fmt.Errorf("carry link %q: %w", source, err)
			}
			if err := os.Symlink(link, target); err != nil {
				return fmt.Errorf("carry link %q: %w", target, err)
			}
		default:
			return privateError("refused", fmt.Errorf("carried state contains non-regular entry %q: %#x", source, kind))
		}
	}
	return nil
}

func userCarryFile(from, to string, mode fs.FileMode) error {
	if _, err := os.Lstat(to); !errors.Is(err, os.ErrNotExist) {
		return privateError("refused", fmt.Errorf("carried state collides with %q: %w", to, err))
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("carry read %q: %w", from, err)
	}
	if err := os.WriteFile(to, data, mode); err != nil {
		return fmt.Errorf("carry write %q: %w", to, err)
	}
	return os.Chmod(to, mode)
}

// userCarrySettings merges the operator's settings.json over the fresh one:
// the operator's keys (models, providers, everything user-configured) are
// preserved; the installer's declarations stay exactly as provisioned.
func userCarrySettings(from, to string) error {
	data, err := os.ReadFile(filepath.Join(from, userAgentCarriedSettings))
	if errors.Is(err, os.ErrNotExist) {
		return nil // Nothing operator-configured to carry.
	}
	if err != nil {
		return fmt.Errorf("carry read settings: %w", err)
	}
	var carried map[string]json.RawMessage
	if json.Unmarshal(data, &carried) != nil {
		return fmt.Errorf("carried settings are malformed")
	}
	fresh, err := os.ReadFile(filepath.Join(to, userAgentCarriedSettings))
	if err != nil {
		return fmt.Errorf("carry fresh settings: %w", err)
	}
	var merged map[string]json.RawMessage
	if json.Unmarshal(fresh, &merged) != nil {
		return fmt.Errorf("fresh settings are malformed")
	}
	for key, value := range carried {
		merged[key] = value
	}
	for _, key := range userAgentInstallerKeys {
		if freshValue, ok := rawKey(fresh, key); ok {
			merged[key] = freshValue
		} else {
			delete(merged, key)
		}
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return userCarryWrite(filepath.Join(to, userAgentCarriedSettings), out)
}

// userCarryMCP merges the operator's non-Context7 servers over the fresh
// mcp.json; the fresh Context7 entry stays authoritative.
func userCarryMCP(from, to string) error {
	data, err := os.ReadFile(filepath.Join(from, userAgentCarriedMCP))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("carry read mcp: %w", err)
	}
	var carried struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(data, &carried) != nil {
		return fmt.Errorf("carried mcp is malformed")
	}
	fresh, err := os.ReadFile(filepath.Join(to, userAgentCarriedMCP))
	if errors.Is(err, os.ErrNotExist) {
		return nil // The profile staged no mcp document; nothing to merge into.
	}
	if err != nil {
		return fmt.Errorf("carry fresh mcp: %w", err)
	}
	var merged struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if json.Unmarshal(fresh, &merged) != nil {
		return fmt.Errorf("fresh mcp is malformed")
	}
	for name, server := range carried.Servers {
		if name == "context7" {
			continue
		}
		merged.Servers[name] = server
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	return userCarryWrite(filepath.Join(to, userAgentCarriedMCP), out)
}

func rawKey(data []byte, key string) (json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return nil, false
	}
	value, ok := fields[key]
	return value, ok
}

func userCarryWrite(path string, data []byte) error {
	if err := os.Chmod(path, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("carry write %q: %w", path, err)
	}
	return os.Chmod(path, 0600)
}
