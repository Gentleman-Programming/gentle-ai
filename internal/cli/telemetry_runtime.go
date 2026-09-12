package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/telemetryruntime"
	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// Tests inject transport and stdin deadlines. Direct send/OpenCode input keeps
// its 500 ms EOF budget. Hook runtimes allow 3 seconds for delayed hook delivery
// but return as soon as one complete JSON value arrives; the sender's separate
// 3-second HTTP budget starts afterward.
var runtimeHTTPClient = func() *http.Client { return nil }
var runtimeStdinTimeout = 500 * time.Millisecond
var runtimeHookStdinTimeout = 3 * time.Second

func runtimeStdinConfig(verb string) (time.Duration, bool) {
	if verb == "claude" || verb == "codex" {
		return runtimeHookStdinTimeout, true
	}
	return runtimeStdinTimeout, false
}

// readRuntimeStdin is CLI/process scoped, not a library reader API. We do not own
// arbitrary caller readers (including shared os.Stdin), so never close them.
// Hook readers stop after the first complete JSON value and ignore trailing
// bytes, because hook hosts can keep stdin open. On timeout at most this one
// reader goroutine can remain until process exit. Its buffered result cannot
// block a late completion, and every mode has a strict byte limit.
func readRuntimeStdin(ctx context.Context, input io.Reader, firstJSONValue bool) ([]byte, bool) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var data []byte
		var err error
		if firstJSONValue {
			data, err = readRuntimeJSONValue(input)
		} else {
			data, err = io.ReadAll(io.LimitReader(input, telemetry.RuntimeMaxBytes+1))
		}
		done <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, false
	case got := <-done:
		return got.data, ctx.Err() == nil && got.err == nil && len(got.data) <= telemetry.RuntimeMaxBytes
	}
}

func readRuntimeJSONValue(input io.Reader) ([]byte, error) {
	decoder := json.NewDecoder(io.LimitReader(input, telemetry.RuntimeMaxBytes+1))
	var value json.RawMessage
	if err := decoder.Decode(&value); err != nil || decoder.InputOffset() > telemetry.RuntimeMaxBytes || len(value) > telemetry.RuntimeMaxBytes {
		// refusal:by-design world-action: the host hook wrote malformed or oversized stdin; only the host process can emit one complete JSON object within the bound, no gentle-ai command repairs its stream
		return nil, errors.New("invalid bounded runtime hook input")
	}
	return value, nil
}

func runTelemetryRuntime(args []string, stdout io.Writer) error {
	return runTelemetryRuntimeInput(args, stdout, os.Stdin)
}

func runTelemetryRuntimeInput(args []string, stdout io.Writer, input io.Reader) error {
	if len(args) != 2 || (args[0] != "send" && args[0] != "opencode" && args[0] != "claude" && args[0] != "codex") || args[1] != "--json" {
		return errors.New("usage: gentle-ai telemetry runtime <send|opencode|claude|codex> --json (bounded aggregate or hook on stdin)")
	}
	decision := "disabled"
	if telemetry.Decide(os.Getenv, telemetry.State{Enabled: true}).Enabled {
		home, err := osUserHomeDir()
		if err != nil {
			decision = "discarded"
		} else {
			// Preserve enrolled-policy gating before any raw input is consumed. The
			// library rechecks fresh policy after this read and immediately before HTTP.
			policy, err := telemetry.LoadPolicyState(home)
			if err == nil && policy.NoticeShown && telemetry.Decide(os.Getenv, policy).Enabled {
				timeout, firstJSONValue := runtimeStdinConfig(args[0])
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				data, ok := readRuntimeStdin(ctx, input, firstJSONValue)
				cancel()
				decision = "discarded"
				if ok {
					if args[0] == "opencode" {
						decision = telemetryruntime.SendOpenCode(context.Background(), home, os.Getenv, bytes.NewReader(data), runtimeHTTPClient())
					} else if args[0] == "claude" {
						decision = telemetryruntime.SendClaude(context.Background(), home, os.Getenv, bytes.NewReader(data), runtimeHTTPClient())
					} else if args[0] == "codex" {
						decision = telemetryruntime.SendCodex(context.Background(), home, os.Getenv, bytes.NewReader(data), runtimeHTTPClient())
					} else {
						decision = telemetry.SendRuntime(context.Background(), home, os.Getenv, bytes.NewReader(data), runtimeHTTPClient())
					}
				}
			}
		}
	}
	// Codex validates JSON-looking hook stdout as hook output even for async
	// commands. Runtime telemetry has no Codex hook response, so stay silent.
	if args[0] == "codex" {
		return nil
	}
	return encodeReviewJSON(stdout, struct {
		Schema   string `json:"schema"`
		Decision string `json:"decision"`
	}{"gentle-ai.telemetry-runtime-send/v1", decision})
}
