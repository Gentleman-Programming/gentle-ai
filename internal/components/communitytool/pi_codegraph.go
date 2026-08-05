package communitytool

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	piagent "github.com/gentleman-programming/gentle-ai/v2/internal/agents/pi"
	"github.com/gentleman-programming/gentle-ai/v2/internal/components/filemerge"
	"github.com/gentleman-programming/gentle-ai/v2/internal/system"
)

const (
	piCodeGraphToolMarker     = "<!-- gentle-ai:pi-codegraph-tool -->"
	piCodeGraphGuidanceMarker = "<!-- gentle-ai:pi-codegraph-guidance -->"
	piCodeGraphEndMarker      = "<!-- /gentle-ai:pi-codegraph -->"
)

var (
	piCodeGraphAtomicWrite  = filemerge.WriteFileAtomic
	piCodeGraphReadFile     = os.ReadFile
	piCodeGraphRemove       = os.Remove
	piCodeGraphManifestStat = os.Stat
)

var piCodeGraphEffectiveMCPProbe PiCodeGraphEffectiveMCPProbe = probePiCodeGraphMCP

// ErrPiCodeGraphAdapterHealthUnavailable means Pi supplied no supported,
// machine-verifiable adapter-health evidence. CodeGraph's direct MCP schema
// verification remains separate capability evidence.
var ErrPiCodeGraphAdapterHealthUnavailable = errors.New("Pi MCP adapter health is not machine-verifiable")

const piCodeGraphPendingAction = "Pi CodeGraph integration remains pending: CodeGraph configuration was installed and preserved, and direct MCP capability was verified. Pi adapter activation health cannot be machine-verified on the detected Pi version."

type PiChildClassification string

const (
	PiChildCompatible    PiChildClassification = "compatible"
	PiChildGuidanceOnly  PiChildClassification = "guidance-only"
	PiChildUnavailable   PiChildClassification = "unavailable"
	PiChildMisconfigured PiChildClassification = "misconfigured"
)

type PiCodeGraphChild struct {
	Name           string
	Source         string
	Target         string
	Classification PiChildClassification
	Tools          []string
	Guidance       bool
	Reason         string
}

type PiCodeGraphOptions struct {
	HomeDir           string
	WorkspaceDir      string
	Selected          bool
	EffectiveMCPProbe PiCodeGraphEffectiveMCPProbe
}

type PiCodeGraphResult struct {
	Changed       bool
	Files         []string
	Children      []PiCodeGraphChild
	MCP           PiCodeGraphMCPVerification
	ManualActions []string
}

// PiCodeGraphMCPVerification records the observed Pi MCP adapter contract.
// The verification is based on the effective Pi MCP config and child tool
// allowlists, never on a parent prompt marker.
type PiCodeGraphMCPVerification struct {
	Adapter         bool
	ReadOnlyExplore bool
	Tools           []string
}

// PiCodeGraphMCPTool is the observed MCP tools/list item required by Pi.
type PiCodeGraphMCPTool struct {
	Name        string         `json:"name"`
	InputSchema map[string]any `json:"inputSchema"`
}

// PiCodeGraphMCPProbeResult captures runtime evidence from Pi's installed MCP
// adapter and the configured CodeGraph stdio server.
type PiCodeGraphMCPProbeResult struct {
	AdapterAvailable bool
	Initialized      bool
	Tools            []PiCodeGraphMCPTool
}

// PreservePiCodeGraphPending converts only unavailable adapter-health evidence
// into the manual action used while Pi has no verifiable health signal.
func PreservePiCodeGraphPending(result PiCodeGraphResult, err error) (PiCodeGraphResult, error) {
	if !isExclusivePiCodeGraphPending(err) {
		return result, err
	}
	if !slices.Contains(result.ManualActions, piCodeGraphPendingAction) {
		result.ManualActions = append(result.ManualActions, piCodeGraphPendingAction)
	}
	return result, nil
}

