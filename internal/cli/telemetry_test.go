package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

func TestTelemetryContractsArePinned(t *testing.T) {
	root := filepath.Join("..", "..", "contracts", "telemetry", "v1", "schemas")
	want := map[string]string{
		"event.schema.json":   "4246d49c1806bf967b1f1ce5ba38bf287f8d1d74c5b20149c787c7153d5baa0f",
		"status.schema.json":  "1ae7ee2f80b1cc51e66093e2a3ab6f10a85a940966c1e84451b44e9456dca822",
		"trigger.schema.json": "0e810811c5a673bc21f06a9fb0c5ce03841d46e3b35fc436b25860c2f0fded58",
	}
	for name, expected := range want {
		payload, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(payload)
		if actual := hex.EncodeToString(digest[:]); actual != expected {
			t.Fatalf("%s digest = %s, want %s", name, actual, expected)
		}
	}
}

// TestEventSchemaHasNoFreeTextStringProperties walks event.schema.json and
// fails if any "type": "string" property lacks enum, const, or pattern — the
// three shapes that close a string's value space. This is what makes it
// impossible to silently add a free-text field to the event contract: any
// new string property must be closed the moment it is added, or this test
// catches it before a reviewer has to.
func TestEventSchemaHasNoFreeTextStringProperties(t *testing.T) {
	path := filepath.Join("..", "..", "contracts", "telemetry", "v1", "schemas", "event.schema.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	var openStrings []string
	var walk func(node any, at string)
	walk = func(node any, at string) {
		obj, ok := node.(map[string]any)
		if !ok {
			return
		}
		if kind, _ := obj["type"].(string); kind == "string" {
			_, hasEnum := obj["enum"]
			_, hasPattern := obj["pattern"]
			_, hasConst := obj["const"]
			if !hasEnum && !hasPattern && !hasConst {
				openStrings = append(openStrings, at)
			}
		}
		if props, ok := obj["properties"].(map[string]any); ok {
			for name, child := range props {
				walk(child, at+"."+name)
			}
		}
		if items, ok := obj["items"]; ok {
			walk(items, at+"[]")
		}
	}
	walk(doc, "$")
	if len(openStrings) != 0 {
		t.Fatalf("event.schema.json has free-text string properties (no enum/const/pattern): %v", openStrings)
	}
}

func compileTelemetrySchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	root := filepath.Join("..", "..", "contracts", "telemetry", "v1", "schemas")
	compiler := jsonschema.NewCompiler()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		payload, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		location := "https://gentle-ai.dev/contracts/telemetry/v1/schemas/" + entry.Name()
		if err := compiler.AddResource(location, document); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("https://gentle-ai.dev/contracts/telemetry/v1/schemas/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func validateTelemetrySchema(t *testing.T, schema *jsonschema.Schema, payload []byte) {
	t.Helper()
	var document any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(document); err != nil {
		t.Fatal(err)
	}
}

// enableTelemetryForTest re-enables telemetry for one test by pinning every
// kill switch, so the test is hermetic on a CI runner or an opted-out shell.
func enableTelemetryForTest(t *testing.T) {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
}

func telemetryTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// Never let a test spawn a real telemetry sender or reach the network:
	// the CLI-level tests exercise decision-making and output shape, which
	// works identically whether or not a send would actually be attempted.
	t.Setenv("DO_NOT_TRACK", "1")
	// Pin the other kill switches too, so a test that re-enables telemetry
	// with DO_NOT_TRACK="" is hermetic on a CI runner or an opted-out shell.
	t.Setenv("GENTLE_AI_TELEMETRY", "")
	t.Setenv("CI", "")
	return home
}

