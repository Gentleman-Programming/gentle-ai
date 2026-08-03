package main

import "testing"

func TestSourceCoupledAxisClassifiesReceiptDrift(t *testing.T) {
	for _, journey := range Journeys() {
		if journey.ID == "j57-sdd-authority-drift-during-discovery-fails-closed" {
			t.Fatal("j57 is source-coupled and must not run in the portable core")
		}
	}
	for _, axis := range Axes() {
		if axis.Name != sourceCoupledAxis {
			continue
		}
		if axis.BlackBox {
			t.Fatal("the receipt-drift seam requires a tagged source build and is not black-box")
		}
		journeys := axis.Journeys()
		if len(journeys) != 1 || journeys[0].ID != "j57-sdd-authority-drift-during-discovery-fails-closed" {
			t.Fatalf("source-coupled journeys = %#v, want j57 only", journeys)
		}
		return
	}
	t.Fatal("the source-coupled axis is not registered")
}
