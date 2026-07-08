package engram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyInstalled(t *testing.T) {
	original := lookPath
	t.Cleanup(func() { lookPath = original })

	lookPath = func(string) (string, error) { return "/opt/homebrew/bin/engram", nil }
	if err := VerifyInstalled(); err != nil {
		t.Fatalf("VerifyInstalled() error = %v", err)
	}

	lookPath = func(string) (string, error) { return "", errors.New("missing") }
	if err := VerifyInstalled(); err == nil {
		t.Fatalf("VerifyInstalled() expected missing binary error")
	}
}

func TestVerifyHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := VerifyHealth(context.Background(), server.URL); err != nil {
		t.Fatalf("VerifyHealth() error = %v", err)
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer badServer.Close()

	if err := VerifyHealth(context.Background(), badServer.URL); err == nil {
		t.Fatalf("VerifyHealth() expected non-200 error")
	}
}

func TestVerifyVersionCommandUsesTimeoutAndProvidedBinary(t *testing.T) {
	original := runVersionCommand
	t.Cleanup(func() { runVersionCommand = original })

	var gotCommand string
	var sawDeadline bool
	runVersionCommand = func(ctx context.Context, command string) ([]byte, error) {
		gotCommand = command
		_, sawDeadline = ctx.Deadline()
		return []byte("engram 1.18.0\n"), nil
	}

	version, err := VerifyVersionCommand("/tmp/beta/engram")
	if err != nil {
		t.Fatalf("VerifyVersionCommand() error = %v", err)
	}
	if version != "engram 1.18.0" {
		t.Fatalf("version = %q, want trimmed output", version)
	}
	if gotCommand != "/tmp/beta/engram" {
		t.Fatalf("command = %q, want provided binary", gotCommand)
	}
	if !sawDeadline {
		t.Fatal("VerifyVersionCommand() context had no deadline")
	}
}

func TestVerifyVersionCommandFallsBackOnTimeoutError(t *testing.T) {
	original := runVersionCommand
	t.Cleanup(func() { runVersionCommand = original })

	runVersionCommand = func(ctx context.Context, _ string) ([]byte, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("VerifyVersionCommand() context had no deadline")
		}
		return nil, context.DeadlineExceeded
	}

	_, err := VerifyVersionCommand("/tmp/hung/engram")
	if err == nil {
		t.Fatal("VerifyVersionCommand() error = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "engram version command failed") {
		t.Fatalf("VerifyVersionCommand() error = %v, want wrapped version command failure", err)
	}
}
