//go:build linux && amd64

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIndependentPNPMCopyCompatibility(t *testing.T) {
	for _, name := range []string{"success", "reference", "cancel"} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			source := filepath.Join(parent, "source")
			target := filepath.Join(parent, "target")
			data := []byte("unchanged supplier bytes\n")
			if name == "reference" {
				data = []byte("/original/pnpm/forbidden")
			}
			if err := os.WriteFile(source, data, 0700); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "cancel" {
				cancel()
			}
			before := sha256.Sum256(data)
			err = userPNPMCopyFileWithSync(ctx, source, target, info, true, "/original", func(file userPNPMSyncFile) error {
				return errors.Join(file.Sync(), file.Close())
			})
			if (err == nil) != (name == "success") {
				t.Fatalf("unexpected copy outcome: %v", err)
			}
			got, readErr := os.ReadFile(source)
			if readErr != nil || sha256.Sum256(got) != before {
				t.Fatal("source changed", readErr)
			}
			result, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			out, statErr := os.Stat(target)
			if statErr != nil {
				t.Fatal(statErr)
			}
			if name == "success" && (sha256.Sum256(result) != before || out.Mode().Perm() != 0700 || os.SameFile(info, out)) {
				t.Fatal("copy bytes/mode/inode mismatch")
			}
			if name == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			t.Logf("COPY_SIGNATURE %s %x %o %v", name, sha256.Sum256(result), out.Mode().Perm(), err)
		})
	}
}