func isExclusivePiCodeGraphPending(err error) bool {
	if err == ErrPiCodeGraphAdapterHealthUnavailable {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !isExclusivePiCodeGraphPending(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isExclusivePiCodeGraphPending(wrapped.Unwrap())
	}
	return false
}

// PiCodeGraphEffectiveMCPProbe initializes the configured MCP server through
// the installed Pi adapter contract and returns its observed tools/list data.
type PiCodeGraphEffectiveMCPProbe func(mcpPath string) (PiCodeGraphMCPProbeResult, error)

type piCodeGraphManifest struct {
	MCPPath  string                          `json:"mcpPath"`
	MCP      *piCodeGraphOwnedFile           `json:"mcp,omitempty"`
	Children map[string]piCodeGraphOwnedFile `json:"children"`
}

// piCodeGraphOwnedFile is a bounded before-image. It lets removal restore a
// user file exactly and lets sync recreate an owned artifact that was removed.
type piCodeGraphOwnedFile struct {
	Before    *string `json:"before,omitempty"`
	After     string  `json:"after"`
	AfterHash string  `json:"afterHash"`
	Overlay   bool    `json:"overlay"`
	Adopted   bool    `json:"adopted,omitempty"`
	Mode      uint32  `json:"mode,omitempty"`
}

// ReconcilePiCodeGraph owns the optional Pi integration. It never writes to
// gentle-pi package paths; package children are copied into a Pi overlay.
func ReconcilePiCodeGraph(options PiCodeGraphOptions) (result PiCodeGraphResult, err error) {
	if !options.Selected {
		return UninstallPiCodeGraph(options.HomeDir)
	}
	paths := piagent.CodeGraphPaths(options.HomeDir)
	manifest, err := readPiCodeGraphManifest(paths.Manifest)
	if err != nil {
		return result, err
	}
	if manifest.Children == nil {
		manifest.Children = map[string]piCodeGraphOwnedFile{}
	}
	journal := newPiJournal(piCodeGraphAllowedRoots(paths, options.WorkspaceDir)...)
	defer func() {
		if err != nil {
			if restoreErr := journal.restore(); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore Pi CodeGraph journal: %w", restoreErr))
			}
			result.MCP = PiCodeGraphMCPVerification{}
		}
	}()
	changed := map[string]struct{}{}
	if manifest.MCP, err = reconcilePiMCP(paths.MCPConfig, journal, changed, manifest.MCP); err != nil {
		return result, err
	}
	if err = restoreMissingPiChildren(manifest.Children, journal, changed); err != nil {
		return result, err
	}
	children, err := piagent.DiscoverCodeGraphChildren(options.HomeDir, options.WorkspaceDir)
	if err != nil {
		return result, err
	}
	manifest.MCPPath = paths.MCPConfig
	for _, discovered := range children {
		if safeErr := journal.validate(discovered.Source); safeErr != nil {
			return result, safeErr
		}
		body, readErr := os.ReadFile(discovered.Source)
		child := PiCodeGraphChild{Name: discovered.Name, Source: discovered.Source, Target: discovered.Target}
		if readErr != nil {
			child.Classification, child.Reason = PiChildUnavailable, readErr.Error()
			result.Children = append(result.Children, child)
			continue
		}
		tools, parseable, malformed := piChildTools(string(body))
		if malformed {
			child.Classification, child.Reason = PiChildMisconfigured, "child tools frontmatter is malformed"
			result.Children = append(result.Children, child)
			continue
		}
		if !parseable {
			child.Classification, child.Reason = PiChildGuidanceOnly, "child has no explicit parseable tools"
		} else if slices.Contains(tools, "bash") {
			child.Classification = PiChildCompatible
			tools = appendUnique(tools, "mcp")
		} else {
			child.Classification, child.Reason = PiChildGuidanceOnly, "child does not allow bash"
		}
		if child.Classification != PiChildCompatible && strings.Contains(string(body), piCodeGraphToolMarker) {
			child.Classification, child.Reason = PiChildMisconfigured, "child CodeGraph tool block conflicts with its effective tool allowlist"
			result.Children = append(result.Children, child)
			continue
		}
		updated, renderErr := renderPiChild(string(body), tools, child.Classification == PiChildCompatible)
		if renderErr != nil {
			return result, fmt.Errorf("render Pi child %q: %w", child.Name, renderErr)
		}
		if strings.Contains(string(body), piCodeGraphGuidanceMarker) &&
			(child.Classification != PiChildCompatible || strings.Contains(string(body), piCodeGraphToolMarker)) {
			updated = string(body)
		}
		if updated != string(body) || discovered.PackageOwned {
			if writeErr := journal.write(discovered.Target, []byte(updated)); writeErr != nil {
				return result, writeErr
			}
			changed[discovered.Target] = struct{}{}
		}
		child.Tools, child.Guidance = tools, true
		if child.Classification == "" {
			child.Classification = PiChildGuidanceOnly
		}
		if _, exists := manifest.Children[discovered.Target]; !exists {
			adopted := !discovered.PackageOwned && updated == string(body)
			if adopted {
				if captureErr := journal.capture(discovered.Target); captureErr != nil {
					return result, captureErr
				}
			}
			manifest.Children[discovered.Target] = journal.ownedFile(discovered.Target, updated, discovered.PackageOwned, adopted)
		}
		result.Children = append(result.Children, child)
	}
	effectiveMCPPath, effectiveErr := piagent.EffectiveCodeGraphMCPPath(options.HomeDir, options.WorkspaceDir)
	if effectiveErr != nil {
		return result, effectiveErr
	}
	probe := options.EffectiveMCPProbe
	if probe == nil {
		probe = piCodeGraphEffectiveMCPProbe
		if effectiveMCPPath != paths.MCPConfig {
			probe = func(mcpPath string) (PiCodeGraphMCPProbeResult, error) {
				return probePiCodeGraphMCPWithAgentDir(mcpPath, paths.AgentDir)
			}
		}
	}
	if err = verifyPiCodeGraphWithProbe(effectiveMCPPath, result.Children, probe); err != nil {
		result, err = PreservePiCodeGraphPending(result, err)
		if err != nil {
			return result, err
		}
	} else {
		result.MCP, err = verifyPiMCPWithProbe(effectiveMCPPath, probe)
		if err != nil {
			return result, err
		}
	}
	encoded, marshalErr := json.MarshalIndent(manifest, "", "  ")
	if marshalErr != nil {
		return result, marshalErr
	}
	encoded = append(encoded, '\n')
	currentManifest, readErr := os.ReadFile(paths.Manifest)
	if readErr != nil && !os.IsNotExist(readErr) {
		return result, readErr
	}
	if string(currentManifest) != string(encoded) {
		if err = journal.write(paths.Manifest, encoded); err != nil {
			return result, err
		}
		changed[paths.Manifest] = struct{}{}
	}
	for path := range changed {
		result.Files = append(result.Files, path)
	}
	slices.Sort(result.Files)
	result.Changed = len(result.Files) > 0
	return result, nil
}

