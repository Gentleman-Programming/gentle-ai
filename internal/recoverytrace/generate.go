package recoverytrace

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

var ErrRenderFailed = errors.New("ledger document could not be rendered")

// Rendered document names. They are the published filenames under
// docs/audits/data/organic-rdd-recovery/, and each one has a matching schema in
// that directory's schemas/ folder.
const (
	SnapshotLedgerFile     = "snapshot-ledger.json"
	ChangeLedgerFile       = "change-ledger.json"
	InvariantLedgerFile    = "invariant-ledger.json"
	ContributionLedgerFile = "contribution-ledger.json"
	TestLedgerFile         = "test-ledger.json"
	DeletionLedgerFile     = "deletion-ledger.json"
	ReleaseLedgerFile      = "release-ledger.json"
)

// OverlapCounts carries the three reconciliation figures Appendix B does not
// record. They are supplied by the caller rather than derived here, because a
// generator that invents them would only ever agree with itself.
type OverlapCounts struct {
	CollisionPRs   int
	Overlaps       int
	Decompositions int
}

// Generate builds the recovery ledgers from the systemic audit plus the
// branch-artifact rows. It fails closed: an audit that no longer reconciles, or
// a row set that validation rejects, produces no ledger at all rather than a
// partially trustworthy one.
func Generate(source []byte, rows []Row, overlaps OverlapCounts) (Ledgers, error) {
	backlog, err := importBacklog(source)
	if err != nil {
		return Ledgers{}, err
	}
	reconciliation, err := reconcile(backlog, overlaps)
	if err != nil {
		return Ledgers{}, err
	}

	ledgers := Ledgers{
		Reconciliation: reconciliation,
		Backlog:        backlog,
		// The rows are copied so a later caller mutation cannot retroactively
		// change evidence that has already been validated.
		Rows: append(make([]Row, 0, len(rows)), rows...),
	}

	if err := ValidateLedgers(ledgers); err != nil {
		return Ledgers{}, fmt.Errorf("%w: generated ledgers are not releasable evidence", err)
	}
	return ledgers, nil
}

// importBacklog lifts the audit's own items into the ledger. They arrive
// unclassified: a release classification can only be assigned once the released
// SHA exists, and inventing one here would be the generator agreeing with
// itself.
func importBacklog(source []byte) ([]BacklogEntry, error) {
	items, err := ImportAppendixB(source)
	if err != nil {
		return nil, err
	}

	backlog := make([]BacklogEntry, 0, len(items))
	for _, item := range items {
		backlog = append(backlog, BacklogEntry{BacklogItem: item})
	}
	return backlog, nil
}

// reconcile counts the carried backlog instead of restating the frozen totals
// or trusting a declared figure, so a drifting audit is caught by validation
// rather than papered over.
func reconcile(backlog []BacklogEntry, overlaps OverlapCounts) (Reconciliation, error) {
	var issues, pullRequests int
	for _, item := range backlog {
		switch item.Kind {
		case BacklogIssue:
			issues++
		case BacklogPullRequest:
			pullRequests++
		default:
			// Counting an unrecognized kind as either one would let a reshaped
			// import reconcile against the frozen totals it no longer matches.
			return Reconciliation{}, fmt.Errorf("%w: backlog item %d has kind %q",
				ErrMalformedRow, item.Number, item.Kind)
		}
	}

	return Reconciliation{
		Issues:         issues,
		PullRequests:   pullRequests,
		CollisionPRs:   overlaps.CollisionPRs,
		Overlaps:       overlaps.Overlaps,
		Decompositions: overlaps.Decompositions,
	}, nil
}

// Render projects the ledgers into the seven published documents. The bytes are
// canonical — object keys ascending, rows ordered by path, two-space indent, one
// trailing newline — because CI proves the checked-in documents are current by
// comparing bytes, and any incidental reordering would read as a real diff.
func Render(ledgers Ledgers) (map[string][]byte, error) {
	rows := byPath(ledgers.Rows)
	backlog := byKindThenNumber(ledgers.Backlog)

	documents := map[string]any{
		SnapshotLedgerFile: snapshotDocument{
			Backlog:        viewBacklog(backlog),
			Reconciliation: viewReconciliation(ledgers.Reconciliation),
		},
		ChangeLedgerFile:       changeDocument{Changes: viewChanges(rows)},
		InvariantLedgerFile:    invariantDocument{Invariants: viewInvariants(rows)},
		ContributionLedgerFile: contributionDocument{Contributions: viewContributions(rows)},
		TestLedgerFile:         testDocument{Tests: viewTests(rows)},
		DeletionLedgerFile:     deletionDocument{Deletions: viewDeletions(rows)},
		ReleaseLedgerFile: releaseDocument{
			Backlog:  viewReleaseBacklog(backlog),
			Releases: viewReleases(rows),
		},
	}

	rendered := make(map[string][]byte, len(documents))
	for name, document := range documents {
		encoded, err := marshalCanonical(document)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %s", ErrRenderFailed, name, err)
		}
		rendered[name] = encoded
	}
	return rendered, nil
}

