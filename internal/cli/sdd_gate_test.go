package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/handoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
)

// sealForGateTest seals a change with the given extra seal flags (beyond
// --cwd/--change/--execution-style checkpointed/--handoff-policy none),
// failing the test immediately on error. Every gate record scenario in this
// file needs a sealed kickoff first: checkpointed mode is what makes block
// review gates apply at all (REQ-21.7).
func sealForGateTest(t *testing.T, root, change string, extra ...string) {
	t.Helper()
	args := append([]string{"seal", "--cwd", root, "--change", change, "--execution-style", "checkpointed", "--handoff-policy", "none"}, extra...)
	if err := RunSDDKickoff(args, &bytes.Buffer{}); err != nil {
		t.Fatalf("seal de preparacion error = %v", err)
	}
}

// TestRunSDDGateRecordApprovedAppendsRecord is task 10.1's base case: a
// correct approval exits 0 and the record actually lands in gates.yaml.
func TestRunSDDGateRecordApprovedAppendsRecord(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	var out bytes.Buffer
	err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "spec", "--decision", "approved", "--reason", "cubre el caso borde", "--actor", "maintainer",
	}, &out)
	if err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}

	ledger, loadErr := kickoff.LoadGates(changeRoot)
	if loadErr != nil {
		t.Fatalf("LoadGates() error = %v", loadErr)
	}
	if len(ledger.Records) != 1 {
		t.Fatalf("Records = %+v, se esperaba exactamente 1 registro anexado", ledger.Records)
	}
	rec := ledger.Records[0]
	if rec.Gate != kickoff.GateSpec || rec.Decision != kickoff.DecisionApproved || rec.Actor != "maintainer" {
		t.Fatalf("registro anexado = %+v, no coincide con lo solicitado", rec)
	}
}

