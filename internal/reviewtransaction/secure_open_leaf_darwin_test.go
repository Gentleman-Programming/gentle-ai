//go:build darwin

package reviewtransaction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSecureOpenLocalStoreLockLeafRetriesENOENT(t *testing.T) {
	const (
		parentFD = 17
		name     = "LOCK"
		flags    = unix.O_RDWR | unix.O_CREAT | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		mode     = 0o600
	)

	tests := []struct {
		name      string
		results   []secureOpenLeafResult
		wantFD    int
		wantErr   error
		wantCalls int
	}{
		{
			name:    "transient ENOENT succeeds on retry",
			results: []secureOpenLeafResult{{fd: -1, err: unix.ENOENT}, {fd: 41}},
			wantFD:  41, wantCalls: 2,
		},
		{
			name:    "persistent ENOENT stops after three attempts",
			results: []secureOpenLeafResult{{fd: -1, err: unix.ENOENT}, {fd: -1, err: unix.ENOENT}, {fd: -1, err: unix.ENOENT}, {fd: 41}},
			wantFD:  -1, wantErr: unix.ENOENT, wantCalls: 3,
		},
		{
			name:    "wrapped ENOENT retries",
			results: []secureOpenLeafResult{{fd: -1, err: fmt.Errorf("open lock: %w", unix.ENOENT)}, {fd: 41}},
			wantFD:  41, wantCalls: 2,
		},
		{
			name:    "non ENOENT is returned immediately",
			results: []secureOpenLeafResult{{fd: -1, err: unix.EACCES}, {fd: 41}},
			wantFD:  -1, wantErr: unix.EACCES, wantCalls: 1,
		},
		{
			name:    "immediate success opens once",
			results: []secureOpenLeafResult{{fd: 41}},
			wantFD:  41, wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []secureOpenLeafCall
			fd, err := secureOpenLocalStoreLockLeafWith(parentFD, name, flags, mode, func(gotParentFD int, gotName string, gotFlags int, gotMode uint32) (int, error) {
				calls = append(calls, secureOpenLeafCall{gotParentFD, gotName, gotFlags, gotMode})
				result := tt.results[len(calls)-1]
				return result.fd, result.err
			})
			if fd != tt.wantFD || !errors.Is(err, tt.wantErr) {
				t.Fatalf("secure open leaf = (%d, %v), want (%d, %v)", fd, err, tt.wantFD, tt.wantErr)
			}
			if len(calls) != tt.wantCalls {
				t.Fatalf("open calls = %d, want %d", len(calls), tt.wantCalls)
			}
			for _, call := range calls {
				if call != (secureOpenLeafCall{parentFD, name, flags, mode}) {
					t.Fatalf("open arguments = %#v, want parent=%d name=%q flags=%#x mode=%#o", call, parentFD, name, flags, mode)
				}
			}
		})
	}
}

type secureOpenLeafResult struct {
	fd  int
	err error
}

type secureOpenLeafCall struct {
	parentFD int
	name     string
	flags    int
	mode     uint32
}

const (
	secureOpenLeafStuckBeforeEntryEnv  = "GENTLE_AI_SECURE_OPEN_LEAF_STUCK_BEFORE_ENTRY"
	secureOpenLeafConcurrentLifeline   = 5 * time.Second
	secureOpenLeafInjectedFailureBound = 100 * time.Millisecond
)

func TestSecureOpenLocalStoreLockLeafConcurrentPersistentENOENTStuckWorkerFailsPromptly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSecureOpenLocalStoreLockLeafConcurrentPersistentENOENT$", "-test.timeout=1s")
	command.Env = append(os.Environ(), secureOpenLeafStuckBeforeEntryEnv+"=1")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("stuck worker child test exceeded its outer bound: %v\n%s", ctx.Err(), output)
	}
	if err == nil {
		t.Fatalf("stuck worker child test passed, want bounded entry failure\n%s", output)
	}
	if strings.Contains(string(output), "test timed out") {
		t.Fatalf("stuck worker child test timed out instead of reporting bounded entry failure\n%s", output)
	}
	if !strings.Contains(string(output), "second worker did not enter the first open attempt") {
		t.Fatalf("stuck worker child test output missing bounded entry failure\n%s", output)
	}
}

