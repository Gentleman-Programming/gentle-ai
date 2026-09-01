package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	catalogTimeout   = 15 * time.Second
	maxCatalogOutput = 1 << 20
)

// Command describes the bounded OpenCode command used for runtime discovery.
type Command struct {
	Path        string
	Args        []string
	Dir         string
	OutputLimit int
}

type CommandOutput struct{ Stdout, Stderr []byte }
type CommandRunner func(context.Context, Command) (CommandOutput, error)

type CatalogErrorKind string

const (
	CatalogErrorMissingBinary     CatalogErrorKind = "missing_binary"
	CatalogErrorTimeout           CatalogErrorKind = "timeout"
	CatalogErrorCommandFailed     CatalogErrorKind = "command_failed"
	CatalogErrorOutputTooLarge    CatalogErrorKind = "output_too_large"
	CatalogErrorMalformed         CatalogErrorKind = "malformed_output"
	CatalogErrorUnsupportedSchema CatalogErrorKind = "unsupported_schema"
)

// CatalogError intentionally exposes a category, never command output.
type CatalogError struct{ Kind CatalogErrorKind }

func (e *CatalogError) Error() string { return string(e.Kind) }

// DiscoverCatalog reads OpenCode's effective provider catalog for projectDir.
func DiscoverCatalog(ctx context.Context, projectDir string) (map[string]Provider, error) {
	return DiscoverCatalogWithRunner(ctx, projectDir, runCatalogCommand)
}

// DiscoverCatalogWithRunner permits deterministic command-boundary tests.
func DiscoverCatalogWithRunner(ctx context.Context, projectDir string, runner CommandRunner) (map[string]Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	output, err := runner(ctx, Command{Path: "opencode", Args: []string{"models", "--verbose"}, Dir: projectDir, OutputLimit: maxCatalogOutput})
	if err != nil {
		return nil, catalogCommandError(ctx, err)
	}
	if len(output.Stdout) > maxCatalogOutput || len(output.Stderr) > maxCatalogOutput {
		return nil, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	return parseVerboseCatalog(output.Stdout)
}

func catalogCommandError(ctx context.Context, err error) error {
	var catalogErr *CatalogError
	if errors.As(err, &catalogErr) {
		return catalogErr
	}
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return &CatalogError{Kind: CatalogErrorTimeout}
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) && (errors.Is(execErr.Err, os.ErrNotExist) || errors.Is(execErr.Err, exec.ErrNotFound)) {
		return &CatalogError{Kind: CatalogErrorMissingBinary}
	}
	return &CatalogError{Kind: CatalogErrorCommandFailed}
}

func parseVerboseCatalog(data []byte) (map[string]Provider, error) {
	providers := map[string]Provider{}
	sawNoise := false
	for len(bytes.TrimSpace(data)) > 0 {
		data = bytes.TrimLeft(data, "\r\n\t ")
		line, rest := data, []byte(nil)
		if end := bytes.IndexByte(data, '\n'); end >= 0 {
			line, rest = data[:end], data[end+1:]
		}
		header := strings.TrimSpace(string(line))
		if !isCatalogHeader(header) || !startsJSONObject(rest) {
			// Plugins and hooks (e.g. the managed skill-registry plugin outside
			// a project root) may log to stdout before or between catalog
			// records. Skip such lines instead of failing the whole discovery,
			// but remember them so noise-only output still fails closed.
			sawNoise = true
			data = rest
			continue
		}
		data = rest
		var raw struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Family       string `json:"family"`
			Capabilities *struct {
				ToolCall  *bool `json:"toolcall"`
				Reasoning *bool `json:"reasoning"`
			} `json:"capabilities"`
			Limit    ModelLimit                 `json:"limit"`
			Cost     ModelCost                  `json:"cost"`
			Variants map[string]json.RawMessage `json:"variants"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&raw); err != nil {
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
			}
			return nil, &CatalogError{Kind: CatalogErrorMalformed}
		}
		consumed := decoder.InputOffset()
		if raw.ID == "" || raw.Capabilities == nil || raw.Capabilities.ToolCall == nil || !strings.HasSuffix(header, "/"+raw.ID) {
			return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
		}
		providerID := strings.TrimSuffix(header, "/"+raw.ID)
		if providerID == "" {
			return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
		}
		provider := providers[providerID]
		if provider.ID == "" {
			provider = Provider{ID: providerID, Name: providerID, Models: map[string]Model{}}
		}
		variants := make([]string, 0, len(raw.Variants))
		for key := range raw.Variants {
			variants = append(variants, key)
		}
		sortVariants(variants)
		reasoning := raw.Capabilities.Reasoning != nil && *raw.Capabilities.Reasoning
		provider.Models[raw.ID] = Model{ID: raw.ID, Name: raw.Name, Family: raw.Family, ToolCall: *raw.Capabilities.ToolCall, Reasoning: reasoning, Limit: raw.Limit, Cost: raw.Cost, Variants: variants}
		providers[providerID] = provider
		data = data[consumed:]
	}
	if len(providers) == 0 && sawNoise {
		return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
	}
	return providers, nil
}

// isCatalogHeader reports whether line has the `provider/model` header shape:
// non-empty, no whitespace, at least one slash, and not the start of a JSON
// block. Anything else on stdout is log noise, never a catalog record.
func isCatalogHeader(line string) bool {
	return line != "" && !strings.ContainsAny(line, " \t") && strings.Contains(line, "/") && line[0] != '{'
}

// startsJSONObject reports whether the bytes after a candidate header begin a
// JSON object. A header-shaped log line (a bare path, a URL) is only accepted
// as a record header when its JSON block actually follows.
func startsJSONObject(rest []byte) bool {
	rest = bytes.TrimLeft(rest, "\r\n\t ")
	return len(rest) > 0 && rest[0] == '{'
}

// effortRank orders the known reasoning-effort variant names semantically.
var effortRank = map[string]int{"low": 0, "medium": 1, "high": 2}

// sortVariants emits low/medium/high semantic order when every variant key is
// a known effort name, falling back to lexical order for anything else.
func sortVariants(variants []string) {
	for _, variant := range variants {
		if _, known := effortRank[variant]; !known {
			sort.Strings(variants)
			return
		}
	}
	sort.Slice(variants, func(i, j int) bool { return effortRank[variants[i]] < effortRank[variants[j]] })
}

func runCatalogCommand(ctx context.Context, command Command) (CommandOutput, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	stdout, stderr := &limitedBuffer{limit: command.OutputLimit, cancel: cancel}, &limitedBuffer{limit: command.OutputLimit, cancel: cancel}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if stdout.overflow || stderr.overflow {
		return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

type limitedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
	cancel   context.CancelFunc
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if len(data) > remaining {
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.overflow = true
		b.cancel()
		return len(data), nil
	}
	return b.buffer.Write(data)
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }
