package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func proveJSON(sandbox *Sandbox, target any, args ...string) error {
	observation := sandbox.readBack(args...)
	if err := json.Unmarshal([]byte(strings.TrimSpace(observation.Stdout)), target); err != nil {
		return fmt.Errorf("%s: %w (stderr: %s)", strings.Join(args, " "), err, firstLine(observation.Stderr))
	}
	return nil
}
