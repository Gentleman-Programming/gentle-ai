package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

const counterProcessHomeEnv = "GENTLE_AI_TEST_COUNTER_PROCESS_HOME"

// TestIncrementSyncsProcessHelper runs only in children of the process test.
// The handshake ensures every child is running before any starts writing.
func TestIncrementSyncsProcessHelper(t *testing.T) {
	home := os.Getenv(counterProcessHomeEnv)
	if home == "" {
		t.Skip("subprocess helper")
	}
	if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
		t.Fatal(err)
	}
	var start [1]byte
	if _, err := io.ReadFull(os.Stdin, start[:]); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := IncrementSyncs(home); err != nil {
			t.Fatal(err)
		}
	}
}

// Separate processes do not share statePathLocks: this exercises the OS lock
// as well as the policy read and state initialization under real contention.
func TestIncrementSyncsAcrossProcessesDoesNotLoseUpdates(t *testing.T) {
	if testing.Short() {
		t.Skip("starts separate test processes")
	}
	home := t.TempDir()
	for _, key := range []string{"DO_NOT_TRACK", "GENTLE_AI_TELEMETRY", "CI", "GITHUB_ACTIONS"} {
		t.Setenv(key, "")
	}
	t.Setenv(counterProcessHomeEnv, home)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type child struct {
		cmd    *exec.Cmd
		reader *bufio.Reader
		stdin  io.WriteCloser
		stderr bytes.Buffer
	}
	children := make([]child, 4)
	for i := range children {
		c := &children[i]
		c.cmd = exec.CommandContext(ctx, self, "-test.run=^TestIncrementSyncsProcessHelper$")
		c.cmd.Stderr = &c.stderr
		stdout, err := c.cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		c.reader = bufio.NewReader(stdout)
		c.stdin, err = c.cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := c.cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = c.stdin.Close()
			if c.cmd.ProcessState == nil {
				_ = c.cmd.Process.Kill()
				_ = c.cmd.Wait()
			}
		})
	}
	for i := range children {
		ready, err := children[i].reader.ReadString('\n')
		if err != nil || ready != "READY\n" {
			t.Fatalf("child %d readiness = %q, error = %v", i, ready, err)
		}
	}
	for i := range children {
		if _, err := children[i].stdin.Write([]byte{1}); err != nil {
			t.Fatal(err)
		}
		if err := children[i].stdin.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for i := range children {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := &children[i]
			stdout, readErr := io.ReadAll(c.reader)
			waitErr := c.cmd.Wait()
			if readErr != nil || waitErr != nil || string(stdout) != "PASS\n" || c.stderr.Len() != 0 {
				t.Errorf("child %d: read=%v exit=%v stdout=%q stderr=%q", i, readErr, waitErr, stdout, c.stderr.String())
			}
		}(i)
	}
	wg.Wait()
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.Counters != (Counters{Syncs: 20}) {
		t.Fatalf("counters = %+v, want only Syncs=20", s.Counters)
	}
	if s.InstallID == "" || !s.Enabled {
		t.Fatalf("state was not initialized enabled with an install ID: %+v", s)
	}
}