func reconcilePiMCP(path string, journal *piJournal, changed map[string]struct{}, existing *piCodeGraphOwnedFile) (*piCodeGraphOwnedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return existing, fmt.Errorf("read Pi MCP config: %w", err)
	}
	root := map[string]any{}
	if len(data) > 0 && json.Unmarshal(data, &root) != nil {
		return existing, fmt.Errorf("misconfigured Pi MCP config %q", path)
	}
	servers, isObject := root["mcpServers"].(map[string]any)
	if root["mcpServers"] != nil && !isObject {
		return existing, fmt.Errorf("misconfigured Pi MCP config %q: mcpServers must be an object", path)
	}
	if servers == nil {
		servers = map[string]any{}
		root["mcpServers"] = servers
	}
	desired := map[string]any{"command": "codegraph", "args": []any{"serve", "--mcp"}}
	if entry, found := servers["codegraph"]; found && !equivalentPiMCP(entry) {
		return existing, fmt.Errorf("misconfigured Pi CodeGraph MCP entry at %q; Gentle AI will not overwrite it", path)
	}
	if entry, found := servers["codegraph"]; found && equivalentPiMCP(entry) {
		return existing, nil
	}
	servers["codegraph"] = desired
	encoded, _ := json.MarshalIndent(root, "", "  ")
	if err := journal.write(path, append(encoded, '\n')); err != nil {
		return existing, err
	}
	changed[path] = struct{}{}
	owned := journal.ownedFile(path, string(append(encoded, '\n')), false, false)
	return &owned, nil
}

func equivalentPiMCP(value any) bool {
	server, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if server["command"] != "codegraph" {
		return false
	}
	args, ok := server["args"].([]any)
	return ok && len(args) == 2 && args[0] == "serve" && args[1] == "--mcp"
}

func piChildTools(body string) ([]string, bool, bool) {
	if !strings.HasPrefix(body, "---\n") {
		return nil, false, false
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return nil, false, true
	}
	frontmatter := body[4 : 4+end]
	lines := strings.Split(frontmatter, "\n")
	for i, line := range lines {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "tools" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			var tools []string
			for _, item := range lines[i+1:] {
				trimmed := strings.TrimSpace(item)
				if !strings.HasPrefix(item, " ") && !strings.HasPrefix(item, "\t") {
					break
				}
				if !strings.HasPrefix(trimmed, "- ") || strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")) == "" {
					return nil, false, true
				}
				tools = append(tools, strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), "\"'"))
			}
			if len(tools) == 0 {
				return nil, false, true
			}
			return tools, true, false
		}
		if strings.HasPrefix(value, "[") != strings.HasSuffix(value, "]") {
			return nil, false, true
		}
		tools := strings.FieldsFunc(strings.Trim(value, "[]"), func(r rune) bool { return r == ',' || r == ' ' })
		if len(tools) == 0 {
			return nil, false, true
		}
		for i := range tools {
			tools[i] = strings.Trim(tools[i], "\"'")
		}
		return tools, true, false
	}
	return nil, false, false
}

