package pi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePiSettings(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSelectPackageSourcePrecedenceAndFallback(t *testing.T) {
	cases := []struct {
		name, project, user, source string
		projectSet, userSet         bool
		scope                       PackageSourceScope
		wantErr                     bool
	}{
		{"project wins", `{"packages":["npm:gentle-pi@project"]}`, `{"packages":["npm:gentle-pi@user"]}`, "npm:gentle-pi@project", true, true, SettingsScopeProject, false},
		{"missing project", "", `{"packages":["npm:gentle-pi@user"]}`, "npm:gentle-pi@user", false, true, SettingsScopeUser, false},
		{"empty project", `{"packages":[]}`, `{"packages":["npm:gentle-pi@user"]}`, "npm:gentle-pi@user", true, true, SettingsScopeUser, false},
		{"nonmatching project", `{"packages":["npm:other"]}`, `{"packages":["npm:gentle-pi@user"]}`, "npm:gentle-pi@user", true, true, SettingsScopeUser, false},
		{"object source plus name", `{"packages":[{"source":"git:opaque/repository","name":"gentle-pi"}]}`, "", "git:opaque/repository", true, false, SettingsScopeProject, false},
		{"bare near miss", `{"packages":["gentle-pi-extra"]}`, "", "", true, false, SettingsScopeNone, true},
		{"npm near miss", `{"packages":["npm:gentle-pi-extra"]}`, "", "", true, false, SettingsScopeNone, true},
		{"npm suffix near miss", `{"packages":["npm:gentle-pi2"]}`, "", "", true, false, SettingsScopeNone, true},
		{"scoped near miss", `{"packages":["npm:@gentle-pi/core"]}`, "", "", true, false, SettingsScopeNone, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			cwd, agent := filepath.Join(base, "project"), filepath.Join(base, "agent")
			projectPath, userPath := filepath.Join(cwd, ".pi", "settings.json"), filepath.Join(agent, "settings.json")
			if tt.projectSet {
				writePiSettings(t, projectPath, tt.project)
			}
			if tt.userSet {
				writePiSettings(t, userPath, tt.user)
			}
			got, err := SelectPackageSource(cwd, agent)
			if tt.wantErr {
				var notConfigured *NotConfiguredError
				if !errors.As(err, &notConfigured) || !errors.Is(err, ErrPackageNotConfigured) {
					t.Fatalf("selection = %#v, error = %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			path := userPath
			if tt.scope == SettingsScopeProject {
				path = projectPath
			}
			if got.Source != tt.source || got.Scope != tt.scope || got.SettingsPath != path || got.CWD != cwd || got.AgentDir != agent {
				t.Fatalf("selection = %#v; want %q/%q/%q", got, tt.source, tt.scope, path)
			}
		})
	}
}
func TestSelectPackageSourceRejectsInvalidSettingsWithoutFallback(t *testing.T) {
	cases := []struct {
		name, body string
		scope      PackageSourceScope
		project    bool
	}{
		{"null name", `{"packages":[{"source":"git:opaque","name":null}]}`, SettingsScopeProject, true},
		{"number name", `{"packages":[{"source":"git:opaque","name":1}]}`, SettingsScopeProject, true},
		{"bool name", `{"packages":[{"source":"git:opaque","name":true}]}`, SettingsScopeProject, true},
		{"array name", `{"packages":[{"source":"git:opaque","name":[]}]}`, SettingsScopeProject, true},
		{"object name", `{"packages":[{"source":"git:opaque","name":{}}]}`, SettingsScopeProject, true},
		{"duplicate name", `{"packages":[{"source":"git:opaque","name":"other","name":"gentle-pi"}]}`, SettingsScopeProject, true},
		{"malformed user", `{`, SettingsScopeUser, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			cwd, agent := filepath.Join(base, "project"), filepath.Join(base, "agent")
			path := filepath.Join(agent, "settings.json")
			if tt.project {
				path = filepath.Join(cwd, ".pi", "settings.json")
				writePiSettings(t, filepath.Join(agent, "settings.json"), `{"packages":["npm:gentle-pi@fallback"]}`)
			}
			writePiSettings(t, path, tt.body)
			_, err := SelectPackageSource(cwd, agent)
			var typed *SettingsError
			if !errors.As(err, &typed) || typed.Scope != tt.scope || typed.Path != path || errors.Is(err, ErrPackageNotConfigured) {
				t.Fatalf("error = %T %v; want invalid %s settings", err, err, tt.scope)
			}
		})
	}
}
func TestSelectPackageSourceResolvesAgentDirectoryConvention(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "project")
	cases := []struct {
		name, input, want string
		setup             func(*testing.T)
	}{
		{"explicit", filepath.Join(base, "explicit-agent"), filepath.Join(base, "explicit-agent"), func(*testing.T) {}},
		{"environment", "", filepath.Join(base, "env-agent"), func(t *testing.T) { t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(base, "env-agent")) }},
		{"default", "", filepath.Join(base, ".pi", "agent"), func(t *testing.T) { t.Setenv("PI_CODING_AGENT_DIR", ""); t.Setenv("HOME", base) }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			path := filepath.Join(tt.want, "settings.json")
			writePiSettings(t, path, `{"packages":["npm:gentle-pi"]}`)
			got, err := SelectPackageSource(cwd, tt.input)
			if err != nil || got.AgentDir != tt.want || got.SettingsPath != path {
				t.Fatalf("selection = %#v, error = %v; want agent %q", got, err, tt.want)
			}
		})
	}
}
func TestSelectPackageSourceRejectsRelativeRoots(t *testing.T) {
	base := t.TempDir()
	cases := []struct{ name, cwd, agent, env string }{
		{"cwd", "project", filepath.Join(base, "agent"), ""},
		{"explicit agent", filepath.Join(base, "project"), "agent", ""},
		{"environment agent", filepath.Join(base, "project"), "", "agent"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PI_CODING_AGENT_DIR", tt.env)
			_, err := SelectPackageSource(tt.cwd, tt.agent)
			var typed *SettingsError
			if !errors.As(err, &typed) || !errors.Is(err, ErrInvalidPiRoot) || typed.Scope != SettingsScopeNone {
				t.Fatalf("error = %T %v; want invalid-root input error", err, err)
			}
		})
	}
}
func TestSelectPackageSourceIsReadOnlyOnSuccessAndFailure(t *testing.T) {
	cases := []struct {
		name, project, user          string
		projectSet, userSet, wantErr bool
	}{
		{"success", `{"packages":["npm:gentle-pi"]}`, "", true, false, false},
		{"malformed project", `{`, `{"packages":["npm:gentle-pi"]}`, true, true, true},
		{"not configured", "", "", false, false, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			cwd, agent := filepath.Join(base, "project"), filepath.Join(base, "agent")
			if tt.projectSet {
				writePiSettings(t, filepath.Join(cwd, ".pi", "settings.json"), tt.project)
			}
			if tt.userSet {
				writePiSettings(t, filepath.Join(agent, "settings.json"), tt.user)
			}
			before := snapshotPiTree(t, base)
			got, err := SelectPackageSource(cwd, agent)
			if (err != nil) != tt.wantErr || snapshotPiTree(t, base) != before {
				t.Fatalf("error = %v or filesystem changed", err)
			}
			if tt.name == "not configured" {
				var typed *NotConfiguredError
				if !errors.As(err, &typed) || !errors.Is(err, ErrPackageNotConfigured) || got.Scope != SettingsScopeNone || typed.ProjectPath == "" || typed.UserPath == "" || !strings.Contains(err.Error(), typed.UserPath) {
					t.Fatalf("not-configured provenance = %#v / %v", got, err)
				}
			}
		})
	}
}

func snapshotPiTree(t *testing.T, root string) string {
	var entries []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		entry := fmt.Sprintf("%s|%o", path, info.Mode().Perm())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			entry += "|" + string(data)
		}
		entries = append(entries, entry)
		return nil
	})
	return strings.Join(entries, "\n")
}
