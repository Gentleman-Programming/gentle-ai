package main

import (
	"bytes"
	"strings"
	"testing"
)

// deadEndColumnCell extracts the dead_end cell of the journey row whose ID is
// given, locating the column from the table header rather than hardcoding an
// index, so a reordered table cannot silently make these tests assert on the
// wrong dimension.
func deadEndColumnCell(t *testing.T, report, journeyID string) string {
	t.Helper()
	column := -1
	for _, line := range strings.Split(report, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "journey" {
			for i, field := range fields {
				if field == "dead" {
					column = i
				}
			}
			continue
		}
		if column >= 0 && fields[0] == journeyID {
			if len(fields) <= column {
				t.Fatalf("row for %q has %d fields, dead_end column is %d\n%s", journeyID, len(fields), column, report)
			}
			return fields[column]
		}
	}
	t.Fatalf("no journey row %q under a header naming a dead column\n%s", journeyID, report)
	return ""
}

// TestDeadEndReadsUnmeasuredWhenNothingDeclaresOne is the guard on issue
// #2490: a corpus in which no journey declares a dead end has not measured
// anything, and printing 0 invites the reading "measured, and there are
// none". The column must say the measurement never happened.
func TestDeadEndReadsUnmeasuredWhenNothingDeclaresOne(t *testing.T) {
	results := byDesignResults() // no command carries a dead-end declaration
	buffer := &bytes.Buffer{}
	writeRunReport(buffer, results)
	report := buffer.String()

	if got := deadEndColumnCell(t, report, "j17-bare-repository"); got != "n/a" {
		t.Errorf("dead_end cell = %q, want %q: an unexercised dead_end must not read as a measured 0", got, "n/a")
	}
	totalLine := ""
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "TOTAL") {
			totalLine = line
		}
	}
	if totalLine == "" {
		t.Fatalf("no TOTAL row in the report\n%s", report)
	}
	if !strings.Contains(totalLine, "n/a") {
		t.Errorf("TOTAL row %q holds no n/a: the unexercised dead_end total still reads as a measurement", totalLine)
	}
	if !strings.Contains(report, "no journey in this run declared a dead end") {
		t.Errorf("legend does not say why dead_end is n/a; the reader is left to guess\n%s", report)
	}
}

// TestDeadEndStaysNumericWhenADeclarationApplied proves the column is still a
// real measurement the moment a journey exercises a declaration: a driven
// dead end must count, not read as n/a.
func TestDeadEndStaysNumericWhenADeclarationApplied(t *testing.T) {
	results := byDesignResults()
	results.Journeys[0].Commands[1].Block = BlockDeadEnd
	results.Journeys[0].Commands[1].DeadEnd = &DeadEndOutcome{Applied: true}
	results.Journeys[0].Metrics.Blocks = BlockCounts{ByDesign: 1, DeadEnd: 1}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(results.Journeys)

	buffer := &bytes.Buffer{}
	writeRunReport(buffer, results)
	report := buffer.String()

	if got := deadEndColumnCell(t, report, "j17-bare-repository"); got != "1" {
		t.Errorf("dead_end cell = %q, want %q: an exercised declaration is a measurement", got, "1")
	}
	if strings.Contains(report, "no journey in this run declared a dead end") {
		t.Errorf("legend claims dead_end is unmeasured while a declaration applied\n%s", report)
	}
}

// TestDeadEndMeasuredZeroPrintsZero is the distinction the issue asks for: a
// stale declaration means the classifier was exercised and found no dead end,
// so its 0 is a genuine measurement and must keep printing as 0, unlike the
// undeclared case above.
func TestDeadEndMeasuredZeroPrintsZero(t *testing.T) {
	results := byDesignResults()
	results.Journeys[0].Commands[1].DeadEnd = &DeadEndOutcome{
		Applied: false,
		Reason:  "the product named a runnable continuation, so the declaration is stale and should be removed",
	}
	results.Totals, results.JourneysCounted, results.JourneysUnsupported, results.JourneysFailed = aggregate(results.Journeys)

	buffer := &bytes.Buffer{}
	writeRunReport(buffer, results)
	report := buffer.String()

	if got := deadEndColumnCell(t, report, "j17-bare-repository"); got != "0" {
		t.Errorf("dead_end cell = %q, want %q: a measured zero is a real result, not an absence", got, "0")
	}
	if strings.Contains(report, "no journey in this run declared a dead end") {
		t.Errorf("legend claims dead_end is unmeasured while a stale declaration proves it was exercised\n%s", report)
	}
}