func TestSecureOpenLocalStoreLockLeafConcurrentPersistentENOENT(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan secureOpenLeafResult, 2)
	var releaseOnce sync.Once
	releaseWorkers := func() { releaseOnce.Do(func() { close(release) }) }
	var calls struct {
		sync.Mutex
		byParent map[int]int
	}
	calls.byParent = make(map[int]int)
	var workers sync.WaitGroup
	for _, parentFD := range []int{17, 18} {
		workers.Add(1)
		go func(parentFD int) {
			defer workers.Done()
			fd, err := secureOpenLocalStoreLockLeafWith(parentFD, "LOCK", unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600, func(parentFD int, name string, flags int, mode uint32) (int, error) {
				calls.Lock()
				calls.byParent[parentFD]++
				attempt := calls.byParent[parentFD]
				calls.Unlock()
				if attempt == 1 {
					if os.Getenv(secureOpenLeafStuckBeforeEntryEnv) == "1" && parentFD == 18 {
						return -1, unix.ENOENT
					}
					entered <- struct{}{}
					<-release
				}
				return -1, unix.ENOENT
			})
			results <- secureOpenLeafResult{fd: fd, err: err}
		}(parentFD)
	}
	finished := make(chan struct{})
	go func() {
		workers.Wait()
		close(finished)
	}()
	workersFinished := false
	t.Cleanup(func() {
		releaseWorkers()
		if !workersFinished && !waitForSecureOpenLeafCompletion(finished, secureOpenLeafConcurrentLifeline) {
			t.Errorf("persistent ENOENT workers did not finish during cleanup within %s", secureOpenLeafConcurrentLifeline)
		}
	})

	entryBound := secureOpenLeafConcurrentLifeline
	if os.Getenv(secureOpenLeafStuckBeforeEntryEnv) == "1" {
		entryBound = secureOpenLeafInjectedFailureBound
	}
	if !waitForSecureOpenLeafEntries(entered, 2, entryBound) {
		releaseWorkers()
		if !waitForSecureOpenLeafCompletion(finished, secureOpenLeafConcurrentLifeline) {
			t.Errorf("persistent ENOENT workers did not finish after entry failure within %s", secureOpenLeafConcurrentLifeline)
		}
		t.Errorf("second worker did not enter the first open attempt within %s", entryBound)
		return
	}
	releaseWorkers()
	if !waitForSecureOpenLeafCompletion(finished, secureOpenLeafConcurrentLifeline) {
		t.Errorf("persistent ENOENT retries did not complete within %s after releasing first attempts", secureOpenLeafConcurrentLifeline)
		return
	}
	workersFinished = true
	close(results)
	for result := range results {
		if result.fd != -1 || !errors.Is(result.err, unix.ENOENT) {
			t.Fatalf("persistent ENOENT result = (%d, %v), want (-1, ENOENT)", result.fd, result.err)
		}
	}
	calls.Lock()
	defer calls.Unlock()
	for _, parentFD := range []int{17, 18} {
		if calls.byParent[parentFD] != 3 {
			t.Fatalf("parent fd %d calls = %d, want 3", parentFD, calls.byParent[parentFD])
		}
	}
}

func TestSecureOpenLocalStoreLockLeafConcurrentFirstCreate(t *testing.T) {
	parent := canonicalTempDir(t)
	parentFDs := make([]int, 2)
	parentStats := make([]unix.Stat_t, 2)
	for i := range parentFDs {
		fd, err := unix.Open(parent, secureDirectoryOpenFlags(), 0)
		if err != nil {
			t.Fatal(err)
		}
		parentFDs[i] = fd
		t.Cleanup(func() { _ = unix.Close(fd) })
		if err := unix.Fstat(fd, &parentStats[i]); err != nil {
			t.Fatal(err)
		}
	}
	if parentStats[0].Dev != parentStats[1].Dev || parentStats[0].Ino != parentStats[1].Ino {
		t.Fatalf("independent parent descriptors opened different inodes: %#v", parentStats)
	}

	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan secureOpenLeafResult, 2)
	var releaseOnce sync.Once
	releaseWorkers := func() { releaseOnce.Do(func() { close(release) }) }
	var workers sync.WaitGroup
	for _, parentFD := range parentFDs {
		workers.Add(1)
		go func(parentFD int) {
			defer workers.Done()
			var firstAttempt sync.Once
			fd, err := secureOpenLocalStoreLockLeafWith(parentFD, "LOCK", unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0o600, func(parentFD int, name string, flags int, mode uint32) (int, error) {
				firstAttempt.Do(func() {
					entered <- struct{}{}
					<-release
				})
				return unix.Openat(parentFD, name, flags, mode)
			})
			results <- secureOpenLeafResult{fd: fd, err: err}
		}(parentFD)
	}
	finished := make(chan struct{})
	go func() {
		workers.Wait()
		close(finished)
	}()
	workersFinished := false
	t.Cleanup(func() {
		releaseWorkers()
		if !workersFinished && !waitForSecureOpenLeafCompletion(finished, secureOpenLeafConcurrentLifeline) {
			t.Errorf("first create workers did not finish during cleanup within %s", secureOpenLeafConcurrentLifeline)
		}
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if result.fd >= 0 {
					_ = unix.Close(result.fd)
				}
			default:
				return
			}
		}
	})

	if !waitForSecureOpenLeafEntries(entered, len(parentFDs), secureOpenLeafConcurrentLifeline) {
		releaseWorkers()
		if !waitForSecureOpenLeafCompletion(finished, secureOpenLeafConcurrentLifeline) {
			t.Errorf("first create workers did not finish after entry failure within %s", secureOpenLeafConcurrentLifeline)
		}
		t.Errorf("concurrent first create workers did not enter the first open attempt within %s", secureOpenLeafConcurrentLifeline)
		return
	}
	releaseWorkers()
	if !waitForSecureOpenLeafCompletion(finished, secureOpenLeafConcurrentLifeline) {
		t.Errorf("concurrent first create workers did not complete within %s after releasing first attempts", secureOpenLeafConcurrentLifeline)
		return
	}
	workersFinished = true
	close(results)

	var stats []unix.Stat_t
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent first create: %v", result.err)
		}
		var stat unix.Stat_t
		if err := unix.Fstat(result.fd, &stat); err != nil {
			_ = unix.Close(result.fd)
			t.Fatal(err)
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFREG {
			_ = unix.Close(result.fd)
			t.Fatalf("concurrent first create mode = %#o, want regular file", stat.Mode)
		}
		if err := unix.Close(result.fd); err != nil {
			t.Fatal(err)
		}
		stats = append(stats, stat)
	}
	if len(stats) != 2 || stats[0].Dev != stats[1].Dev || stats[0].Ino != stats[1].Ino {
		t.Fatalf("concurrent first create opened different inodes: %#v", stats)
	}
}

func waitForSecureOpenLeafEntries(entered <-chan struct{}, count int, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for range count {
		select {
		case <-entered:
		case <-timer.C:
			return false
		}
	}
	return true
}

func waitForSecureOpenLeafCompletion(finished <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-finished:
		return true
	case <-timer.C:
		return false
	}
}
