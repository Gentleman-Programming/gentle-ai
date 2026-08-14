package main

import (
	"fmt"
	"strings"
)

// Transition journeys: states reached by SEQUENCE, not by damage.
//
// Every other journey in this corpus runs with a fixed configuration decided
// before the first command: SDD on or off, RDD on or off, never changed
// mid-flight. The damaged-store axis covers states reached by corrupting files
// the CLI cannot produce. Between those two lies the gap this axis exists for:
// states reached by two ordinary operations that are each individually valid.
//
// #2830 is why. `begin`, `finish failed`, `rescope`, `rescope` are four
// ordinary calls with no damaged file and no mode change, and the fourth
// leaves the attempt ledger permanently unreadable on a published build. Zero
// journeys chained them, so the corpus was green while the product could brick
// a change. A user found it, not us.
//
// Each journey here ends by reading the ledger back. That last assertion is
// the invariant #2830 broke and nothing checked: a refusal may reject an
// operation, and must never leave the store unable to answer.

const transitionAxis = "transition"

func init() {
	RegisterAxis(Axis{
		Name:     transitionAxis,
		Title:    "Journeys that change mode or chain operations in the middle of a lifecycle",
		BlackBox: true,
		Properties: []string{
			"Every state here is reached through the CLI alone. No fixture authors a store file, which is what separates this axis from damaged-store: these are sequences a user can type, not corruption a user cannot cause.",
			"Each journey asserts the ledger is still READABLE after the sequence, not merely that the last command refused. A refusal that leaves the store unable to answer is a wedge, and that distinction is the reason this axis exists.",
			"The sequences are drawn from what operators actually do between phases: narrow a scope twice, change their mind about the scope, turn review off mid-change and keep delivering. None of them require an unusual repository.",
		},
		Journeys: transitionJourneys,
	})
}

