package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gentleman-programming/gentle-ai/v4/internal/starprompt"
)

// starTestEnv points the model's seams at a temp dir and a controllable clock.
type starTestEnv struct {
	m         Model
	dir       string
	now       time.Time
	ghCalls   []string
	ghStar    starprompt.Outcome
	ghStarred starprompt.Outcome
}

func newStarTestEnv(t *testing.T) *starTestEnv {
	t.Helper()
	env := &starTestEnv{
		m:         Model{},
		dir:       t.TempDir(),
		now:       time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		ghStar:    starprompt.OutcomeYes,
		ghStarred: starprompt.OutcomeNo,
	}
	origDir, origNow, origRunner, origGh := starStateDirFn, starTimeNow, starGhRunner, starGhPath
	t.Cleanup(func() {
		starStateDirFn, starTimeNow, starGhRunner, starGhPath = origDir, origNow, origRunner, origGh
	})
	starStateDirFn = func() string { return env.dir }
	starTimeNow = func() time.Time { return env.now }
	starGhRunner = func(ctx context.Context, ghPath string, args ...string) (int, string, error) {
		env.ghCalls = append(env.ghCalls, args[len(args)-1])
		outcome := env.ghStarred
		if len(args) > 0 && args[1] == "--method" {
			outcome = env.ghStar
		}
		switch outcome {
		case starprompt.OutcomeYes:
			return 0, "", nil
		case starprompt.OutcomeNo:
			return 1, "HTTP 404", nil
		default:
			return -1, "", context.DeadlineExceeded
		}
	}
	starGhPath = "gh"
	return env
}

func (env *starTestEnv) readState() starprompt.State {
	data, err := os.ReadFile(filepath.Join(env.dir, starprompt.FileName))
	if err != nil {
		return starprompt.Default()
	}
	var s starprompt.State
	_ = json.Unmarshal(data, &s)
	return s
}

func TestStarRecordOperationCountsAndPersists(t *testing.T) {
	env := newStarTestEnv(t)
	m := &env.m
	for i := 1; i <= starprompt.Threshold; i++ {
		if cmd := m.starRecordOperation(); cmd != nil {
			t.Fatalf("operation %d must stay silent", i)
		}
	}
	if env.readState().CompletedOps != starprompt.Threshold {
		t.Fatalf("counter not persisted: %+v", env.readState())
	}
	if cmd := m.starRecordOperation(); cmd == nil {
		t.Fatal("operation past threshold must arm the idle wait")
	}
}

