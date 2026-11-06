//go:build windows

package shellinstaller

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Temporary native measurement; no budget or custody checks are relaxed.
func TestUserWindowsWriteLatency(t *testing.T) {
	root := t.TempDir()
	if err := userWindowsPrivate(root); err != nil {
		t.Fatal(err)
	}
	for _, procs := range []int{1, 32} {
		old := runtime.GOMAXPROCS(procs)
		defer runtime.GOMAXPROCS(old)
		var parent, write, flush, read time.Duration
		for i := 0; i < 256; i++ {
			path := filepath.Join(root, fmt.Sprintf("file-%d-%d.go", procs, i))
			start := time.Now()
			if _, err := userWindowsIdentity(root, true); err != nil {
				t.Fatal(err)
			}
			parent += time.Since(start)
			start = time.Now()
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.Write([]byte("package fixture\n")); err != nil {
				t.Fatal(err)
			}
			write += time.Since(start)
			start = time.Now()
			if err := file.Sync(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			flush += time.Since(start)
			start = time.Now()
			if data, err := userWindowsRead(path, 16); err != nil || string(data) != "package fixture\n" {
				t.Fatal("readback differs", err)
			}
			read += time.Since(start)
		}
		t.Logf("256 verified native writes GOMAXPROCS=%d: parent-identity=%dms open-write=%dms flush-close=%dms readback-identities=%dms", procs, parent.Milliseconds(), write.Milliseconds(), flush.Milliseconds(), read.Milliseconds())
	}
}
