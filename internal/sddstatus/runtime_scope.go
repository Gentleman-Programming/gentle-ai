package sddstatus

import (
	"context"
	"errors"
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

// ResolveVerifyReportAuthority derives slice report totals from the
// provider-owned runtime objective. Whole-change totals remain CLI input.
func ResolveVerifyReportAuthority(ctx context.Context, cwd, change, sliceID string) (SpecCounts, *RuntimeObjective, error) {
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
}
