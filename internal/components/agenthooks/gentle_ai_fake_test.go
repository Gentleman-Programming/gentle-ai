package agenthooks

// Re-execute a copy of the test binary as gentle-ai to capture argv without
// introducing a shell that would re-tokenize arguments on Windows.
import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	if logPath := os.Getenv("GENTLE_AI_FAKE_LOG"); logPath != "" && (name == "gentle-ai" || name == "gentle-ai.exe") {
		os.Exit(runGentleAIFake(logPath))
	}
	os.Exit(m.Run())
}

func runGentleAIFake(logPath string) int {
	var log bytes.Buffer
	for i, arg := range os.Args[1:] {
		fmt.Fprintf(&log, "argv[%d]=%s\n", i, arg)
	}
	if projectDir, ok := os.LookupEnv("CLAUDE_PROJECT_DIR"); ok {
		fmt.Fprintf(&log, "env-CLAUDE_PROJECT_DIR=%s\n", projectDir)
	} else {
		log.WriteString("env-CLAUDE_PROJECT_DIR=<unset>\n")
	}
	stdinBytes := readStdinBounded(os.Stdin, 10*time.Second)
	fmt.Fprintf(&log, "stdin-bytes=%d\n", len(stdinBytes))
	log.WriteString("stdin-begin\n")
	log.Write(stdinBytes)
	log.WriteString("\nstdin-end\n")
	exitCode := 0
	if raw := os.Getenv("GENTLE_AI_FAKE_EXIT"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			exitCode = parsed
		}
	}
	fmt.Fprintf(&log, "exit=%d\n", exitCode)
	if err := os.WriteFile(logPath, log.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gentle-ai fake: write log %q: %v\n", logPath, err)
		return 70
	}
	return exitCode
}

func readStdinBounded(r io.Reader, limit time.Duration) []byte {
	if f, ok := r.(*os.File); ok {
		if err := f.SetReadDeadline(time.Now().Add(limit)); err == nil {
			data, _ := io.ReadAll(f)
			return data
		}
	}
	ch := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(r)
		ch <- data
	}()
	select {
	case data := <-ch:
		return data
	case <-time.After(limit):
		return nil
	}
}