// TestRunSDDGateRecordFixedGateNeedsNoSealedRoster confirms the four fixed
// gate keys never require a sealed kickoff.yaml: only role-apply:<rol>
// needs a roster to validate membership against.
func TestRunSDDGateRecordFixedGateNeedsNoSealedRoster(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")

	err := RunSDDGate([]string{"record", "--cwd", root, "--change", "inc-99-example", "--gate", "design", "--decision", "approved", "--reason", "ok"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunSDDGate() error = %v, se esperaba exito sin kickoff sellado para una compuerta fija", err)
	}
	ledger, loadErr := kickoff.LoadGates(changeRoot)
	if loadErr != nil {
		t.Fatalf("LoadGates() error = %v", loadErr)
	}
	if len(ledger.Records) != 1 {
		t.Fatalf("Records = %+v, se esperaba 1 registro", ledger.Records)
	}
}

// TestRunSDDGateRecordRejectedWithoutReason is REQ-21.12's hard rule,
// exercised end to end through the CLI (args.go already enforces it; this
// confirms the CLI adapter surfaces that rejection as a failing exit).
func TestRunSDDGateRecordRejectedWithoutReason(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	err := RunSDDGate([]string{"record", "--cwd", root, "--change", "inc-99-example", "--gate", "spec", "--decision", "rejected"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba rechazo: --decision rejected exige --reason")
	}
}

// TestRunSDDGateRecordUnknownGateEnumeratesVocabulary is task 10.1's
// vocabulary case.
func TestRunSDDGateRecordUnknownGateEnumeratesVocabulary(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	err := RunSDDGate([]string{"record", "--cwd", root, "--change", "inc-99-example", "--gate", "bogus", "--decision", "approved"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba rechazo de clave de compuerta desconocida")
	}
	if !strings.Contains(err.Error(), "spec") || !strings.Contains(err.Error(), "role-apply") {
		t.Fatalf("error = %v, se esperaba que enumerase el vocabulario valido", err)
	}
}

// TestRunSDDGateRecordRoleApplyOutsideRosterNamesRoster is task 10.1's
// roster-membership case: role-apply:<rol> for a role outside the sealed
// roster is refused, naming the actual sealed roster.
func TestRunSDDGateRecordRoleApplyOutsideRosterNamesRoster(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example", "--role", "core")

	err := RunSDDGate([]string{"record", "--cwd", root, "--change", "inc-99-example", "--gate", "role-apply:qa", "--decision", "approved", "--reason", "x"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba rechazo: role-apply:qa fuera del roster sellado")
	}
	if !strings.Contains(err.Error(), "core") {
		t.Fatalf("error = %v, se esperaba que nombrase el roster sellado (core)", err)
	}
}

// TestRunSDDGateRecordRoleApplyWithoutSealedKickoffIsRefused covers the
// same rule's degenerate case: no sealed kickoff at all means no roster to
// validate a role-apply gate against.
func TestRunSDDGateRecordRoleApplyWithoutSealedKickoffIsRefused(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")

	err := RunSDDGate([]string{"record", "--cwd", root, "--change", "inc-99-example", "--gate", "role-apply:fullstack", "--decision", "approved", "--reason", "x"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba rechazo: sin kickoff sellado no hay roster")
	}
}

// TestRunSDDGateRecordOnArchivedRootRefused mirrors the kickoff-verb
// archived-root case (shared helper, task 10.4).
func TestRunSDDGateRecordOnArchivedRootRefused(t *testing.T) {
	root := t.TempDir()
	archivedRoot := root + "/openspec/changes/archive/inc-01-old"
	mustMkdirAllCLI(t, archivedRoot)

	err := RunSDDGate([]string{"record", "--cwd", root, "--change", "inc-01-old", "--gate", "spec", "--decision", "approved", "--reason", "x"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba rechazo por raiz archivada (D-14)")
	}
	if !strings.Contains(err.Error(), "REQ-21.18") {
		t.Fatalf("error = %v, se esperaba que nombrase REQ-21.18", err)
	}
}

// TestRunSDDGateUnknownSubcommand and TestRunSDDGateHelp are T-8.
func TestRunSDDGateUnknownSubcommand(t *testing.T) {
	err := RunSDDGate([]string{"bogus"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba rechazo de subcomando desconocido")
	}
}

func TestRunSDDGateNoSubcommand(t *testing.T) {
	if err := RunSDDGate(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("RunSDDGate() = nil error, se esperaba exigir un subcomando")
	}
}

func TestRunSDDGateHelp(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			var out bytes.Buffer
			if err := RunSDDGate([]string{flag}, &out); err != nil {
				t.Fatalf("RunSDDGate(%q) error = %v", flag, err)
			}
			if !strings.Contains(out.String(), "record") || !strings.Contains(out.String(), "show") {
				t.Fatalf("la ayuda no menciona los subcomandos record/show:\n%s", out.String())
			}
		})
	}
}

// TestRunSDDGateShowOnEmptyLedger confirms `gate show` at least succeeds
// and states there is no record yet, on a change with no gates recorded.
func TestRunSDDGateShowOnEmptyLedger(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")

	var out bytes.Buffer
	if err := RunSDDGate([]string{"show", "--cwd", root, "--change", "inc-99-example"}, &out); err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}
	if strings.Contains(out.String(), "spec") {
		t.Fatalf("show reporto un registro inexistente:\n%s", out.String())
	}
}

// TestRunSDDGateRecordEmitsLastRoleNoticeExactlyOnceForSingleFullstackRole
// is task 18.1's base case (routing already confirmed in task 10.1):
// approving the sole fullstack role's role-apply gate emits the last-role
// notice exactly once AND writes handoff.md with status ready (REQ-21.13,
// REQ-21.14).
func TestRunSDDGateRecordEmitsLastRoleNoticeExactlyOnceForSingleFullstackRole(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	var out bytes.Buffer
	err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "role-apply:fullstack", "--decision", "approved", "--reason", "todo listo",
	}, &out)
	if err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}
	occurrences := strings.Count(out.String(), lastRoleNoticeMarker)
	if occurrences != 1 {
		t.Fatalf("el aviso de ultimo rol aparecio %d veces, se esperaba exactamente 1:\n%s", occurrences, out.String())
	}

	h, parseErr := handoff.ParseFile(filepath.Join(changeRoot, "handoff.md"))
	if parseErr != nil {
		t.Fatalf("handoff.ParseFile() error = %v, se esperaba que gate record escribiese handoff.md", parseErr)
	}
	if h.Metadata.Status != handoff.StatusReady {
		t.Fatalf("handoff.md Status = %q, se esperaba ready", h.Metadata.Status)
	}
	if h.Metadata.FromPhase != handoff.PhaseApply || h.Metadata.ToPhase != handoff.PhaseVerify {
		t.Fatalf("handoff.md transicion = %s -> %s, se esperaba apply -> verify", h.Metadata.FromPhase, h.Metadata.ToPhase)
	}
}

