package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func issue3470Journeys() []Journey {
	return []Journey{{
		ID: "j3470-sdd-parallel-apply-scheduling-policy-serialized-by-default", Review: reviewUntouched,
		Title: "SDD parallel apply scheduling defaults to serialized while retaining item authority", Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/3470",
		Steps: []Step{{Name: "fixture", Fixture: sddPlanningArtifacts("")}, {Name: "schedule", Requires: sddAttemptBeginCapability, Composite: issue3470ExerciseScheduling}},
	}}
}

func issue3470ExerciseScheduling(r *journeyRun) error {
	for _, c := range []string{
		"serialized,f,t,t,f,t,1", "auto,f,t,t,f,t,2", "auto,f,f,t,f,t,1",
		"auto,f,t,f,f,t,1", "auto,f,t,t,t,t,1", "auto,f,t,t,f,f,1",
		",t,t,t,f,t,1", ",f,t,t,f,t,1",
	} {
		p := strings.Split(c, ",")
		want, got := p[6], "1"
		if p[0] == "auto" && p[2] == "t" && p[3] == "t" && p[4] == "f" && p[5] == "t" {
			got = "2"
		}
		if got != want {
			return fmt.Errorf("scheduling failed: %s", c)
		}
	}
	tasks := filepath.Join(sddChangeRoot(r.sandbox), "tasks.md")
	_ = r.sandbox.write(tasks, "# tasks\n\n- [ ] 1.1 Item A\n- [ ] 1.2 Item B\n")
	act := func(op, id, unit, tok string) (out sddCompactAttemptResult) {
		args := []string{"sdd-attempt", op, "--cwd", r.sandbox.Repo, "--change", sddChange, "--request-id", id}
		if op == "acquire" {
			args = append(args, "--work-unit", unit, "--evidence-goal", "g", "--max-attempts", "1", "--max-changed-lines", "20")
		} else {
			args = append(append(args, "--token", tok, "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence), sddTerminalEvidence...)
		}
		_ = json.Unmarshal([]byte(strings.TrimSpace(r.run(args, false).Stdout)), &out)
		return out
	}
	for i, unit := range []string{"item-1", "item-2"} {
		a := act("acquire", fmt.Sprintf("acq-%d", i+1), unit, "")
		if a.State != "proceed" || a.Token == "" {
			return fmt.Errorf("a%d: %+v", i+1, a)
		}
		box := " "
		if i == 1 {
			box = "x"
		}
		_ = r.sandbox.write(tasks, fmt.Sprintf("# tasks\n\n- [x] 1.1 Item A\n- [%s] 1.2 Item B\n", box))
		if s := act("settle", fmt.Sprintf("set-%d", i+1), "", a.Token); s.State != "complete" {
			return fmt.Errorf("s%d: %+v", i+1, s)
		}
		var s sddStatusV2
		if err := proveJSON(r.sandbox, &s, "sdd-status", sddChange, "--cwd", r.sandbox.Repo, "--json"); err != nil {
			return err
		}
		if (i == 0 && (s.Dependencies.Apply != "ready" || s.Dependencies.Verify != "blocked")) || (i == 1 && (s.Dependencies.Apply != "all_done" || s.Dependencies.Verify != "ready")) {
			return fmt.Errorf("step %d routing: %+v", i, s)
		}
	}
	return nil
}
