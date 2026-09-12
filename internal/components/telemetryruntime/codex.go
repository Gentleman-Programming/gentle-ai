package telemetryruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

const codexStateMaxBytes = 65536
const codexTranscriptHeadMaxBytes = telemetry.CodexTranscriptHeadMaxBytes

// SendCodex performs one policy-gated, memory-only normalization and send.
// It never retries, persists source data, or logs hook fields and paths.
func SendCodex(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) string {
	decision, err := sendCodex(ctx, home, getenv, input, client)
	if err != nil {
		return "discarded"
	}
	return decision
}

func sendCodex(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) (string, error) {
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	data, err := io.ReadAll(io.LimitReader(input, telemetry.CodexMaxBytes+1))
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	if err != nil || len(data) > telemetry.CodexMaxBytes {
		return "", errEnvelope
	}
	source, err := telemetry.DecodeCodexHook(bytes.NewReader(data))
	if err != nil {
		return "", errEnvelope
	}
	if source == nil {
		return "ignored", nil
	}
	if source.Event == "SubagentStop" {
		head, _ := readCodexTranscriptHead(source.TranscriptPath)
		if resolved, resolveErr := telemetry.ResolveCodexAgentType(*source, bytes.NewReader(head)); resolveErr == nil {
			source = &resolved
		}
	}
	assignment := readCodexAssignment(home, *source)
	transcript, _ := readCodexTranscriptTail(source.TranscriptPath)
	if !allowed(home, getenv) {
		return "disabled", nil
	}
	observation, err := telemetry.NormalizeCodex(*source, bytes.NewReader(transcript), assignment)
	if err != nil || observation == nil {
		return "", errEnvelope
	}
	batch, err := json.Marshal(telemetry.RuntimeBatch{
		Schema: telemetry.RuntimeSchema, Registry: json.RawMessage("1"), Host: "codex", Rows: []telemetry.RuntimeRow{observation.Row},
	})
	if err != nil || len(batch) > telemetry.RuntimeMaxBytes {
		return "", errEnvelope
	}
	return telemetry.SendRuntime(ctx, home, getenv, bytes.NewReader(batch), client), nil
}

func readCodexTranscriptHead(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errEnvelope
	}
	data, err := io.ReadAll(io.LimitReader(f, codexTranscriptHeadMaxBytes))
	if err != nil {
		return nil, errEnvelope
	}
	return data, nil
}

func readCodexTranscriptTail(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errEnvelope
	}
	start := int64(0)
	if info.Size() > telemetry.CodexTranscriptMaxBytes {
		start = info.Size() - telemetry.CodexTranscriptMaxBytes
	}
	readStart := start
	readLimit := int64(telemetry.CodexTranscriptMaxBytes + 1)
	if start > 0 {
		readStart--
		readLimit++
	}
	if _, err := f.Seek(readStart, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, readLimit))
	if err != nil {
		return nil, errEnvelope
	}
	if start > 0 {
		if len(data) == 0 || len(data) > telemetry.CodexTranscriptMaxBytes+1 {
			return nil, errEnvelope
		}
		lookBehind := data[0]
		data = data[1:]
		if lookBehind != '\n' {
			newline := bytes.IndexByte(data, '\n')
			if newline < 0 {
				return nil, nil
			}
			data = data[newline+1:]
		}
	} else if len(data) > telemetry.CodexTranscriptMaxBytes {
		return nil, errEnvelope
	}
	return data, nil
}

func readCodexAssignment(home string, source telemetry.CodexHook) telemetry.CodexAssignment {
	f, err := os.Open(state.Path(home))
	if err != nil {
		return telemetry.CodexAssignment{}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, codexStateMaxBytes+1))
	if err != nil || len(data) > codexStateMaxBytes {
		return telemetry.CodexAssignment{}
	}
	var persisted struct {
		CodexModelAssignments       map[string]string                       `json:"codexModelAssignments"`
		CodexOrchestratorAssignment *state.CodexOrchestratorAssignmentState `json:"codexOrchestratorAssignment"`
		CodexCarrilModels           map[string]string                       `json:"codexCarrilModelAssignments"`
		CodexPhaseModels            map[string]string                       `json:"codexPhaseModelAssignments"`
	}
	if json.Unmarshal(data, &persisted) != nil {
		return telemetry.CodexAssignment{}
	}
	if source.Event == "Stop" {
		if persisted.CodexOrchestratorAssignment == nil {
			return telemetry.CodexAssignment{}
		}
		return telemetry.CodexAssignment{Model: persisted.CodexOrchestratorAssignment.Model, Effort: persisted.CodexOrchestratorAssignment.Effort}
	}
	if source.AgentType == "" {
		return telemetry.CodexAssignment{}
	}
	assignment := telemetry.CodexAssignment{
		Model:  persisted.CodexPhaseModels[source.AgentType],
		Effort: persisted.CodexModelAssignments[source.AgentType],
	}
	if assignment.Model != "" {
		return assignment
	}
	for _, tier := range model.CodexTierGroups() {
		for _, phase := range tier.Phases {
			if phase != source.AgentType {
				continue
			}
			assignment.Model = persisted.CodexCarrilModels[tier.Profile]
			if assignment.Model == "" {
				assignment.Model = tier.Model
			}
			return assignment
		}
	}
	return assignment
}
