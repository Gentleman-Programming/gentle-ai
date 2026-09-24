package pi

import (
	"errors"
	"net/url"
	"path/filepath"
	"testing"
)

func rootSelection(base, source string, scope PackageSourceScope) PackageSourceSelection {
	project, agent := filepath.Join(base, "project"), filepath.Join(base, "agent")
	settings := filepath.Join(agent, "settings.json")
	if scope == SettingsScopeProject {
		settings = filepath.Join(project, ".pi", "settings.json")
	}
	return PackageSourceSelection{Source: source, Scope: scope, SettingsPath: settings, CWD: project, AgentDir: agent}
}
func assertRoot(t *testing.T, selection PackageSourceSelection, want string) {
	t.Helper()
	before := selection
	tree := snapshotPiTree(t, filepath.Dir(selection.CWD))
	got, err := ResolvePackageRoot(selection)
	if err != nil || got != (PackageRoot{Path: want, Source: selection.Source, Scope: selection.Scope}) || selection != before || snapshotPiTree(t, filepath.Dir(selection.CWD)) != tree {
		t.Fatalf("root = %#v, err = %v, selection = %#v; want %q and unchanged selection", got, err, selection, want)
	}
}
func wantSourceError(t *testing.T, err error, source, kind string, sentinel error) *SourceError {
	t.Helper()
	var typed *SourceError
	if !errors.As(err, &typed) || typed.Source != source || typed.Kind != kind || typed.Cause == nil || !errors.Is(err, sentinel) {
		t.Fatalf("error = %T %v; want SourceError source=%q kind=%q and errors.Is(..., %v)", err, err, source, kind, sentinel)
	}
	return typed
}
func localFileURL(path, host string) string {
	value := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		value = "/" + value
	}
	return (&url.URL{Scheme: "file", Host: host, Path: value}).String()
}

func TestResolvePackageRootNPMAndLocal(t *testing.T) {
	base := t.TempDir()
	npm := []struct {
		name, source, packagePath string
		scope                     PackageSourceScope
	}{
		{"bare unscoped", "gentle-pi", "gentle-pi", SettingsScopeProject},
		{"versioned npm", "npm:gentle-pi@1.2.3", "gentle-pi", SettingsScopeProject},
		{"bare scoped", "@team/gentle-pi", "@team/gentle-pi", SettingsScopeUser},
		{"versioned scoped range", "npm:@team/gentle-pi@^1.2", "@team/gentle-pi", SettingsScopeUser},
	}
	for _, tt := range npm {
		t.Run(tt.name, func(t *testing.T) {
			s := rootSelection(base, tt.source, tt.scope)
			managed := s.AgentDir
			if tt.scope == SettingsScopeProject {
				managed = filepath.Join(s.CWD, ".pi")
			}
			install := filepath.Join(managed, "npm", "node_modules")
			assertRoot(t, s, filepath.Join(install, filepath.FromSlash(tt.packagePath)))
			if !pathWithin(install, filepath.Join(install, filepath.FromSlash(tt.packagePath))) {
				t.Fatal("npm root escaped managed root")
			}
		})
	}
	project := filepath.Join(base, "project")
	outside, fileRoot := filepath.Join(base, "outside", "gentle-pi"), filepath.Join(base, "file", "gentle pi")
	local := []struct {
		name, source, want string
		scope              PackageSourceScope
	}{
		{"settings-relative", "./packages/gentle-pi", filepath.Join(project, ".pi", "packages", "gentle-pi"), SettingsScopeProject},
		{"absolute", outside, outside, SettingsScopeUser},
		{"file-empty-authority", localFileURL(fileRoot, ""), fileRoot, SettingsScopeProject},
		{"file-localhost-authority", localFileURL(outside, "localhost"), outside, SettingsScopeUser},
	}
	for _, tt := range local {
		t.Run(tt.name, func(t *testing.T) { assertRoot(t, rootSelection(base, tt.source, tt.scope), tt.want) })
	}
}

