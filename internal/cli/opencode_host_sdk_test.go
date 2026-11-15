package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenCodeHostSDKMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, version, manifest, lock, sdk string
		localRequired, unsupported         bool
	}{
		{name: "older host missing SDK", version: "2.0.24", localRequired: true},
		{name: "host capability boundary", version: "2.0.25"},
		{name: "current fresh host", version: "2.3.0"},
		{name: "current Bun manifest", version: "2.3.0", manifest: `{"packageManager":"bun@1.3.2"}`},
		{name: "current Bun text lock", version: "2.3.0", lock: "bun.lock"},
		{name: "current Bun binary lock", version: "2.3.0", lock: "bun.lockb"},
		{name: "older Bun owner", version: "2.0.24", manifest: `{"packageManager":"bun@1.3.2"}`, localRequired: true},
		{name: "older compatible SDK", version: "2.0.24", sdk: "2.0.4"},
		{name: "older newer compatible SDK", version: "2.0.24", sdk: "2.3.0"},
		{name: "older incompatible SDK", version: "2.0.24", sdk: "2.0.3", localRequired: true},
		{name: "V1 unchanged", version: "1.18.30"},
		{name: "unknown host", version: "not-a-version", unsupported: true},
		{name: "unsupported major", version: "3.0.0", unsupported: true},
		{name: "unverified prerelease", version: "2.3.0-rc.1", unsupported: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, config := sdkRangeConfig(t, `{}`, tc.sdk)
			// Runtime evidence must not implicitly materialize a local SDK.
			stubOpenCodeRuntimeVersionOnly(t, tc.version)
			if tc.manifest == "" {
				if err := os.Remove(filepath.Join(config, "package.json")); err != nil {
					t.Fatal(err)
				}
			} else {
				writeSDKRangeFile(t, filepath.Join(config, "package.json"), tc.manifest)
			}
			if tc.lock != "" {
				writeSDKRangeFile(t, filepath.Join(config, tc.lock), "user-owned lock marker")
			}
			writeSDKRangeFile(t, filepath.Join(config, "plugins", "user plugin.ts"), "// preserve user plugin\n")
			before := hostSDKTree(t, home)

			proposal, proposalErr := OpenCodeSDKInstallProposal(home)
			result, syncErr := RunSync([]string{"--agent", "opencode", "--dry-run"})
			switch {
			case tc.unsupported:
				if proposalErr == nil || !strings.Contains(proposalErr.Error(), "runtime version unavailable or unsupported") {
					t.Errorf("unsupported runtime proposal error = %v", proposalErr)
				}
				if syncErr != nil {
					t.Fatalf("dry-run error = %v", syncErr)
				}
				if !strings.Contains(RenderSyncReport(result), "Would stop: OpenCode runtime version unavailable or unsupported") {
					t.Errorf("missing unsupported-runtime refusal: %s", RenderSyncReport(result))
				}
			case tc.localRequired:
				if proposal == nil && proposalErr == nil {
					t.Error("older host no longer requires a local SDK")
				}
				if syncErr != nil {
					t.Fatalf("dry-run error = %v", syncErr)
				}
				if !strings.Contains(RenderSyncReport(result), "Would stop: OpenCode V2 requires installed @opencode/plugin@2.0.4") {
					t.Errorf("missing older-host SDK refusal: %s", RenderSyncReport(result))
				}
			default:
				if proposal != nil || proposalErr != nil {
					t.Errorf("unnecessary SDK proposal = %v, error = %v", proposal, proposalErr)
				}
				if syncErr != nil {
					t.Fatalf("dry-run error = %v", syncErr)
				}
				if strings.Contains(RenderSyncReport(result), "Would stop:") {
					t.Errorf("unexpected dry-run refusal: %s", RenderSyncReport(result))
				}
			}
			if after := hostSDKTree(t, home); !reflect.DeepEqual(before, after) {
				t.Errorf("SDK proposal/dry-run changed isolated state: before=%v after=%v", before, after)
			}
		})
	}
}

func hostSDKTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[relative] = "directory"
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[relative] = string(data)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
