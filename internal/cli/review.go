package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const (
	ReviewStartSchema    = "gentle-ai.review-start/v1"
	ReviewResumeSchema   = "gentle-ai.review-resume/v1"
	ReviewBundleSchema   = "gentle-ai.review-bundle-result/v1"
	ReviewValidateSchema = "gentle-ai.review-gate-result/v1"
)

type ReviewStartResult struct {
	Schema         string                        `json:"schema"`
	Operation      string                        `json:"operation"`
	Target         reviewtransaction.Snapshot    `json:"target"`
	Transaction    reviewtransaction.Transaction `json:"transaction"`
	StoreAuthority string                        `json:"store_authority"`
	StoreRevision  string                        `json:"store_revision,omitempty"`
}

type ReviewValidateResult struct {
	Schema  string                       `json:"schema"`
	Result  reviewtransaction.GateResult `json:"result"`
	Allowed bool                         `json:"allowed"`
	Action  string                       `json:"action"`
	Reason  string                       `json:"reason"`
}

type ReviewResumeResult struct {
	Schema          string                        `json:"schema"`
	Operation       string                        `json:"operation"`
	Target          reviewtransaction.Snapshot    `json:"target"`
	Transaction     reviewtransaction.Transaction `json:"transaction"`
	StoreAuthority  string                        `json:"store_authority"`
	StoreRevision   string                        `json:"store_revision"`
	GenesisRevision string                        `json:"genesis_revision"`
	ChainIdentity   string                        `json:"chain_identity"`
}

type ReviewBundleResult struct {
	Schema          string `json:"schema"`
	Operation       string `json:"operation"`
	LineageID       string `json:"lineage_id"`
	BundleDigest    string `json:"bundle_digest"`
	StoreRevision   string `json:"store_revision"`
	GenesisRevision string `json:"genesis_revision"`
	ChainIdentity   string `json:"chain_identity"`
	BundlePath      string `json:"bundle_path,omitempty"`
}

type ReviewGateDeniedError struct {
	Result reviewtransaction.GateResult
}

// ReviewStepInput keeps lifecycle mutations explicit while ensuring every
// accepted state transition is performed by the transaction API and appended
// to the authoritative CAS store.
type ReviewStepInput struct {
	Findings        []reviewtransaction.Finding               `json:"findings"`
	LedgerHash      string                                    `json:"ledger_hash"`
	Evidence        []reviewtransaction.FindingEvidence       `json:"evidence"`
	RefuterOutcomes []reviewtransaction.EvidenceResult        `json:"refuter_outcomes"`
	FailedEvidence  string                                    `json:"failed_evidence_revision"`
	Snapshot        *reviewtransaction.Snapshot               `json:"snapshot"`
	FixDeltaHash    string                                    `json:"fix_delta_hash"`
	LedgerIDs       []string                                  `json:"ledger_ids"`
	Validation      *reviewtransaction.ScopedValidationResult `json:"validation"`
	EvidenceHash    string                                    `json:"evidence_hash"`
	Approved        bool                                      `json:"approved"`
	Release         *reviewtransaction.ReleaseEvidence        `json:"release"`
	JudgeProofs     []reviewtransaction.JudgeProof            `json:"judge_proofs"`
	JudgeAgreement  string                                    `json:"judge_agreement_hash"`
	LensResult      *reviewtransaction.LensResult             `json:"lens_result"`
}

