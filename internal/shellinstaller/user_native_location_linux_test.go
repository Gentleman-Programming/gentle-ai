//go:build linux && amd64

package shellinstaller

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestNativeInstallationHandoff(t *testing.T) {
	root := "/private/stage"
	result := json.RawMessage(`{"outcome":"terminal-action-required","action":"open-new-terminal","completed":["shell-setup"],"retained":"original"}`)
	location := map[string]string{
		"packageRoot": root + "/pnpm/store/v11/links/@/gentle-pi/4.3.0/hash/node_modules/gentle-pi",
		"pnpmHome":    root + "/pnpm", "globalBin": root + "/pnpm/bin",
		"shellVersion": "4.3.0", "piVersion": "1.3.0",
	}
	encode := func(result json.RawMessage, location any) []byte {
		t.Helper()
		data, err := json.Marshal(map[string]any{"result": result, "installation": location})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Run("retains-original-result-and-adjacent-peer", func(t *testing.T) {
		data := encode(result, location)
		got, installed, err := decodeNativeInstallation(root, data)
		if err != nil || installed == nil || installed.PackageRoot != location["packageRoot"] || installed.PNPMHome != location["pnpmHome"] || installed.GlobalBin != location["globalBin"] || installed.ShellVersion != "4.3.0" || installed.PiVersion != "1.3.0" {
			t.Fatal(got, installed, err)
		}
		if string(got.Details) != string(result) || got.Outcome != "terminal-action-required" || got.Reason != "" || !reflect.DeepEqual(got.Completed, []string{"shell-setup"}) {
			t.Fatal("original result changed", got)
		}
		installed.PackageRoot = "/changed"
		got.Details[0] = '['
		again, copy, err := decodeNativeInstallation(root, data)
		if err != nil || copy.PackageRoot != location["packageRoot"] || string(again.Details) != string(result) {
			t.Fatal("returned state aliases the transport input", again, copy, err)
		}
	})
	t.Run("blocked-has-no-installation", func(t *testing.T) {
		blocked := json.RawMessage(`{"outcome":"blocked","reason":"consent-required","completed":[]}`)
		got, installed, err := decodeNativeInstallation(root, encode(blocked, nil))
		if err != nil || installed != nil || string(got.Details) != string(blocked) {
			t.Fatal(got, installed, err)
		}
	})
	for _, tt := range []struct{ name, key, value string }{
		{"foreign-home", "pnpmHome", "/personal/pnpm"},
		{"foreign-bin", "globalBin", root + "/bin"},
		{"foreign-package", "packageRoot", "/personal/gentle-pi"},
		{"store-root", "packageRoot", root + "/pnpm/store/v11/links"},
		{"traversal", "packageRoot", root + "/pnpm/store/v11/links/../../outside"},
		{"noncanonical", "packageRoot", root + "/pnpm/store/v11/links/a/../b"},
		{"empty-version", "shellVersion", ""},
		{"invalid-peer", "piVersion", "1.3.0\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bad := make(map[string]string, len(location))
			for key, value := range location {
				bad[key] = value
			}
			bad[tt.key] = tt.value
			_, _, err := decodeNativeInstallation(root, encode(result, bad))
			var failure *PrivateRuntimeError
			if !errors.As(err, &failure) || failure.Kind != "output" {
				t.Fatal("invalid handoff accepted", err)
			}
		})
	}
	for name, data := range map[string][]byte{
		"malformed":        []byte(`{`),
		"legacy-output":    result,
		"missing-callback": encode(result, nil),
		"blocked-callback": encode(json.RawMessage(`{"outcome":"blocked"}`), location),
		"wrong-terminal":   encode(json.RawMessage(`{"outcome":"terminal-action-required","action":"other"}`), location),
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeNativeInstallation(root, data); err == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
}

func TestRunWithLocationRejectsUnownedPlanBeforeExecution(t *testing.T) {
	worker := &NativeInstallerRuntime{}
	for _, consent := range []bool{false, true} {
		_, location, err := worker.RunWithLocation(context.Background(), NativeInstallerPlan{}, consent)
		var failure *PrivateRuntimeError
		if location != nil || !errors.As(err, &failure) || failure.Kind != "refused" {
			t.Fatal(location, err)
		}
	}
}
