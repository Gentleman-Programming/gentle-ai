package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

var currentExecutable = os.Executable

// captureOpenCodeRuntimeProvenance binds managed reviewer assets to the exact
// process that wrote them. The plugin verifies this record before it can ask
// native Go to read review authority.
func captureOpenCodeRuntimeProvenance() (*state.OpenCodeRuntimeProvenance, error) {
	executable, err := currentExecutable()
	if err != nil {
		return nil, fmt.Errorf("resolve current gentle-ai executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, fmt.Errorf("make current gentle-ai executable path absolute: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return nil, fmt.Errorf("inspect current gentle-ai executable %q: %w", executable, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("current gentle-ai executable %q is not a regular file", executable)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("current gentle-ai executable %q is not executable", executable)
	}
	file, err := os.Open(executable)
	if err != nil {
		return nil, fmt.Errorf("open current gentle-ai executable %q: %w", executable, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, fmt.Errorf("hash current gentle-ai executable %q: %w", executable, err)
	}
	return &state.OpenCodeRuntimeProvenance{
		Executable: executable,
		SHA256:     "sha256:" + hex.EncodeToString(hash.Sum(nil)),
		Version:    "gentle-ai " + AppVersion,
	}, nil
}

func hasOpenCodeReviewerPlugin(agentIDs []model.AgentID) bool {
	for _, agentID := range agentIDs {
		if agentID == model.AgentOpenCode {
			return true
		}
	}
	return false
}
