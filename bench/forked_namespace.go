package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The fork renamed the product's identity from `gentle-ai` to `axiom`, and the
// rename is deliberately partial: the OpenCode orchestrator agent key and the
// install state moved, while telemetry, backups and the `gentle-ai:` comment
// markers kept their original spelling. A corpus that hard-codes one literal
// therefore measures the repository it was written in rather than the contract
// it means to prove.
//
// These helpers let a journey name the thing -- "the orchestrator agent", "the
// install state" -- and leave the spelling to whichever product is under test.
// The product carries the same ordered tolerance on its side
// (`internal/opencode.managedOpenCodeAgentKeys`), so accepting both keeps these
// journeys honest against the fork and against an upstream that renamed nothing.

// orchestratorAgentKeys lists the OpenCode agent keys that may hold the managed
// orchestrator definition, current spelling first. `sync` migrates the retired
// key to the current one and carries the user's own prompt bytes across intact,
// so a journey that seeds the retired key still proves byte preservation -- it
// just has to read the result back under either name.
var orchestratorAgentKeys = []string{"axiom-orchestrator", "gentle-orchestrator"}

// orchestratorEntry returns the managed orchestrator entry of a decoded OpenCode
// `agent` object, under whichever key holds it. An absent orchestrator returns
// the zero value and false, so callers keep their own shape checks.
func orchestratorEntry[T any](agents map[string]T) (T, bool) {
	for _, key := range orchestratorAgentKeys {
		if entry, ok := agents[key]; ok {
			return entry, true
		}
	}
	var missing T
	return missing, false
}

// managedStatePath returns the install state file the product actually writes
// under home: `.axiom` in the fork, `.gentle-ai` upstream. A fixture that stages
// install state MUST target the live file. Writing the retired path beside it is
// silently ignored, and the journey then measures a pristine install instead of
// the state it staged -- a false green, not a failure. Absence is an error for
// the same reason: it means nothing was staged over.
func managedStatePath(home string) (string, error) {
	candidates := []string{
		filepath.Join(home, ".axiom", "state.json"),
		filepath.Join(home, ".gentle-ai", "state.json"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no install state to stage over at %s", strings.Join(candidates, " or "))
}