func TestRunTelemetryPreviewJSONValidatesAgainstSchema(t *testing.T) {
	telemetryTestHome(t)
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"preview", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	schema := compileTelemetrySchema(t, "event.schema.json")
	validateTelemetrySchema(t, schema, buf.Bytes())

	var ev telemetry.Event
	if err := json.Unmarshal(buf.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Event != telemetry.EventInstall {
		t.Fatalf("first-ever preview event = %q, want %q", ev.Event, telemetry.EventInstall)
	}
	if ev.InstallID == "" {
		t.Fatal("preview must carry a real install_id")
	}
}

func TestRunTelemetryStatusJSONValidatesAgainstSchema(t *testing.T) {
	telemetryTestHome(t)
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"status", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	schema := compileTelemetrySchema(t, "status.schema.json")
	validateTelemetrySchema(t, schema, buf.Bytes())

	var result TelemetryStatusResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Source != string(telemetry.SourceDoNotTrack) {
		t.Fatalf("source = %q, want %q (DO_NOT_TRACK is set)", result.Source, telemetry.SourceDoNotTrack)
	}
	if result.Enabled {
		t.Fatal("enabled must be false when DO_NOT_TRACK=1")
	}
}

func TestRunTelemetryEnableDisableRoundTrip(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)

	var disableOut bytes.Buffer
	if err := RunTelemetry([]string{"disable", "--json"}, &disableOut); err != nil {
		t.Fatal(err)
	}
	var disabled TelemetryStatusResult
	if err := json.Unmarshal(disableOut.Bytes(), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || disabled.Source != string(telemetry.SourceStateDisable) {
		t.Fatalf("after disable: %+v", disabled)
	}

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Enabled {
		t.Fatal("disable must persist enabled=false")
	}

	var enableOut bytes.Buffer
	if err := RunTelemetry([]string{"enable", "--json"}, &enableOut); err != nil {
		t.Fatal(err)
	}
	var enabled TelemetryStatusResult
	if err := json.Unmarshal(enableOut.Bytes(), &enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled || enabled.Source != string(telemetry.SourceDefault) {
		t.Fatalf("after enable: %+v", enabled)
	}
}

// TestRunTelemetryStatusAndPreviewShareAStableInstallID replaces the old
// "status never creates a state file" assertion: EnsureState means the FIRST
// call to either status or preview does create the file (deliberately, so
// the id is stable from then on), and the property that actually matters —
// checked here — is that every subsequent call, from either command, reports
// exactly the same install_id. status still touches nothing else: enabled,
// notice_shown, and counters are unaffected by a bare `status`.
func TestRunTelemetryStatusAndPreviewShareAStableInstallID(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)

	var firstStatus bytes.Buffer
	if err := RunTelemetry([]string{"status", "--json"}, &firstStatus); err != nil {
		t.Fatal(err)
	}
	var first TelemetryStatusResult
	if err := json.Unmarshal(firstStatus.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.InstallID == "" {
		t.Fatal("status must report a real install_id even on its first call")
	}

	var preview bytes.Buffer
	if err := RunTelemetry([]string{"preview", "--json"}, &preview); err != nil {
		t.Fatal(err)
	}
	var ev telemetry.Event
	if err := json.Unmarshal(preview.Bytes(), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.InstallID != first.InstallID {
		t.Fatalf("preview install_id = %q, want the same %q status already reported", ev.InstallID, first.InstallID)
	}

	var secondStatus bytes.Buffer
	if err := RunTelemetry([]string{"status", "--json"}, &secondStatus); err != nil {
		t.Fatal(err)
	}
	var second TelemetryStatusResult
	if err := json.Unmarshal(secondStatus.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("a bare status call must never mutate state: first=%+v second=%+v", first, second)
	}

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.InstallID != first.InstallID {
		t.Fatalf("persisted install_id = %q, want %q", persisted.InstallID, first.InstallID)
	}
}

func TestRunTelemetryPreviewIsStableAcrossRepeatedCalls(t *testing.T) {
	telemetryTestHome(t)
	enableTelemetryForTest(t)

	var first, second bytes.Buffer
	if err := RunTelemetry([]string{"preview", "--json"}, &first); err != nil {
		t.Fatal(err)
	}
	if err := RunTelemetry([]string{"preview", "--json"}, &second); err != nil {
		t.Fatal(err)
	}
	var a, b telemetry.Event
	if err := json.Unmarshal(first.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if a.InstallID != b.InstallID {
		t.Fatalf("consecutive preview calls minted different install_ids: %q vs %q", a.InstallID, b.InstallID)
	}
}

// --- telemetry trigger ------------------------------------------------

func TestRunTelemetryTriggerDisabledValidatesAgainstSchema(t *testing.T) {
	telemetryTestHome(t) // DO_NOT_TRACK=1 by default
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"trigger", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	schema := compileTelemetrySchema(t, "trigger.schema.json")
	validateTelemetrySchema(t, schema, buf.Bytes())
	var result TelemetryTriggerResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Decision != "disabled" {
		t.Fatalf("decision = %q, want disabled", result.Decision)
	}
}

func TestRunTelemetryTriggerEnrolledValidatesAgainstSchema(t *testing.T) {
	telemetryTestHome(t)
	enableTelemetryForTest(t)
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"trigger", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	schema := compileTelemetrySchema(t, "trigger.schema.json")
	validateTelemetrySchema(t, schema, buf.Bytes())
	var result TelemetryTriggerResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Decision != "enrolled" {
		t.Fatalf("decision = %q, want enrolled on the very first trigger", result.Decision)
	}
}

func TestRunTelemetryTriggerRateLimitedValidatesAgainstSchema(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)
	sentInstall := time.Now().Add(-time.Hour).UTC()
	lastHeartbeat := time.Now().Add(-time.Minute).UTC()
	seed := telemetry.State{
		InstallID: "trigger-rate-limited", Enabled: true, NoticeShown: true,
		LastInstallSentAt: &sentInstall, LastHeartbeatAt: &lastHeartbeat,
	}
	if err := telemetry.Save(home, seed); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"trigger", "--json"}, &buf); err != nil {
		t.Fatal(err)
	}
	schema := compileTelemetrySchema(t, "trigger.schema.json")
	validateTelemetrySchema(t, schema, buf.Bytes())
	var result TelemetryTriggerResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Decision != "rate_limited" {
		t.Fatalf("decision = %q, want rate_limited within the 24h heartbeat window", result.Decision)
	}
}

// TestRunTelemetryTriggerCalledTwiceWithin24hSpawnsAtMostOneSend is the
// direct regression test for a host (e.g. Gentle Pi) that calls `telemetry
// trigger` on every session start: back-to-back calls inside the heartbeat
// window must never spawn a second sender.
func TestRunTelemetryTriggerCalledTwiceWithin24hSpawnsAtMostOneSend(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)
	telemetryTestSpawnRecorder.Reset()

	sentInstall := time.Now().Add(-48 * time.Hour).UTC()
	lastHeartbeat := time.Now().Add(-25 * time.Hour).UTC()
	seed := telemetry.State{
		InstallID: "trigger-host", Enabled: true, NoticeShown: true,
		LastInstallSentAt: &sentInstall, LastHeartbeatAt: &lastHeartbeat,
	}
	if err := telemetry.Save(home, seed); err != nil {
		t.Fatal(err)
	}

	var first bytes.Buffer
	if err := RunTelemetry([]string{"trigger", "--json"}, &first); err != nil {
		t.Fatal(err)
	}
	if got := len(telemetryTestSpawnRecorder.Calls()); got != 1 {
		t.Fatalf("spawn calls after the first trigger = %d, want exactly 1", got)
	}

	// The RecordingSpawner records a payload but never runs PerformSend, so
	// LastHeartbeatAt never advances on its own the way a real detached send
	// completing would. Simulate that completion so the second trigger below
	// exercises the 24h rate limit itself.
	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	justSent := time.Now().UTC()
	persisted.LastHeartbeatAt = &justSent
	if err := telemetry.Save(home, persisted); err != nil {
		t.Fatal(err)
	}

	var second bytes.Buffer
	if err := RunTelemetry([]string{"trigger", "--json"}, &second); err != nil {
		t.Fatal(err)
	}
	if got := len(telemetryTestSpawnRecorder.Calls()); got != 1 {
		t.Fatalf("spawn calls after a second trigger moments later = %d, want still exactly 1", got)
	}
}

