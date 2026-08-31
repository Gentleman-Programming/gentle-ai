package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
)

const reviewIntendedUntrackedSelectionSchema = "gentle-ai.review-intended-untracked-selection/v1"

// reviewIntendedUntrackedInventoryCommand is the runnable form of the STATUS
// that publishes the canonical untracked inventory. `--next-transition` is
// refused without a negotiated contract and runtime identity, so a refusal
// that names the bare form sends the operator to a command that fails
// (issue #2895).
const reviewIntendedUntrackedInventoryCommand = "gentle-ai review status --cwd <repo> --contract " + ReviewIntegrationContractV2 + " --agent <runtime> --next-transition"

type reviewRepeatedPathFlag []string

func (paths *reviewRepeatedPathFlag) String() string { return strings.Join(*paths, "\n") }
func (paths *reviewRepeatedPathFlag) Set(value string) error {
	*paths = append(*paths, value)
	return nil
}

type reviewSingleValueFlag struct {
	value string
	set   bool
}

func (flag *reviewSingleValueFlag) String() string { return flag.value }
func (flag *reviewSingleValueFlag) Set(value string) error {
	if flag.set {
		return errors.New("untracked scope flags may only be specified once; rerun gentle-ai review start with one declaration")
	}
	flag.value, flag.set = value, true
	return nil
}

type reviewIntendedUntrackedScope struct {
	Inventory, Intended      []string
	Digest                   string
	NeedsSelection, Declared bool
}

func reviewIntendedUntrackedDeclared(mode reviewSingleValueFlag, selected reviewRepeatedPathFlag, digest reviewSingleValueFlag) bool {
	return mode.set || len(selected) != 0 || digest.set
}

func reviewIntendedUntrackedScopeForTarget(ctx context.Context, builder reviewtransaction.SnapshotBuilder, mode reviewSingleValueFlag, selected reviewRepeatedPathFlag, expectedDigest reviewSingleValueFlag) (reviewIntendedUntrackedScope, error) {
	return intendedUntrackedScopeForTarget(ctx, builder, mode, selected, expectedDigest, reviewIntendedUntrackedInventoryCommand, "gentle-ai review start")
}

func intendedUntrackedScopeForTarget(ctx context.Context, builder reviewtransaction.SnapshotBuilder, mode reviewSingleValueFlag, selected reviewRepeatedPathFlag, expectedDigest reviewSingleValueFlag, inventoryCommand, selectionCommand string) (reviewIntendedUntrackedScope, error) {
	inventory, digest, err := builder.IntendedUntrackedInventory(ctx)
	if err != nil {
		return reviewIntendedUntrackedScope{}, err
	}
	scope := reviewIntendedUntrackedScope{Inventory: inventory, Intended: []string{}, Digest: digest, Declared: reviewIntendedUntrackedDeclared(mode, selected, expectedDigest)}
	if len(inventory) == 0 && !scope.Declared {
		return scope, nil
	}
	if !scope.Declared {
		scope.NeedsSelection = true
		return scope, nil
	}
	if !mode.set || !expectedDigest.set {
		// refusal:by-design operator-knowledge: only the caller can choose whether to exclude or select the current untracked population.
		return reviewIntendedUntrackedScope{}, fmt.Errorf("untracked selection requires --untracked-scope and --expected-untracked-inventory; run `%s` to obtain the canonical inventory, then rerun `%s`", inventoryCommand, selectionCommand)
	}
	switch mode.value {
	case "exclude":
		if len(selected) != 0 {
			// refusal:by-design operator-knowledge: only the caller can decide whether the named paths should be selected or the population excluded.
			return reviewIntendedUntrackedScope{}, fmt.Errorf("--untracked-scope=exclude does not accept --intended-untracked; run `%s` to refresh the canonical inventory, then rerun `%s --untracked-scope=select`", inventoryCommand, selectionCommand)
		}
	case "select":
		if len(selected) == 0 {
			// refusal:by-design operator-knowledge: only the caller knows which eligible paths it intends to include.
			return reviewIntendedUntrackedScope{}, fmt.Errorf("--untracked-scope=select requires at least one --intended-untracked; run `%s` to refresh the canonical inventory, then rerun `%s --untracked-scope=select --intended-untracked=<repo-relative-path> --expected-untracked-inventory=%s`", inventoryCommand, selectionCommand, digest)
		}
	default:
		// refusal:by-design operator-knowledge: only the caller can choose the intended selection mode for its workspace.
		return reviewIntendedUntrackedScope{}, fmt.Errorf("--untracked-scope must be exclude or select, got %q; run `%s` to obtain the canonical inventory, then rerun `%s`", mode.value, inventoryCommand, selectionCommand)
	}
	intended, err := builder.ValidateIntendedUntrackedSelection(ctx, expectedDigest.value, selected)
	if err != nil {
		return reviewIntendedUntrackedScope{}, err
	}
	scope.Digest, scope.Intended = expectedDigest.value, intended
	return scope, nil
}

func reviewIntendedUntrackedSelectionRequired(scope reviewIntendedUntrackedScope) error {
	return intendedUntrackedSelectionRequired(scope, reviewIntendedUntrackedInventoryCommand, "gentle-ai review start")
}

func intendedUntrackedSelectionRequired(scope reviewIntendedUntrackedScope, inventoryCommand, selectionCommand string) error {
	// refusal:by-design operator-knowledge: only the caller can choose whether to exclude or select the eligible untracked paths.
	return fmt.Errorf("untracked files require an explicit declaration; run `%s` to obtain the canonical inventory, then rerun `%s` with --untracked-scope=exclude --expected-untracked-inventory=%s or --untracked-scope=select --intended-untracked=<repo-relative-path> --expected-untracked-inventory=%s", inventoryCommand, selectionCommand, scope.Digest, scope.Digest)
}

func reviewIntendedUntrackedCollection(status ReviewTargetStatusResult, scope reviewIntendedUntrackedScope) ReviewNextTransition {
	paths, _ := json.Marshal(scope.Inventory)
	return reviewCollectTransition("intended_untracked_selection_required", ReviewTransitionInput{
		Name: "intended_untracked_selection", Schema: reviewIntendedUntrackedSelectionSchema,
		CaptureOperation: "external.select_intended_untracked",
		Arguments: append(reviewTargetArguments(status),
			ReviewTransitionArgument{Name: "eligible_paths_json", Value: string(paths)},
			ReviewTransitionArgument{Name: "expected_untracked_inventory", Value: scope.Digest}),
	})
}

func reviewStartIntendedUntrackedArguments(scope reviewIntendedUntrackedScope) []ReviewTransitionArgument {
	if !scope.Declared {
		return nil
	}
	arguments := []ReviewTransitionArgument{
		{Name: "untracked-scope", Value: map[bool]string{true: "select", false: "exclude"}[len(scope.Intended) != 0]},
		{Name: "expected-untracked-inventory", Value: scope.Digest},
	}
	for _, path := range scope.Intended {
		arguments = append(arguments, ReviewTransitionArgument{Name: "intended-untracked", Value: path})
	}
	return arguments
}
