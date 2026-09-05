package reviewerprovider

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const codexReviewerLoopbackBaseURLEnvironment = "GENTLE_AI_CODEX_REVIEWER_LOOPBACK_BASE_URL"

const codexReviewerLoopbackProviderID = "gentle_ai_reviewer_loopback"

// CodexAdapter invokes Codex with an opaque provider invocation and returns
// the CLI's raw final-message bytes without interpreting them.
type CodexAdapter struct {
	LookPath       func(string) (string, error)
	commandContext func(context.Context, string, ...string) *exec.Cmd
}

// NewCodexAdapter returns an adapter using the Codex binary resolved from PATH.
func NewCodexAdapter() *CodexAdapter {
	return &CodexAdapter{LookPath: exec.LookPath, commandContext: exec.CommandContext}
}

// Review runs Codex in an empty temporary directory. The prompt is delivered
// through stdin so command arguments never carry provider material.
func (adapter *CodexAdapter) Review(ctx context.Context, invocation Invocation) ([]byte, error) {
	lookPath := adapter.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	binary, err := lookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex reviewer transport unavailable: %w", err)
	}

	scratch, err := os.MkdirTemp("", "gentle-ai-codex-reviewer-*")
	if err != nil {
		return nil, fmt.Errorf("codex reviewer transport unavailable: create scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)

	outputPath := filepath.Join(scratch, "result")
	commandContext := adapter.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}
	arguments, err := codexReviewerArguments(scratch, outputPath)
	if err != nil {
		return nil, fmt.Errorf("codex reviewer transport unavailable: %w", err)
	}
	command := commandContext(ctx, binary, arguments...)
	command.Dir = scratch
	command.Stdin = bytes.NewReader(invocation.Prompt())
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("codex reviewer transport failed: %w: %s", err, stderr.String())
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("codex reviewer transport produced no final message: %w", err)
	}
	return raw, nil
}

func codexReviewerArguments(scratch, outputPath string) ([]string, error) {
	arguments := []string{
		"exec", "--skip-git-repo-check", "--ignore-user-config",
		"--sandbox", "read-only", "-C", scratch,
		"--output-last-message", outputPath,
	}

	baseURL, enabled, err := codexReviewerLoopbackBaseURL(os.Getenv(codexReviewerLoopbackBaseURLEnvironment))
	if err != nil {
		return nil, err
	}
	if !enabled {
		return arguments, nil
	}

	return append(arguments,
		"--config", `model_provider="`+codexReviewerLoopbackProviderID+`"`,
		"--config", fmt.Sprintf(`model_providers.%s={name="Gentle AI reviewer loopback",base_url=%q,wire_api="responses"}`, codexReviewerLoopbackProviderID, baseURL),
	), nil
}

func codexReviewerLoopbackBaseURL(raw string) (string, bool, error) {
	if raw == "" {
		return "", false, nil
	}
	if strings.TrimSpace(raw) != raw {
		return "", false, fmt.Errorf("loopback base URL must not contain leading or trailing whitespace")
	}

	endpoint, err := url.ParseRequestURI(raw)
	if err != nil {
		return "", false, fmt.Errorf("invalid loopback base URL: %w", err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return "", false, fmt.Errorf("loopback base URL scheme must be http or https")
	}
	if endpoint.User != nil {
		return "", false, fmt.Errorf("loopback base URL must not include credentials")
	}
	if endpoint.Host == "" || endpoint.Opaque != "" {
		return "", false, fmt.Errorf("loopback base URL must include a host")
	}
	host := net.ParseIP(endpoint.Hostname())
	if host == nil || !host.IsLoopback() {
		return "", false, fmt.Errorf("loopback base URL host must be a numeric loopback address")
	}
	port := endpoint.Port()
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsedPort == 0 {
		return "", false, fmt.Errorf("loopback base URL must include a valid port")
	}
	if endpoint.Path != "/v1" || endpoint.RawPath != "" {
		return "", false, fmt.Errorf("loopback base URL path must be exactly /v1")
	}
	if endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" {
		return "", false, fmt.Errorf("loopback base URL must not include a query or fragment")
	}

	return endpoint.String(), true, nil
}
