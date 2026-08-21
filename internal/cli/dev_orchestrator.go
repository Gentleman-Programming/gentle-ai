package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/batch"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/devjournal"
	"github.com/gentleman-programming/gentle-ai/v2/internal/devorchestrator/intent"
	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

// ErrEngramArtifactStoreUnsupported is returned by route/dispatch when the
// resolved SDD artifact store is Engram: intent.Router only writes to the
// filesystem, so status alone stays allowed (it is read-only).
var ErrEngramArtifactStoreUnsupported = errors.New("dev-orchestrator: route/dispatch are unsupported when the SDD artifact store is engram")

// RunDevOrchestrator is the CLI entry point for
// `gentle-ai dev-orchestrator <operation>`. Always compiled: the
// `--dev-orchestrator` install opt-in only selects overlay packaging.
func RunDevOrchestrator(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("dev-orchestrator requires an operation: status, route, context, or dispatch")
	}
	operation, rest := args[0], args[1:]
	switch operation {
	case "status":
		return runDevOrchestratorStatus(rest, stdout)
	case "route":
		return runDevOrchestratorRoute(rest, stdout)
	case "context":
		return runDevOrchestratorContext(rest, stdout)
	case "dispatch":
		return runDevOrchestratorDispatch(rest, stdout)
	default:
		return fmt.Errorf("unknown dev-orchestrator operation %q; want one of status, route, context, or dispatch", operation)
	}
}

// devOrchestratorProjection resolves and projects the SDD status for change
// under cwd, so callers can inspect ArtifactStore before any write.
func devOrchestratorProjection(cwd, change string) (sddstatus.StatusV1Projection, error) {
	status, err := sddstatus.Resolve(sddstatus.ResolveOptions{
		CWD: cwd, ChangeName: change, ReviewDisabledForWorkspace: sddReviewDisabledForWorkspace,
	})
	if err != nil {
		return sddstatus.StatusV1Projection{}, fmt.Errorf("resolve sdd status: %w", err)
	}
	projection, err := sddstatus.ProjectStatusV1(status)
	if err != nil {
		return sddstatus.StatusV1Projection{}, fmt.Errorf("project sdd status: %w", err)
	}
	return projection, nil
}

// refuseEngramWrite is the pure decision behind the spec's Engram-mode write
// refusal: route and dispatch call it after projecting status; status never
// does, because it is read-only and unaffected (spec scenarios "Write
// operations refuse..." / "Read-only status remains allowed...").
func refuseEngramWrite(projection sddstatus.StatusV1Projection) error {
	if projection.ArtifactStore == sddstatus.ArtifactStoreEngram {
		return fmt.Errorf("%w: intent.Router writes only to openspec/changes/... on disk", ErrEngramArtifactStoreUnsupported)
	}
	return nil
}

func runDevOrchestratorStatus(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("dev-orchestrator status", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "required; repository working directory")
	change := flags.String("change", "", "required; SDD change identifier")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("dev-orchestrator status requires --cwd")
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("dev-orchestrator status requires --change")
	}

	projection, err := devOrchestratorProjection(*cwd, *change)
	if err != nil {
		return err
	}
	batches := devorchestrator.New(*cwd).PrepareBatches(projection, "")
	store, loaded, err := devOrchestratorJournal(*cwd, *change)
	if err != nil {
		return err
	}
	return encodeDevOrchestratorResult(stdout, map[string]any{
		"artifactStore":   projection.ArtifactStore,
		"nextRecommended": projection.NextRecommended,
		"batches":         batches,
		"journal":         loaded.Record,
		"journalPath":     store.Path(),
		"journalFallback": store.UsesFallback(),
	})
}

func runDevOrchestratorRoute(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("dev-orchestrator route", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "required; repository working directory")
	intentText := flags.String("intent", "", "required; raw intent text")
	source := flags.String("source", "", "optional; source identifier for the change")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("dev-orchestrator route requires --cwd")
	}
	if strings.TrimSpace(*intentText) == "" {
		return errors.New("dev-orchestrator route requires --intent")
	}

	projection, err := devOrchestratorProjection(*cwd, intent.NormalizeChangeID(*source))
	if err != nil {
		return err
	}
	if err := refuseEngramWrite(projection); err != nil {
		return err
	}

	result, err := devorchestrator.New(*cwd).RouteIntent(*intentText, *source)
	if err != nil {
		return err
	}
	return encodeDevOrchestratorResult(stdout, result)
}

func runDevOrchestratorContext(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("dev-orchestrator context", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "required; repository working directory")
	agentName := flags.String("agent", "", "required; agent contract name")
	artifact := flags.String("artifact", "", "required; workspace-relative primary artifact path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("dev-orchestrator context requires --cwd")
	}
	if strings.TrimSpace(*agentName) == "" {
		return errors.New("dev-orchestrator context requires --agent")
	}
	if strings.TrimSpace(*artifact) == "" {
		return errors.New("dev-orchestrator context requires --artifact")
	}

	// repos/architecture/skills/source-artifact/expected-* are left for a
	// later slice: every function they would feed is already reachable
	// through this call regardless of their values (H-01/D8).
	prompt, err := devorchestrator.New(*cwd).GenerateAgentPrompt(
		"dev-orchestrator-context", *agentName, *artifact, nil, "", nil, "", "", "", "",
	)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, prompt)
	return err
}

