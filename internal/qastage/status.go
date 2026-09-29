package qastage

import "context"

// Status is the read-only, always-recalculated view of a change's ledger —
// never cached (design decision 1.1 / baseline Escenario 8, preserved).
type Status struct {
	Change     string
	Revision   string
	NextAction string // "begin" | "finish" | "complete"
	// Stage is the stage next_action applies to: the stage to begin, or the
	// stage of the attempt currently running to finish. Empty once complete.
	// qa-supervisor (Fase 3B) relies on this to know WHAT to delegate,
	// without ever deciding the order itself.
	Stage    string
	Complete bool
}

// Status recomputes change's current status directly from its history.
func (m *QAStateMachine) Status(ctx context.Context, change string) (Status, error) {
	state, head, err := m.readState(ctx, change)
	if err != nil {
		return Status{}, err
	}

	status := Status{Change: change, Revision: head.Revision}

	if len(state.Attempts) > 0 && state.Attempts[len(state.Attempts)-1].Outcome == OutcomeRunning {
		status.NextAction = "finish"
		status.Stage = state.Attempts[len(state.Attempts)-1].Stage
		return status, nil
	}

	next := nextExpectedStage(state)
	if next == "" {
		status.NextAction = "complete"
		status.Complete = true
		return status, nil
	}

	status.NextAction = "begin"
	status.Stage = next
	return status, nil
}
