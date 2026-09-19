package absorptionledger

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

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
