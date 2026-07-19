package opencodedefault

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	check(t, err)
	return data
}
func TestOwnershipLifecycle(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "opencode.json")
	write := func(body string) { check(t, os.WriteFile(settings, []byte(body), 0o644)) }
	install := func() {
		plan, err := PrepareInstall(settings)
		check(t, err)
		_, err = plan.Apply()
		check(t, err)
	}
	uninstall := func() {
		plan, err := PrepareUninstall(settings)
		check(t, err)
		raw, err := os.ReadFile(settings)
		_, _, applyErr := plan.Apply(raw, err == nil)
		check(t, applyErr)
	}
	wantDefault := func(want string) {
		got := read(t, settings)
		if !bytes.Contains(got, []byte(`"default_agent": "`+want+`"`)) {
			t.Fatalf("default %q not restored: %s", want, got)
		}
	}
	write(`{"default_agent":"build","agent":{"gentle-orchestrator":{}},"profile":true}`)
	install()
	install()
	uninstall()
	wantDefault("build")
	write(`{"default_agent":"plan","profile":true}`)
	install()
	uninstall()
	wantDefault("plan")
	install()
	check(t, os.Remove(settings))
	write(`{"default_agent":"gentle-orchestrator","profile":true}`)
	install()
	uninstall()
	wantDefault("gentle-orchestrator")
	install()
	check(t, os.Remove(settings))
	uninstall()
	if _, err := os.Stat(OwnershipPath(settings)); !os.IsNotExist(err) {
		t.Fatalf("stale ownership remains: %v", err)
	}
	install()
	uninstall()
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Fatalf("fresh absence was not restored: %v", err)
	}
	write(`{"default_agent":"build"}`)
	before := read(t, settings)
	check(t, os.WriteFile(OwnershipPath(settings), []byte(`{"schema":"wrong"}`), 0o644))
	if _, err := PrepareUninstall(settings); err == nil {
		t.Fatal("malformed ownership was accepted")
	}
	if after := read(t, settings); !bytes.Equal(before, after) {
		t.Fatalf("settings changed: %q", after)
	}
}

func TestUninstallWithoutOwnershipHandlesDefaultAgent(t *testing.T) {
	for _, tt := range []struct {
		name         string
		defaultAgent string
		wantPresent  bool
	}{
		{name: "managed default is removed", defaultAgent: ManagedAgent},
		{name: "user default is preserved", defaultAgent: "build", wantPresent: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			settings := filepath.Join(t.TempDir(), "opencode.json")
			original := `{"default_agent":"` + tt.defaultAgent + `","agent":{"gentle-orchestrator":{}},"profile":true}`
			check(t, os.WriteFile(settings, []byte(original), 0o644))
			plan, err := PrepareUninstall(settings)
			check(t, err)

			cleaned := []byte(`{"default_agent":"` + tt.defaultAgent + `","profile":true}`)
			_, _, err = plan.Apply(cleaned, true)
			check(t, err)

			got := read(t, settings)
			present := bytes.Contains(got, []byte(`"default_agent"`))
			if present != tt.wantPresent || present && !bytes.Contains(got, []byte(`"default_agent": "`+tt.defaultAgent+`"`)) {
				t.Fatalf("settings = %s", got)
			}
		})
	}
}
