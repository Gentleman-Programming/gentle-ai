package recoverytrace

import (
	"errors"
	"testing"
)

// presentEverywhere is the publication evidence for a path that already exists
// on every authoritative ref, so it may never take the early-removal deviation.
func presentEverywhere() []PublicationRef {
	return []PublicationRef{
		{Ref: "origin/main", Present: true},
		{Ref: "main", Present: true},
		{Ref: "v2.1.11", Present: true},
	}
}

// absentEverywhere is the publication evidence that authorizes early removal.
func absentEverywhere() []PublicationRef {
	return []PublicationRef{
		{Ref: "origin/main", Present: false},
		{Ref: "main", Present: false},
		{Ref: "v2.1.11", Present: false},
	}
}

// reconciledBacklog rebuilds a backlog that matches the frozen totals item by
// item. The items are synthesized instead of imported because these cases
// exercise the count-versus-item cross-check, not the importer.
func reconciledBacklog() []BacklogEntry {
	backlog := make([]BacklogEntry, 0, expectedIssues+expectedPullRequests)
	for number := 1; number <= expectedIssues; number++ {
		backlog = append(backlog, BacklogEntry{BacklogItem: BacklogItem{
			Number:      number,
			Kind:        BacklogIssue,
			Context:     ContextRAR,
			Disposition: "KEEP",
			Action:      "retain",
		}})
	}
	for number := 1; number <= expectedPullRequests; number++ {
		backlog = append(backlog, BacklogEntry{BacklogItem: BacklogItem{
			Number:      number,
			Kind:        BacklogPullRequest,
			Context:     ContextPAD,
			Disposition: "TRANSPLANT",
			Action:      "decompose",
		}})
	}
	return backlog
}

// validLedgers returns the smallest ledger set that satisfies every recovery
// requirement. Each case below mutates exactly one field, so a rejection names
// exactly one cause.
func validLedgers() Ledgers {
	return Ledgers{
		Reconciliation: Reconciliation{
			Issues:         241,
			PullRequests:   92,
			CollisionPRs:   74,
			Overlaps:       499,
			Decompositions: 16,
		},
		Backlog: reconciledBacklog(),
		Rows: []Row{
			{
				Path:        "internal/reviewtransaction/rar_path_safety.go",
				Disposition: DispositionKeep,
				Context:     ContextRAR,
				Invariant:   "candidate path containment",
				Proof:       []string{"internal/reviewtransaction/rar_path_safety_windows_test.go"},
				Contributor: "@Alan-TheGentleman",
				Publication: presentEverywhere(),
			},
			{
				Path:        "internal/workrun/verification_consent.go",
				Disposition: DispositionTransplant,
				Context:     ContextRAR,
				Invariant:   "frozen verification consent replay",
				Proof:       []string{"internal/reviewtransaction/verification_consent_test.go"},
				Contributor: "@Alan-TheGentleman",
				Publication: absentEverywhere(),
			},
			{
				Path:             "internal/workprovider/runtime_connector.go",
				Disposition:      DispositionDelete,
				Context:          ContextRAR,
				Invariant:        "authenticated connector transport",
				Contributor:      "@Alan-TheGentleman",
				Publication:      absentEverywhere(),
				EarlyDeviation:   true,
				DestinationPath:  "internal/reviewtransaction/verification_consent.go",
				DestinationProof: []string{"internal/reviewtransaction/verification_consent_test.go"},
			},
			{
				Path:                "contracts/work-routing/v1/schemas/work-capabilities-v2.schema.json",
				Disposition:         DispositionDelete,
				Context:             ContextEPD,
				Contributor:         "@Alan-TheGentleman",
				Publication:         absentEverywhere(),
				EarlyDeviation:      true,
				NoRetainedInvariant: true,
			},
		},
	}
}

func TestValidateLedgersAcceptsProvenRecovery(t *testing.T) {
	t.Parallel()

	if err := ValidateLedgers(validLedgers()); err != nil {
		t.Fatalf("ValidateLedgers() error = %v, want nil", err)
	}
}

