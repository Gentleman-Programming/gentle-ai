package sddstatus

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const workItemMetadataVersion = "gentle-ai.sdd-items/v1"

var (
	errInvalidWorkItem = errors.New("invalid work item metadata; correct the block and rerun `gentle-ai sdd-status --cwd <repo> --json`")
	workItemMarker     = regexp.MustCompile(`<!--\s*gentle-ai\.sdd-items/v1(?:\s|$)`)
	workItemBlock      = regexp.MustCompile(`(?s)<!--\s*gentle-ai\.sdd-items/v1\s*\n(.*?)\n\s*-->`)
	workItemID         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	workItemCheckbox   = regexp.MustCompile(`^\s*(?:[-*]|\d+[.)])\s+\[([ xX])\]\s+([a-z0-9][a-z0-9._-]{0,127}):(?:\s|$)`)
)

// WorkItem is an optional, read-only projection of declared task metadata.
// Runtime references are populated only for the item's exact work-unit.
type WorkItem struct {
	ID               string          `json:"id"`
	DependsOn        []string        `json:"dependsOn"`
	WorkUnit         string          `json:"workUnit"`
	EditRoots        []string        `json:"editRoots"`
	MaxAttempts      int             `json:"maxAttempts"`
	MaxChangedLines  int             `json:"maxChangedLines"`
	EvidenceGoal     string          `json:"evidenceGoal"`
	Done             bool            `json:"done"`
	Active           bool            `json:"active"`
	Blocked          bool            `json:"blocked"`
	Ready            bool            `json:"ready"`
	RuntimeAttempt   *RuntimeAttempt `json:"runtimeAttempt,omitempty"`
	EvidenceRevision string          `json:"evidenceRevision,omitempty"`
}

type workItemDocument struct {
	Items []workItemMetadata `json:"items"`
}

type workItemMetadata struct {
	ID              string    `json:"id"`
	DependsOn       *[]string `json:"dependsOn"`
	WorkUnit        string    `json:"workUnit"`
	EditRoots       []string  `json:"editRoots"`
	MaxAttempts     int       `json:"maxAttempts"`
	MaxChangedLines int       `json:"maxChangedLines"`
	EvidenceGoal    string    `json:"evidenceGoal"`
}

func applyWorkItemProjection(status *Status, tasks string) {
	items, present, err := projectWorkItems(tasks, *status)
	if !present {
		return
	}
	if err != nil {
		status.Dependencies.Apply = DependencyBlocked
		status.NextRecommended = "resolve-blockers"
		status.BlockedReasons = append(status.BlockedReasons, fmt.Sprintf("work item metadata is invalid: %v; correct the metadata block and rerun `gentle-ai sdd-status --cwd %s --json`", err, status.ActionContext.WorkspaceRoot))
		return
	}
	status.Items = items
}

