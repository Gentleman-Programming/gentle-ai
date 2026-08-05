package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// fixture is one recorded gentle-ai invocation from testdata/observations.json.
// The file was produced by driving a real gentle-ai binary; nothing in it is
// hand-written prose, so the classifier is tested against bytes the product
// actually emits rather than against what a test author imagines it emits.
type fixture struct {
	Name string `json:"name"`
	Observation
}

func loadFixtures(t *testing.T) map[string]Observation {
	t.Helper()
	content, err := os.ReadFile("testdata/observations.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var records []fixture
	if err := json.Unmarshal(content, &records); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	index := map[string]Observation{}
	for _, record := range records {
		index[record.Name] = record.Observation
	}
	if len(index) == 0 {
		t.Fatal("fixture file is empty")
	}
	return index
}

// TestClassifyRecordedObservations pins the classifier to output the product
// really emitted, so the in_band / out_of_band split is decided by bytes rather
// than by what a test author imagines a refusal looks like.
//
// It guards the classifier, NOT the product. The fixtures are frozen, so this
// test cannot notice a gentle-ai that stops naming a runnable continuation; it
// would keep passing against the recording. Several fixtures below already
// describe blocks the product no longer has, and they are kept on purpose: the
// classifier still has to recognise that shape if it ever returns. What guards
// the product is the bench run itself, plus the CLI tests that take a command
// out of an emitted message, dispatch it, and require the block to clear.
//
// So when a refusal is fixed, do not update these fixtures to match. Add the
// fixed shape as a new case if it is a shape the classifier has not seen.
func TestClassifyRecordedObservations(t *testing.T) {
	fixtures := loadFixtures(t)

	cases := []struct {
		name string
		want string
		why  string
	}{
		{"low_risk_start", NotABlock, "exit 0 and no denial"},
		{"low_risk_finalize", NotABlock, "exit 0 and no denial"},
		{"gate_allow", NotABlock, "allowed: true"},
		{"status_collect", NotABlock, "read-only status, exit 0"},
		{"mode_disable", NotABlock, "the kill switch turning off is not a block"},
		{"high_risk_start", NotABlock, "the consent-skipped notice is a prompt, not a block"},
		// These two are one letter apart in the envelope and opposite in
		// meaning, which is exactly why both are pinned. The first hands
		// delivery to ordinary repository policy and stops nothing; counting
		// it would have reported the kill switch working as friction it
		// caused. The second is the switch ON with no receipt, where the
		// operator really is stopped.
		{"gate_disabled_unmanaged", NotABlock,
			"delivery is disabled/unmanaged and action is repository-policy, so the commit proceeds"},
		{"gate_unmanaged_switch_on", BlockInBand,
			"exit 1 with no delivery disposition, and stderr names `gentle-ai review start`"},

		{"finalize_without_evidence", BlockInBand,
			"stderr names `gentle-ai review capture-evidence` and the follow-up finalize"},
		{"invalid_flag_combination", BlockInBand,
			"stderr names both runnable escapes verbatim"},

		// The three marked [fixed] recorded a block the product has since
		// stopped producing. They stay because the classifier must still
		// recognise the shape, and because a recording is the only honest
		// evidence of what the shape looked like.
		{"gate_without_receipt", BlockOutOfBand,
			"[fixed] exit 1, action is explicit-maintainer-action, nothing runnable is named"},
		{"finalize_without_results", BlockOutOfBand,
			"names the requirement but no command that satisfies it"},
		{"rejected_capture", BlockOutOfBand,
			"[fixed] binding_mismatch names no recapture command"},
		{"start_while_disabled", BlockOutOfBand,
			"[fixed] does not name `gentle-ai review mode enable`"},
		{"abandon_without_token", BlockOutOfBand,
			"lists required flags but no runnable command that produces the token"},

		// The bytes alone are out_of_band: the refusal names an action but no
		// runnable command. Only a by-design declaration whose quoted text is
		// verified present in these bytes moves it, and the corpus -- not this
		// fixture -- carries that declaration.
		{"bare_repository_start", BlockOutOfBand,
			"names what to do but no runnable command; the exemption lives in the corpus, not in the bytes"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			observation, ok := fixtures[testCase.name]
			if !ok {
				t.Fatalf("fixture %q missing", testCase.name)
			}
			if got := Classify(observation); got != testCase.want {
				t.Fatalf("Classify(%s) = %q, want %q (%s)\nstderr: %s\nstdout: %s",
					testCase.name, got, testCase.want, testCase.why, observation.Stderr, observation.Stdout)
			}
		})
	}
}