func transitionJourneys() []Journey {
	return []Journey{
		{
			ID:     "tr01-consecutive-rescope-keeps-the-ledger-readable",
			Title:  "Two rescopes in a row: whatever the second one answers, the ledger must still answer",
			Source: "community report #2830 on published 2.4.0-rc.1",
			// The reported shape, reproduced through the CLI only. The first
			// rescope succeeds. The second is where write-time preconditions
			// and replay-time preconditions disagree: replay requires the last
			// attempt to belong to the CURRENT objective, and after the first
			// rescope it belongs to the previous generation, so the record is
			// committed and then rejected by its own validator.
			//
			// #2830's ratified boundary rejects the second rescope before
			// publication. The unit regression pins that exact writer behavior;
			// this journey proves the operator-visible invariant that follows it:
			// `status` still works after the refusal.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an objective and fail it with the workspace unchanged",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAndFail},
				{Name: "narrow the objective once", Requires: sddAttemptRescopeCapability,
					Composite: transitionRescope("bench-rescope-one", "narrower work unit one")},
				{Name: "narrow it again, which is where the report lands",
					Requires:  sddAttemptRescopeCapability,
					Composite: transitionRescope("bench-rescope-two", "narrower work unit two")},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
			},
		},
		{
			ID:     "tr02-reset-after-rescope-keeps-the-ledger-readable",
			Title:  "Narrow, spend the budget, then reset: the widening route, driven end to end",
			Source: "#2769's documented exhaust-to-widen route + #2830's write-versus-replay surface",
			// What this proves, stated narrowly because two decoys narrowed it:
			// the exhaust-to-widen route that #2769's refusals advertise can be
			// walked end to end. Narrow the objective, spend the narrowed
			// budget, land on decision-required, reset from there.
			//
			// It does NOT guard the wedge class tr01 measures, and the honest
			// reason is worth keeping. An earlier version ran `reset` straight
			// after the rescope; injecting #2830's exact asymmetry into reset's
			// replay changed nothing, because reset is refused at write time in
			// that state and never reaches the publication path a wedge lives
			// on. Rebuilt to reach a reset that genuinely publishes, the same
			// injected asymmetry still does not fire, because by then the last
			// attempt belongs to the current objective and the condition is
			// unreachable. Reset does not have rescope's asymmetry.
			//
			// The assertion does bite: while the exhaust loop was misconfigured
			// against the pre-rescope work unit, every command "ran", nothing
			// errored, and this journey failed on the state it never reached.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an objective and fail it with the workspace unchanged",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAndFail},
				{Name: "narrow the objective", Requires: sddAttemptRescopeCapability,
					Composite: transitionRescope("bench-rescope-before-reset", "narrower before reset")},
				{Name: "spend the narrowed objective's remaining attempts",
					Requires: sddAttemptBeginCapability, Composite: transitionExhaustAttempts},
				{Name: "reset from decision-required, which is the route the refusals name",
					Requires: sddAttemptResetCapability, Composite: transitionReset("bench-reset-after-rescope")},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
			},
		},
		{
			ID:     "tr04-reset-then-rescope-keeps-the-ledger-readable",
			Title:  "The mirror of tr02: reset first, then narrow",
			Source: "systematic pairing of the mutating ledger verbs",
			// tr01 and tr02 cover rescope-then-X. This is X-then-rescope, and
			// it exists because the asymmetry #2830 exposed is about which
			// generation the last attempt belongs to, and reset advances that
			// generation too. If the defect is a property of generation
			// advancement rather than of rescope specifically, this is where
			// it appears.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an objective and fail it with the workspace unchanged",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAndFail},
				{Name: "spend the objective's attempts to reach a state reset publishes from",
					Requires: sddAttemptBeginCapability, Composite: transitionExhaustOriginal},
				{Name: "reset the objective", Requires: sddAttemptResetCapability,
					Composite: transitionReset("bench-reset-before-rescope")},
				{Name: "then narrow the fresh objective", Requires: sddAttemptRescopeCapability,
					Composite: transitionRescope("bench-rescope-after-reset", "narrower after reset")},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
			},
		},
		{
			ID:     "tr05-review-re-enabled-mid-change-keeps-sdd-moving",
			Title:  "Turn review back ON in the middle of a change that started without it",
			Source: "the mirror of tr03; the switch is documented as reversible",
			// tr03 proves work survives the switch going off. Nothing proved
			// the other direction, and it is the riskier one: turning review ON
			// mid-change means delivery starts demanding a receipt for work
			// that has none yet. If that wedges, a user who enables review to
			// be careful is punished for it, which is the opposite of what the
			// switch is for.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "turn review off before starting", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone")},
				{Name: "begin an objective and fail it with the workspace unchanged",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAndFail},
				{Name: "turn review back on mid-change", Requires: modeCapability,
					Args: productArgs("review", "mode", "enable")},
				{Name: "the ledger still answers with review back on", Composite: transitionProveLedgerReadable},
				{Name: "and the work continues", Requires: sddAttemptBeginCapability, Composite: transitionBeginAgain},
			},
		},
		{
			ID:     "tr06-mode-flip-during-an-active-attempt",
			Title:  "Flip the switch while an attempt is OPEN, not between attempts",
			Source: "the sequence tr03 and tr05 both avoid: no terminal boundary to land on",
			// tr03 and tr05 both flip the switch between attempts, which is the
			// polite moment. A user hits the switch when they are stuck, and
			// being stuck usually means an attempt is open. That is a harder
			// state: the ledger holds an active attempt whose settlement rules
			// were decided under the previous mode.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an attempt and leave it open", Requires: sddAttemptBeginCapability,
					Composite: transitionBeginOnly},
				{Name: "flip review off with the attempt still open", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone")},
				{Name: "the ledger still answers mid-attempt", Composite: transitionProveLedgerReadable},
				{Name: "and the open attempt can still be settled",
					Requires: sddAttemptFinishCapability, Composite: transitionFinishOpenAttempt},
				{Name: "the ledger still answers after settling", Composite: transitionProveLedgerReadable},
			},
		},
		{
			ID:     "tr07-reset-then-reset-keeps-the-ledger-readable",
			Title:  "Reset twice: the same-verb pair tr01 proves is dangerous for rescope",
			Source: "systematic pairing; tr01 shows a same-verb repeat can wedge",
			// tr01 established that repeating a generation-advancing verb is a
			// real hazard, and tr04 showed the hazard is not generic to
			// generation advancement. This closes the third corner: the other
			// same-verb repeat. If reset shares any part of rescope's
			// write-versus-replay asymmetry, repeating it is where it surfaces.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an objective and fail it with the workspace unchanged",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAndFail},
				{Name: "spend the attempts to reach a state reset publishes from",
					Requires: sddAttemptBeginCapability, Composite: transitionExhaustOriginal},
				{Name: "reset once", Requires: sddAttemptResetCapability,
					Composite: transitionReset("bench-reset-first")},
				{Name: "reset again immediately", Requires: sddAttemptResetCapability,
					Composite: transitionReset("bench-reset-second")},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
			},
		},
		{
			ID:     "tr08-scope-change-after-a-delegated-handoff",
			Title:  "Hand the attempt to a linked worktree, then change the scope",
			Source: "systematic pairing across surfaces; handoff rebinds the worktree an attempt belongs to",
			// Handoff moves an attempt's effective worktree, and a scope change
			// rewrites the objective that attempt belongs to. Both edit the
			// provenance the other reads, and no journey ran them in sequence.
			// #2783 is a live handoff defect on Windows, so this pair is not
			// theoretical: it is the neighbourhood of a known-bad surface.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddHandoffRuntimeRepo},
				{Name: "fixture: a delegated linked worktree", Fixture: sddHandoffDelegatedWorktree},
				{Name: "acquire, hand off, and settle in the delegated worktree",
					Requires: sddAttemptHandoffCapability, Composite: sddHandoffAndSettleDelegatedWorktree},
				{Name: "the ledger answers after the handoff", Composite: transitionProveLedgerReadable},
				{Name: "now change the scope of the handed-off objective",
					Requires:  sddAttemptRescopeCapability,
					Composite: transitionRescope("bench-rescope-after-handoff", "narrower after handoff")},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
			},
		},
		{
			ID:     "tr09-mode-flip-while-a-review-lineage-is-open",
			Title:  "Turn review off while a REVIEW is open, not merely while an attempt is",
			Source: "tr03 and tr06 flip the switch against SDD state; this flips it against review state",
			// The switch's own contract says delivery falls back to ordinary
			// repository policy. That is easy to honour when no review exists.
			// The interesting state is a started, unfinalized lineage: authority
			// exists, no receipt does, and the switch says stop consulting it.
			// If that combination wedges, the escape hatch fails exactly when
			// someone reaches for it.
			Steps: []Step{
				{Name: "fixture: repo with remote", Fixture: baseRepoWithRemote},
				{Name: "fixture: stage docs", Fixture: stageDocs("transition")},
				{Name: "start a review and leave it open", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "turn review off with the lineage still open", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone")},
				{Name: "review status still answers", Requires: statusCapability,
					Args: productArgs("review", "status")},
				{Name: "and the delivery gate answers under ordinary policy",
					Requires: validateCapability, Args: productArgs("review", "validate", "--gate", "pre-commit")},
			},
		},
		{
			ID:     "tr10-scope-change-after-the-review-is-bound",
			Title:  "Bind an approved review to the change, then move the objective under it",
			Source: "cross-surface pairing: the binding names an objective a scope change replaces",
			// The highest-value pair left after tr08, and the most plausible in
			// real use: you get a review approved, bind it to the change, and
			// then discover the scope was wrong. The binding records the
			// objective it was bound against; a rescope replaces that objective
			// with a new generation. Two records now disagree about which
			// objective is current, which is the same shape as #2830 one
			// surface over.
			//
			// Whether the rescope should be admitted here is a product
			// question. Whether both surfaces still answer afterwards is not.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin, fail, begin again", Requires: sddAttemptBeginCapability, Composite: sddBeginFailBegin},
				{Name: "fixture: the bounded correction moves the candidate", Fixture: sddBoundedCorrection},
				{Name: "review start on the corrected candidate", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "review finalize", Requires: finalizeCapability,
					Args: productArgs("review", "finalize"), After: rememberLineage},
				{Name: "bind the approved review to the change", Requires: bindSDDCapability,
					Composite: sddBindApprovedReview},
				{Name: "now move the objective the binding names",
					Requires:  sddAttemptRescopeCapability,
					Composite: transitionRescope("bench-rescope-after-bind", "narrower after binding")},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
				{Name: "and so does review status", Requires: statusCapability,
					Args: productArgs("review", "status")},
			},
		},
		{
			ID:     "tr11-abandon-the-review-while-an-attempt-is-open",
			Title:  "Quarantine the review out from under a running SDD attempt",
			Source: "cross-surface pairing: the two surfaces own separate state and nothing sequences them",
			// SDD attempts and review lineages are separate authorities that a
			// change uses together. Abandoning the review while an attempt is
			// open removes one of them mid-flight. Nothing chained these, and
			// the failure mode if they disagree is the worst kind: the work is
			// neither reviewable nor abandonable, with each surface pointing at
			// the other.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an attempt and leave it open", Requires: sddAttemptBeginCapability,
					Composite: transitionBeginOnly},
				{Name: "start a review alongside it", Requires: startCapability,
					Args: productArgs("review", "start"), After: rememberLineage},
				{Name: "abandon that review while the attempt runs", Requires: abandonCapability,
					Composite: abandonNonTerminalLineage},
				{Name: "the ledger still answers", Composite: transitionProveLedgerReadable},
				{Name: "and the open attempt can still be settled",
					Requires: sddAttemptFinishCapability, Composite: transitionFinishOpenAttempt},
			},
		},
		{
			ID:     "tr03-review-disabled-mid-change-keeps-sdd-moving",
			Title:  "Turn review off in the middle of a change: SDD must keep working under ordinary policy",
			Source: "maintainer-named coverage gap: no journey changes mode mid-lifecycle",
			// The kill switch is user-owned and documented as always available.
			// Every journey until now decided it before the first command, so
			// nothing proved a change already in flight survives the switch.
			//
			// The claim under test is the product's own: disabling review hands
			// delivery to ordinary repository policy and never wedges the work
			// that was already underway. If SDD cannot continue after the
			// switch, the switch is not the escape hatch it is advertised as.
			Steps: []Step{
				{Name: "fixture: repository with a committed OpenSpec change", Fixture: sddRuntimeRepo},
				{Name: "begin an objective and fail it with the workspace unchanged",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAndFail},
				{Name: "turn review off for this repository only", Requires: modeCapability,
					Args: productArgs("review", "mode", "disable", "--scope", "clone")},
				{Name: "the ledger still answers with review off", Composite: transitionProveLedgerReadable},
				{Name: "and the work continues: begin the next attempt",
					Requires: sddAttemptBeginCapability, Composite: transitionBeginAgain},
			},
		},
	}
}

