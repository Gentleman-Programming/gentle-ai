package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/internal/modelcatalog"
)

var piRPCCommandFactory = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

var defaultPiRPCCommand = []string{"pi", "--mode", "rpc"}

// PI RPC startup can emit extension/UI events before model catalog response.
// Allow extra time so startup noise does not cause premature process kill.
const defaultPiRPCModelsTimeout = 30 * time.Second

// UserFacingRPCModelLoadError returns an actionable message suitable for TUI
// rendering when PI model loading via RPC fails.
func UserFacingRPCModelLoadError(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "signal: killed") {
		return "PI RPC process was terminated before it returned models. Gentle AI waits for PI extension startup and model-catalog response; Run `pi --mode rpc` manually to verify it responds, then check PI auth/runtime provider setup and retry."
	}

	if errors.Is(err, exec.ErrNotFound) {
		return "PI binary was not found on PATH. Install PI or make sure `pi` is available in your shell, then retry."
	}

	if errors.Is(err, os.ErrPermission) {
		return "PI binary is not executable due to permissions. Fix execute permissions for `pi`, then retry."
	}

	if strings.Contains(msg, "did not return model catalog") || strings.Contains(msg, "extension UI events only") {
		return "PI RPC started but did not return a model catalog; extensions emitted UI events only. Try disabling noisy extensions, then run `pi --mode rpc get_available_models` manually to confirm your PI build/protocol returns model catalogs."
	}

	return fmt.Sprintf("PI RPC call failed: %v. Run `pi --mode rpc` manually and verify provider/auth configuration.", err)
}

// LoadModelCatalogRPC loads PI models through `pi --mode rpc` get_available_models.
func LoadModelCatalogRPC(ctx context.Context, command []string) (modelcatalog.Catalog, error) {
	cmdline := command
	if len(cmdline) == 0 {
		cmdline = append([]string(nil), defaultPiRPCCommand...)
	}

	if len(cmdline) == 0 || strings.TrimSpace(cmdline[0]) == "" {
		return modelcatalog.Catalog{}, errors.New("pi RPC command is empty")
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultPiRPCModelsTimeout)
		defer cancel()
	}

	cmd := piRPCCommandFactory(ctx, cmdline[0], cmdline[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return modelcatalog.Catalog{}, fmt.Errorf("open PI RPC stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return modelcatalog.Catalog{}, fmt.Errorf("open PI RPC stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return modelcatalog.Catalog{}, fmt.Errorf("start PI RPC command: %w", err)
	}

	request := map[string]string{
		"id":   "gentle-ai-model-picker",
		"type": "get_available_models",
	}
	if err := json.NewEncoder(stdin).Encode(request); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return modelcatalog.Catalog{}, fmt.Errorf("write get_available_models RPC request: %w", err)
	}
	_ = stdin.Close()

	models, err := parseGetAvailableModelsResponse(stdout)
	waitErr := cmd.Wait()
	if err != nil {
		if waitErr != nil {
			return modelcatalog.Catalog{}, fmt.Errorf("PI RPC parse error (%v), process error: %w", err, waitErr)
		}
		return modelcatalog.Catalog{}, err
	}
	if waitErr != nil {
		return modelcatalog.Catalog{}, fmt.Errorf("PI RPC command failed: %w", waitErr)
	}

	return buildCatalogFromPIModels(models), nil
}

type piRPCEnvelope struct {
	Type    string         `json:"type"`
	Command string         `json:"command"`
	Success bool           `json:"success"`
	Error   string         `json:"error"`
	Data    piRPCModelData `json:"data"`
}

type piRPCModelData struct {
	Models []piRPCModel `json:"models"`
}

type piRPCModel struct {
	Provider      string         `json:"provider"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Reasoning     bool           `json:"reasoning"`
	Input         []string       `json:"input"`
	ContextWindow int            `json:"contextWindow"`
	MaxTokens     int            `json:"maxTokens"`
	Cost          piRPCModelCost `json:"cost"`
}

type piRPCModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

func parseGetAvailableModelsResponse(stdout io.Reader) ([]piRPCModel, error) {
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	receivedExtensionUIEvents := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var envelope piRPCEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		if envelope.Type != "response" || envelope.Command != "get_available_models" {
			if envelope.Type == "extension_ui_request" {
				receivedExtensionUIEvents = true
			}
			continue
		}

		if !envelope.Success {
			if envelope.Error == "" {
				return nil, errors.New("PI RPC returned get_available_models failure")
			}
			return nil, errors.New(envelope.Error)
		}

		return envelope.Data.Models, nil
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read PI RPC output: %w", err)
	}

	if receivedExtensionUIEvents {
		return nil, errors.New("PI RPC did not return model catalog; received extension UI events only")
	}

	return nil, errors.New("PI RPC get_available_models response not found")
}

func buildCatalogFromPIModels(models []piRPCModel) modelcatalog.Catalog {
	providers := map[string]modelcatalog.Provider{}
	available := []string{}
	sddModels := map[string][]modelcatalog.Model{}

	for _, item := range models {
		providerID := strings.TrimSpace(item.Provider)
		modelID := strings.TrimSpace(item.ID)
		if providerID == "" || modelID == "" {
			continue
		}

		provider := providers[providerID]
		if provider.ID == "" {
			provider = modelcatalog.Provider{
				ID:     providerID,
				Name:   providerID,
				Models: map[string]modelcatalog.Model{},
			}
			available = append(available, providerID)
		}

		modelName := strings.TrimSpace(item.Name)
		if modelName == "" {
			modelName = modelID
		}

		normalized := modelcatalog.Model{
			ID:            modelID,
			Name:          modelName,
			Reasoning:     item.Reasoning,
			ContextWindow: item.ContextWindow,
			MaxTokens:     item.MaxTokens,
			Cost: modelcatalog.ModelCost{
				Input:      item.Cost.Input,
				Output:     item.Cost.Output,
				CacheRead:  item.Cost.CacheRead,
				CacheWrite: item.Cost.CacheWrite,
			},
		}

		provider.Models[modelID] = normalized
		providers[providerID] = provider
		sddModels[providerID] = append(sddModels[providerID], normalized)
	}

	sort.Strings(available)
	for providerID := range sddModels {
		sort.Slice(sddModels[providerID], func(i, j int) bool {
			return sddModels[providerID][i].Name < sddModels[providerID][j].Name
		})
	}

	return modelcatalog.Catalog{
		Providers:            providers,
		AvailableProviderIDs: available,
		SDDModels:            sddModels,
	}
}
