package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/gentleman-programming/gentle-ai/v4/internal/app"
	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// version is set by GoReleaser via ldflags at build time.
var version = "dev"

func main() {
	app.Version = app.ResolveVersion(version)

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitCode(os.Args[1:], err))
	}
}

func exitCode(args []string, err error) int {
	if len(args) == 0 || args[0] != "shell" {
		return 1
	}
	var failure *shellinstaller.PrivateRuntimeError
	if errors.As(err, &failure) {
		return 1 // Installation/readback failures outrank any nested child status.
	}
	var child *exec.ExitError
	if errors.As(err, &child) && child.ProcessState != nil {
		if code := child.ExitCode(); code > 0 {
			return code
		}
		if status, ok := child.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
	}
	return 1
}
