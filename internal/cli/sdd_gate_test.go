package cli

import (
	"bytes"
	"strings"
	"testing"

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
// is task 10.1's routing check for REQ-21.13: approving the sole
// fullstack role's role-apply gate emits the last-role notice exactly
// once. This phase only verifies routing — the full handoff body is
// completed in Phase 18.
func TestRunSDDGateRecordEmitsLastRoleNoticeExactlyOnceForSingleFullstackRole(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
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
}

// TestRunSDDGateRecordNoNoticeWithRolesStillPending is the mirror case: a
// multi-role roster with only one role approved must never fire the
// notice.
func TestRunSDDGateRecordNoNoticeWithRolesStillPending(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
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
}

// TestRunSDDGateRecordRejectedRoleApplyNeverEmitsNotice is REQ-21.13's
// third scenario: a rejection never fires the notice, even for the only
// pending role.
func TestRunSDDGateRecordRejectedRoleApplyNeverEmitsNotice(t *testing.T) {
	root, _ := newGovernanceWorkspace(t, "inc-99-example")
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
}
