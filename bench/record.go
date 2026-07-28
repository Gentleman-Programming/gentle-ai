package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionRecord is one recorded gentle-ai invocation from a real agent
// session. Timestamps exist only to order the records; no duration is ever
// reported, because wall-clock is provider-dependent and deliberately not a
// dimension of this benchmark.
type SessionRecord struct {
	Sequence       int      `json:"sequence"`
	TimestampNanos int64    `json:"timestamp_nanos"`
	Argv           []string `json:"argv"`
	Cwd            string   `json:"cwd"`
	ExitCode       int      `json:"exit_code"`
	Stdout         string   `json:"stdout"`
	Stderr         string   `json:"stderr"`
	StdoutCaptured bool     `json:"stdout_captured"`
	StderrCaptured bool     `json:"stderr_captured"`
	// Envelope names the recognized JSON envelope on stdout, or "" when
	// stdout was not one.
	Envelope string `json:"envelope"`
	GitCalls *int   `json:"git_calls,omitempty"`
}

// knownEnvelopes are the schema identifiers this benchmark recognizes. The
// list is informational: classification never depends on it.
var knownEnvelopes = []string{
	"gentle-ai.review-gate-result/v1",
	"gentle-ai.review-integration.status/v2",
	"gentle-ai.review-capture-preflight/v1",
	"gentle-ai.review-result-artifact/v2",
	"gentle-ai.review-verification-evidence/v1",
	"gentle-ai.review-mode/v1",
	"gentle-ai.rdd-mode-status/v1",
}

func envelopeName(stdout string) string {
	var object map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &object); err != nil {
		return ""
	}
	if schema, ok := object["schema"].(string); ok {
		for _, known := range knownEnvelopes {
			if schema == known {
				return schema
			}
		}
		return schema
	}
	if operation, ok := object["operation"].(string); ok {
		return operation
	}
	return "json"
}

// setupRecording writes the shim and prints the two lines the user must run.
// The shim is a two-line sh script that re-executes THIS binary in __shim
// mode, so all the transparency work happens in Go where it can be done
// correctly.
func setupRecording(realBinary, outPath string) error {
	real, err := filepath.Abs(realBinary)
	if err != nil {
		return err
	}
	if _, err := os.Stat(real); err != nil {
		return fmt.Errorf("real binary: %w", err)
	}
	out, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}

	shimDir := out + ".shim"
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return err
	}
	shim := filepath.Join(shimDir, "gentle-ai")
	script := "#!/bin/sh\n" +
		"exec " + shellQuote(self) + " __shim --real " + shellQuote(real) +
		" --log " + shellQuote(out) + " -- \"$@\"\n"
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, nil, 0o644); err != nil {
		return err
	}
	traceDir := out + ".gittrace"
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		return err
	}

	fmt.Printf("Recording shim ready.\n\n")
	fmt.Printf("1. Put the shim first on PATH in the shell your agent runs in:\n\n")
	fmt.Printf("   export PATH=%s:$PATH\n\n", shimDir)
	fmt.Printf("   (fish: set -gx PATH %s $PATH)\n\n", shimDir)
	fmt.Printf("2. Run your agent through the testing guide as usual. Every gentle-ai\n")
	fmt.Printf("   invocation is appended to:\n\n   %s\n\n", out)
	fmt.Printf("3. When the session is over, compute the friction dimensions:\n\n")
	fmt.Printf("   %s analyze --session %s --out results-observed.json\n\n", self, out)
	fmt.Printf("The shim delegates to %s and preserves argv, stdin, stdout, stderr and\n", real)
	fmt.Printf("the exit code exactly. Terminal streams are passed through untouched\n")
	fmt.Printf("rather than teed, so the product's own interactivity detection is\n")
	fmt.Printf("unchanged; the affected dimension is reported as null, never guessed.\n")
	return nil
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// runShim is the transparent proxy. Fidelity rules:
//   - argv is forwarded byte for byte
//   - stdin is the parent's stdin, unwrapped
//   - a stream that is a terminal is passed through directly and NOT captured,
//     because replacing a TTY with a pipe changes how the product behaves
//   - a stream that is not a terminal is teed, so an agent session (always
//     pipes) is fully observed
//   - the child's exit code is the shim's exit code
func runShim(real, logPath string, args []string) int {
	cmd := exec.Command(real, args...)
	cmd.Stdin = os.Stdin

	var stdoutBuffer, stderrBuffer bytes.Buffer
	stdoutCaptured := !isCharDevice(os.Stdout)
	stderrCaptured := !isCharDevice(os.Stderr)
	if stdoutCaptured {
		cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuffer)
	} else {
		cmd.Stdout = os.Stdout
	}
	if stderrCaptured {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuffer)
	} else {
		cmd.Stderr = os.Stderr
	}

	environment := os.Environ()
	tracePath := ""
	if os.Getenv("GIT_TRACE") == "" {
		tracePath = filepath.Join(logPath+".gittrace", fmt.Sprintf("%d.log", time.Now().UnixNano()))
		environment = append(environment, "GIT_TRACE="+tracePath)
	}
	cmd.Env = environment

	exitCode := 0
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		// The real binary could not be executed at all. Say so on stderr and
		// use the shell's conventional "command not executable" code, rather
		// than silently swallowing the failure.
		fmt.Fprintf(os.Stderr, "gentle-ai-bench shim: %v\n", err)
		exitCode = 126
	}

	record := SessionRecord{
		TimestampNanos: time.Now().UnixNano(),
		Argv:           args,
		ExitCode:       exitCode,
		Stdout:         stdoutBuffer.String(),
		Stderr:         stderrBuffer.String(),
		StdoutCaptured: stdoutCaptured,
		StderrCaptured: stderrCaptured,
	}
	record.Cwd, _ = os.Getwd()
	record.Envelope = envelopeName(record.Stdout)
	if tracePath != "" {
		record.GitCalls = countTrace(tracePath)
		_ = os.Remove(tracePath)
	}
	appendSessionRecord(logPath, record)
	return exitCode
}

func countTrace(path string) *int {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	count := bytes.Count(content, []byte("trace: built-in: git "))
	count += bytes.Count(content, []byte("trace: exec: git "))
	return &count
}

func isCharDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func appendSessionRecord(logPath string, record SessionRecord) {
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	unlock := lockFile(file)
	defer unlock()
	_, _ = file.Write(append(line, '\n'))
}

// readSession loads a recorded session and puts it in invocation order.
//
// The shim cannot number its own records: every invocation is a separate
// process that would have to read the whole log to learn how many came before
// it, which is both racy under concurrent appends and wasteful. So the shim
// writes Sequence as zero and this function assigns it here, once, from the
// timestamps it already recorded. Leaving the field at zero on every record
// was worse than not publishing it at all: a reader who trusted it concluded
// the invocations had been recorded out of order.
//
// Ordering comes from TimestampNanos rather than file order, because appends
// from concurrent invocations can interleave in the file even when the clock
// ordering is unambiguous. Sequence is 1-based so it reads as a count.
func readSession(path string) ([]SessionRecord, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	records := []SessionRecord{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record SessionRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("session line %d: %w", len(records)+1, err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		return nil, errors.New("session contains no recorded invocations")
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].TimestampNanos < records[j].TimestampNanos
	})
	for index := range records {
		records[index].Sequence = index + 1
	}
	return records, nil
}
