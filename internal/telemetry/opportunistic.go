package telemetry

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Deps bundles the effectful dependencies Opportunistic needs, all
// swappable for tests: Now and Ctx default to real time and
// context.Background when left nil; Spawn falls back to the package-level
// DefaultSpawn (never to a hidden test-mode branch — see spawn.go).
type Deps struct {
	HomeDir    string
	Getenv     Getenv
	Now        func() time.Time
	Ctx        context.Context
	Spawn      Spawner
	Stderr     io.Writer
	Version    string
	Agents     []string
	Components []string
	RDDEnabled bool
}

// Decision names, exhaustively, what a single Opportunistic call did. It is
// the vocabulary `gentle-ai telemetry trigger --json` reports.
type TriggerDecision string

const (
	DecisionEnrolled      TriggerDecision = "enrolled"
	DecisionSentInstall   TriggerDecision = "sent_install"
	DecisionSentHeartbeat TriggerDecision = "sent_heartbeat"
	DecisionRateLimited   TriggerDecision = "rate_limited"
	DecisionBackoff       TriggerDecision = "backoff"
	DecisionDisabled      TriggerDecision = "disabled"
)

// Outcome reports what Opportunistic decided. It exists for logging, the
// `telemetry trigger` subcommand, and tests; callers never need to branch on
// it because a skipped or failed attempt is exactly as inert as never having
// called Opportunistic at all.
type Outcome struct {
	Attempted bool
	Kind      string          // EventInstall, EventHeartbeat, or "" when nothing was sent
	Decision  TriggerDecision // exhaustive machine-readable classification
	Source    Source          // the kill-switch source that decided it; SourceDefault when nothing opted out
	Reason    string          // human-readable detail, set only when Attempted is false
}

// Opportunistic is the single entry point install, update, and sync call at
// the end of a successful run.
//
// The very first time it ever runs for an install (persisted.NoticeShown is
// false), it does exactly one thing: print the one-time disclosure notice to
// Stderr, record notice_shown=true and the freshly assigned install_id, and
// return without sending anything. Nothing about this installation has left
// the machine yet at that point, so a user who disables telemetry before the
// next trigger never had anything sent. Only a later, separate trigger
// (should be the very next install/update/sync, or a 24h-later heartbeat)
// actually attempts a send.
//
// Once enrolled, it decides, from persisted state and the kill switches,
// whether an install or heartbeat event is due; if one is, it hands the
// built payload to Spawn, which performs the actual network call and
// updates state on success (see PerformSend). Opportunistic never returns an
// error: every failure is reported through Outcome.Reason and otherwise
// swallowed, because telemetry must never change a triggering command's exit
// code or output.
func Opportunistic(d Deps) Outcome {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Ctx == nil {
		d.Ctx = context.Background()
	}
	spawn := d.Spawn
	if spawn == nil {
		spawn = DefaultSpawn
	}

	// Decide BEFORE ever touching disk: EnsureState mints and persists an
	// install_id, so an opted-out host must never reach it.
	preState, err := loadForDecision(d.HomeDir)
	if err != nil {
		return Outcome{Decision: DecisionDisabled, Reason: "load state: " + err.Error()}
	}
	decision := Decide(d.Getenv, preState)
	if !decision.Enabled {
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "disabled by " + string(decision.Source)}
	}

	persisted, err := EnsureState(d.HomeDir)
	if err != nil {
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "ensure state: " + err.Error()}
	}

	if !persisted.NoticeShown {
		if d.Stderr == nil {
			// A machine-driven trigger (review or SDD closure) has nowhere to
			// show the notice. Enrollment waits for a human-facing command;
			// nothing is sent and nothing is recorded until then.
			return Outcome{Decision: DecisionEnrolled, Source: decision.Source, Reason: "enrollment pending: the notice is shown only by an interactive command"}
		}
		_, _ = fmt.Fprintln(d.Stderr, NoticeLine)
		persisted.NoticeShown = true
		if err := Save(d.HomeDir, persisted); err != nil {
			return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "persist enrollment: " + err.Error()}
		}
		return Outcome{Decision: DecisionEnrolled, Source: decision.Source, Reason: "enrolled: notice shown, first send deferred to the next trigger"}
	}

	now := d.Now()
	// Locked through the LastAttemptAt persist below, with persisted reloaded
	// fresh once held, so a concurrent writer can't be lost (R4).
	unlock, err := lockState(d.HomeDir)
	if err != nil {
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "lock state: " + err.Error()}
	}
	if persisted, err = Load(d.HomeDir); err != nil {
		unlock()
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "load state: " + err.Error()}
	}
	var kind string
	switch {
	case persisted.LastInstallSentAt == nil:
		// Sent once per install_id: the first opportunity from any trigger
		// (install, update, or sync) sends it, which also covers "an
		// existing install picks it up on its next update" for free.
		kind = EventInstall
	case persisted.LastHeartbeatAt == nil || now.Sub(*persisted.LastHeartbeatAt) >= HeartbeatInterval:
		kind = EventHeartbeat
	default:
		unlock()
		return Outcome{Decision: DecisionRateLimited, Source: decision.Source, Reason: "nothing due"}
	}

	if persisted.LastFailureAt != nil && now.Sub(*persisted.LastFailureAt) < FailureBackoff {
		unlock()
		return Outcome{Decision: DecisionBackoff, Source: decision.Source, Reason: "backing off after a recent failed send"}
	}
	// A spawn attempt was recorded recently and no success since then: the
	// prior child may still be in flight, or may have been killed before it
	// could record success or failure. Either way, do not fork another one
	// yet.
	if persisted.LastAttemptAt != nil && now.Sub(*persisted.LastAttemptAt) < FailureBackoff {
		success := persisted.LastInstallSentAt
		if persisted.LastHeartbeatAt != nil && (success == nil || persisted.LastHeartbeatAt.After(*success)) {
			success = persisted.LastHeartbeatAt
		}
		if success == nil || success.Before(*persisted.LastAttemptAt) {
			unlock()
			return Outcome{Decision: DecisionBackoff, Source: decision.Source, Reason: "recent attempt still in flight or failed"}
		}
	}

	ev := Build(BuildInput{
		Kind: kind, InstallID: persisted.InstallID, Now: now, Version: d.Version,
		Agents: d.Agents, Components: d.Components, RDDEnabled: d.RDDEnabled,
		Counters: persisted.Counters,
	})
	payload, err := Marshal(ev)
	if err != nil {
		unlock()
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "marshal event: " + err.Error()}
	}
	persisted.LastAttemptAt = &now
	saveErr := Save(d.HomeDir, persisted)
	unlock()
	if saveErr != nil {
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "persist attempt: " + saveErr.Error()}
	}
	if err := spawn(d.Ctx, payload); err != nil {
		return Outcome{Decision: DecisionDisabled, Source: decision.Source, Reason: "spawn sender: " + err.Error()}
	}
	sentDecision := DecisionSentInstall
	if kind == EventHeartbeat {
		sentDecision = DecisionSentHeartbeat
	}
	return Outcome{Attempted: true, Kind: kind, Decision: sentDecision, Source: decision.Source}
}
