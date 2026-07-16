package catalog

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// 2.1 — SupportedTriggerEvents returns the closed lifecycle event set.
func TestSupportedTriggerEvents_ClosedSet(t *testing.T) {
	events := SupportedTriggerEvents()
	if len(events) != 7 {
		t.Fatalf("SupportedTriggerEvents() len = %d, want 7", len(events))
	}
	want := []model.TriggerEvent{
		model.EventPreCommit,
		model.EventPrePush,
		model.EventPrePR,
		model.EventRelease,
		model.EventPostSDDPhase,
		model.EventOnCI,
		model.EventOnSchedule,
	}
	got := map[model.TriggerEvent]bool{}
	for _, e := range events {
		got[e] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("SupportedTriggerEvents() missing %q", w)
		}
	}
}

// 2.2 — KnownAgents returns the closed agent set covering 4R, judgment-day, and 8 SDD phases.
func TestKnownAgents_ClosedSet(t *testing.T) {
	agents := KnownAgents()
	required := []string{
		// 4R review lenses
		"review-risk", "review-readability", "review-reliability", "review-resilience",
		// Adversarial
		"judgment-day",
		// SDD phases
		"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design",
		"sdd-tasks", "sdd-apply", "sdd-verify", "sdd-archive",
	}
	agentSet := map[string]bool{}
	for _, a := range agents {
		agentSet[a] = true
	}
	for _, r := range required {
		if !agentSet[r] {
			t.Errorf("KnownAgents() missing %q", r)
		}
	}
}

