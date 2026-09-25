package telemetrycollector

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v3/internal/telemetry"
)

// twoRowRuntimeFixture returns a valid runtime event with two distinct rows,
// so registry tests can assert both per-row accumulation (row/response/token
// counters) and per-delivery accumulation (deliveries_total) in one Observe.
func twoRowRuntimeFixture(t *testing.T) telemetry.RuntimeEvent {
	t.Helper()
	tokenA := `{"reported":1,"unavailable":2,"unsupported":3,"sum":999999999999}`
	rowA := `{"model":{"provider":"openai","id":"gpt-5.4"},"model_evidence":"response","agent_kind":"built_in","agent_class":"sdd-apply","selected_effort":"minimal","effective_effort":"medium","launches":null,"responses":6,`
	for _, key := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
		rowA += `"` + key + `":` + tokenA + `,`
	}
	rowA += `"error_category":"none","duration":{"kind":"message","measured_count":2,"sum_ms":1.25}}`

	tokenB := `{"reported":2,"unavailable":0,"unsupported":0,"sum":500}`
	rowB := `{"model":{"provider":"anthropic","id":"claude-hi"},"model_evidence":"selected","agent_kind":"orchestrator","agent_class":"orchestrator","selected_effort":"high","effective_effort":"high","launches":3,"responses":4,`
	for _, key := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_creation_tokens", "reasoning_tokens", "total_tokens"} {
		rowB += `"` + key + `":` + tokenB + `,`
	}
	rowB += `"error_category":"rate_limit","duration":{"kind":"unavailable","measured_count":0,"sum_ms":null}}`

	raw := []byte(`{"schema":"gentle-ai.telemetry-runtime-event/v1","registry":1,"delivery_id":"0123456789abcdef0123456789abcdef","host":"pi","rows":[` + rowA + `,` + rowB + `]}`)
	event, err := telemetry.ParseRuntimeEvent(raw)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return event
}

func TestRuntimeMetricsObserve_DeliveryAndRowCounters(t *testing.T) {
	m := NewRuntimeMetrics()
	m.Observe(twoRowRuntimeFixture(t))

	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`gentle_runtime_deliveries_total{host="pi"} 1`,
		`gentle_runtime_rows_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal"} 1`,
		`gentle_runtime_rows_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high"} 1`,
		`gentle_runtime_responses_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal"} 6`,
		`gentle_runtime_responses_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high"} 4`,
		`gentle_runtime_launches_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high"} 3`,
		`gentle_runtime_tokens_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",kind="input"} 999999999999`,
		`gentle_runtime_tokens_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high",kind="output"} 500`,
		`gentle_runtime_token_fields_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",kind="cache_read",state="reported"} 1`,
		`gentle_runtime_token_fields_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",kind="cache_read",state="unavailable"} 2`,
		`gentle_runtime_token_fields_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",kind="cache_read",state="unsupported"} 3`,
		`gentle_runtime_errors_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high",category="rate_limit"} 1`,
		`gentle_runtime_duration_ms_sum{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",duration_kind="message"} 1.25`,
		`gentle_runtime_duration_measured_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",duration_kind="message"} 2`,
		`gentle_runtime_rows_by_evidence_total{host="pi",model_evidence="response",effective_effort="medium"} 1`,
		`gentle_runtime_rows_by_evidence_total{host="pi",model_evidence="selected",effective_effort="high"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing line %q\nfull output:\n%s", want, out)
		}
	}

	// Zero-valued observations must not create series.
	for _, absent := range []string{
		`gentle_runtime_launches_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal"}`,
		`gentle_runtime_duration_measured_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high",duration_kind="unavailable"}`,
	} {
		if strings.Contains(out, absent) {
			t.Errorf("zero-valued series rendered %q:\n%s", absent, out)
		}
	}

	// error_category "none" must never create a series (rowA).
	if strings.Contains(out, `agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",category=`) {
		t.Errorf("row with error_category=none produced an errors series:\n%s", out)
	}
}