func TestResolvePackageRootRejectsGrammarAndLocalAmbiguity(t *testing.T) {
	base := t.TempDir()
	file := localFileURL(filepath.Join(base, "pkg"), "")
	cases := []struct {
		name, source, kind string
		decode             bool
	}{
		{"empty", "", "empty-source", false}, {"npm empty", "npm:", "invalid-npm", false},
		{"empty version", "npm:gentle-pi@", "empty-npm-version", false}, {"uppercase name", "npm:Gentle-pi", "unsafe-npm-name", false},
		{"name punctuation", "npm:gentle-pi!", "unsafe-npm-name", false}, {"unsafe range", "npm:gentle-pi@>=1.0.0", "unsafe-npm-version", false},
		{"dot name", "npm:@team/../gentle-pi", "unsafe-npm-name", false}, {"query", "npm:gentle-pi?query", "invalid-npm", false},
		{"relative parent", "./packages/../gentle-pi", "unsafe-local", false}, {"relative dot", "./packages/./gentle-pi", "unsafe-local", false},
		{"encoded traversal", "./packages/%2e%2e/gentle-pi", "unsafe-local", false}, {"ambiguous relative", "packages/gentle-pi", "invalid-local", false},
		{"file remote host", "file://remote/tmp/pkg", "invalid-file-url", false}, {"file query", file + "?query", "invalid-file-url", false},
		{"file encoded separator", file + "%2Fchild", "unsafe-file-url", false}, {"file malformed escape", file + "%zz", "invalid-file-url", true},
		{"scp", "git@github.com:team/gentle-pi", "unsupported-source", false}, {"host shorthand", "github.com/team/gentle-pi", "unsupported-source", false}, {"git+ssh", "git+ssh://git@github.com/team/gentle-pi", "unsupported-source", false}, {"localhost shorthand", "localhost/team/gentle-pi", "unsupported-source", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := rootSelection(base, tt.source, SettingsScopeProject)
			before := s
			tree := snapshotPiTree(t, filepath.Dir(s.CWD))
			_, err := ResolvePackageRoot(s)
			if s != before || snapshotPiTree(t, filepath.Dir(s.CWD)) != tree {
				t.Fatal("selection or filesystem changed on failure")
			}
			sentinel := ErrInvalidPackageSource
			if tt.kind == "unsupported-source" {
				sentinel = ErrUnsupportedPackageSource
			}
			typed := wantSourceError(t, err, tt.source, tt.kind, sentinel)
			if tt.decode {
				var escape url.EscapeError
				if !errors.As(typed, &escape) {
					t.Fatalf("error = %v; want underlying URL escape cause", typed)
				}
			}
		})
	}
}

func TestResolvePackageRootRejectsInconsistentSelections(t *testing.T) {
	base := t.TempDir()
	cases := []struct {
		name, kind string
		change     func(*PackageSourceSelection)
	}{
		{"project settings outside", "invalid-selection-settings", func(s *PackageSourceSelection) { s.SettingsPath = filepath.Join(base, "other", "settings.json") }},
		{"user settings outside agent", "invalid-selection-settings", func(s *PackageSourceSelection) {
			s.Scope, s.SettingsPath = SettingsScopeUser, filepath.Join(base, "project", ".pi", "settings.json")
		}},
		{"relative cwd", "invalid-selection-path", func(s *PackageSourceSelection) { s.CWD = "project" }},
		{"unclean agent", "invalid-selection-path", func(s *PackageSourceSelection) { s.AgentDir += string(filepath.Separator) + "." }},
		{"invalid scope", "invalid-scope", func(s *PackageSourceSelection) { s.Scope = SettingsScopeNone }},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := rootSelection(base, "npm:gentle-pi", SettingsScopeProject)
			tt.change(&s)
			_, err := ResolvePackageRoot(s)
			wantSourceError(t, err, s.Source, tt.kind, ErrInvalidPackageSource)
		})
	}
}
