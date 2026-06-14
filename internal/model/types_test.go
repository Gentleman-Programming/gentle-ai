package model

import "testing"

func TestAgentAntigravity(t *testing.T) {
	if AgentAntigravity != "antigravity" {
		t.Errorf("AgentAntigravity = %q, want %q", AgentAntigravity, "antigravity")
	}
}

// Unit 1 — TriggerEvent, TriggerMode, TriggerWhen, TriggerBinding, TriggerRuleSet

// 1.1 — all six TriggerEvent constants exist with correct string values.
func TestTriggerEvent_ClosedSet(t *testing.T) {
	events := []struct {
		name  string
		got   TriggerEvent
		want  string
	}{
		{"EventPreCommit", EventPreCommit, "pre-commit"},
		{"EventPrePush", EventPrePush, "pre-push"},
		{"EventPrePR", EventPrePR, "pre-pr"},
		{"EventPostSDDPhase", EventPostSDDPhase, "post-sdd-phase"},
		{"EventOnCI", EventOnCI, "on-ci"},
		{"EventOnSchedule", EventOnSchedule, "on-schedule"},
	}
	if len(events) != 6 {
		t.Fatalf("expected 6 TriggerEvent constants, got %d", len(events))
	}
	for _, tc := range events {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// 1.2 — all two TriggerMode constants exist with correct string values.
func TestTriggerMode_ClosedSet(t *testing.T) {
	modes := []struct {
		name string
		got  TriggerMode
		want string
	}{
		{"ModeAdvisory", ModeAdvisory, "advisory"},
		{"ModeStrong", ModeStrong, "strong"},
	}
	if len(modes) != 2 {
		t.Fatalf("expected 2 TriggerMode constants, got %d", len(modes))
	}
	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// 1.3 — TriggerBinding has required fields; zero-value Mode is empty; Reason is optional.
func TestTriggerBinding_Fields(t *testing.T) {
	var b TriggerBinding
	// On, When, Run, Mode are required fields that exist on the struct.
	_ = b.On
	_ = b.When
	_ = b.Run
	_ = b.Mode
	_ = b.Reason

	// Zero-value Mode must be empty (not pre-filled by the struct).
	if b.Mode != "" {
		t.Errorf("zero-value TriggerBinding.Mode = %q, want empty string", b.Mode)
	}
	// Reason is the only optional field — zero-value is empty string.
	if b.Reason != "" {
		t.Errorf("zero-value TriggerBinding.Reason = %q, want empty string", b.Reason)
	}
}

// 1.4 — TriggerWhen has correct fields; all are zero-value by default.
func TestTriggerWhen_Fields(t *testing.T) {
	var w TriggerWhen
	if w.Always {
		t.Error("zero-value TriggerWhen.Always = true, want false")
	}
	if w.PathGlobs != nil {
		t.Errorf("zero-value TriggerWhen.PathGlobs = %v, want nil", w.PathGlobs)
	}
	if w.MinDiffLines != 0 {
		t.Errorf("zero-value TriggerWhen.MinDiffLines = %d, want 0", w.MinDiffLines)
	}
	if w.Phases != nil {
		t.Errorf("zero-value TriggerWhen.Phases = %v, want nil", w.Phases)
	}
	if w.Combine != "" {
		t.Errorf("zero-value TriggerWhen.Combine = %q, want empty string", w.Combine)
	}
}

// 1.5 — TriggerRuleSet has Events and Bindings fields.
func TestTriggerRuleSet_Fields(t *testing.T) {
	var rs TriggerRuleSet
	if rs.Events != nil {
		t.Errorf("zero-value TriggerRuleSet.Events = %v, want nil", rs.Events)
	}
	if rs.Bindings != nil {
		t.Errorf("zero-value TriggerRuleSet.Bindings = %v, want nil", rs.Bindings)
	}
}
