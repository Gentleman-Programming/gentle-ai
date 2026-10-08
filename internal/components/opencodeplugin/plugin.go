package opencodeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v4/internal/model"
	"github.com/gentleman-programming/gentle-ai/v4/internal/opencode"
)

type Definition struct {
	ID          model.OpenCodeCommunityPluginID
	Name        string
	PackageName string
	RepoURL     string
	Owner       string
	Repo        string
	Description string
}

type Result struct {
	Changed bool
	Files   []string
}

// Retained only to identify registrations owned by older installations during
// explicit CLI uninstall. External plugins are never installable or updatable.
var legacyStatuslineDefinition = Definition{
	ID:          model.OpenCodePluginSubAgentStatusline,
	Name:        "Sub-agent Statusline",
	PackageName: "opencode-subagent-statusline",
	RepoURL:     "https://github.com/Joaquinvesapa/sub-agent-statusline",
	Owner:       "Joaquinvesapa",
	Repo:        "sub-agent-statusline",
	Description: "OpenCode sidebar/statusline for sub-agent activity",
}

// Retain only for identifying registrations owned by older installations during
// uninstall. It is not offered in the installation or update catalogs.
var legacySDDEngramDefinition = Definition{
	ID:          model.OpenCodePluginSDDEngramManage,
	Name:        "SDD Engram Manager",
	PackageName: "opencode-sdd-engram-manage",
	RepoURL:     "https://github.com/j0k3r-dev-rgl/sdd-engram-plugin",
	Owner:       "j0k3r-dev-rgl",
	Repo:        "sdd-engram-plugin",
	Description: "OpenCode TUI for SDD profiles and Engram memories",
}

const gentleLogoPluginFile = "gentle-logo.tsx"

const gentleLogoPluginSource = `// @ts-nocheck
/** @jsxImportSource @opentui/solid */
import type { TuiPlugin, TuiPluginApi, TuiPluginModule } from "@opencode-ai/plugin/tui"
import { useTerminalDimensions } from "@opentui/solid"
import { createMemo } from "solid-js"

const id = "gentle-logo"

// Local shim (V2 module shape, issue #5364): OpenCode 2.x validates plugin
// modules against the server-side hook contract, so expose the optional
// setup entry as the server hook when the module does not define one of its own.
const Plugin = {
  define(mod: TuiPluginModule) {
    if (mod.setup && !mod.server) {
      return { ...mod, server: mod.setup }
    }
    return mod
  },
}

const roseArt = [
  "             ⣠⣾⣷⣶⣦⣤⣤⣄⣠⣄⣀  ⢀⣀⣀",
  "          ⢀⣴⣿⣿⠿⣋⣭⣭⣯⣭⣍⣭⣿⣟⠛⠛⠿⠿⣿⣷⣄",
  "      ⢀⣴⣾⡟⢻⣿⡟⠁⣼⣿⠏⣵⢻⣿⣻⣿⣿⢿⡻⣿⣿⣶⡌⢿⣿⣷⣦⣤⡄",
  "   ⣤⣶⣾⣿⣿⠏ ⠈⢿⣄ ⢹⣏⠠⠟⣾⣿⣿⣿⣿⣿⠷⣏⣼⠟⢡⣿⡟⠋⢻⣿⣿⡄",
  "   ⠈⣿⣿⣿⣿⡆   ⣽⢧⡘⠈⠳⣦⣍⠛⠛⢦⣉⣴⣛⣫⣭⣴⡟⠋  ⣾⣿⣿⡿",
  "   ⢀⠹⣿⣿⣿⣷⣤⡄ ⠋ ⠙⢆ ⣠⠴⠟⠛⣛⣛⣛⠟⠋⠁⠺⡇ ⣀⣴⣿⣿⡟⠁",
  "   ⠈⣀⠈⠛⠷⠿⣿⣿⣷⣤⣀ ⢠⠋   ⠈⠉⠉    ⣠⣴⣥⠾⠛⠉⣰⣿⣷",
  "          ⠹⣯⣝⠛⠛⠷⢶⣤⣤⣀   ⢀⡠⠖⠋⠉⢉⣀⣀⣴⣾⣿⠿⠟⠃ ⠠⠦",
  "⠁       ⠖  ⠘⠻⢿⣦⣄⡀  ⠉⠛⢦⠠⢊⠤⠴⢒⣛⣛⣩⣽⡿⠟⠁⢀⡀",
  "⠲⠶⣦⠴⠶⠶⠶⠶⡶⠶⢶⣤⣄⡀⠨⠭⠽⠟⣓⢦⣀⠈⢇⡥⠖⠛⠋⠉⠉⠉    ⠈  ⢠⡤",
  "  ⠈⢷ ⠐⠂⢤⣽⣄ ⠰⡎⠙⠳⣄⡀ ⠈⢣⠘⢦⠋⣀⡬⠟⠛⠛⠉⢀⣀⣀⣠⡤⠄⠃",
  "   ⠈⢳⣀⡒⠉⠉⣉⠙⡲⣽⣄ ⣏⠳⡄ ⠘⡇ ⡾⠁ ⢀⡤⠖⣻⣿⡏⢡⡎ ⠰⠄",
  "     ⠛⠻⢦⣄⣉⡁⣀⣀⣈⣙⣺⣌⡇⢠⢀⡇⡾  ⣴⣿⡷⠊ ⢲⣠⠟",
  "          ⠈⠉    ⠈⠳⡄⣸⢱⠇⢀⣰⣯⣭⣥⠭⠾⠛⠃",
  "                  ⡷⠡⡯⢖⠉   ⢠⠤",
  "                ⡠⢊⡴⠤⠂⠃ ⠒",
  "             ⢀⡴⢪⠔⣉⠔⠋",
  "               ⠐⠈",
]

const compactArt = ["✦ Gentle AI ✦"]

const Logo = () => {
  const dim = useTerminalDimensions()
  const lines = createMemo(() => {
    const term = dim()
    return term.height >= roseArt.length + 6 && term.width >= 64 ? roseArt : compactArt
  })

  return (
    <box flexDirection="column" alignItems="center">
      {lines().map((line) => (
        <text fg="magenta">{line}</text>
      ))}
    </box>
  )
}

const tui: TuiPlugin = async (api: TuiPluginApi) => {
  // Defensive guard: older runtimes may not expose the slots API, and the
  // logo is decorative, so its absence must never break the TUI boot.
  if (api.slots && typeof api.slots.register === "function") {
    api.slots.register({
      id,
      order: 100,
      slots: {
        home_logo() {
          return <Logo />
        },
      },
    })
  }
}

// The V2 loader requires a server-side entry to accept the module; the logo
// is TUI-only, so setup is intentionally a no-op.
const setup = async () => {}

export default Plugin.define({ id, setup, tui })
`

