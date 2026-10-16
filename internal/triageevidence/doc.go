// Package triageevidence implements the read-only classification core behind the
// gentle-ai triage evidence workflow (issue #2236).
//
// Every function in this package is pure and deterministic: it never touches the
// network, the filesystem, or the clock, so each decision is directly testable
// outside the workflow YAML. Inputs (issue records, related-change results) are
// plain values produced by the caller; outputs are advisory report data and never
// a label, a root-cause conclusion, or a mutation.
package triageevidence