func RunReviewStep(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("review-step", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	operation := flags.String("operation", "", "lifecycle operation")
	inputPath := flags.String("input", "", "JSON operation input")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-step argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*operation) == "" || strings.TrimSpace(*inputPath) == "" {
		return errors.New("review-step requires --cwd, --lineage, --operation, and --input")
	}
	payload, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("read review step input: %w", err)
	}
	var input ReviewStepInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return fmt.Errorf("parse review step input: %w", err)
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		return fmt.Errorf("load authoritative review transaction: %w", err)
	}
	tx := chain.Records[len(chain.Records)-1].Transaction
	switch *operation {
	case "record-lens-result":
		if input.LensResult == nil {
			return errors.New("record-lens-result requires lens_result")
		}
		err = tx.RecordLensResult(*input.LensResult)
	case "record-judge-proofs":
		err = tx.RecordJudgeProofs(input.JudgeProofs, input.JudgeAgreement)
	case "freeze-findings":
		err = tx.FreezeFindings(input.Findings, input.LedgerHash)
	case "classify-evidence":
		_, err = tx.ClassifyEvidence(input.Evidence)
	case "apply-refuter-outcomes":
		err = tx.ApplyRefuterOutcomes(input.RefuterOutcomes)
	case "begin-fix":
		err = tx.BeginFix(input.FailedEvidence)
	case "complete-fix":
		if input.Snapshot == nil {
			return errors.New("complete-fix requires snapshot")
		}
		derived, buildErr := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).Build(context.Background(), reviewtransaction.Target{
			Kind: reviewtransaction.TargetFixDiff, BaseRef: tx.FinalCandidateTree,
			IntendedUntracked: input.Snapshot.IntendedUntracked, LedgerIDs: input.LedgerIDs,
		})
		if buildErr != nil {
			return fmt.Errorf("derive correction snapshot: %w", buildErr)
		}
		err = tx.CompleteFix(derived, input.FixDeltaHash, input.LedgerIDs)
	case "validate-fix":
		if input.Validation == nil {
			return errors.New("validate-fix requires validation")
		}
		err = tx.ValidateFixDeltaResult(*input.Validation)
	case "bind-release":
		if input.Release == nil {
			return errors.New("bind-release requires release")
		}
		err = tx.BindReleaseEvidence(*input.Release)
	case "begin-final-verification":
		err = tx.BeginFinalVerification()
	case "complete-final-verification":
		err = tx.CompleteFinalVerification(input.EvidenceHash, input.Approved)
	default:
		return fmt.Errorf("unsupported review lifecycle operation %q", *operation)
	}
	if err != nil {
		return fmt.Errorf("apply review lifecycle operation: %w", err)
	}
	operationName := "review/" + *operation
	if *operation == "bind-release" {
		operationName = "review/bind-release-evidence"
	}
	if *operation == "validate-fix" {
		operationName = "review/validate-targeted-fix"
	}
	revision, err := store.Append(chain.HeadRevision, reviewtransaction.Record{Operation: operationName, Transaction: tx})
	if err != nil {
		return fmt.Errorf("append review lifecycle operation: %w", err)
	}
	result := ReviewResumeResult{Schema: ReviewResumeSchema, Operation: operationName, Target: tx.Snapshot, Transaction: tx, StoreAuthority: "repository-git-common-dir", StoreRevision: revision, GenesisRevision: chain.GenesisRevision}
	if updated, loadErr := store.LoadChain(); loadErr == nil {
		result.ChainIdentity = updated.Identity
	}
	return encodeReviewJSON(stdout, result)
}

func (err ReviewGateDeniedError) Error() string {
	return fmt.Sprintf("review lifecycle gate denied: %s", err.Result)
}

type repeatedString []string

