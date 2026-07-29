package recoverytrace

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrLedgerNotFound       = errors.New("appendix ledger block is missing or truncated")
	ErrMalformedRow         = errors.New("appendix ledger row is not a complete backlog entry")
	ErrUnknownContext       = errors.New("appendix ledger row names a context outside the nine")
	ErrDuplicateBacklogItem = errors.New("appendix ledger repeats a backlog item")
)

type BacklogKind string

const (
	BacklogIssue       BacklogKind = "issue"
	BacklogPullRequest BacklogKind = "pull_request"
)

// BacklogItem is one Appendix B row. It carries the snapshot disposition and
// action verbatim: those vocabularies belong to the audit, and re-interpreting
// them here would let the importer silently disagree with its own source.
type BacklogItem struct {
	Number      int             `json:"number"`
	Kind        BacklogKind     `json:"kind"`
	Context     SystemicContext `json:"context"`
	Disposition string          `json:"disposition"`
	Action      string          `json:"action"`
}

type ledgerBlock struct {
	kind  BacklogKind
	start string
	end   string
}

var appendixBlocks = []ledgerBlock{
	{kind: BacklogIssue, start: "<!-- ISSUE_LEDGER_START -->", end: "<!-- ISSUE_LEDGER_END -->"},
	{kind: BacklogPullRequest, start: "<!-- PR_LEDGER_START -->", end: "<!-- PR_LEDGER_END -->"},
}

// ImportAppendixB reads the authoritative backlog reconciliation out of the
// systemic audit. It never infers a missing row: a truncated or reshaped table
// fails so a drifting audit cannot quietly reconcile against fewer items.
func ImportAppendixB(source []byte) ([]BacklogItem, error) {
	document := string(source)
	items := make([]BacklogItem, 0, expectedIssues+expectedPullRequests)

	for _, block := range appendixBlocks {
		body, err := ledgerBody(document, block)
		if err != nil {
			return nil, err
		}
		parsed, err := parseLedgerRows(body, block.kind)
		if err != nil {
			return nil, err
		}
		items = append(items, parsed...)
	}
	return items, nil
}

func ledgerBody(document string, block ledgerBlock) (string, error) {
	start := strings.Index(document, block.start)
	if start < 0 {
		return "", fmt.Errorf("%w: %s", ErrLedgerNotFound, block.start)
	}
	start += len(block.start)

	end := strings.Index(document[start:], block.end)
	if end < 0 {
		return "", fmt.Errorf("%w: %s", ErrLedgerNotFound, block.end)
	}
	return document[start : start+end], nil
}

func parseLedgerRows(body string, kind BacklogKind) ([]BacklogItem, error) {
	items := make([]BacklogItem, 0, expectedIssues)
	seen := make(map[int]struct{})

	for _, line := range strings.Split(body, "\n") {
		cells, ok := tableCells(line)
		if !ok || isHeaderOrSeparator(cells) {
			continue
		}
		item, err := parseLedgerRow(cells, kind)
		if err != nil {
			return nil, err
		}
		// Numbers repeat across kinds — issue #688 and pull request #688 are
		// different items — so deduplication is scoped to one block.
		if _, duplicate := seen[item.Number]; duplicate {
			return nil, fmt.Errorf("%w: %s %d", ErrDuplicateBacklogItem, kind, item.Number)
		}
		seen[item.Number] = struct{}{}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("%w: %s ledger has no rows", ErrMalformedRow, kind)
	}
	return items, nil
}

// tableCells splits one Markdown table row into its trimmed cells. Lines that are
// not table rows are reported as such rather than as malformed rows, because the
// block legitimately contains blank lines and prose.
func tableCells(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") || len(trimmed) < 2 {
		return nil, false
	}
	parts := strings.Split(strings.Trim(trimmed, "|"), "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells, true
}

func isHeaderOrSeparator(cells []string) bool {
	if len(cells) == 0 {
		return true
	}
	if cells[0] == "#" {
		return true
	}
	return strings.Trim(cells[0], "-:") == ""
}

func parseLedgerRow(cells []string, kind BacklogKind) (BacklogItem, error) {
	const expectedCells = 4
	if len(cells) != expectedCells {
		return BacklogItem{}, fmt.Errorf("%w: %v", ErrMalformedRow, cells)
	}

	number, err := strconv.Atoi(cells[0])
	if err != nil {
		return BacklogItem{}, fmt.Errorf("%w: %q is not a backlog number", ErrMalformedRow, cells[0])
	}

	context := SystemicContext(unquote(cells[1]))
	if !knownContext(context) {
		return BacklogItem{}, fmt.Errorf("%w: %q", ErrUnknownContext, context)
	}

	disposition := unquote(cells[2])
	action := unquote(cells[3])
	if disposition == "" || action == "" {
		return BacklogItem{}, fmt.Errorf("%w: item %d lacks a disposition or action", ErrMalformedRow, number)
	}

	return BacklogItem{
		Number:      number,
		Kind:        kind,
		Context:     context,
		Disposition: disposition,
		Action:      action,
	}, nil
}

func unquote(cell string) string {
	return strings.Trim(cell, "`")
}

func knownContext(context SystemicContext) bool {
	switch context {
	case ContextHCR, ContextMMI, ContextACI, ContextMCA,
		ContextRAR, ContextEPD, ContextDSR, ContextSDD, ContextPAD:
		return true
	default:
		return false
	}
}
