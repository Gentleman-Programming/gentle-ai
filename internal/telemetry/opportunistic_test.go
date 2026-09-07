package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type recordedSpawn struct {
	ctx     context.Context
	payload []byte
}

func fakeSpawner(calls *[]recordedSpawn) Spawner {
	return func(ctx context.Context, payload []byte) error {
		*calls = append(*calls, recordedSpawn{ctx: ctx, payload: append([]byte(nil), payload...)})
		return nil
	}
}

// enrollHome drives one Opportunistic call whose only job is to enroll (show
// the notice, persist notice_shown, mint the install_id) and returns the
// install_id it minted. Every test below that wants to observe a real send
// attempt must call this first, exactly as production does: the very first
// trigger never sends anything.
func enrollHome(t *testing.T, home string, now time.Time) (installID string, notice string) {
	t.Helper()
	var calls []recordedSpawn
	var stderr bytes.Buffer
	outcome := Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return now }, Spawn: fakeSpawner(&calls), Stderr: &stderr,
	})
	if outcome.Attempted {
		t.Fatalf("enrollment outcome = %+v, want nothing attempted (enrollment never sends)", outcome)
	}
	if len(calls) != 0 {
		t.Fatalf("enrollment spawned %d times, want 0", len(calls))
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if !s.NoticeShown {
		t.Fatal("enrollment must persist notice_shown=true")
	}
	if s.InstallID == "" {
		t.Fatal("enrollment must mint and persist an install_id")
	}
	return s.InstallID, stderr.String()
}

func TestOpportunisticFirstTriggerOnlyEnrollsAndNeverSends(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	installID, notice := enrollHome(t, home, now)
	if installID == "" {
		t.Fatal("want a minted install_id")
	}
	if !strings.Contains(notice, NoticeLine) {
		t.Fatalf("stderr = %q, want the notice line", notice)
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.LastInstallSentAt != nil {
		t.Fatal("enrollment must never record a send: nothing was sent yet")
	}
}

func TestOpportunisticSecondTriggerSendsInstall(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	installID, _ := enrollHome(t, home, now)

	var calls []recordedSpawn
	outcome := Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return now.Add(time.Minute) }, Spawn: fakeSpawner(&calls),
	})
	if !outcome.Attempted || outcome.Kind != EventInstall {
		t.Fatalf("second trigger outcome = %+v, want install attempted", outcome)
	}
	if len(calls) != 1 {
		t.Fatalf("spawn called %d times, want 1", len(calls))
	}
	var ev Event
	if err := json.Unmarshal(calls[0].payload, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.InstallID != installID {
		t.Fatalf("sent install_id = %q, want the enrolled %q", ev.InstallID, installID)
	}
	if ev.Event != EventInstall {
		t.Fatalf("sent event = %q, want %q", ev.Event, EventInstall)
	}
}

func TestOpportunisticNoticeIsNeverPrintedTwice(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enrollHome(t, home, now)

	var calls []recordedSpawn
	var stderr bytes.Buffer
	Opportunistic(Deps{HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return now.Add(time.Hour) }, Spawn: fakeSpawner(&calls), Stderr: &stderr})
	if stderr.Len() != 0 {
		t.Fatalf("stderr on the second trigger = %q, want no further notice", stderr.String())
	}
}

func TestOpportunisticHeartbeatRateLimit(t *testing.T) {
	home := t.TempDir()
	sentInstall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	lastHeartbeat := sentInstall.Add(2 * time.Hour)
	// Enrollment already happened, install already confirmed sent, and one
	// heartbeat already confirmed sent: the next opportunity is governed
	// purely by the 24h heartbeat interval.
	seed := State{Enabled: true, NoticeShown: true, InstallID: "seed-install-id", LastInstallSentAt: &sentInstall, LastHeartbeatAt: &lastHeartbeat}
	if err := Save(home, seed); err != nil {
		t.Fatal(err)
	}

	var calls []recordedSpawn
	within24h := lastHeartbeat.Add(23 * time.Hour)
	outcome := Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return within24h }, Spawn: fakeSpawner(&calls),
	})
	if outcome.Attempted {
		t.Fatalf("outcome = %+v, want nothing attempted within the 24h window", outcome)
	}

	after24h := lastHeartbeat.Add(25 * time.Hour)
	outcome = Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return after24h }, Spawn: fakeSpawner(&calls),
	})
	if !outcome.Attempted || outcome.Kind != EventHeartbeat {
		t.Fatalf("outcome = %+v, want heartbeat attempted after the 24h window", outcome)
	}
}

