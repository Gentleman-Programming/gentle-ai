//go:build !windows

package telemetry

import "os/exec"

// configureDetachedProcAttr is a no-op on Unix: SpawnDetachedSend's existing
// behavior (no SysProcAttr, reaped via a background goroutine's cmd.Wait())
// is unchanged by this seam.
func configureDetachedProcAttr(cmd *exec.Cmd) {}