// transitionBeginAndFail reaches the state every journey here builds on: one
// terminal failed attempt with the workspace untouched, which is the zero-drift
// shape rescope admits.
func transitionBeginAndFail(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-transition-begin", sddObjective...), false)

	if status, err = readRuntimeStatus(r); err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-transition-finish",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)

	status, err = readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if len(status.Attempts) != 1 {
		return fmt.Errorf("expected exactly one terminal attempt, got %d", len(status.Attempts))
	}
	return nil
}

// transitionRescope narrows the objective. A journey's following status check
// measures whether the result, including a correct refusal, left the store
// readable; #2830's unit regression separately pins its exact refusal.
func transitionRescope(requestID, workUnit string) func(*journeyRun) error {
	return func(r *journeyRun) error {
		status, err := readRuntimeStatus(r)
		if err != nil {
			return err
		}
		r.run(sddAttemptArgs(r, "rescope", status.Revision, requestID,
			"--work-unit", workUnit,
			"--evidence-goal", "bench proves the narrowed objective",
			"--max-attempts", "6", "--max-changed-lines", "600",
			"--reason", "the benchmark narrows this objective",
			"--actor", "bench maintainer"), false)
		return nil
	}
}

// narrowedObjective is the scope transitionRescope publishes. The exhaust loop
// must begin against THIS, not sddObjective: begin with a work unit other than
// the current objective's is refused as an objective change, so a loop using
// the original scope spins without ever creating an attempt. That mistake is
// worth naming because it is invisible from the outside: every command "ran",
// nothing errored, and the journey simply never reached the state it claimed.
var narrowedObjective = []string{
	"--work-unit", "narrower before reset",
	"--evidence-goal", "bench proves the narrowed objective",
	"--max-attempts", "6", "--max-changed-lines", "600",
}

