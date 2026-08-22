package cli

// These tests drive the negotiated consent-relay surface end to end through
// the real RunReview router: a caller that declares it can relay a blocking
// question receives the typed consent envelope instead of the silent
// skip-and-notice, answers it with a runnable follow-up invocation scoped to
// the exact frozen candidate, and a caller that declares nothing keeps
// today's behavior byte for byte (the start-v2 fixture test pins the exact
// bytes; TestNegotiatedStartWithoutConsentDeclarationKeepsTodaysEnvelope pins
// the field set strictly).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// runConsentRelayStart drives one negotiated START through the real router
// and returns stdout.
func runConsentRelayStart(t *testing.T, args []string) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	if err := RunReview(args, &output); err != nil {
		t.Fatalf("negotiated START: %v\n%s", err, output.String())
	}
	return &output
}

func decodeConsentQuestion(t *testing.T, payload []byte) ReviewIntegrationConsentResult {
	t.Helper()
	var result ReviewIntegrationConsentResult
	decodeStrictReviewJSON(t, payload, &result)
	return result
}

// invocationArgs turns a runnable `gentle-ai review start ...` invocation from
// the consent envelope into router arguments, proving the invocation is
// literally runnable rather than merely descriptive.
func invocationArgs(t *testing.T, invocation string) []string {
	t.Helper()
	words, err := SplitPrintedCommandWords(invocation)
	if err != nil {
		t.Fatalf("parse consent invocation: %v", err)
	}
	if len(words) < 3 || words[0] != "gentle-ai" || words[1] != "review" || words[2] != "start" {
		t.Fatalf("consent invocation is not a runnable gentle-ai review start command: %q", invocation)
	}
	return words[2:]
}