func (values *repeatedString) String() string { return strings.Join(*values, ",") }
func (values *repeatedString) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func RunReviewStart(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("review-start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "repository root")
	kind := flags.String("kind", string(reviewtransaction.TargetCurrentChanges), "target kind")
	baseRef := flags.String("base-ref", "", "base revision")
	revision := flags.String("revision", "", "exact commit or A..B range")
	manifest := flags.String("intended-untracked-manifest", "", "newline-delimited intended untracked paths")
	lineage := flags.String("lineage", "", "review lineage identifier")
	mode := flags.String("mode", string(reviewtransaction.ModeOrdinary4R), "review mode")
	generation := flags.Int("generation", 1, "lineage generation")
	policyFile := flags.String("policy-file", "", "review policy artifact to hash")
	machineTransactionOut := flags.String("machine-transaction-out", "", "optional non-authoritative transaction JSON output path")
	var intended repeatedString
	var ledgerIDs repeatedString
	var selectedLenses repeatedString
	flags.Var(&intended, "intended-untracked", "repository-relative intended untracked path; repeatable")
	flags.Var(&ledgerIDs, "ledger-id", "frozen ledger finding ID for fix-diff; repeatable and comma-safe")
	flags.Var(&selectedLenses, "lens", "selected ordinary bounded review lens; repeatable in canonical 4R order")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-start argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*policyFile) == "" {
		return errors.New("review-start requires --cwd, --lineage, and --policy-file")
	}
	policyHash, err := reviewtransaction.HashArtifact(*policyFile)
	if err != nil {
		return fmt.Errorf("hash review policy: %w", err)
	}
	manifestPaths, err := readIntendedManifest(*manifest)
	if err != nil {
		return err
	}
	intended = append(intended, manifestPaths...)
	targetKind := reviewtransaction.TargetKind(*kind)
	if (targetKind == reviewtransaction.TargetCurrentChanges || targetKind == reviewtransaction.TargetFixDiff) && intended == nil && strings.TrimSpace(*manifest) != "" {
		intended = repeatedString{}
	}
	if targetKind == reviewtransaction.TargetCurrentChanges && intended == nil {
		intended = repeatedString{}
	}
	if err := validateReviewStartTargetArgs(targetKind, *baseRef, *revision, intended, ledgerIDs); err != nil {
		return err
	}

	target := reviewtransaction.Target{
		Kind: targetKind, BaseRef: *baseRef, Revision: *revision,
		IntendedUntracked: []string(intended), LedgerIDs: []string(ledgerIDs),
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).Build(context.Background(), target)
	if err != nil {
		return fmt.Errorf("build review target: %w", err)
	}
	riskLevel := reviewtransaction.RiskLevel("")
	if reviewtransaction.Mode(*mode) == reviewtransaction.ModeOrdinaryBounded {
		riskLevel, err = classifyReviewSnapshot(context.Background(), *cwd, snapshot)
		if err != nil {
			return fmt.Errorf("classify immutable review target: %w", err)
		}
	}
	transaction, err := reviewtransaction.NewTransaction(reviewtransaction.Start{
		LineageID: *lineage, Mode: reviewtransaction.Mode(*mode), Generation: *generation,
		Snapshot: snapshot, PolicyHash: policyHash, RiskLevel: riskLevel, SelectedLenses: []string(selectedLenses),
	})
	if err != nil {
		return fmt.Errorf("create review transaction: %w", err)
	}
	if err := transaction.StartReview(); err != nil {
		return fmt.Errorf("start review transaction: %w", err)
	}

	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	result := ReviewStartResult{
		Schema: ReviewStartSchema, Operation: "review/start", Target: snapshot, Transaction: *transaction,
		StoreAuthority: "repository-git-common-dir",
	}
	revisionValue, err := store.Append("", reviewtransaction.Record{
		Operation: "review/start", Transaction: *transaction,
	})
	if err != nil {
		return fmt.Errorf("persist review transaction: %w", err)
	}
	result.StoreRevision = revisionValue
	if strings.TrimSpace(*machineTransactionOut) != "" {
		if err := reviewtransaction.WriteTransactionAtomic(*machineTransactionOut, *transaction); err != nil {
			return fmt.Errorf("write non-authoritative machine transaction output: %w", err)
		}
	}
	return encodeReviewJSON(stdout, result)
}

