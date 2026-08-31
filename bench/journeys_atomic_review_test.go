package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusJourneysDeclareReviewMode(t *testing.T) {
	for _, journey := range allDeclaredJourneys() {
		if journey.Review != reviewOptedIn && journey.Review != reviewUntouched {
			t.Errorf("journey %q declares review mode %q; every runnable journey must explicitly opt in or remain untouched", journey.ID, journey.Review)
		}
	}
}

func TestCorpusRejectsRetiredOrdinaryPathReviewJourneys(t *testing.T) {
	for _, journey := range allDeclaredJourneys() {
		if replacement, stale := retiredAtomicJourneyReplacements[journey.ID]; stale {
			t.Errorf("retired lifecycle journey %q is still registered; #3417/#3587 replaces it with %q", journey.ID, replacement)
		}
	}
}

func TestCorpusDoesNotReintroduceRetiredReceiptAndGatePins(t *testing.T) {
	retiredPins := []string{
		"receipt survives",
		"receipt preserves",
		"receipt allows",
		"pre-push allows",
		"pre-pr binds",
		"gate allows",
		"gate denies",
		"gate blocks",
		"delivery is discovered selector-free",
		"staged delivery candidate can validate",
	}
	for _, journey := range allDeclaredJourneys() {
		if journey.ID == "j44-sdd-historical-requirement-stale-pass" || journey.ID == "j85-review-parse-refusals-are-preflight" {
			// TestHistoricalCompatibilityJourneysAreNarrowlyNamed constrains the
			// only retained historical parser/refusal compatibility fixtures.
			continue
		}
		declaration := strings.ToLower(journey.Title)
		for _, step := range journey.Steps {
			declaration += " " + strings.ToLower(step.Name)
		}
		for _, pin := range retiredPins {
			if strings.Contains(declaration, pin) {
				t.Errorf("journey %q reintroduced retired #3417 receipt/gate pin %q: %s", journey.ID, pin, declaration)
			}
		}
	}
}

func TestActiveJourneyAndAxisDeclarationsExcludeRetiredLifecyclePins(t *testing.T) {
	for _, journey := range allDeclaredJourneys() {
		for _, step := range journey.Steps {
			if step.Requires == nil {
				continue
			}
			verb := strings.Join(step.Requires.Verb, " ")
			if verb == "review finalize" || verb == "review capture-evidence" || verb == "review retry-final-verification" {
				t.Errorf("active journey %q still requires retired lifecycle verb %q", journey.ID, verb)
			}
		}
	}
}

func TestRetiredLifecycleImplementationsArePhysicallyRemoved(t *testing.T) {
	for path, forbidden := range map[string][]string{
		"journeys_provider_capture.go": {"finalizeProviderCaptureRetry"},
		"journeys_atomic_review.go": {
			"atomicBurnFinalizeCapability",
			"finalizeAtomicBurnReviewerSlots",
			"captureAtomicFinalEvidenceDescriptorFor",
			"captureAtomicBurnEvidence",
			"captureJ02FinalEvidence",
			"finalizeAtomicBurnEvidence",
		},
		"axis_damaged_store.go": {"review\", \"finalize"},
		"axis_real_world.go":    {"review\", \"finalize"},
	} {
		payload, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(payload), token) {
				t.Errorf("%s retains retired lifecycle residue %q", path, token)
			}
		}
	}
}

func TestHistoricalCompatibilityJourneysAreNarrowlyNamed(t *testing.T) {
	for _, journey := range allDeclaredJourneys() {
		if journey.ID != "j44-sdd-historical-requirement-stale-pass" && journey.ID != "j85-review-parse-refusals-are-preflight" {
			continue
		}
		declaration := strings.ToLower(journey.ID + " " + journey.Title + " " + journey.Source)
		if !strings.Contains(declaration, "historical") || !strings.Contains(declaration, "compatibility") {
			t.Errorf("historical compatibility journey %q must say historical and compatibility: %s", journey.ID, declaration)
		}
	}
}

func TestBenchRunnerDoesNotRestoreNewLineageEnvironmentActivation(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("runner.go"))
	if err != nil {
		t.Fatalf("read runner.go: %v", err)
	}
	if strings.Contains(string(content), "GENTLE_AI_RDD_NEW_LINEAGE") {
		t.Fatal("bench runner restored retired GENTLE_AI_RDD_NEW_LINEAGE ambient-authority activation")
	}
}

