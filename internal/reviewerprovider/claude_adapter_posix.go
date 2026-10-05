//go:build !windows

package reviewerprovider

import "os/exec"

// configureWindowsBatchLaunch is a no-op outside Windows: only Windows routes
// .bat/.cmd execution through cmd.exe's own command-line reparsing (issue
// #4039). POSIX exec never involves a shell for this adapter, so a space in
// the binary path is never a hazard here.
func configureWindowsBatchLaunch(*exec.Cmd, string, []string) {}