func projectWorkItems(tasks string, status Status) ([]WorkItem, bool, error) {
	matches := workItemBlock.FindAllStringSubmatch(tasks, -1)
	markers := workItemMarker.FindAllStringIndex(tasks, -1)
	if len(markers) == 0 {
		return nil, false, nil
	}
	if len(markers) != 1 || len(matches) != 1 {
		return nil, true, invalidWorkItem("expected one %s block", workItemMetadataVersion)
	}
	var document workItemDocument
	decoder := json.NewDecoder(strings.NewReader(matches[0][1]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, true, invalidWorkItem("decode %s: %v", workItemMetadataVersion, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, true, invalidWorkItem("decode %s: trailing content", workItemMetadataVersion)
	}
	checkboxes := map[string]bool{}
	for _, line := range strings.Split(tasks, "\n") {
		match := workItemCheckbox.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		if _, exists := checkboxes[match[2]]; exists {
			return nil, true, invalidWorkItem("duplicate checkbox id %q", match[2])
		}
		checkboxes[match[2]] = match[1] == "x" || match[1] == "X"
	}
	if len(document.Items) == 0 {
		return nil, true, invalidWorkItem("items must not be empty")
	}
	metadata := map[string]workItemMetadata{}
	workUnits := map[string]bool{}
	for _, item := range document.Items {
		if err := validateWorkItem(item, status.ActionContext.WorkspaceRoot); err != nil {
			return nil, true, err
		}
		if _, exists := metadata[item.ID]; exists {
			return nil, true, invalidWorkItem("duplicate item id %q", item.ID)
		}
		if workUnits[item.WorkUnit] {
			return nil, true, invalidWorkItem("duplicate work unit %q", item.WorkUnit)
		}
		metadata[item.ID] = item
		workUnits[item.WorkUnit] = true
		if _, exists := checkboxes[item.ID]; !exists {
			return nil, true, invalidWorkItem("missing checkbox id %q", item.ID)
		}
	}
	if len(checkboxes) != len(metadata) {
		return nil, true, invalidWorkItem("checkbox ids must exactly match metadata ids")
	}
	for _, item := range document.Items {
		for _, dependency := range *item.DependsOn {
			if dependency == item.ID {
				return nil, true, invalidWorkItem("item %q depends on itself", item.ID)
			}
			if _, exists := metadata[dependency]; !exists {
				return nil, true, invalidWorkItem("item %q depends on unknown %q", item.ID, dependency)
			}
		}
	}
	if workItemCycle(document.Items, metadata) {
		return nil, true, invalidWorkItem("item dependencies contain a cycle")
	}

	result := make([]WorkItem, 0, len(document.Items))
	for _, declared := range document.Items {
		item := WorkItem{ID: declared.ID, DependsOn: append([]string{}, (*declared.DependsOn)...), WorkUnit: declared.WorkUnit, EditRoots: declared.EditRoots, MaxAttempts: declared.MaxAttempts, MaxChangedLines: declared.MaxChangedLines, EvidenceGoal: declared.EvidenceGoal, Done: checkboxes[declared.ID]}
		scopeAllowed := workItemRootsAllowed(declared.EditRoots, status.ActionContext)
		dependenciesDone := true
		for _, dependency := range *declared.DependsOn {
			dependenciesDone = dependenciesDone && checkboxes[dependency]
		}
		runtimeMatch := !item.Done && workItemRuntimeMatches(declared, status.ActionContext.WorkspaceRoot, status.RuntimeStatus)
		if runtimeMatch {
			item.EvidenceRevision = status.RuntimeStatus.EvidenceRevision
			if status.RuntimeStatus.ActiveAttempt != nil {
				item.Active, item.RuntimeAttempt = true, status.RuntimeStatus.ActiveAttempt
			}
		}
		otherActive := status.RuntimeStatus != nil && status.RuntimeStatus.ActiveAttempt != nil && !runtimeMatch
		terminalRuntime := runtimeMatch && status.RuntimeStatus.ActiveAttempt == nil && (status.RuntimeStatus.Complete || status.RuntimeStatus.DecisionRequired || lastWorkItemAttemptFailed(status.RuntimeStatus))
		item.Blocked = !item.Done && (!scopeAllowed || !dependenciesDone || otherActive || terminalRuntime || status.Dependencies.Apply != DependencyReady)
		item.Ready = !item.Done && !item.Active && !item.Blocked
		result = append(result, item)
	}
	return result, true, nil
}

func workItemRuntimeMatches(item workItemMetadata, workspace string, runtime *RuntimeStatus) bool {
	if runtime == nil || runtime.Objective == nil {
		return false
	}
	objective := runtime.Objective
	if objective.ItemID == "" {
		return objective.WorkUnit == item.WorkUnit
	}
	roots, ok := canonicalWorkItemRoots(item.EditRoots, workspace)
	return ok && objective.ItemID == item.ID && objective.WorkUnit == item.WorkUnit &&
		objective.EvidenceGoal == item.EvidenceGoal && objective.MaxAttempts == item.MaxAttempts &&
		objective.MaxChangedLines == item.MaxChangedLines && runtimeItemBindingEqual(item.ID, roots, objective.ItemID, objective.ItemEditRoots)
}

func lastWorkItemAttemptFailed(runtime *RuntimeStatus) bool {
	return len(runtime.Attempts) != 0 && runtime.Attempts[len(runtime.Attempts)-1].Outcome == AttemptFailed
}

func validateWorkItem(item workItemMetadata, workspace string) error {
	if item.DependsOn == nil || !workItemID.MatchString(item.ID) || !workItemID.MatchString(item.WorkUnit) || strings.TrimSpace(item.EvidenceGoal) == "" || item.MaxAttempts < 1 || item.MaxChangedLines < 1 {
		return invalidWorkItem("invalid item %q", item.ID)
	}
	if len(item.EditRoots) == 0 {
		return invalidWorkItem("item %q has no edit roots", item.ID)
	}
	seen := map[string]bool{}
	for _, root := range item.EditRoots {
		if root == "" || filepath.IsAbs(root) || root == "." || strings.HasPrefix(filepath.Clean(root), ".."+string(filepath.Separator)) || strings.Contains("/"+filepath.ToSlash(root)+"/", "/../") || seen[root] {
			return invalidWorkItem("invalid edit root %q", root)
		}
		seen[root] = true
		if !withinAnyRoot(resolveExistingPath(filepath.Join(workspace, root)), []string{resolveExistingPath(workspace)}) {
			return invalidWorkItem("edit root %q escapes workspace", root)
		}
	}
	return nil
}

func invalidWorkItem(_ string, _ ...any) error { return errInvalidWorkItem }

func workItemRootsAllowed(roots []string, context ActionContext) bool {
	allowed := make([]string, 0, len(context.AllowedEditRoots))
	for _, root := range context.AllowedEditRoots {
		canonical, ok := prospectiveWorkItemPath(root)
		if !ok {
			return false
		}
		allowed = append(allowed, canonical)
	}
	for _, root := range roots {
		canonical, ok := prospectiveWorkItemPath(filepath.Join(context.WorkspaceRoot, root))
		if !ok || !withinAnyRoot(canonical, allowed) {
			return false
		}
	}
	return true
}

// canonicalWorkItemRoots shares projection's prospective-path semantics with
// item-selected acquire, preserving absent suffixes after resolving symlinks.
func canonicalWorkItemRoots(roots []string, workspace string) ([]string, bool) {
	canonical := make([]string, 0, len(roots))
	for _, root := range roots {
		path := root
		if workspace != "" {
			path = filepath.Join(workspace, root)
		}
		resolved, ok := prospectiveWorkItemPath(path)
		if !ok {
			return nil, false
		}
		canonical = append(canonical, resolved)
	}
	sort.Strings(canonical)
	for index := 1; index < len(canonical); index++ {
		if canonical[index] == canonical[index-1] {
			return nil, false
		}
	}
	return canonical, true
}

// ResolveItemAcquire selects one projected item without status's marker write
// and derives the immutable request that RuntimeStore.Acquire will serialize.
func ResolveItemAcquire(options ResolveOptions, itemID, requestID string) (BeginAttemptRequest, error) {
	options.ReadOnly = true
	status, err := Resolve(options)
	if err != nil {
		return BeginAttemptRequest{}, fmt.Errorf("resolve item-selected acquire: %w", err)
	}
	for _, item := range status.Items {
		if item.ID != itemID {
			continue
		}
		if item.Done || item.Blocked || (!item.Ready && !item.Active) {
			// refusal:by-design operator-knowledge: the coordinator must update prerequisites or the checkbox before this item can open.
			return BeginAttemptRequest{}, fmt.Errorf("item-selected acquire refused: item %q is not ready", itemID)
		}
		roots, ok := canonicalWorkItemRoots(item.EditRoots, status.ActionContext.WorkspaceRoot)
		if !ok || !workItemRootsAllowed(item.EditRoots, status.ActionContext) {
			// refusal:by-design human-authority: only the coordinator may authorize or correct item edit roots.
			return BeginAttemptRequest{}, fmt.Errorf("item-selected acquire refused: item %q has incompatible edit roots", itemID)
		}
		return BeginAttemptRequest{RequestID: requestID, WorkUnit: item.WorkUnit, EvidenceGoal: item.EvidenceGoal,
			MaxAttempts: item.MaxAttempts, MaxChangedLines: item.MaxChangedLines, ItemID: item.ID, ItemEditRoots: roots}, nil
	}
	// refusal:by-design operator-knowledge: the caller selected an ID absent from authoritative metadata.
	return BeginAttemptRequest{}, fmt.Errorf("item-selected acquire refused: projected item %q was not found", itemID)
}

// prospectiveWorkItemPath preserves absent suffixes while canonicalizing the
// nearest existing ancestor, so unrelated future siblings cannot overlap.
func prospectiveWorkItemPath(path string) (string, bool) {
	path = filepath.Clean(path)
	var suffix []string
	for {
		if _, err := os.Lstat(path); err == nil {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", false
		}
		suffix = append([]string{filepath.Base(path)}, suffix...)
		path = parent
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	return filepath.Join(append([]string{canonical}, suffix...)...), true
}

func workItemCycle(items []workItemMetadata, metadata map[string]workItemMetadata) bool {
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependency := range *metadata[id].DependsOn {
			if visit(dependency) {
				return true
			}
		}
		delete(visiting, id)
		visited[id] = true
		return false
	}
	for _, item := range items {
		if visit(item.ID) {
			return true
		}
	}
	return false
}
