package main

import "testing"

// The tests below exercise the axis SEAM and nothing that lives behind it. They
// must keep passing with every axis file deleted, because that is the property
// the seam exists to provide: the core corpus does not depend on any axis.
//
// Anything specific to one axis belongs in that axis's own test file, so it is
// removed by the same `rm` that removes the axis.

func withAxes(t *testing.T, axes []Axis) {
	t.Helper()
	saved := registeredAxes
	registeredAxes = axes
	t.Cleanup(func() { registeredAxes = saved })
}

func stubAxis(name string) Axis {
	return Axis{
		Name:       name,
		Title:      "stub",
		Properties: []string{"stub property"},
		Journeys:   func() []Journey { return []Journey{{ID: name + "-01", Title: "stub"}} },
	}
}

func TestSelectAxesDefaultsToNone(t *testing.T) {
	withAxes(t, []Axis{stubAxis("alpha")})
	selected, err := selectAxes("")
	if err != nil {
		t.Fatalf("selectAxes(\"\") = %v", err)
	}
	if len(selected) != 0 {
		t.Fatalf("a run that says nothing about axes selected %d of them; the default must be the black-box core alone", len(selected))
	}
}

func TestSelectAxesByNameAndAll(t *testing.T) {
	withAxes(t, []Axis{stubAxis("beta"), stubAxis("alpha")})

	selected, err := selectAxes("alpha")
	if err != nil || len(selected) != 1 || selected[0].Name != "alpha" {
		t.Fatalf("selectAxes(\"alpha\") = %+v, %v", selected, err)
	}

	all, err := selectAxes("all")
	if err != nil || len(all) != 2 {
		t.Fatalf("selectAxes(\"all\") = %+v, %v", all, err)
	}
	if all[0].Name != "alpha" || all[1].Name != "beta" {
		t.Fatalf("Axes() must be name-ordered so two runs agree, got %q then %q", all[0].Name, all[1].Name)
	}
}

// A typo must stop the run. The whole reason `--axis` exists is that a core run
// and a core-plus-axis run are different measurements; silently producing the
// first while the caller asked for the second is the failure this rejects.
func TestSelectAxesRefusesAnUnknownName(t *testing.T) {
	withAxes(t, []Axis{stubAxis("alpha")})
	if _, err := selectAxes("alhpa"); err == nil {
		t.Fatal("an unknown axis name was accepted; a run must never quietly fall back to the core")
	}
}

func TestValidateAxesRejectsAnAxisThatDeclaresNothing(t *testing.T) {
	silent := stubAxis("silent")
	silent.Properties = nil
	if err := validateAxes([]Axis{silent}, nil); err == nil {
		t.Fatal("an axis with no declared properties was accepted; an axis that will not say how it differs from the core differs silently")
	}
}

func TestValidateAxesRejectsAJourneyIDTheCoreAlreadyOwns(t *testing.T) {
	clashing := stubAxis("clash")
	core := []Journey{{ID: "clash-01", Title: "core"}}
	if err := validateAxes([]Axis{clashing}, core); err == nil {
		t.Fatal("an axis reusing a core journey id was accepted; the two populations would be indistinguishable in the results")
	}
}

// Every registered axis has to survive the same corpus validation the core
// does. A broken declaration inside an axis must fail the run, not decorate it.
func TestEveryRegisteredAxisPassesCorpusValidation(t *testing.T) {
	for _, axis := range Axes() {
		if err := validateAxes([]Axis{axis}, Journeys()); err != nil {
			t.Fatalf("axis %q: %v", axis.Name, err)
		}
		if err := validateCorpus(axis.Journeys()); err != nil {
			t.Fatalf("axis %q: %v", axis.Name, err)
		}
	}
}

// sortJourneys must never interleave the black-box core with a coupled axis:
// an unlabelled run of rows mixing the two is exactly what the seam prevents.
func TestSortJourneysKeepsTheCoreAndTheAxesApart(t *testing.T) {
	journeys := []JourneyResult{
		{ID: "zz-core"},
		{ID: "aa-axis", Axis: "damaged-store"},
		{ID: "aa-core"},
	}
	sortJourneys(journeys)
	if journeys[0].ID != "aa-core" || journeys[1].ID != "zz-core" || journeys[2].ID != "aa-axis" {
		t.Fatalf("core journeys must come first, in id order, then each axis: got %q %q %q",
			journeys[0].ID, journeys[1].ID, journeys[2].ID)
	}
}
