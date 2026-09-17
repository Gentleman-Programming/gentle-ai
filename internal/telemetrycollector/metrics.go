package telemetrycollector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// runtimeMetricNames lists every metric family this registry can hold, in
// the fixed order WriteTo renders them. A family absent from an observed
// delivery is simply skipped, never rendered empty.
var runtimeMetricNames = []string{
	"gentle_runtime_deliveries_total",
	"gentle_runtime_rows_total",
	"gentle_runtime_responses_total",
	"gentle_runtime_launches_total",
	"gentle_runtime_tokens_total",
	"gentle_runtime_token_fields_total",
	"gentle_runtime_errors_total",
	"gentle_runtime_duration_ms_sum",
	"gentle_runtime_duration_measured_total",
	"gentle_runtime_rows_by_evidence_total",
}

// runtimeLabel is one ordered name/value pair of a rendered series. Order
// within a series is caller-determined (Observe always builds the same
// order for a given metric), never re-sorted: only series are sorted, not
// labels within a series.
type runtimeLabel struct {
	name  string
	value string
}

// runtimeSeries is one label combination's running counter value.
type runtimeSeries struct {
	labels []runtimeLabel
	value  float64
}

// RuntimeMetrics is a dependency-free, mutex-guarded registry of
// monotonically increasing float64 counters, keyed by metric name and an
// ordered label set. It never resets or decrements a counter: Observe only
// adds, and a counter reset only ever happens by process restart (handled
// downstream by increase()/rate() in VictoriaMetrics/Prometheus, not here).
// Deduplication of a repeated delivery is the caller's responsibility
// (runtime_handlers.go only calls Observe for a newly stored delivery), not
// this registry's: calling Observe twice with the same event simply doubles
// every counter it touches.
type RuntimeMetrics struct {
	mu   sync.Mutex
	data map[string]map[string]*runtimeSeries // metric name -> series key -> series
}

// NewRuntimeMetrics returns an empty registry ready for concurrent use.
func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{data: make(map[string]map[string]*runtimeSeries)}
}

// add increments (creating if absent) the series identified by metric and
// labels by delta. labels order is preserved verbatim in the rendered
// output; callers must pass the same order for the same metric every time.
func (m *RuntimeMetrics) add(metric string, delta float64, labels ...runtimeLabel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	family := m.data[metric]
	if family == nil {
		family = make(map[string]*runtimeSeries)
		m.data[metric] = family
	}
	key := runtimeSeriesKey(labels)
	series := family[key]
	if series == nil {
		series = &runtimeSeries{labels: labels}
		family[key] = series
	}
	series.value += delta
}

// runtimeSeriesKey builds a stable map key from a label set. \x1f (ASCII
// unit separator) can never appear in a sanitized label value, so this
// cannot collide two distinct label sets onto the same key.
func runtimeSeriesKey(labels []runtimeLabel) string {
	var b strings.Builder
	for _, l := range labels {
		b.WriteString(l.name)
		b.WriteByte('=')
		b.WriteString(l.value)
		b.WriteByte('\x1f')
	}
	return b.String()
}

// sanitizeRuntimeLabel escapes a label value for the Prometheus text
// exposition format (backslash, then double quote, then newline, in that
// order so escaping one never re-escapes another) and defaults an empty
// value to "unknown" so a series is never rendered with an empty label
// value.
func sanitizeRuntimeLabel(v string) string {
	if v == "" {
		return "unknown"
	}
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

// runtimeMetricNumber reads a canonicalized telemetry.RuntimeRow numeric
// field (a JSON number, "null", or a quoted sentinel such as
// `"unsupported"`) as a counter delta. "null" and any quoted string
// contribute 0: a row that could not observe a value still counts as one
// row, just with nothing to add.
func runtimeMetricNumber(raw json.RawMessage) float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	if raw[0] == '"' {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0
	}
	return f
}

// runtimeBaseLabels is the label set shared by every row-scoped metric
// (gentle_runtime_rows_total, gentle_runtime_responses_total,
// gentle_runtime_launches_total, gentle_runtime_tokens_total,
// gentle_runtime_token_fields_total, gentle_runtime_errors_total,
// gentle_runtime_duration_ms_sum, gentle_runtime_duration_measured_total).
func runtimeBaseLabels(host string, row telemetry.RuntimeRow) []runtimeLabel {
	return []runtimeLabel{
		{"host", host},
		{"agent_kind", sanitizeRuntimeLabel(row.AgentKind)},
		{"agent_class", sanitizeRuntimeLabel(row.AgentClass)},
		{"provider", sanitizeRuntimeLabel(row.Model.Provider)},
		{"model", sanitizeRuntimeLabel(row.Model.ID)},
		{"selected_effort", sanitizeRuntimeLabel(row.SelectedEffort)},
	}
}

