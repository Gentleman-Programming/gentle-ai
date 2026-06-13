package kilojsonc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/internal/model"
)

// kiloConfig represents the structure of kilo.jsonc.
type kiloConfig struct {
	Providers map[string]providerConfig `json:"providers"`
	Models    modelConfig               `json:"models"`
}

type providerConfig struct {
	BaseURL string `json:"baseUrl,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
}

type modelConfig struct {
	Default string `json:"default"`
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

	// Build the overlay config.
	overlay := kiloConfig{
		Providers: map[string]providerConfig{
			"kilo-gateway": {
				BaseURL: "https://api.kilocode.ai/v1",
				APIKey:  "${KILO_API_KEY}",
			},
		},
		Models: modelConfig{
			Default: "gateway/auto",
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
