package sddstatus

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/consentenvelope"
	"github.com/gentleman-programming/gentle-ai/v2/internal/pathquote"
)

// Issue #2563 (S4b of #2540): the status layer owns the change-instance
// identity that S5 (#2557) made the ledger's containment boundary. The ledger
// keys its chain by change name alone and archive never touches it, so the
// identity must live somewhere that IS the change instance: the change's own
// directory. The marker file travels with the directory into archive/ and a
// recreated change starts with a fresh directory, so the token is stable
// across one instance's life and distinct across recreations — exactly the
// meaning RuntimeStore.ForInstance requires its caller to own. Deriving the
// token from artifact content instead was rejected: a recreated change with
// byte-identical artifacts would inherit the archived change's authority,
// which is the resurrection hazard S5 exists to close.
const changeInstanceMarkerFile = ".gentle-ai-instance"

const (
	// sddConsentGrantActor and sddConsentGrantReason are the audit fields the
	// envelope's grant invocation carries. The consent conversation itself is
	// the authorization evidence; the ledger records who ran it and when.
	sddConsentGrantActor  = "maintainer"
	sddConsentGrantReason = "edit-authority-consent-granted"
)

// readChangeInstanceMarker loads the persisted change-instance token, or ""
// when none has been minted. It never mints: an ordinary status on a
// change with no missing edit roots must leave zero filesystem footprint.
func readChangeInstanceMarker(changeRoot string) (string, error) {
	payload, err := os.ReadFile(filepath.Join(changeRoot, changeInstanceMarkerFile))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read change-instance marker: %w", err)
	}
	return strings.TrimSpace(string(payload)), nil
}

// ensureChangeInstanceMarker returns the existing token or mints and persists
// a fresh opaque one. Minting happens only when the consent envelope needs a
// token to embed, so the marker exists exactly for changes that ever raised
// the edit-authority question.
func ensureChangeInstanceMarker(changeRoot string) (string, error) {
	existing, err := readChangeInstanceMarker(changeRoot)
	if err != nil || existing != "" {
		return existing, err
	}
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return "", fmt.Errorf("mint change-instance identity: %w", err)
	}
	token := "sdd-" + hex.EncodeToString(seed)
	if err := os.WriteFile(filepath.Join(changeRoot, changeInstanceMarkerFile), []byte(token+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("persist change-instance marker: %w", err)
	}
	return token, nil
}

// sddConsentGrantRequestID derives the grant invocation's request-id from the
// exact inputs the grant would bind. Deterministic on purpose: re-rendering
// the same blocked status names the same request-id, so an accidentally
// repeated execution replays idempotently instead of double-granting, while a
// widening (different roots) or a moved ledger head (different expected
// revision) derives a fresh id.
func sddConsentGrantRequestID(change, instance, expectedRevision string, roots []string) string {
	hash := sha256.New()
	for _, part := range append([]string{"gentle-ai.sdd-consent-grant-request/v1", change, instance, expectedRevision}, roots...) {
		hash.Write([]byte(part))
		hash.Write([]byte{0})
	}
	return "grant-" + hex.EncodeToString(hash.Sum(nil))[:16]
}

// newEditAuthorityConsent builds the typed blocking consent question for one
// blocked(edit_authority_missing) status: the missing edit roots are the evidence,
// the granted choice names the exact runnable grant invocation (including the
// persisted change-instance token and, when the ledger already has a head,
// the compare-and-swap revision a widening grant must chain on), and the
// declined choice re-enters through native status. The envelope satisfies
// SDDIntegrationConsentResult.Validate by construction.
func newEditAuthorityConsent(change, workspaceRoot string, missingRoots []string, instance, expectedRevision string) SDDIntegrationConsentResult {
	statusInvocation := fmt.Sprintf("gentle-ai sdd-status %s --cwd %s", change, pathquote.Quote(workspaceRoot))
	evidence := make([]string, 0, len(missingRoots))
	for _, root := range missingRoots {
		evidence = append(evidence, fmt.Sprintf("%s is outside the authorized edit roots", root))
	}
	var grant strings.Builder
	fmt.Fprintf(&grant, "%s--cwd %s --change %s", sddConsentGrantInvocationPrefix, pathquote.Quote(workspaceRoot), change)
	if expectedRevision != "" {
		fmt.Fprintf(&grant, " --expected-revision %s", expectedRevision)
	}
	for _, root := range missingRoots {
		fmt.Fprintf(&grant, " --root %s", pathquote.Quote(root))
	}
	fmt.Fprintf(&grant, " --actor %s --reason %s --request-id %s --change-instance %s",
		sddConsentGrantActor, sddConsentGrantReason,
		sddConsentGrantRequestID(change, instance, expectedRevision, missingRoots), instance)
	return SDDIntegrationConsentResult{
		Schema:       SDDIntegrationConsentSchema,
		Contract:     SDDIntegrationContractV1,
		Operation:    sddConsentOperation,
		Action:       sddConsentActionRequired,
		Blocking:     true,
		Change:       change,
		MissingRoots: append([]string{}, missingRoots...),
		Headline:     "This change plans work outside its authorized edit roots.",
		Reason:       "the task plan targets edit paths that no edit authority covers, so apply stays blocked until a human decides.",
		Value:        "Granting edit authority to this change alone: the grant is recorded in the change's ledger, auditable, and dies with archive.",
		Evidence:     evidence,
		Choices: []consentenvelope.Choice{
			{
				Answer:     sddConsentAnswerGranted,
				Label:      "Grant this change edit authority over the named edit roots",
				Effect:     "This change's apply actor may edit paths under the named edit roots. The grant is per-change, audited (who, when, which roots), and dies with archive; nothing is granted to any other change.",
				Invocation: grant.String(),
			},
			{
				Answer:     sddConsentAnswerDeclined,
				Label:      "Keep the change blocked",
				Effect:     "The change stays blocked(edit_authority_missing) and nothing is persisted. Both exits stay open: edit the change's tasks.md so no work unit targets an unauthorized edit path, or grant edit authority for the named edit roots with the named grant invocation.",
				Invocation: statusInvocation,
			},
		},
		OffPath: consentenvelope.OffPath{
			Note:    fmt.Sprintf("To keep this change inside its authorized edit roots instead, edit its tasks.md so no work unit targets an unauthorized edit path, then re-enter through '%s'.", statusInvocation),
			Command: statusInvocation,
		},
	}
}
