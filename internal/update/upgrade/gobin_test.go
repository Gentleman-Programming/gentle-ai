package upgrade

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// stubExePath overrides the executable path used by go-install detection.
func stubExePath(t *testing.T, exePath string, exeErr error) {
	t.Helper()
	orig := osExecutableFn
	t.Cleanup(func() { osExecutableFn = orig })
	osExecutableFn = func() (string, error) { return exePath, exeErr }
}

// stubGoEnv overrides `go env` lookups used by go-install detection.
func stubGoEnv(t *testing.T, values map[string]string, err error) {
	t.Helper()
	orig := goEnvFn
	t.Cleanup(func() { goEnvFn = orig })
	goEnvFn = func(key string) (string, error) {
		if err != nil {
			return "", err
		}
		return values[key], nil
	}
}

// TestRunningFromGoBinDir verifies that detection of a go-installed gentle-ai
// binary covers every way Go resolves its binary directory: GOBIN (env or
// `go env -w`), multi-entry GOPATH, and the ~/go/bin default — and that it
// never misfires for binaries installed elsewhere (issue #999).
func TestRunningFromGoBinDir(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name        string
		exePath     string
		exeErr      error
		gobinEnv    string
		gopathEnv   string
		goAvailable bool
		goEnv       map[string]string
		goEnvErr    error
		want        bool
	}{
		{
			name:     "exe in GOBIN env dir",
			exePath:  filepath.Join("/custom", "gobin", "gentle-ai"),
			gobinEnv: filepath.Join("/custom", "gobin"),
			want:     true,
		},
		{
			name:      "exe in second entry of multi-entry GOPATH",
			exePath:   filepath.Join("/two", "bin", "gentle-ai"),
			gopathEnv: strings.Join([]string{"/one", "/two"}, string(filepath.ListSeparator)),
			want:      true,
		},
		{
			name:        "exe in go env GOBIN (set via go env -w, not process env)",
			exePath:     filepath.Join("/from-go-env", "gobin", "gentle-ai"),
			goAvailable: true,
			goEnv:       map[string]string{"GOBIN": filepath.Join("/from-go-env", "gobin")},
			want:        true,
		},
		{
			name:        "exe in go env GOPATH bin",
			exePath:     filepath.Join("/from-go-env", "gopath", "bin", "gentle-ai"),
			goAvailable: true,
			goEnv:       map[string]string{"GOPATH": filepath.Join("/from-go-env", "gopath")},
			want:        true,
		},
		{
			name:     "exe in default home go bin with go unavailable",
			exePath:  filepath.Join(home, "go", "bin", "gentle-ai"),
			goEnvErr: errors.New("go not found"),
			want:     true,
		},
		{
			name:        "go env errors fall back to process env",
			exePath:     filepath.Join("/env", "gobin", "gentle-ai"),
			gobinEnv:    filepath.Join("/env", "gobin"),
			goAvailable: true,
			goEnvErr:    errors.New("go env failed"),
			want:        true,
		},
		{
			name:        "exe outside every go bin dir",
			exePath:     filepath.Join("/usr", "local", "bin", "gentle-ai"),
			goAvailable: true,
			goEnv:       map[string]string{"GOPATH": "/from-go-env"},
			want:        false,
		},
		{
			name:   "executable path lookup error",
			exeErr: errors.New("cannot resolve executable"),
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOBIN", tc.gobinEnv)
			t.Setenv("GOPATH", tc.gopathEnv)
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			stubExePath(t, tc.exePath, tc.exeErr)
			stubGoEnv(t, tc.goEnv, tc.goEnvErr)

			if got := runningFromGoBinDir(tc.goAvailable); got != tc.want {
				t.Errorf("runningFromGoBinDir(%v) with exe %q = %v, want %v",
					tc.goAvailable, tc.exePath, got, tc.want)
			}
		})
	}
}

func TestSameInstallDir(t *testing.T) {
	t.Run("cleans trailing separators", func(t *testing.T) {
		a := filepath.Join("/a", "b") + string(filepath.Separator)
		if !sameInstallDir(a, filepath.Join("/a", "b")) {
			t.Errorf("sameInstallDir(%q, %q) = false, want true", a, "/a/b")
		}
	})

	t.Run("different dirs", func(t *testing.T) {
		if sameInstallDir(filepath.Join("/a", "b"), filepath.Join("/a", "c")) {
			t.Errorf("sameInstallDir on different dirs = true, want false")
		}
	})

	t.Run("case insensitive when platform requires", func(t *testing.T) {
		orig := pathCaseInsensitive
		t.Cleanup(func() { pathCaseInsensitive = orig })
		pathCaseInsensitive = true
		if !sameInstallDir(`C:\Go\Bin`, `c:\go\bin`) {
			t.Errorf("sameInstallDir with case-insensitive paths = false, want true")
		}
	})
}