func TestRunTelemetryUnknownSubcommand(t *testing.T) {
	telemetryTestHome(t)
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"bogus"}, &buf); err == nil {
		t.Fatal("want an error for an unknown telemetry subcommand")
	}
}

func TestRunTelemetrySendTakesNoArguments(t *testing.T) {
	telemetryTestHome(t)
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"send", "--payload-file", "whatever"}, &buf); err == nil {
		t.Fatal("want an error: send takes its payload from stdin, not a --payload-file flag")
	}
}

func TestRunTelemetrySendReadsPayloadFromStdinAndUpdatesState(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	t.Setenv(telemetry.EndpointEnvVar, server.URL)

	payload, err := telemetry.Marshal(telemetry.Build(telemetry.BuildInput{
		Kind: telemetry.EventInstall, InstallID: "install-from-stdin", Now: time.Now(),
	}))
	if err != nil {
		t.Fatal(err)
	}

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	go func() {
		_, _ = w.Write(payload)
		_ = w.Close()
	}()

	var buf bytes.Buffer
	if err := RunTelemetry([]string{"send"}, &buf); err != nil {
		t.Fatalf("send: %v", err)
	}

	loaded, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastInstallSentAt == nil {
		t.Fatal("a successful send must record LastInstallSentAt")
	}
}

