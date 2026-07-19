package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantCode   int
		wantOutput []string
	}{
		{name: "clean", files: map[string]string{"clean.go": "package clean\n"}},
		{
			name: "lists every malformed path",
			files: map[string]string{
				"first.go":         "package first\n\nvar x=1\n",
				"nested/second.go": "package second\n\nvar y=2\n",
			},
			wantCode:   1,
			wantOutput: []string{"first.go", "nested/second.go"},
		},
		{
			name:       "parse error is nonzero and reported",
			files:      map[string]string{"broken.go": "package broken\nfunc nope(\n"},
			wantCode:   1,
			wantOutput: []string{"broken.go", "expected"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			runCommand(t, root, "git", "init", "--quiet")
			for name, content := range tt.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runCommand(t, root, "git", "add", ".")

			var stdout, stderr bytes.Buffer
			if got := run(root, &stdout, &stderr); got != tt.wantCode {
				t.Fatalf("run() = %d, want %d; stdout=%q stderr=%q", got, tt.wantCode, stdout.String(), stderr.String())
			}
			output := filepath.ToSlash(stdout.String() + stderr.String())
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("output %q does not contain %q", output, want)
				}
			}
			for name, content := range tt.files {
				got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != content {
					t.Errorf("%s was modified", name)
				}
			}
		})
	}
}

func runCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s failed: %v: %s", name, err, output)
	}
}
