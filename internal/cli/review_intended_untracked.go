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

func (value *reviewSingleValueFlag) String() string { return value.value }
func (value *reviewSingleValueFlag) Set(input string) error {
	if value.set {
		return errors.New("untracked selection flag may only be specified once") // refusal:by-design operator-knowledge: the caller must rerun its command with one unambiguous value
	}
	value.value, value.set = input, true
	return nil
}

type reviewIntendedUntrackedScope struct {
	Inventory, Intended      []string
	Digest                   string
	NeedsSelection, Declared bool
}

func reviewIntendedUntrackedScopeForTarget(ctx context.Context, builder reviewtransaction.SnapshotBuilder, mode reviewSingleValueFlag, selected reviewRepeatedPathFlag, expected reviewSingleValueFlag) (reviewIntendedUntrackedScope, error) {
	inventory, digest, err := builder.IntendedUntrackedInventory(ctx)
	if err != nil {
		return reviewIntendedUntrackedScope{}, err
	}
	scope := reviewIntendedUntrackedScope{Inventory: inventory, Intended: []string{}, Digest: digest, Declared: mode.set || len(selected) > 0 || expected.set}
	if !scope.Declared {
		scope.NeedsSelection = len(inventory) > 0
		return scope, nil
	}
	if !mode.set || !expected.set {
		return scope, errors.New("untracked selection requires --untracked-scope and --expected-untracked-inventory; rerun `gentle-ai review status --next-transition`")
	}
	switch mode.value {
	case "exclude":
		if len(selected) > 0 {
			return scope, errors.New("--untracked-scope=exclude does not accept --intended-untracked; rerun `gentle-ai review start --untracked-scope=exclude` without selected paths")
		}
	case "select":
		if len(selected) == 0 {
			return scope, errors.New("--untracked-scope=select requires --intended-untracked; rerun `gentle-ai review status --next-transition` to select a path")
		}
	default:
		return scope, fmt.Errorf("--untracked-scope must be exclude or select, got %q; rerun `gentle-ai review status --next-transition`", mode.value)
	}
	scope.Intended, err = builder.ValidateIntendedUntrackedSelection(ctx, expected.value, selected)
	return scope, err
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
	arguments := []ReviewTransitionArgument{{Name: "untracked-scope", Value: "exclude"}}
	if len(scope.Intended) > 0 {
		arguments[0].Value = "select"
	}
	for _, path := range scope.Intended {
		arguments = append(arguments, ReviewTransitionArgument{Name: "intended-untracked", Value: path})
	}
	return append(arguments, ReviewTransitionArgument{Name: "expected-untracked-inventory", Value: scope.Digest})
}
