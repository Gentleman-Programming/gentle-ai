package catalog

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// defaultLargeChangedLineThreshold remains the authored-code threshold used by
// the explicit review/start risk classifier. Generated goldens do not count
// toward this threshold, but they remain part of the immutable target tree.
const defaultLargeChangedLineThreshold = 400

// on-ci and on-schedule have no built-in default because the appropriate
// agent/cadence is installation-specific; users opt in via override.

var defaultRuleSet = model.TriggerRuleSet{
	Events: []model.TriggerEvent{
		model.EventPreCommit,
		model.EventPrePush,
		model.EventPrePR,
		model.EventRelease,
		model.EventPostSDDPhase,
		model.EventOnCI,
		model.EventOnSchedule,
	},
	Bindings: []model.TriggerBinding{
		{
			On:     model.EventPreCommit,
			When:   model.TriggerWhen{Always: true},
			Run:    []string{"review-receipt-validator"},
			Mode:   model.ModeStrong,
			Reason: "validate the staged/intended content against the existing receipt; never create a review budget",
		},
		{
			On:     model.EventPrePush,
			When:   model.TriggerWhen{Always: true},
			Run:    []string{"review-receipt-validator"},
			Mode:   model.ModeStrong,
			Reason: "validate pushed commits against the same content-bound receipt",
		},
		{
			On:     model.EventPrePR,
			When:   model.TriggerWhen{Always: true},
			Run:    []string{"review-receipt-validator"},
			Mode:   model.ModeStrong,
			Reason: "validate candidate tree, paths, policy, evidence, base relationship, and receipt without reopening review",
		},
		{
			On:     model.EventRelease,
			When:   model.TriggerWhen{Always: true},
			Run:    []string{"review-receipt-validator"},
			Mode:   model.ModeStrong,
			Reason: "validate immutable release tree, provenance, evidence, and publication boundary",
		},
		{
			On: model.EventPostSDDPhase,
			When: model.TriggerWhen{
				Phases: []string{"apply"},
			},
			Run:    []string{"review-start"},
			Mode:   model.ModeStrong,
			Reason: "explicitly start ordinary bounded implementation review after apply only when no valid receipt exists",
		},
	},
}

// SupportedTriggerEvents returns a defensive copy of the closed set of
// lifecycle events the orchestrator is told to recognize.
func SupportedTriggerEvents() []model.TriggerEvent {
	events := make([]model.TriggerEvent, len(defaultRuleSet.Events))
	copy(events, defaultRuleSet.Events)
	return events
}

// knownAgentList is the closed set of agent identifiers that may appear in a
// trigger binding's `run` list — strictly the trigger-bindable agents: the 4R
// review lenses, judgment-day, and all SDD phase identifiers. `review-refuter`
// is deliberately absent: refuters are invoked by the orchestrator inside a
// review's adversarial-verification step, never bound to a lifecycle event.
var knownAgentList = []string{
	// Native bounded-review lifecycle operations.
	"review-start",
	"review-receipt-validator",
	// 4R review lenses
	"review-risk",
	"review-readability",
	"review-reliability",
	"review-resilience",
	// Adversarial verification at SDD phase boundaries (trigger-bindable)
	"judgment-day",
	// SDD phase identifiers
	"sdd-explore",
	"sdd-propose",
	"sdd-spec",
	"sdd-design",
	"sdd-tasks",
	"sdd-apply",
	"sdd-verify",
	"sdd-archive",
}

// KnownAgents returns a defensive copy of the closed set of recognized agent
// identifiers (4R lenses + judgment-day + all SDD phase identifiers).
func KnownAgents() []string {
	agents := make([]string, len(knownAgentList))
	copy(agents, knownAgentList)
	return agents
}

// DefaultTriggerRuleSet returns a defensive copy of the built-in default
// trigger rule set. Post-apply starts an explicit bounded review only when no
// valid receipt exists. Commit, push, PR, and release events validate that same
// receipt and never create another review budget or launch Judgment Day.
//
// on-ci and on-schedule have no built-in default because the appropriate
// agent/cadence is installation-specific; users opt in via override.
func DefaultTriggerRuleSet() model.TriggerRuleSet {
	events := make([]model.TriggerEvent, len(defaultRuleSet.Events))
	copy(events, defaultRuleSet.Events)

	bindings := make([]model.TriggerBinding, len(defaultRuleSet.Bindings))
	for i, b := range defaultRuleSet.Bindings {
		// Deep-copy slices inside TriggerWhen and Run.
		bc := b
		if b.When.PathGlobs != nil {
			bc.When.PathGlobs = make([]string, len(b.When.PathGlobs))
			copy(bc.When.PathGlobs, b.When.PathGlobs)
		}
		if b.When.Phases != nil {
			bc.When.Phases = make([]string, len(b.When.Phases))
			copy(bc.When.Phases, b.When.Phases)
		}
		if b.Run != nil {
			bc.Run = make([]string, len(b.Run))
			copy(bc.Run, b.Run)
		}
		bindings[i] = bc
	}

	return model.TriggerRuleSet{
		Events:   events,
		Bindings: bindings,
	}
}