func TestValidateLedgersRejectsUnprovenRecovery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*Ledgers)
		wantErr error
	}{
		{
			name: "deletion without destination or no-invariant proof",
			mutate: func(ledgers *Ledgers) {
				row := &ledgers.Rows[2]
				row.DestinationPath = ""
				row.DestinationProof = nil
			},
			wantErr: ErrUnprovenDeletion,
		},
		{
			name: "deletion naming a destination but no test proving it",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[2].DestinationProof = nil
			},
			wantErr: ErrUnprovenDeletion,
		},
		{
			name: "retained invariant without an owning context",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[0].Context = ""
			},
			wantErr: ErrOrphanedInvariant,
		},
		{
			name: "retained invariant without an exact proof reference",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[0].Proof = nil
			},
			wantErr: ErrOrphanedInvariant,
		},
		{
			name: "transplanted invariant without destination proof",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[1].Proof = nil
			},
			wantErr: ErrOrphanedInvariant,
		},
		{
			name: "row without contributor credit",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[0].Contributor = ""
			},
			wantErr: ErrMissingCredit,
		},
		{
			name: "issue count does not reconcile",
			mutate: func(ledgers *Ledgers) {
				ledgers.Reconciliation.Issues = 240
			},
			wantErr: ErrCountMismatch,
		},
		{
			name: "pull request count does not reconcile",
			mutate: func(ledgers *Ledgers) {
				ledgers.Reconciliation.PullRequests = 91
			},
			wantErr: ErrCountMismatch,
		},
		{
			name: "collision component does not reconcile",
			mutate: func(ledgers *Ledgers) {
				ledgers.Reconciliation.CollisionPRs = 73
			},
			wantErr: ErrCountMismatch,
		},
		{
			name: "overlap count does not reconcile",
			mutate: func(ledgers *Ledgers) {
				ledgers.Reconciliation.Overlaps = 498
			},
			wantErr: ErrCountMismatch,
		},
		{
			name: "decomposition count does not reconcile",
			mutate: func(ledgers *Ledgers) {
				ledgers.Reconciliation.Decompositions = 15
			},
			wantErr: ErrCountMismatch,
		},
		{
			name: "early deviation for a path present on origin/main",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[2].Publication = presentEverywhere()
			},
			wantErr: ErrUnauthorizedDeviation,
		},
		{
			name: "early deviation for a path present on only one ref",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[2].Publication = []PublicationRef{
					{Ref: "origin/main", Present: false},
					{Ref: "main", Present: false},
					{Ref: "v2.1.11", Present: true},
				}
			},
			wantErr: ErrUnauthorizedDeviation,
		},
		{
			name: "early deviation without recorded publication evidence",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[2].Publication = nil
			},
			wantErr: ErrUnauthorizedDeviation,
		},
		{
			name: "publication evidence omits an authoritative ref",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[2].Publication = []PublicationRef{
					{Ref: "origin/main", Present: false},
				}
			},
			wantErr: ErrUnauthorizedDeviation,
		},
		{
			name: "unknown disposition",
			mutate: func(ledgers *Ledgers) {
				ledgers.Rows[0].Disposition = Disposition("ARCHIVE")
			},
			wantErr: ErrUnknownDisposition,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ledgers := validLedgers()
			testCase.mutate(&ledgers)

			err := ValidateLedgers(ledgers)
			if err == nil {
				t.Fatalf("ValidateLedgers() error = nil, want %v", testCase.wantErr)
			}
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ValidateLedgers() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestValidateLedgersAllowsDeletionWithoutRetainedInvariant(t *testing.T) {
	t.Parallel()

	ledgers := validLedgers()
	// The connector row keeps its retained invariant but drops the transplant,
	// declaring instead that nothing retained lives there. That declaration must
	// be explicit, never inferred from a missing destination.
	row := &ledgers.Rows[2]
	row.DestinationPath = ""
	row.DestinationProof = nil
	row.Invariant = ""
	row.NoRetainedInvariant = true

	if err := ValidateLedgers(ledgers); err != nil {
		t.Fatalf("ValidateLedgers() error = %v, want nil", err)
	}
}

func TestValidateLedgersRejectsDuplicatePathDisposition(t *testing.T) {
	t.Parallel()

	// The ledger records exactly one disposition per item. Two rows for the same
	// path let a contradictory pair authorize a deletion that no single row could.
	ledgers := validLedgers()
	duplicate := ledgers.Rows[0]
	duplicate.Disposition = DispositionDelete
	duplicate.Invariant = ""
	duplicate.NoRetainedInvariant = true
	ledgers.Rows = append(ledgers.Rows, duplicate)

	if err := ValidateLedgers(ledgers); !errors.Is(err, ErrDuplicatePath) {
		t.Fatalf("ValidateLedgers() error = %v, want %v", err, ErrDuplicatePath)
	}
}

func TestValidateLedgersRejectsNonTagReleaseEvidence(t *testing.T) {
	t.Parallel()

	// Early removal is authorized by absence from both branches and a release tag.
	// Accepting any third ref would let a branch or a typo stand in for the tag,
	// which is the one piece of evidence proving the path was never shipped.
	ledgers := validLedgers()
	ledgers.Rows[2].Publication = []PublicationRef{
		{Ref: "origin/main", Present: false},
		{Ref: "main", Present: false},
		{Ref: "origin/feat/some-branch", Present: false},
	}

	if err := ValidateLedgers(ledgers); !errors.Is(err, ErrUnauthorizedDeviation) {
		t.Fatalf("ValidateLedgers() error = %v, want %v", err, ErrUnauthorizedDeviation)
	}
}

func TestValidateLedgersAcceptsQualifiedReleaseTagEvidence(t *testing.T) {
	t.Parallel()

	ledgers := validLedgers()
	ledgers.Rows[2].Publication = []PublicationRef{
		{Ref: "origin/main", Present: false},
		{Ref: "main", Present: false},
		{Ref: "refs/tags/v2.1.11", Present: false},
	}

	if err := ValidateLedgers(ledgers); err != nil {
		t.Fatalf("ValidateLedgers() error = %v, want nil", err)
	}
}

func TestValidateLedgersRejectsBacklogThatDisagreesWithDeclaredCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*Ledgers)
	}{
		{
			name: "one issue short of the declared count",
			mutate: func(ledgers *Ledgers) {
				ledgers.Backlog = ledgers.Backlog[1:]
			},
		},
		{
			name: "one pull request short of the declared count",
			mutate: func(ledgers *Ledgers) {
				ledgers.Backlog = ledgers.Backlog[:len(ledgers.Backlog)-1]
			},
		},
		{
			name: "one item repeated in place of a distinct one",
			mutate: func(ledgers *Ledgers) {
				ledgers.Backlog[1] = ledgers.Backlog[0]
			},
		},
		{
			name: "an item of neither kind counts toward neither total",
			mutate: func(ledgers *Ledgers) {
				ledgers.Backlog[0].Kind = BacklogKind("discussion")
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ledgers := validLedgers()
			testCase.mutate(&ledgers)

			if err := ValidateLedgers(ledgers); !errors.Is(err, ErrBacklogMismatch) {
				t.Fatalf("ValidateLedgers() error = %v, want %v", err, ErrBacklogMismatch)
			}
		})
	}
}