func renderPiChild(body string, tools []string, injectTools bool) (string, error) {
	var err error
	body, err = stripPiCodeGraphBlocks(body)
	if err != nil {
		return "", err
	}
	if injectTools {
		body = replacePiChildTools(body, tools)
		body += "\n\n" + piCodeGraphToolMarker + "\nUse the Pi MCP proxy tool `mcp` for the read-only CodeGraph server.\n" + piCodeGraphEndMarker
	}
	return strings.TrimRight(body, "\n") + "\n\n" + piCodeGraphGuidanceMarker + "\n" + CodeGraphGuidanceMarkdown() + "\n" + piCodeGraphEndMarker + "\n", nil
}

func replacePiChildTools(body string, tools []string) string {
	end := strings.Index(body[4:], "\n---")
	if !strings.HasPrefix(body, "---\n") || end < 0 {
		return body
	}
	frontEnd := 4 + end
	front := body[:frontEnd]
	lines := strings.Split(front, "\n")
	for i, line := range lines {
		if key, _, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) == "tools" {
			lines[i] = "tools: " + strings.Join(tools, ", ")
			end := i + 1
			for end < len(lines) && (strings.HasPrefix(lines[end], " ") || strings.HasPrefix(lines[end], "\t")) {
				end++
			}
			lines = append(lines[:i+1], lines[end:]...)
			break
		}
	}
	return strings.Join(lines, "\n") + body[frontEnd:]
}

func stripPiCodeGraphBlocks(body string) (string, error) {
	for _, marker := range []string{piCodeGraphToolMarker, piCodeGraphGuidanceMarker} {
		for {
			start := strings.Index(body, marker)
			if start < 0 {
				break
			}
			end := strings.Index(body[start:], piCodeGraphEndMarker)
			if end < 0 {
				return body, fmt.Errorf("unmatched managed marker %q", marker)
			}
			body = strings.TrimRight(body[:start], "\n") + strings.TrimLeft(body[start+end+len(piCodeGraphEndMarker):], "\n")
		}
	}
	return body, nil
}

func verifyPiCodeGraph(mcpPath string, children []PiCodeGraphChild) error {
	return verifyPiCodeGraphWithProbe(mcpPath, children, piCodeGraphEffectiveMCPProbe)
}

func verifyPiCodeGraphWithProbe(mcpPath string, children []PiCodeGraphChild, probe PiCodeGraphEffectiveMCPProbe) error {
	for _, child := range children {
		if child.Classification == PiChildMisconfigured {
			return fmt.Errorf("Pi child %q is misconfigured: %s", child.Name, child.Reason)
		}
		if child.Classification == PiChildUnavailable {
			continue
		}
		body, err := os.ReadFile(child.Target)
		if err != nil || !strings.Contains(string(body), piCodeGraphGuidanceMarker) {
			return fmt.Errorf("Pi child %q lacks CodeGraph lazy-init guidance", child.Name)
		}
		if child.Classification == PiChildCompatible && (!strings.Contains(string(body), piCodeGraphToolMarker) || !slices.Contains(child.Tools, "bash") || !slices.Contains(child.Tools, "mcp")) {
			return fmt.Errorf("Pi child %q lacks verified CodeGraph tools", child.Name)
		}
	}
	if _, err := verifyPiMCPWithProbe(mcpPath, probe); err != nil {
		return err
	}
	return nil
}

func verifyPiMCP(mcpPath string) (PiCodeGraphMCPVerification, error) {
	return verifyPiMCPWithProbe(mcpPath, piCodeGraphEffectiveMCPProbe)
}

