// Package absorptionledger parses docs/upstream-absorption-ledger.md, the
// durable record of which upstream commits Axiom has absorbed, deliberately
// dropped, or reverted. It is the only piece of the reconciliation protocol
// with knowledge of the ledger's Markdown shape (design.md D-06): every
// absorption batch only reads or appends to the document itself, never
// re-derives its grammar.
//
// Parse never asserts that a row's disposition is actually true — that is
// RA-1/RA-2 evidence, reviewed per absorption batch. It only asserts that
// the document is internally coherent with what it declares about itself:
// every state is one of the three recognized values, every non-absorbed row
// carries a reason, the declared per-state counts match the real rows, no
// sha repeats or falls outside its expected shape, and the total row count
// matches the universe the header declares. A ledger that has not yet
// reached its declared universe is incomplete, not malformed: Parse reports
// that specific gap by wrapping ErrUniverseMismatch on its own, so callers
// can tell "still in progress" apart from "actually broken".
package absorptionledger

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// State is the disposition assigned to one upstream commit in the durable
// absorption ledger. The Go identifier is English, but its constant values
// are the literal Spanish strings the ledger document itself uses and a
// human maintainer reads.
type State string

const (
	// StateAbsorbed marks a commit whose content landed in the fork.
	StateAbsorbed State = "absorbido"
	// StateDeliberatelyDropped marks a commit excluded on purpose, with a
	// written reason (for example, it collides with a deliberate divergence
	// from the V1-V8 non-reversion inventory).
	StateDeliberatelyDropped State = "descartado-deliberadamente"
	// StateReverted marks a commit that was absorbed and later reverted.
	// Reverting never deletes the row; it only moves it to this state.
	StateReverted State = "revertido"
)

// Row is one line of the ledger: a single upstream commit and its
// disposition inside one reconciliation batch (phase).
type Row struct {
	SHA      string
	Subject  string
	Phase    string // F0..F7
	State    State
	Evidence string
	Reason   string // required whenever State != StateAbsorbed
}

// Ledger is the fully parsed durable absorption ledger.
type Ledger struct {
	MeasuredOn     string // AAAA-MM-DD, from the header
	CommonAncestor string
	UpstreamHead   string
	// Universe is the number of no-merge commits the ledger DECLARES it
	// covers, read from its own header. It is intentionally not a package
	// constant: it grows every time upstream advances (design.md D-06,
	// "alternativas descartadas" #6), so a compiled constant would force a
	// Go change on every reconciliation and guarantee the ledger is born
	// out of sync. It is measured with
	// `git rev-list --count --no-merges 266574b0..upstream/main` when the
	// ledger opens; Parse only ever compares real rows against this value.
	Universe int
	// Rows are the real, parsed commit dispositions across every phase
	// section (F0..F7).
	Rows []Row
	// DeclaredCounts is the "## Recuento" table, keyed by state, checked
	// against the real per-state row counts in Rows.
	DeclaredCounts map[State]int
}

// Sentinel errors, one per class of ledger defect, so callers can
// distinguish them with errors.Is. Parse joins every defect it finds with
// errors.Join, so more than one of these can be present in a single
// returned error at once.
var (
	ErrUnknownState     = errors.New("estado de absorción no reconocido")
	ErrMissingReason    = errors.New("un estado distinto de absorbido exige motivo escrito")
	ErrCountMismatch    = errors.New("el recuento declarado no cuadra con las filas")
	ErrUniverseMismatch = errors.New("el número de filas no cubre el universo medido")
	ErrDuplicateSHA     = errors.New("sha repetido o con formato inválido en el registro")
)