func TestValidateLedgersRejectsEmptyBacklogBehindNonZeroCounts(t *testing.T) {
	t.Parallel()

	// Counts alone once satisfied the validator with no items behind them. An
	// empty backlog under the frozen totals is exactly that failure.
	ledgers := validLedgers()
	ledgers.Backlog = nil

	if err := ValidateLedgers(ledgers); !errors.Is(err, ErrBacklogMismatch) {
		t.Fatalf("ValidateLedgers() error = %v, want %v", err, ErrBacklogMismatch)
	}
}

func TestValidateLedgersRejectsUnknownReleaseClassification(t *testing.T) {
	t.Parallel()

	ledgers := validLedgers()
	ledgers.Backlog[0].Release = ReleaseClass("wontfix")

	if err := ValidateLedgers(ledgers); !errors.Is(err, ErrUnknownReleaseClass) {
		t.Fatalf("ValidateLedgers() error = %v, want %v", err, ErrUnknownReleaseClass)
	}
}

func TestValidateLedgersAcceptsEveryDeclaredReleaseClassification(t *testing.T) {
	t.Parallel()

	// The empty value is included on purpose: classification happens after the
	// release exists, so an unclassified item is valid until then.
	classes := []ReleaseClass{
		"",
		ReleaseClose,
		ReleaseSuperseded,
		ReleasePartiallyCovered,
		ReleaseStillValid,
		ReleaseNeedsReproduction,
	}

	for _, class := range classes {
		t.Run(string(class), func(t *testing.T) {
			t.Parallel()

			ledgers := validLedgers()
			ledgers.Backlog[0].Release = class

			if err := ValidateLedgers(ledgers); err != nil {
				t.Fatalf("ValidateLedgers() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateLedgersRejectsSilentNoRetainedInvariantClaim(t *testing.T) {
	t.Parallel()

	ledgers := validLedgers()
	// Claiming no retained invariant while still naming one is contradictory and
	// must fail closed rather than silently trusting either field.
	ledgers.Rows[2].NoRetainedInvariant = true
	ledgers.Rows[2].DestinationPath = ""
	ledgers.Rows[2].DestinationProof = nil

	if err := ValidateLedgers(ledgers); !errors.Is(err, ErrUnprovenDeletion) {
		t.Fatalf("ValidateLedgers() error = %v, want %v", err, ErrUnprovenDeletion)
	}
}
