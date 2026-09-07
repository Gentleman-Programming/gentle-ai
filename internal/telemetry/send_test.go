package telemetry

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPerformSendSuccessUpdatesState(t *testing.T) {
	home := t.TempDir()
	if err := Save(home, State{Enabled: true, NoticeShown: true, InstallID: "install-1"}); err != nil {
		t.Fatal(err)
	}

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	getenv := envMap(map[string]string{EndpointEnvVar: server.URL})
	now := func() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }

	firstPayload, err := Marshal(Build(BuildInput{Kind: EventInstall, InstallID: "install-1", Now: now()}))
	if err != nil {
		t.Fatal(err)
	}
	if err := PerformSend(home, firstPayload, getenv, now, NewHTTPClient()); err != nil {
		t.Fatalf("first send: %v", err)
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastInstallSentAt == nil || !loaded.LastInstallSentAt.Equal(now()) {
		t.Fatalf("LastInstallSentAt = %v, want %v", loaded.LastInstallSentAt, now())
	}

	secondPayload, err := Marshal(Build(BuildInput{Kind: EventHeartbeat, InstallID: "install-1", Now: now(), Counters: Counters{Syncs: 2}}))
	if err != nil {
		t.Fatal(err)
	}
	if err := PerformSend(home, secondPayload, getenv, now, NewHTTPClient()); err != nil {
		t.Fatalf("second send: %v", err)
	}
	loaded, err = Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Counters.IsZero() {
		t.Fatalf("counters after a successful heartbeat send = %+v, want reset to zero", loaded.Counters)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("server received %d requests, want 2", hits)
	}
}

func TestPerformSendNeverTouchesNoticeShown(t *testing.T) {
	// PerformSend must never manage the disclosure notice: Opportunistic's
	// enrollment step is the only place that ever flips notice_shown, and it
	// always does so before any send is attempted. This guards against the
	// exact defect this rework fixes: a notice printed only after data left
	// the machine.
	home := t.TempDir()
	if err := Save(home, State{Enabled: true, NoticeShown: false, InstallID: "install-1"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	getenv := envMap(map[string]string{EndpointEnvVar: server.URL})
	now := func() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }
	payload, err := Marshal(Build(BuildInput{Kind: EventInstall, InstallID: "install-1", Now: now()}))
	if err != nil {
		t.Fatal(err)
	}
	if err := PerformSend(home, payload, getenv, now, NewHTTPClient()); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.NoticeShown {
		t.Fatal("PerformSend must never set notice_shown; that is Opportunistic's enrollment step alone")
	}
}

func TestPerformSendFailureLeavesStateUntouched(t *testing.T) {
	home := t.TempDir()
	before := State{Enabled: true, NoticeShown: true, InstallID: "install-1", Counters: Counters{Syncs: 7}}
	if err := Save(home, before); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	getenv := envMap(map[string]string{EndpointEnvVar: server.URL})
	now := func() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }

	payload, err := Marshal(Build(BuildInput{Kind: EventHeartbeat, InstallID: "install-1", Now: now()}))
	if err != nil {
		t.Fatal(err)
	}
	if err := PerformSend(home, payload, getenv, now, NewHTTPClient()); err == nil {
		t.Fatal("want an error for a non-2xx response")
	}

	after, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if after.Counters != before.Counters || after.LastHeartbeatAt != nil {
		t.Fatalf("state changed after a failed send: before=%+v after=%+v", before, after)
	}
	if after.LastFailureAt == nil || !after.LastFailureAt.Equal(now()) {
		t.Fatalf("LastFailureAt = %v, want %v recorded for the backoff check", after.LastFailureAt, now())
	}
}

func TestPerformSendUnreachableServerLeavesStateUntouched(t *testing.T) {
	home := t.TempDir()
	before := State{Enabled: true, NoticeShown: true, InstallID: "install-1"}
	if err := Save(home, before); err != nil {
		t.Fatal(err)
	}
	// An address nothing listens on: exercises the connect-timeout path
	// without ever making a real network request to a live host.
	getenv := envMap(map[string]string{EndpointEnvVar: "http://127.0.0.1:1"})
	now := func() time.Time { return time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) }
	payload, err := Marshal(Build(BuildInput{Kind: EventInstall, InstallID: "install-1", Now: now()}))
	if err != nil {
		t.Fatal(err)
	}

	if err := PerformSend(home, payload, getenv, now, NewHTTPClient()); err == nil {
		t.Fatal("want an error when the endpoint is unreachable")
	}
	after, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastInstallSentAt != nil {
		t.Fatalf("state changed after an unreachable send: %+v", after)
	}
}