func marshalCanonical(document any) ([]byte, error) {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// byPath returns the rows in a stable published order. Validation forbids a
// repeated path, so path alone is a total order; the sort stays stable anyway so
// that an unvalidated ledger still renders the same bytes twice.
func byPath(rows []Row) []Row {
	ordered := append(make([]Row, 0, len(rows)), rows...)
	sort.SliceStable(ordered, func(left, right int) bool {
		return ordered[left].Path < ordered[right].Path
	})
	return ordered
}

// byKindThenNumber returns the backlog in its published order. Kind then number
// is a total order over the items, so the emitted bytes never depend on the
// order the audit happened to list them in.
func byKindThenNumber(backlog []BacklogEntry) []BacklogEntry {
	ordered := append(make([]BacklogEntry, 0, len(backlog)), backlog...)
	sort.SliceStable(ordered, func(left, right int) bool {
		if ordered[left].Kind != ordered[right].Kind {
			return ordered[left].Kind < ordered[right].Kind
		}
		return ordered[left].Number < ordered[right].Number
	})
	return ordered
}

// The view types below exist so every emitted object declares its keys in
// ascending order. The persisted model orders its fields for reading, and the
// two concerns are kept apart rather than reshaping the validated model.

type snapshotDocument struct {
	Backlog        []backlogView      `json:"backlog"`
	Reconciliation reconciliationView `json:"reconciliation"`
}

// backlogView publishes the audit record for one item. The snapshot ledger is
// the view that must show the items themselves, so this document repeats each
// item's disposition and action rather than pointing at a count of them.
type backlogView struct {
	Action      string          `json:"action"`
	Context     SystemicContext `json:"context"`
	Disposition string          `json:"disposition"`
	Kind        BacklogKind     `json:"kind"`
	Number      int             `json:"number"`
}

type reconciliationView struct {
	CollisionPRs   int `json:"collisionPrs"`
	Decompositions int `json:"decompositions"`
	Issues         int `json:"issues"`
	Overlaps       int `json:"overlaps"`
	PullRequests   int `json:"pullRequests"`
}

type changeDocument struct {
	Changes []changeView `json:"changes"`
}

type changeView struct {
	Context     SystemicContext `json:"context,omitempty"`
	Disposition Disposition     `json:"disposition"`
	Path        string          `json:"path"`
}

type invariantDocument struct {
	Invariants []invariantView `json:"invariants"`
}

type invariantView struct {
	Context   SystemicContext `json:"context"`
	Invariant string          `json:"invariant"`
	Path      string          `json:"path"`
	Proof     []string        `json:"proof"`
}

type contributionDocument struct {
	Contributions []contributionView `json:"contributions"`
}

type contributionView struct {
	Contributor string `json:"contributor"`
	Path        string `json:"path"`
}

type testDocument struct {
	Tests []testView `json:"tests"`
}

type testView struct {
	DestinationProof []string `json:"destinationProof,omitempty"`
	Path             string   `json:"path"`
	Proof            []string `json:"proof,omitempty"`
}

type deletionDocument struct {
	Deletions []deletionView `json:"deletions"`
}

type deletionView struct {
	DestinationPath     string            `json:"destinationPath,omitempty"`
	DestinationProof    []string          `json:"destinationProof,omitempty"`
	EarlyDeviation      bool              `json:"earlyDeviation"`
	NoRetainedInvariant bool              `json:"noRetainedInvariant"`
	Path                string            `json:"path"`
	Publication         []publicationView `json:"publication"`
}

type releaseDocument struct {
	Backlog  []releaseBacklogView `json:"backlog"`
	Releases []releaseView        `json:"releases"`
}

// releaseBacklogView classifies one item against the released SHA. Release is
// omitted while the item is unclassified, because an absent classification and
// an empty one would otherwise read as the same fact as a decided outcome.
type releaseBacklogView struct {
	Kind    BacklogKind  `json:"kind"`
	Number  int          `json:"number"`
	Release ReleaseClass `json:"release,omitempty"`
}

type releaseView struct {
	EarlyDeviation bool              `json:"earlyDeviation"`
	Path           string            `json:"path"`
	Publication    []publicationView `json:"publication"`
}

type publicationView struct {
	Present bool   `json:"present"`
	Ref     string `json:"ref"`
}

func viewReconciliation(reconciliation Reconciliation) reconciliationView {
	return reconciliationView{
		CollisionPRs:   reconciliation.CollisionPRs,
		Decompositions: reconciliation.Decompositions,
		Issues:         reconciliation.Issues,
		Overlaps:       reconciliation.Overlaps,
		PullRequests:   reconciliation.PullRequests,
	}
}

func viewBacklog(backlog []BacklogEntry) []backlogView {
	items := make([]backlogView, 0, len(backlog))
	for _, item := range backlog {
		items = append(items, backlogView{
			Action:      item.Action,
			Context:     item.Context,
			Disposition: item.Disposition,
			Kind:        item.Kind,
			Number:      item.Number,
		})
	}
	return items
}

// viewReleaseBacklog lists every item, classified or not. Emitting only the
// classified ones would make an unreviewed backlog indistinguishable from a
// fully reviewed one.
func viewReleaseBacklog(backlog []BacklogEntry) []releaseBacklogView {
	items := make([]releaseBacklogView, 0, len(backlog))
	for _, item := range backlog {
		items = append(items, releaseBacklogView{
			Kind:    item.Kind,
			Number:  item.Number,
			Release: item.Release,
		})
	}
	return items
}

func viewChanges(rows []Row) []changeView {
	changes := make([]changeView, 0, len(rows))
	for _, row := range rows {
		changes = append(changes, changeView{
			Context:     row.Context,
			Disposition: row.Disposition,
			Path:        row.Path,
		})
	}
	return changes
}

// viewInvariants publishes only invariants the recovery still owns. A deleted
// path's transplanted invariant is proven in the deletion ledger by its
// destination, so repeating it here would double-count one obligation.
func viewInvariants(rows []Row) []invariantView {
	invariants := make([]invariantView, 0, len(rows))
	for _, row := range rows {
		if !retainsInvariant(row) {
			continue
		}
		invariants = append(invariants, invariantView{
			Context:   row.Context,
			Invariant: row.Invariant,
			Path:      row.Path,
			Proof:     copyStrings(row.Proof),
		})
	}
	return invariants
}

func viewContributions(rows []Row) []contributionView {
	contributions := make([]contributionView, 0, len(rows))
	for _, row := range rows {
		contributions = append(contributions, contributionView{
			Contributor: row.Contributor,
			Path:        row.Path,
		})
	}
	return contributions
}

// viewTests keeps retained and transplanted proof in separate fields. Merging
// them would hide whether a failure class is still proven where it lived or only
// where it moved.
func viewTests(rows []Row) []testView {
	tests := make([]testView, 0, len(rows))
	for _, row := range rows {
		if len(row.Proof) == 0 && len(row.DestinationProof) == 0 {
			continue
		}
		tests = append(tests, testView{
			DestinationProof: copyStrings(row.DestinationProof),
			Path:             row.Path,
			Proof:            copyStrings(row.Proof),
		})
	}
	return tests
}

func viewDeletions(rows []Row) []deletionView {
	deletions := make([]deletionView, 0, len(rows))
	for _, row := range rows {
		if row.Disposition != DispositionDelete {
			continue
		}
		deletions = append(deletions, deletionView{
			DestinationPath:     row.DestinationPath,
			DestinationProof:    copyStrings(row.DestinationProof),
			EarlyDeviation:      row.EarlyDeviation,
			NoRetainedInvariant: row.NoRetainedInvariant,
			Path:                row.Path,
			Publication:         viewPublication(row.Publication),
		})
	}
	return deletions
}

// viewReleases records the publication classification for every path that was
// actually checked against the authoritative refs. A path with no recorded
// evidence is absent rather than reported as unpublished.
func viewReleases(rows []Row) []releaseView {
	releases := make([]releaseView, 0, len(rows))
	for _, row := range rows {
		if len(row.Publication) == 0 {
			continue
		}
		releases = append(releases, releaseView{
			EarlyDeviation: row.EarlyDeviation,
			Path:           row.Path,
			Publication:    viewPublication(row.Publication),
		})
	}
	return releases
}

// viewPublication preserves the recorded ref order. The refs are evidence in the
// order they were queried, so sorting them would rewrite the record.
func viewPublication(publication []PublicationRef) []publicationView {
	refs := make([]publicationView, 0, len(publication))
	for _, ref := range publication {
		refs = append(refs, publicationView{Present: ref.Present, Ref: ref.Ref})
	}
	return refs
}

// copyStrings returns nil for an empty input so an omitempty field stays absent
// rather than rendering as an empty array that reads like checked-and-none.
func copyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append(make([]string, 0, len(values)), values...)
}
