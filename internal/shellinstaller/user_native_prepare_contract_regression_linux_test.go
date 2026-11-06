//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentPrepareDiagnosticContract(t *testing.T) {
	marker := "PRIVATE_OUTPUT_DO_NOT_PRINT"
	for _, name := range []string{"tail", "existing", "symlink", "empty", "success", "collect", "run", "deadline", "canceled", "uncertain"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "native-prepare.stderr")
			outside := filepath.Join(t.TempDir(), "preserve")
			if err := os.WriteFile(outside, []byte("outside marker"), 0600); err != nil {
				t.Fatal(err)
			}
			operation, output := "prepare", strings.Repeat("prefix", 900)+marker
			var cause error = privateError("nonzero", errors.New("exit status 23"))
			switch name {
			case "existing":
				if err := os.WriteFile(path, []byte("first diagnostic"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "empty":
				output = ""
			case "success":
				cause = nil
			case "collect", "run":
				operation = name
			case "deadline", "canceled", "uncertain":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if name == "uncertain" {
					cause = privateError("uncertain", errors.New("unsettled group"))
				} else if name == "deadline" {
					cause = privateError("deadline", context.DeadlineExceeded)
				} else {
					cause = privateError("canceled", context.Canceled)
				}
				cause = nativeRuntimeError(ctx, cause)
			}
			err := userNativePrepareDiagnostic(root, operation, output, cause)
			if cause == nil {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if !errors.Is(err, cause) || strings.Contains(err.Error(), marker) {
					t.Fatal("lost cause or leaked raw output", err)
				}
				var before, after *PrivateRuntimeError
				if !errors.As(cause, &before) || !errors.As(err, &after) || before.Kind != after.Kind {
					t.Fatal("taxonomy changed", cause, err)
				}
				if name == "uncertain" && after.Kind != "uncertain" {
					t.Fatal("uncertain downgraded", err)
				}
			}
			data, readErr := os.ReadFile(path)
			switch name {
			case "existing":
				if readErr != nil || string(data) != "first diagnostic" {
					t.Fatal("overwrite", readErr)
				}
			case "symlink":
				target, e := os.Readlink(path)
				if e != nil || target != outside || readErr != nil || string(data) != "outside marker" {
					t.Fatal("symlink changed", e, readErr)
				}
			case "empty", "success", "collect", "run":
				if !os.IsNotExist(readErr) || err != cause {
					t.Fatal("unrelated action changed state", err, readErr)
				}
			default:
				st, e := os.Lstat(path)
				want := []byte(output)
				want = want[len(want)-4096:]
				if e != nil || readErr != nil || st.Mode() != 0600 || !bytes.Equal(data, want) {
					t.Fatal("private bounded tail mismatch", e, readErr, len(data))
				}
			}
			if b, e := os.ReadFile(outside); e != nil || string(b) != "outside marker" {
				t.Fatal("outside marker changed", e)
			}
		})
	}
}
