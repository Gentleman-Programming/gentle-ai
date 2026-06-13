package kilojsonc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// kiloConfig represents the structure of kilo.jsonc per the official schema.
// Root-level key is "model" (singular), provider map key is "provider" (singular),
// and each provider's options use "baseURL" (camelCase).
type kiloConfig struct {
	Model    string                    `json:"model,omitempty"`
	Provider map[string]providerConfig `json:"provider,omitempty"`
}

type providerConfig struct {
	API     string            `json:"api,omitempty"`
	Name    string            `json:"name,omitempty"`
	Env     []string          `json:"env,omitempty"`
	Options providerOptions   `json:"options,omitempty"`
}

type providerOptions struct {
	APIKey string `json:"apiKey,omitempty"`
	BaseURL string `json:"baseURL,omitempty"`
}

// Generate writes ~/.config/kilo/kilo.jsonc with Kilo Gateway provider config
// and model routing. The file is created if it does not exist, or deep-merged
// if it already exists.
//
// modelAssignments maps phase names to KiloModelAlias values. When nil or
// empty, the default balanced preset is used.
func Generate(homeDir string, modelAssignments map[string]model.KiloModelAlias) (bool, error) {
	if homeDir == "" {
		return false, nil
	}

	configDir := filepath.Join(homeDir, ".config", "kilo")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return false, fmt.Errorf("create Kilo config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "kilo.jsonc")

	// Build the overlay config matching Kilo Code's official schema.
	overlay := kiloConfig{
		Model: "gateway/auto",
		Provider: map[string]providerConfig{
			"kilo-gateway": {
				API:  "openai",
				Name: "Kilo Gateway",
				Env:  []string{"KILO_API_KEY"},
				Options: providerOptions{
					APIKey:  "${KILO_API_KEY}",
					BaseURL: "https://api.kilocode.ai/v1",
				},
			},
		},
	}

	overlayBytes, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal kilo.jsonc overlay: %w", err)
	}
	overlayBytes = append(overlayBytes, '\n')

	// Merge with existing file if present.
	existingBytes, readErr := os.ReadFile(configPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, fmt.Errorf("read existing kilo.jsonc: %w", readErr)
	}

	var merged []byte
	if readErr == nil && len(existingBytes) > 0 {
		merged, err = filemerge.MergeJSONObjects(existingBytes, overlayBytes)
		if err != nil {
			// Fallback: write overlay directly if merge fails.
			merged = overlayBytes
		}
	} else {
		merged = overlayBytes
	}

	writeResult, err := filemerge.WriteFileAtomic(configPath, merged, 0o644)
	if err != nil {
		return false, fmt.Errorf("write kilo.jsonc: %w", err)
	}

	return writeResult.Changed, nil
}