// TestUnsupportedIsNotAPassAndNotABlock is the cross-version guard. An older
// binary that lacks a verb or a flag must record `unsupported`, which is
// neither a pass nor a block, so a comparison can never read "this version
// could not do it" as "this version needed zero".
func TestUnsupportedIsNotAPassAndNotABlock(t *testing.T) {
	fixtures := loadFixtures(t)
	for _, name := range []string{"undefined_flag", "unknown_verb"} {
		observation := fixtures[name]
		if !IsUnsupported(observation) {
			t.Fatalf("%s: IsUnsupported = false, want true\nstderr: %s", name, observation.Stderr)
		}
		if IsBlock(observation) {
			t.Fatalf("%s: IsBlock = true, want false (unsupported must not inflate blocks)", name)
		}
		if got := Classify(observation); got != NotABlock {
			t.Fatalf("%s: Classify = %q, want %q", name, got, NotABlock)
		}
	}
}

func TestHasRunnableCommand(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"backticked command", "capture it first with `gentle-ai review capture-evidence`, then", true},
		{"double quoted command", `rerun with "gentle-ai review start --projection staged"`, true},
		{"bare command", "run gentle-ai review mode disable to turn reviews off", true},
		{"placeholder is not runnable", "validate delivery with gentle-ai review validate --gate <gate>", false},
		{"bare product name", "gentle-ai could not read an answer", true},
		{"product name with no argument", "reported by gentle-ai", false},
		{"no mention", "review finalize requires all 4 original reviewer result(s)", false},
		{"placeholder plus a clean command", "run gentle-ai review status --json or gentle-ai review repair --lineage <id>", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := HasRunnableCommand(testCase.text); got != testCase.want {
				t.Fatalf("HasRunnableCommand(%q) = %v, want %v", testCase.text, got, testCase.want)
			}
		})
	}
}

func TestHasEnvelopeContinuation(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		want   bool
	}{
		{"collect capture operation", `{"next_transition":{"kind":"collect","collect":{"inputs":[{"capture_operation":"review.capture-result"}]}}}`, true},
		{"execute operation with a runnable command", `{"next_transition":{"kind":"execute","execute":{"operation":"review.finalize","command":"gentle-ai review finalize --lineage=review-1"}}}`, true},
		// The defect this benchmark missed: an execute transition names an
		// operation, carries an empty command, and the reader is told to run
		// something with nothing to run. An operation alone is a label, not a
		// continuation.
		{"execute operation with no command at all", `{"next_transition":{"kind":"execute","execute":{"operation":"review.recover"}}}`, false},
		{"execute operation with an empty command", `{"next_transition":{"kind":"execute","execute":{"operation":"review.recover","command":""}}}`, false},
		{"execute command that is still a template", `{"next_transition":{"kind":"execute","execute":{"operation":"review.validate","command":"gentle-ai review validate --gate <gate>"}}}`, false},
		{"execute command naming no arguments", `{"next_transition":{"kind":"execute","execute":{"operation":"review.status","command":"gentle-ai"}}}`, false},
		{"execute with a command but no operation", `{"next_transition":{"kind":"execute","execute":{"operation":"","command":"gentle-ai review status --cwd=."}}}`, false},
		// A dead execute transition must not be rescued by a continuation key
		// nested inside it: the transition the reader was handed is the thing
		// under test, not the vocabulary it happens to mention.
		{"nested key does not rescue a commandless execute", `{"next_transition":{"kind":"execute","execute":{"operation":"review.recover","binding":{"recovery_operation":"review.recover"}}}}`, false},
		{"next_action", `{"next_action":"review.repair"}`, true},
		{"recovery_operation", `{"context":{"recovery_operation":"review.recover"}}`, true},
		{"stop is not a continuation", `{"next_action":"stop"}`, false},
		{"empty is not a continuation", `{"next_action":""}`, false},
		{"action is deliberately not a continuation key", `{"action":"explicit-maintainer-action"}`, false},
		{"not json", "Error: something went wrong", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := HasEnvelopeContinuation(testCase.stdout); got != testCase.want {
				t.Fatalf("HasEnvelopeContinuation(%q) = %v, want %v", testCase.stdout, got, testCase.want)
			}
		})
	}
}

