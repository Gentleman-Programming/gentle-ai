package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// #6086: zero authored lines must not turn valid authority into corruption.
func correctionUnavailableJourneys() []Journey {
	return []Journey{{
		ID:     "j129-correction-request-unavailable-zero-budget",
		Title:  "Zero-budget correction stops truthfully and offers only manual eligible release",
		Source: "#6086 / #6087",
		Review: reviewOptedIn,
		Steps: []Step{
			{Name: "base repository", Fixture: baseRepo},
			{Name: "mode-only and golden candidate", Fixture: zeroBudgetCorrectionFixture},
			{Name: "capture finding and check read-only stop", Composite: proveUnavailableCorrection},
		},
	}}
}

func zeroBudgetCorrectionFixture(s *Sandbox) error {
	for path, body := range map[string]string{
		"candidate.go":                  "package candidate\n\nfunc value() int { return 1 }\n",
		"testdata/golden/output.golden": "before\n",
	} {
		if err := s.write(filepath.Join(s.Repo, path), body); err != nil {
			return err
		}
	}
	if err := s.git(s.Repo, "add", "candidate.go", "testdata/golden/output.golden"); err != nil {
		return err
	}
	if err := s.git(s.Repo, "commit", "-qm", "zero-budget base"); err != nil {
		return err
	}
	if err := s.git(s.Repo, "config", "core.filemode", "true"); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(s.Repo, "candidate.go"), 0o755); err != nil {
		return err
	}
	if err := s.write(filepath.Join(s.Repo, "testdata/golden/output.golden"), "after\n"); err != nil {
		return err
	}
	return s.git(s.Repo, "add", "candidate.go", "testdata/golden/output.golden")
}

func proveUnavailableCorrection(r *journeyRun) error {
	lineage, err := startAtomicTransactionFromSelectorlessStatus(r)
	if err != nil {
		return err
	}
	r.sandbox.Lineage = lineage
	var terminal Observation
	for capture := 0; capture < 8; capture++ {
		status, err := readAtomicReviewStatus(r, lineage)
		if err != nil {
			return err
		}
		if status.NextTransition.ReasonCode == "provider_refuter_required" {
			terminal, err = corroborateProviderRefuter(r, status)
			if err != nil {
				return err
			}
			break
		}
		if status.NextTransition.ReasonCode != "reviewer_results_required" || len(status.NextTransition.Collect.Inputs) == 0 {
			return fmt.Errorf("expected reviewer slot, got %+v", status.NextTransition)
		}
		input := status.NextTransition.Collect.Inputs[0]
		payload, err := synthesizeReviewerResult(input.ArtifactSubject.SubjectHash, status.paths())
		if err != nil {
			return err
		}
		if capture == 0 {
			payload, err = json.Marshal(map[string]any{
				"subject_hash": input.ArtifactSubject.SubjectHash,
				"inspection":   map[string]any{"status": "completed", "paths": status.paths()},
				"findings": []any{map[string]any{
					"location": "testdata/golden/output.golden:1", "severity": "CRITICAL",
					"claim":          "candidate records the wrong output",
					"proof_refs":     []string{"testdata/golden/output.golden:1 changed hunk replaces before with after"},
					"evidence_class": "deterministic", "causal_disposition": "introduced",
				}},
				"evidence": []string{"inspected the exact frozen golden candidate"},
			})
			if err != nil {
				return err
			}
		}
		path, err := writeScratch(r.sandbox, fmt.Sprintf("j129-reviewer-%d.json", capture), payload)
		if err != nil {
			return err
		}
		terminal = r.run([]string{"review", "capture-result", "--cwd", r.sandbox.Repo,
			"--lineage", status.argument("lineage"), "--target", status.argument("target"),
			"--expected-revision", status.argument("expected-revision"), "--lens", status.argument("lens"),
			"--order", status.argument("order"), "--input", path}, true)
		if terminal.ExitCode != 0 {
			return fmt.Errorf("capture exited %d: %s", terminal.ExitCode, terminal.Stderr)
		}
		var closure lastEventClosure
		if err := json.Unmarshal([]byte(terminal.Stdout), &closure); err != nil {
			return err
		}
		if closure.State == "correction_required" {
			break
		}
	}
	var closure lastEventClosure
	if err := json.Unmarshal([]byte(terminal.Stdout), &closure); err != nil {
		return err
	}
	if terminal.ExitCode != 0 || closure.LineageID != lineage || closure.State != "correction_required" || closure.StatusContinuation == nil || closure.StatusContinuation.Operation != "review.status" {
		return fmt.Errorf("expected correction closure, got %+v (exit %d)", closure, terminal.ExitCode)
	}
	inventory, err := proveStoreStatus(r.sandbox)
	if err != nil {
		return err
	}
	revision := ""
	for _, entry := range inventory.Entries {
		if entry.LineageID == lineage {
			revision = entry.Revision
		}
	}
	if revision == "" {
		return fmt.Errorf("inventory omitted authority revision")
	}
	args := []string{"review", "status"}
	for _, arg := range closure.StatusContinuation.Arguments {
		if arg.Token == "" {
			return fmt.Errorf("status continuation contains empty token")
		}
		args = append(args, arg.Token)
	}
	for repeat := 0; repeat < 2; repeat++ {
		observed := r.run(args, false)
		if observed.ExitCode != 0 || observed.Stderr != "" {
			return fmt.Errorf("STATUS exit %d stderr %q", observed.ExitCode, observed.Stderr)
		}
		var status struct {
			statusEnvelope
			Forecast struct {
				Horizon string `json:"horizon"`
				Steps   []struct {
					Kind       string `json:"kind"`
					ReasonCode string `json:"reason_code"`
				} `json:"steps"`
			} `json:"forecast"`
		}
		if err := json.Unmarshal([]byte(observed.Stdout), &status); err != nil {
			return err
		}
		if status.Authority.LineageID != lineage || status.Authority.State != "correction_required" || status.Authority.Revision != revision || status.NextTransition.Kind != "stop" || status.NextTransition.ReasonCode != "correction_request_unavailable" {
			return fmt.Errorf("unexpected stop or mutated authority: %+v", status)
		}
		if status.Forecast.Horizon != "terminal" || len(status.Forecast.Steps) != 1 || status.Forecast.Steps[0].ReasonCode != "correction_request_unavailable" {
			return fmt.Errorf("unexpected forecast: %+v", status.Forecast)
		}
		if c := status.NextTransition.Continuation; c == nil || c.Operation != "abandon" || c.Command == "" {
			return fmt.Errorf("eligible stop omitted manual release: %+v", c)
		}
	}
	return nil
}