// transitionExhaustOriginal spends the ORIGINAL objective's attempts, which is
// how tr04 reaches a state reset publishes from rather than one it refuses.
func transitionExhaustOriginal(r *journeyRun) error {
	return transitionExhaustWith(r, sddObjective, "bench-exhaust-original")
}

// transitionBeginOnly leaves an attempt open, which is the state a user is
// usually in when they reach for the kill switch.
func transitionBeginOnly(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-open-attempt", sddObjective...), false)
	status, err = readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if status.ActiveAttempt == nil {
		return fmt.Errorf("expected an open attempt, got none")
	}
	return nil
}

// transitionFinishOpenAttempt settles the attempt tr06 left open. An open
// attempt that cannot be closed after a mode flip is a wedge with extra steps.
func transitionFinishOpenAttempt(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "finish", status.Revision, "bench-settle-after-flip",
		append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
	status, err = readRuntimeStatus(r)
	if err != nil {
		return err
	}
	if status.ActiveAttempt != nil {
		return fmt.Errorf("the attempt could not be settled after the mode flip")
	}
	return nil
}

// transitionExhaustAttempts drives the narrowed objective to decision-required
// by failing attempts until the ledger stops offering begin. That is the state
// where reset genuinely publishes, which is what makes tr02 able to observe a
// wedge at all: an operation refused before publication cannot leave a bad
// record behind, so a journey that only reaches a refusal measures nothing.
func transitionExhaustAttempts(r *journeyRun) error {
	return transitionExhaustWith(r, narrowedObjective, "bench-exhaust")
}

