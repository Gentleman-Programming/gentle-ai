package telemetry

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Only the envelope changes; direct delivery cannot reinterpret common public
// rows or serialize the ignored source-compatibility batch identity.
func TestRuntimeEventLocalParity(t *testing.T) {
	row := RuntimeRow{
		Model: RuntimeModel{Provider: "unknown", ID: "unknown"}, ModelEvidence: "unknown",
		AgentKind: "unknown", AgentClass: "unknown", SelectedEffort: "off", EffectiveEffort: "unsupported",
		Launches: json.RawMessage(`0`), Responses: json.RawMessage(`"unsupported"`),
		ErrorCategory: "unknown", Duration: RuntimeDuration{Kind: "unavailable", MeasuredCount: json.RawMessage(`0`), SumMS: json.RawMessage(`null`)},
	}
	for _, token := range []*json.RawMessage{&row.Input, &row.Output, &row.CacheRead, &row.CacheCreation, &row.ReasoningTokens, &row.TotalTokens} {
		*token = json.RawMessage(`{"reported":0,"unavailable":1,"unsupported":2,"sum":0}`)
	}
	batch := RuntimeBatch{Schema: RuntimeSchema, Registry: json.RawMessage(`1`), BatchID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Host: "pi", Rows: []RuntimeRow{row}}
	raw, _ := json.Marshal(batch)
	if bytes.Contains(raw, []byte("batch_id")) || bytes.Contains(raw, []byte(batch.BatchID)) {
		t.Fatal("source identity in stdin contract")
	}
	local, err := decodeRuntime(raw)
	if err != nil {
		t.Fatal(err)
	}
	event := RuntimeEvent{Schema: RuntimeEventSchema, Registry: json.RawMessage(`1`), DeliveryID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Host: batch.Host, Rows: batch.Rows}
	raw, _ = json.Marshal(event)
	remote, err := ParseRuntimeEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	localRows, _ := json.Marshal(local.Rows)
	remoteRows, _ := json.Marshal(remote.Rows)
	if string(localRows) != string(remoteRows) {
		t.Fatal("transport changed common rows")
	}
	if _, err := decodeRuntime(raw); err == nil {
		t.Fatal("transport accepted as local intake")
	}
	raw, _ = json.Marshal(batch)
	if _, err := ParseRuntimeEvent(raw); err == nil {
		t.Fatal("local batch accepted as transport")
	}
}
