//go:build linux && amd64

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserNativePrepareDiagnostic(t *testing.T) {
	for _, name := range []string{"prepare", "empty", "collect", "run", "existing", "symlink"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			receipt := filepath.Join(root, "native-prepare.stderr")
			output, operation := strings.Repeat("x", 5000), "prepare"
			if name == "empty" {
				output = ""
			}
			if name == "collect" || name == "run" {
				operation = name
			}
			outside := filepath.Join(t.TempDir(), "preserved")
			if err := os.WriteFile(outside, []byte("private marker"), 0600); err != nil {
				t.Fatal(err)
			}
			if name == "existing" {
				if err := os.WriteFile(receipt, []byte("first failure"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if name == "symlink" {
				if err := os.Symlink(outside, receipt); err != nil {
					t.Fatal(err)
				}
			}
			failure := privateError("nonzero", errors.New("exit status 1"))
			err := userNativePrepareDiagnostic(root, operation, output, failure)
			if !errors.Is(err, failure) || strings.Contains(err.Error(), "xxxx") {
				t.Fatal("original error lost or output leaked", err)
			}
			var typed *PrivateRuntimeError
			if !errors.As(err, &typed) || typed.Kind != "nonzero" {
				t.Fatal("error taxonomy changed", err)
			}
			data, readErr := os.ReadFile(receipt)
			switch name {
			case "prepare":
				info, statErr := os.Lstat(receipt)
				if readErr != nil || statErr != nil || len(data) != 4096 || info.Mode() != 0600 {
					t.Fatal("bounded private receipt missing", readErr, statErr, len(data))
				}
			case "existing":
				if readErr != nil || string(data) != "first failure" {
					t.Fatal("receipt overwritten", readErr)
				}
			case "symlink":
				if readErr != nil || string(data) != "private marker" {
					t.Fatal("symlink target modified", readErr)
				}
			default:
				if !os.IsNotExist(readErr) {
					t.Fatal("unrelated operation wrote receipt", readErr)
				}
			}
			if b, e := os.ReadFile(outside); e != nil || string(b) != "private marker" {
				t.Fatal("outside data changed", e)
			}
		})
	}
}