// TestEnvelopeContinuationMakesABlockInBand covers rule 4 in isolation: an
// envelope that names an operation is in-band even when the prose names
// nothing.
func TestEnvelopeContinuationMakesABlockInBand(t *testing.T) {
	observation := Observation{
		ExitCode:       1,
		Stdout:         `{"result":"invalidated","allowed":false,"next_action":"review.repair"}`,
		StdoutCaptured: true,
		StderrCaptured: true,
	}
	if got := Classify(observation); got != BlockInBand {
		t.Fatalf("Classify = %q, want %q", got, BlockInBand)
	}
}

func TestDeclaredDeadEndOnlyAppliesWhenNothingIsNamed(t *testing.T) {
	base := Observation{ExitCode: 1, Stderr: "Error: nothing to do", StdoutCaptured: true, StderrCaptured: true}
	base.DeclaredDeadEnd = true
	if got := Classify(base); got != BlockDeadEnd {
		t.Fatalf("Classify = %q, want %q", got, BlockDeadEnd)
	}

	// A named continuation always wins over the author's declaration: the
	// mechanical evidence outranks the corpus annotation.
	base.Stderr = "Error: rerun gentle-ai review start --projection staged"
	if got := Classify(base); got != BlockInBand {
		t.Fatalf("Classify = %q, want %q", got, BlockInBand)
	}
}

// TestByDesignNeedsItsQuotedNextActionInTheEmittedBytes is the load-bearing
// guard on the second author-declared input. A declaration is a claim about
// what the product printed, and the claim is CHECKED: if the quoted text is
// not in the emitted bytes, the exemption does not apply and the block stays
// a defect. This is what stops `Error: no.` from being labelled by-design.
func TestByDesignNeedsItsQuotedNextActionInTheEmittedBytes(t *testing.T) {
	declaration := &ByDesignDeclaration{
		Shape:      ByDesignOperatorKnowledge,
		NextAction: "run the same command again from a checkout",
	}

	silent := Observation{
		ExitCode:         1,
		Stderr:           "Error: no.",
		StdoutCaptured:   true,
		StderrCaptured:   true,
		DeclaredByDesign: declaration,
	}
	if got := Classify(silent); got != BlockOutOfBand {
		t.Fatalf("Classify(a refusal that names nothing) = %q, want %q: a declaration cannot excuse a message that says nothing", got, BlockOutOfBand)
	}

	// And the stale declaration must be visible rather than silently dropped.
	outcome := byDesignOutcome(silent, BlockOutOfBand)
	if outcome == nil || outcome.Applied {
		t.Fatalf("outcome = %+v, want a recorded, non-applied declaration", outcome)
	}
	if outcome.Reason == "" {
		t.Fatal("a declaration that did not apply must say why")
	}

	// The same declaration against the bytes the product really emits.
	speaking := loadFixtures(t)["bare_repository_start"]
	speaking.DeclaredByDesign = declaration
	if got := Classify(speaking); got != BlockByDesign {
		t.Fatalf("Classify(the recorded bare-repository refusal) = %q, want %q\nstderr: %s", got, BlockByDesign, speaking.Stderr)
	}
	if outcome := byDesignOutcome(speaking, BlockByDesign); outcome == nil || !outcome.Applied || outcome.Shape != ByDesignOperatorKnowledge {
		t.Fatalf("outcome = %+v, want an applied operator-knowledge exemption", outcome)
	}
}

// TestByDesignLosesToANamedRunnableCommand is the same discipline
// TestDeclaredDeadEndOnlyAppliesWhenNothingIsNamed pins for dead_end:
// mechanical evidence outranks the author's annotation. A block whose output
// names a command that runs is in_band whatever the corpus declared, and the
// stale declaration is recorded rather than swallowed.
func TestByDesignLosesToANamedRunnableCommand(t *testing.T) {
	observation := Observation{
		ExitCode:       1,
		Stderr:         "Error: nothing to review; run the same command again from a checkout, or rerun gentle-ai review start --projection staged",
		StdoutCaptured: true,
		StderrCaptured: true,
		DeclaredByDesign: &ByDesignDeclaration{
			Shape:      ByDesignOperatorKnowledge,
			NextAction: "run the same command again from a checkout",
		},
	}
	if got := Classify(observation); got != BlockInBand {
		t.Fatalf("Classify = %q, want %q: a named runnable command outranks the declaration", got, BlockInBand)
	}
	outcome := byDesignOutcome(observation, BlockInBand)
	if outcome == nil || outcome.Applied || outcome.Reason == "" {
		t.Fatalf("outcome = %+v, want the stale declaration recorded with a reason", outcome)
	}
}

