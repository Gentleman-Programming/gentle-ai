package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeV2RelayRefusesDuplicateOriginalLensStart(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("requires git fixtures and the Unix relay subprocess shim")
	}
	reviewEnabledHome(t)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	stubOpenCodeReviewRuntimeVersion(t, "2.4.2", openCodeRelayContractV2)

	// The Go subprocess uses its real V2 capability gate with a version-only
	// fixture. No OpenCode Task runs and no provider is contacted in this test.
	versionProbe := filepath.Join(t.TempDir(), "opencode-version")
	if err := os.WriteFile(versionProbe, []byte("#!/bin/sh\n[ \"$#\" -eq 1 ] && [ \"$1\" = --version ] || exit 1\nprintf '2.4.2\\n'\n"), 0o700); err != nil {
		t.Fatal("could not create OpenCode version fixture")
	}
	shim := openCodeV2HostShim(t, openCodeV2HostInputs{host: versionProbe}, openCodeV2GateReal)
	host := t.TempDir()
	for _, scenario := range []struct {
		name       string
		disconnect bool
	}{
		{name: "original_in_flight"},
		{name: "original_disconnected_without_stop_confirmation", disconnect: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repo, started, _, record := newArtifactReview(t, false)
			task := openCodeV2StatusProviderTask(t, repo, started.LineageID)
			if len(record.State.SelectedLenses) != 1 || task.Agent != record.State.SelectedLenses[0] ||
				strings.HasPrefix(task.Prompt, openCodeTransportMaterializationHeader) {
				t.Fatal("expected one canonical lens and its original unmaterialized Task")
			}
			start := openCodeTransportEnvelope{Schema: openCodeReviewTransportSchema, Operation: "start", Agent: task.Agent, Prompt: task.Prompt}
			original := startOpenCodeDuplicateRelayProcess(t, shim, host, start)
			first := original.readFrame(t)
			if !openCodeDuplicateDispatchable(first) {
				t.Fatal("positive control: original_dispatchable=false")
			}
			if scenario.disconnect {
				// Relay EOF is not an acknowledgement that a host Task has
				// stopped. Send neither a completion nor a host stop signal.
				original.closeInput(t)
				frame := original.readFrame(t)
				if !original.refused(t, frame, openCodeRefusalProviderFailed) {
					t.Fatal("original_disconnect_refused=false")
				}
			}

			status := func() []byte {
				return frozenCollectEnvelope(t, explicitFrozenReviewingStatus(t, repo, started.LineageID))
			}
			before := status()
			// Send the same original start, not first.Prompt: feeding back the
			// materialization would exercise re-interception, a different path.
			duplicate := startOpenCodeDuplicateRelayProcess(t, shim, host, start)
			second := duplicate.readFrame(t)
			dispatchable := openCodeDuplicateDispatchable(second)
			noncesDistinct := second.Nonce != "" && first.Nonce != second.Nonce
			refused, captured := false, false
			if dispatchable {
				// If erroneously authorized, offer valid synthetic reviewer
				// output on the second relay's own nonce to test capture too.
				payload := string(admittedReviewerPayloadForTest(t, repo, record, task.Agent, 0))
				duplicate.send(t, openCodeTransportEnvelope{
					Schema: openCodeReviewTransportSchema, Operation: "complete", Nonce: second.Nonce, Output: &payload,
				})
				duplicate.closeInput(t)
				frame := duplicate.readFrame(t)
				captured = duplicate.wait(t) == 0 && frame.Schema == openCodeReviewTransportSchema && frame.Operation == "result" && frame.Output != nil
			} else {
				duplicate.closeInput(t)
				refused = duplicate.refused(t, second, "")
			}
			statusChanged := !bytes.Equal(before, status())
			t.Logf("original_dispatchable=true relay_disconnected=%t host_stop_confirmed=false duplicate_dispatchable=%t nonces_distinct=%t duplicate_refused=%t duplicate_capture_result=%t status_changed=%t",
				scenario.disconnect, dispatchable, noncesDistinct, refused, captured, statusChanged)
			if dispatchable || !refused || captured || statusChanged {
				t.Error("the second original start must be refused without a dispatchable prompt, capture, or STATUS change")
			}
		})
	}
}

func openCodeDuplicateDispatchable(frame openCodeTransportEnvelope) bool {
	return frame.Schema == openCodeReviewTransportSchema && frame.Operation == "prompt" && frame.Nonce != "" &&
		strings.HasPrefix(frame.Prompt, openCodeTransportMaterializationHeader+" ")
}

type openCodeDuplicateRelayProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *json.Decoder
	stderr bytes.Buffer
}

func startOpenCodeDuplicateRelayProcess(t *testing.T, shim, host string, start openCodeTransportEnvelope) *openCodeDuplicateRelayProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, shim, "review", "opencode-transport")
	cmd.Dir = host
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"), "USERPROFILE=" + os.Getenv("HOME"), "TMPDIR=" + os.TempDir(),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
		openCodeRelayContractEnvironment + "=" + openCodeRelayContractV2,
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("could not open relay stdin")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("could not open relay stdout")
	}
	relay := &openCodeDuplicateRelayProcess{cmd: cmd, stdin: stdin, stdout: json.NewDecoder(stdout)}
	cmd.Stderr = &relay.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal("could not start Go relay subprocess")
	}
	t.Cleanup(func() {
		cancel()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	})
	relay.send(t, start)
	return relay
}

func (relay *openCodeDuplicateRelayProcess) send(t *testing.T, frame openCodeTransportEnvelope) {
	t.Helper()
	if err := json.NewEncoder(relay.stdin).Encode(frame); err != nil {
		t.Fatal("could not send relay frame")
	}
}

func (relay *openCodeDuplicateRelayProcess) readFrame(t *testing.T) openCodeTransportEnvelope {
	t.Helper()
	var frame openCodeTransportEnvelope
	if err := relay.stdout.Decode(&frame); err != nil {
		t.Fatal("expected a single JSON relay frame")
	}
	return frame
}

func (relay *openCodeDuplicateRelayProcess) closeInput(t *testing.T) {
	t.Helper()
	if err := relay.stdin.Close(); err != nil {
		t.Fatal("could not close relay stdin")
	}
}

func (relay *openCodeDuplicateRelayProcess) wait(t *testing.T) int {
	t.Helper()
	var extra json.RawMessage
	if err := relay.stdout.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatal("unexpected extra relay output")
	}
	_ = relay.cmd.Wait()
	return relay.cmd.ProcessState.ExitCode()
}

func (relay *openCodeDuplicateRelayProcess) refused(t *testing.T, frame openCodeTransportEnvelope, reason string) bool {
	t.Helper()
	return relay.wait(t) == 1 && relay.stderr.Len() > 0 &&
		frame.Schema == openCodeReviewTransportSchema && frame.Operation == "refused" &&
		frame.Prompt == "" && frame.Nonce == "" && frame.Output == nil &&
		slices.Contains(openCodeRefusalReasons, frame.Error) && (reason == "" || frame.Error == reason)
}