func TestOpportunisticFirstHeartbeatFiresImmediatelyAfterInstall(t *testing.T) {
	home := t.TempDir()
	sentInstall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seed := State{Enabled: true, NoticeShown: true, InstallID: "seed-install-id", LastInstallSentAt: &sentInstall}
	if err := Save(home, seed); err != nil {
		t.Fatal(err)
	}
	var calls []recordedSpawn
	outcome := Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return sentInstall.Add(time.Minute) }, Spawn: fakeSpawner(&calls),
	})
	if !outcome.Attempted || outcome.Kind != EventHeartbeat {
		t.Fatalf("outcome = %+v, want the first-ever heartbeat to fire without waiting 24h", outcome)
	}
}

func TestOpportunisticBacksOffAfterAFailureAndResumesAfterTheWindow(t *testing.T) {
	home := t.TempDir()
	sentInstall := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	failedAt := sentInstall.Add(time.Hour)
	seed := State{Enabled: true, NoticeShown: true, InstallID: "seed-install-id", LastInstallSentAt: &sentInstall, LastFailureAt: &failedAt}
	if err := Save(home, seed); err != nil {
		t.Fatal(err)
	}

	var calls []recordedSpawn
	withinBackoff := failedAt.Add(FailureBackoff - time.Minute)
	outcome := Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return withinBackoff }, Spawn: fakeSpawner(&calls),
	})
	if outcome.Attempted || outcome.Decision != DecisionBackoff {
		t.Fatalf("outcome = %+v, want backoff within the window", outcome)
	}
	if len(calls) != 0 {
		t.Fatalf("spawn called %d times during backoff, want 0", len(calls))
	}

	afterBackoff := failedAt.Add(FailureBackoff + time.Minute)
	outcome = Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return afterBackoff }, Spawn: fakeSpawner(&calls),
	})
	if !outcome.Attempted || outcome.Decision != DecisionSentHeartbeat {
		t.Fatalf("outcome = %+v, want a heartbeat attempted once the backoff window has passed", outcome)
	}
}

func TestOpportunisticDisabledByEachKillSwitch(t *testing.T) {
	cases := []struct {
		name  string
		env   map[string]string
		state State
	}{
		{"DO_NOT_TRACK", map[string]string{"DO_NOT_TRACK": "1"}, State{Enabled: true, NoticeShown: true}},
		{"GENTLE_AI_TELEMETRY", map[string]string{"GENTLE_AI_TELEMETRY": "0"}, State{Enabled: true, NoticeShown: true}},
		{"CI", map[string]string{"CI": "true"}, State{Enabled: true, NoticeShown: true}},
		{"state disabled", map[string]string{}, State{Enabled: false, NoticeShown: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			seedState := c.state
			seedState.InstallID = "seed"
			if err := Save(home, seedState); err != nil {
				t.Fatal(err)
			}
			var calls []recordedSpawn
			outcome := Opportunistic(Deps{HomeDir: home, Getenv: envMap(c.env), Spawn: fakeSpawner(&calls)})
			if outcome.Attempted {
				t.Fatalf("outcome = %+v, want disabled", outcome)
			}
			if len(calls) != 0 {
				t.Fatalf("spawn called %d times, want 0", len(calls))
			}
		})
	}
}

