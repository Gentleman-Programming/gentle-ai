package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type issue1302Attempt struct {
	State          string `json:"state"`
	Reason         string `json:"reason"`
	Token          string `json:"token"`
	ItemSettlement *struct {
		ItemID string `json:"item_id"`
	} `json:"item_settlement"`
}

type issue1302Status struct {
	NextRecommended string `json:"nextRecommended"`
	Dependencies    struct {
		Apply   string `json:"apply"`
		Verify  string `json:"verify"`
		Archive string `json:"archive"`
	} `json:"dependencies"`
	Items []struct {
		ID     string `json:"id"`
		Active bool   `json:"active"`
		Ready  bool   `json:"ready"`
	} `json:"items"`
}

func (status issue1302Status) item(id string) (bool, bool) {
	for _, item := range status.Items {
		if item.ID == id {
			return item.Ready, item.Active
		}
	}
	return false, false
}

func issue1302Tasks(done map[string]bool) string {
	mark := func(id string) string {
		if done[id] {
			return "x"
		}
		return " "
	}
	return fmt.Sprintf("- [%s] p: Prior work\n- [%s] a: A\n- [%s] b: B\n- [%s] c: C\n- [%s] d: D\n<!-- gentle-ai.sdd-items/v1\n{\"items\":[{\"id\":\"p\",\"dependsOn\":[],\"workUnit\":\"p\",\"editRoots\":[\"p\"],\"maxAttempts\":1,\"maxChangedLines\":20,\"evidenceGoal\":\"prior\"},{\"id\":\"a\",\"dependsOn\":[],\"workUnit\":\"a\",\"editRoots\":[\"a\"],\"maxAttempts\":1,\"maxChangedLines\":20,\"evidenceGoal\":\"a\"},{\"id\":\"b\",\"dependsOn\":[],\"workUnit\":\"b\",\"editRoots\":[\"b\"],\"maxAttempts\":1,\"maxChangedLines\":20,\"evidenceGoal\":\"b\"},{\"id\":\"c\",\"dependsOn\":[\"a\",\"b\"],\"workUnit\":\"c\",\"editRoots\":[\"c\"],\"maxAttempts\":1,\"maxChangedLines\":20,\"evidenceGoal\":\"c\"},{\"id\":\"d\",\"dependsOn\":[],\"workUnit\":\"d\",\"editRoots\":[\"a/child\"],\"maxAttempts\":1,\"maxChangedLines\":20,\"evidenceGoal\":\"d\"}]}\n-->", mark("p"), mark("a"), mark("b"), mark("c"), mark("d"))
}

func issue1302Fixture(sandbox *Sandbox) error {
	if err := sddRuntimeRepo(sandbox); err != nil {
		return err
	}
	for _, root := range []string{"a", "a/child", "b", "c", "p"} {
		if err := sandbox.write(filepath.Join(sandbox.Repo, root, ".keep"), "\n"); err != nil {
			return err
		}
	}
	root := sddChangeRoot(sandbox)
	for path, content := range map[string]string{
		"design.md":          "# Design\n",
		"specs/item/spec.md": "### Requirement: Item\n#### Scenario: Join\n",
		"tasks.md":           issue1302Tasks(map[string]bool{"p": true}),
	} {
		if err := sandbox.write(filepath.Join(root, path), content); err != nil {
			return err
		}
	}
	return sandbox.git(sandbox.Repo, "add", "-A")
}

func issue1302AttemptRun(r *journeyRun, item, request string) (issue1302Attempt, error) {
	observation := r.run([]string{"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--item", item, "--request-id", request}, false)
	var result issue1302Attempt
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return result, fmt.Errorf("parse %s acquire: %w", item, err)
	}
	return result, nil
}

func issue1302Settle(r *journeyRun, token, request, item string) error {
	observation := r.run(append([]string{"sdd-attempt", "settle", "--cwd", r.sandbox.Repo, "--change", sddChange, "--token", token, "--request-id", request, "--outcome", "passed", "--evidence-revision", sddCorrectedEvidence}, sddTerminalEvidence...), false)
	var result issue1302Attempt
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &result); err != nil {
		return err
	}
	if result.State != "proceed" || result.ItemSettlement == nil || result.ItemSettlement.ItemID != item {
		return fmt.Errorf("%s settlement = %#v", item, result)
	}
	return nil
}

