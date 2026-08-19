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

type reviewIntendedUntrackedSubmissionContext struct {
	Selector reviewTransitionSelector
	Runtime  string
	Lineage  string
}

func reviewIntendedUntrackedDeclared(mode reviewSingleValueFlag, selected reviewRepeatedPathFlag, digest reviewSingleValueFlag) bool {
	return mode.set || len(selected) != 0 || digest.set
}

func reviewIntendedUntrackedScopeForTarget(ctx context.Context, builder reviewtransaction.SnapshotBuilder, mode reviewSingleValueFlag, selected reviewRepeatedPathFlag, expectedDigest reviewSingleValueFlag) (reviewIntendedUntrackedScope, error) {
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
		return reviewIntendedUntrackedScope{}, errors.New("untracked selection requires --untracked-scope and --expected-untracked-inventory; rerun `gentle-ai review status --next-transition`")
	}
	switch mode.value {
	case "exclude":
		if len(selected) != 0 {
			return reviewIntendedUntrackedScope{}, errors.New("--untracked-scope=exclude does not accept --intended-untracked; rerun `gentle-ai review start --untracked-scope=select`")
		}
	case "select":
		if len(selected) == 0 {
			return reviewIntendedUntrackedScope{}, errors.New("--untracked-scope=select requires at least one --intended-untracked; rerun `gentle-ai review start --untracked-scope=exclude`")
		}
	default:
		return reviewIntendedUntrackedScope{}, fmt.Errorf("--untracked-scope must be exclude or select, got %q; rerun `gentle-ai review status --next-transition`", mode.value)
	}
	intended, err := builder.ValidateIntendedUntrackedSelection(ctx, expectedDigest.value, selected)
	if err != nil {
		return reviewIntendedUntrackedScope{}, err
	}
	scope.Digest, scope.Intended = expectedDigest.value, intended
	return scope, nil
}

func reviewIntendedUntrackedSelectionRequired(scope reviewIntendedUntrackedScope) error {
	return fmt.Errorf("untracked files require an explicit declaration; run `gentle-ai review status --next-transition`, then rerun with --untracked-scope=exclude --expected-untracked-inventory=%s or --untracked-scope=select --intended-untracked=<repo-relative-path> --expected-untracked-inventory=%s", scope.Digest, scope.Digest)
}

func reviewIntendedUntrackedCollection(status ReviewTargetStatusResult, scope reviewIntendedUntrackedScope) ReviewNextTransition {
	paths, _ := json.Marshal(scope.Inventory)
	return reviewCollectTransition("intended_untracked_selection_required", ReviewTransitionInput{
		Name: "intended_untracked_selection", Schema: reviewIntendedUntrackedSelectionSchema,
		CaptureOperation: "external.select_intended_untracked",
		Arguments: append(reviewTargetArguments(status),
			ReviewTransitionArgument{Name: "eligible_paths_json", Value: string(paths)},
			ReviewTransitionArgument{Name: "expected_untracked_inventory", Value: scope.Digest}),
		Submission: reviewIntendedUntrackedSubmission(status.Contract, scope.Digest, status.intendedUntrackedSubmission),
	})
}

func reviewIntendedUntrackedSubmission(contract, digest string, context reviewIntendedUntrackedSubmissionContext) *ReviewTransitionSubmission {
	if contract != ReviewIntegrationContractV2 || context.Selector.Projection != reviewtransaction.ProjectionWorkspace {
		return nil
	}

	argumentTokens := []string{
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "contract", Value: contract}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "next_transition", Value: "true"}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "cwd", Value: "{{cwd}}"}),
	}
	if strings.TrimSpace(context.Runtime) != "" {
		argumentTokens = append(argumentTokens, reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "agent", Value: context.Runtime}))
	}
	if strings.TrimSpace(context.Lineage) != "" {
		argumentTokens = append(argumentTokens, reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "lineage", Value: context.Lineage}))
	}
	argumentTokens = append(argumentTokens, reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "projection", Value: string(context.Selector.Projection)}))
	switch context.Selector.Kind {
	case reviewtransaction.TargetCurrentChanges:
	case reviewtransaction.TargetBaseDiff:
		if strings.TrimSpace(context.Selector.BaseRef) == "" || context.Selector.BaseTree != "" || context.Selector.WorkspaceOverlay {
			return nil
		}
		argumentTokens = append(argumentTokens,
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "base_ref", Value: context.Selector.BaseRef}),
			reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "committed_only", Value: "true"}),
		)
	case reviewtransaction.TargetBaseWorkspaceOverlay:
		if !context.Selector.WorkspaceOverlay || context.Selector.BaseRef == "" && context.Selector.BaseTree == "" ||
			context.Selector.BaseRef != "" && context.Selector.BaseTree != "" {
			return nil
		}
		if context.Selector.BaseRef != "" {
			argumentTokens = append(argumentTokens, reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "base_ref", Value: context.Selector.BaseRef}))
		} else {
			argumentTokens = append(argumentTokens, reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "base_tree", Value: context.Selector.BaseTree}))
		}
		argumentTokens = append(argumentTokens, reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "workspace_overlay", Value: "true"}))
	default:
		return nil
	}

	scopeLocation := len(argumentTokens)
	argumentTokens = append(argumentTokens,
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "untracked_scope", Value: "{{untracked_scope}}"}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "expected_untracked_inventory", Value: digest}),
		reviewTransitionArgumentToken(ReviewTransitionArgument{Name: "intended_untracked", Value: "{{intended_untracked}}"}),
	)
	return &ReviewTransitionSubmission{
		OperationToken: "status",
		ArgumentTokens: argumentTokens,
		Values: []ReviewTransitionSubmissionValue{
			{Slot: "cwd", Domain: "repository_path", SubstitutionLocation: 2},
			{Slot: "untracked_scope", Domain: "enum", AllowedValues: []string{"select", "exclude"}, SubstitutionLocation: scopeLocation},
			{Slot: "intended_untracked", Domain: "repo_relative_path", Schema: reviewIntendedUntrackedSelectionSchema, SubstitutionLocation: scopeLocation + 2, Repeated: true},
		},
	}
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
