package catalog

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// defaultLargeChangedLineThreshold is the minimum number of changed lines in a
// diff that triggers the full 4R review fan-out on pre-pr events.
//
// Token-budget rationale (three-tier model):
//   - Tier 1 (everyday): pre-commit and pre-push each run ONE cheap advisory lens
//     (~1x cost). This keeps the everyday development loop lightweight.
//   - Tier 2 (hot paths / large diffs): pre-pr on auth/update/security paths OR diffs
//     above this threshold fan out to all four 4R lenses (~4x cost). This gates the
//     expensive review behind meaningful signal.
//   - Tier 3 (high-stakes SDD): post-sdd-phase on design/apply runs judgment-day
//     (~4 + 3*findings cost). Reserved for high-stakes SDD phases only.
const defaultLargeChangedLineThreshold = 400

// on-ci and on-schedule have no built-in default because the appropriate
// agent/cadence is installation-specific; users opt in via override.

var defaultRuleSet = model.TriggerRuleSet{
	Events: []model.TriggerEvent{
		model.EventPreCommit,
		model.EventPrePush,
		model.EventPrePR,
		model.EventPostSDDPhase,
		model.EventOnCI,
		model.EventOnSchedule,
	},
	Bindings: []model.TriggerBinding{
		{
			On:   model.EventPreCommit,
			When: model.TriggerWhen{Always: true},
			Run:  []string{"review-readability"},
			Mode: model.ModeAdvisory,
			Reason: "everyday event → ONE cheap advisory lens (~1x); " +
				"full 4R fan-out reserved for pre-pr",
		},
		{
			On:   model.EventPrePush,
			When: model.TriggerWhen{Always: true},
			Run:  []string{"review-readability"},
			Mode: model.ModeAdvisory,
			Reason: "everyday event → ONE cheap advisory lens (~1x); " +
				"4R fan-out reserved for pre-pr on hot paths / large diffs",
		},
		{
			On: model.EventPrePR,
			When: model.TriggerWhen{
				PathGlobs:    []string{"**/auth/**", "**/update/**", "**/security/**", "**/payments/**"},
				MinDiffLines: defaultLargeChangedLineThreshold,
				Combine:      "or",
			},
			Run:  []string{"review-risk", "review-resilience", "review-readability", "review-reliability"},
			Mode: model.ModeStrong,
			Reason: "full 4R fan-out (~4x) only on hot paths (auth/update/security/payments) " +
				"or diffs exceeding 400 changed lines",
		},
		{
			On: model.EventPostSDDPhase,
			When: model.TriggerWhen{
				Phases: []string{"design", "apply"},
			},
			Run:  []string{"judgment-day"},
			Mode: model.ModeStrong,
			Reason: "adversarial verification (~4 + 3*findings cost) only at " +
				"high-stakes SDD phases (design and apply)",
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

// knownAgentList is the closed set of recognized agent identifiers.
// It covers the 4R review lenses, adversarial verification, and all SDD phases.
var knownAgentList = []string{
	// 4R review lenses
	"review-risk",
	"review-readability",
	"review-reliability",
	"review-resilience",
	// Adversarial verification
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
// trigger rule set. The default bindings are tuned so everyday events cost
// little (single advisory lens) and expensive lenses fire only when warranted
// (hot paths / large diffs / high-stakes SDD phases).
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
		// This keeps the everyday development loop lightweight (~1x cost).
		if (b.On == model.EventPreCommit || b.On == model.EventPrePush) && w.Always {
			if has4RFanOut(b.Run) {
				return fmt.Errorf(
					"binding[%d]: full 4R fan-out (review-risk, review-readability, review-reliability, review-resilience) "+
						"on %q with When.Always=true is prohibited — everyday events must use a single advisory lens, "+
						"not the full 4R fan-out (spec G token-budget rule)",
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
		agentRisk         = "review-risk"
		agentReadability  = "review-readability"
		agentReliability  = "review-reliability"
		agentResilience   = "review-resilience"
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
