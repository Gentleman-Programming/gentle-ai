package reviewtransaction

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MaxOracleHistoryEvents = 8
	MaxOracleSearchStates  = 4096
)

var (
	ErrOracleHistoryBound = errors.New("review history exceeds bounded oracle input")
	ErrOracleSearchBound  = errors.New("review history oracle exhausted its search budget")
)

type HistoryOperation string

const (
	HistoryStart     HistoryOperation = "START"
	HistoryFinalize  HistoryOperation = "FINALIZE"
	HistoryStatus    HistoryOperation = "STATUS"
	HistoryValidate  HistoryOperation = "VALIDATE"
	HistoryReconcile HistoryOperation = "RECONCILE"
)

type HistoryAuthority string

const (
	HistoryAbsent    HistoryAuthority = "absent"
	HistoryReviewing HistoryAuthority = "reviewing"
	HistoryApproved  HistoryAuthority = "approved"
)

type HistoryEffect string

const (
	HistoryEffectNone    HistoryEffect = "none"
	HistoryEffectPending HistoryEffect = "pending"
	HistoryEffectApplied HistoryEffect = "applied"
	HistoryEffectBlocked HistoryEffect = "blocked"
)

type HistoryEvent struct {
	InvocationID     string
	Actor            string
	Operation        HistoryOperation
	LineageID        string
	IdempotencyKey   string
	Started          uint64
	Completed        uint64
	ExpectedRevision string
	ObservedRevision string
	Result           string
	BeforeAuthority  HistoryAuthority
	AfterAuthority   HistoryAuthority
	BeforeEffect     HistoryEffect
	AfterEffect      HistoryEffect
}

type historyModel struct {
	authority     HistoryAuthority
	effect        HistoryEffect
	revision      string
	idempotencyID string
}

func initialHistoryModel() historyModel {
	return historyModel{authority: HistoryAbsent, effect: HistoryEffectNone}
}

func (model historyModel) step(event HistoryEvent) (historyModel, bool) {
	if event.BeforeAuthority != model.authority || event.BeforeEffect != model.effect {
		return model, false
	}
	next := model
	switch event.Operation {
	case HistoryStart:
		switch model.authority {
		case HistoryAbsent:
			if event.Result != "created" || event.AfterAuthority != HistoryReviewing || event.AfterEffect != model.effect || event.ObservedRevision == "" {
				return model, false
			}
			next.authority, next.revision, next.idempotencyID = HistoryReviewing, event.ObservedRevision, event.IdempotencyKey
		case HistoryReviewing:
			if event.IdempotencyKey != model.idempotencyID || event.Result != "resumed" || !event.preserves(model) {
				return model, false
			}
		default:
			return model, false
		}
	case HistoryFinalize:
		if model.authority == HistoryReviewing && event.ExpectedRevision == model.revision && event.Result == "approved" && event.AfterAuthority == HistoryApproved && event.AfterEffect == HistoryEffectPending && event.ObservedRevision != "" {
			next.authority, next.effect, next.revision = HistoryApproved, HistoryEffectPending, event.ObservedRevision
		} else if model.authority == HistoryApproved && event.Result == "idempotent" && event.preserves(model) {
			return model, true
		} else {
			return model, false
		}
	case HistoryStatus:
		valid := model.authority == HistoryAbsent && event.Result == "start" ||
			model.authority == HistoryReviewing && event.Result == "finalize" ||
			model.authority == HistoryApproved && model.effect == HistoryEffectPending && event.Result == "reconcile" ||
			model.authority == HistoryApproved && model.effect == HistoryEffectApplied && event.Result == "complete"
		if !valid || !event.preserves(model) {
			return model, false
		}
	case HistoryValidate:
		valid := model.authority == HistoryApproved && event.Result == "allow" || model.authority != HistoryApproved && event.Result == "blocked"
		if !valid || !event.preserves(model) {
			return model, false
		}
	case HistoryReconcile:
		if model.authority != HistoryApproved {
			return model, false
		}
		if model.effect == HistoryEffectPending && event.Result == "applied" && event.AfterAuthority == model.authority && event.AfterEffect == HistoryEffectApplied && event.ObservedRevision == model.revision {
			next.effect = HistoryEffectApplied
		} else if model.effect == HistoryEffectApplied && event.Result == "idempotent" && event.preserves(model) {
			return model, true
		} else {
			return model, false
		}
	default:
		return model, false
	}
	return next, true
}

func (event HistoryEvent) preserves(model historyModel) bool {
	return event.AfterAuthority == model.authority && event.AfterEffect == model.effect && event.ObservedRevision == model.revision
}

func canonicalSearchKey(used uint16, models map[string]historyModel) string {
	if len(models) == 0 {
		return fmt.Sprintf("%d:", used)
	}
	lineages := make([]string, 0, len(models))
	for lineage := range models {
		lineages = append(lineages, lineage)
	}
	sort.Strings(lineages)
	var b strings.Builder
	fmt.Fprintf(&b, "%d", used)
	for _, lineage := range lineages {
		m := models[lineage]
		fmt.Fprintf(&b, "|%q:%q:%q:%q:%q", lineage, m.authority, m.effect, m.revision, m.idempotencyID)
	}
	return b.String()
}

func CheckHistory(events []HistoryEvent) ([]HistoryEvent, error) {
	if len(events) > MaxOracleHistoryEvents {
		return nil, fmt.Errorf("%w: %d events, maximum %d", ErrOracleHistoryBound, len(events), MaxOracleHistoryEvents)
	}
	for _, event := range events {
		if event.InvocationID == "" || event.LineageID == "" || event.Completed < event.Started {
			return nil, fmt.Errorf("invalid history event %q", event.InvocationID)
		}
	}
	predecessors := make([]uint16, len(events))
	for index := range events {
		for prior := range events {
			if events[prior].Completed < events[index].Started {
				predecessors[index] |= 1 << prior
			}
		}
	}
	visited := make(map[string]bool)
	states, order := 0, make([]HistoryEvent, 0, len(events))
	var search func(uint16, map[string]historyModel) bool
	search = func(used uint16, models map[string]historyModel) bool {
		states++
		if states > MaxOracleSearchStates {
			return false
		}
		if len(order) == len(events) {
			return true
		}
		key := canonicalSearchKey(used, models)
		if visited[key] {
			return false
		}
		visited[key] = true
		for index, event := range events {
			bit := uint16(1 << index)
			if used&bit != 0 || predecessors[index]&^used != 0 {
				continue
			}
			model, ok := models[event.LineageID]
			if !ok {
				model = initialHistoryModel()
			}
			next, legal := model.step(event)
			if !legal {
				continue
			}
			copyModels := make(map[string]historyModel, len(models)+1)
			for lineage, state := range models {
				copyModels[lineage] = state
			}
			copyModels[event.LineageID] = next
			order = append(order, event)
			if search(used|bit, copyModels) {
				return true
			}
			order = order[:len(order)-1]
		}
		return false
	}
	if search(0, map[string]historyModel{}) {
		return append([]HistoryEvent(nil), order...), nil
	}
	if states > MaxOracleSearchStates {
		return nil, ErrOracleSearchBound
	}
	return nil, errors.New("review history has no legal real-time-compatible serialization")
}