func runDevOrchestratorDispatch(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("dev-orchestrator dispatch", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "required; repository working directory")
	change := flags.String("change", "", "required; SDD change identifier")
	agentName := flags.String("agent", "", "required; default agent for generated batches")
	maxWorkers := flags.Int("max-workers", 1, "optional; concurrent dispatch worker limit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*cwd) == "" {
		return errors.New("dev-orchestrator dispatch requires --cwd")
	}
	if strings.TrimSpace(*change) == "" {
		return errors.New("dev-orchestrator dispatch requires --change")
	}
	if strings.TrimSpace(*agentName) == "" {
		return errors.New("dev-orchestrator dispatch requires --agent")
	}
	if err := intent.ValidateIdentifier(*change); err != nil {
		return err
	}

	projection, err := devOrchestratorProjection(*cwd, *change)
	if err != nil {
		return err
	}
	if err := refuseEngramWrite(projection); err != nil {
		return err
	}

	orch := devorchestrator.New(*cwd)
	batches := orch.PrepareBatches(projection, *agentName)
	prompts := make(map[string]string, len(batches))
	for _, b := range batches {
		prompts[b.RepoName] = fmt.Sprintf("dev-orchestrator dispatch: repo=%q agent=%q change=%q", b.RepoName, b.AgentName, *change)
	}
	// dispatch ships no subprocess in this change (design D3): the runner
	// only emits prompts and journals outcome "planned", while ExecuteBatches
	// is genuinely invoked so the whole facade is reachable.
	runner := &plannedAgentRunner{stdout: stdout}
	dispatchErrors := orch.ExecuteBatches(context.Background(), batches, prompts, runner, *maxWorkers)

	store, loaded, err := devOrchestratorJournal(*cwd, *change)
	if err != nil {
		return err
	}
	// Written exactly once here, after ExecuteBatches' wg.Wait() has already
	// returned -- never from a worker goroutine (design D1).
	record := devjournal.Record{
		Schema: devjournal.SchemaV1, Change: *change, UpdatedAt: time.Now().UTC(),
		StatusDigest: devOrchestratorStatusDigest(projection),
		Dispatches:   devOrchestratorDispatchRecords(batches, *agentName, dispatchErrors),
	}
	if err := store.Save(record, loaded.Revision); err != nil {
		return fmt.Errorf("save dev-orchestrator journal: %w", err)
	}

	return encodeDevOrchestratorResult(stdout, map[string]any{
		"batches": batches, "errors": dispatchErrors, "journal": record,
		"journalPath": store.Path(), "journalFallback": store.UsesFallback(),
	})
}

// plannedAgentRunner implements executor.AgentRunner as dispatch's
// in-process writer (design D3): it emits each generated prompt to stdout
// and always succeeds, deferring the subprocess/agent-invocation boundary to
// a follow-up change.
type plannedAgentRunner struct {
	stdout io.Writer
}

func (r *plannedAgentRunner) Run(_ context.Context, b batch.ExecutionBatch, prompt string) error {
	_, err := fmt.Fprintf(r.stdout, "planned dispatch: repo=%q agent=%q\n%s\n", b.RepoName, b.AgentName, prompt)
	return err
}

// devOrchestratorJournal opens and loads change's journal, returning the
// Store (dispatch needs it for Save; status reports Path/UsesFallback --
// design D1's fallback disclosure) alongside the loaded record.
func devOrchestratorJournal(cwd, change string) (devjournal.Store, devjournal.Loaded, error) {
	store, err := devjournal.Open(cwd, change)
	if err != nil {
		return devjournal.Store{}, devjournal.Loaded{}, fmt.Errorf("open dev-orchestrator journal: %w", err)
	}
	loaded, err := store.Load()
	if err != nil {
		return devjournal.Store{}, devjournal.Loaded{}, fmt.Errorf("load dev-orchestrator journal: %w", err)
	}
	return store, loaded, nil
}

func devOrchestratorDispatchRecords(batches []batch.ExecutionBatch, agentName string, dispatchErrors map[string]error) []devjournal.Dispatch {
	now := time.Now().UTC()
	dispatches := make([]devjournal.Dispatch, 0, len(batches))
	for _, b := range batches {
		if !b.Ready {
			continue
		}
		outcome, errMsg := devjournal.OutcomePlanned, ""
		if dispatchErr, failed := dispatchErrors[b.RepoName]; failed {
			outcome, errMsg = devjournal.OutcomeFailed, dispatchErr.Error()
		}
		dispatches = append(dispatches, devjournal.Dispatch{
			RepoSlug: b.RepoName, Agent: agentName, Attempt: 1, Outcome: outcome,
			Error: errMsg, StartedAt: now, FinishedAt: now,
		})
	}
	return dispatches
}

func devOrchestratorStatusDigest(projection sddstatus.StatusV1Projection) string {
	data, err := json.Marshal(projection)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeDevOrchestratorResult(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