func classifyReviewSnapshot(ctx context.Context, repo string, snapshot reviewtransaction.Snapshot) (reviewtransaction.RiskLevel, error) {
	command := exec.CommandContext(ctx, "git", "-C", repo, "diff", "--numstat", "--no-renames", snapshot.BaseTree, snapshot.CandidateTree, "--")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	stats := make([]reviewtransaction.DiffStat, 0, len(snapshot.Paths))
	onlyNonExecutable := true
	touchesConfiguration := false
	seenPaths := make(map[string]struct{}, len(snapshot.Paths))
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return "", fmt.Errorf("unexpected immutable diff stat %q", line)
		}
		stat := reviewtransaction.DiffStat{Path: fields[2]}
		if fields[0] == "-" && fields[1] == "-" {
			stat.Binary = true
		} else {
			stat.Additions, err = strconv.Atoi(fields[0])
			if err != nil {
				return "", fmt.Errorf("parse additions for %q: %w", stat.Path, err)
			}
			stat.Deletions, err = strconv.Atoi(fields[1])
			if err != nil {
				return "", fmt.Errorf("parse deletions for %q: %w", stat.Path, err)
			}
		}
		stats = append(stats, stat)
		seenPaths[stat.Path] = struct{}{}
		onlyNonExecutable = onlyNonExecutable && isNonExecutableReviewPath(stat.Path)
		touchesConfiguration = touchesConfiguration || isConfigurationReviewPath(stat.Path)
	}
	for _, path := range snapshot.Paths {
		if _, ok := seenPaths[path]; !ok {
			return "", fmt.Errorf("immutable snapshot path %q is missing from tree diff stats", path)
		}
	}
	if len(seenPaths) != len(snapshot.Paths) {
		return "", errors.New("immutable tree diff contains paths outside the review snapshot")
	}
	return reviewtransaction.ClassifyRisk(reviewtransaction.RiskInput{
		Stats: stats, OnlyNonExecutableChanges: onlyNonExecutable, TouchesConfiguration: touchesConfiguration,
	})
}

func isNonExecutableReviewPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".mdx", ".rst", ".adoc", ".png", ".jpg", ".jpeg", ".gif", ".svg":
		return true
	default:
		return false
	}
}

func isConfigurationReviewPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "dockerfile", "makefile":
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml", ".toml", ".ini", ".env":
		return true
	default:
		return false
	}
}

