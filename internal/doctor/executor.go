package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/state"
)

type executor struct {
	registry         *registry
	platformDetector func() Platform
}

type preflightResult struct {
	executor   *executor
	id         RemedyID
	category   RemedyCategory
	actionMode ActionMode
	platform   Platform
}

type managedOperation interface {
	id() RemedyID
	preflight(context.Context, Platform) (preflightEvidence, error)
}

type preflightEvidence struct {
	id                  RemedyID
	category            RemedyCategory
	actionMode          ActionMode
	platform            Platform
	eligible            bool
	prerequisitesReady  bool
	ownershipVerified   bool
	containmentVerified bool
}

type registry struct{ operations map[RemedyID]managedOperation }

var errPreflightDenied = errors.New("remedy preflight denied")

// PreflightRemedySync is the package-owned read-only gate used by Doctor.
// Callers receive no registration or evidence-submission surface.
func PreflightRemedySync(ctx context.Context, homeDir string) bool {
	e, err := newProductionExecutor(homeDir)
	if err != nil {
		return false
	}
	_, err = e.preflight(ctx, RemedySync)
	return err == nil
}

func newProductionExecutor(homeDir string) (*executor, error) {
	return newExecutorWithPlatform(runtimePlatform, syncOperation{homeDir: homeDir})
}

func runtimePlatform() Platform {
	switch runtime.GOOS {
	case "windows":
		return PlatformWindows
	case "darwin":
		return PlatformMacOS
	case "linux":
		return PlatformLinux
	default:
		return ""
	}
}

func newExecutorWithPlatform(detector func() Platform, operations ...managedOperation) (*executor, error) {
	if detector == nil {
		return nil, errPreflightDenied
	}
	registered, err := newRegistry(operations...)
	if err != nil {
		return nil, err
	}
	return &executor{registry: &registered, platformDetector: detector}, nil
}

func newRegistry(operations ...managedOperation) (registry, error) {
	registered := make(map[RemedyID]managedOperation, len(operations))
	for _, operation := range operations {
		id := operationID(operation)
		if id == "" {
			return registry{}, errPreflightDenied
		}
		if _, exists := registered[id]; exists {
			return registry{}, errPreflightDenied
		}
		registered[id] = operation
	}
	return registry{operations: registered}, nil
}

func operationID(operation managedOperation) (id RemedyID) {
	if operation == nil {
		return ""
	}
	value := reflect.ValueOf(operation)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		if value.IsNil() {
			return ""
		}
	}
	defer func() {
		if recover() != nil {
			id = ""
		}
	}()
	return operation.id()
}

func (e *executor) preflight(ctx context.Context, id RemedyID) (preflightResult, error) {
	if e == nil || e.registry == nil || contextState(ctx) != nil {
		return preflightDenied()
	}
	operation, ok := e.registry.operations[id]
	if !ok || operationID(operation) != id {
		return preflightDenied()
	}
	remedy := NewRemedy(id, "")
	if remedy.Category == "" || !remedy.Eligible || remedy.ActionMode != ActionConfirmation {
		return preflightDenied()
	}
	platform := e.detectPlatform()
	if !supportsPlatform(remedy, platform) {
		return preflightDenied()
	}
	evidence, err := runPreflight(operation, ctx, platform)
	if contextState(ctx) != nil || err != nil || operationID(operation) != id || !validateEvidence(id, platform, evidence) {
		return preflightDenied()
	}
	return preflightResult{executor: e, id: id, category: evidence.category, actionMode: evidence.actionMode, platform: platform}, nil
}

func preflightDenied() (preflightResult, error) { return preflightResult{}, errPreflightDenied }

func runPreflight(operation managedOperation, ctx context.Context, platform Platform) (e preflightEvidence, err error) {
	defer func() {
		if recover() != nil {
			err = errPreflightDenied
		}
	}()
	return operation.preflight(ctx, platform)
}

func validateEvidence(id RemedyID, platform Platform, evidence preflightEvidence) bool {
	remedy := NewRemedy(id, "")
	if evidence.id != id || remedy.Category == "" || evidence.category != remedy.Category ||
		!remedy.Eligible || !evidence.eligible || evidence.actionMode != remedy.ActionMode ||
		remedy.ActionMode != ActionConfirmation || evidence.platform != platform ||
		!supportsPlatform(remedy, platform) || !evidence.prerequisitesReady ||
		!evidence.ownershipVerified || !evidence.containmentVerified {
		return false
	}
	return true
}

func supportsPlatform(remedy *Remedy, platform Platform) bool {
	for _, supported := range remedy.SupportedPlatforms {
		if supported == platform {
			return true
		}
	}
	return false
}

func contextState(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}

func (e *executor) detectPlatform() (platform Platform) {
	defer func() {
		if recover() != nil {
			platform = ""
		}
	}()
	if e.platformDetector != nil {
		platform = e.platformDetector()
	}
	return platform
}

type syncOperation struct{ homeDir string }

func (syncOperation) id() RemedyID { return RemedySync }

func (o syncOperation) preflight(ctx context.Context, platform Platform) (preflightEvidence, error) {
	if contextState(ctx) != nil || platform == "" || !filepath.IsAbs(o.homeDir) {
		return preflightEvidence{}, errPreflightDenied
	}
	rootInfo, err := os.Stat(o.homeDir)
	if err != nil || !rootInfo.IsDir() {
		return preflightEvidence{}, errPreflightDenied
	}
	installed, err := state.Read(o.homeDir)
	if err != nil || len(installed.InstalledAgents) == 0 {
		return preflightEvidence{}, errPreflightDenied
	}

	missing := false
	for _, agentID := range installed.InstalledAgents {
		target, ok := syncAgentConfigDir(o.homeDir, agentID)
		if !ok || !syncTargetContained(o.homeDir, target) {
			return preflightEvidence{}, errPreflightDenied
		}
		info, statErr := os.Stat(target)
		switch {
		case os.IsNotExist(statErr):
			missing = true
		case statErr != nil || !info.IsDir():
			return preflightEvidence{}, errPreflightDenied
		}
	}
	if !missing {
		return preflightEvidence{}, errPreflightDenied
	}
	return preflightEvidence{
		id:                  RemedySync,
		category:            RemedyCategoryConfiguration,
		actionMode:          ActionConfirmation,
		platform:            platform,
		eligible:            true,
		prerequisitesReady:  true,
		ownershipVerified:   true,
		containmentVerified: true,
	}, nil
}

func syncAgentConfigDir(homeDir, agentID string) (string, bool) {
	configBase := filepath.Join(homeDir, ".config")
	switch agentID {
	case "claude-code":
		return filepath.Join(homeDir, ".claude"), true
	case "opencode":
		return filepath.Join(configBase, "opencode"), true
	case "cursor":
		return filepath.Join(homeDir, ".cursor"), true
	case "windsurf":
		return filepath.Join(homeDir, ".codeium", "windsurf"), true
	case "vscode":
		return filepath.Join(configBase, "Code"), true
	case "codex":
		return filepath.Join(homeDir, ".codex"), true
	case "kiro":
		return filepath.Join(homeDir, ".kiro"), true
	default:
		return "", false
	}
}

func containedPath(root, target string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func syncTargetContained(root, target string) bool {
	if !containedPath(root, target) {
		return false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	for current := target; ; current = filepath.Dir(current) {
		resolvedTarget, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			return containedPath(resolvedRoot, resolvedTarget)
		}
		if !os.IsNotExist(evalErr) || filepath.Dir(current) == current {
			return false
		}
	}
}