func TestRunTelemetrySendRefusesOversizedStdin(t *testing.T) {
	telemetryTestHome(t)
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin })
	go func() {
		_, _ = w.Write(bytes.Repeat([]byte("x"), TelemetryStdinReadLimit+1))
		_ = w.Close()
	}()
	var buf bytes.Buffer
	if err := RunTelemetry([]string{"send"}, &buf); err == nil {
		t.Fatal("want an error for a payload larger than the contract's 4 KiB ceiling")
	}
}

// --- telemetryRecordReviewOutcome (finding 4: counters respect the kill
// switches, and checking them must never touch disk) -----------------------

func TestTelemetryRecordReviewOutcomeApprovedIncrementsExactlyOneCounter(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)

	telemetryRecordReviewOutcome("approved")

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	want := telemetry.Counters{ReviewsApproved: 1}
	if persisted.Counters != want {
		t.Fatalf("counters = %+v, want %+v", persisted.Counters, want)
	}
}

func TestTelemetryRecordReviewOutcomeCorrectionIncrementsExactlyOneCounter(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)

	telemetryRecordReviewOutcome("correction")

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	want := telemetry.Counters{ReviewsCorrection: 1}
	if persisted.Counters != want {
		t.Fatalf("counters = %+v, want %+v", persisted.Counters, want)
	}
}

func TestTelemetryRecordReviewOutcomeEscalatedIncrementsExactlyOneCounter(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)

	telemetryRecordReviewOutcome("escalated")

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	want := telemetry.Counters{ReviewsEscalated: 1}
	if persisted.Counters != want {
		t.Fatalf("counters = %+v, want %+v", persisted.Counters, want)
	}
}

// TestTelemetryRecordReviewOutcomeDisabledCreatesNoStateFile is the direct
// regression test for finding 4: recording a review outcome while telemetry
// is disabled by an environment switch must never resolve a home directory
// or touch disk at all, let alone create a state file.
func TestTelemetryRecordReviewOutcomeDisabledCreatesNoStateFile(t *testing.T) {
	home := telemetryTestHome(t) // telemetryTestHome already sets DO_NOT_TRACK=1

	telemetryRecordReviewOutcome("approved")

	if _, err := os.Stat(telemetry.Path(home)); !os.IsNotExist(err) {
		t.Fatal("a disabled review outcome must never create the telemetry state file")
	}
}

// TestTelemetryRecordReviewOutcomeStateDisabledSkipsAfterRead verifies the
// second half of the gate: when the environment allows telemetry but a
// previously-persisted `enabled: false` disables it, the counter is not
// incremented (reading the existing file is not the same as "no state file
// created" — this test seeds the file itself precisely to exercise that
// read, so the outer disabled test above stays about the disk-free path).
func TestTelemetryRecordReviewOutcomeStateDisabledSkipsAfterRead(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)
	if err := telemetry.Save(home, telemetry.State{InstallID: "seed", Enabled: false}); err != nil {
		t.Fatal(err)
	}

	telemetryRecordReviewOutcome("approved")

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.Counters.IsZero() {
		t.Fatalf("counters = %+v, want untouched zero while locally disabled", persisted.Counters)
	}
}