func normalizeConsentFixtureCWD(t *testing.T, payload []byte, root string) []byte {
	t.Helper()
	var envelope struct {
		Choices []struct {
			Invocation string `json:"invocation"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode consent fixture: %v", err)
	}

	normalized := append([]byte(nil), payload...)
	for _, choice := range envelope.Choices {
		args := invocationArgs(t, choice.Invocation)
		cwd := -1
		for index, arg := range args {
			if arg != "--cwd" {
				continue
			}
			if cwd >= 0 {
				t.Fatalf("consent invocation repeats --cwd: %q", choice.Invocation)
			}
			cwd = index
		}
		if cwd < 0 || cwd+1 >= len(args) || args[cwd+1] != root {
			t.Fatalf("consent invocation CWD = %q, want %q: %q", args, root, choice.Invocation)
		}
		args[cwd+1] = "/repo"

		words := append([]string{"gentle-ai", "review"}, args...)
		for index, word := range words {
			words[index] = reviewTransitionShellWord(word)
		}
		normalizedInvocation := strings.Join(words, " ")
		encodedInvocation, err := json.Marshal(choice.Invocation)
		if err != nil {
			t.Fatalf("encode consent invocation: %v", err)
		}
		encodedNormalizedInvocation, err := json.Marshal(normalizedInvocation)
		if err != nil {
			t.Fatalf("encode normalized consent invocation: %v", err)
		}
		if next := bytes.ReplaceAll(normalized, encodedInvocation, encodedNormalizedInvocation); bytes.Equal(next, normalized) {
			t.Fatalf("consent invocation not found in fixture payload: %q", choice.Invocation)
		} else {
			normalized = next
		}
	}
	return normalized
}

func TestInvocationArgsParsesQuotedWindowsConsentFollowUp(t *testing.T) {
	root := `C:\Users\Jane Doe\repo`
	invocation := "gentle-ai review start --cwd '" + root + "' --contract " + ReviewIntegrationContractV1

	want := []string{"start", "--cwd", root, "--contract", ReviewIntegrationContractV1}
	if got := invocationArgs(t, invocation); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("invocation args = %#v, want %#v", got, want)
	}
}

func TestConsentFixtureNormalizationStripsRenderedWindowsCWDQuoting(t *testing.T) {
	root := `C:\Users\Jane Doe\repo`
	payload, err := json.Marshal(ReviewIntegrationConsentResult{Choices: []ReviewIntegrationConsentChoice{{
		Invocation: "gentle-ai review start --cwd '" + root + "' --contract " + ReviewIntegrationContractV1,
	}}})
	if err != nil {
		t.Fatal(err)
	}

	normalized := normalizeConsentFixtureCWD(t, payload, root)
	if !bytes.Contains(normalized, []byte("--cwd /repo")) || bytes.Contains(normalized, []byte("--cwd '/repo'")) {
		t.Fatalf("normalized fixture CWD = %s, want canonical --cwd /repo", normalized)
	}
}

func TestNegotiatedHighRiskStartWithRelayDeclarationEmitsBlockingConsentQuestion(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-consent-question", "--consent", "relay",
	}))
	question := decodeConsentQuestion(t, output.Bytes())
	if err := question.Validate(); err != nil {
		t.Fatalf("consent question does not validate: %v\n%s", err, output.String())
	}
	if question.Schema != ReviewIntegrationConsentSchema || question.Contract != ReviewIntegrationContractV1 ||
		question.Operation != "review.start" || question.Action != "consent_required" || !question.Blocking {
		t.Fatalf("consent question identity = %#v", question)
	}
	if question.RiskLevel != reviewtransaction.RiskHigh || question.ChangedFiles != 1 || question.ChangedLines != 1 {
		t.Fatalf("consent question does not describe the frozen candidate: %#v", question)
	}
	if !strings.HasPrefix(question.TargetIdentity, "sha256:") {
		t.Fatalf("consent question carries no exact target identity: %#v", question)
	}
	// The envelope must carry the same semantic phrases the interactive question
	// uses, so the orchestrator can localize the complete decision faithfully.
	if question.Headline != reviewConsentHeadline || question.Value != reviewConsentValue {
		t.Fatalf("consent question dropped the interactive framing: %#v", question)
	}
	if !strings.Contains(question.Reason, "deeper review") {
		t.Fatalf("consent question reason does not explain the tier: %q", question.Reason)
	}
	evidence := strings.Join(question.RiskEvidence, "\n")
	if !strings.Contains(evidence, "shell scripting in scripts/deploy.sh") {
		t.Fatalf("consent question does not name the risk evidence: %q", evidence)
	}
	// The complete choice set: proceed and decline-once. The kill switch stays
	// a documented off path, never a numbered choice, because the interactive
	// prompt's own design says a decline is not the kill switch.
	if len(question.Choices) != 2 {
		t.Fatalf("consent question offers %d choices, want exactly 2: %#v", len(question.Choices), question.Choices)
	}
	granted, declined := question.Choices[0], question.Choices[1]
	if granted.Answer != "granted" || granted.Label != reviewConsentAnswerRunLabel ||
		declined.Answer != "declined" || declined.Label != reviewConsentAnswerNotNowLabel {
		t.Fatalf("consent choices = %#v", question.Choices)
	}
	for _, choice := range question.Choices {
		if !strings.Contains(choice.Invocation, "--consent "+choice.Answer) ||
			!strings.Contains(choice.Invocation, "--target "+question.TargetIdentity) ||
			!strings.Contains(choice.Invocation, "--contract "+ReviewIntegrationContractV1) ||
			!strings.Contains(choice.Invocation, "--lineage review-consent-question") {
			t.Fatalf("consent invocation is not scoped and runnable: %#v", choice)
		}
		if choice.Effect == "" {
			t.Fatalf("consent choice states no effect: %#v", choice)
		}
	}
	if !strings.Contains(declined.Effect, "not the kill switch") {
		t.Fatalf("decline effect must say it is not the kill switch: %q", declined.Effect)
	}
	if !strings.Contains(granted.Effect, "exact frozen candidate") || !strings.Contains(granted.Effect, "asks again") {
		t.Fatalf("grant effect must remain candidate-scoped: %q", granted.Effect)
	}
	if question.OffPath.Command != reviewConsentOffPathCommand || !strings.Contains(question.OffPath.Note, "for good") {
		t.Fatalf("consent off path = %#v", question.OffPath)
	}

	// Asking is not persisting: no authority, no latch, and no skip notice.
	if store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-consent-question"); err == nil {
		if _, loadErr := store.Load(); !errors.Is(loadErr, os.ErrNotExist) {
			t.Fatalf("consent question persisted review authority: %v", loadErr)
		}
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("consent question consumed the one-time latch: asked=%v err=%v", asked, err)
	}
	if console.Len() != 0 {
		t.Fatalf("relay declaration must replace the console notice, not add to it:\n%s", console.String())
	}
}

func TestRelayedConsentGrantRunsTheReviewAndIsReplaySafe(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-consent-grant", "--consent", "relay",
	}))
	question := decodeConsentQuestion(t, output.Bytes())
	if question.Contract != ReviewIntegrationContractV2 || question.Schema != ReviewIntegrationConsentSchemaV3 || question.Agent != "claude-code" {
		t.Fatalf("v2.1 relay question identity = %#v", question)
	}

	grantArgs := invocationArgs(t, question.Choices[0].Invocation)
	started := decodeNegotiatedReviewStart(t, runConsentRelayStart(t, grantArgs).Bytes())
	if started.Action != "created" || started.LineageID != "review-consent-grant" ||
		started.RiskLevel != reviewtransaction.RiskHigh || len(started.SelectedLenses) != 4 {
		t.Fatalf("granted consent did not reach the lens plan: %#v", started)
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("a negotiated grant must not touch the legacy latch: asked=%v err=%v", asked, err)
	}

	// The grant leg replays the same compact authority.
	replayed := decodeNegotiatedReviewStart(t, runConsentRelayStart(t, grantArgs).Bytes())
	if replayed.Action != "replayed" || replayed.LineageID != started.LineageID {
		t.Fatalf("replayed grant = %#v", replayed)
	}

	// Candidate A's grant authorizes only A. Candidate B receives a fresh
	// question even though global review mode still permits review.
	writeReviewStartCandidate(t, repo, "scripts/second.sh", "echo second\n", 0o644)
	next := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-consent-grant-next", "--consent", "relay",
	})).Bytes())
	if next.Action != reviewConsentActionRequired || next.TargetIdentity == question.TargetIdentity {
		t.Fatalf("candidate B did not receive its own consent question: %#v", next)
	}
}

func TestPriorCloneLatchCannotSuppressNegotiatedRelay(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	if err := reviewtransaction.RecordRDDConsentAsked(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	question := decodeConsentQuestion(t, runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV2, "--cwd", repo,
		"--lineage", "review-consent-prior-latch", "--consent", "relay",
	})).Bytes())
	if question.Action != reviewConsentActionRequired || question.Contract != ReviewIntegrationContractV2 {
		t.Fatalf("legacy latch suppressed v2 relay: %#v", question)
	}
}

func TestGlobalReviewModeEnabledPermitsButDoesNotGrantV2Consent(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	var modeOutput bytes.Buffer
	if err := RunReviewMode([]string{"enable", "--cwd", repo, "--json"}, &modeOutput); err != nil {
		t.Fatal(err)
	}
	if mode := decodeReviewModeResult(t, modeOutput.Bytes()).Status; mode.Effective != reviewtransaction.RDDModeOn {
		t.Fatalf("global review mode = %#v", mode)
	}
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	status := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV2, "--lineage", "review-consent-global-enabled")
	question := decodeConsentQuestion(t, runConsentRelayStart(t, transitionStartArgs(repo, status)).Bytes())
	if question.Action != reviewConsentActionRequired || !question.Blocking {
		t.Fatalf("global enabled was treated as automatic candidate consent: %#v", question)
	}
}

// TestGlobalReviewModeOffRefusesBeforeV2Consent still tests an explicit off,
// so it opts in first and then turns the switch off: the START transition it
// disables has to exist before the refusal means anything. An unconfigured home
// would refuse too, but for the shipped opt-in default rather than the explicit
// global off this test is about.
func TestGlobalReviewModeOffRefusesBeforeV2Consent(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)
	status := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV2, "--lineage", "review-consent-global-off")
	var modeOutput bytes.Buffer
	if err := RunReviewMode([]string{"disable", "--cwd", repo, "--json"}, &modeOutput); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	startArgs := transitionStartArgs(repo, status)
	if strings.Contains(strings.Join(startArgs, " "), "--agent=") {
		t.Fatalf("manual v2 disabled START invented a runtime identity: %v", startArgs)
	}
	err := RunReview(startArgs, &output)
	if err == nil || strings.Contains(output.String(), reviewConsentActionRequired) {
		t.Fatalf("global off did not refuse before consent: err=%v\n%s", err, output.String())
	}
}

func TestRelayedConsentDeclineIsScopedToTheCandidate(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	relayArgs := boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-consent-decline", "--consent", "relay",
	})
	question := decodeConsentQuestion(t, runConsentRelayStart(t, relayArgs).Bytes())

	declineArgs := invocationArgs(t, question.Choices[1].Invocation)
	first := runConsentRelayStart(t, declineArgs)
	var declinedResult ReviewFacadeStartResult
	decodeStrictReviewJSON(t, first.Bytes(), &declinedResult)
	if declinedResult.Action != "declined" || declinedResult.Consent != ReviewStartConsentDeclinedThisCandidate ||
		declinedResult.TargetIdentity != question.TargetIdentity || declinedResult.LineageID != "" ||
		len(declinedResult.SelectedLenses) != 0 {
		t.Fatalf("declined START = %#v", declinedResult)
	}
	if store, err := reviewtransaction.CompactAuthoritativeStore(context.Background(), repo, "review-consent-decline"); err == nil {
		if _, loadErr := store.Load(); !errors.Is(loadErr, os.ErrNotExist) {
			t.Fatalf("declined START persisted review authority: %v", loadErr)
		}
	}
	if asked, err := reviewtransaction.RDDConsentAsked(context.Background(), repo); err != nil || asked {
		t.Fatalf("a decline must never latch: asked=%v err=%v", asked, err)
	}

	// Replay safety: the same decline invocation reports the same choice.
	replay := runConsentRelayStart(t, declineArgs)
	if !bytes.Equal(first.Bytes(), replay.Bytes()) {
		t.Fatalf("replayed decline differs:\nfirst=%s\nreplay=%s", first.String(), replay.String())
	}

	// The decline is scoped to this one candidate: the same relay declaration
	// asks the question again rather than remembering the no.
	again := decodeConsentQuestion(t, runConsentRelayStart(t, relayArgs).Bytes())
	if again.Action != "consent_required" || again.TargetIdentity != question.TargetIdentity {
		t.Fatalf("relay after a decline must ask again for the candidate: %#v", again)
	}
}

// TestNegotiatedStartWithoutConsentDeclarationKeepsTodaysEnvelope pins the
// no-declaration path: the negotiated envelope carries no consent fields (the
// strict decoder refuses unknown fields), the start completes in the
// fail-safe direction, and the console stays byte-silent — the skip notices
// are a human surface reserved for plain (non-negotiated) starts, because a
// successful negotiated operation writes zero bytes to stderr. Byte-identity
// of the envelope itself is pinned by
// TestNegotiatedReviewStartMatchesVersionedFixture.
func TestNegotiatedStartWithoutConsentDeclarationKeepsTodaysEnvelope(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-undeclared",
	}))
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.Action != "created" || len(started.SelectedLenses) != 4 {
		t.Fatalf("undeclared negotiated START changed behavior: %#v", started)
	}
	if strings.Contains(output.String(), "consent") {
		t.Fatalf("undeclared negotiated START leaked consent fields:\n%s", output.String())
	}
	if console.Len() != 0 {
		t.Fatalf("undeclared negotiated START must stay silent on the console:\n%s", console.String())
	}
}

func TestLowRiskStartWithRelayDeclarationProceedsWithoutQuestion(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "docs/notes.md", "notes\n", 0o644)

	status := negotiatedStartStatusForContract(t, repo, ReviewIntegrationContractV2, "--lineage", "review-consent-low")
	output := runConsentRelayStart(t, transitionStartArgs(repo, status))
	started := decodeNegotiatedReviewStart(t, output.Bytes())
	if started.RiskLevel != reviewtransaction.RiskLow || len(started.SelectedLenses) != 0 ||
		started.Action != "created" {
		t.Fatalf("low-risk relay START = %#v", started)
	}
	if console.Len() != 0 {
		t.Fatalf("a low-risk start has nothing to consent to and nothing to announce:\n%s", console.String())
	}
}

func TestConsentDeclineOnLowRiskCandidateIsRefused(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "docs/notes.md", "notes\n", 0o644)

	var output bytes.Buffer
	err := RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo,
		"--lineage", "review-consent-low-decline", "--consent", "declined",
	}), &output)
	if err == nil || !strings.Contains(output.String(), "nothing to decline") ||
		!strings.Contains(output.String(), "rerun gentle-ai review start without --consent") {
		t.Fatalf("low-risk decline must be refused with the reason and rerun: %v\n%s", err, output.String())
	}
}

func TestConsentFlagRequiresNegotiatedContract(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	err := RunReview([]string{"start", "--cwd", repo, "--consent", "relay"}, &output)
	if err == nil || !strings.Contains(err.Error(), "--contract") {
		t.Fatalf("--consent without --contract must name the negotiated rerun: %v", err)
	}

	output.Reset()
	err = RunReview(boundNegotiatedStartArgs(t, []string{
		"start", "--contract", ReviewIntegrationContractV1, "--cwd", repo, "--consent", "sometimes",
	}), &output)
	if err == nil || !strings.Contains(output.String(), "relay, granted, or declined") {
		t.Fatalf("an unknown consent value must name the exact allowed answers: %v\n%s", err, output.String())
	}
}

// TestHeadlessSkipNoticeFollowsAnExplicitOptIn replaces a provenance notice
// that can no longer happen. It used to cover the case where reviews ran
// because nobody chose anything, and the notice had to admit that. Making
// receipt-driven development opt-in deletes that case outright: a default-source
// clone never reaches the consent ceremony, because the start is refused before
// it. So the two halves of the old test become the two facts worth pinning --
// an unconfigured headless clone is refused and told how to opt in, and an
// explicitly enabled one gets the skip notice with no provenance excuse
// attached, because there is no longer any way for reviews to be on by accident.
func TestHeadlessSkipNoticeFollowsAnExplicitOptIn(t *testing.T) {
	reviewModeHome(t)
	repo := initReviewCLIRepo(t)
	console := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

	var output bytes.Buffer
	err := RunReviewFacadeStart([]string{"--cwd", repo, "--lineage", "review-provenance-default"}, &output)
	if err == nil {
		t.Fatalf("an unconfigured clone started a review nobody asked for:\n%s", output.String())
	}
	if !strings.Contains(err.Error(), "gentle-ai review mode enable --scope=global") {
		t.Fatalf("the opt-in refusal names no way in: %v", err)
	}
	if console.String() != "" {
		t.Fatalf("a refused start reached the consent ceremony:\n%s", console.String())
	}

	// An explicit opt-in is the only route to a review, and its headless notice
	// carries no provenance sentence: the switch was chosen, not inherited.
	reviewEnabledHome(t)
	repoExplicit := initReviewCLIRepo(t)
	consoleExplicit := stubReviewConsole(t, false, "")
	writeReviewStartCandidate(t, repoExplicit, "scripts/deploy.sh", "echo deploy\n", 0o644)
	output.Reset()
	if err := RunReviewFacadeStart([]string{"--cwd", repoExplicit, "--lineage", "review-provenance-explicit"}, &output); err != nil {
		t.Fatalf("headless start with explicit mode: %v\n%s", err, output.String())
	}
	explicitNotice := consoleExplicit.String()
	if !strings.Contains(explicitNotice, reviewConsentSkippedNotice) {
		t.Fatalf("explicit-source start lost the skip notice:\n%s", explicitNotice)
	}
	if strings.Contains(explicitNotice, "by default") {
		t.Fatalf("an explicitly chosen mode must not claim a default: %s", explicitNotice)
	}
}

// TestConsentQuestionMatchesVersionedFixture pins the consent question to the
// versioned contract artifact, the same way the negotiated START response is
// pinned to start-v2.fixture.json. The repository path inside the runnable
// invocations is the one caller-relative value, normalized to the fixture's
// /repo placeholder.
func TestConsentQuestionMatchesVersionedFixture(t *testing.T) {
	for _, tt := range []struct {
		name, contract, fixture, schema string
	}{
		{name: "v1", contract: ReviewIntegrationContractV1, fixture: filepath.Join("v1", "fixtures", "consent.fixture.json"), schema: filepath.Join("v1", "schemas", "consent.schema.json")},
		{name: "v2.1", contract: ReviewIntegrationContractV2, fixture: filepath.Join("v2", "fixtures", "consent-v3.fixture.json"), schema: filepath.Join("v2", "schemas", "consent-v3.schema.json")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reviewEnabledHome(t)
			repo := initReviewCLIRepo(t)
			root, err := resolveReviewMutationRoot(context.Background(), repo)
			if err != nil {
				t.Fatalf("resolve fixture repository root: %v", err)
			}
			stubReviewConsole(t, false, "")
			writeReviewStartCandidate(t, repo, "scripts/deploy.sh", "echo deploy\n", 0o644)

			output := runConsentRelayStart(t, boundNegotiatedStartArgs(t, []string{
				"start", "--contract", tt.contract, "--cwd", repo,
				"--lineage", "review-consent-fixture", "--consent", "relay",
			}))
			question := decodeConsentQuestion(t, output.Bytes())
			if err := question.Validate(); err != nil {
				t.Fatal(err)
			}
			normalized := normalizeConsentFixtureCWD(t, output.Bytes(), root)
			fixturePath := filepath.Join("..", "..", "contracts", "review-integration", tt.fixture)
			if os.Getenv("GENTLE_AI_CONSENT_FIXTURE_UPDATE") == "1" {
				if err := os.WriteFile(fixturePath, normalized, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			fixture, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(normalized, fixture) {
				t.Fatalf("consent fixture mismatch:\ngot=%s\nwant=%s", normalized, fixture)
			}

			schemaPayload, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", tt.schema))
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(schemaPayload, &schema); err != nil {
				t.Fatal(err)
			}
			wantSchemaID := ReviewIntegrationConsentSchemaID
			if tt.contract == ReviewIntegrationContractV2 {
				wantSchemaID = ReviewIntegrationConsentSchemaIDV3
			}
			if schema["$id"] != wantSchemaID || schema["additionalProperties"] != false {
				t.Fatalf("consent schema identity = %v additionalProperties = %v", schema["$id"], schema["additionalProperties"])
			}
			var fixtureFields map[string]any
			if err := json.Unmarshal(fixture, &fixtureFields); err != nil {
				t.Fatal(err)
			}
			required, _ := schema["required"].([]any)
			requiredNames := make(map[string]bool, len(required))
			for _, name := range required {
				requiredNames[fmt.Sprint(name)] = true
			}
			for field := range fixtureFields {
				if !requiredNames[field] {
					t.Fatalf("consent fixture field %q is not required by the schema", field)
				}
			}
			if len(requiredNames) != len(fixtureFields) {
				t.Fatalf("consent schema requires %d fields, fixture carries %d", len(requiredNames), len(fixtureFields))
			}
		})
	}
}

func TestV21ConsentInvocationMustMatchProviderOwnedRequest(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("..", "..", "contracts", "review-integration", "v2", "fixtures", "consent-v3.fixture.json"))
	if err != nil {
		t.Fatal(err)
	}
	var question ReviewIntegrationConsentResult
	if err := json.Unmarshal(fixture, &question); err != nil {
		t.Fatal(err)
	}
	base := reviewConsentFollowUpBase("/repo", question.TargetIdentity, question.Projection, "review-consent-fixture", "", "", "reliability", "", false, false, ReviewIntegrationContractV2, "", "", reviewIntendedUntrackedScope{})
	if err := validateReviewConsentInvocations(question, base); err != nil {
		t.Fatalf("canonical v2.1 consent invocation: %v", err)
	}

	for _, test := range []struct {
		name       string
		invocation string
	}{
		{name: "unexpected Claude agent", invocation: strings.Replace(question.Choices[0].Invocation, " --consent granted", " --agent claude-code --consent granted", 1)},
		{name: "unexpected OpenCode agent", invocation: strings.Replace(question.Choices[0].Invocation, " --consent granted", " --agent opencode --consent granted", 1)},
		{name: "duplicate unexpected agent", invocation: strings.Replace(question.Choices[0].Invocation, " --consent granted", " --agent claude-code --agent opencode --consent granted", 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := question
			mutated.Choices = append([]ReviewIntegrationConsentChoice(nil), question.Choices...)
			mutated.Choices[0].Invocation = test.invocation
			if err := validateReviewConsentInvocations(mutated, base); err == nil {
				t.Fatalf("accepted non-canonical invocation %q", test.invocation)
			}
		})
	}
}