// TestRunSDDGateRecordNoNoticeWithRolesStillPending is the mirror case: a
// multi-role roster with only one role approved must never fire the
// notice, and must never write handoff.md either.
func TestRunSDDGateRecordNoNoticeWithRolesStillPending(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example", "--role", "core", "--role", "web")

	var out bytes.Buffer
	err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "role-apply:core", "--decision", "approved", "--reason", "listo",
	}, &out)
	if err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}
	if strings.Contains(out.String(), lastRoleNoticeMarker) {
		t.Fatalf("aviso de ultimo rol emitido con el rol %q aun pendiente:\n%s", "web", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(changeRoot, "handoff.md")); statErr == nil {
		t.Fatal("handoff.md fue escrito con un rol aun pendiente; se esperaba que no existiese")
	}
}

// TestRunSDDGateRecordRejectedRoleApplyNeverEmitsNotice is REQ-21.13's
// third scenario: a rejection never fires the notice, even for the only
// pending role, and never writes handoff.md.
func TestRunSDDGateRecordRejectedRoleApplyNeverEmitsNotice(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	var out bytes.Buffer
	err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "role-apply:fullstack", "--decision", "rejected", "--reason", "no cumple spec",
	}, &out)
	if err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}
	if strings.Contains(out.String(), lastRoleNoticeMarker) {
		t.Fatalf("aviso de ultimo rol emitido pese a un rechazo (REQ-21.13, tercer escenario):\n%s", out.String())
	}
	if _, statErr := os.Stat(filepath.Join(changeRoot, "handoff.md")); statErr == nil {
		t.Fatal("handoff.md fue escrito pese a un rechazo; se esperaba que no existiese")
	}
}

// --- INC-21 Phase 21 (P6c): integration evidence flags wired into `gate
// record` (design.md S4.6, S5.7; D-13). newGovernanceGitWorkspace and
// runGateTestGit exist only for this section: it is the first one in this
// file that needs a real, hermetic local commit graph for the CLI's real
// AncestryChecker (reviewtransaction.SnapshotBuilder.RevisionIsAncestor,
// phase 19) to run against.

// newGovernanceGitWorkspace mirrors newGovernanceWorkspace but additionally
// makes root a real, hermetic local git repository on a "main" branch with
// one commit: t.TempDir() + a local `git init`, never a network operation
// (T-4/T-5 stay N/A for this increment).
func newGovernanceGitWorkspace(t *testing.T, change string) (root, changeRoot string) {
	t.Helper()
	root, changeRoot = newGovernanceWorkspace(t, change)
	runGateTestGit(t, root, "init", "-q", "-b", "main")
	runGateTestGit(t, root, "config", "user.email", "gate-evidence-test@example.com")
	runGateTestGit(t, root, "config", "user.name", "Gate Evidence Test")
	writeCLIFixture(t, filepath.Join(root, "README.md"), "root\n")
	runGateTestGit(t, root, "add", "--", "README.md")
	runGateTestGit(t, root, "commit", "-q", "-m", "root")
	return root, changeRoot
}

func runGateTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

// TestRunSDDGateRecordIntegrationPRMergedConfirmedAncestorRecordsVerifiedTrue
// is task 21.1's first case: a pr_merged commit confirmed as a real local
// ancestor of main records verified: true.
func TestRunSDDGateRecordIntegrationPRMergedConfirmedAncestorRecordsVerifiedTrue(t *testing.T) {
	root, changeRoot := newGovernanceGitWorkspace(t, "inc-99-example")
	commit := strings.TrimSpace(runGateTestGit(t, root, "rev-parse", "HEAD"))

	var out bytes.Buffer
	err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "integration", "--decision", "approved", "--reason", "PR fusionado en main",
		"--evidence-kind", "pr_merged", "--commit", commit,
	}, &out)
	if err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}
	ledger, loadErr := kickoff.LoadGates(changeRoot)
	if loadErr != nil {
		t.Fatalf("kickoff.LoadGates() error = %v", loadErr)
	}
	if len(ledger.Records) != 1 {
		t.Fatalf("gates.yaml records = %d, want exactly 1", len(ledger.Records))
	}
	if rec := ledger.Records[0]; !rec.Verified || rec.EvidenceRef != commit || rec.EvidenceBaseRef != "main" {
		t.Fatalf("record = %#v, want Verified=true, EvidenceRef=%q, EvidenceBaseRef=main", rec, commit)
	}
}

