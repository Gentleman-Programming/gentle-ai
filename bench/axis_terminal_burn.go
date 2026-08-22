package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const terminalBurnAxis = "terminal-burn"
const terminalBurnLineage = "issue-3572-terminal-burn"
const terminalBurnContract = "gentle-ai.review-integration/v2"

// This axis is deliberately non-portable: bench_fixture exposes only the
// admission barrier. Both contenders still run the shipped FINALIZE command;
// the harness controls ordering and checks the post-burn filesystem invariant.
func init() {
	RegisterAxis(Axis{
		Name: terminalBurnAxis, Title: "Deterministic concurrent FINALIZE admission after terminal burn", BlackBox: false,
		Review: reviewOptedIn, Properties: []string{
			"Requires a product binary built with -tags bench_fixture for the real contender admission barrier.",
			"Both contenders are real gentle-ai review finalize subprocesses; no authority bytes are authored by the harness.",
			"Opt-in and non-portable by declaration; the ordinary black-box corpus remains unchanged.",
		}, Journeys: terminalBurnJourneys,
	})
}

func terminalBurnJourneys() []Journey {
	return []Journey{{
		ID:     "tb01-concurrent-finalize-cannot-recreate-burned-lineage",
		Title:  "#3572: a late concurrent FINALIZE cannot recreate a burned lineage journal",
		Source: "#3572: the winner burns authority while a real contender is held before journal admission; release proves no lineage or residue is recreated",
		Steps: []Step{
			{Name: "fixture: repository", Fixture: baseRepo},
			{Name: "fixture: staged low-risk candidate", Fixture: stageDocs("issue-3572")},
			{Name: "start the exact active lineage", Requires: startNamedCapability, Args: productArgs("review", "start", "--lineage", terminalBurnLineage), After: func(sandbox *Sandbox, observation Observation) error {
				if observation.ExitCode != 0 {
					return fmt.Errorf("review start exited %d: %s", observation.ExitCode, firstLine(observation.Stderr))
				}
				sandbox.Lineage = terminalBurnLineage
				return nil
			}},
			{Name: "concurrent FINALIZE burns before the held contender is admitted", Requires: finalizeCapability, Composite: runTerminalBurnConcurrency},
		},
	}}
}

type terminalBurnProcess struct {
	args           []string
	cmd            *exec.Cmd
	stdout, stderr *bytes.Buffer
	done           chan error
	cancel         context.CancelFunc
	waitErr        error
	waited         bool
}

func startTerminalBurnFinalize(sandbox *Sandbox, lineage, barrier string, negotiated bool) (*terminalBurnProcess, error) {
	args := []string{"review", "finalize"}
	if negotiated {
		args = append(args, "--contract", terminalBurnContract)
	}
	args = append(args, "--cwd", sandbox.Repo, "--lineage", lineage)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, sandbox.Binary, args...)
	cmd.Dir, cmd.Env = sandbox.Repo, sandbox.env()
	if barrier != "" {
		cmd.Env = append(cmd.Env, "GENTLE_AI_BENCH_FINALIZE_ADMISSION_BARRIER="+barrier)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	process := &terminalBurnProcess{args: args, cmd: cmd, stdout: stdout, stderr: stderr, done: make(chan error, 1), cancel: cancel}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() { process.done <- cmd.Wait() }()
	return process, nil
}

func (process *terminalBurnProcess) wait() Observation {
	if !process.waited {
		process.waitErr, process.waited = <-process.done, true
		process.cancel()
	}
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(process.waitErr, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if process.waitErr != nil {
		exitCode = -1
	}
	return Observation{Args: process.args, ExitCode: exitCode, Stdout: process.stdout.String(), Stderr: process.stderr.String(), StdoutCaptured: true, StderrCaptured: true}
}

func waitTerminalBurnReady(process *terminalBurnProcess, path string) error {
	ticker, timeout := time.NewTicker(10*time.Millisecond), time.NewTimer(10*time.Second)
	defer ticker.Stop()
	defer timeout.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case err := <-process.done:
			process.done <- err
			if err != nil {
				return fmt.Errorf("held FINALIZE contender exited before admission barrier: %w", err)
			}
			return errors.New("held FINALIZE contender exited before admission barrier")
		case <-timeout.C:
			return errors.New("timed out waiting for the FINALIZE contender admission barrier")
		case <-ticker.C:
		}
	}
}

type terminalBurnFailure struct {
	Schema                 string   `json:"schema"`
	Contract               string   `json:"contract"`
	Operation              string   `json:"operation"`
	Phase                  string   `json:"phase"`
	Code                   string   `json:"code"`
	Message                string   `json:"message"`
	Cause                  string   `json:"cause"`
	MutationOutcome        string   `json:"mutation_outcome"`
	AuthorityApplicability string   `json:"authority_applicability"`
	RetrySafe              bool     `json:"retry_safe"`
	Replayability          string   `json:"replayability"`
	LineageID              string   `json:"lineage_id"`
	RequiredInputs         []string `json:"required_inputs"`
	NextAction             string   `json:"next_action"`
}