func issue1302Refused(r *journeyRun, item, request string, attempts int) error {
	observation := r.run([]string{"sdd-attempt", "acquire", "--cwd", r.sandbox.Repo, "--change", sddChange, "--item", item, "--request-id", request}, false)
	if observation.ExitCode == 0 || !strings.Contains(observation.Stderr, "not ready") {
		return fmt.Errorf("%s refusal = exit=%d stderr=%q", item, observation.ExitCode, observation.Stderr)
	}
	runtime, err := proveRuntime(r.sandbox)
	if err != nil || len(runtime.Attempts) != attempts {
		return fmt.Errorf("%s refusal mutated runtime = %#v: %v", item, runtime, err)
	}
	return nil
}

func issue1302StatusRead(r *journeyRun) (issue1302Status, error) {
	observation := r.run([]string{"sdd-status", sddChange, "--cwd", r.sandbox.Repo, "--json"}, false)
	var status issue1302Status
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), &status); err != nil {
		return status, err
	}
	return status, nil
}

func issue1302DriveJoin(r *journeyRun) error {
	a, err := issue1302AttemptRun(r, "a", "issue1302-a")
	if err != nil || a.State != "proceed" || a.Token == "" {
		return fmt.Errorf("A acquire = %#v: %v", a, err)
	}
	b, err := issue1302AttemptRun(r, "b", "issue1302-b")
	if err != nil || b.State != "proceed" || b.Token == "" || a.Token == b.Token {
		return fmt.Errorf("B acquire = %#v: %v", b, err)
	}
	if err := issue1302Refused(r, "d", "issue1302-overlap", 2); err != nil {
		return err
	}
	if err := issue1302Settle(r, a.Token, "issue1302-a-settle", "a"); err != nil {
		return err
	}
	if err := issue1302Refused(r, "c", "issue1302-c-early", 2); err != nil {
		return err
	}
	if err := issue1302Settle(r, b.Token, "issue1302-b-settle", "b"); err != nil {
		return err
	}
	status, err := issue1302StatusRead(r)
	cReady, _ := status.item("c")
	if err != nil || !cReady || status.NextRecommended != "apply" || status.Dependencies.Verify != "blocked" {
		return fmt.Errorf("post A/B status = %#v: %v", status, err)
	}
	if err := r.sandbox.write(filepath.Join(sddChangeRoot(r.sandbox), "tasks.md"), issue1302Tasks(map[string]bool{"p": true, "a": true, "b": true})); err != nil {
		return err
	}
	c, err := issue1302AttemptRun(r, "c", "issue1302-c")
	if err != nil || c.State != "proceed" || c.Token == "" {
		return fmt.Errorf("C acquire = %#v: %v", c, err)
	}
	d, err := issue1302AttemptRun(r, "d", "issue1302-d")
	if err != nil || d.State != "proceed" || d.Token == "" {
		return fmt.Errorf("D acquire = %#v: %v", d, err)
	}
	if err := r.sandbox.write(filepath.Join(sddChangeRoot(r.sandbox), "tasks.md"), issue1302Tasks(map[string]bool{"p": true, "a": true, "b": true, "c": true, "d": true})); err != nil {
		return err
	}
	status, err = issue1302StatusRead(r)
	if err != nil || status.NextRecommended != "apply" || status.Dependencies.Verify != "blocked" {
		return fmt.Errorf("prechecked active C status = %#v: %v", status, err)
	}
	if err := issue1302Settle(r, c.Token, "issue1302-c-settle", "c"); err != nil {
		return err
	}
	if err := issue1302Settle(r, d.Token, "issue1302-d-settle", "d"); err != nil {
		return err
	}
	status, err = issue1302StatusRead(r)
	if err != nil || status.NextRecommended != "verify" || status.Dependencies.Verify != "ready" || status.Dependencies.Archive != "blocked" {
		return fmt.Errorf("joined status = %#v: %v", status, err)
	}
	return nil
}

func issue1302Journeys() []Journey {
	return []Journey{{
		ID:     "j106-sdd-concurrent-item-join-barrier",
		Title:  "Concurrent SDD items join only after immutable settlement and coordinator projection",
		Source: "https://github.com/Gentleman-Programming/gentle-ai/issues/1302",
		Steps: []Step{
			{Name: "fixture: prechecked predecessor and concurrent item plan", Fixture: issue1302Fixture},
			{Name: "disable RDD", Requires: modeCapability, Args: productArgs("review", "mode", "disable", "--json")},
			{Name: "acquire, settle, project, and join concurrent items", Requires: sddAttemptAcquireCapability, Composite: issue1302DriveJoin},
		},
	}}
}
