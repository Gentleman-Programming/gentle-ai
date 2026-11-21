package cli

import (
	"context"
	"fmt"
	"io"
	"os/user"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/pathquote"
	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// ReviewAbandonResult is the JSON `review abandon` prints. Record is the
// requested lineage's own quarantine; Cascaded lists every other entry
// quarantined with it (recovery successors, deepest first, and a same-lineage
// legacy-v1 entry).
type ReviewAbandonResult struct {
	Operation string                                   `json:"operation"`
	Record    reviewtransaction.CompactReclaimRecord   `json:"record"`
	Cascaded  []reviewtransaction.CompactReclaimRecord `json:"cascaded,omitempty"`
}

// reviewAbandonLineageRefusal answers a flagless `review abandon`, which is the
// one form the negotiated STATUS continuation can carry: it names every
// lineage in the repository's authority store with its status, and the exact
// command that abandons one. It writes nothing.
func reviewAbandonLineageRefusal(ctx context.Context, cwd string) error {
	message := "review abandon requires --lineage: the review lineage to declare lost and quarantine"
	root, err := (reviewtransaction.SnapshotBuilder{Repo: cwd}).ResolveRepositoryRoot(ctx)
	if err != nil {
		return fmt.Errorf("%s. Run `gentle-ai review abandon --cwd %s --lineage <lineage_id>`", message, pathquote.Quote(cwd))
	}
	report, err := reviewtransaction.InventoryAuthority(ctx, root)
	if err != nil || len(report.Entries) == 0 {
		return fmt.Errorf("%s. Run `gentle-ai review abandon --cwd %s --lineage <lineage_id>`; `gentle-ai review status --cwd %s` lists the lineages", message, pathquote.Quote(root), pathquote.Quote(root))
	}
	lines := make([]string, 0, len(report.Entries))
	for _, entry := range report.Entries {
		if entry.LineageID == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("gentle-ai review abandon --cwd %s --lineage %s", pathquote.Quote(root), entry.LineageID))
	}
	if len(lines) == 0 {
		return fmt.Errorf("%s. Run `gentle-ai review abandon --cwd %s --lineage <lineage_id>`; `gentle-ai review status --cwd %s` lists the lineages", message, pathquote.Quote(root), pathquote.Quote(root))
	}
	return fmt.Errorf("%s. Run `gentle-ai review abandon --cwd <repo> --lineage <lineage_id>` for the one to abandon, i.e. one of:\n%s\nAbandoning a lineage also abandons its recovery successors; every entry is moved to the audited quarantine, never deleted", message, strings.Join(lines, "\n"))
}

// reviewAbandonDefaultActor names who ran the abandonment when --actor is not
// given: the OS account, since abandonment no longer carries an authorization
// binding naming a maintainer.
func reviewAbandonDefaultActor() string {
	if account, err := user.Current(); err == nil && strings.TrimSpace(account.Username) != "" {
		return strings.TrimSpace(account.Username)
	}
	return "operator"
}

// reviewAbandonRetiredFlags are the inputs the gated abandon accepted before
// abandonment dropped its authorization binding. They are refused by name
// rather than ignored, so a stale caller learns the new form instead of
// believing a binding it supplied was honored.
var reviewAbandonRetiredFlags = []string{"expected-revision", "maintainer-authorization"}

func reviewAbandonRetiredFlagRefusal(args []string) error {
	retired := []string{}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name := strings.TrimLeft(arg, "-")
		if name == arg {
			continue
		}
		name, _, _ = strings.Cut(name, "=")
		for _, flag := range reviewAbandonRetiredFlags {
			if name == flag {
				retired = append(retired, "--"+flag)
			}
		}
	}
	if len(retired) == 0 {
		return nil
	}
	return fmt.Errorf("review abandon no longer takes %s or an authorization binding; nothing was changed. Run: gentle-ai review abandon --cwd <repo> --lineage <id>", strings.Join(retired, " or "))
}

func RunReviewAbandon(args []string, stdout io.Writer) error {
	if !reviewHelpRequested(args) {
		if err := reviewAbandonRetiredFlagRefusal(args); err != nil {
			return err
		}
	}
	flags := newReviewFlagSet("review abandon", stdout, "Declare one review lineage lost and move it to the audited quarantine. Accepts every state: terminal (approved, escalated, invalidated), unreadable or missing review-state.json, and a legacy-v1 entry. Recovery successors of the lineage are abandoned with it, deepest first, so no orphan recovery edge remains; ancestors and unrelated lineages are never touched. Nothing is deleted and no authorization binding is required. The v2 store lock is taken without waiting: a live holder fails the command with the lock-busy error and nothing moves. An exact replay of a committed abandonment converges idempotently. On partial failure the prepared audit record JSON is still emitted to stdout and the command exits non-zero.")
	cwd := flags.String("cwd", ".", "repository path")
	lineage := flags.String("lineage", "", "review lineage to abandon")
	reason := flags.String("reason", "", "abandonment reason recorded in the audit record (default: "+reviewtransaction.CompactAbandonReasonOperatorDisposition+")")
	actor := flags.String("actor", "", "abandonment actor recorded in the audit record (default: the OS account)")
	if err := parseReviewFlags(flags, args); err != nil {
		return err
	}
	if reviewHelpRequested(args) {
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected review abandon argument %q", flags.Arg(0))
	}
	if strings.TrimSpace(*lineage) == "" {
		return reviewAbandonLineageRefusal(context.Background(), *cwd)
	}
	if strings.TrimSpace(*reason) == "" {
		*reason = reviewtransaction.CompactAbandonReasonOperatorDisposition
	}
	if strings.TrimSpace(*actor) == "" {
		*actor = reviewAbandonDefaultActor()
	}
	root, err := resolveReviewOperationRoot(context.Background(), *cwd, reviewtransaction.RDDOperationAbandon)
	if err != nil {
		return withPlainReviewContinuation(fmt.Errorf("resolve review repository root: %w", err))
	}
	result, err := reviewtransaction.AbandonCompactStore(context.Background(), root, reviewtransaction.CompactAbandonRequest{
		LineageID: *lineage, Reason: *reason, Actor: *actor,
	})
	output := ReviewAbandonResult{Operation: "review/abandon", Record: result.CompactReclaimRecord, Cascaded: result.Cascaded}
	if err != nil {
		// A partial abandonment persisted the prepared audit record and may have
		// moved the entry; surface the quarantine location for reconciliation.
		if result.QuarantinePath != "" || len(result.Cascaded) != 0 {
			_ = encodeReviewJSON(stdout, output)
		}
		return err
	}
	return encodeReviewJSON(stdout, output)
}
