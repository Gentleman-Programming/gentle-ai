package main

import (
	"fmt"
	"strings"
)

// stopHookJourneys drives #4064's `gentle-ai review stop-hook --agent
// claude-code`: the Claude Code hook that, on SessionStart, baselines the
// repository's current unreviewed-candidate identity for the session, and on
// Stop reminds the agent -- once per session and candidate, and only for a
// candidate the session itself produced -- to run the selectorless STATUS
// preflight instead of reporting completion silently. The stdin payloads'
// own cwd is left empty here because Step's Stdin field is a literal string,
// not a per-sandbox template like Args; the sandbox path is supplied through
// --cwd via productArgs instead, exactly as the hook's own --cwd override is
// documented to take precedence.
func stopHookJourneys() []Journey {
	sessionStart := `{"session_id":"bench-stop-hook","hook_event_name":"SessionStart","source":"startup"}`
	stdin := `{"session_id":"bench-stop-hook","cwd":"","hook_event_name":"Stop","stop_hook_active":false}`
	stdinActive := `{"session_id":"bench-stop-hook","cwd":"","hook_event_name":"Stop","stop_hook_active":true}`
	return []Journey{{
		ID:     "j125-claude-code-stop-hook-reminds-once-per-candidate",
		Review: reviewOptedIn,
		Title:  "Claude Code Stop hook reminds once per session and candidate",
		Source: "#4064: the hook hands the agent the STATUS preflight route instead of letting it report completion silently",
		Steps: []Step{
			{Name: "fixture: repo", Fixture: baseRepo},
			{
				Name:  "SessionStart baselines the clean repository silently",
				Args:  productArgs("review", "stop-hook", "--agent", "claude-code"),
				Stdin: sessionStart,
				After: func(_ *Sandbox, observation Observation) error {
					if observation.ExitCode != 0 {
						return fmt.Errorf("stop-hook exited %d, want 0: %s", observation.ExitCode, observation.Stderr)
					}
					if strings.TrimSpace(observation.Stdout) != "" {
						return fmt.Errorf("expected empty stdout on SessionStart, got: %s", observation.Stdout)
					}
					return nil
				},
			},
			{Name: "fixture: stage docs", Fixture: stageDocs("stop-hook")},
			{
				Name:  "an unreviewed candidate blocks with the STATUS preflight route",
				Args:  productArgs("review", "stop-hook", "--agent", "claude-code"),
				Stdin: stdin,
				After: func(_ *Sandbox, observation Observation) error {
					if observation.ExitCode != 0 {
						return fmt.Errorf("stop-hook exited %d, want 0: %s", observation.ExitCode, observation.Stderr)
					}
					if !strings.Contains(observation.Stdout, `"decision":"block"`) {
						return fmt.Errorf("stdout missing %q: %s", `"decision":"block"`, observation.Stdout)
					}
					if !strings.Contains(observation.Stdout, "gentle-ai review start") {
						return fmt.Errorf("stdout missing %q: %s", "gentle-ai review start", observation.Stdout)
					}
					return nil
				},
			},
			{
				Name:  "the same session and candidate is reminded only once",
				Args:  productArgs("review", "stop-hook", "--agent", "claude-code"),
				Stdin: stdin,
				After: func(_ *Sandbox, observation Observation) error {
					if observation.ExitCode != 0 {
						return fmt.Errorf("stop-hook exited %d, want 0: %s", observation.ExitCode, observation.Stderr)
					}
					if strings.TrimSpace(observation.Stdout) != "" {
						return fmt.Errorf("expected empty stdout on the repeat reminder, got: %s", observation.Stdout)
					}
					return nil
				},
			},
			{
				Name:  "an active Stop hook never reminds",
				Args:  productArgs("review", "stop-hook", "--agent", "claude-code"),
				Stdin: stdinActive,
				After: func(_ *Sandbox, observation Observation) error {
					if observation.ExitCode != 0 {
						return fmt.Errorf("stop-hook exited %d, want 0: %s", observation.ExitCode, observation.Stderr)
					}
					if strings.TrimSpace(observation.Stdout) != "" {
						return fmt.Errorf("expected empty stdout while stop_hook_active is true, got: %s", observation.Stdout)
					}
					return nil
				},
			},
		},
	}}
}
