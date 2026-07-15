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
	"strings"

	"github.com/gentleman-programming/gentle-ai/internal/reviewtransaction"
)

const (
	ReviewResumeSchema   = "gentle-ai.review-resume/v1"
	ReviewBundleSchema   = "gentle-ai.review-bundle-result/v1"
	ReviewValidateSchema = "gentle-ai.review-gate-result/v1"
)

type ReviewValidateResult struct {
	Schema  string                        `json:"schema"`
	Result  reviewtransaction.GateResult  `json:"result"`
	Allowed bool                          `json:"allowed"`
	Action  string                        `json:"action"`
	Reason  string                        `json:"reason"`
	Context reviewtransaction.GateContext `json:"context"`
}

func newReviewFlagSet(name string, stdout io.Writer, details string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stdout)
	flags.Usage = func() {
		_, _ = fmt.Fprintf(stdout, "Usage: gentle-ai %s [flags]\n\n%s\n\nFlags:\n", name, details)
		flags.VisitAll(func(current *flag.Flag) {
			_, _ = fmt.Fprintf(stdout, "  --%s <value>\n      %s", current.Name, current.Usage)
			if current.DefValue != "" {
				_, _ = fmt.Fprintf(stdout, " (default %q)", current.DefValue)
			}
			_, _ = fmt.Fprintln(stdout)
		})
		_, _ = fmt.Fprintln(stdout, "  -h, --help\n      show this help")
	}
	return flags
}

func parseReviewFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	return nil
}

func reviewHelpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
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

func RunReviewStep(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-step", stdout, "Read-only legacy v1 compatibility command. Mutation is rejected; use gentle-ai review finalize for compact authority.")
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	operation := flags.String("operation", "", "legacy lifecycle operation rejected as read-only")
	inputPath := flags.String("input", "", "legacy JSON operation input")
	_ = flags.String("ledger", "", "legacy ledger input")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-step argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*operation) == "" || strings.TrimSpace(*inputPath) == "" {
		return errors.New("review-step requires --cwd, --lineage, --operation, and --input")
	}
	store, err := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
	if err != nil {
		return fmt.Errorf("derive authoritative review store: %w", err)
	}
	if _, err := store.LoadChain(); err != nil {
		return fmt.Errorf("load authoritative review transaction: %w", err)
	}
	attemptedOperation := strings.TrimSpace(*operation)
	if !strings.HasPrefix(attemptedOperation, "review/") {
		attemptedOperation = "review/" + attemptedOperation
	}
	return fmt.Errorf("%w: review-step cannot mutate shipped v1 authority; use gentle-ai review finalize", reviewtransaction.NewLegacyReadOnlyError(attemptedOperation, *lineage))
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
	flags := newReviewFlagSet("review-start", stdout, "Read-only legacy v1 compatibility command. New authority is created with gentle-ai review start.")
	cwd := flags.String("cwd", "", "repository root")
	_ = flags.String("kind", string(reviewtransaction.TargetCurrentChanges), "legacy target kind")
	_ = flags.String("base-ref", "", "legacy base revision")
	_ = flags.String("revision", "", "legacy exact commit or A..B range")
	_ = flags.String("intended-untracked-manifest", "", "legacy intended-untracked manifest")
	lineage := flags.String("lineage", "", "review lineage identifier")
	_ = flags.String("mode", string(reviewtransaction.ModeOrdinary4R), "legacy review mode")
	_ = flags.Int("generation", 1, "legacy lineage generation")
	policyFile := flags.String("policy-file", "", "legacy review policy argument (read-only compatibility)")
	_ = flags.String("machine-transaction-out", "", "legacy non-authoritative transaction output")
	var ignored repeatedString
	flags.Var(&ignored, "intended-untracked", "legacy intended-untracked path")
	flags.Var(&ignored, "ledger-id", "legacy ledger finding ID")
	flags.Var(&ignored, "lens", "legacy selected review lens")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-start argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*policyFile) == "" {
		return errors.New("review-start requires --cwd, --lineage, and --policy-file")
	}
	return fmt.Errorf("%w: review-start cannot create v1 authority; use gentle-ai review start", reviewtransaction.NewLegacyReadOnlyError("review/start", *lineage))
}