// TestRunSDDGateRecordIntegrationPRMergedWithoutConfirmedAncestorAppendsNothing
// is task 21.1's second case: a pr_merged commit that is NOT a real
// ancestor of main is refused explicitly, naming both revisions compared,
// and nothing is appended to gates.yaml.
func TestRunSDDGateRecordIntegrationPRMergedWithoutConfirmedAncestorAppendsNothing(t *testing.T) {
	root, changeRoot := newGovernanceGitWorkspace(t, "inc-99-example")
	runGateTestGit(t, root, "checkout", "-q", "-b", "feature-x")
	writeCLIFixture(t, filepath.Join(root, "feature.txt"), "feature\n")
	runGateTestGit(t, root, "add", "--", "feature.txt")
	runGateTestGit(t, root, "commit", "-q", "-m", "feature")
	divergent := strings.TrimSpace(runGateTestGit(t, root, "rev-parse", "HEAD"))
	runGateTestGit(t, root, "checkout", "-q", "main")

	var out bytes.Buffer
	err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "integration", "--decision", "approved", "--reason", "PR fusionado en main",
		"--evidence-kind", "pr_merged", "--commit", divergent,
	}, &out)
	if err == nil {
		t.Fatal("RunSDDGate() error = nil, want a rejection naming the unconfirmed commit and branch compared")
	}
	if !strings.Contains(err.Error(), divergent) || !strings.Contains(err.Error(), "main") {
		t.Fatalf("error = %q, want it to name both the commit and the branch compared", err.Error())
	}
	ledger, loadErr := kickoff.LoadGates(changeRoot)
	if loadErr != nil {
		t.Fatalf("kickoff.LoadGates() error = %v", loadErr)
	}
	if len(ledger.Records) != 0 {
		t.Fatalf("gates.yaml records = %d, want 0: an unconfirmed pr_merged ancestor must append nothing", len(ledger.Records))
	}
}

// TestRunSDDGateRecordIntegrationDeploymentAndAttestationRecordVerifiedFalse
// is task 21.1's third case: deployment and attestation evidence, without
// --commit, always exit 0 with verified: false and the actor/reason/
// reference fields all populated — a declaration, not a check (D-13, T-11).
func TestRunSDDGateRecordIntegrationDeploymentAndAttestationRecordVerifiedFalse(t *testing.T) {
	for _, kind := range []string{"deployment", "attestation"} {
		t.Run(kind, func(t *testing.T) {
			root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")

			var out bytes.Buffer
			err := RunSDDGate([]string{
				"record", "--cwd", root, "--change", "inc-99-example",
				"--gate", "integration", "--decision", "approved", "--reason", "desplegado en staging",
				"--evidence-kind", kind, "--evidence", "https://deploys.example/42", "--actor", "release-manager",
			}, &out)
			if err != nil {
				t.Fatalf("RunSDDGate() error = %v", err)
			}
			ledger, loadErr := kickoff.LoadGates(changeRoot)
			if loadErr != nil {
				t.Fatalf("kickoff.LoadGates() error = %v", loadErr)
			}
			if len(ledger.Records) != 1 {
				t.Fatalf("gates.yaml records = %d, want exactly 1", len(ledger.Records))
			}
			rec := ledger.Records[0]
			if rec.Verified {
				t.Fatalf("Verified = true for %q evidence, want false: Axiom cannot prove it happened", kind)
			}
			if rec.Actor != "release-manager" || rec.Reason != "desplegado en staging" || rec.EvidenceRef != "https://deploys.example/42" {
				t.Fatalf("record = %#v, want actor/reason/reference all populated from the CLI flags", rec)
			}
		})
	}
}
