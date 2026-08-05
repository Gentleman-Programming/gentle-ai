package sddstatus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/consentenvelope"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// SDDIntegrationConsentSchema identifies the SDD edit-authority consent
// question (#2540 S4a), in the same envelope family as
// gentle-ai.review-integration.consent/v2.
const SDDIntegrationConsentSchema = "gentle-ai.sdd-integration.consent/v1"

// SDDIntegrationConsentSchemaID is the published JSON-schema identity for the
// envelope, mirroring the review-integration contract layout.
const SDDIntegrationConsentSchemaID = "https://gentle-ai.dev/contracts/sdd-integration/v1/schemas/consent.schema.json"

// SDDIntegrationContractV1 names the sdd-integration contract the envelope
// belongs to.
const SDDIntegrationContractV1 = "gentle-ai.sdd-integration/v1"

const (
	sddConsentOperation      = "sdd-attempt.grant"
	sddConsentActionRequired = "consent_required"

	// sddConsentAnswerGranted and sddConsentAnswerDeclined are the machine
	// answer tokens, identical to the review family so orchestrators relay
	// one vocabulary.
	sddConsentAnswerGranted  = "granted"
	sddConsentAnswerDeclined = "declined"

	// sddConsentGrantInvocationPrefix fronts the real `sdd-attempt grant`
	// verb as shipped by S3 (#2558) plus S5's reconciliation (#2559):
	// `sdd-attempt grant --cwd <repo> --change <name>
	// [--expected-revision <rev>] --root <path>... --actor <actor>
	// --reason <reason> --request-id <id> --change-instance <token>`. S4a's
	// provisional pin lacked --request-id (found by S3) and
	// --change-instance (added by S5); #2563 reconciled the fixture, the
	// schema regex, and the test pin together to this flag set.
	sddConsentGrantInvocationPrefix = "gentle-ai sdd-attempt grant "

	// sddConsentStatusInvocationPrefix is the decline and off-path re-entry:
	// declining persists nothing, so the runnable follow-up is native SDD
	// status for the same change.
	sddConsentStatusInvocationPrefix = "gentle-ai sdd-status "
)

// SDDIntegrationConsentResult is the typed blocking consent question a
// multi-repository SDD change raises when its task plan targets repository
// roots outside the authorized edit roots (#2540). Like the review consent
// envelope it is a Lossless Blocking Prompt: WHY input is required, the
// COMPLETE choice set, and the EXACT runnable way to answer, scoped to one
// change. The identity block is the change name plus the missing roots.
// Granting is per-change and audited; declining persists nothing and the
// change stays blocked. Emission into native SDD status is a later slice
// (S4b); this type pins the contract first.
type SDDIntegrationConsentResult struct {
	Schema    string `json:"schema"`
	Contract  string `json:"contract"`
	Operation string `json:"operation"`
	Action    string `json:"action"`
	// Blocking marks this envelope as a decision the caller must relay before
	// apply can proceed; nothing has been persisted.
	Blocking bool `json:"blocking"`
	// Change and MissingRoots are the identity block: which change is blocked
	// and which resolved repository roots no edit authority covers.
	Change       string   `json:"change"`
	MissingRoots []string `json:"missing_roots"`
	Headline     string   `json:"headline"`
	Reason       string   `json:"reason"`
	Value        string   `json:"value"`
	Evidence     []string `json:"evidence"`
	// Choices carries exactly the granted and declined tokens; the granted
	// invocation is the provisional S3 grant verb, the declined invocation is
	// the status re-entry that shows the block again.
	Choices []consentenvelope.Choice `json:"choices"`
	// OffPath documents the deliberate alternative outside the choice set:
	// keep the change single-repository by editing its tasks.md, then
	// re-enter through native status.
	OffPath consentenvelope.OffPath `json:"off_path"`
}

// Validate enforces the envelope's SDD identity and delegates the generic
// completeness half to the shared consent-envelope core (#2554).
func (result SDDIntegrationConsentResult) Validate() error {
	if result.Schema != SDDIntegrationConsentSchema || result.Contract != SDDIntegrationContractV1 ||
		result.Operation != sddConsentOperation || result.Action != sddConsentActionRequired || !result.Blocking {
		return errors.New("invalid SDD consent question identity") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	if strings.TrimSpace(result.Change) == "" {
		return errors.New("SDD consent question requires the blocked change name") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	if len(result.MissingRoots) == 0 {
		return errors.New("SDD consent question requires the missing repository roots") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	for _, root := range result.MissingRoots {
		if strings.TrimSpace(root) == "" {
			return errors.New("SDD consent question names a blank missing root") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
		}
	}
	core := consentenvelope.Core{
		Headline: result.Headline, Reason: result.Reason, Value: result.Value,
		Evidence: result.Evidence, Choices: result.Choices, OffPath: result.OffPath,
	}
	if err := core.ValidateCompleteness(sddConsentAnswerGranted, sddConsentAnswerDeclined); err != nil {
		return err
	}
	// The missing roots ARE the evidence: every root the human is asked to
	// authorize must appear in the evidence the envelope shows them.
	for _, root := range result.MissingRoots {
		if !sddConsentEvidenceNames(result.Evidence, root) {
			return fmt.Errorf("SDD consent evidence does not name missing root %q", root) // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
		}
	}
	granted := result.Choices[0]
	if !strings.HasPrefix(granted.Invocation, sddConsentGrantInvocationPrefix) ||
		!strings.Contains(granted.Invocation, " --cwd ") ||
		!strings.Contains(granted.Invocation, " --change "+result.Change) ||
		!strings.Contains(granted.Invocation, " --actor ") ||
		!strings.Contains(granted.Invocation, " --reason ") ||
		!strings.Contains(granted.Invocation, " --request-id ") ||
		!strings.Contains(granted.Invocation, " --change-instance ") {
		return errors.New("SDD consent grant choice does not name the runnable grant invocation") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	for _, root := range result.MissingRoots {
		if !strings.Contains(granted.Invocation, " --root "+pathquote.Quote(root)) {
			return fmt.Errorf("SDD consent grant invocation does not carry missing root %q", root) // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
		}
	}
	declined := result.Choices[1]
	if !strings.HasPrefix(declined.Invocation, sddConsentStatusInvocationPrefix) ||
		!strings.Contains(declined.Invocation, result.Change) ||
		!strings.Contains(declined.Invocation, " --cwd ") {
		return errors.New("SDD consent decline choice does not name the status re-entry for the blocked change") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	if !strings.HasPrefix(result.OffPath.Command, sddConsentStatusInvocationPrefix) {
		return errors.New("SDD consent off path must re-enter through native status") // refusal:by-design world-action: this envelope is built and validated by the same package; the exit is a code fix, not a command
	}
	return nil
}

func sddConsentEvidenceNames(evidence []string, root string) bool {
	for _, entry := range evidence {
		if strings.Contains(entry, root) {
			return true
		}
	}
	return false
}