func DefinitionFor(id model.OpenCodeCommunityPluginID) (Definition, bool) {
	if id == model.OpenCodePluginSDDEngramManage {
		return legacySDDEngramDefinition, true
	}
	if id == model.OpenCodePluginSubAgentStatusline {
		return legacyStatuslineDefinition, true
	}
	return Definition{}, false
}

func Install(homeDir string, id model.OpenCodeCommunityPluginID) (Result, error) {
	// Legacy lookup is uninstall-only: refuse before probing or mutating anything.
	if id != model.OpenCodePluginGentleLogo {
		return Result{}, fmt.Errorf("OpenCode community plugin %q is not installable; existing configuration preserved", id)
	}
	// Runtime detection remains a fail-closed pre-flight: an unresolvable
	// runtime still blocks every managed asset, including the logo.
	if _, err := opencode.DetectRuntimeMajor(context.Background()); err != nil {
		return Result{}, err
	}

	return installGentleLogo(homeDir)
}

func installGentleLogo(homeDir string) (Result, error) {
	opencodeDir := filepath.Join(homeDir, ".config", "opencode")
	pluginPath := filepath.Join(opencodeDir, "tui-plugins", gentleLogoPluginFile)
	tuiPath := filepath.Join(opencodeDir, "tui.json")

	prior, err := capturePriorFile(pluginPath)
	if err != nil {
		return Result{}, fmt.Errorf("capture prior Gentle Logo TUI plugin state: %w", err)
	}
	tuiPrior, err := capturePriorFile(tuiPath)
	if err != nil {
		return Result{}, fmt.Errorf("capture prior OpenCode TUI config state: %w", err)
	}

	pluginWrite, err := writeFileAtomicFn(pluginPath, []byte(gentleLogoPluginSource), 0o644)
	if err != nil {
		// WriteFileAtomic can publish the replacement and still return an
		// error (#1676), so compensate the source before returning;
		// tui.json has not been touched at this point.
		restoreErr := prior.restore(pluginPath)
		if restoreErr != nil {
			return Result{}, errors.Join(
				fmt.Errorf("write Gentle Logo TUI plugin: %w", err),
				fmt.Errorf("roll back Gentle Logo TUI plugin, the previous state could not be restored: %w", restoreErr),
			)
		}
		return Result{}, fmt.Errorf("write Gentle Logo TUI plugin: %w", err)
	}
	tuiChanged, err := ensureTUIPluginFn(tuiPath, pluginPath)
	if err != nil {
		// WriteFileAtomic can report the replacement as landed even when it
		// returns an error (#1676), so compensate both files unconditionally;
		// restoring a tui.json write that never landed is a no-op.
		restoreErr := prior.restore(pluginPath)
		tuiRestoreErr := tuiPrior.restore(tuiPath)
		if restoreErr != nil || tuiRestoreErr != nil {
			joined := []error{fmt.Errorf("register Gentle Logo TUI plugin: %w", err)}
			if restoreErr != nil {
				joined = append(joined, fmt.Errorf("roll back Gentle Logo TUI plugin, the previous state could not be restored: %w", restoreErr))
			}
			if tuiRestoreErr != nil {
				joined = append(joined, fmt.Errorf("roll back OpenCode TUI config, the previous state could not be restored: %w", tuiRestoreErr))
			}
			return Result{}, errors.Join(joined...)
		}
		return Result{}, fmt.Errorf("register Gentle Logo TUI plugin: %w", err)
	}

	return Result{
		Changed: pluginWrite.Changed || tuiChanged,
		Files:   []string{pluginPath, tuiPath},
	}, nil
}