// withLabel returns a fresh slice (base is never mutated in place) with one
// more label appended, so sibling series built from the same base never
// alias each other's backing array.
func withLabel(base []runtimeLabel, name, value string) []runtimeLabel {
	out := make([]runtimeLabel, len(base), len(base)+1)
	copy(out, base)
	return append(out, runtimeLabel{name, value})
}

// runtimeTokenFields are the six token coverage buckets a RuntimeRow
// carries, paired with the "kind" label value each one renders as.
var runtimeTokenFields = []string{"input", "output", "cache_read", "cache_creation", "reasoning", "total"}

func runtimeTokenRaw(row telemetry.RuntimeRow, kind string) json.RawMessage {
	switch kind {
	case "input":
		return row.Input
	case "output":
		return row.Output
	case "cache_read":
		return row.CacheRead
	case "cache_creation":
		return row.CacheCreation
	case "reasoning":
		return row.ReasoningTokens
	case "total":
		return row.TotalTokens
	default:
		return nil
	}
}

// Observe applies one delivery to the registry: one increment of
// gentle_runtime_deliveries_total, plus every row-scoped series for each of
// event.Rows. It is the caller's responsibility to call Observe at most
// once per newly stored delivery id (see runtime_handlers.go); Observe
// itself performs no deduplication.
func (m *RuntimeMetrics) Observe(event telemetry.RuntimeEvent) {
	host := sanitizeRuntimeLabel(event.Host)
	m.add("gentle_runtime_deliveries_total", 1, runtimeLabel{"host", host})

	for _, row := range event.Rows {
		base := runtimeBaseLabels(host, row)

		m.add("gentle_runtime_rows_total", 1, base...)
		m.add("gentle_runtime_responses_total", runtimeMetricNumber(row.Responses), base...)
		m.add("gentle_runtime_launches_total", runtimeMetricNumber(row.Launches), base...)

		for _, kind := range runtimeTokenFields {
			var token telemetry.RuntimeToken
			_ = json.Unmarshal(runtimeTokenRaw(row, kind), &token)
			withKind := withLabel(base, "kind", kind)
			m.add("gentle_runtime_tokens_total", runtimeMetricNumber(token.Sum), withKind...)
			m.add("gentle_runtime_token_fields_total", runtimeMetricNumber(token.Reported), withLabel(withKind, "state", "reported")...)
			m.add("gentle_runtime_token_fields_total", runtimeMetricNumber(token.Unavailable), withLabel(withKind, "state", "unavailable")...)
			m.add("gentle_runtime_token_fields_total", runtimeMetricNumber(token.Unsupported), withLabel(withKind, "state", "unsupported")...)
		}

		if category := row.ErrorCategory; category != "" && category != "none" {
			m.add("gentle_runtime_errors_total", 1, withLabel(base, "category", sanitizeRuntimeLabel(category))...)
		}

		withDurationKind := withLabel(base, "duration_kind", sanitizeRuntimeLabel(row.Duration.Kind))
		m.add("gentle_runtime_duration_ms_sum", runtimeMetricNumber(row.Duration.SumMS), withDurationKind...)
		m.add("gentle_runtime_duration_measured_total", runtimeMetricNumber(row.Duration.MeasuredCount), withDurationKind...)

		m.add("gentle_runtime_rows_by_evidence_total", 1,
			runtimeLabel{"host", host},
			runtimeLabel{"model_evidence", sanitizeRuntimeLabel(row.ModelEvidence)},
			runtimeLabel{"effective_effort", sanitizeRuntimeLabel(row.EffectiveEffort)},
		)
	}
}

// WriteTo renders every series as Prometheus text exposition (one "# TYPE"
// line per metric family, then one line per series), families in
// runtimeMetricNames order and series within a family sorted by their
// rendered label key, so two calls against the same state always produce
// byte-identical output.
func (m *RuntimeMetrics) WriteTo(w io.Writer) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var buf bytes.Buffer
	for _, metric := range runtimeMetricNames {
		family := m.data[metric]
		if len(family) == 0 {
			continue
		}
		keys := make([]string, 0, len(family))
		for k := range family {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		fmt.Fprintf(&buf, "# TYPE %s counter\n", metric)
		for _, k := range keys {
			series := family[k]
			buf.WriteString(metric)
			buf.WriteByte('{')
			for i, l := range series.labels {
				if i > 0 {
					buf.WriteByte(',')
				}
				buf.WriteString(l.name)
				buf.WriteString(`="`)
				buf.WriteString(l.value)
				buf.WriteByte('"')
			}
			buf.WriteString("} ")
			buf.WriteString(strconv.FormatFloat(series.value, 'f', -1, 64))
			buf.WriteByte('\n')
		}
	}
	return buf.WriteTo(w)
}
