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
// We only inject the $schema key to ensure validity. Model routing and provider
// config are handled natively by Kilo Code — injecting custom providers or model
// overrides conflicts with its built-in gateway and causes server errors.
type kiloConfig struct {
	Schema string `json:"$schema,omitempty"`
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

	// Build the overlay config — only inject $schema for validity.
	// Kilo Code has built-in providers and model routing; injecting custom
	// provider entries conflicts with its gateway and causes server errors.
	overlay := kiloConfig{
		Schema: "https://app.kilo.ai/config.json",
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
			// Attempt to preserve existing content by injecting $schema manually.
			var existing map[string]any
			if jsonErr := json.Unmarshal(existingBytes, &existing); jsonErr == nil {
				existing["$schema"] = "https://app.kilo.ai/config.json"
				preserved, marshalErr := json.MarshalIndent(existing, "", "  ")
				if marshalErr == nil {
					merged = append(preserved, '\n')
				} else {
					merged = overlayBytes
				}
			} else {
				merged = overlayBytes
			}
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