// 2.3 — DefaultTriggerRuleSet has exactly the token-shaped bindings described in the spec.
func TestDefaultTriggerRuleSet_TokenShape(t *testing.T) {
	rs := DefaultTriggerRuleSet()

	// Helper: filter bindings by event.
	bindingsFor := func(event model.TriggerEvent) []model.TriggerBinding {
		var out []model.TriggerBinding
		for _, b := range rs.Bindings {
			if b.On == event {
				out = append(out, b)
			}
		}
		return out
	}

	// (a) pre-commit: exactly one strong receipt validator; no reviewer launch.
	t.Run("pre-commit", func(t *testing.T) {
		bs := bindingsFor(model.EventPreCommit)
		if len(bs) != 1 {
			t.Fatalf("pre-commit bindings count = %d, want 1", len(bs))
		}
		b := bs[0]
		if b.Mode != model.ModeStrong {
			t.Errorf("pre-commit mode = %q, want strong", b.Mode)
		}
		if len(b.Run) != 1 || b.Run[0] != "review-receipt-validator" {
			t.Errorf("pre-commit run = %v, want [review-receipt-validator]", b.Run)
		}
		if !b.When.Always {
			t.Error("pre-commit When.Always = false, want true")
		}
	})

	// (b) pre-push validates the same receipt and launches no review lenses.
	t.Run("pre-push", func(t *testing.T) {
		bs := bindingsFor(model.EventPrePush)
		if len(bs) != 1 {
			t.Fatalf("pre-push bindings count = %d, want 1", len(bs))
		}
		b := bs[0]
		if b.Mode != model.ModeStrong {
			t.Errorf("pre-push mode = %q, want strong", b.Mode)
		}
		if len(b.Run) != 1 || b.Run[0] != "review-receipt-validator" {
			t.Errorf("pre-push run = %v, want [review-receipt-validator]", b.Run)
		}
		if !b.When.Always {
			t.Error("pre-push When.Always = false, want true")
		}
		// Must NOT include all four 4R agents simultaneously.
		has4R := false
		reviewAgents := 0
		for _, r := range b.Run {
			switch r {
			case "review-risk", "review-readability", "review-reliability", "review-resilience":
				reviewAgents++
			}
		}
		if reviewAgents == 4 {
			has4R = true
		}
		if has4R {
			t.Error("pre-push must not include all four 4R agents simultaneously")
		}
	})

	// (c) pre-pr validates tree/base/receipt and never starts full 4R.
	t.Run("pre-pr", func(t *testing.T) {
		bs := bindingsFor(model.EventPrePR)
		if len(bs) != 1 {
			t.Fatalf("pre-pr bindings count = %d, want 1", len(bs))
		}
		b := bs[0]
		if b.Mode != model.ModeStrong {
			t.Errorf("pre-pr mode = %q, want strong", b.Mode)
		}
		if len(b.Run) != 1 || b.Run[0] != "review-receipt-validator" || !b.When.Always {
			t.Errorf("pre-pr binding = %#v, want always receipt validator", b)
		}
	})

	// (d) post-sdd-phase starts ordinary review after apply only.
	t.Run("post-sdd-phase", func(t *testing.T) {
		bs := bindingsFor(model.EventPostSDDPhase)
		if len(bs) != 1 {
			t.Fatalf("post-sdd-phase bindings count = %d, want 1", len(bs))
		}
		b := bs[0]
		if b.Mode != model.ModeStrong {
			t.Errorf("post-sdd-phase mode = %q, want strong", b.Mode)
		}
		if len(b.Run) != 1 || b.Run[0] != "review-start" {
			t.Errorf("post-sdd-phase run = %v, want [review-start]", b.Run)
		}
		phaseSet := map[string]bool{}
		for _, p := range b.When.Phases {
			phaseSet[p] = true
		}
		if !phaseSet["apply"] {
			t.Error("post-sdd-phase When.Phases missing apply")
		}
		// No other phase names.
		for p := range phaseSet {
			if p != "apply" {
				t.Errorf("post-sdd-phase When.Phases contains unexpected phase %q", p)
			}
		}
	})

	// (e) on-ci: zero bindings.
	t.Run("on-ci-zero", func(t *testing.T) {
		bs := bindingsFor(model.EventOnCI)
		if len(bs) != 0 {
			t.Errorf("on-ci bindings count = %d, want 0", len(bs))
		}
	})

	// (f) on-schedule: zero bindings.
	t.Run("on-schedule-zero", func(t *testing.T) {
		bs := bindingsFor(model.EventOnSchedule)
		if len(bs) != 0 {
			t.Errorf("on-schedule bindings count = %d, want 0", len(bs))
		}
	})

	// (g) Every emitted binding has a non-empty Reason field.
	t.Run("all-bindings-have-reason", func(t *testing.T) {
		for i, b := range rs.Bindings {
			if b.Reason == "" {
				t.Errorf("binding[%d] (on=%q) has empty Reason", i, b.On)
			}
		}
	})

	// (g2) Lifecycle reasons describe receipt reuse rather than reviewer fan-out.
	t.Run("reasons-use-bounded-lifecycle-language", func(t *testing.T) {
		for i, b := range rs.Bindings {
			if strings.Contains(strings.ToLower(b.Reason), "advisory") {
				t.Errorf("binding[%d] (on=%q) Reason still uses v1 advisory language: %q", i, b.On, b.Reason)
			}
		}
		for _, event := range []model.TriggerEvent{model.EventPreCommit, model.EventPrePush, model.EventPrePR, model.EventRelease} {
			for _, b := range bindingsFor(event) {
				if !strings.Contains(strings.ToLower(b.Reason), "receipt") && event != model.EventRelease {
					t.Errorf("%q binding Reason must state receipt validation; got %q", event, b.Reason)
				}
			}
		}
	})

	// (h) judgment-day does NOT appear in any pre-commit or pre-push binding.
	t.Run("judgment-day-not-in-pre-commit-push", func(t *testing.T) {
		for _, event := range []model.TriggerEvent{model.EventPreCommit, model.EventPrePush} {
			for _, b := range bindingsFor(event) {
				for _, r := range b.Run {
					if r == "judgment-day" {
						t.Errorf("%q binding contains judgment-day in Run, which is not allowed", event)
					}
				}
			}
		}
	})
}

// 2.4 — DefaultTriggerRuleSet returns a defensive copy.
func TestDefaultTriggerRuleSet_CopyIsolation(t *testing.T) {
	rs1 := DefaultTriggerRuleSet()
	originalLen := len(rs1.Bindings)
	// Mutate the returned slice.
	rs1.Bindings = append(rs1.Bindings, model.TriggerBinding{On: model.EventOnCI})

	rs2 := DefaultTriggerRuleSet()
	if len(rs2.Bindings) != originalLen {
		t.Errorf("DefaultTriggerRuleSet() not isolated: second call returned %d bindings, want %d", len(rs2.Bindings), originalLen)
	}
}

// 2.5 — The authored-code threshold remains canonical for review/start risk classification.
func TestDefaultTriggerRuleSet_ThresholdConstant(t *testing.T) {
	if defaultLargeChangedLineThreshold != 400 {
		t.Errorf("defaultLargeChangedLineThreshold = %d, want 400", defaultLargeChangedLineThreshold)
	}
}