func TestOpportunisticDisabledDuringEnrollmentStillPersistsNothingBeyondTheKillSwitchCheck(t *testing.T) {
	// A brand-new install whose very first trigger already runs under a kill
	// switch: no notice, no send, and no enrollment bookkeeping either, since
	// the switch is checked before the notice step and before EnsureState
	// ever gets a chance to mint and persist an install_id.
	home := t.TempDir()
	var calls []recordedSpawn
	var stderr bytes.Buffer
	outcome := Opportunistic(Deps{
		HomeDir: home, Getenv: envMap(map[string]string{"DO_NOT_TRACK": "1"}), Spawn: fakeSpawner(&calls), Stderr: &stderr,
	})
	if outcome.Attempted {
		t.Fatalf("outcome = %+v, want disabled", outcome)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no notice while disabled", stderr.String())
	}
	if _, err := os.Stat(Path(home)); !os.IsNotExist(err) {
		t.Fatalf("state file stat err = %v, want a disabled trigger to leave no state file behind", err)
	}
}

func TestOpportunisticAttemptWindowThrottlesRespawnUntilSuccessOrWindowElapses(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enrollHome(t, home, now)

	var calls []recordedSpawn
	first := now.Add(time.Minute)
	outcome := Opportunistic(Deps{HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return first }, Spawn: fakeSpawner(&calls)})
	if !outcome.Attempted || len(calls) != 1 {
		t.Fatalf("first attempt outcome = %+v, calls = %d, want one attempted spawn", outcome, len(calls))
	}

	// A second trigger inside the attempt window, before any success or
	// failure was ever recorded (the detached child never finished): must
	// not fork another sender.
	second := first.Add(time.Minute)
	outcome = Opportunistic(Deps{HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return second }, Spawn: fakeSpawner(&calls)})
	if outcome.Attempted || outcome.Decision != DecisionBackoff || len(calls) != 1 {
		t.Fatalf("in-flight retry outcome = %+v, calls = %d, want backoff with still 1 spawn", outcome, len(calls))
	}

	// A recorded success after the attempt lifts the throttle even inside
	// the window (simulates PerformSend completing).
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	sentAt := second.Add(time.Second)
	s.LastInstallSentAt = &sentAt
	if err := Save(home, s); err != nil {
		t.Fatal(err)
	}
	third := second.Add(time.Minute)
	outcome = Opportunistic(Deps{HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return third }, Spawn: fakeSpawner(&calls)})
	if !outcome.Attempted || len(calls) != 2 {
		t.Fatalf("post-success outcome = %+v, calls = %d, want a second spawn once success was recorded", outcome, len(calls))
	}
}

func TestOpportunisticNeverErrorsWhenStateUnwritable(t *testing.T) {
	// Point HomeDir at a path whose parent cannot be created (a file where a
	// directory is expected), and confirm Opportunistic reports the failure
	// through Outcome rather than panicking or requiring the caller to
	// handle an error return.
	home := t.TempDir()
	blocker := home + "/blocked-parent"
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	var calls []recordedSpawn
	outcome := Opportunistic(Deps{HomeDir: blocker + "/child", Getenv: envMap(nil), Spawn: fakeSpawner(&calls)})
	if outcome.Attempted {
		t.Fatalf("outcome = %+v, want not attempted", outcome)
	}
	if outcome.Reason == "" {
		t.Fatal("want a non-empty reason explaining why nothing was attempted")
	}
}

func TestOpportunisticQuietTriggerNeverEnrollsSilently(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var calls []recordedSpawn
	// A closure hook has no stderr to show the notice on.
	outcome := Opportunistic(Deps{HomeDir: home, Getenv: envMap(nil), Now: func() time.Time { return now }, Spawn: fakeSpawner(&calls)})
	if outcome.Decision != DecisionEnrolled || outcome.Attempted || len(calls) != 0 {
		t.Fatalf("quiet first trigger = %+v (spawns %d), want enrolled with nothing attempted", outcome, len(calls))
	}
	s, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if s.NoticeShown {
		t.Fatal("a trigger that could not show the notice must not record it as shown")
	}
	// The next interactive trigger shows the notice; only then does enrollment complete.
	if _, notice := enrollHome(t, home, now); !strings.Contains(notice, NoticeLine) {
		t.Fatalf("stderr = %q, want the notice line", notice)
	}
}