// TestTelemetryRecordReviewOutcomeTriggersAtMostOneHeartbeatPerDay is the
// direct regression test for the Gentle Pi scenario: a host that drives
// gentle-ai only through `review ...` must still get a heartbeat, but no
// more than the ordinary 24h limit already enforces. It seeds an install
// already past enrollment and its one-time install send, with a heartbeat
// window that has already expired, so the very first review closure below
// is exactly "an enrolled install with an expired heartbeat window".
func TestTelemetryRecordReviewOutcomeTriggersAtMostOneHeartbeatPerDay(t *testing.T) {
	home := telemetryTestHome(t)
	enableTelemetryForTest(t)
	telemetryTestSpawnRecorder.Reset()

	sentInstall := time.Now().Add(-48 * time.Hour).UTC()
	lastHeartbeat := time.Now().Add(-25 * time.Hour).UTC()
	seed := telemetry.State{
		InstallID: "review-only-host", Enabled: true, NoticeShown: true,
		LastInstallSentAt: &sentInstall, LastHeartbeatAt: &lastHeartbeat,
	}
	if err := telemetry.Save(home, seed); err != nil {
		t.Fatal(err)
	}

	telemetryRecordReviewOutcome("approved")
	if got := len(telemetryTestSpawnRecorder.Calls()); got != 1 {
		t.Fatalf("spawn calls after the first closure = %d, want exactly 1", got)
	}

	// The RecordingSpawner only records a payload; it never runs PerformSend,
	// so LastHeartbeatAt never advances on its own the way a real detached
	// send completing would. Simulate that completion here so the second
	// closure below exercises the 24h rate limit itself, not the unrelated
	// (and separately accepted) gap between "spawned" and "actually sent".
	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	justSent := time.Now().UTC()
	persisted.LastHeartbeatAt = &justSent
	if err := telemetry.Save(home, persisted); err != nil {
		t.Fatal(err)
	}

	telemetryRecordReviewOutcome("approved")
	if got := len(telemetryTestSpawnRecorder.Calls()); got != 1 {
		t.Fatalf("spawn calls after a second closure inside the 24h window = %d, want still exactly 1 (no new send)", got)
	}

	persisted, err = telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Counters.ReviewsApproved != 2 {
		t.Fatalf("ReviewsApproved = %d, want 2 (the counter still increments every time, only the send is rate-limited)", persisted.Counters.ReviewsApproved)
	}
}

// --- sdd_phase_runs (finding 3) --------------------------------------------

func settleOneSDDAttempt(t *testing.T, repo, change, suffix string) compactAttemptOutput {
	t.Helper()
	acquired, _ := runCompactSDDAttempt(t, []string{
		"acquire", "--cwd", repo, "--change", change,
		"--request-id", "telemetry-sdd-acquire-" + suffix,
		"--work-unit", "telemetry proof", "--evidence-goal", "prove sdd_phase_runs wiring",
		"--max-attempts", "12", "--max-changed-lines", "200",
	})
	if acquired.State != "proceed" {
		t.Fatalf("acquire = %#v", acquired)
	}
	settled, _ := runCompactSDDAttempt(t, []string{
		"settle", "--cwd", repo, "--change", change, "--token", acquired.Token,
		"--request-id", "telemetry-sdd-settle-" + suffix, "--outcome", "interrupted",
		"--diagnosis", "bounded execution was interrupted", "--harness-disposition", "reused",
		"--cleanup-evidence", "process group exited", "--process-evidence", "no descendants remained",
	})
	if settled.State != "proceed" {
		t.Fatalf("settle = %#v", settled)
	}
	return settled
}

func TestSDDAttemptSettleIncrementsSDDPhaseRunsExactlyOnce(t *testing.T) {
	home := reviewEnabledHome(t)
	enableTelemetryForTest(t)
	repo := initReviewCLIRepo(t)

	settleOneSDDAttempt(t, repo, "telemetry-sdd-phase", "1")

	persisted, err := telemetry.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Counters.SDDPhaseRuns != 1 {
		t.Fatalf("SDDPhaseRuns = %d, want 1", persisted.Counters.SDDPhaseRuns)
	}
}

func TestSDDAttemptSettleDisabledLeavesStateUntouched(t *testing.T) {
	home := reviewEnabledHome(t) // DO_NOT_TRACK stays at the TestMain default ("1")
	repo := initReviewCLIRepo(t)

	settleOneSDDAttempt(t, repo, "telemetry-sdd-phase-disabled", "2")

	if _, err := os.Stat(telemetry.Path(home)); !os.IsNotExist(err) {
		t.Fatal("a disabled sdd_phase_runs increment must never create the telemetry state file")
	}
}