func requireTerminalBurnLoser(observation Observation) error {
	if observation.ExitCode == 0 {
		return errors.New("late concurrent FINALIZE contender unexpectedly succeeded")
	}
	payload := strings.TrimSpace(observation.Stdout)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return fmt.Errorf("parse negotiated FINALIZE loser: %w", err)
	}
	for _, name := range []string{"schema", "contract", "operation", "phase", "code", "message", "cause", "mutation_outcome", "authority_applicability", "retry_safe", "replayability", "lineage_id", "required_inputs", "next_action"} {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("negotiated FINALIZE loser omitted contract field %q", name)
		}
	}
	var failure terminalBurnFailure
	if err := json.Unmarshal([]byte(payload), &failure); err != nil {
		return fmt.Errorf("decode negotiated FINALIZE loser: %w", err)
	}
	if failure.Schema != "gentle-ai.review-integration.failure/v2" || failure.Contract != terminalBurnContract || failure.Operation != "review.finalize" ||
		failure.Phase == "" || failure.Code != "operation_outcome_unknown" || failure.Message == "" || failure.Cause == "" || failure.MutationOutcome != "unknown" ||
		failure.AuthorityApplicability != "not_evaluated" || failure.RetrySafe || failure.Replayability != "status_required" ||
		failure.LineageID != terminalBurnLineage || failure.RequiredInputs == nil || failure.NextAction != "review.status" {
		return fmt.Errorf("negotiated FINALIZE loser classification = %+v", failure)
	}
	return nil
}

func recordTerminalBurnObservation(run *journeyRun, observation Observation) error {
	gitCalls := run.sandbox.gitCallsSince()
	if gitCalls == nil {
		return errors.New("git subprocess count for direct FINALIZE observation was unavailable")
	}
	record := run.accumulator.observe(run.step, observation, gitCalls, false)
	run.accumulator.records = append(run.accumulator.records, record)
	return nil
}

func runTerminalBurnConcurrency(run *journeyRun) (err error) {
	ready := filepath.Join(run.sandbox.Root, "terminal-burn-contender.ready")
	release := filepath.Join(run.sandbox.Root, "terminal-burn-contender.release")
	barrier := strings.Join([]string{terminalBurnLineage, ready, release}, "|")
	var contender, winner *terminalBurnProcess
	released := false
	defer func() {
		if !released {
			_ = os.WriteFile(release, []byte("release\n"), 0o600)
		}
		if winner != nil {
			winner.cancel()
			_ = winner.wait()
		}
		if contender != nil {
			contender.cancel()
			_ = contender.wait()
		}
	}()
	contender, err = startTerminalBurnFinalize(run.sandbox, terminalBurnLineage, barrier, true)
	if err != nil {
		return fmt.Errorf("start held FINALIZE contender: %w", err)
	}
	if err = waitTerminalBurnReady(contender, ready); err != nil {
		return err
	}
	winner, err = startTerminalBurnFinalize(run.sandbox, terminalBurnLineage, "", false)
	if err != nil {
		return fmt.Errorf("start FINALIZE winner: %w", err)
	}
	winnerObservation := winner.wait()
	if err = recordTerminalBurnObservation(run, winnerObservation); err != nil {
		return err
	}
	if err = requireBurnedApproval(terminalBurnLineage)(run.sandbox, winnerObservation); err != nil {
		return fmt.Errorf("winner FINALIZE: %w", err)
	}
	if err = os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		return fmt.Errorf("release held FINALIZE contender: %w", err)
	}
	released = true
	contenderObservation := contender.wait()
	if err = recordTerminalBurnObservation(run, contenderObservation); err != nil {
		return err
	}
	if err = requireTerminalBurnLoser(contenderObservation); err != nil {
		return err
	}
	return assertTerminalBurnResiduesAbsent(run.sandbox, terminalBurnLineage)
}

func assertTerminalBurnResiduesAbsent(sandbox *Sandbox, lineage string) error {
	common, err := terminalBurnGitCommonDir(sandbox)
	if err != nil {
		return err
	}
	base := filepath.Join(common, "gentle-ai", "review-transactions")
	for _, path := range []string{filepath.Join(base, "v2", lineage), filepath.Join(base, "effect-markers", "v1", lineage), filepath.Join(base, "incidents", lineage)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("terminal burn residue at %s: %v", path, err)
		}
	}
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := ".review-burn-v2-" + lineage + "-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			return fmt.Errorf("terminal burn staging residue at %s", entry.Name())
		}
	}
	return nil
}

func terminalBurnGitCommonDir(sandbox *Sandbox) (string, error) {
	cmd := exec.Command("git", "-C", sandbox.Repo, "rev-parse", "--git-common-dir")
	env := sandbox.env()
	for index := range env {
		if strings.HasPrefix(env[index], "GIT_TRACE=") {
			env[index] = "GIT_TRACE="
		}
	}
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve Git common directory: %w", err)
	}
	common := strings.TrimSpace(string(output))
	if !filepath.IsAbs(common) {
		common = filepath.Join(sandbox.Repo, common)
	}
	return filepath.Clean(common), nil
}