func transitionExhaustWith(r *journeyRun, objective []string, prefix string) error {
	for attempt := 0; attempt < 8; attempt++ {
		status, err := readRuntimeStatus(r)
		if err != nil {
			return err
		}
		if status.NextAction != "begin" {
			return nil
		}
		id := fmt.Sprintf("%s-%d", prefix, attempt)
		r.run(sddAttemptArgs(r, "begin", status.Revision, id+"-begin", objective...), false)

		if status, err = readRuntimeStatus(r); err != nil {
			return err
		}
		r.run(sddAttemptArgs(r, "finish", status.Revision, id+"-finish",
			append([]string{"--outcome", "failed", "--evidence-revision", sddFailedEvidence}, sddTerminalEvidence...)...), false)
	}
	return fmt.Errorf("the objective never reached decision-required after eight attempts")
}

// transitionReset consumes the post-rescope state through reset instead.
func transitionReset(requestID string) func(*journeyRun) error {
	return func(r *journeyRun) error {
		status, err := readRuntimeStatus(r)
		if err != nil {
			return err
		}
		r.run(sddAttemptArgs(r, "reset", status.Revision, requestID,
			"--reason", "the benchmark changes the objective",
			"--actor", "bench maintainer"), false)
		return nil
	}
}

// transitionBeginAgain proves the lifecycle still moves, not merely that the
// store parses. A readable ledger that admits no next operation is still stuck.
func transitionBeginAgain(r *journeyRun) error {
	status, err := readRuntimeStatus(r)
	if err != nil {
		return err
	}
	r.run(sddAttemptArgs(r, "begin", status.Revision, "bench-transition-begin-again", sddObjective...), false)
	return nil
}

// transitionProveLedgerReadable is this axis's whole point.
//
// `sdd-attempt status` is read-only and must answer from any state a sequence
// of ordinary commands can reach. #2830 is exactly this assertion failing: the
// second rescope committed a record its own replay rejects, so every later read
// walked into it and the change could not be continued or abandoned.
//
// It reads through the product rather than the store files, so it measures what
// an operator sees.
func transitionProveLedgerReadable(r *journeyRun) error {
	observation := r.run([]string{"sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", sddChange}, false)
	if observation.ExitCode != 0 {
		return fmt.Errorf("the ledger stopped answering after this sequence: exit %d: %s",
			observation.ExitCode, firstLine(observation.Stderr))
	}
	if !strings.Contains(observation.Stdout, "\"revision\"") {
		return fmt.Errorf("sdd-attempt status answered without a revision: %s", firstLine(observation.Stdout))
	}
	return nil
}
