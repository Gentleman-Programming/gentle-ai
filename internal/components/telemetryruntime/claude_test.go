package telemetryruntime

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

func TestReadClaudeFileReturnsOnlyBoundedTail(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "projects", "transcript.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	recent := []byte(`{"type":"assistant","message":{"model":"claude-opus-5","content":"MATCH","usage":{"input_tokens":21,"output_tokens":22,"cache_read_input_tokens":23,"cache_creation_input_tokens":24}}}` + "\n")
	body := append(bytes.Repeat([]byte("x"), telemetry.ClaudeTranscriptMaxBytes+100), '\n')
	body = append(body, recent...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	tail, truncated, err := readClaudeFile(home, path, telemetry.ClaudeTranscriptMaxBytes, true)
	if err != nil || !truncated || len(tail) > telemetry.ClaudeTranscriptMaxBytes {
		t.Fatal(len(tail), truncated, err)
	}
	usage, ok := telemetry.ParseClaudeTranscriptTail(tail, truncated, sha256.Sum256([]byte("MATCH")))
	if !ok || string(usage.Input) != "21" {
		t.Fatalf("tail usage: %+v %v", usage, ok)
	}
}

func TestReadClaudeFileExactNewlineBoundaryKeepsFirstRecord(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "transcript.jsonl")
	line := []byte(`{"type":"assistant","message":{"model":"claude-opus-5","content":"MATCH","usage":{"input_tokens":41}}}` + "\n")
	tail := append(append([]byte(nil), line...), bytes.Repeat([]byte{'\n'}, telemetry.ClaudeTranscriptMaxBytes-len(line))...)
	body := append(append([]byte("old"), '\n'), tail...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	tail, firstPartial, err := readClaudeFile(home, path, telemetry.ClaudeTranscriptMaxBytes, true)
	if err != nil || firstPartial {
		t.Fatalf("boundary misclassified: %v partial=%v", err, firstPartial)
	}
	usage, ok := telemetry.ParseClaudeTranscriptTail(tail, firstPartial, sha256.Sum256([]byte("MATCH")))
	if !ok || string(usage.Input) != "41" {
		t.Fatalf("boundary record lost: %+v %v", usage, ok)
	}
}

func TestReadClaudeFileRefusesOutsideHomeSymlink(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "transcript.jsonl")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "transcript.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readClaudeFile(home, link, telemetry.ClaudeTranscriptMaxBytes, true); err == nil {
		t.Fatal("followed symlink outside home")
	}
}
