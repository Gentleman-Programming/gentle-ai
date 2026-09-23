package skillregistry

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestDestinationAndMirrorStatusLiterals pins the exact CLI literals
// (REQ-22.10, REQ-22.11): the engine and the presentation must never invent a
// spelling of their own.
func TestDestinationAndMirrorStatusLiterals(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "DestUpdated", got: string(DestUpdated), want: "updated"},
		{name: "DestUnchanged", got: string(DestUnchanged), want: "unchanged"},
		{name: "DestOmitted", got: string(DestOmitted), want: "omitted"},
		{name: "MirrorOK", got: string(MirrorOK), want: "mirror ok"},
		{name: "MirrorFailed", got: string(MirrorFailed), want: "mirror failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s literal = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

// TestResultCarriesOneOutcomePerDestination pins the Result constructor shape
// of D-09 so `refresh` can declare its per-destination lines without re-reading
// any file.
func TestResultCarriesOneOutcomePerDestination(t *testing.T) {
	result := Result{
		Regenerated: true,
		SkillCount:  3,
		Reason:      "forced",
		Registry:    "/ws/.atl/skill-registry.md",
		Cache:       "/ws/.atl/.skill-registry.cache.json",
		Agents:      DestinationOutcome{Status: DestUpdated, Path: "/ws/AGENTS.md"},
		Mirror:      MirrorOutcome{Status: MirrorFailed, Err: errors.New("boom")},
	}
	if !result.Regenerated || result.SkillCount != 3 || result.Reason != "forced" {
		t.Fatalf("Result primary fields not preserved: %+v", result)
	}
	if result.Registry == "" || result.Cache == "" {
		t.Fatalf("Result must carry both primary destinations: %+v", result)
	}
	if result.Agents.Status != DestUpdated || result.Agents.Path == "" {
		t.Fatalf("Result.Agents = %+v, want updated with a path", result.Agents)
	}
	if result.Mirror.Status != MirrorFailed || result.Mirror.Err == nil {
		t.Fatalf("Result.Mirror = %+v, want mirror failed with an error", result.Mirror)
	}
}

// TestNilMirrorFuncReportsMirrorFailed covers T-8: a nil MirrorFunc is
// `mirror failed` ("mirror not configured") and never `mirror ok` — the
// command must not claim a write it never attempted. The failure is not fatal.
func TestNilMirrorFuncReportsMirrorFailed(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))

	result, err := Regenerate(cwd, home, RegenerateOptions{})
	if err != nil {
		t.Fatalf("nil MirrorFunc must not be fatal: %v", err)
	}
	if result.Mirror.Status != MirrorFailed {
		t.Fatalf("Mirror.Status = %q, want %q", result.Mirror.Status, MirrorFailed)
	}
	if result.Mirror.Err == nil || !strings.Contains(result.Mirror.Err.Error(), "mirror not configured") {
		t.Fatalf("Mirror.Err = %v, want reason %q", result.Mirror.Err, "mirror not configured")
	}
}

// TestMirrorOKOnlyAfterNilFuncReturn covers the other half of T-8.
func TestMirrorOKOnlyAfterNilFuncReturn(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	writeSkill(t, filepath.Join(cwd, "skills", "go-testing", "SKILL.md"), minimalSkill("go-testing"))

	ok, err := Regenerate(cwd, home, RegenerateOptions{Force: true, Mirror: func(MirrorRequest) error { return nil }})
	if err != nil {
		t.Fatalf("Regenerate() error = %v", err)
	}
	if ok.Mirror.Status != MirrorOK {
		t.Fatalf("Mirror.Status = %q, want %q", ok.Mirror.Status, MirrorOK)
	}

	failed, err := Regenerate(cwd, home, RegenerateOptions{Force: true, Mirror: func(MirrorRequest) error {
		return errors.New("engram unreachable")
	}})
	if err != nil {
		t.Fatalf("mirror failure must not be fatal: %v", err)
	}
	if failed.Mirror.Status != MirrorFailed {
		t.Fatalf("Mirror.Status = %q, want %q", failed.Mirror.Status, MirrorFailed)
	}
}
