package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProvisionEngramMCPRepairsDuplicateDeclarations(t *testing.T) {
	for _, tt := range []struct {
		name     string
		packages string
		want     string
		changed  bool
	}{
		{"historical pair", `["npm:gentle-engram","npm:gentle-engram@0.1.16"]`, `["npm:gentle-engram@0.1.16"]`, true},
		{"reverse order", `["npm:gentle-engram@0.1.16","npm:gentle-engram"]`, `["npm:gentle-engram@0.1.16"]`, true},
		{"identical bare entries", `["npm:gentle-engram","npm:gentle-engram"]`, `["npm:gentle-engram"]`, true},
		{"identical pinned entries", `["npm:gentle-engram@0.1.16","npm:gentle-engram@0.1.16"]`, `["npm:gentle-engram@0.1.16"]`, true},
		{"preserve object options", `["npm:gentle-engram",{"source":"npm:gentle-engram@0.1.16","extensions":["index.ts"],"skills":[]}]`, `[{"source":"npm:gentle-engram@0.1.16","extensions":["index.ts"],"skills":[]}]`, true},
		{"preserve unversioned object", `["npm:gentle-engram",{"source":"npm:gentle-engram","extensions":[]}]`, `[{"source":"npm:gentle-engram","extensions":[]}]`, true},
		{"identical objects", `[{"source":"npm:gentle-engram@0.1.16","extensions":[]},{"source":"npm:gentle-engram@0.1.16","extensions":[]}]`, `[{"source":"npm:gentle-engram@0.1.16","extensions":[]}]`, true},
		{"conflicting pins", `["npm:gentle-engram","npm:gentle-engram@0.1.16","npm:gentle-engram@0.2.0"]`, `["npm:gentle-engram","npm:gentle-engram@0.1.16","npm:gentle-engram@0.2.0"]`, false},
		{"conflicting options", `[{"source":"npm:gentle-engram@0.1.16","extensions":[]},{"source":"npm:gentle-engram@0.1.16","extensions":["index.ts"]}]`, `[{"source":"npm:gentle-engram@0.1.16","extensions":[]},{"source":"npm:gentle-engram@0.1.16","extensions":["index.ts"]}]`, false},
		{"single pin", `["npm:gentle-engram@0.1.16"]`, `["npm:gentle-engram@0.1.16"]`, false},
		{"unrelated duplicates", `["npm:other","npm:other","npm:gentle-engram-helper@1.0.0"]`, `["npm:other","npm:other","npm:gentle-engram-helper@1.0.0"]`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			a := NewAdapter()
			path := a.SettingsPath(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			initial := []byte(`{"theme":"kanagawa","custom":{"enabled":true},"packages":` + tt.packages + `}`)
			if err := os.WriteFile(path, initial, 0o644); err != nil {
				t.Fatal(err)
			}
			changed, paths, err := a.ProvisionEngramMCP(home)
			if err != nil {
				t.Fatal(err)
			}
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v", changed, tt.changed)
			}
			var wantPaths []string
			if tt.changed {
				wantPaths = []string{path}
			}
			if !reflect.DeepEqual(paths, wantPaths) {
				t.Fatalf("paths = %v, want %v", paths, wantPaths)
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(actual, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(`{"theme":"kanagawa","custom":{"enabled":true},"packages":`+tt.want+`}`), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("settings = %s, want packages %s with other settings preserved", actual, tt.want)
			}
			if !tt.changed && string(actual) != string(initial) {
				t.Fatal("unchanged settings were rewritten")
			}
			changed, paths, err = a.ProvisionEngramMCP(home)
			if err != nil || changed || len(paths) != 0 {
				t.Fatalf("second provision = %v, %v, %v, want no change", changed, paths, err)
			}
			again, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(again) != string(actual) {
				t.Fatal("second provision rewrote settings")
			}
			if _, err := os.Stat(a.MCPConfigPath(home, "")); !os.IsNotExist(err) {
				t.Fatalf("native Engram created MCP config: %v", err)
			}
		})
	}
}
