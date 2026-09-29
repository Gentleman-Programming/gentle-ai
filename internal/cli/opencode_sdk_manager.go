package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	opencodeactivation "github.com/gentleman-programming/gentle-ai/v3/internal/opencode"
)

// openCodeRuntimeMajorForManagedAssets fails closed when the runtime is
// unknown: every plan that includes OpenCode writes version-specific managed
// plugins and telemetry, so V1 and V2 assets must never be guessed.
func openCodeRuntimeMajorForManagedAssets() (opencodeactivation.RuntimeMajor, error) {
	major, err := opencodeactivation.DetectRuntimeMajor(context.Background())
	if err != nil {
		return major, fmt.Errorf("%w; Gentle AI could not choose V1 or V2 managed OpenCode plugins and telemetry; make sure `opencode --version` succeeds in this shell (install, update, or add OpenCode to PATH), or deselect OpenCode, then retry", err)
	}
	return major, nil
}

// openCodeSDKIsolatedPathDirs is the only PATH the credential-isolated npm
// process receives: the resolved npm location plus standard system locations.
func openCodeSDKIsolatedPathDirs(executable, physicalExe string) []string {
	return []string{filepath.Dir(executable), filepath.Dir(physicalExe), "/usr/local/bin", "/opt/homebrew/bin", "/usr/bin", "/bin"}
}

// openCodeSDKIsolationBlocker explains, without naming private paths, why the
// resolved npm cannot run once HOME, XDG and PATH are replaced by the isolated
// installer environment. Version-manager shims resolve the real tool through
// the user's home configuration, which isolation deliberately withholds.
func openCodeSDKIsolationBlocker(executable, physicalExe string) string {
	for _, candidate := range []string{executable, physicalExe} {
		if name := openCodeSDKShimManagerFromPath(candidate); name != "" {
			return "is a " + name + " version-manager shim"
		}
	}
	if runtime.GOOS == "windows" {
		return ""
	}
	head, err := openCodeSDKScriptHead(physicalExe)
	if err != nil || !bytes.HasPrefix(head, []byte("#!")) {
		return ""
	}
	interpreter, shell := openCodeSDKShebangInterpreter(head)
	if interpreter == "" {
		return ""
	}
	if shell {
		lower := strings.ToLower(string(head))
		for _, marker := range []struct{ text, name string }{
			{"asdf exec", "asdf"}, {"nodenv exec", "nodenv"}, {"mise exec", "mise"}, {"mise x ", "mise"}, {"rtx exec", "mise"}, {"volta run", "volta"},
		} {
			if strings.Contains(lower, marker.text) {
				return "is a " + marker.name + " version-manager shim"
			}
		}
	}
	if !openCodeSDKInterpreterAvailable(interpreter, openCodeSDKIsolatedPathDirs(executable, physicalExe)) {
		return "needs interpreter `" + filepath.Base(interpreter) + "`, which is not on the isolated installer PATH"
	}
	return ""
}

func openCodeSDKShimManagerFromPath(path string) string {
	if path == "" {
		return ""
	}
	segments := strings.Split(strings.ToLower(filepath.ToSlash(path)), "/")
	base := strings.TrimSuffix(segments[len(segments)-1], ".exe")
	switch base {
	case "volta", "volta-shim":
		return "volta"
	case "mise", "rtx":
		return "mise"
	case "asdf":
		return "asdf"
	}
	for i, segment := range segments {
		switch {
		case (segment == ".volta" || segment == "volta") && i+1 < len(segments) && segments[i+1] == "bin":
			return "volta"
		case segment == "shims":
			for j := i - 1; j >= 0; j-- {
				switch {
				case strings.Contains(segments[j], "asdf"):
					return "asdf"
				case strings.Contains(segments[j], "mise") || strings.Contains(segments[j], "rtx"):
					return "mise"
				case strings.Contains(segments[j], "nodenv"):
					return "nodenv"
				}
			}
			return "shim-based"
		}
	}
	return ""
}

func openCodeSDKScriptHead(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, 4096))
}

// openCodeSDKShebangInterpreter returns the interpreter named by the first
// line and whether it is a POSIX shell.
func openCodeSDKShebangInterpreter(head []byte) (string, bool) {
	line, _, _ := bufio.NewReader(bytes.NewReader(head)).ReadLine()
	fields := strings.Fields(strings.TrimPrefix(string(line), "#!"))
	if len(fields) == 0 {
		return "", false
	}
	interpreter := fields[0]
	if filepath.Base(interpreter) == "env" {
		interpreter = ""
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "-") || strings.Contains(field, "=") {
				continue
			}
			interpreter = field
			break
		}
	}
	switch filepath.Base(interpreter) {
	case "sh", "bash", "zsh", "dash", "ksh":
		return interpreter, true
	}
	return interpreter, false
}

func openCodeSDKInterpreterAvailable(interpreter string, dirs []string) bool {
	if interpreter == "" {
		return true
	}
	if strings.Contains(interpreter, "/") {
		info, err := os.Stat(interpreter)
		return err == nil && !info.IsDir()
	}
	for _, dir := range dirs {
		if info, err := os.Stat(filepath.Join(dir, interpreter)); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return true
		}
	}
	return false
}

// openCodeSDKFailureClass names how the isolated manager failed without
// reflecting its raw error or output, which may embed credentials or paths.
func openCodeSDKFailureClass(err, ctxErr error, timeout time.Duration) string {
	if ctxErr != nil {
		return "timed out after " + timeout.String()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return fmt.Sprintf("exited with code %d", code)
		}
		return "was terminated by a signal"
	}
	return "could not start"
}