func TestRuntimeMetricsAdd_ZeroDeltaDoesNotCreateOrRemoveSeries(t *testing.T) {
	m := NewRuntimeMetrics()
	label := runtimeLabel{"host", "pi"}
	m.add("gentle_runtime_responses_total", 0, label)

	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo after absent zero: %v", err)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("absent zero-delta series rendered: %q", got)
	}

	m.add("gentle_runtime_responses_total", 3, label)
	buf.Reset()
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo after increment: %v", err)
	}
	before := buf.String()
	if !strings.Contains(before, `gentle_runtime_responses_total{host="pi"} 3`) {
		t.Fatalf("non-zero delta was not rendered: %q", before)
	}

	m.add("gentle_runtime_responses_total", 0, label)
	buf.Reset()
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo after existing zero: %v", err)
	}
	if got := buf.String(); got != before {
		t.Errorf("existing series changed after zero delta:\nbefore: %q\nafter: %q", before, got)
	}
}

func TestRuntimeMetricsWriteTo_IdleTTL(t *testing.T) {
	const metric = "gentle_runtime_responses_total"
	start := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	clock := start
	m := NewRuntimeMetricsWithTTL(time.Hour)
	m.now = func() time.Time { return clock }
	stale := runtimeLabel{"host", "stale"}
	active := runtimeLabel{"host", "active"}
	m.add(metric, 3, stale)
	m.add(metric, 5, active)

	clock = clock.Add(30 * time.Minute)
	m.add(metric, 2, active)
	m.add(metric, 0, stale) // zero is not an update
	clock = start.Add(time.Hour)
	assertMetricsContain(t, m, `gentle_runtime_responses_total{host="stale"} 3`)

	clock = start.Add(time.Hour + time.Nanosecond)
	out := metricsOutput(t, m)
	if !strings.Contains(out, `gentle_runtime_responses_total{host="stale"} 3`) {
		t.Errorf("idle series not rendered on eviction scrape: %q", out)
	}
	if next := metricsOutput(t, m); strings.Contains(next, `host="stale"`) {
		t.Errorf("idle series rendered after eviction: %q", next)
	}
	if !strings.Contains(out, `gentle_runtime_responses_total{host="active"} 7`) {
		t.Errorf("recently updated series missing: %q", out)
	}
	if _, exists := m.data[metric][runtimeSeriesKey([]runtimeLabel{stale})]; exists {
		t.Error("evicted series retained in registry")
	}

	clock = start.Add(2 * time.Hour)
	assertMetricsContain(t, m, `gentle_runtime_responses_total{host="active"} 7`)
	if out := metricsOutput(t, m); out != "" {
		t.Errorf("empty family still rendered: %q", out)
	}
	if _, exists := m.data[metric]; exists {
		t.Error("empty family retained in registry")
	}
	m.add(metric, 4, stale)
	assertMetricsContain(t, m, `gentle_runtime_responses_total{host="stale"} 4`)
}

type failingMetricsWriter struct{}

func (failingMetricsWriter) Write([]byte) (int, error) { return 0, errors.New("scrape write failed") }

func TestRuntimeMetricsWriteTo_WriteErrorPreservesExpiredSeries(t *testing.T) {
	const metric = "gentle_runtime_responses_total"
	clock := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	m := NewRuntimeMetricsWithTTL(time.Hour)
	m.now = func() time.Time { return clock }
	m.add(metric, 3, runtimeLabel{"host", "pi"})
	clock = clock.Add(2 * time.Hour)
	if _, err := m.WriteTo(failingMetricsWriter{}); err == nil {
		t.Fatal("expected write error")
	}
	if _, exists := m.data[metric]; !exists {
		t.Fatal("failed scrape evicted series")
	}
	assertMetricsContain(t, m, `gentle_runtime_responses_total{host="pi"} 3`)
	if out := metricsOutput(t, m); out != "" {
		t.Errorf("series survived successful eviction scrape: %q", out)
	}
}

func TestRuntimeMetricsWriteTo_ZeroTTLNeverEvicts(t *testing.T) {
	m := NewRuntimeMetricsWithTTL(0)
	clock := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return clock }
	m.add("gentle_runtime_responses_total", 3, runtimeLabel{"host", "pi"})
	clock = clock.Add(365 * 24 * time.Hour)
	assertMetricsContain(t, m, `gentle_runtime_responses_total{host="pi"} 3`)
}

func metricsOutput(t *testing.T, m *RuntimeMetrics) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.String()
}

func assertMetricsContain(t *testing.T, m *RuntimeMetrics, want string) {
	t.Helper()
	if got := metricsOutput(t, m); !strings.Contains(got, want) {
		t.Errorf("output missing %q: %q", want, got)
	}
}

