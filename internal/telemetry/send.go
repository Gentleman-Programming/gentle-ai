package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PerformSend executes one queued event end to end: it POSTs payload to the
// resolved endpoint under the contract's timeouts and, on a 2xx response,
// updates state (records the send timestamp and resets counters after a
// successful heartbeat). A failed or non-2xx send records LastFailureAt (and
// touches nothing else, so the next opportunity still retries the same
// event kind) — that timestamp is what Opportunistic's FailureBackoff check
// reads to avoid respawning a sender against a blocked or unreachable
// endpoint on every trigger.
//
// It never prints the one-time disclosure notice and never touches
// notice_shown: that decision is made once, synchronously, by the triggering
// command itself (Opportunistic) before any send is ever attempted — see
// Opportunistic's enrollment step — precisely so the notice can never be
// printed after data has already left the machine.
//
// This is the body of the hidden `gentle-ai telemetry send` subcommand,
// which reads payload from its own stdin (never a file, so there is never a
// path to remove); SpawnDetachedSend launches that subcommand in the
// background, but PerformSend itself is called directly and synchronously so
// it stays unit-testable against an httptest.Server.
func PerformSend(homeDir string, payload []byte, getenv Getenv, now func() time.Time, client *http.Client) error {
	if now == nil {
		now = time.Now
	}
	if client == nil {
		client = NewHTTPClient()
	}

	var ev Event
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decode telemetry payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), TotalTimeout)
	defer cancel()
	ok, sendErr := PostEvent(ctx, client, Endpoint(getenv), payload)
	if sendErr != nil || !ok {
		recordSendFailure(homeDir, now)
		if sendErr != nil {
			return fmt.Errorf("send telemetry event: %w", sendErr)
		}
		return fmt.Errorf("telemetry endpoint rejected the event")
	}

	sentAt := now().UTC()
	err := Update(homeDir, func(s *State) {
		switch ev.Event {
		case EventInstall:
			s.LastInstallSentAt = &sentAt
		case EventHeartbeat:
			s.LastHeartbeatAt = &sentAt
			s.Counters = Counters{}
		}
	})
	if err != nil {
		return fmt.Errorf("persist telemetry state after send: %w", err)
	}
	return nil
}

// recordSendFailure persists LastFailureAt so Opportunistic's backoff check
// can see it on the next trigger. Best-effort: a failure to record the
// failure must not change what PerformSend itself already reports.
func recordSendFailure(homeDir string, now func() time.Time) {
	failedAt := now().UTC()
	_ = Update(homeDir, func(s *State) { s.LastFailureAt = &failedAt })
}
