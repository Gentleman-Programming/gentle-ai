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
