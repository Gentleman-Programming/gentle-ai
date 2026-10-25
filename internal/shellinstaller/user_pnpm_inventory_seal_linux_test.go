//go:build linux && amd64

package shellinstaller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUserPNPMCodeSealPreservation(t *testing.T) {
	for _, scenario := range []string{"content", "mode", "symlink", "fifo", "oversize", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "pnpm")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "entry")
			if err := os.WriteFile(file, []byte("retained bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := PNPMCodeSeal(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch scenario {
			case "content":
				err = os.WriteFile(file, []byte("changed bytes"), 0600)
			case "mode":
				err = os.Chmod(file, 0750)
			case "symlink":
				err = os.Symlink("entry", filepath.Join(dir, "link"))
			case "fifo":
				err = syscall.Mkfifo(filepath.Join(dir, "pipe"), 0600)
			case "oversize":
				err = os.Truncate(file, 268435457)
				if errors.Is(err, syscall.EFBIG) {
					// Guest runners enforce RLIMIT_FSIZE below the 256 MiB
					// oversize threshold, so even a sparse extension is
					// rejected with EFBIG. The refusal itself is exercised
					// on hosts without that limit.
					t.Skipf("oversize fixture blocked by RLIMIT_FSIZE: %v", err)
				}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Lstat(file)
			if err != nil {
				t.Fatal(err)
			}
			var data []byte
			if scenario != "oversize" {
				data, err = os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := PNPMCodeSeal(ctx, root)
			wantOK := scenario == "content" || scenario == "symlink"
			if (err == nil) != wantOK || (wantOK && got == before) {
				t.Fatalf("unexpected seal result: %q, %v", got, err)
			}
			after, err := os.Lstat(file)
			if err != nil || !os.SameFile(info, after) || info.Mode() != after.Mode() || info.Size() != after.Size() {
				t.Fatal("seal mutated source metadata", err)
			}
			if scenario != "oversize" {
				now, err := os.ReadFile(file)
				if err != nil || string(now) != string(data) {
					t.Fatal("seal changed source bytes", err)
				}
			}
		})
	}
}
