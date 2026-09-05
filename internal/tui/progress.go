package tui

import (
	"fmt"

	"github.com/gentleman-programming/gentle-ai/v2/internal/pipeline"
	"github.com/gentleman-programming/gentle-ai/v2/internal/tui/screens"
)

type ProgressItem struct {
	Label  string
	Status string
	Nested bool
}

const (
	ProgressStatusPending = "pending"
	ProgressStatusRunning = "running"
)

type ProgressState struct {
	Items   []ProgressItem
	Current int
	Logs    []string
}

func NewProgressState(labels []string) ProgressState {
	items := make([]ProgressItem, 0, len(labels))
	for _, label := range labels {
		items = append(items, ProgressItem{Label: label, Status: ProgressStatusPending})
	}

	return ProgressState{Items: items, Current: -1}
}

func (p *ProgressState) Start(index int) {
	if index < 0 || index >= len(p.Items) {
		return
	}

	p.Current = index
	p.Items[index].Status = ProgressStatusRunning
}

func (p *ProgressState) Mark(index int, status string) {
	if index < 0 || index >= len(p.Items) {
		return
	}

	p.Items[index].Status = string(status)
	if index+1 < len(p.Items) {
		p.Current = index + 1
		return
	}

	p.Current = len(p.Items)
}

func (p *ProgressState) AppendLog(format string, args ...any) {
	p.Logs = append(p.Logs, fmt.Sprintf(format, args...))
}

func (p ProgressState) Done() bool {
	return p.Percent() >= 100
}

func (p ProgressState) Percent() int {
	if len(p.Items) == 0 {
		return 100
	}

	completed := 0
	for _, item := range p.Items {
		if item.Status == string(pipeline.StepStatusSucceeded) || item.Status == string(pipeline.StepStatusFailed) {
			completed++
		}
	}

	return (completed * 100) / len(p.Items)
}

func ProgressFromExecution(result pipeline.ExecutionResult) ProgressState {
	labels := make([]string, 0, len(result.Prepare.Steps)+len(result.Apply.Steps)+len(result.Rollback.Steps))
	statuses := make([]pipeline.StepStatus, 0, cap(labels))

	appendSteps := func(stage pipeline.StageResult) {
		for _, step := range stage.Steps {
			labels = append(labels, step.StepID)
			statuses = append(statuses, step.Status)
		}
	}

	appendSteps(result.Prepare)
	appendSteps(result.Apply)
	appendSteps(result.Rollback)

	progress := NewProgressState(labels)
	for idx, status := range statuses {
		progress.Mark(idx, string(status))
	}

	return progress
}

func (p ProgressState) HasFailures() bool {
	for _, item := range p.Items {
		if item.Status == string(pipeline.StepStatusFailed) {
			return true
		}
	}
	return false
}

func (p ProgressState) ViewModel() screens.InstallProgress {
	items := make([]screens.ProgressItem, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, screens.ProgressItem{Label: item.Label, Status: item.Status})
	}

	current := ""
	if p.Current >= 0 && p.Current < len(p.Items) {
		current = p.Items[p.Current].Label
	}

	return screens.InstallProgress{
		Percent:     p.Percent(),
		CurrentStep: current,
		Items:       items,
		Logs:        append([]string(nil), p.Logs...),
		Done:        p.Percent() >= 100,
		Failed:      p.HasFailures(),
	}
}