func RunReviewResume(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("review-resume", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	machineTransactionOut := flags.String("machine-transaction-out", "", "optional non-authoritative transaction JSON output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-resume argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" {
		return errors.New("review-resume requires --cwd and --lineage")
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	chain, err := store.LoadChain()
	if err != nil {
		return fmt.Errorf("load authoritative review transaction: %w", err)
	}
	transaction := chain.Records[len(chain.Records)-1].Transaction
	if strings.TrimSpace(*machineTransactionOut) != "" {
		if err := reviewtransaction.WriteTransactionAtomic(*machineTransactionOut, transaction); err != nil {
			return fmt.Errorf("write non-authoritative machine transaction output: %w", err)
		}
	}
	return encodeReviewJSON(stdout, ReviewResumeResult{
		Schema: ReviewResumeSchema, Operation: "review/resume", Target: transaction.Snapshot,
		Transaction: transaction, StoreAuthority: "repository-git-common-dir",
		StoreRevision: chain.HeadRevision, GenesisRevision: chain.GenesisRevision, ChainIdentity: chain.Identity,
	})
}

func RunReviewBundleExport(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("review-bundle-export", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	out := flags.String("out", "", "portable review chain bundle output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-bundle-export argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*out) == "" {
		return errors.New("review-bundle-export requires --cwd, --lineage, and --out")
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	bundle, err := store.ExportBundle()
	if err != nil {
		return fmt.Errorf("export authoritative review chain: %w", err)
	}
	if err := reviewtransaction.WriteChainBundleAtomic(*out, bundle); err != nil {
		return fmt.Errorf("write portable review chain bundle: %w", err)
	}
	return encodeReviewJSON(stdout, ReviewBundleResult{
		Schema: ReviewBundleSchema, Operation: "review/bundle-export", LineageID: bundle.LineageID,
		BundleDigest: bundle.BundleDigest, StoreRevision: bundle.HeadRevision,
		GenesisRevision: bundle.GenesisRevision, ChainIdentity: bundle.ChainIdentity, BundlePath: *out,
	})
}

func RunReviewBundleImport(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("review-bundle-import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "repository root")
	bundlePath := flags.String("bundle", "", "portable review chain bundle")
	receiptPath := flags.String("receipt", "", "terminal review receipt")
	requestPath := flags.String("request", "", "gate request binding current artifacts and expected chain identity")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-bundle-import argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*bundlePath) == "" || strings.TrimSpace(*requestPath) == "" {
		return errors.New("review-bundle-import requires --cwd, --bundle, and --request")
	}
	bundlePayload, err := os.ReadFile(*bundlePath)
	if err != nil {
		return fmt.Errorf("read review chain bundle: %w", err)
	}
	bundle, err := reviewtransaction.ParseChainBundle(bundlePayload)
	if err != nil {
		return fmt.Errorf("parse review chain bundle: %w", err)
	}
	var receipt reviewtransaction.Receipt
	if strings.TrimSpace(*receiptPath) != "" {
		receiptPayload, err := os.ReadFile(*receiptPath)
		if err != nil {
			return fmt.Errorf("read review receipt: %w", err)
		}
		receipt, err = reviewtransaction.ParseReceipt(receiptPayload)
		if err != nil {
			return fmt.Errorf("parse review receipt: %w", err)
		}
		if bundle.TerminalReceipt == nil {
			return errors.New("nonterminal review bundle cannot be imported with a terminal receipt")
		}
	} else if bundle.TerminalReceipt != nil {
		return errors.New("terminal review bundle import requires --receipt")
	}
	requestPayload, err := os.ReadFile(*requestPath)
	if err != nil {
		return fmt.Errorf("read review gate request: %w", err)
	}
	request, err := reviewtransaction.ParseGateRequest(requestPayload)
	if err != nil {
		return fmt.Errorf("parse review gate request: %w", err)
	}
	snapshot, err := (reviewtransaction.SnapshotBuilder{Repo: *cwd}).Build(context.Background(), request.Target)
	if err != nil {
		return fmt.Errorf("derive current repository target: %w", err)
	}
	policyHash, ledgerHash, evidenceHash := bundle.PolicyHash, bundle.LedgerHash, bundle.EvidenceHash
	if bundle.TerminalReceipt != nil {
		policyHash, err = reviewtransaction.HashArtifact(request.PolicyArtifact)
		if err != nil {
			return fmt.Errorf("hash policy artifact: %w", err)
		}
		ledgerHash, err = reviewtransaction.HashLedgerArtifact(request.LedgerArtifact)
		if err != nil {
			return fmt.Errorf("hash ledger artifact: %w", err)
		}
		evidenceHash, err = reviewtransaction.HashArtifact(request.EvidenceArtifact)
		if err != nil {
			return fmt.Errorf("hash evidence artifact: %w", err)
		}
	}
	fixDeltaHash := ""
	chain, err := reviewtransaction.ImportBundle(context.Background(), *cwd, bundle, reviewtransaction.BundleImportExpectation{
		LineageID: bundle.LineageID, Snapshot: snapshot,
		PolicyHash: policyHash, LedgerHash: ledgerHash, EvidenceHash: evidenceHash, FixDeltaHash: fixDeltaHash, Receipt: receipt,
		GenesisRevision: request.GenesisRevision, HeadRevision: request.StoreRevision,
		ChainIdentity: request.ChainIdentity, BundleDigest: request.BundleDigest,
	})
	if err != nil {
		return fmt.Errorf("install validated review chain bundle: %w", err)
	}
	return encodeReviewJSON(stdout, ReviewBundleResult{
		Schema: ReviewBundleSchema, Operation: "review/bundle-import", LineageID: bundle.LineageID,
		BundleDigest: bundle.BundleDigest, StoreRevision: chain.HeadRevision,
		GenesisRevision: chain.GenesisRevision, ChainIdentity: chain.Identity, BundlePath: *bundlePath,
	})
}

func RunReviewValidate(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("review-validate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "repository root")
	receiptPath := flags.String("receipt", "", "review receipt JSON")
	requestPath := flags.String("request", "", "review gate request JSON containing artifact paths, not derived facts")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-validate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*receiptPath) == "" || strings.TrimSpace(*requestPath) == "" {
		return errors.New("review-validate requires --cwd, --receipt, and --request")
	}
	receiptPayload, err := os.ReadFile(*receiptPath)
	if err != nil {
		return fmt.Errorf("read review receipt: %w", err)
	}
	receipt, err := reviewtransaction.ParseReceipt(receiptPayload)
	if err != nil {
		return fmt.Errorf("parse review receipt: %w", err)
	}
	requestPayload, err := os.ReadFile(*requestPath)
	if err != nil {
		return fmt.Errorf("read review gate request: %w", err)
	}
	request, err := reviewtransaction.ParseGateRequest(requestPayload)
	if err != nil {
		return fmt.Errorf("parse review gate request: %w", err)
	}
	evaluation := reviewtransaction.EvaluateNativeGate(context.Background(), *cwd, receipt, request)
	result := ReviewValidateResult{
		Schema: ReviewValidateSchema, Result: evaluation.Result, Allowed: evaluation.Result == reviewtransaction.GateAllow,
		Action: reviewGateAction(evaluation.Result), Reason: evaluation.Reason,
	}
	if err := encodeReviewJSON(stdout, result); err != nil {
		return err
	}
	if !result.Allowed {
		return ReviewGateDeniedError{Result: result.Result}
	}
	return nil
}

func validateReviewStartTargetArgs(kind reviewtransaction.TargetKind, baseRef, revision string, intended, ledgerIDs []string) error {
	switch kind {
	case reviewtransaction.TargetCurrentChanges:
		if strings.TrimSpace(baseRef) != "" || strings.TrimSpace(revision) != "" || len(ledgerIDs) != 0 {
			return errors.New("current-changes does not accept --base-ref, --revision, or --ledger-id")
		}
	case reviewtransaction.TargetBaseDiff:
		if strings.TrimSpace(baseRef) == "" {
			return errors.New("base-diff requires --base-ref")
		}
		if strings.TrimSpace(revision) != "" || len(ledgerIDs) != 0 {
			return errors.New("base-diff does not accept --revision or --ledger-id")
		}
	case reviewtransaction.TargetExactRevision:
		if strings.TrimSpace(revision) == "" {
			return errors.New("commit-range requires --revision")
		}
		if strings.TrimSpace(baseRef) != "" || len(ledgerIDs) != 0 {
			return errors.New("commit-range does not accept --base-ref or --ledger-id")
		}
	case reviewtransaction.TargetFixDiff:
		if strings.TrimSpace(baseRef) == "" || len(ledgerIDs) == 0 {
			return errors.New("fix-diff requires --base-ref and at least one repeatable --ledger-id")
		}
		if intended == nil {
			return errors.New("fix-diff requires --intended-untracked or --intended-untracked-manifest, including an explicit empty manifest")
		}
		if strings.TrimSpace(revision) != "" {
			return errors.New("fix-diff does not accept --revision")
		}
	default:
		return fmt.Errorf("unsupported target kind %q", kind)
	}
	return nil
}

func readIntendedManifest(path string) ([]string, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read intended-untracked manifest: %w", err)
	}
	defer file.Close()
	paths := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if value := strings.TrimSpace(scanner.Text()); value != "" {
			paths = append(paths, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read intended-untracked manifest: %w", err)
	}
	return paths, nil
}

func reviewGateAction(result reviewtransaction.GateResult) string {
	switch result {
	case reviewtransaction.GateAllow:
		return "continue"
	case reviewtransaction.GateScopeChanged:
		return "create-new-lineage"
	case reviewtransaction.GateEscalated:
		return "stop"
	default:
		return "explicit-maintainer-action"
	}
}

func encodeReviewJSON(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
