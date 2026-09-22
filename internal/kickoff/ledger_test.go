package kickoff

import (
	"fmt"
	"sync"
	"testing"
)

func TestLoadGatesAbsentFileReturnsEmptyLedgerNoError(t *testing.T) {
	changeRoot := t.TempDir()
	ledger, err := LoadGates(changeRoot)
	if err != nil {
		t.Fatalf("LoadGates() error = %v, se esperaba nil", err)
	}
	if len(ledger.Records) != 0 {
		t.Fatalf("Records = %+v, se esperaba vacio sin fichero", ledger.Records)
	}
}

func TestAppendGatePreservesPreviousRecords(t *testing.T) {
	changeRoot := t.TempDir()
	if err := AppendGate(changeRoot, GateRecord{Gate: GateSpec, Decision: DecisionRejected, Reason: "primero", Actor: "maintainer"}); err != nil {
		t.Fatalf("primer AppendGate() error = %v", err)
	}
	if err := AppendGate(changeRoot, GateRecord{Gate: GateSpec, Decision: DecisionApproved, Reason: "segundo tras remediar", Actor: "maintainer"}); err != nil {
		t.Fatalf("segundo AppendGate() error = %v", err)
	}

	ledger, err := LoadGates(changeRoot)
	if err != nil {
		t.Fatalf("LoadGates() error = %v", err)
	}
	if len(ledger.Records) != 2 {
		t.Fatalf("Records = %+v, se esperaban 2 (el anexado no debe perder el primero)", ledger.Records)
	}
	if ledger.Records[0].Reason != "primero" || ledger.Records[1].Reason != "segundo tras remediar" {
		t.Fatalf("orden/registros = %+v, se esperaba preservar el primero y anadir el segundo", ledger.Records)
	}
}

// TestAppendGateConcurrentWritesLoseNoRecords must be run with -race: two
// goroutines append to the SAME ledger concurrently, and AppendGate's
// exclusive lock must serialize them so neither append is lost.
func TestAppendGateConcurrentWritesLoseNoRecords(t *testing.T) {
	changeRoot := t.TempDir()
	const n = 20
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = AppendGate(changeRoot, GateRecord{
				Gate:     GateSpec,
				Decision: DecisionApproved,
				Reason:   fmt.Sprintf("motivo-%d", i),
				Actor:    "maintainer",
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("AppendGate() goroutine %d error = %v", i, err)
		}
	}

	ledger, err := LoadGates(changeRoot)
	if err != nil {
		t.Fatalf("LoadGates() error = %v", err)
	}
	if len(ledger.Records) != n {
		t.Fatalf("len(Records) = %d, se esperaban %d (ninguno perdido bajo concurrencia)", len(ledger.Records), n)
	}
	seenReasons := make(map[string]bool, n)
	for _, r := range ledger.Records {
		seenReasons[r.Reason] = true
	}
	if len(seenReasons) != n {
		t.Fatalf("se esperaban %d motivos distintos, se vieron %d (registro duplicado o perdido)", n, len(seenReasons))
	}
}

func TestAppendGateRejectsInvalidRecord(t *testing.T) {
	changeRoot := t.TempDir()
	err := AppendGate(changeRoot, GateRecord{Gate: GateKey("bogus"), Decision: DecisionApproved, Actor: "maintainer"})
	if err == nil {
		t.Fatal("AppendGate() con una clave de compuerta desconocida debia devolver error")
	}
	ledger, loadErr := LoadGates(changeRoot)
	if loadErr != nil {
		t.Fatalf("LoadGates() error = %v", loadErr)
	}
	if len(ledger.Records) != 0 {
		t.Fatalf("Records = %+v, un registro invalido no debio anexarse", ledger.Records)
	}
}

// TestGateLockRetryBackoffGrowsCapsAndStaysAboveBase pins the three
// properties AppendGate's contention retry depends on. A flat delay makes
// every contender retry on the same tick and keep colliding, which is what
// drained the budget under twenty-way contention on a loaded CI machine.
func TestGateLockRetryBackoffGrowsCapsAndStaysAboveBase(t *testing.T) {
	original := randomInt63n
	t.Cleanup(func() { randomInt63n = original })

	// No jitter: the raw backoff curve is what this case pins.
	randomInt63n = func(int64) int64 { return 0 }

	first := gateLockRetryBackoff(0)
	if first != gateLockRetryDelay {
		t.Fatalf("gateLockRetryBackoff(0) = %v, se esperaba el retardo base %v", first, gateLockRetryDelay)
	}

	previous := first
	for attempt := 1; attempt <= 6; attempt++ {
		got := gateLockRetryBackoff(attempt)
		if got < previous {
			t.Fatalf("gateLockRetryBackoff(%d) = %v, menor que el intento anterior %v: el backoff debe crecer", attempt, got, previous)
		}
		previous = got
	}

	for _, attempt := range []int{20, 100, gateLockAcquireAttempts - 1} {
		if got := gateLockRetryBackoff(attempt); got > gateLockMaxRetryDelay {
			t.Fatalf("gateLockRetryBackoff(%d) = %v, por encima del techo %v", attempt, got, gateLockMaxRetryDelay)
		}
	}

	// Maximum jitter must never drive a retry below the base delay, or a
	// contender would spin hot instead of yielding.
	randomInt63n = func(n int64) int64 {
		if n <= 0 {
			return 0
		}
		return n - 1
	}
	for attempt := 0; attempt <= 12; attempt++ {
		if got := gateLockRetryBackoff(attempt); got < gateLockRetryDelay {
			t.Fatalf("con jitter maximo, gateLockRetryBackoff(%d) = %v, por debajo del retardo base %v", attempt, got, gateLockRetryDelay)
		}
	}
}