// 2.6 — ValidateTriggerRuleSet table-driven success/failure cases.
func TestValidateTriggerRuleSet(t *testing.T) {
	tests := []struct {
		name    string
		set     model.TriggerRuleSet
		wantErr bool
	}{
		{
			name:    "default set passes",
			set:     DefaultTriggerRuleSet(),
			wantErr: false,
		},
		{
			name: "well-formed custom binding passes",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{PathGlobs: []string{"**/auth/**"}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "unknown run entry rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPreCommit,
						Run:  []string{"review-seo"},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown on value rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.TriggerEvent("post-merge"),
						Run:  []string{"review-risk"},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "unknown mode rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPreCommit,
						Run:  []string{"review-readability"},
						Mode: model.TriggerMode("blocking"),
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "MinDiffLines <= 0 rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{MinDiffLines: 0},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "PathGlobs empty slice rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{PathGlobs: []string{}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid Combine rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{PathGlobs: []string{"**/auth/**"}, Combine: "xor"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Phases with unrecognized phase rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPostSDDPhase,
						Run:  []string{"judgment-day"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{Phases: []string{"not-a-phase"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Phases used on non-post-sdd-phase event rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{Phases: []string{"design"}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty Run rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPreCommit,
						Run:  []string{},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no when condition set rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPreCommit,
						Run:  []string{"review-readability"},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{}, // all zero
					},
				},
			},
			wantErr: true,
		},
		// W1: MinDiffLines=0 with PathGlobs passes — 0 is not an active numeric
		// condition; PathGlobs provides the sole condition. The binding is valid.
		// This proves MinDiffLines:0 is not silently treated as a meaningful threshold.
		{
			name: "MinDiffLines zero with valid PathGlobs passes",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{MinDiffLines: 0, PathGlobs: []string{"**/auth/**"}},
					},
				},
			},
			wantErr: false,
		},
		// W1: negative MinDiffLines is invalid — must be a positive integer (> 0).
		{
			name: "MinDiffLines negative rejected",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{MinDiffLines: -1, PathGlobs: []string{"**/auth/**"}},
					},
				},
			},
			wantErr: true,
		},
		// W2: full 4R fan-out on pre-commit with always=true is prohibited (token-budget rule).
		{
			name: "pre-commit always with all 4R agents prohibited",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPreCommit,
						Run:  []string{"review-risk", "review-readability", "review-reliability", "review-resilience"},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: true,
		},
		// W2: full 4R fan-out on pre-push with always=true is prohibited (token-budget rule).
		{
			name: "pre-push always with all 4R agents prohibited",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePush,
						Run:  []string{"review-risk", "review-readability", "review-reliability", "review-resilience"},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: true,
		},
		// W2: full 4R on pre-pr is fine (hot paths are allowed).
		{
			name: "pre-pr with all 4R agents allowed",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPrePR,
						Run:  []string{"review-risk", "review-readability", "review-reliability", "review-resilience"},
						Mode: model.ModeStrong,
						When: model.TriggerWhen{PathGlobs: []string{"**/auth/**"}},
					},
				},
			},
			wantErr: false,
		},
		// W2: pre-commit with 3 of 4 agents and always=true is allowed (not all four 4R).
		{
			name: "pre-commit always with only 3 review agents allowed",
			set: model.TriggerRuleSet{
				Bindings: []model.TriggerBinding{
					{
						On:   model.EventPreCommit,
						Run:  []string{"review-risk", "review-readability", "review-reliability"},
						Mode: model.ModeAdvisory,
						When: model.TriggerWhen{Always: true},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTriggerRuleSet(tc.set)
			if tc.wantErr && err == nil {
				t.Error("ValidateTriggerRuleSet() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateTriggerRuleSet() = %v, want nil", err)
			}
		})
	}
}

// 2.7 — Structural: importing and calling catalog functions produces no I/O, goroutines, or exec.
// This is a compile-time + review assertion. The test verifies the functions are callable.
func TestDefaultTriggerRuleSet_NoExecNoHooks(t *testing.T) {
	// Simply calling these functions must not panic or produce side effects.
	_ = SupportedTriggerEvents()
	_ = KnownAgents()
	rs := DefaultTriggerRuleSet()
	_ = ValidateTriggerRuleSet(rs)
}
