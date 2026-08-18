package sddstatus

import "errors"

type runtimeObjectiveAccountingKey struct {
	id         string
	generation int
}

type runtimeObjectiveAccounting struct{ attempts, lines int }

type runtimeAccounting struct {
	buckets                                      map[runtimeObjectiveAccountingKey]runtimeObjectiveAccounting
	lifetimeAttempts, lifetimeLines, nextOrdinal int
}

func (accounting *runtimeAccounting) fresh(objective *RuntimeObjective) error {
	if objective == nil {
		return errors.New("runtime accounting objective is missing") // refusal:by-design world-action: replay cannot reconstruct an objective-less accounting bucket
	}
	key := runtimeObjectiveAccountingKey{objective.ID, objective.Generation}
	if accounting.buckets == nil {
		accounting.buckets = map[runtimeObjectiveAccountingKey]runtimeObjectiveAccounting{}
	}
	if _, exists := accounting.buckets[key]; exists {
		return errors.New("runtime accounting objective key already exists") // refusal:by-design world-action: a duplicate immutable key means the replayed authority is corrupt
	}
	accounting.buckets[key] = runtimeObjectiveAccounting{}
	return nil
}

func (accounting *runtimeAccounting) carry(objective, predecessor *RuntimeObjective) error {
	consumed, err := accounting.current(predecessor)
	if err != nil {
		return err
	}
	if err := accounting.fresh(objective); err != nil {
		return err
	}
	accounting.buckets[runtimeObjectiveAccountingKey{objective.ID, objective.Generation}] = consumed
	return nil
}

func (accounting runtimeAccounting) current(objective *RuntimeObjective) (runtimeObjectiveAccounting, error) {
	if objective == nil {
		return runtimeObjectiveAccounting{}, errors.New("runtime accounting objective is missing") // refusal:by-design world-action: replay cannot reconstruct an objective-less accounting bucket
	}
	consumed, exists := accounting.buckets[runtimeObjectiveAccountingKey{objective.ID, objective.Generation}]
	if !exists {
		return runtimeObjectiveAccounting{}, errors.New("runtime accounting objective key is missing") // refusal:by-design world-action: a missing immutable key means the replayed authority is corrupt
	}
	return consumed, nil
}

func (accounting *runtimeAccounting) begin(objective *RuntimeObjective) error {
	consumed, err := accounting.current(objective)
	if err != nil {
		return err
	}
	consumed.attempts++
	accounting.buckets[runtimeObjectiveAccountingKey{objective.ID, objective.Generation}] = consumed
	accounting.lifetimeAttempts++
	accounting.nextOrdinal++
	return nil
}

func (accounting *runtimeAccounting) finish(objective *RuntimeObjective, lines int) error {
	consumed, err := accounting.current(objective)
	if err != nil {
		return err
	}
	consumed.lines += lines
	accounting.buckets[runtimeObjectiveAccountingKey{objective.ID, objective.Generation}] = consumed
	accounting.lifetimeLines += lines
	return nil
}

func (accounting runtimeAccounting) materialize(status *RuntimeStatus) error {
	objective := status.runtimeObjective()
	if compatible, active := status.runtimeCompatibilityActive(); active != nil {
		objective = compatible
	}
	if objective == nil {
		status.CumulativeAttempts, status.CumulativeChangedLines = 0, 0
	} else {
		consumed, err := accounting.current(objective)
		if err != nil {
			return err
		}
		status.CumulativeAttempts = consumed.attempts
		status.CumulativeChangedLines = consumed.lines
	}
	status.LifetimeAttempts = accounting.lifetimeAttempts
	status.LifetimeChangedLines = accounting.lifetimeLines
	status.NextOrdinal = accounting.nextOrdinal
	return nil
}