// TestByDesignShapeVocabularyIsClosed proves the shape is not free text.
func TestByDesignShapeVocabularyIsClosed(t *testing.T) {
	for _, shape := range []string{ByDesignOperatorKnowledge, ByDesignWorldAction, ByDesignHumanAuthority} {
		declaration := &ByDesignDeclaration{Shape: shape, NextAction: "do the thing"}
		if err := declaration.Validate(); err != nil {
			t.Fatalf("%s: Validate = %v, want nil", shape, err)
		}
	}
	unrecognised := &ByDesignDeclaration{Shape: "because-i-said-so", NextAction: "do the thing"}
	if err := unrecognised.Validate(); err == nil {
		t.Fatal("an unrecognised shape must be rejected, not accepted as free text")
	}
	quoteless := &ByDesignDeclaration{Shape: ByDesignWorldAction}
	if err := quoteless.Validate(); err == nil {
		t.Fatal("a declaration with no next-action text to verify must be rejected")
	}

	// An unrecognised shape never buys the exemption either, so a corpus error
	// can only ever make a block look WORSE than it is, never better.
	observation := Observation{
		ExitCode: 1, Stderr: "Error: do the thing", StdoutCaptured: true, StderrCaptured: true,
		DeclaredByDesign: unrecognised,
	}
	if got := Classify(observation); got != BlockOutOfBand {
		t.Fatalf("Classify = %q, want %q", got, BlockOutOfBand)
	}
}

func TestSelfRecoveredWinsOverEverything(t *testing.T) {
	observation := Observation{
		ExitCode:      1,
		Stderr:        "Error: rerun gentle-ai review start",
		SelfRecovered: true,
	}
	if got := Classify(observation); got != BlockSelfRecovered {
		t.Fatalf("Classify = %q, want %q", got, BlockSelfRecovered)
	}
}

// TestCorpusDeclarationsAreValid runs the shipped corpus through the same
// check `run` performs before it drives anything. A shape typo in a journey is
// a corpus error, and it fails here rather than quietly costing an exemption.
func TestCorpusDeclarationsAreValid(t *testing.T) {
	if err := validateCorpus(Journeys()); err != nil {
		t.Fatalf("shipped corpus is invalid: %v", err)
	}
}

// TestValidateCorpusFailsLoudly proves each way a declaration can be unusable
// stops the run instead of passing silently.
func TestValidateCorpusFailsLoudly(t *testing.T) {
	step := func(mutate func(*Step)) Journey {
		base := Step{Name: "s", Args: productArgs("review", "start")}
		mutate(&base)
		return Journey{ID: "jXX", Steps: []Step{base}}
	}
	cases := []struct {
		name    string
		journey Journey
		want    string
	}{
		{"unrecognised shape", step(func(s *Step) {
			s.ByDesign = &ByDesignDeclaration{Shape: "because-i-said-so", NextAction: "do the thing"}
		}), "not one of"},
		{"no quoted next action", step(func(s *Step) {
			s.ByDesign = &ByDesignDeclaration{Shape: ByDesignWorldAction}
		}), "next-action"},
		{"both dead_end and by_design", step(func(s *Step) {
			s.DeadEnd = true
			s.ByDesign = &ByDesignDeclaration{Shape: ByDesignWorldAction, NextAction: "free some disk space"}
		}), "opposite"},
		{"declared on a step that never invokes", func() Journey {
			journey := step(func(s *Step) {
				s.ByDesign = &ByDesignDeclaration{Shape: ByDesignWorldAction, NextAction: "free some disk space"}
			})
			journey.Steps[0].Args = nil
			return journey
		}(), "never reaches the classifier"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateCorpus([]Journey{testCase.journey})
			if err == nil {
				t.Fatal("want a corpus error, got nil")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error %q does not explain the problem (want it to mention %q)", err, testCase.want)
			}
		})
	}
}