func verifyPiMCPWithProbe(mcpPath string, probe PiCodeGraphEffectiveMCPProbe) (PiCodeGraphMCPVerification, error) {
	data, err := os.ReadFile(mcpPath)
	root := map[string]any{}
	if err != nil || json.Unmarshal(data, &root) != nil {
		return PiCodeGraphMCPVerification{}, fmt.Errorf("Pi CodeGraph MCP transport is not configured")
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil || !equivalentPiMCP(servers["codegraph"]) {
		return PiCodeGraphMCPVerification{}, fmt.Errorf("Pi CodeGraph MCP transport is not configured")
	}
	if probe == nil {
		return PiCodeGraphMCPVerification{}, fmt.Errorf("Pi CodeGraph MCP capability probe is not configured")
	}
	result, err := probe(mcpPath)
	if err != nil && !errors.Is(err, ErrPiCodeGraphAdapterHealthUnavailable) {
		return PiCodeGraphMCPVerification{}, fmt.Errorf("Pi CodeGraph MCP capability probe failed: %w", err)
	}
	if !result.AdapterAvailable || !result.Initialized {
		return PiCodeGraphMCPVerification{}, fmt.Errorf("Pi CodeGraph MCP capability probe did not observe an available adapter and initialized server")
	}
	if !isReadOnlyCodeGraphExploreSchema(result.Tools) {
		return PiCodeGraphMCPVerification{}, fmt.Errorf("Pi CodeGraph MCP tools/list does not expose the required read-only codegraph_explore schema")
	}
	verification := PiCodeGraphMCPVerification{Adapter: true, ReadOnlyExplore: true, Tools: []string{"codegraph_explore"}}
	if err != nil {
		return verification, ErrPiCodeGraphAdapterHealthUnavailable
	}
	return verification, nil
}

func probePiCodeGraphMCP(mcpPath string) (PiCodeGraphMCPProbeResult, error) {
	return probePiCodeGraphMCPWithAgentDir(mcpPath, filepath.Dir(mcpPath))
}

func probePiCodeGraphMCPWithAgentDir(mcpPath, agentDir string) (PiCodeGraphMCPProbeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return probePiCodeGraphMCPWithAgentDirContext(ctx, mcpPath, agentDir)
}

func probePiCodeGraphMCPWithAgentDirContext(ctx context.Context, mcpPath, agentDir string) (probeResult PiCodeGraphMCPProbeResult, returnErr error) {
	adapterPath := filepath.Join(agentDir, "npm", "node_modules", "pi-mcp-adapter", "index.ts")
	if _, err := os.Stat(adapterPath); err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("Pi MCP adapter extension is unavailable at %q: %w", adapterPath, err)
	}
	command := exec.CommandContext(ctx, "codegraph", "serve", "--mcp")
	system.EnsureCommandDir(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return PiCodeGraphMCPProbeResult{}, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return PiCodeGraphMCPProbeResult{}, err
	}
	if err := command.Start(); err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("start CodeGraph MCP server: %w", err)
	}
	return probePiCodeGraphMCPWithTransport(ctx, stdin, stdout, func() (error, error) {
		return command.Process.Kill(), command.Wait()
	})
}

func probePiCodeGraphMCPWithTransport(ctx context.Context, stdin io.WriteCloser, stdout io.Reader, cleanup func() (error, error)) (probeResult PiCodeGraphMCPProbeResult, returnErr error) {
	phase := "MCP initialize"
	defer func() {
		_ = stdin.Close()
		killErr, waitErr := cleanup()
		if errors.Is(returnErr, context.DeadlineExceeded) {
			return
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			probeResult = PiCodeGraphMCPProbeResult{}
			returnErr = fmt.Errorf("%s: %w", phase, context.DeadlineExceeded)
			return
		}
		if returnErr == nil && errors.Is(killErr, os.ErrProcessDone) && waitErr != nil {
			returnErr = fmt.Errorf("wait for CodeGraph MCP server: %w", waitErr)
		}
	}()

	encoder := json.NewEncoder(stdin)
	decoder := json.NewDecoder(bufio.NewReader(stdout))
	if err := encoder.Encode(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "gentle-ai", "version": "1"}},
	}); err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("send MCP initialize: %w", err)
	}
	initializeResponse, err := readPiMCPResponse(decoder, 1)
	if err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("MCP initialize: %w", piCodeGraphMCPDeadlineError(ctx, "read response", err))
	}
	initializeResult, ok := initializeResponse["result"].(map[string]any)
	protocolVersion, versionOK := initializeResult["protocolVersion"].(string)
	if initializeResponse["jsonrpc"] != "2.0" || !ok || !versionOK || strings.TrimSpace(protocolVersion) == "" {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("MCP initialize: invalid JSON-RPC 2.0 result")
	}
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}); err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("send MCP initialized notification: %w", err)
	}
	phase = "MCP tools/list"
	if err := encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}}); err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("send MCP tools/list: %w", err)
	}
	response, err := readPiMCPResponse(decoder, 2)
	if err != nil {
		return PiCodeGraphMCPProbeResult{}, fmt.Errorf("MCP tools/list: %w", piCodeGraphMCPDeadlineError(ctx, "read response", err))
	}
	result, _ := response["result"].(map[string]any)
	rawTools, _ := result["tools"].([]any)
	tools := make([]PiCodeGraphMCPTool, 0, len(rawTools))
	for _, rawTool := range rawTools {
		encoded, marshalErr := json.Marshal(rawTool)
		if marshalErr != nil {
			return PiCodeGraphMCPProbeResult{}, marshalErr
		}
		var tool PiCodeGraphMCPTool
		if err := json.Unmarshal(encoded, &tool); err != nil {
			return PiCodeGraphMCPProbeResult{}, err
		}
		tools = append(tools, tool)
	}
	return PiCodeGraphMCPProbeResult{AdapterAvailable: true, Initialized: true, Tools: tools}, ErrPiCodeGraphAdapterHealthUnavailable
}

func piCodeGraphMCPDeadlineError(ctx context.Context, phase string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w", phase, context.DeadlineExceeded)
	}
	return err
}

