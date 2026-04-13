package agentbuilder

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// GenerationEngine abstracts the AI CLI tool used to generate a skill.
type GenerationEngine interface {
	// Agent returns the AgentID this engine wraps.
	Agent() model.AgentID
	// Generate sends prompt to the AI CLI and returns the raw output.
	Generate(ctx context.Context, prompt string) (string, error)
	// Available reports whether the underlying binary is on PATH.
	Available() bool
}

// NewEngine returns the GenerationEngine for the given AgentID.
// Returns nil when the agentID is unknown.
func NewEngine(agentID model.AgentID) GenerationEngine {
	switch agentID {
	case model.AgentClaudeCode:
		return &ClaudeEngine{}
	case model.AgentOpenCode:
		return &OpenCodeEngine{}
	case model.AgentGeminiCLI:
		return &GeminiEngine{}
	case model.AgentCodex:
		return &CodexEngine{}
	default:
		return nil
	}
}

// ClaudeEngine drives Claude Code via `claude --print -p "{prompt}"`.
type ClaudeEngine struct{}

func (e *ClaudeEngine) Agent() model.AgentID { return model.AgentClaudeCode }

func (e *ClaudeEngine) Available() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func (e *ClaudeEngine) Generate(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", "--print", "-p", prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("claude generate: %w\nstderr: %s", err, stderr.String())
	}
	return string(out), nil
}

// OpenCodeEngine drives OpenCode via `opencode run "{prompt}"`.
type OpenCodeEngine struct{}

func (e *OpenCodeEngine) Agent() model.AgentID { return model.AgentOpenCode }

func (e *OpenCodeEngine) Available() bool {
	_, err := exec.LookPath("opencode")
	return err == nil
}

func (e *OpenCodeEngine) Generate(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "opencode", "run", prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("opencode generate: %w\nstderr: %s", err, stderr.String())
	}
	return string(out), nil
}

// GeminiEngine drives Gemini CLI via `gemini -p "{prompt}"`.
type GeminiEngine struct{}

func (e *GeminiEngine) Agent() model.AgentID { return model.AgentGeminiCLI }

func (e *GeminiEngine) Available() bool {
	_, err := exec.LookPath("gemini")
	return err == nil
}

func (e *GeminiEngine) Generate(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "gemini", "-p", prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gemini generate: %w\nstderr: %s", err, stderr.String())
	}
	return string(out), nil
}

// CodexEngine drives Codex via `codex exec "{prompt}"`.
type CodexEngine struct{}

func (e *CodexEngine) Agent() model.AgentID { return model.AgentCodex }

func (e *CodexEngine) Available() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

func (e *CodexEngine) Generate(ctx context.Context, prompt string) (string, error) {
	cmd := exec.CommandContext(ctx, "codex", "exec", prompt)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("codex generate: %w\nstderr: %s", err, stderr.String())
	}
	return string(out), nil
}

// MockEngine is a test double for GenerationEngine.
type MockEngine struct {
	AgentIDVal  model.AgentID
	Output      string
	Err         error
	IsAvailable bool
}

func (e *MockEngine) Agent() model.AgentID { return e.AgentIDVal }
func (e *MockEngine) Available() bool      { return e.IsAvailable }
func (e *MockEngine) Generate(_ context.Context, _ string) (string, error) {
	return e.Output, e.Err
}
