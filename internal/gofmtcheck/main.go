package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	os.Exit(run(".", os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	cmd := exec.Command("git", "ls-files", "-z", "--", "*.go")
	cmd.Dir = root
	tracked, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(stderr, "list tracked Go files: %v\n", err)
		return 1
	}

	failed := false
	for _, name := range strings.Split(strings.TrimSuffix(string(tracked), "\x00"), "\x00") {
		if name == "" {
			continue
		}
		var output bytes.Buffer
		cmd = exec.Command("gofmt", "-l", filepath.FromSlash(name))
		cmd.Dir = root
		cmd.Stdout = &output
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(stderr, "gofmt %s: %v\n", name, err)
			failed = true
		}
		if output.Len() != 0 {
			_, _ = io.Copy(stdout, &output)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}
