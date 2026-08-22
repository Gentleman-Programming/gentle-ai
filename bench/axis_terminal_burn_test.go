package main

import (
	"strings"
	"testing"
)

func TestTerminalBurnAxisDeclaresItsFixtureBoundary(t *testing.T) {
	for _, axis := range Axes() {
		if axis.Name != terminalBurnAxis {
			continue
		}
		if axis.BlackBox {
			t.Fatal("terminal-burn uses the bench_fixture admission seam and is not black-box")
		}
		journeys := axis.Journeys()
		if len(journeys) != 1 || !strings.HasPrefix(journeys[0].ID, "tb01-") {
			t.Fatalf("terminal-burn journeys = %+v, want one tb01 journey", journeys)
		}
		if journeys[0].Review != reviewOptedIn {
			t.Fatalf("terminal-burn review precondition = %q, want opted-in", journeys[0].Review)
		}
		return
	}
	t.Fatal("terminal-burn axis is not registered")
}
