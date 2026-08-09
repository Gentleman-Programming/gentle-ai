package sddstatus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
)

// RuntimeScope is the provider-owned assignment for one runtime objective.
// It is deliberately metadata on the objective rather than a separate registry:
// the objective record and identity are the only authority for a slice.
type RuntimeScope struct {
	Tasks        []string `json:"tasks"`
	Requirements []string `json:"requirements"`
	Scenarios    []string `json:"scenarios"`
}

func (scope RuntimeScope) counts() SpecCounts {
	return SpecCounts{Requirements: len(scope.Requirements), Scenarios: len(scope.Scenarios)}
}

func normalizeRuntimeScope(scope *RuntimeScope) (*RuntimeScope, error) {
	if scope == nil {
		return nil, nil
	}
	normalized := RuntimeScope{
		Tasks:        append([]string(nil), scope.Tasks...),
		Requirements: append([]string(nil), scope.Requirements...),
		Scenarios:    append([]string(nil), scope.Scenarios...),
	}
	if len(normalized.Tasks) == 0 {
		return nil, errors.New("scoped objective requires at least one assigned task") // refusal:by-design world-action: provider-owned scope construction must assign a task before it can create authority
	}
	for _, assignments := range [][]string{normalized.Tasks, normalized.Requirements, normalized.Scenarios} {
		seen := make(map[string]struct{}, len(assignments))
		for _, assignment := range assignments {
			if err := validateRuntimeText(assignment, 160); err != nil {
				return nil, errors.New("invalid scoped objective assignment") // refusal:by-design world-action: the authority constructor must supply canonical assignment text
			}
			if _, duplicate := seen[assignment]; duplicate {
				return nil, errors.New("duplicate scoped objective assignment") // refusal:by-design world-action: the authority constructor must make every scoped assignment unique
			}
			seen[assignment] = struct{}{}
		}
	}
	return &normalized, nil
}

func runtimeScopeEqual(left, right *RuntimeScope) bool {
	return reflect.DeepEqual(left, right)
}

func runtimeScopeOverlaps(existing []RuntimeScope, proposed *RuntimeScope) bool {
	if proposed == nil {
		return false
	}
	for _, prior := range existing {
		for _, pair := range [][2][]string{
			{prior.Tasks, proposed.Tasks},
			{prior.Requirements, proposed.Requirements},
			{prior.Scenarios, proposed.Scenarios},
		} {
			seen := make(map[string]struct{}, len(pair[0]))
			for _, value := range pair[0] {
				seen[value] = struct{}{}
			}
			for _, value := range pair[1] {
				if _, overlap := seen[value]; overlap {
					return true
				}
			}
		}
	}
	return false
}

// ResolveVerifyReportAuthority derives report totals from the provider-owned
// objective or from the change's actual specifications. Callers never supply
// totals for either path.
func ResolveVerifyReportAuthority(ctx context.Context, cwd, change, scope, sliceID string) (SpecCounts, *RuntimeObjective, error) {
	if scope == "" {
		scope = "whole"
	}
	switch scope {
	case "whole":
		if sliceID != "" {
			return SpecCounts{}, nil, errors.New("whole verification does not accept a slice_id") // refusal:by-design operator-knowledge: only the caller can choose whole verification rather than a provider-owned slice
		}
		root := filepath.Join(cwd, "openspec", "changes", change, "specs")
		contents := []string{}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			contents = append(contents, string(content))
			return nil
		})
		if err != nil || len(contents) == 0 {
			return SpecCounts{}, nil, errors.New("whole verification cannot read authoritative change specifications") // refusal:by-design world-action: authoritative specifications must exist and be readable before their totals can be derived
		}
		return countSpecRequirementsAndScenarios(contents), nil, nil
	case "slice":
		if sliceID == "" {
			return SpecCounts{}, nil, errors.New("slice verification requires a slice_id") // refusal:by-design operator-knowledge: only the caller can name the provider-owned slice it intends to verify
		}
		store, err := OpenRuntimeStore(ctx, cwd, change)
		if err != nil {
			return SpecCounts{}, nil, err
		}
		status, err := store.Status()
		if err != nil {
			return SpecCounts{}, nil, err
		}
		if status.Objective == nil || status.Objective.Scope == nil || status.Objective.ID != sliceID {
			return SpecCounts{}, nil, errors.New("slice verification does not match the provider-owned runtime objective") // refusal:by-design operator-knowledge: only the caller can select the intended current provider-owned slice
		}
		objective := *status.Objective
		return objective.Scope.counts(), &objective, nil
	default:
		return SpecCounts{}, nil, errors.New("verification scope must be whole or slice") // refusal:by-design operator-knowledge: only the caller can choose whether its report verifies a whole change or one slice
	}
}