// ensureTUIPluginFn is a package-level seam so tests can inject a failing
// registration, mirroring the syncDirFn/renameFn seams in filemerge.
var ensureTUIPluginFn = ensureTUIPlugin

// writeFileAtomicFn is the source-write seam for installGentleLogo, so tests
// can exercise the landed-with-error window of WriteFileAtomic (#1676).
var writeFileAtomicFn = filemerge.WriteFileAtomic

// priorFile captures the prior on-disk state of a file so a multi-step install
// can compensate as one recoverable operation (#1678): a newly created file is
// removed and a pre-existing file is restored byte-exactly, including its mode.
type priorFile struct {
	existed bool
	data    []byte
	mode    os.FileMode
}

func capturePriorFile(path string) (priorFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return priorFile{}, nil
		}
		return priorFile{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return priorFile{}, err
	}
	return priorFile{existed: true, data: data, mode: info.Mode()}, nil
}

// restore puts back the captured bytes through the same durable write path used
// by installs, or removes the file when it did not previously exist. Removing an
// already-absent file is not an error.
func (p priorFile) restore(path string) error {
	if !p.existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if _, err := filemerge.WriteFileAtomicMode(path, p.data, p.mode.Perm()); err != nil {
		return err
	}
	return nil
}

func ensureTUIPlugin(path, pkg string) (bool, error) {
	root := map[string]any{"$schema": "https://opencode.ai/tui.json"}
	if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			return false, fmt.Errorf("parse OpenCode TUI config %q: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("read OpenCode TUI config %q: %w", path, err)
	}

	plugins := stringSlice(root["plugin"])
	for _, existing := range plugins {
		if existing == pkg {
			return false, nil
		}
	}
	plugins = append(plugins, pkg)
	root["plugin"] = plugins

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	wr, err := filemerge.WriteFileAtomic(path, out, 0o644)
	if err != nil {
		// WriteFileAtomic can publish the replacement and still return an error
		// (#1676), so report wr.Changed truthfully alongside the error instead
		// of pretending nothing happened.
		return wr.Changed, err
	}
	return wr.Changed, nil
}

// removeTUIPlugin is the uninstall-side mirror of ensureTUIPlugin. It removes
// every occurrence of pkg from tui.json's plugin[] list. It returns the exact
// replacement bytes without writing so the caller can perform a guarded write.
// If the file is missing or pkg is not present, it returns (false, nil, nil).
func removeTUIPlugin(path, pkg string) (bool, []byte, error) {
	root := map[string]any{"$schema": "https://opencode.ai/tui.json"}
	data, readErr := os.ReadFile(path)
	switch {
	case readErr == nil && len(bytes.TrimSpace(data)) > 0:
		if err := json.Unmarshal(data, &root); err != nil {
			return false, nil, fmt.Errorf("parse OpenCode TUI config %q: %w", path, err)
		}
	case readErr != nil && !os.IsNotExist(readErr):
		return false, nil, fmt.Errorf("read OpenCode TUI config %q: %w", path, readErr)
	}

	plugins := stringSlice(root["plugin"])
	kept := make([]string, 0, len(plugins))
	changedAny := false
	for _, existing := range plugins {
		if existing == pkg {
			changedAny = true
			continue
		}
		kept = append(kept, existing)
	}
	if !changedAny {
		return false, nil, nil
	}
	root["plugin"] = kept

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, nil, err
	}
	out = append(out, '\n')
	return true, out, nil
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
