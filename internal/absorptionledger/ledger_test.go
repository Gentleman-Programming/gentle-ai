package absorptionledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readRealLedger reads the actual docs/upstream-absorption-ledger.md the
// repository ships, as opposed to a synthetic fixture.
func readRealLedger(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "upstream-absorption-ledger.md"))
	if err != nil {
		t.Fatalf("no se pudo leer docs/upstream-absorption-ledger.md: %v", err)
	}
	return raw
}

// fixtureRow is the raw, string-typed shape of one ledger row as it would
// appear in Markdown, deliberately untyped against State so a test case can
// author an invalid value (e.g. an unrecognized state) on purpose.
type fixtureRow struct {
	sha, subject, phase, state, evidence, reason string
}

// buildLedgerDoc renders a minimal, syntactically valid ledger document: the
// same shape Parse must understand, with the caller controlling exactly
// what would make it valid or invalid. It always emits the header, the
// "Reglas de aceptación" section and a "Recuento" table, then one phase
// section per distinct fixtureRow.phase (defaulting to a single empty "F0"
// section when rows is empty).
func buildLedgerDoc(universe int, declared map[string]int, rows []fixtureRow) string {
	var b strings.Builder
	b.WriteString("# Registro de absorción upstream — Axiom (fixture)\n\n")
	b.WriteString("> **Medido el:** 2026-09-19 · **Ancestro común:** `266574b0` · **`upstream/main`:** `82a6de96`\n")
	fmt.Fprintf(&b, "> **Universo:** %d commits de `266574b0..upstream/main` **sin merges**\n", universe)
	b.WriteString("> **Comando:** `git rev-list --count --no-merges 266574b0..upstream/main`\n\n")
	b.WriteString("## Reglas de aceptación\n\n1. Fixture de prueba, sin reglas reales.\n\n")
	b.WriteString("## Recuento\n\n| Estado | Filas |\n|---|---|\n")
	fmt.Fprintf(&b, "| `absorbido` | %d |\n", declared["absorbido"])
	fmt.Fprintf(&b, "| `descartado-deliberadamente` | %d |\n", declared["descartado-deliberadamente"])
	fmt.Fprintf(&b, "| `revertido` | %d |\n", declared["revertido"])
	b.WriteString("| **Total** | **-** |\n\n")

	byPhase := map[string][]fixtureRow{}
	var order []string
	for _, r := range rows {
		if _, ok := byPhase[r.phase]; !ok {
			order = append(order, r.phase)
		}
		byPhase[r.phase] = append(byPhase[r.phase], r)
	}
	if len(order) == 0 {
		order = []string{"F0"}
	}
	for _, phase := range order {
		fmt.Fprintf(&b, "## %s — Fixture\n\n", phase)
		b.WriteString("| `sha` | Asunto | Estado | Evidencia | Motivo (si no es `absorbido`) |\n|---|---|---|---|---|\n")
		for _, r := range byPhase[phase] {
			reason := r.reason
			if reason == "" {
				reason = "—"
			}
			fmt.Fprintf(&b, "| `%s` | %s | `%s` | %s | %s |\n", r.sha, r.subject, r.state, r.evidence, reason)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestParseLedgerFixtures(t *testing.T) {
	tests := []struct {
		name     string
		universe int
		declared map[string]int
		rows     []fixtureRow
		wantErr  error // sentinel checked with errors.Is; nil means Parse must succeed
	}{
		{
			name:     "estado desconocido",
			universe: 1,
			declared: map[string]int{"absorbido": 1},
			rows: []fixtureRow{
				{sha: "1234567", subject: "algo absorbido a medias", phase: "F0", state: "en-progreso", evidence: "PR #1", reason: "—"},
			},
			wantErr: ErrUnknownState,
		},
		{
			name:     "motivo vacío en estado distinto de absorbido",
			universe: 1,
			declared: map[string]int{"descartado-deliberadamente": 1},
			rows: []fixtureRow{
				{sha: "1234567", subject: "colisiona con una divergencia firme", phase: "F0", state: "descartado-deliberadamente", evidence: "-", reason: ""},
			},
			wantErr: ErrMissingReason,
		},
		{
			name:     "tabla de recuento que no cuadra con las filas reales",
			universe: 1,
			declared: map[string]int{"absorbido": 5},
			rows: []fixtureRow{
				{sha: "1234567", subject: "un solo commit absorbido", phase: "F0", state: "absorbido", evidence: "PR #1", reason: "—"},
			},
			wantErr: ErrCountMismatch,
		},
		{
			name:     "total de filas menor que el universo declarado",
			universe: 5,
			declared: map[string]int{"absorbido": 1},
			rows: []fixtureRow{
				{sha: "1234567", subject: "un solo commit absorbido", phase: "F0", state: "absorbido", evidence: "PR #1", reason: "—"},
			},
			wantErr: ErrUniverseMismatch,
		},
		{
			name:     "sha repetido",
			universe: 2,
			declared: map[string]int{"absorbido": 2},
			rows: []fixtureRow{
				{sha: "1234567", subject: "primera vez", phase: "F0", state: "absorbido", evidence: "PR #1", reason: "—"},
				{sha: "1234567", subject: "repetido a propósito", phase: "F0", state: "absorbido", evidence: "PR #2", reason: "—"},
			},
			wantErr: ErrDuplicateSHA,
		},
		{
			name:     "fixture válida y completa",
			universe: 2,
			declared: map[string]int{"absorbido": 1, "descartado-deliberadamente": 1},
			rows: []fixtureRow{
				{sha: "1234567", subject: "un commit absorbido", phase: "F0", state: "absorbido", evidence: "PR #1", reason: "—"},
				{sha: "89abcde", subject: "un commit descartado", phase: "F0", state: "descartado-deliberadamente", evidence: "-", reason: "colisiona con V2 del inventario de no-reversión"},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := buildLedgerDoc(tt.universe, tt.declared, tt.rows)
			ledger, err := Parse([]byte(doc))

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Parse() error inesperado = %v", err)
				}
				if ledger.Universe != tt.universe {
					t.Fatalf("Universe = %d, se esperaba %d", ledger.Universe, tt.universe)
				}
				if len(ledger.Rows) != len(tt.rows) {
					t.Fatalf("len(Rows) = %d, se esperaban %d", len(ledger.Rows), len(tt.rows))
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Parse() error = %v, se esperaba que envolviera %v", err, tt.wantErr)
			}
		})
	}
}

// TestUpstreamAbsorptionLedgerRealDocumentIsInternallyCoherent parses the
// real, still-incomplete ledger and confirms rules 2-6 of D-06: every state
// is recognized, every non-absorbed row carries a reason, the declared
// per-state counts match the real rows, no phase-section heading repeats or
// falls outside F0..F7, and no sha repeats. Rule 1 (the ledger covers its
// declared universe) is deliberately NOT asserted here — it is the one
// known-incomplete condition this test tolerates, verified on its own by
// TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose. This test must
// already pass against the empty skeleton docs/upstream-absorption-ledger.md
// ships with: 0 declared rows match 0 real rows for every state.
func TestUpstreamAbsorptionLedgerRealDocumentIsInternallyCoherent(t *testing.T) {
	raw := readRealLedger(t)
	ledger, err := Parse(raw)
	if ledger == nil {
		t.Fatalf("Parse() devolvió un Ledger nulo")
	}

	for _, sentinel := range []error{ErrUnknownState, ErrMissingReason, ErrCountMismatch, ErrDuplicateSHA} {
		if errors.Is(err, sentinel) {
			t.Fatalf("el registro real viola una regla de coherencia inesperada (%v); error completo: %v", sentinel, err)
		}
	}
	if err != nil && !errors.Is(err, ErrUniverseMismatch) {
		t.Fatalf("error inesperado más allá de la incompletitud del universo: %v", err)
	}

	// D-06 regla 5: ninguna cabecera de sección de tanda se repite y todas
	// pertenecen a F0..F7. Parse no lo afirma con un centinela propio (el
	// esbozo del diseño no define uno para esta regla); se comprueba aquí
	// directamente sobre el texto, reutilizando el mismo patrón que Parse.
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		m := phaseHeadingPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if seen[m[1]] {
			t.Fatalf("cabecera de sección de tanda repetida: %q", m[1])
		}
		seen[m[1]] = true
	}
	if len(seen) != 8 {
		t.Fatalf("se esperaban 8 secciones de tanda (F0..F7), se encontraron %d", len(seen))
	}
}

// TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose confirms D-06
// regla 1: el total de filas reales del registro es exactamente el universo
// declarado en su cabecera. Es el ÚNICO fallo aceptado en `go test ./...`
// desde la Fase 5 hasta el cierre de la Fase 16 (regla 3 de "Reglas de
// Comprobación y Alcance" de tasks.md); la Fase 16 retira el t.Skipf de
// abajo como su primer paso.
func TestUpstreamAbsorptionLedgerCoversDeclaredUniverseAtClose(t *testing.T) {
	t.Skipf("Fase 16 (F7) retira este Skip al cerrar el registro: 0 de 91 filas reales hoy frente al universo declarado en la cabecera de docs/upstream-absorption-ledger.md (medido 2026-09-19, 266574b0..82a6de96 sin merges). Es el único fallo aceptado en `go test ./...` hasta entonces.")

	raw := readRealLedger(t)
	ledger, err := Parse(raw)
	if err != nil && !errors.Is(err, ErrUniverseMismatch) {
		t.Fatalf("error inesperado al parsear el registro real: %v", err)
	}
	if ledger == nil {
		t.Fatalf("Parse() devolvió un Ledger nulo")
	}
	if len(ledger.Rows) != ledger.Universe {
		t.Fatalf("el registro declara un universo de %d commits pero solo tiene %d filas reales", ledger.Universe, len(ledger.Rows))
	}
}
