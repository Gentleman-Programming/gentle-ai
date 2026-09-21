package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/sddstatus"
)

// TestRunSDDGateRecordRejectionPersistsAcrossStatusReads reproduces the
// user-visible defect confirmed live against the compiled binary
// (apply-progress.md, Fases 11-14, Issues Found #8): `axiom sdd gate
// record --decision rejected` on a fixed gate judged against a REAL,
// non-empty artifact must survive the very next `sdd status` read as
// "rejected", carrying its exact reason — not silently reopen to
// "pending" the moment status is re-resolved.
//
// Root cause (design.md D-08, machine.go's resolveGateStatus): a rejection
// only stays rejected while its recorded ArtifactDigest still matches the
// artifact's CURRENT digest. runSDDGateRecord never computed that digest at
// all, so every recorded GateRecord carried ArtifactDigest == "" while
// sddstatus.loadGovernance always recomputes a real, non-empty digest for
// an artifact that actually has content — the two can never match, so
// resolveGateStatus's "digest unchanged" branch is never taken and the
// gate is reported "reopened" on every subsequent read, even though
// nothing was ever remediated.
func TestRunSDDGateRecordRejectionPersistsAcrossStatusReads(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	// A real, non-empty design.md: the digest recomputed at read time must
	// be non-empty for this defect to reproduce (an absent or empty
	// artifact already digests to "", which the pre-fix writer's empty
	// ArtifactDigest would spuriously "match").
	if err := os.WriteFile(filepath.Join(changeRoot, "design.md"), []byte("# Diseno\n\nContenido real.\n"), 0644); err != nil {
		t.Fatalf("preparar design.md: %v", err)
	}

	const rejectionReason = "falta cubrir un caso limite"
	var out bytes.Buffer
	if err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "design", "--decision", "rejected", "--reason", rejectionReason, "--actor", "reviewer",
	}, &out); err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{CWD: root, ChangeName: "inc-99-example"})
	if err != nil {
		t.Fatalf("sddstatus.Resolve() error = %v", err)
	}
	if status.Governance == nil {
		t.Fatal("status.Governance = nil, se esperaba gobernanza proyectada (kickoff sellado en checkpointed)")
	}

	found := false
	for _, gate := range status.Governance.Gates {
		if string(gate.Key) != "design" {
			continue
		}
		found = true
		if gate.Status != "rejected" {
			t.Fatalf(`gate "design".Status = %q, se esperaba "rejected" (el rechazo no debe reabrirse solo porque se releyo el estado)`, gate.Status)
		}
		if gate.Reason != rejectionReason {
			t.Fatalf("gate \"design\".Reason = %q, se esperaba %q", gate.Reason, rejectionReason)
		}
		if gate.Reopened {
			t.Fatal(`gate "design".Reopened = true, se esperaba false: el artefacto no cambio desde el rechazo`)
		}
	}
	if !found {
		t.Fatal(`no se encontro la compuerta "design" en status.Governance.Gates`)
	}

	if status.NextRecommended != "design" {
		t.Fatalf("NextRecommended = %q, se esperaba \"design\" (la fase duena del rechazo, D-09) en vez de await-gate", status.NextRecommended)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), rejectionReason) {
		t.Fatalf("BlockedReasons = %v, se esperaba el motivo exacto del rechazo", status.BlockedReasons)
	}
}

// TestRunSDDGateRecordRejectionReopensAfterRealRemediation triangulates the
// fix from the opposite direction: D-08's whole point is that a rejection
// DOES reopen once the artifact genuinely changes. Fixing the digest gap
// must not turn every rejection into a permanent one — remediation must
// still work exactly as machine.go already proves it does at the unit
// level.
func TestRunSDDGateRecordRejectionReopensAfterRealRemediation(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	designPath := filepath.Join(changeRoot, "design.md")
	if err := os.WriteFile(designPath, []byte("# Diseno\n\nVersion original.\n"), 0644); err != nil {
		t.Fatalf("preparar design.md: %v", err)
	}
	if err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "design", "--decision", "rejected", "--reason", "falta X", "--actor", "reviewer",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}

	// Remediate: the artifact's content genuinely changes.
	if err := os.WriteFile(designPath, []byte("# Diseno\n\nVersion remediada con X cubierto.\n"), 0644); err != nil {
		t.Fatalf("remediar design.md: %v", err)
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{CWD: root, ChangeName: "inc-99-example"})
	if err != nil {
		t.Fatalf("sddstatus.Resolve() error = %v", err)
	}
	for _, gate := range status.Governance.Gates {
		if string(gate.Key) != "design" {
			continue
		}
		if gate.Status != "pending" || !gate.Reopened {
			t.Fatalf(`gate "design" = {Status:%q Reopened:%v}, se esperaba pending+Reopened tras remediar de verdad`, gate.Status, gate.Reopened)
		}
	}
}