var (
	measuredOnPattern   = regexp.MustCompile(`Medido el:\*\*\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	ancestorPattern     = regexp.MustCompile("Ancestro común:\\*\\*\\s*`([0-9a-f]{7,40})`")
	upstreamHeadPattern = regexp.MustCompile("upstream/main`:\\*\\*\\s*`([0-9a-f]{7,40})`")
	universePattern     = regexp.MustCompile(`Universo:\*\*\s*([0-9]+)`)
	phaseHeadingPattern = regexp.MustCompile(`^## (F[0-7])\b`)
	shaFormatPattern    = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// Parse decodes raw as a durable absorption ledger document. It returns the
// parsed Ledger even when it also returns a non-nil error: the error is the
// join of every defect class Parse found, so a caller can distinguish "no
// problems" (err == nil) from "the ledger just hasn't reached its declared
// universe yet" (errors.Is(err, ErrUniverseMismatch) and nothing else) from
// a genuine structural defect (any of the other four sentinels).
func Parse(raw []byte) (*Ledger, error) {
	lines := strings.Split(string(raw), "\n")
	ledger := &Ledger{DeclaredCounts: map[State]int{}}

	if err := parseHeader(lines, ledger); err != nil {
		return ledger, fmt.Errorf("cabecera del registro: %w", err)
	}

	recuentoIdx := indexOfPrefix(lines, "## Recuento")
	if recuentoIdx == -1 {
		return ledger, errors.New("no se encontró la sección \"## Recuento\"")
	}
	countRows, afterCounts := scanTable(lines, recuentoIdx+1)
	for _, cells := range countRows {
		if len(cells) < 2 {
			continue
		}
		label := strings.Trim(cells[0], "*` ")
		if strings.EqualFold(label, "Total") {
			continue
		}
		n, convErr := strconv.Atoi(strings.TrimSpace(cells[1]))
		if convErr != nil {
			continue
		}
		ledger.DeclaredCounts[State(label)] = n
	}

	var errs []error
	actual := map[State]int{}
	seenSHA := map[string]bool{}

	for i := afterCounts; i < len(lines); {
		m := phaseHeadingPattern.FindStringSubmatch(lines[i])
		if m == nil {
			i++
			continue
		}
		phase := m[1]
		rows, next := scanTable(lines, i+1)
		i = next
		for _, cells := range rows {
			if len(cells) < 5 {
				continue
			}
			row := Row{
				SHA:      strings.Trim(cells[0], "` "),
				Subject:  strings.TrimSpace(cells[1]),
				Phase:    phase,
				State:    State(strings.Trim(cells[2], "` ")),
				Evidence: strings.TrimSpace(cells[3]),
				Reason:   strings.TrimSpace(cells[4]),
			}

			switch {
			case !validState(row.State):
				errs = append(errs, fmt.Errorf("fila %q declara un estado desconocido %q: %w", row.SHA, row.State, ErrUnknownState))
			case row.State != StateAbsorbed && isEmptyReason(row.Reason):
				errs = append(errs, fmt.Errorf("fila %q en estado %q no lleva motivo escrito: %w", row.SHA, row.State, ErrMissingReason))
			}

			if row.SHA == "" || seenSHA[row.SHA] || !shaFormatPattern.MatchString(row.SHA) {
				errs = append(errs, fmt.Errorf("sha %q inválido o repetido: %w", row.SHA, ErrDuplicateSHA))
			}
			seenSHA[row.SHA] = true

			actual[row.State]++
			ledger.Rows = append(ledger.Rows, row)
		}
	}

	for _, st := range []State{StateAbsorbed, StateDeliberatelyDropped, StateReverted} {
		if ledger.DeclaredCounts[st] != actual[st] {
			errs = append(errs, fmt.Errorf("el recuento declara %d filas en estado %q pero hay %d filas reales: %w", ledger.DeclaredCounts[st], st, actual[st], ErrCountMismatch))
		}
	}

	if len(ledger.Rows) != ledger.Universe {
		errs = append(errs, fmt.Errorf("el universo declarado es %d y hay %d filas reales: %w", ledger.Universe, len(ledger.Rows), ErrUniverseMismatch))
	}

	return ledger, errors.Join(errs...)
}

// parseHeader fills the provenance fields of ledger from the "> **...**"
// block at the top of the document. It returns an error, never one of the
// five defect sentinels, when a required field is missing: a header that
// cannot be read at all is not a "ledger defect class", it is not a ledger.
func parseHeader(lines []string, ledger *Ledger) error {
	for _, line := range lines {
		if m := measuredOnPattern.FindStringSubmatch(line); m != nil {
			ledger.MeasuredOn = m[1]
		}
		if m := ancestorPattern.FindStringSubmatch(line); m != nil {
			ledger.CommonAncestor = m[1]
		}
		if m := upstreamHeadPattern.FindStringSubmatch(line); m != nil {
			ledger.UpstreamHead = m[1]
		}
		if m := universePattern.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return fmt.Errorf("el universo declarado %q no es un entero: %w", m[1], err)
			}
			ledger.Universe = n
		}
	}
	if ledger.MeasuredOn == "" || ledger.CommonAncestor == "" || ledger.UpstreamHead == "" {
		return errors.New("faltan campos de procedencia (medido el, ancestro común o upstream/main)")
	}
	return nil
}

// scanTable reads consecutive Markdown pipe-table lines starting at index
// start (skipping any leading blank lines, the column-header row, and the
// "|---|---|" separator row) and returns their data-row cells plus the
// index of the first line after the table.
func scanTable(lines []string, start int) (rows [][]string, next int) {
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}

	headerSeen := false
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		if isSeparatorRow(trimmed) {
			i++
			continue
		}
		if !headerSeen {
			headerSeen = true
			i++
			continue
		}
		rows = append(rows, splitTableRow(trimmed))
		i++
	}
	return rows, i
}

// isSeparatorRow reports whether line is a Markdown table separator, such
// as "|---|---|" or "|:--|--:|".
func isSeparatorRow(line string) bool {
	return strings.Trim(line, "|-: ") == ""
}

// splitTableRow splits one Markdown pipe-table line into trimmed cells.
func splitTableRow(line string) []string {
	trimmed := strings.Trim(line, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// validState reports whether s is one of the three recognized dispositions.
func validState(s State) bool {
	switch s {
	case StateAbsorbed, StateDeliberatelyDropped, StateReverted:
		return true
	default:
		return false
	}
}

// isEmptyReason treats both a blank cell and the em-dash placeholder the
// ledger template uses for "not applicable" as an absent reason.
func isEmptyReason(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || s == "—" || s == "-"
}

// indexOfPrefix returns the index of the first line whose trimmed content
// starts with prefix, or -1 if none does.
func indexOfPrefix(lines []string, prefix string) int {
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), prefix) {
			return i
		}
	}
	return -1
}
