package opencodedefault

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLegacyUninstallOwnership(t *testing.T) {
	for _, tt := range []struct {
		name, current, previousState, previousDefault, want string
		wantDefault, removeRecord                           bool
	}{
		{name: "owned previous default restored", current: ManagedAgent, previousState: "value", previousDefault: "build", want: "build", wantDefault: true, removeRecord: true},
		{name: "owned absent default removed", current: ManagedAgent, previousState: "absent", removeRecord: true},
		{name: "user modified default and metadata preserved", current: "user-agent", previousState: "value", previousDefault: "build", want: "user-agent", wantDefault: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			settings := filepath.Join(t.TempDir(), "opencode.json")
			original := []byte(`{"default_agent":"` + tt.current + `","unrelated":true}`)
			if err := os.WriteFile(settings, original, 0600); err != nil {
				t.Fatal(err)
			}
			ownerPath := OwnershipPath(settings)
			metadata := []byte(`{"schema":"gentle-ai.opencode-default-agent","version":1,"state":"managed","previous_state":"` + tt.previousState + `","previous_default":"` + tt.previousDefault + `"}`)
			if err := os.WriteFile(ownerPath, metadata, 0600); err != nil {
				t.Fatal(err)
			}
			plan, err := PrepareUninstall(settings)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := plan.Apply(original, true); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(body, &root); err != nil {
				t.Fatal(err)
			}
			value, present := root["default_agent"]
			if present != tt.wantDefault || present && value != tt.want || root["unrelated"] != true {
				t.Fatalf("unexpected settings: %s", body)
			}
			info, err := os.Stat(settings)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Fatalf("settings permissions changed: %v", info.Mode().Perm())
			}
			_, err = os.Stat(ownerPath)
			if tt.removeRecord && !os.IsNotExist(err) || !tt.removeRecord && err != nil {
				t.Fatalf("ownership record existence mismatch: %v", err)
			}
		})
	}
}

func TestUninstallWithoutOwnershipPreservesDefault(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	original := []byte(`{"default_agent":"gentle-orchestrator","unrelated":true}`)
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareUninstall(settings)
	if err != nil {
		t.Fatal(err)
	}
	changed, removed, err := plan.Apply(original, true)
	if err != nil || changed || removed {
		t.Fatalf("unowned default changed: changed=%v removed=%v err=%v", changed, removed, err)
	}
	body, err := os.ReadFile(settings)
	if err != nil || !bytes.Equal(body, original) {
		t.Fatalf("unowned settings changed: %q, %v", body, err)
	}
}

func TestMalformedLegacyOwnershipRefusesUninstall(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	original := []byte(`{"default_agent":"gentle-orchestrator"}`)
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(OwnershipPath(settings), []byte(`{"schema":"wrong"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareUninstall(settings); err == nil {
		t.Fatal("malformed ownership accepted")
	}
	body, err := os.ReadFile(settings)
	if err != nil || !bytes.Equal(body, original) {
		t.Fatalf("settings changed: %q, %v", body, err)
	}
}

// TestInstallOwnershipLifecycle restores the v3.7.0 install contract: install
// always manages default_agent, records the replaced value once, and uninstall
// hands it back.
func TestInstallOwnershipLifecycle(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	write := func(body string) { check(os.WriteFile(settings, []byte(body), 0o644)) }
	read := func() []byte {
		t.Helper()
		data, err := os.ReadFile(settings)
		check(err)
		return data
	}
	install := func() {
		t.Helper()
		plan, err := PrepareInstall(settings)
		check(err)
		_, err = plan.Apply()
		check(err)
	}
	uninstall := func() {
		t.Helper()
		plan, err := PrepareUninstall(settings)
		check(err)
		raw, err := os.ReadFile(settings)
		_, _, applyErr := plan.Apply(raw, err == nil)
		check(applyErr)
	}
	wantDefault := func(want string) {
		t.Helper()
		if got := read(); !bytes.Contains(got, []byte(`"default_agent": "`+want+`"`)) {
			t.Fatalf("default %q not present: %s", want, got)
		}
	}

	write(`{"default_agent":"build","agent":{"gentle-orchestrator":{}},"profile":true}`)
	install()
	wantDefault(ManagedAgent)
	first := read()
	install()
	if !bytes.Equal(first, read()) {
		t.Fatal("repeated install changed settings")
	}
	uninstall()
	wantDefault("build")

	write(`{"default_agent":"plan","profile":true}`)
	install()
	uninstall()
	wantDefault("plan")

	write(`{"profile":true}`)
	install()
	uninstall()
	if bytes.Contains(read(), []byte(`"default_agent"`)) {
		t.Fatalf("absent default was not restored: %s", read())
	}
	if _, err := os.Stat(OwnershipPath(settings)); !os.IsNotExist(err) {
		t.Fatalf("ownership record remains after uninstall: %v", err)
	}
}

func TestApplyShareDefault(t *testing.T) {
	for _, tt := range []struct {
		name, body, want string
		wantChanged      bool
	}{
		{name: "absent is disabled", body: `{"profile":true}`, want: "disabled", wantChanged: true},
		{name: "user choice is kept", body: `{"share":"manual"}`, want: "manual"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			settings := filepath.Join(t.TempDir(), "opencode.json")
			if err := os.WriteFile(settings, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			changed, err := ApplyShareDefault(settings)
			if err != nil || changed != tt.wantChanged {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			var root map[string]any
			raw, _ := os.ReadFile(settings)
			if err := json.Unmarshal(raw, &root); err != nil || root["share"] != tt.want {
				t.Fatalf("share = %v (%s), want %s", root["share"], raw, tt.want)
			}
			if changed, err := ApplyShareDefault(settings); err != nil || changed {
				t.Fatalf("second apply changed=%v err=%v", changed, err)
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "opencode.json")
	if changed, err := ApplyShareDefault(missing); err != nil || changed {
		t.Fatalf("missing settings: changed=%v err=%v", changed, err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("share default created a settings file")
	}
}

func TestApplyShareDefaultPreservesPrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	settings := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(settings, []byte(`{"provider":{"example":{"options":{"apiKey":"secret"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(settings, 0o600); err != nil {
		t.Fatal(err)
	}
	if changed, err := ApplyShareDefault(settings); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	info, err := os.Stat(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("settings mode = %v, want 0600", got)
	}
}
