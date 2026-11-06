//go:build unix

package legacyassets

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO (directly or behind a link) is never opened: reading it would block
// sync until another process writes to it.
func TestRetiredSDDPromptTextNeverReadsAFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("FIFOs unavailable: %v", err)
	}
	link := filepath.Join(dir, "KIMI.md")
	if err := os.Symlink(fifo, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, path := range []string{fifo, link} {
		done := make(chan TextRetireResult, 2)
		go func() {
			res, _ := RetireKimiSDDInclude(dir, path)
			done <- res
			block, _ := RetireSDDOrchestratorBlock(dir, path, "")
			done <- block
		}()
		for range 2 {
			select {
			case res := <-done:
				if res.Removed || len(res.ManualActions()) != 0 {
					t.Errorf("%s: %+v", path, res)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("retirement blocked reading %s", path)
			}
		}
		if RetirablePromptFile(dir, path) {
			t.Errorf("%s is retirable", path)
		}
	}
}