func RunReviewResume(args []string, stdout io.Writer) error {
	flags := newReviewFlagSet("review-resume", stdout, "Re-emit the current authoritative review transaction without consuming budget.")
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	machineTransactionOut := flags.String("machine-transaction-out", "", "optional non-authoritative transaction JSON output path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
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
	flags := newReviewFlagSet("review-bundle-export", stdout, "Export compact current authority or a read-only legacy v1 chain.")
	cwd := flags.String("cwd", "", "repository root")
	lineage := flags.String("lineage", "", "review lineage identifier")
	out := flags.String("out", "", "portable review chain bundle output path")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-bundle-export argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*out) == "" {
		return errors.New("review-bundle-export requires --cwd, --lineage, and --out")
	}
	compact, compactErr := reviewtransaction.CompactAuthoritativeStore(context.Background(), *cwd, *lineage)
	if compactErr == nil {
		if transport, loadErr := compact.ExportTransport(); loadErr == nil {
			legacy, legacyErr := reviewtransaction.AuthoritativeStore(context.Background(), *cwd, *lineage)
			if legacyErr == nil {
				if _, legacyLoadErr := legacy.LoadChain(); legacyLoadErr == nil {
					return errors.New("lineage exists in both compact and legacy stores; explicit cleanup is required")
				}
			}
			if err := reviewtransaction.WriteCompactTransportAtomic(*out, transport); err != nil {
				return fmt.Errorf("write compact review transport: %w", err)
			}
			return encodeReviewJSON(stdout, ReviewBundleResult{
				Schema: ReviewBundleSchema, Operation: "review/bundle-export", LineageID: transport.Record.State.LineageID,
				BundleDigest: transport.BundleDigest, StoreRevision: transport.Record.Revision,
				GenesisRevision: transport.Record.Revision, ChainIdentity: transport.Record.Revision, BundlePath: *out,
			})
		}
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
	flags := newReviewFlagSet("review-bundle-import", stdout, "Recover compact current authority or install a read-only legacy v1 chain.")
	cwd := flags.String("cwd", "", "repository root")
	bundlePath := flags.String("bundle", "", "portable review chain bundle")
	receiptPath := flags.String("receipt", "", "terminal review receipt")
	requestPath := flags.String("request", "", "gate request binding current artifacts and expected chain identity")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-bundle-import argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*bundlePath) == "" {
		return errors.New("review-bundle-import requires --cwd and --bundle")
	}
	bundlePayload, err := os.ReadFile(*bundlePath)
	if err != nil {
		return fmt.Errorf("read review chain bundle: %w", err)
	}
	var envelope struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(bundlePayload, &envelope); err != nil {
		return fmt.Errorf("parse review transport header: %w", err)
	}
	if envelope.Schema == reviewtransaction.CompactTransportSchema {
		if strings.TrimSpace(*receiptPath) != "" || strings.TrimSpace(*requestPath) != "" {
			return errors.New("compact transport contains its receipt and does not accept legacy --receipt or --request ceremony")
		}
		transport, err := reviewtransaction.ParseCompactTransport(bundlePayload)
		if err != nil {
			return fmt.Errorf("parse compact review transport: %w", err)
		}
		record, err := reviewtransaction.ImportCompactTransport(context.Background(), *cwd, transport)
		if err != nil {
			return fmt.Errorf("recover compact review authority: %w", err)
		}
		return encodeReviewJSON(stdout, ReviewBundleResult{
			Schema: ReviewBundleSchema, Operation: "review/bundle-import", LineageID: record.State.LineageID,
			BundleDigest: transport.BundleDigest, StoreRevision: record.Revision,
			GenesisRevision: record.Revision, ChainIdentity: record.Revision, BundlePath: *bundlePath,
		})
	}
	if strings.TrimSpace(*requestPath) == "" {
		return errors.New("legacy v1 review bundle import requires --request")
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
	flags := newReviewFlagSet("review-validate", stdout, "Validate a receipt using either --request or native authority flags. Explicit and native modes are mutually exclusive.")
	cwd := flags.String("cwd", "", "repository root")
	receiptPath := flags.String("receipt", "", "review receipt JSON")
	requestPath := flags.String("request", "", "review gate request JSON containing artifact paths, not derived facts")
	lineage := flags.String("lineage", "", "authoritative review lineage identifier (native mode)")
	gate := flags.String("gate", "", "lifecycle gate: post-apply, pre-commit, pre-push, pre-pr, or release (native mode)")
	bundlePath := flags.String("bundle", "", "authoritative chain bundle artifact (native mode)")
	policyPath := flags.String("policy", "", "receipt-bound policy artifact (native mode)")
	ledgerPath := flags.String("ledger", "", "frozen ledger artifact (native mode)")
	fixDeltaPath := flags.String("fix-delta", "", "optional correction delta artifact (native mode)")
	evidencePath := flags.String("evidence", "", "final verification evidence artifact (native mode)")
	baseRef := flags.String("base-ref", "", "optional expected remote publication base for pre-pr native mode")
	ciAttestation := flags.String("pre-pr-ci-attestation", "", "signed exact-merged-tree CI attestation for a compatible base advance")
	requestOut := flags.String("request-out", "", "optional canonical native gate request output path")
	releaseConfiguration := flags.String("release-configuration", "", "release configuration artifact")
	releaseGenerated := flags.String("release-generated", "", "generated artifact manifest")
	releaseProvenance := flags.String("release-provenance", "", "release provenance artifact")
	releaseBoundary := flags.String("release-publication-boundary", "", "semantic sealed publication boundary artifact")
	releaseFreshness := flags.String("release-evidence-freshness", "", "semantic current evidence freshness artifact")
	manifest := flags.String("intended-untracked-manifest", "", "newline-delimited intended untracked paths")
	var intended repeatedString
	flags.Var(&intended, "intended-untracked", "repository-relative intended untracked path; repeatable")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review-validate argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*cwd) == "" || strings.TrimSpace(*receiptPath) == "" {
		return errors.New("review-validate requires --cwd and --receipt")
	}
	receiptPayload, err := os.ReadFile(*receiptPath)
	if err != nil {
		return fmt.Errorf("read review receipt: %w", err)
	}
	if reviewtransaction.CompactReceiptSchemaOf(receiptPayload) == reviewtransaction.CompactReceiptSchema {
		if strings.TrimSpace(*requestPath) != "" {
			return errors.New("compact review receipts require native authority flags")
		}
		compactReceipt, err := reviewtransaction.ParseCompactReceipt(receiptPayload)
		if err != nil {
			return fmt.Errorf("parse compact review receipt: %w", err)
		}
		manifestPaths, err := readIntendedManifest(*manifest)
		if err != nil {
			return err
		}
		intended = append(intended, manifestPaths...)
		evaluation := reviewtransaction.EvaluateCompactGate(context.Background(), *cwd, compactReceipt, reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GateKind(*gate), LineageID: *lineage, BundleArtifact: *bundlePath,
			PolicyArtifact: *policyPath, LedgerArtifact: *ledgerPath, FixDeltaArtifact: *fixDeltaPath, EvidenceArtifact: *evidencePath,
			IntendedUntracked: []string(intended), BaseRef: *baseRef, PrePRCIAttestation: *ciAttestation,
			ReleaseConfiguration: *releaseConfiguration, ReleaseGenerated: *releaseGenerated, ReleaseProvenance: *releaseProvenance,
			ReleasePublicationBoundary: *releaseBoundary, ReleaseEvidenceFreshness: *releaseFreshness,
		})
		result := ReviewValidateResult{
			Schema: ReviewValidateSchema, Result: evaluation.Result, Allowed: evaluation.Result == reviewtransaction.GateAllow,
			Action: reviewGateAction(evaluation.Result), Reason: evaluation.Reason, Context: evaluation.Context,
		}
		if err := encodeReviewJSON(stdout, result); err != nil {
			return err
		}
		if !result.Allowed {
			return ReviewGateDeniedError{Result: result.Result}
		}
		return nil
	}
	receipt, err := reviewtransaction.ParseReceipt(receiptPayload)
	if err != nil {
		return fmt.Errorf("parse review receipt: %w", err)
	}
	nativeFlags := map[string]bool{}
	flags.Visit(func(current *flag.Flag) {
		switch current.Name {
		case "cwd", "receipt", "request":
		default:
			nativeFlags[current.Name] = true
		}
	})
	var request reviewtransaction.GateRequest
	if strings.TrimSpace(*requestPath) != "" {
		if len(nativeFlags) != 0 {
			return errors.New("review-validate --request mode cannot be combined with native request flags")
		}
		requestPayload, err := os.ReadFile(*requestPath)
		if err != nil {
			return fmt.Errorf("read review gate request: %w", err)
		}
		request, err = reviewtransaction.ParseGateRequest(requestPayload)
		if err != nil {
			return fmt.Errorf("parse review gate request: %w", err)
		}
	} else {
		if strings.TrimSpace(*lineage) == "" || strings.TrimSpace(*gate) == "" {
			return errors.New("review-validate native mode requires --lineage and --gate")
		}
		manifestPaths, err := readIntendedManifest(*manifest)
		if err != nil {
			return err
		}
		intended = append(intended, manifestPaths...)
		request, err = reviewtransaction.BuildNativeGateRequest(context.Background(), *cwd, reviewtransaction.NativeGateRequestInput{
			Gate: reviewtransaction.GateKind(*gate), LineageID: *lineage, BundleArtifact: *bundlePath,
			PolicyArtifact: *policyPath, LedgerArtifact: *ledgerPath, FixDeltaArtifact: *fixDeltaPath, EvidenceArtifact: *evidencePath,
			IntendedUntracked: []string(intended), BaseRef: *baseRef, PrePRCIAttestation: *ciAttestation,
			ReleaseConfiguration: *releaseConfiguration, ReleaseGenerated: *releaseGenerated, ReleaseProvenance: *releaseProvenance,
			ReleasePublicationBoundary: *releaseBoundary, ReleaseEvidenceFreshness: *releaseFreshness,
		})
		if err != nil {
			return fmt.Errorf("build native review gate request: %w", err)
		}
		if strings.TrimSpace(*requestOut) != "" {
			if err := writeCanonicalReviewJSON(*requestOut, request); err != nil {
				return fmt.Errorf("write canonical review gate request: %w", err)
			}
		}
	}
	evaluation := reviewtransaction.EvaluateNativeGate(context.Background(), *cwd, receipt, request)
	result := ReviewValidateResult{
		Schema: ReviewValidateSchema, Result: evaluation.Result, Allowed: evaluation.Result == reviewtransaction.GateAllow,
		Action: reviewGateAction(evaluation.Result), Reason: evaluation.Reason, Context: evaluation.Context,
	}
	if err := encodeReviewJSON(stdout, result); err != nil {
		return err
	}
	if !result.Allowed {
		return ReviewGateDeniedError{Result: result.Result}
	}
	return nil
}

func writeCanonicalReviewJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o644)
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
		return "explicit-maintainer-action"
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
