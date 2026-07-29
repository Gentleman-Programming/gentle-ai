//go:build !unix

package main

import "os"

// lockFile is a no-op where advisory locking is unavailable. A single-threaded
// agent session still records correctly; concurrent invocations may interleave.
func lockFile(*os.File) func() { return func() {} }
