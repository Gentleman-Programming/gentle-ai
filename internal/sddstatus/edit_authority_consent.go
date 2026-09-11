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
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
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
	markerPath := filepath.Join(changeRoot, changeInstanceMarkerFile)
	info, err := os.Lstat(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect change-instance marker: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("change-instance marker must be a regular file") // refusal:by-design world-action: inspect and recover the change-local marker before continuation
	}
	payload, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read change-instance marker: %w", err)
	}
	marker := strings.TrimSpace(string(payload))
	if !validChangeInstanceMarker(marker) {
		return "", fmt.Errorf("persisted change-instance marker is malformed") // refusal:by-design world-action: replace the malformed change-local marker through an authorized recovery path
	}
	return marker, nil
}

func validChangeInstanceMarker(marker string) bool {
	const prefix = "sdd-"
	if !strings.HasPrefix(marker, prefix) || len(marker) != len(prefix)+32 {
		return false
	}
	encoded := strings.TrimPrefix(marker, prefix)
	decoded, err := hex.DecodeString(encoded)
	return err == nil && hex.EncodeToString(decoded) == encoded
}

// Test seam for deterministic publication/readback failures; the shared publisher remains unchanged.
var publishChangeInstanceMarker = reviewtransaction.PublishFileNoReplace

// ensureChangeInstanceMarker reads the existing or no-replace publishes and reads back the winner.
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
	markerPath := filepath.Join(changeRoot, changeInstanceMarkerFile)
	temporary, err := os.CreateTemp(changeRoot, ".gentle-ai-instance-*")
	if err != nil {
		return "", fmt.Errorf("create change-instance marker publication: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(token + "\n"); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write change-instance marker publication: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close change-instance marker publication: %w", err)
	}
	if err := publishChangeInstanceMarker(temporaryPath, markerPath); err != nil {
		winner, readErr := readChangeInstanceMarker(changeRoot)
		if errors.Is(err, os.ErrExist) && readErr == nil && winner != "" {
			return winner, nil
		}
		return "", fmt.Errorf("publish change-instance marker: %w", err)
	}
	winner, err := readChangeInstanceMarker(changeRoot)
	if err != nil {
		return "", err
	}
	if winner == "" {
		return "", fmt.Errorf("published change-instance marker is empty") // refusal:by-design world-action: inspect the change-local marker publication before retrying continuation
	}
	return winner, nil
}

// PrepareChangeInstanceConsent is the sole explicit-continuation marker mutation; it grants no roots.
func PrepareChangeInstanceConsent(status Status) error {
	if len(status.consentPreparationRoots) == 0 || status.ChangeRoot == nil {
		return nil
	}
	changeRoot, err := filepath.EvalSymlinks(*status.ChangeRoot)
	if err != nil {
		return fmt.Errorf("resolve selected change directory: %w", err)
	}
	planning, err := filepath.EvalSymlinks(status.PlanningHome.Path)
	if err != nil {
		return fmt.Errorf("resolve selected planning directory: %w", err)
	}
	workspace, err := filepath.EvalSymlinks(status.ActionContext.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve selected workspace: %w", err)
	}
	if planning != filepath.Join(workspace, "openspec") || status.ChangeName == nil || changeRoot != filepath.Join(planning, "changes", *status.ChangeName) {
		return errors.New("selected change escaped its workspace planning directory") // refusal:by-design human-authority: select the canonical active change inside the authorized workspace
	}
	planningChanges := filepath.Join(planning, "changes")
	relative, err := filepath.Rel(planningChanges, changeRoot)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("change root is outside the selected planning directory") // refusal:by-design human-authority: invoke continue only for the selected OpenSpec change directory
	}
	before, err := os.Stat(changeRoot)
	if err != nil {
		return fmt.Errorf("inspect change directory before marker preparation: %w", err)
	}
	if !before.IsDir() {
		return fmt.Errorf("selected change root is not a directory") // refusal:by-design human-authority: select an active OpenSpec change directory before continuing
	}
	if _, err := ensureChangeInstanceMarker(changeRoot); err != nil {
		return err
	}
	after, err := os.Stat(changeRoot)
	if err != nil {
		return fmt.Errorf("inspect change directory after marker preparation: %w", err)
	}
	if !os.SameFile(before, after) {
		return fmt.Errorf("selected change directory changed during marker preparation") // refusal:by-design world-action: re-read status for the recreated change before preparing consent
	}
	return nil
}

// ForCurrentChangeInstance opts grants into bounded current-marker checks, not filesystem atomicity.
func (store RuntimeStore) ForCurrentChangeInstance(instance string) (RuntimeStore, error) {
	bound, err := store.ForInstance(instance)
	if err != nil {
		return RuntimeStore{}, err
	}
	bound.grantInstanceCheck = func() error { return ValidateCurrentChangeInstance(store.Workspace, store.Change, instance) }
	return bound, bound.grantInstanceCheck()
}

// ValidateCurrentChangeInstance binds a grant to the current persisted marker without preparing one.
func ValidateCurrentChangeInstance(cwd, change, instance string) error {
	status, err := Resolve(ResolveOptions{CWD: cwd, ChangeName: change})
	if err != nil {
		return err
	}
	if status.ChangeRoot == nil {
		return fmt.Errorf("selected change has no OpenSpec marker path") // refusal:by-design human-authority: select an active OpenSpec change before granting edit roots
	}
	current, err := readChangeInstanceMarker(*status.ChangeRoot)
	if err != nil {
		return err
	}
	if current == "" {
		return fmt.Errorf("selected change has no prepared change-instance marker") // refusal:by-design human-authority: run the explicit authorized sdd-continue preparation first
	}
	if current != instance {
		return fmt.Errorf("supplied change-instance marker does not match the current selected change") // refusal:by-design human-authority: rerun status and use the current change's consent invocation
	}
	return nil
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
