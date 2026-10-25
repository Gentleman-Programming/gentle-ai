package shellinstaller

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// PreviewUserInstall discloses settings effects, never grants installation or recovery authority.
// inspected must be the unchanged confirmation from InspectUserInstall.
func PreviewUserInstall(req UserInstallRequest, inspected string) (string, error) {
	if req.Mode != "shared" {
		return "", nil
	}
	data, existing, err := userPreviewReadSettings(req, inspected)
	if err != nil {
		return "", err
	}
	undo := fmt.Sprintf("Undo: gentle-ai shell recover %s inspect\nIf installation fails before %s exists, use ROOT=WORKSPACE/installed from the printed failure instead; never a prefix or agent.\nRecovery requires fresh confirmation; restoring prefix/agent preimages can overwrite later changes. It leaves evidence and command bindings; it is not uninstall or target deletion.\n", req.Destination, req.Destination)
	if existing {
		return "Existing destination: no new provisioning preview; installed graph readback remains required.\n" + undo, nil
	}
	settings := map[string]json.RawMessage{}
	if data != nil && (json.Unmarshal(data, &settings) != nil || settings == nil) {
		return "", errors.New("malformed shared settings")
	}
	if _, present := settings["npmCommand"]; present {
		return "", errors.New("foreign npm override in shared settings")
	}
	packages := []json.RawMessage{}
	before := "(absent)"
	if raw, present := settings["packages"]; present {
		if json.Unmarshal(raw, &packages) != nil || packages == nil {
			return "", errors.New("malformed shared packages")
		}
		compact, _ := json.Marshal(packages)
		before = string(compact)
	}
	packageRoot := filepath.Join(req.SharedPrefix, "lib/node_modules/gentle-pi")
	for _, entry := range packages {
		var source string
		if json.Unmarshal(entry, &source) != nil || string(entry) == "null" {
			var filtered map[string]json.RawMessage
			if json.Unmarshal(entry, &filtered) != nil || filtered["source"] == nil || string(filtered["source"]) == "null" || json.Unmarshal(filtered["source"], &source) != nil {
				return "", errors.New("malformed shared package source")
			}
		}
		if strings.HasPrefix(source, "npm:gentle-pi") || source == packageRoot {
			return "", errors.New("foreign Gentle declaration in shared settings")
		}
	}
	rootJSON, _ := json.Marshal(packageRoot)
	after, _ := json.Marshal(append(packages, rootJSON))
	command, _ := json.Marshal([]string{filepath.Join(req.Destination, "runtime/node/bin/node"),
		filepath.Join(req.Destination, "runtime/node/lib/node_modules/npm/bin/npm-cli.js"), "--prefix", req.SharedPrefix})
	tools := fmt.Sprintf("Agent tools: new files %s and %s (pinned fd 10.5.0, ripgrep 15.2.0); existing fd/rg or an agent bin that is not an owned 0700/0755 directory refuse before changes.\n",
		filepath.Join(req.SharedAgent, "bin/fd"), filepath.Join(req.SharedAgent, "bin/rg"))
	return fmt.Sprintf("Shared settings: %s\npackages before: %s\npackages after: %s\nnpmCommand before: (absent)\nnpmCommand after: %s\n%s%s",
		filepath.Join(req.SharedAgent, "settings.json"), before, after, command, tools, undo), nil
}
