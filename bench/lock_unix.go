//go:build unix

package main

import (
	"os"
	"syscall"
)

// lockFile serializes appends from concurrent shim processes so two
// invocations never interleave inside one JSONL line.
func lockFile(file *os.File) func() {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return func() {}
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
}