func TestRuntimeMetricsObserve_ReportedOnlySkipsZeroCoverageStates(t *testing.T) {
	m := NewRuntimeMetrics()
	m.Observe(twoRowRuntimeFixture(t))

	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.String()
	prefix := `gentle_runtime_token_fields_total{host="pi",agent_kind="orchestrator",agent_class="orchestrator",provider="anthropic",model="claude-hi",selected_effort="high",kind="input",state="`
	if !strings.Contains(out, prefix+`reported"} 2`) {
		t.Fatalf("reported token coverage missing:\n%s", out)
	}
	for _, state := range []string{"unavailable", "unsupported"} {
		if strings.Contains(out, prefix+state+`"}`) {
			t.Errorf("zero %s token coverage rendered:\n%s", state, out)
		}
	}
}

func TestRuntimeMetricsObserve_DoublesOnSecondCall(t *testing.T) {
	m := NewRuntimeMetrics()
	event := twoRowRuntimeFixture(t)
	m.Observe(event)
	m.Observe(event)

	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `gentle_runtime_deliveries_total{host="pi"} 2`) {
		t.Errorf("deliveries_total did not double:\n%s", out)
	}
	if !strings.Contains(out, `gentle_runtime_rows_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal"} 2`) {
		t.Errorf("rows_total did not double:\n%s", out)
	}
	if !strings.Contains(out, `gentle_runtime_tokens_total{host="pi",agent_kind="built_in",agent_class="sdd-apply",provider="openai",model="gpt-5.4",selected_effort="minimal",kind="total"} 1999999999998`) {
		t.Errorf("tokens_total did not double:\n%s", out)
	}
}

func TestRuntimeMetricsObserve_SanitizesLabelsAndDefaultsEmptyToUnknown(t *testing.T) {
	m := NewRuntimeMetrics()
	event := telemetry.RuntimeEvent{
		Schema:     telemetry.RuntimeEventSchema,
		DeliveryID: "0123456789abcdef0123456789abcdef",
		Host:       "",
		Rows: []telemetry.RuntimeRow{
			{
				Model:           telemetry.RuntimeModel{Provider: `weird"prov\ider`, ID: "line\nbreak"},
				ModelEvidence:   "response",
				AgentKind:       "built_in",
				AgentClass:      "sdd-apply",
				SelectedEffort:  "minimal",
				EffectiveEffort: "minimal",
				Launches:        []byte("null"),
				Responses:       []byte("1"),
				Input:           []byte(`{"reported":1,"unavailable":0,"unsupported":0,"sum":1}`),
				Output:          []byte(`{"reported":1,"unavailable":0,"unsupported":0,"sum":1}`),
				CacheRead:       []byte(`{"reported":1,"unavailable":0,"unsupported":0,"sum":1}`),
				CacheCreation:   []byte(`{"reported":1,"unavailable":0,"unsupported":0,"sum":1}`),
				ReasoningTokens: []byte(`{"reported":1,"unavailable":0,"unsupported":0,"sum":1}`),
				TotalTokens:     []byte(`{"reported":1,"unavailable":0,"unsupported":0,"sum":1}`),
				ErrorCategory:   "none",
				Duration:        telemetry.RuntimeDuration{Kind: "message", MeasuredCount: []byte("1"), SumMS: []byte("1")},
			},
		},
	}
	m.Observe(event)

	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `gentle_runtime_deliveries_total{host="unknown"} 1`) {
		t.Errorf("empty host did not default to unknown:\n%s", out)
	}
	if !strings.Contains(out, `provider="weird\"prov\\ider"`) {
		t.Errorf("provider quote/backslash not escaped:\n%s", out)
	}
	if !strings.Contains(out, `model="line\nbreak"`) {
		t.Errorf("model newline not escaped:\n%s", out)
	}
}

func TestRuntimeMetricsWriteTo_DeterministicAcrossCalls(t *testing.T) {
	m := NewRuntimeMetrics()
	m.Observe(twoRowRuntimeFixture(t))

	var first, second bytes.Buffer
	if _, err := m.WriteTo(&first); err != nil {
		t.Fatalf("first WriteTo: %v", err)
	}
	if _, err := m.WriteTo(&second); err != nil {
		t.Fatalf("second WriteTo: %v", err)
	}
	if first.String() != second.String() {
		t.Errorf("WriteTo output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.String(), second.String())
	}
	if !strings.Contains(first.String(), "# TYPE gentle_runtime_rows_total counter\n") {
		t.Errorf("missing TYPE line for gentle_runtime_rows_total:\n%s", first.String())
	}
}
