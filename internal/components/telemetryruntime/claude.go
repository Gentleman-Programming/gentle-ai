package telemetryruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/telemetry"
)

// SendClaude handles one Claude Code hook in memory and sends at most once.
// Source paths and file contents never enter the aggregate or an error message.
func SendClaude(ctx context.Context, home string, getenv func(string) string, input io.Reader, client *http.Client) string {
	if !allowed(home, getenv) {
		return "disabled"
	}
	hook, err := telemetry.ParseClaudeHook(input)
	if err != nil {
		return "discarded"
	}
	if hook.HookEventName != "Stop" && hook.HookEventName != "SubagentStop" {
		return "ignored"
	}
	if !allowed(home, getenv) {
		return "disabled"
	}
	var usage telemetry.ClaudeUsage
	var definition []byte
	if hook.HookEventName == "SubagentStop" {
		if tail, firstPartial, err := readClaudeFile(home, hook.AgentTranscriptPath, telemetry.ClaudeTranscriptMaxBytes, true); err == nil {
			usage, _ = telemetry.ParseClaudeTranscriptTail(tail, firstPartial, hook.LastAssistantDigest)
		}
		if telemetry.ClaudeNamedAgent(hook.AgentType) {
			definition, _, _ = readClaudeFile(home, filepath.Join(home, ".claude", "agents", hook.AgentType+".md"), telemetry.ClaudeAgentMaxBytes, false)
		}
	}
	if !allowed(home, getenv) {
		return "disabled"
	}
	observation := telemetry.NormalizeClaude(hook, usage, definition)
	if observation == nil {
		return "ignored"
	}
	batch, err := json.Marshal(telemetry.RuntimeBatch{Schema: telemetry.RuntimeSchema, Registry: json.RawMessage("1"), Host: "claude-code", Rows: []telemetry.RuntimeRow{observation.Row}})
	if err != nil || len(batch) > telemetry.RuntimeMaxBytes {
		return "discarded"
	}
	return telemetry.SendRuntime(ctx, home, getenv, bytes.NewReader(batch), client)
}

func readClaudeFile(home, source string, limit int, tail bool) ([]byte, bool, error) {
	if strings.HasPrefix(source, "~/") {
		source = filepath.Join(home, strings.TrimPrefix(source, "~/"))
	}
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return nil, false, err
	}
	sourceAbs, err := filepath.Abs(source)
	if err != nil || !withinClaudeHome(homeAbs, sourceAbs) {
		return nil, false, os.ErrPermission
	}
	homeReal, err := filepath.EvalSymlinks(homeAbs)
	if err != nil {
		return nil, false, err
	}
	real, err := filepath.EvalSymlinks(sourceAbs)
	if err != nil || !withinClaudeHome(homeReal, real) {
		return nil, false, os.ErrPermission
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false, os.ErrInvalid
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	firstPartial := false
	if tail && info.Size() > int64(limit) {
		if _, err := f.Seek(-int64(limit)-1, io.SeekEnd); err != nil {
			return nil, false, err
		}
		boundary := make([]byte, 1)
		if _, err := io.ReadFull(f, boundary); err != nil {
			return nil, false, err
		}
		firstPartial = boundary[0] != '\n'
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, false, os.ErrInvalid
	}
	return data, firstPartial, nil
}

func withinClaudeHome(home, candidate string) bool {
	rel, err := filepath.Rel(home, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
