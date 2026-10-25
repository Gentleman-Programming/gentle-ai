package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctor_AssetVersionRemediation(t *testing.T) {
	stubDoctorToolProbe(t)
	setStdioProbeForTest(t, nil)
	t.Setenv(engramHealthEnvVar, "")
	origExec, origLook := osExecutableDoctor, lookPathFn
	origHome, origDirs, origAvail := osUserHomeDirDoctor, pathDirsFn, availableBytesFn
	origGOOS := reviewManagedAssetsGOOS
	t.Cleanup(func() {
		osExecutableDoctor, lookPathFn = origExec, origLook
		osUserHomeDirDoctor, pathDirsFn, availableBytesFn = origHome, origDirs, origAvail
		reviewManagedAssetsGOOS = origGOOS
	})

	home := t.TempDir()
	statePath := filepath.Join(home, ".gentle-ai", "state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDoctorEngramConfig(t, home, "engram", []string{"mcp", "--tools=agent"})
	osUserHomeDirDoctor = func() (string, error) { return home, nil }
	pathDirsFn = func() []string { return nil }
	availableBytesFn = func(string) (int64, error) { return 1024 * 1024 * 1024, nil }

	simple := filepath.ToSlash(filepath.Join(home, "pinned", "gentle-ai"))
	// Temp dirs can contain characters outside the bare allowlist (the "~" of
	// Windows 8.3 names such as RUNNER~1), which the continuation must quote.
	simpleWord := simple
	if !managedAssetsBareExecutable.MatchString(simple) {
		simpleWord = "'" + simple + "'"
	}
	spaces := filepath.Join(home, "pinned build", "gentle-ai")
	apostrophe := filepath.Join(home, "user's build", "gentle-ai")
	other := filepath.Join(home, "homebrew", "gentle-ai")
	for _, tc := range []struct {
		name, invoked, pathCopy, goos, want string
		execErr                             error
		matching                            bool
	}{
		{name: "different PATH build", invoked: simple, pathCopy: other, goos: "linux", want: simpleWord + " sync"},
		{name: "same PATH build", invoked: simple, pathCopy: simple, goos: "linux", want: simpleWord + " sync"},
		{name: "POSIX spaces", invoked: spaces, pathCopy: other, goos: "linux", want: "'" + spaces + "' sync"},
		{name: "POSIX apostrophe", invoked: apostrophe, pathCopy: other, goos: "linux", want: "'" + strings.ReplaceAll(apostrophe, "'", `'\''`) + "' sync"},
		{name: "Windows spaces", invoked: spaces, pathCopy: other, goos: "windows", want: "\"" + spaces + "\" sync"},
		{name: "lookup failure", pathCopy: other, execErr: errors.New("unavailable"), goos: "linux"},
		{name: "relative identity", invoked: "gentle-ai", pathCopy: other, goos: "linux"},
		{name: "multiline identity", invoked: simple + "\nunsafe", pathCopy: other, goos: "linux"},
		{name: "matching assets", invoked: simple, pathCopy: simple, goos: "linux", matching: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := "v0.9.0"
			if tc.matching {
				version = AppVersion
			}
			before := []byte(fmt.Sprintf(`{"installed_binary_version":%q}`, version))
			if err := os.WriteFile(statePath, before, 0o600); err != nil {
				t.Fatal(err)
			}
			osExecutableDoctor = func() (string, error) { return tc.invoked, tc.execErr }
			lookPathFn = func(name string) (string, error) {
				if name == "gentle-ai" {
					return tc.pathCopy, nil
				}
				return filepath.Join(home, name), nil
			}
			reviewManagedAssetsGOOS = tc.goos
			var output bytes.Buffer
			if err := RunDoctor(context.Background(), &output); err != nil {
				t.Fatalf("RunDoctor: %v", err)
			}
			var line string
			for _, candidate := range strings.Split(output.String(), "\n") {
				if strings.Contains(candidate, "installed:asset_version") {
					line = candidate
				}
			}
			want := "installed assets match running binary version (" + AppVersion + ")"
			if !tc.matching {
				want = "installed assets were configured by gentle-ai v0.9.0, but running binary is " + AppVersion
				if tc.want == "" {
					want += " — cannot determine the absolute invoked executable path; run sync using the absolute path of this running build to update installed assets"
				} else {
					want += " — run `" + tc.want + "` to update installed assets"
					if tc.invoked != tc.pathCopy {
						want += "; gentle-ai on PATH resolves to " + tc.pathCopy + ", which differs from the running executable; use the invoked path above"
					}
				}
			}
			if !strings.HasSuffix(line, want) {
				t.Errorf("asset check:\n%s\nwant suffix:\n%s", line, want)
			}
			icon := "[!!]"
			if tc.matching {
				icon = "[ok]"
			}
			if !strings.Contains(line, icon) {
				t.Errorf("missing %s: %s", icon, line)
			}
			after, err := os.ReadFile(statePath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("doctor modified state: %q, %v", after, err)
			}
		})
	}
}
