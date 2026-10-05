package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// The portable corpus measures policy, not a live model runtime. A private
// native alias of this harness supplies synthetic V1 version evidence only.
// Observed sessions and organic real-runtime tests never use this sandbox.
func policyRuntimeVersionFixture(args []string, out io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	name := strings.ToLower(path.Base(strings.ReplaceAll(args[0], `\`, "/")))
	if name != "opencode" && name != "opencode.exe" {
		return false, 0
	}
	if len(args) != 2 || args[1] != "--version" {
		return true, 2
	}
	fmt.Fprintln(out, "1.18.10")
	return true, 0
}

func installPolicyRuntimeFixture(root string) (string, error) {
	dir := filepath.Join(root, "policy-runtime-bin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	name := "opencode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(dir, name)
	source, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := os.Link(source, target); err == nil {
		return dir, nil
	}
	// Cross-device temporary directories cannot hardlink; copying retains a
	// native executable on Windows without requiring symlink privileges.
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dir, nil
}
