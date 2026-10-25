//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

// NativeInstallationLocation is the original installer's successful fresh-release
// locator. It is not a manifest, filesystem authentication or publication proof.
// PiVersion describes the adjacent peer, not the independently installed CLI.
type NativeInstallationLocation struct {
	PackageRoot  string `json:"packageRoot"`
	PNPMHome     string `json:"pnpmHome"`
	GlobalBin    string `json:"globalBin"`
	ShellVersion string `json:"shellVersion"`
	PiVersion    string `json:"piVersion"`
}

// RunWithLocation opts into the original runner's verified-installation callback.
// The caller owns confinement and physical consent. Prepared plans require consent
// before invoking the runner; the result and installation locator remain separate.
func (w *NativeInstallerRuntime) RunWithLocation(ctx context.Context, plan NativeInstallerPlan, consent bool) (NativeInstallerResult, *NativeInstallationLocation, error) {
	if plan.owner != w || len(plan.data) == 0 || (len(plan.bootstrapEnv) != 0 && !consent) {
		return NativeInstallerResult{}, nil, privateError("refused", nil)
	}
	data, err := w.invokeWithEnvironment(ctx, "run-with-location", plan.JSON(), consent, plan.bootstrapEnv)
	if err != nil {
		return NativeInstallerResult{}, nil, err
	}
	return decodeNativeInstallation(w.root, data)
}

func decodeNativeInstallation(root string, data []byte) (NativeInstallerResult, *NativeInstallationLocation, error) {
	var envelope struct {
		Result       json.RawMessage             `json:"result"`
		Installation *NativeInstallationLocation `json:"installation"`
	}
	var result struct {
		NativeInstallerResult
		Action string `json:"action"`
	}
	refuse := func() (NativeInstallerResult, *NativeInstallationLocation, error) {
		return NativeInstallerResult{}, nil, privateError("output", nil)
	}
	if json.Unmarshal(data, &envelope) != nil || json.Unmarshal(envelope.Result, &result) != nil || result.Outcome == "" {
		return refuse()
	}
	location := envelope.Installation
	success := result.Outcome == "terminal-action-required" && result.Action == "open-new-terminal"
	if success != (location != nil) {
		return refuse()
	}
	if location != nil {
		// Validate only the locator's shape and private namespace. The publishing
		// boundary must independently authenticate physical paths and all bytes.
		home := filepath.Join(root, "pnpm")
		store := filepath.Join(home, "store/v11/links") + string(filepath.Separator)
		if !privateHierarchyPath(root) || !privateHierarchyPath(location.PackageRoot) ||
			location.PNPMHome != home || location.GlobalBin != filepath.Join(home, "bin") ||
			!strings.HasPrefix(location.PackageRoot, store) || filepath.Base(location.PackageRoot) != "gentle-pi" ||
			filepath.Base(filepath.Dir(location.PackageRoot)) != "node_modules" {
			return refuse()
		}
		for _, version := range []string{location.ShellVersion, location.PiVersion} {
			if version == "" || len(version) > 128 || strings.ContainsAny(version, "\x00\r\n\t ") {
				return refuse()
			}
		}
	}
	result.Details = append(json.RawMessage(nil), envelope.Result...)
	return result.NativeInstallerResult, location, nil
}