func readPiMCPResponse(decoder *json.Decoder, id int) (map[string]any, error) {
	for {
		var response map[string]any
		if err := decoder.Decode(&response); err != nil {
			return nil, err
		}
		responseID, ok := response["id"].(float64)
		if ok && int(responseID) == id {
			if rpcError, exists := response["error"]; exists {
				return nil, fmt.Errorf("%v", rpcError)
			}
			return response, nil
		}
	}
}

func isReadOnlyCodeGraphExploreSchema(tools []PiCodeGraphMCPTool) bool {
	if len(tools) != 1 || tools[0].Name != "codegraph_explore" {
		return false
	}
	schema := tools[0].InputSchema
	if schema["type"] != "object" {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != 3 || !hasSchemaType(properties, "query", "string") ||
		(!hasSchemaType(properties, "maxFiles", "integer") && !hasSchemaType(properties, "maxFiles", "number")) ||
		!hasSchemaType(properties, "projectPath", "string") {
		return false
	}
	required, ok := schema["required"].([]any)
	if !ok || len(required) == 0 || len(required) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, field := range required {
		name, ok := field.(string)
		if !ok || (name != "query" && name != "projectPath") || seen[name] {
			return false
		}
		seen[name] = true
	}
	return seen["query"]
}

func hasSchemaType(properties map[string]any, name, want string) bool {
	property, ok := properties[name].(map[string]any)
	return ok && property["type"] == want
}

// PiCodeGraphConfigured verifies child capability directly. Parent prompt
// markers are intentionally not consulted because they cannot prove child MCP
// tools or per-child lazy-init guidance.
func PiCodeGraphConfigured(homeDir, workspaceDir string) (bool, string) {
	configured, reason, _ := inspectPiCodeGraph(homeDir, workspaceDir)
	return configured, reason
}

func inspectPiCodeGraph(homeDir, workspaceDir string) (bool, string, []PiCodeGraphChild) {
	paths := piagent.CodeGraphPaths(homeDir)
	children, err := piagent.DiscoverCodeGraphChildren(homeDir, workspaceDir)
	if err != nil {
		return false, err.Error(), nil
	}
	if len(children) == 0 {
		if _, err := os.Stat(paths.Manifest); err != nil {
			return false, "no effective Pi children were discovered and no Gentle-AI ownership record exists", nil
		}
		if err := verifyPiCodeGraph(paths.MCPConfig, nil); err != nil {
			return false, err.Error(), nil
		}
		return true, "verified Pi MCP transport; no effective children were discovered", nil
	}
	reports := make([]PiCodeGraphChild, 0, len(children))
	for _, child := range children {
		body, err := os.ReadFile(child.Target)
		if err != nil {
			return false, fmt.Sprintf("cannot read Pi child %q: %v", child.Name, err), reports
		}
		tools, parseable, malformed := piChildTools(string(body))
		if malformed {
			reports = append(reports, PiCodeGraphChild{Name: child.Name, Source: child.Source, Target: child.Target, Classification: PiChildMisconfigured, Reason: "child tools frontmatter is malformed"})
			continue
		}
		classification := PiChildGuidanceOnly
		if parseable && slices.Contains(tools, "bash") {
			classification = PiChildCompatible
		}
		if classification != PiChildCompatible && strings.Contains(string(body), piCodeGraphToolMarker) {
			return false, fmt.Sprintf("Pi child %q has stale CodeGraph tools without bash support", child.Name), reports
		}
		reports = append(reports, PiCodeGraphChild{Name: child.Name, Source: child.Source, Target: child.Target, Tools: tools, Classification: classification})
	}
	if err := verifyPiCodeGraph(paths.MCPConfig, reports); err != nil {
		return false, err.Error(), reports
	}
	return true, "verified Pi MCP transport and every effective child", reports
}

// PiCodeGraphPaths returns only files Gentle AI may reconcile or remove.
func PiCodeGraphPaths(homeDir, workspaceDir string) []string {
	paths := piagent.CodeGraphPaths(homeDir)
	result := []string{paths.MCPConfig, paths.Manifest}
	if manifest, err := readPiCodeGraphManifest(paths.Manifest); err == nil {
		allowedRoots := piCodeGraphAllowedRoots(paths, workspaceDir)
		if manifest.MCPPath != "" && piCodeGraphPathWithinRoots(manifest.MCPPath, allowedRoots) {
			result = append(result, manifest.MCPPath)
		}
		for path := range manifest.Children {
			if piCodeGraphPathWithinRoots(path, allowedRoots) {
				result = append(result, path)
			}
		}
	}
	children, err := piagent.DiscoverCodeGraphChildren(homeDir, workspaceDir)
	if err != nil {
		return result
	}
	for _, child := range children {
		result = append(result, child.Target)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

// ValidatePiCodeGraphRoot enforces the lazy-init safety boundary used by the
// injected guidance before CodeGraph is initialized in a child workspace.
func ValidatePiCodeGraphRoot(root, homeDir string) error {
	clean := filepath.Clean(root)
	if root == "" || clean == string(filepath.Separator) || clean == filepath.Clean(homeDir) || clean == filepath.Clean(os.TempDir()) {
		return fmt.Errorf("unsafe CodeGraph root %q", root)
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("CodeGraph root %q is not a directory", root)
	}
	if _, err := os.Stat(filepath.Join(clean, ".git")); err != nil {
		return fmt.Errorf("CodeGraph root %q is not a project workspace", root)
	}
	return nil
}

// RefreshPiCodeGraphIfConfigured repairs selected Pi integration after sync.
func RefreshPiCodeGraphIfConfigured(homeDir, workspaceDir string) (PiCodeGraphResult, bool, error) {
	paths := piagent.CodeGraphPaths(homeDir)
	if _, err := os.Stat(paths.Manifest); os.IsNotExist(err) {
		return PiCodeGraphResult{}, false, nil
	} else if err != nil {
		return PiCodeGraphResult{}, false, err
	}
	result, err := ReconcilePiCodeGraph(PiCodeGraphOptions{HomeDir: homeDir, WorkspaceDir: workspaceDir, Selected: true})
	return result, true, err
}

// UninstallPiCodeGraph removes only manifest-owned entries. Drifted child files
// are preserved and surfaced for manual review.
func UninstallPiCodeGraph(homeDir string) (result PiCodeGraphResult, err error) {
	paths := piagent.CodeGraphPaths(homeDir)
	data, err := os.ReadFile(paths.Manifest)
	if os.IsNotExist(err) {
		return PiCodeGraphResult{}, nil
	}
	if err != nil {
		return PiCodeGraphResult{}, err
	}
	var manifest piCodeGraphManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return PiCodeGraphResult{}, err
	}
	journal := newPiJournal(piCodeGraphAllowedRoots(paths, "")...)
	if err = journal.capture(paths.Manifest); err != nil {
		return result, fmt.Errorf("capture Pi CodeGraph manifest: %w", err)
	}
	for path := range manifest.Children {
		if err = journal.capture(path); err != nil {
			return result, fmt.Errorf("capture Pi CodeGraph child %q: %w", path, err)
		}
	}
	if manifest.MCP != nil {
		if err = journal.capture(manifest.MCPPath); err != nil {
			return result, fmt.Errorf("capture Pi CodeGraph MCP %q: %w", manifest.MCPPath, err)
		}
	}
	defer func() {
		if err == nil {
			return
		}
		if restoreErr := journal.restore(); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore Pi CodeGraph uninstall journal: %w", restoreErr))
		}
		result = PiCodeGraphResult{}
	}()
	for path, owned := range manifest.Children {
		if safeErr := journal.validate(path); safeErr != nil {
			return result, safeErr
		}
		body, readErr := piCodeGraphReadFile(path)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return result, fmt.Errorf("read Pi CodeGraph child %q: %w", path, readErr)
		}
		if hashPiBytes(body) != owned.AfterHash {
			result.ManualActions = append(result.ManualActions, "Pi CodeGraph child drifted; preserved: "+path)
			continue
		}
		if err := restorePiOwnedFile(path, owned); err != nil {
			return result, err
		}
		result.Files = append(result.Files, path)
	}
	if manifest.MCP != nil {
		if safeErr := journal.validate(manifest.MCPPath); safeErr != nil {
			return result, safeErr
		}
		if current, readErr := piCodeGraphReadFile(manifest.MCPPath); readErr == nil && hashPiBytes(current) == manifest.MCP.AfterHash {
			if err := restorePiOwnedFile(manifest.MCPPath, *manifest.MCP); err != nil {
				return result, err
			}
			result.Files = append(result.Files, manifest.MCPPath)
		} else if readErr == nil {
			result.ManualActions = append(result.ManualActions, "Pi CodeGraph MCP drifted; preserved: "+manifest.MCPPath)
		} else if !os.IsNotExist(readErr) {
			return result, readErr
		}
	}
	if err := piCodeGraphRemove(paths.Manifest); err != nil && !os.IsNotExist(err) {
		return result, err
	}
	result.Files = append(result.Files, paths.Manifest)
	result.Changed = len(result.Files) > 0
	return result, nil
}

