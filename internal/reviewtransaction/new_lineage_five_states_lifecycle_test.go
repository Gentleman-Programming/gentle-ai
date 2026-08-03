package reviewtransaction

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// forbiddenDerivedCategoryStrings are the five derived-category words this
// package's own NewLineageState doc comment (authority_store.go) names as
// "a distinct, unpersisted concept a future reader derives at evaluation
// time": invalidated, scope_changed (GateResult's own spelling is
// "scope-changed" — both variants are checked), ambiguous, repairable,
// corrupted. None of the five may ever appear in review-state.json or
// review-receipt.json bytes; only the five persisted states
// (reviewing/correcting/validating/approved/escalated) may.
var forbiddenDerivedCategoryStrings = []string{
	`"invalidated"`, `"scope_changed"`, `"scope-changed"`, `"ambiguous"`, `"repairable"`, `"corrupted"`,
}

func assertNoDerivedCategoryPersisted(t *testing.T, label string, raw []byte) {
	t.Helper()
	for _, forbidden := range forbiddenDerivedCategoryStrings {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("%s bytes contain the derived-category string %s, which must never be persisted: %s", label, forbidden, raw)
		}
	}
}

func assertPersistedStateInClosedVocabulary(t *testing.T, raw []byte) NewLineageState {
	t.Helper()
	var probe struct {
		Authority struct {
			State NewLineageState `json:"state"`
		} `json:"authority"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("unmarshal review-state.json probe: %v", err)
	}
	if !probe.Authority.State.valid() {
		t.Fatalf("persisted state %q is not one of the five closed-vocabulary states", probe.Authority.State)
	}
	return probe.Authority.State
}

// TestNewLineageFivePersistedStatesLifecycleNeverPersistsDerivedCategory is
// task C4a: walks a full new-lineage lifecycle end to end — reviewing ->
// correcting -> validating -> approved (one lineage) and reviewing ->
// validating -> escalated (a second lineage) — asserting that
// review-state.json's state field only ever holds one of the five closed
// values at every step, and that neither artifact's raw bytes ever contain
// one of the five derived-category words this package's own doc comment
// names as unpersisted (spec rdd-authority-store, "Five Persisted States,
// Everything Else Derived").
func TestNewLineageFivePersistedStatesLifecycleNeverPersistsDerivedCategory(t *testing.T) {
	repo := initSnapshotRepo(t)

	t.Run("approved path", func(t *testing.T) {
		store, err := NewLineageAuthorityStore(context.Background(), repo, "five-states-approved-lineage")
		if err != nil {
			t.Fatal(err)
		}
		identity := fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("b", 40), "c", "d")
		revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
			next.State = NewLineageStateReviewing
			next.CandidateIdentity = identity
			next.Tier = RiskLow
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(store.StatePath())
		if err != nil {
			t.Fatal(err)
		}
		if got := assertPersistedStateInClosedVocabulary(t, raw); got != NewLineageStateReviewing {
			t.Fatalf("state = %q, want reviewing", got)
		}
		assertNoDerivedCategoryPersisted(t, "review-state.json (reviewing)", raw)

		for _, step := range []NewLineageState{NewLineageStateCorrecting, NewLineageStateValidating, NewLineageStateApproved} {
			revision, err = store.Mutate(context.Background(), revision, func(next *NewLineageAuthority) error {
				next.State = step
				return nil
			})
			if err != nil {
				t.Fatalf("advance to %s: %v", step, err)
			}
			raw, err = os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if got := assertPersistedStateInClosedVocabulary(t, raw); got != step {
				t.Fatalf("state = %q, want %q", got, step)
			}
			assertNoDerivedCategoryPersisted(t, "review-state.json ("+string(step)+")", raw)
		}

		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		receiptRevision, err := NewLineageRevisionForState(record.Authority)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.WriteReceipt(context.Background(), NewLineageReceipt{
			Schema: NewLineageReceiptSchema, LineageID: record.Authority.LineageID,
			TerminalState: record.Authority.State, AuthorityRevision: receiptRevision,
			CandidateIdentity: record.Authority.CandidateIdentity,
		}); err != nil {
			t.Fatal(err)
		}
		receiptRaw, err := os.ReadFile(store.ReceiptPath())
		if err != nil {
			t.Fatal(err)
		}
		assertNoDerivedCategoryPersisted(t, "review-receipt.json", receiptRaw)
	})

	t.Run("escalated path", func(t *testing.T) {
		store, err := NewLineageAuthorityStore(context.Background(), repo, "five-states-escalated-lineage")
		if err != nil {
			t.Fatal(err)
		}
		identity := fixtureCandidateIdentity(strings.Repeat("a", 40), strings.Repeat("1", 40), "e", "f")
		revision, err := store.Mutate(context.Background(), "", func(next *NewLineageAuthority) error {
			next.State = NewLineageStateReviewing
			next.CandidateIdentity = identity
			next.Tier = RiskLow
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range []NewLineageState{NewLineageStateValidating, NewLineageStateEscalated} {
			revision, err = store.Mutate(context.Background(), revision, func(next *NewLineageAuthority) error {
				next.State = step
				return nil
			})
			if err != nil {
				t.Fatalf("advance to %s: %v", step, err)
			}
			raw, err := os.ReadFile(store.StatePath())
			if err != nil {
				t.Fatal(err)
			}
			if got := assertPersistedStateInClosedVocabulary(t, raw); got != step {
				t.Fatalf("state = %q, want %q", got, step)
			}
			assertNoDerivedCategoryPersisted(t, "review-state.json ("+string(step)+")", raw)
		}

		record, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		receiptRevision, err := NewLineageRevisionForState(record.Authority)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.WriteReceipt(context.Background(), NewLineageReceipt{
			Schema: NewLineageReceiptSchema, LineageID: record.Authority.LineageID,
			TerminalState: record.Authority.State, AuthorityRevision: receiptRevision,
			CandidateIdentity: record.Authority.CandidateIdentity,
		}); err != nil {
			t.Fatal(err)
		}
		receiptRaw, err := os.ReadFile(store.ReceiptPath())
		if err != nil {
			t.Fatal(err)
		}
		assertNoDerivedCategoryPersisted(t, "review-receipt.json", receiptRaw)
	})
}
