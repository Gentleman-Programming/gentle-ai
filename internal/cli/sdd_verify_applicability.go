package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

// SDDVerifyApplicabilitySchema versions the assessment payload. SDD owns this
// decision, so it carries its own schema rather than projecting a review one.
const SDDVerifyApplicabilitySchema = "gentle-ai.sdd-verify-applicability/v1"

// SDD final-verification decisions. `required` is the conservative default:
// every outcome that is not a proven-passive candidate resolves to it.
const (
	SDDVerifyRequired    = "required"
	SDDVerifyNotRequired = "not_required"
)

// Reason codes for the assessment. They explain the decision; they never carry
// authority, and a consumer must route from Decision alone.
const (
	SDDVerifyReasonEmptyCandidate   = "empty_candidate"
	SDDVerifyReasonActiveContent    = "active_content"
	SDDVerifyReasonUnknownContent   = "unknown_content"
	SDDVerifyReasonPassiveCandidate = "passive_candidate"
)

// SDDVerifyApplicability is the assessment result. It reports a decision for
// one exact candidate tree and never claims verification happened.
type SDDVerifyApplicability struct {
	Schema        string   `json:"schema"`
	Decision      string   `json:"decision"`
	Reason        string   `json:"reason"`
	CandidateTree string   `json:"candidate_tree"`
	PathsDigest   string   `json:"paths_digest"`
	DecidingPaths []string `json:"deciding_paths"`
	EvidenceGoal  string   `json:"evidence_goal,omitempty"`
	CoveredPaths  []string `json:"covered_paths,omitempty"`
}

// decideSDDVerifyApplicability is the SDD-owned policy. It reads shared
// classification facts and answers only SDD's question: is an independent
// final verification still required for this candidate?
//
// It is deliberately separate from the review lifecycle's own answer. A
// candidate that review does not consider risky may still owe SDD a
// verification, so this never consults a review verdict, plan, or gate.
//
// Every outcome except a fully passive, non-empty candidate is `required`.
func decideSDDVerifyApplicability(facts []reviewtransaction.CandidateContentFact) (decision, reason string, deciding []string) {
	if len(facts) == 0 {
		// An empty candidate proves nothing passive. Routing it to
		// not_required would let a stale or misresolved target skip the
		// verifier, so it fails closed like any other unproven case.
		return SDDVerifyRequired, SDDVerifyReasonEmptyCandidate, nil
	}
	active := make([]string, 0, len(facts))
	unknown := make([]string, 0, len(facts))
	covered := make([]string, 0, len(facts))
	for _, fact := range facts {
		switch fact.Activity {
		case reviewtransaction.VerificationContentPassive:
			covered = append(covered, fact.Path)
		case reviewtransaction.VerificationContentActive:
			active = append(active, fact.Path)
		default:
			unknown = append(unknown, fact.Path)
		}
	}
	// Active content is reported ahead of unknown content because it is the
	// stronger, non-speculative statement about why the verifier must run.
	if len(active) != 0 {
		sort.Strings(active)
		return SDDVerifyRequired, SDDVerifyReasonActiveContent, active
	}
	if len(unknown) != 0 {
		sort.Strings(unknown)
		return SDDVerifyRequired, SDDVerifyReasonUnknownContent, unknown
	}
	sort.Strings(covered)
	return SDDVerifyNotRequired, SDDVerifyReasonPassiveCandidate, covered
}

// sddVerifyEvidenceGoals names what an independent verification must still
// establish for each required reason, so a consumer can explain the decision
// without restating policy. A not_required decision has no goal.
var sddVerifyEvidenceGoals = map[string]string{
	SDDVerifyReasonEmptyCandidate: "resolve a non-empty candidate, then obtain a current passing verification report",
	SDDVerifyReasonActiveContent:  "obtain a current passing verification report covering the active paths",
	SDDVerifyReasonUnknownContent: "obtain a current passing verification report covering the unclassifiable paths",
}

func evidenceGoalForSDDVerify(reason string) string { return sddVerifyEvidenceGoals[reason] }

// RunSDDVerifyApplicability assesses whether an independent final SDD
// verification is still required for the current candidate.
//
// It is read-only. It creates no authority, writes no report, and does not
// make archive ready; a consumer still routes from persisted verification
// state exactly as it does today.
func RunSDDVerifyApplicability(args []string, stdout io.Writer) error {
	return runSDDVerifyApplicability(context.Background(), args, stdout)
}

func runSDDVerifyApplicability(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("sdd-verify-applicability", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "Repository directory; defaults to the process working directory")
	projection := flags.String("projection", string(reviewtransaction.ProjectionWorkspace), "Candidate projection: workspace or staged")
	var intended, operational repeatedPathFlag
	flags.Var(&intended, "intended-untracked", "Untracked path that belongs to the candidate; repeatable")
	flags.Var(&operational, "operational-path", "Path SDD treats as operational; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected sdd-verify-applicability argument %q; the candidate comes from the repository, so run `gentle-ai sdd-verify-applicability --cwd <repo>` with flags only", flags.Arg(0))
	}
	selected := reviewtransaction.Projection(strings.TrimSpace(*projection))
	if selected != reviewtransaction.ProjectionWorkspace && selected != reviewtransaction.ProjectionStaged {
		return fmt.Errorf("unsupported projection %q; run `gentle-ai sdd-verify-applicability --projection workspace` or `gentle-ai sdd-verify-applicability --projection staged`", *projection)
	}
	directory := strings.TrimSpace(*cwd)
	if directory == "" {
		working, err := os.Getwd()
		if err != nil {
			return err
		}
		directory = working
	}
	root, err := (reviewtransaction.SnapshotBuilder{Repo: directory}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return err
	}
	builder := reviewtransaction.SnapshotBuilder{Repo: root}
	snapshot, err := builder.Build(ctx, reviewtransaction.Target{
		Kind: reviewtransaction.TargetCurrentChanges, Projection: selected,
		IntendedUntracked: intended.values(),
	})
	if err != nil {
		return err
	}
	facts, err := builder.CandidateContentFacts(ctx, snapshot, operational.values(), nil)
	if err != nil {
		return err
	}
	decision, reason, deciding := decideSDDVerifyApplicability(facts)
	result := SDDVerifyApplicability{
		Schema: SDDVerifyApplicabilitySchema, Decision: decision, Reason: reason,
		CandidateTree: snapshot.CandidateTree, PathsDigest: snapshot.PathsDigest,
		DecidingPaths: deciding,
	}
	if decision == SDDVerifyRequired {
		result.EvidenceGoal = evidenceGoalForSDDVerify(reason)
	} else {
		result.CoveredPaths = deciding
		result.DecidingPaths = nil
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%s\n", payload)
	return err
}

// repeatedPathFlag collects a repeatable path flag. It always yields a
// non-nil slice, because an explicit empty list and an absent list are
// distinct inputs to snapshot construction.
type repeatedPathFlag struct {
	collected []string
}

func (flagValue *repeatedPathFlag) String() string { return strings.Join(flagValue.collected, ",") }

func (flagValue *repeatedPathFlag) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("path must not be empty; pass a repository-relative path, as in `gentle-ai sdd-verify-applicability --intended-untracked docs/guide.md`")
	}
	flagValue.collected = append(flagValue.collected, trimmed)
	return nil
}

func (flagValue *repeatedPathFlag) values() []string {
	if flagValue.collected == nil {
		return []string{}
	}
	return flagValue.collected
}