func TestStarUnwritableStateDisablesAsking(t *testing.T) {
	env := newStarTestEnv(t)
	// A file where the state dir must be makes MkdirAll fail: writes are impossible.
	blocker := filepath.Join(env.dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	starStateDirFn = func() string { return filepath.Join(blocker, "state") }
	m := &env.m
	for i := 0; i < 10; i++ {
		if cmd := m.starRecordOperation(); cmd != nil {
			t.Fatal("unwritable state must never arm the offer")
		}
	}
	if !m.StarUnusable {
		t.Fatal("unwritable state must mark unusable")
	}
}

func TestStarChordsOnlyWhileVisible(t *testing.T) {
	env := newStarTestEnv(t)
	m := &env.m
	m.StarLoaded = true
	// Hidden: chords pass through untouched.
	if _, cmd, consumed := m.starObserveKey("ctrl+g"); consumed || cmd != nil {
		t.Fatal("hidden offer must not consume chords")
	}
	// Visible: ctrl+g stars through gh (direct flow) and dismisses forever.
	m.StarVisible = true
	m.StarState = starprompt.State{Schema: starprompt.Schema, CompletedOps: 50, DirectStar: true}
	updated, cmd, consumed := m.starObserveKey("ctrl+g")
	if !consumed || cmd == nil {
		t.Fatal("visible ctrl+g must be consumed with an action cmd")
	}
	if len(env.ghCalls) != 0 {
		t.Fatal("gh must run inside the cmd, not on keypress")
	}
	msg := cmd()
	done, ok := msg.(StarGhDoneMsg)
	if !ok || !done.Starred {
		t.Fatalf("ctrl+g must star via gh, got %+v", msg)
	}
	if cmd := updated.starHandleMsg(done); cmd != nil {
		t.Fatal("thanks handling must not schedule anything")
	}
	if !updated.StarState.Dismissed || updated.StarThanksAt.IsZero() {
		t.Fatalf("star must dismiss and thank: %+v", updated)
	}
	if s := env.readState(); !s.Dismissed {
		t.Fatalf("dismissal not persisted: %+v", s)
	}
}

func TestStarDismissChordPersistsForever(t *testing.T) {
	env := newStarTestEnv(t)
	m := &env.m
	m.StarLoaded = true
	m.StarVisible = true
	m.StarState = starprompt.State{Schema: starprompt.Schema, CompletedOps: 50}
	updated, cmd, consumed := m.starObserveKey("alt+d")
	if !consumed || cmd != nil {
		t.Fatal("alt+d must be consumed with no cmd")
	}
	if !updated.StarState.Dismissed || env.readState().Dismissed != true {
		t.Fatalf("alt+d must dismiss forever: %+v", env.readState())
	}
	if updated.StarVisible {
		t.Fatal("alt+d must hide the offer")
	}
}

func TestStarIdleFlowShowsOfferOncePerSession(t *testing.T) {
	env := newStarTestEnv(t)
	m := &env.m
	m.StarLoaded = true
	m.StarState = starprompt.State{Schema: starprompt.Schema, CompletedOps: 50}
	m.StarOffered = false

	if cmd := m.starArmIdle(); cmd == nil {
		t.Fatal("eligible model must arm the idle wait")
	}
	// The tick fires while the user is still active: it must re-arm.
	env.now = env.now.Add(starprompt.IdleDelay - time.Second)
	m.StarLastKey = env.now
	if cmd := m.starHandleMsg(StarIdleElapsedMsg{}); cmd == nil {
		t.Fatal("recent activity must re-arm the idle wait")
	}
	if m.StarVisible {
		t.Fatal("offer must not show while active")
	}
	// The re-armed window elapses fully idle: the offer shows.
	env.now = env.now.Add(starprompt.IdleDelay)
	if cmd := m.starHandleMsg(StarIdleElapsedMsg{}); cmd == nil {
		t.Fatal("idle window must schedule the visible timeout")
	}
	if !m.StarVisible {
		t.Fatal("idle window must show the offer")
	}
	if env.ghCalls != nil {
		t.Fatal("browser-only default must not read credentials")
	}
	// Unanswered until the visible timeout: auto-hide and stop for this session.
	env.now = env.now.Add(starprompt.VisibleTimeout)
	m.starHandleMsg(StarVisibleTimeoutMsg{})
	if m.StarVisible || !m.StarOffered {
		t.Fatalf("timeout must hide and mark offered: visible=%v offered=%v", m.StarVisible, m.StarOffered)
	}
	if cmd := m.starArmIdle(); cmd != nil {
		t.Fatal("once per session: no re-offer after timeout")
	}
}

func TestStarAlreadyStarredSkipsOfferWithDirectOptIn(t *testing.T) {
	env := newStarTestEnv(t)
	env.ghStarred = starprompt.OutcomeYes
	m := &env.m
	m.StarLoaded = true
	m.StarState = starprompt.State{Schema: starprompt.Schema, CompletedOps: 50, DirectStar: true}
	if cmd := m.starArmIdle(); cmd == nil {
		t.Fatal("must arm to reach the starred check")
	}
	env.now = env.now.Add(starprompt.IdleDelay)
	if cmd := m.starHandleMsg(StarIdleElapsedMsg{}); cmd != nil {
		t.Fatal("already starred must not schedule the visible timeout")
	}
	if m.StarVisible {
		t.Fatal("already starred must never show the offer")
	}
	if !env.readState().Dismissed {
		t.Fatal("already starred must persist the dismissal")
	}
}

func TestStarGhFailureFallsBackToBrowserAndManualURL(t *testing.T) {
	env := newStarTestEnv(t)
	m := &env.m
	m.StarLoaded = true
	m.StarState = starprompt.State{Schema: starprompt.Schema, CompletedOps: 50, DirectStar: true}
	env.ghStar = starprompt.OutcomeUnknown
	opened := false
	origOpen := tuiOpenBrowserFn
	t.Cleanup(func() { tuiOpenBrowserFn = origOpen })
	tuiOpenBrowserFn = func(url string) error { opened = true; return os.ErrPermission }

	m.StarVisible = true
	updated, cmd, consumed := m.starObserveKey("ctrl+g")
	if !consumed || cmd == nil {
		t.Fatal("chord must be consumed even when gh will fail")
	}
	msg := cmd()
	done, ok := msg.(StarGhDoneMsg)
	if !ok || done.Starred || done.Opened {
		t.Fatalf("gh failure with browser failure must surface the manual URL, got %+v", done)
	}
	_ = updated
	m.starHandleMsg(done)
	if !opened {
		t.Fatal("gh failure must fall back to attempting the browser")
	}
	if m.StarManualURL == "" {
		t.Fatal("manual URL must be shown when both paths fail")
	}
	if line := m.starFooterLine(); line == "" {
		t.Fatal("footer must carry the manual URL")
	}
}

func TestStarFooterLines(t *testing.T) {
	env := newStarTestEnv(t)
	m := &env.m
	if line := m.starFooterLine(); line != "" {
		t.Fatalf("silent model must render no star line, got %q", line)
	}
	m.StarVisible = true
	if line := m.starFooterLine(); line == "" || !strings.Contains(line, "Ctrl+G") || !strings.Contains(line, "Alt+D") {
		t.Fatalf("visible offer must render both chords, got %q", line)
	}
	m.StarVisible = false
	m.StarThanksAt = starTimeNow()
	if line := m.starFooterLine(); !strings.Contains(line, "Thanks for the star") {
		t.Fatalf("thanks line missing, got %q", line)
	}
	env.now = env.now.Add(6 * time.Second)
	if line := m.starFooterLine(); line != "" {
		t.Fatalf("thanks must expire, got %q", line)
	}
}