func restorePiOwnedFile(path string, owned piCodeGraphOwnedFile) error {
	if owned.Before == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	mode := os.FileMode(owned.Mode)
	if mode == 0 {
		mode = 0o600
	}
	_, err := piCodeGraphAtomicWrite(path, []byte(*owned.Before), mode)
	return err
}

func readPiCodeGraphManifest(path string) (piCodeGraphManifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return piCodeGraphManifest{Children: map[string]piCodeGraphOwnedFile{}}, nil
	}
	if err != nil {
		return piCodeGraphManifest{}, err
	}
	info, err := piCodeGraphManifestStat(path)
	if err != nil {
		return piCodeGraphManifest{}, fmt.Errorf("stat Pi CodeGraph manifest %q: %w", path, err)
	}
	if !piCodeGraphManifestPermissionsSafe(runtime.GOOS, info.Mode()) {
		return piCodeGraphManifest{}, fmt.Errorf("Pi CodeGraph manifest %q has unsafe permissions %#o", path, info.Mode().Perm())
	}
	var manifest piCodeGraphManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return piCodeGraphManifest{}, err
	}
	return manifest, nil
}

func piCodeGraphManifestPermissionsSafe(goos string, mode os.FileMode) bool {
	return goos == "windows" || mode.Perm()&0o077 == 0
}