// ValidateTriggerRuleSet validates each binding in set against the closed
// vocabularies (events, agents, modes, when conditions). Returns a descriptive
// error on the first violation. Reason presence or absence is not validated.
func ValidateTriggerRuleSet(set model.TriggerRuleSet) error {
	supportedEvents := map[model.TriggerEvent]bool{}
	for _, e := range SupportedTriggerEvents() {
		supportedEvents[e] = true
	}

	knownAgents := map[string]bool{}
	for _, a := range KnownAgents() {
		knownAgents[a] = true
	}

	validModes := map[model.TriggerMode]bool{
		model.ModeAdvisory: true,
		model.ModeStrong:   true,
	}

	validSDDPhases := map[string]bool{
		"sdd-explore": true,
		"sdd-propose": true,
		"sdd-spec":    true,
		"sdd-design":  true,
		"sdd-tasks":   true,
		"sdd-apply":   true,
		"sdd-verify":  true,
		"sdd-archive": true,
		// Short names used in post-sdd-phase conditions.
		"explore": true,
		"propose": true,
		"spec":    true,
		"design":  true,
		"tasks":   true,
		"apply":   true,
		"verify":  true,
		"archive": true,
	}

	validCombine := map[string]bool{
		"":    true,
		"or":  true,
		"and": true,
	}

	for i, b := range set.Bindings {
		// Validate On.
		if !supportedEvents[b.On] {
			return fmt.Errorf("binding[%d]: unknown event %q", i, b.On)
		}

		// Validate Run.
		if len(b.Run) == 0 {
			return fmt.Errorf("binding[%d]: Run must not be empty", i)
		}
		for _, agent := range b.Run {
			if !knownAgents[agent] {
				return fmt.Errorf("binding[%d]: unknown run agent %q", i, agent)
			}
		}

		// Validate Mode.
		if !validModes[b.Mode] {
			return fmt.Errorf("binding[%d]: unknown mode %q", i, b.Mode)
		}

		// Validate When vocabulary.
		w := b.When
		// Must have at least one condition set.
		if !w.Always && len(w.PathGlobs) == 0 && w.MinDiffLines <= 0 && len(w.Phases) == 0 {
			return fmt.Errorf("binding[%d]: When must have at least one condition (Always, PathGlobs, MinDiffLines, or Phases)", i)
		}
		// PathGlobs non-nil but empty is invalid.
		if w.PathGlobs != nil && len(w.PathGlobs) == 0 {
			return fmt.Errorf("binding[%d]: When.PathGlobs must not be an empty slice", i)
		}
		// MinDiffLines when non-zero must be a positive integer (> 0).
		// Zero is valid as an unset/unused value; negative values are always rejected.
		if w.MinDiffLines < 0 {
			return fmt.Errorf("binding[%d]: When.MinDiffLines must be a positive integer (> 0)", i)
		}
		// Combine must be a recognized value.
		if !validCombine[w.Combine] {
			return fmt.Errorf("binding[%d]: When.Combine %q is not in {\"\" \"or\" \"and\"}", i, w.Combine)
		}
		// Phases must be recognized SDD phase identifiers.
		for _, p := range w.Phases {
			if !validSDDPhases[p] {
				return fmt.Errorf("binding[%d]: When.Phases entry %q is not a recognized SDD phase identifier", i, p)
			}
		}
		// Phases is only valid for post-sdd-phase event.
		if len(w.Phases) > 0 && b.On != model.EventPostSDDPhase {
			return fmt.Errorf("binding[%d]: When.Phases may only be used with the post-sdd-phase event (got %q)", i, b.On)
		}

		// Token-budget prohibition (spec G): the full 4R fan-out on an everyday event
		// (pre-commit or pre-push) with When.Always=true is actively prohibited.
		// Everyday events stay in the trivial/standard tiers (at most ONE lens,
		// ~1x cost); the full 4R fan-out belongs to pre-pr hot paths / large diffs.
		if (b.On == model.EventPreCommit || b.On == model.EventPrePush) && w.Always {
			if has4RFanOut(b.Run) {
				return fmt.Errorf(
					"binding[%d]: full 4R fan-out (review-risk, review-readability, review-reliability, review-resilience) "+
						"on %q with When.Always=true is prohibited — everyday events run at most a single lens, "+
						"never the full 4R fan-out (spec G token-budget rule)",
					i, b.On,
				)
			}
		}
	}

	return nil
}

// has4RFanOut reports whether run contains all four 4R review agents.
func has4RFanOut(run []string) bool {
	const (
		agentRisk        = "review-risk"
		agentReadability = "review-readability"
		agentReliability = "review-reliability"
		agentResilience  = "review-resilience"
	)
	found := 0
	for _, r := range run {
		switch r {
		case agentRisk, agentReadability, agentReliability, agentResilience:
			found++
		}
	}
	return found == 4
}
