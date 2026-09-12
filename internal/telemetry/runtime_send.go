package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"
)

const RuntimeSendTimeout = 3 * time.Second

// SendRuntime reads a sanitized aggregate and makes one best-effort HTTPS POST.
// It reads existing opt-out policy only: no enrollment, metric state, retry,
// subprocess, or cleanup. Hosts invoke the stdin command without awaiting it.
// The optional client is a transport seam for hermetic tests; production uses nil.
// Timeout covers HTTP including its bounded acknowledgement, not arbitrary
// caller-owned readers. The CLI pre-reads stdin under its own 500 ms deadline;
// embedded callers must provide bounded readers (for example bytes.Reader).
func SendRuntime(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) string {
	if !runtimeAllowed(home, getenv) {
		return "disabled"
	}
	data, err := io.ReadAll(io.LimitReader(input, RuntimeMaxBytes+1))
	if !runtimeAllowed(home, getenv) {
		return "disabled"
	}
	if err != nil || len(data) > RuntimeMaxBytes {
		return "discarded"
	}
	batch, err := decodeRuntime(data)
	if err != nil {
		return "discarded"
	}
	endpoint, err := url.Parse(Endpoint(getenv))
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.Opaque != "" {
		return "discarded"
	}
	endpoint.Path = "/v1/runtime-events"
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.ForceQuery = false
	endpoint.Fragment = ""
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return "discarded"
	}
	body, err := json.Marshal(RuntimeEvent{RuntimeEventSchema, batch.Registry, hex.EncodeToString(id[:]), batch.Host, batch.Rows})
	if err != nil || len(body) > RuntimeMaxBytes {
		return "discarded"
	}
	ctx, cancel := context.WithTimeout(ctx, RuntimeSendTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return "discarded"
	}
	req.Header.Set("Content-Type", "application/json")
	// No replayable request body or idempotency header. Even a transport-level
	// connection failure must not turn this observation into another attempt.
	req.GetBody = nil
	if client == nil {
		client = NewHTTPClient()
		defer client.CloseIdleConnections()
	}
	bounded := *client
	bounded.Timeout = RuntimeSendTimeout
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if !runtimeAllowed(home, getenv) {
		return "disabled"
	}
	resp, err := bounded.Do(req)
	if err != nil {
		return "discarded"
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(io.LimitReader(resp.Body, 1025))
	if err != nil || len(data) > 1024 || resp.StatusCode != http.StatusOK {
		return "discarded"
	}
	return runtimeAcknowledgement(data)
}

func runtimeAcknowledgement(data []byte) string {
	d := json.NewDecoder(bytes.NewReader(data))
	if runtimeUnique(d) != nil {
		return "discarded"
	}
	if _, err := d.Token(); err != io.EOF {
		return "discarded"
	}
	obj, ok := runtimeObject(data, "schema|decision")
	if !ok {
		return "discarded"
	}
	var schema, decision string
	if json.Unmarshal(obj["schema"], &schema) != nil || schema != RuntimeDeliverySchema || json.Unmarshal(obj["decision"], &decision) != nil || !runtimeMember(decision, "stored|duplicate") {
		return "discarded"
	}
	return decision
}