func restoreMissingPiChildren(children map[string]piCodeGraphOwnedFile, journal *piJournal, changed map[string]struct{}) error {
	paths := make([]string, 0, len(children))
	for path := range children {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		owned := children[path]
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := journal.writeWithMode(path, []byte(owned.After), os.FileMode(owned.Mode)); err != nil {
			return err
		}
		changed[path] = struct{}{}
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	if !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}
func hashPiBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

type piJournalFile struct {
	data *[]byte
	mode os.FileMode
}
type piJournal struct {
	before map[string]*piJournalFile
	roots  []string
}

func newPiJournal(roots ...string) *piJournal {
	return &piJournal{before: map[string]*piJournalFile{}, roots: roots}
}
func (j *piJournal) write(path string, data []byte) error {
	return j.writeWithMode(path, data, 0)
}
func (j *piJournal) writeWithMode(path string, data []byte, mode os.FileMode) error {
	if err := j.validate(path); err != nil {
		return err
	}
	if _, ok := j.before[path]; !ok {
		old, err := os.ReadFile(path)
		if err == nil {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return statErr
			}
			j.before[path] = &piJournalFile{data: &old, mode: info.Mode().Perm()}
		} else if os.IsNotExist(err) {
			j.before[path] = nil
		} else {
			return err
		}
	}
	if mode == 0 {
		mode = 0o600
	}
	if previous := j.before[path]; previous != nil {
		mode = previous.mode
	}
	_, err := piCodeGraphAtomicWrite(path, data, mode)
	return err
}
func (j *piJournal) capture(path string) error {
	if err := j.validate(path); err != nil {
		return err
	}
	if _, exists := j.before[path]; exists {
		return nil
	}
	before, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		j.before[path] = nil
		return nil
	}
	if err != nil {
		return err
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return statErr
	}
	j.before[path] = &piJournalFile{data: &before, mode: info.Mode().Perm()}
	return nil
}
func (j *piJournal) ownedFile(path, after string, overlay, adopted bool) piCodeGraphOwnedFile {
	before := j.before[path]
	var snapshot *string
	if before != nil {
		value := string(*before.data)
		snapshot = &value
	}
	mode := uint32(0o600)
	if before != nil {
		mode = uint32(before.mode)
	}
	return piCodeGraphOwnedFile{Before: snapshot, After: after, AfterHash: hashPiBytes([]byte(after)), Overlay: overlay, Adopted: adopted, Mode: mode}
}
func (j *piJournal) restore() error {
	paths := make([]string, 0, len(j.before))
	for path := range j.before {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	var restoreErrors []error
	for _, path := range paths {
		old := j.before[path]
		if old == nil {
			if err := piCodeGraphRemove(path); err != nil && !os.IsNotExist(err) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove %q: %w", path, err))
			}
			continue
		}
		if _, err := piCodeGraphAtomicWrite(path, *old.data, old.mode); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %q: %w", path, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func piCodeGraphAllowedRoots(paths piagent.CodeGraphPathSet, workspace string) []string {
	roots := []string{paths.AgentDir, filepath.Dir(paths.Manifest)}
	if workspace != "" {
		roots = append(roots, workspace)
	}
	return roots
}

func piCodeGraphPathWithinRoots(path string, roots []string) bool {
	canonicalPath, err := canonicalPiCodeGraphPath(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		canonicalRoot, err := canonicalPiCodeGraphPath(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(canonicalRoot, canonicalPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func canonicalPiCodeGraphPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(abs)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", err
		}
		suffix = append(suffix, filepath.Base(abs))
		abs = parent
	}
}

func (j *piJournal) validate(path string) error {
	if path == "" {
		return fmt.Errorf("empty Pi CodeGraph path")
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symlink Pi CodeGraph path %q", path)
	}
	if piCodeGraphPathWithinRoots(path, j.roots) {
		return nil
	}
	return fmt.Errorf("Pi CodeGraph path %q escapes allowed roots", path)
}