func TestFinalAtomicMigrationJourneysRatifyIssue3417(t *testing.T) {
	journeys := map[string]Journey{}
	for _, journey := range Journeys() {
		journeys[journey.ID] = journey
	}

	for id, required := range map[string][]string{
		"j106-captured-provider-validator-terminal-capture": {"#3587", "exact active lineage", "provider validator"},
		"j51-negotiated-status-correction-continuation":     {"#3587", "selectorless", "exact active lineage"},
		"j12-rejected-capture-then-recapture":               {"#3587", "exact active lineage", "recapture"},
		"j78-lens-finding-id-prefix-discovery":              {"#3587", "exact active lineage", "finding"},
	} {
		journey, ok := journeys[id]
		if !ok {
			t.Errorf("missing final #3417 migration journey %q", id)
			continue
		}
		declaration := strings.ToLower(journey.Source + " " + journey.Title)
		for _, step := range journey.Steps {
			declaration += " " + strings.ToLower(step.Name)
		}
		for _, phrase := range required {
			if !strings.Contains(declaration, strings.ToLower(phrase)) {
				t.Errorf("final #3417 migration journey %q declaration does not name %q: %s", id, phrase, declaration)
			}
		}
	}
}

func TestCurrentTerminalJourneysRequireAcknowledgementBeforeBurn(t *testing.T) {
	journeys := map[string]Journey{}
	for _, journey := range Journeys() {
		journeys[journey.ID] = journey
	}
	for _, id := range []string{
		"j105-compiled-provider-capture-retries-same-binding",
		"j110-untracked-terminal-burn-and-unmanaged-staged-validation",
		"j111-approved-transaction-burns-and-shipped-gates-are-unmanaged",
		"j114-last-reviewer-capture-closes-and-burns",
	} {
		journey, ok := journeys[id]
		if !ok {
			t.Errorf("missing acknowledgement journey %q", id)
			continue
		}
		declaration := strings.ToLower(journey.Source + " " + journey.Title)
		for _, step := range journey.Steps {
			declaration += " " + strings.ToLower(step.Name)
		}
		if !strings.Contains(declaration, "acknowledgement") {
			t.Errorf("terminal journey %q omits the pending acknowledgement continuation: %s", id, declaration)
		}
	}
}

func TestAtomicReviewJourneysRatifyAcknowledgementContract(t *testing.T) {
	journeys := map[string]Journey{}
	for _, journey := range Journeys() {
		journeys[journey.ID] = journey
	}

	for id, required := range map[string][]string{
		"j59-current-status-and-start-ignore-sibling-worktree-transaction": {
			"#3587", "selectorless STATUS", "START", "sibling worktree",
		},
		"j60-explicit-active-lineage-keeps-four-lens-correction-and-validator-flow": {
			"#3587", "four lenses", "correction", "validator",
		},
		"j111-approved-transaction-burns-and-shipped-gates-are-unmanaged": {
			"#3797", "selectorless STATUS", "printed START", "acknowledgement",
		},
		"j114-last-reviewer-capture-closes-and-burns": {
			"#3797", "last admitted reviewer capture", "acknowledgement",
		},
		"j89-staged-validation-is-informational-and-unmanaged": {
			"#3587", "staged", "informational", "unmanaged",
		},
		"j110-untracked-terminal-burn-and-unmanaged-staged-validation": {
			"#3797", "untracked", "acknowledgement", "unmanaged",
		},
		"j113-correction-removes-candidate-only-path": {
			"#3587", "correction", "terminal validator",
		},
	} {
		journey, ok := journeys[id]
		if !ok {
			t.Errorf("missing #3417 journey %q", id)
			continue
		}
		if journey.Review != reviewOptedIn {
			t.Errorf("acknowledgement journey %q review mode = %q, want %q", id, journey.Review, reviewOptedIn)
		}
		declaration := journey.Source + " " + journey.Title
		for _, step := range journey.Steps {
			declaration += " " + step.Name
		}
		for _, phrase := range required {
			if !strings.Contains(strings.ToLower(declaration), strings.ToLower(phrase)) {
				t.Errorf("acknowledgement journey %q declaration does not name %q: %s", id, phrase, declaration)
			}
		}
	}
}
