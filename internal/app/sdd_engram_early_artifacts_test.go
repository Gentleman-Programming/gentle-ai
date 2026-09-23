package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeEarlyArtifactAppFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func installEarlyArtifactEngramExport(t *testing.T) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "engram-export.json")
	writeEarlyArtifactAppFile(t, fixture, `{"observations":[
  {"title":"sdd/early-artifacts/explore","content":"# Explore\n","project":"gentle-ai","scope":"project"},
  {"title":"sdd/early-artifacts/pre-proposal","content":"# Pre-proposal\n","project":"gentle-ai","scope":"project"}
]}
`, 0o644)

	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		writeEarlyArtifactAppFile(t, filepath.Join(binDir, "engram.cmd"), "@echo off\r\nif /I not \"%~1\"==\"export\" exit /b 2\r\nif \"%~2\"==\"\" exit /b 2\r\nif \"%ENGRAM_EARLY_ARTIFACT_EXPORT%\"==\"\" exit /b 2\r\ncopy /Y \"%ENGRAM_EARLY_ARTIFACT_EXPORT%\" \"%~2\" >nul\r\nexit /b %errorlevel%\r\n", 0o755)
	} else {
		writeEarlyArtifactAppFile(t, filepath.Join(binDir, "engram"), "#!/bin/sh\nif [ \"$1\" != \"export\" ] || [ -z \"$2\" ] || [ -z \"$ENGRAM_EARLY_ARTIFACT_EXPORT\" ]; then exit 2; fi\nexec cp \"$ENGRAM_EARLY_ARTIFACT_EXPORT\" \"$2\"\n", 0o755)
	}
	t.Setenv("ENGRAM_EARLY_ARTIFACT_EXPORT", fixture)
	t.Setenv("ENGRAM_PROJECT", "gentle-ai")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunArgsEngramEarlyArtifactsDiscoverAtPublicCommandBoundary(t *testing.T) {
	installEarlyArtifactEngramExport(t)
	setupMockHome(t, t.TempDir())

	root := t.TempDir()
	writeEarlyArtifactAppFile(t, filepath.Join(root, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n", 0o644)

	for _, command := range []string{"sdd-status", "sdd-continue"} {
		t.Run(command, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunArgs([]string{command, "--cwd", root, "--json"}, &output); err != nil {
				t.Fatalf("RunArgs(%s): %v", command, err)
			}
			var status struct {
				ChangeName      *string `json:"changeName"`
				NextRecommended string  `json:"nextRecommended"`
				Dependencies    struct {
					Proposal string `json:"proposal"`
				} `json:"dependencies"`
			}
			if err := json.Unmarshal(output.Bytes(), &status); err != nil {
				t.Fatalf("decode %s output: %v\n%s", command, err, output.String())
			}
			if status.ChangeName == nil || *status.ChangeName != "early-artifacts" || status.NextRecommended != "propose" || status.Dependencies.Proposal != "blocked" {
				t.Fatalf("%s status = change %v next %q proposal %q, want discovered early-artifacts routed to propose without proposal completion\n%s", command, status.ChangeName, status.NextRecommended, status.Dependencies.Proposal, strings.TrimSpace(output.String()))
			}
		})
	}
}
