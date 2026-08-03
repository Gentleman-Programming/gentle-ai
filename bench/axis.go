package main

import (
	"fmt"
	"sort"
	"strings"
)

// An Axis is an OPT-IN extension to the corpus, and it exists for one reason:
// the core corpus is black-box and must stay that way.
//
// The core drives a `gentle-ai` binary through the process boundary and touches
// nothing else, which is what lets it measure any build including old releases.
// Some states a real repository reaches cannot be built that way — the product
// refuses to construct them, which is exactly why they are worth measuring — and
// reaching them means reading or writing something the product owns. A journey
// that does that is no longer black-box, and it is no longer portable across
// builds, because it is coupled to an internal format instead of to a CLI.
//
// Rather than let that property leak into the whole harness and be explained
// away in a footnote, it is confined here. An axis carries its own properties,
// declares them, and the report prints them next to the journeys it contributed.
// The core does not know any axis exists: nothing outside this file and the
// `--axis` flag references one, and deleting an axis file leaves the corpus
// compiling and reporting exactly the numbers it reported before.
//
// The seam is deliberately small. One registry, one flag, one report section.
// A second axis is a second file with a second init(); it needs nothing here.
type Axis struct {
	// Name is the value `--axis` takes. Lower-case, hyphenated, stable: it is
	// written into the results JSON as provenance.
	Name string
	// Title is one line describing what the axis measures.
	Title string
	// BlackBox is false for any axis that reads or writes something the product
	// owns rather than driving it through the CLI. It is a claim the report
	// prints verbatim, so it must be the truth about the fixtures, not an
	// aspiration.
	BlackBox bool
	// Properties are the axis's own honesty contract: what it costs to have
	// these journeys, and how it fails when the coupling it depends on moves.
	// The report prints every line. An axis with no properties is a corpus
	// error — an axis exists precisely because it differs from the core, and an
	// axis that will not say how differs silently.
	Properties []string
	// Journeys returns the axis's corpus. It is a function for the same reason
	// the core's is: the corpus is data, built fresh on each call.
	Journeys func() []Journey
}

// registeredAxes is populated by each axis file's init(). Registration order is
// irrelevant: Axes sorts by name.
var registeredAxes []Axis

// RegisterAxis makes one axis available to `--axis`. It is called from an axis
// file's init() and from nowhere else.
func RegisterAxis(axis Axis) {
	registeredAxes = append(registeredAxes, axis)
}

// Axes is every registered axis, sorted by name. It is empty when no axis file
// is present, which is the state the core is designed to work in.
func Axes() []Axis {
	sorted := append([]Axis{}, registeredAxes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}

func axisNames() []string {
	names := []string{}
	for _, axis := range Axes() {
		names = append(names, axis.Name)
	}
	return names
}

// selectAxes resolves the `--axis` value. Empty selects nothing, which is the
// default: a run that says nothing about axes is a core run.
//
// An unknown name is a hard error rather than a warning. The whole point of the
// flag is that "45 journeys" and "45 journeys plus a damaged-store axis" must
// never look alike, and a typo that silently produced the first while the caller
// asked for the second would defeat it.
func selectAxes(value string) ([]Axis, error) {
	requested := []string{}
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			requested = append(requested, name)
		}
	}
	if len(requested) == 0 {
		return nil, nil
	}
	available := Axes()
	if len(requested) == 1 && requested[0] == "all" {
		return available, nil
	}
	selected := []Axis{}
	for _, name := range requested {
		found := false
		for _, axis := range available {
			if axis.Name == name {
				selected, found = append(selected, axis), true
				break
			}
		}
		if !found {
			registered := "none are registered in this build"
			if len(available) > 0 {
				registered = "registered: " + strings.Join(axisNames(), ", ")
			}
			return nil, fmt.Errorf("unknown axis %q (%s, or `all`)", name, registered)
		}
	}
	return selected, nil
}

// validateAxes checks every axis a run selected before a single journey is
// driven, for the same reason validateCorpus checks declarations: an axis is a
// claim about how these numbers were obtained, and a broken claim must stop the
// run rather than decorate it.
func validateAxes(axes []Axis, core []Journey) error {
	problems := []string{}
	seen := map[string]string{}
	for _, journey := range core {
		seen[journey.ID] = "the core corpus"
	}
	for _, axis := range axes {
		if strings.TrimSpace(axis.Title) == "" {
			problems = append(problems, axis.Name+": declares no title")
		}
		if len(axis.Properties) == 0 {
			problems = append(problems,
				axis.Name+": declares no properties; an axis exists because it differs from the black-box core, and one that will not say how differs silently")
		}
		if axis.Journeys == nil {
			problems = append(problems, axis.Name+": contributes no journeys")
			continue
		}
		for _, journey := range axis.Journeys() {
			if owner, duplicate := seen[journey.ID]; duplicate {
				problems = append(problems, axis.Name+": journey id "+journey.ID+" is already used by "+owner)
				continue
			}
			seen[journey.ID] = "axis " + axis.Name
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("axis declaration errors:\n  %s", strings.Join(problems, "\n  "))
}

// AxisRecord is one axis's provenance inside the results JSON. It is written
// for every axis a run selected, so a reader of the file — not only a reader of
// the terminal — can tell which journeys were black-box and which were not.
type AxisRecord struct {
	Name       string   `json:"name"`
	Title      string   `json:"title"`
	BlackBox   bool     `json:"black_box"`
	Properties []string `json:"properties"`
	JourneyIDs []string `json:"journey_ids"`
}

func axisRecord(axis Axis, journeys []Journey) AxisRecord {
	record := AxisRecord{Name: axis.Name, Title: axis.Title, BlackBox: axis.BlackBox, Properties: axis.Properties}
	for _, journey := range journeys {
		record.JourneyIDs = append(record.JourneyIDs, journey.ID)
	}
	return record
}