// TestRunSDDGateRecordRoleApplyRejectionPersistsAcrossStatusReads
// triangulates the fix for a role-apply:<rol> gate, not only the four
// fixed gates: the same digest gap applied there too
// (roleApplyArtifactDigestForRecord), judged against the role's own
// sealed TasksFile.
func TestRunSDDGateRecordRoleApplyRejectionPersistsAcrossStatusReads(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	if err := os.WriteFile(filepath.Join(changeRoot, "tasks.md"), []byte("- [x] 1.1 hecho\n"), 0644); err != nil {
		t.Fatalf("preparar tasks.md: %v", err)
	}

	const rejectionReason = "no cumple la spec asignada"
	if err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "role-apply:fullstack", "--decision", "rejected", "--reason", rejectionReason, "--actor", "reviewer",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{CWD: root, ChangeName: "inc-99-example"})
	if err != nil {
		t.Fatalf("sddstatus.Resolve() error = %v", err)
	}
	found := false
	for _, gate := range status.Governance.Gates {
		if string(gate.Key) != "role-apply:fullstack" {
			continue
		}
		found = true
		if gate.Status != "rejected" || gate.Reason != rejectionReason {
			t.Fatalf(`gate "role-apply:fullstack" = {Status:%q Reason:%q}, se esperaba rejected/%q sin remediacion`, gate.Status, gate.Reason, rejectionReason)
		}
	}
	if !found {
		t.Fatal(`no se encontro la compuerta "role-apply:fullstack" en status.Governance.Gates`)
	}
}

// TestRunSDDGateRecordSpecGateDigestCoversMultiFileSpecs confirms the fix
// resolves the SAME multi-file "spec" artifact set
// internal/sddstatus/status.go's findSpecFiles does (every
// specs/**/spec.md, sorted), not only a flat spec.md — a rejection
// judged against a real per-capability spec file must persist too.
func TestRunSDDGateRecordSpecGateDigestCoversMultiFileSpecs(t *testing.T) {
	root, changeRoot := newGovernanceWorkspace(t, "inc-99-example")
	sealForGateTest(t, root, "inc-99-example")

	specPath := filepath.Join(changeRoot, "specs", "auth", "spec.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0755); err != nil {
		t.Fatalf("preparar specs/auth: %v", err)
	}
	if err := os.WriteFile(specPath, []byte("### Requirement: Auth\n#### Scenario: x\n"), 0644); err != nil {
		t.Fatalf("preparar specs/auth/spec.md: %v", err)
	}

	const rejectionReason = "falta cubrir sesion expirada"
	if err := RunSDDGate([]string{
		"record", "--cwd", root, "--change", "inc-99-example",
		"--gate", "spec", "--decision", "rejected", "--reason", rejectionReason, "--actor", "reviewer",
	}, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunSDDGate() error = %v", err)
	}

	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{CWD: root, ChangeName: "inc-99-example"})
	if err != nil {
		t.Fatalf("sddstatus.Resolve() error = %v", err)
	}
	found := false
	for _, gate := range status.Governance.Gates {
		if string(gate.Key) != "spec" {
			continue
		}
		found = true
		if gate.Status != "rejected" || gate.Reason != rejectionReason {
			t.Fatalf(`gate "spec" = {Status:%q Reason:%q}, se esperaba rejected/%q sin remediacion`, gate.Status, gate.Reason, rejectionReason)
		}
	}
	if !found {
		t.Fatal(`no se encontro la compuerta "spec" en status.Governance.Gates`)
	}
}
