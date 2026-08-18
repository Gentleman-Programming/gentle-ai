package sddstatus

type runtimeAccounting struct {
	attempts, lines, lifetimeAttempts, lifetimeLines, nextOrdinal int
}

func (accounting *runtimeAccounting) begin() {
	accounting.attempts++
	accounting.lifetimeAttempts++
	accounting.nextOrdinal++
}

func (accounting *runtimeAccounting) finish(lines int) {
	accounting.lines += lines
	accounting.lifetimeLines += lines
}

func (accounting *runtimeAccounting) reset() { accounting.attempts, accounting.lines = 0, 0 }

func (accounting *runtimeAccounting) materialize(status *RuntimeStatus) {
	status.CumulativeAttempts = accounting.attempts
	status.CumulativeChangedLines = accounting.lines
	status.LifetimeAttempts = accounting.lifetimeAttempts
	status.LifetimeChangedLines = accounting.lifetimeLines
	status.NextOrdinal = accounting.nextOrdinal
}
