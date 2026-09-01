// Package sddtaskresult owns the SDD phase result contract: the grammar that
// decides whether a delegated phase produced a usable result, the two public
// failure codes, and the typed terminal handoff every runtime must emit.
//
// #3818: this contract used to live only in
// internal/assets/opencode/plugins/sdd-task-result-artifacts.ts. Every other
// runtime carried it as prose in a Markdown prompt with nothing enforcing it,
// so the typed terminal failure was reliably produced only where that plugin
// ran; elsewhere the failure mode was whatever the model did when its
// instructions did not resolve. The plugin is now a transport for this
// decision rather than the place the decision is made.
package sddtaskresult

import (
	"regexp"
	"strings"
)

// Class is the verdict for one delegated phase's raw output.
type Class string

const (
	// ClassOK means the output carries a usable phase result.
	ClassOK Class = "ok"
	// ClassEmpty means the phase produced no result at all.
	ClassEmpty Class = "empty_result"
	// ClassMalformed means a task envelope is present but not admissible.
	ClassMalformed Class = "malformed_result"
)

// FailureCode is the public token consumers route on. ClassOK has none.
func (class Class) FailureCode() string {
	switch class {
	case ClassEmpty:
		return "sdd_task_result_empty"
	case ClassMalformed:
		return "sdd_task_result_malformed"
	default:
		return ""
	}
}

// taskResultEnvelope and taskTag are ports of the shipped grammar, kept
// character-for-character so this package and the transport cannot disagree
// about what a well-formed result is. Go's default (?m)-off semantics match the
// JavaScript source's unflagged anchors.
var (
	taskResultEnvelope = regexp.MustCompile(`^<task id="[^"\r\n]+" state="completed">\n(?:<summary>[^<>\r\n]+</summary>\n)?<task_result>\n([\s\S]*?)\n</task_result>\n</task>$`)
	taskTag            = regexp.MustCompile(`</?(?:task|task_result|summary)(?:\s|>)`)
)

// Classify decides one delegated phase's raw output.
//
// Bare output that carries no task markup is admitted: a phase that answers
// directly is not a failure. An envelope that is present but does not match the
// grammar exactly is malformed rather than admitted, so a truncated or nested
// frame can never be read as a result.
func Classify(output string) Class {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ClassEmpty
	}
	envelope := taskResultEnvelope.FindStringSubmatch(trimmed)
	if envelope == nil {
		if taskTag.MatchString(trimmed) {
			return ClassMalformed
		}
		return ClassOK
	}
	if strings.TrimSpace(envelope[1]) == "" {
		return ClassEmpty
	}
	if taskTag.MatchString(envelope[1]) {
		return ClassMalformed
	}
	return ClassOK
}