func TestIsBlockCoversZeroExitDenials(t *testing.T) {
	cases := []struct {
		name string
		o    Observation
		want bool
	}{
		{"allowed false with exit 0", Observation{Stdout: `{"result":"allow","allowed":false}`}, true},
		{"denial result with exit 0", Observation{Stdout: `{"result":"scope-changed"}`}, true},
		{"stop action with exit 0", Observation{Stdout: `{"action":"stop"}`}, true},
		{"allow with exit 0", Observation{Stdout: `{"result":"allow","allowed":true}`}, false},
		{"non json with exit 0", Observation{Stdout: "ok"}, false},
		{"non zero exit", Observation{ExitCode: 1, Stderr: "Error: nope"}, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := IsBlock(testCase.o); got != testCase.want {
				t.Fatalf("IsBlock = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestManualAuthorizationDetection(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"separate value", []string{"review", "abandon", "--maintainer-authorization", "schema\nlineage=x"}, true},
		{"equals value", []string{"review", "abandon", "--maintainer-authorization=schema\nlineage=x"}, true},
		{"single dash spelling", []string{"review", "abandon", "-maintainer-authorization", "token"}, true},
		{"empty value", []string{"review", "abandon", "--maintainer-authorization", "  "}, false},
		{"flag absent", []string{"review", "abandon", "--lineage", "x"}, false},
		{"trailing flag with no value", []string{"review", "abandon", "--maintainer-authorization"}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := hasManualAuthorization(testCase.args); got != testCase.want {
				t.Fatalf("hasManualAuthorization(%v) = %v, want %v", testCase.args, got, testCase.want)
			}
		})
	}
}

// TestConsentNoticeIsCountedFromRealStderr proves dimension 1 is grounded in
// the product's own string, not a paraphrase.
func TestConsentNoticeIsCountedFromRealStderr(t *testing.T) {
	fixtures := loadFixtures(t)
	if got := countConsentNotices(fixtures["high_risk_start"].Stderr); got != 1 {
		t.Fatalf("consent notices in a high-risk start = %d, want 1\nstderr: %q",
			got, fixtures["high_risk_start"].Stderr)
	}
	if got := countConsentNotices(fixtures["low_risk_start"].Stderr); got != 0 {
		t.Fatalf("consent notices in a low-risk start = %d, want 0", got)
	}
}

func TestCaptureResultDetectionExcludesPreflight(t *testing.T) {
	if !isCaptureResult([]string{"review", "capture-result", "--lens", "review-risk", "--input", "r.json"}) {
		t.Fatal("a real capture must count as a model run")
	}
	if isCaptureResult([]string{"review", "capture-result", "--preflight", "--lens", "review-risk"}) {
		t.Fatal("a preflight reads no reviewer result and must not count as a model run")
	}
	if isCaptureResult([]string{"review", "capture-evidence", "--input", "e.txt"}) {
		t.Fatal("evidence capture is not a reviewer run")
	}
}

// A declared dead end that the product has since outgrown must be reported,
// not silently outranked. `dead_end` is the most expensive claim the corpus
// allows, so a stale one left unsaid keeps advertising a closed gap as open.
func TestStaleDeadEndDeclarationIsReportedNotDropped(t *testing.T) {
	observation := Observation{
		DeclaredDeadEnd: true,
		ExitCode:        1,
		Stderr:          "review abandon refused: capture the diagnosis with `gentle-ai review inspect-authority --cwd \"/repo\"` and escalate that report",
	}
	class := Classify(observation)
	if class != BlockInBand {
		t.Fatalf("a refusal naming a runnable command must classify in_band, got %q", class)
	}
	outcome := deadEndOutcome(observation, class)
	if outcome == nil {
		t.Fatal("a declaration the classifier refused must still be recorded")
	}
	if outcome.Applied {
		t.Fatal("the declaration must not be reported as applied")
	}
	if !strings.Contains(outcome.Reason, "stale") {
		t.Fatalf("the reason must say the declaration is stale, got %q", outcome.Reason)
	}
}

// The ordinary case: no declaration means no accounting entry at all.
func TestUndeclaredStepsCarryNoDeadEndOutcome(t *testing.T) {
	if outcome := deadEndOutcome(Observation{ExitCode: 1}, BlockOutOfBand); outcome != nil {
		t.Fatalf("undeclared step produced %#v", outcome)
	}
}
