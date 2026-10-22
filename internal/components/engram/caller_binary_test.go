package engram

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/system"
)

func TestValidateCallerBinary(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "engram")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name    string
		value   string
		wantErr string
		posix   bool
	}{
		{name: "unset", value: ""},
		{name: "absolute executable", value: executable},
		{name: "relative path", value: "bin/engram", wantErr: "absolute"},
		{name: "missing file", value: filepath.Join(dir, "missing"), wantErr: "missing"},
		{name: "directory", value: dir, wantErr: "not a regular file"},
		{name: "not executable", value: plain, wantErr: "not executable", posix: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.posix && runtime.GOOS == "windows" {
				t.Skip("Windows has no executable bit")
			}
			t.Setenv(system.EngramBinEnv, tt.value)
			err := ValidateCallerBinary()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateCallerBinary() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), system.EngramBinEnv) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateCallerBinary() = %v, want an error naming %s and %q", err, system.EngramBinEnv, tt.wantErr)
			}
		})
	}
}

func TestCallerBinaryReplacesPathLookups(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "engram")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	SetLookPathForTest(t, "", "engram not on PATH")
	restore := lookPath
	t.Cleanup(func() { lookPath = restore })
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }

	t.Setenv(system.EngramBinEnv, "")
	if err := VerifyInstalled(); err == nil {
		t.Fatal("VerifyInstalled() without engram on PATH = nil, want the existing PATH error")
	}
	if cmd, found := resolveEngramCommand(); cmd != "engram" || found {
		t.Fatalf("resolveEngramCommand() default = %q, %v", cmd, found)
	}

	t.Setenv(system.EngramBinEnv, bin)
	if err := VerifyInstalled(); err != nil {
		t.Fatalf("VerifyInstalled() with %s = %v", system.EngramBinEnv, err)
	}
	if cmd, found := resolveEngramCommand(); cmd != bin || !found {
		t.Fatalf("resolveEngramCommand() = %q, %v; want %q", cmd, found, bin)
	}
	if cmd := preferredStableEngramCommand(); cmd != bin {
		t.Fatalf("preferredStableEngramCommand() = %q, want %q", cmd, bin)
	}
}
