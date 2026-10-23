package opencode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

const verboseCatalog = `custom/qwen/qwen3
{
  "id": "qwen/qwen3",
  "name": "Qwen 3",
  "capabilities": {"toolcall": true, "reasoning": true},
  "limit": {"context": 32768, "output": 4096},
  "cost": {"input": 0.2, "output": 0.8},
  "variants": {"high": {}, "low": {}}
}
other/plain
{"id":"plain","name":"Plain","capabilities":{"toolcall":false,"reasoning":false}}
`

func discoverV1CatalogWithRunner(ctx context.Context, dir string, runner CommandRunner) (map[string]Provider, error) {
	return DiscoverCatalogWithRunner(ctx, dir, func(ctx context.Context, command Command) (io.Reader, error) {
		if strings.Join(command.Args, " ") == "--version" {
			return strings.NewReader("opencode v1.2.0\n"), nil
		}
		return runner(ctx, command)
	})
}

func TestDiscoverCatalogV2Models(t *testing.T) {
	var commands []string
	runner := func(_ context.Context, command Command) (io.Reader, error) {
		commands = append(commands, strings.Join(command.Args, " "))
		switch strings.Join(command.Args, " ") {
		case "--version":
			return strings.NewReader("opencode v2.0.18\n"), nil
		case "api get /api/model?location%5Bdirectory%5D=project":
			if command.Dir != "project" {
				t.Fatalf("API directory = %q", command.Dir)
			}
			return strings.NewReader(`{"data":[{"id":"gpt-5.5-fast","modelID":"gpt-5.5","providerID":"openai","name":"GPT 5.5 Fast","capabilities":{"tools":true},"enabled":true,"variants":[{"id":"low"},{"id":"high"}],"limit":{"context":128000,"output":8192}},{"id":"text-only","modelID":"text-only","providerID":"openai","name":"Text Only","capabilities":{"tools":false},"enabled":true,"variants":[]},{"id":"disabled","modelID":"disabled","providerID":"openai","name":"Disabled","capabilities":{"tools":true},"enabled":false,"variants":[]},{"id":"qwen/qwen3","modelID":"qwen/qwen3","providerID":"custom","name":"Qwen 3","capabilities":{"tools":true},"enabled":true,"variants":[]}]}`), nil
		default:
			t.Fatalf("unsupported command %v", command.Args)
			return nil, nil
		}
	}
	providers, err := DiscoverCatalogWithRunner(context.Background(), "project", runner)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(commands, ",") != "--version,api get /api/model?location%5Bdirectory%5D=project" {
		t.Fatalf("commands = %v", commands)
	}
	if providers["openai"].Models["gpt-5.5-fast"].ID != "gpt-5.5-fast" || !providers["openai"].Models["gpt-5.5-fast"].ToolCall || providers["custom"].Models["qwen/qwen3"].ID != "qwen/qwen3" || providers["openai"].Models["text-only"].ToolCall {
		t.Fatalf("catalog = %+v", providers)
	}
	if _, ok := providers["openai"].Models["disabled"]; ok {
		t.Fatal("disabled model offered")
	}
	if got := strings.Join(providers["openai"].Models["gpt-5.5-fast"].Variants, ","); got != "low,high" {
		t.Fatalf("variants = %q", got)
	}
	if got := FilterModelsForSDD(providers["openai"]); len(got) != 1 || got[0].ID != "gpt-5.5-fast" {
		t.Fatalf("SDD-selectable models = %+v, want only tool-capable model", got)
	}
}

// TestParseV2ModelAPIReasoningFromCompatibility pins the real V2 schema:
// reasoning intent lives in the optional compatibility.requireReasoning
// field. The capabilities object has no reasoning field on real hosts, so a
// payload carrying capabilities.reasoning must NOT set Reasoning.
func TestParseV2ModelAPIReasoningFromCompatibility(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "compatibility requireReasoning true", payload: `{"data":[{"id":"model","modelID":"model","providerID":"openai","name":"Model","enabled":true,"capabilities":{"tools":true},"compatibility":{"requireReasoning":true},"variants":[]}]}`, want: true},
		{name: "compatibility requireReasoning false", payload: `{"data":[{"id":"model","modelID":"model","providerID":"openai","name":"Model","enabled":true,"capabilities":{"tools":true},"compatibility":{"requireReasoning":false},"variants":[]}]}`},
		{name: "compatibility absent", payload: `{"data":[{"id":"model","modelID":"model","providerID":"openai","name":"Model","enabled":true,"capabilities":{"tools":true},"variants":[]}]}`},
		{name: "capabilities.reasoning must not be consulted", payload: `{"data":[{"id":"model","modelID":"model","providerID":"openai","name":"Model","enabled":true,"capabilities":{"tools":true,"reasoning":true},"variants":[]}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			providers, err := parseV2ModelAPI(strings.NewReader(tt.payload))
			if err != nil {
				t.Fatal(err)
			}
			model := providers["openai"].Models["model"]
			if model.Reasoning != tt.want {
				t.Fatalf("model = %+v, want reasoning=%t", model, tt.want)
			}
		})
	}
}

// TestParseV2ModelAPISelectsUntieredBaseCost pins the base-price selection:
// the first untiered cost entry is the base rate; tiered entries describe
// context-window discounts. When every entry is tiered the first one is the
// closest available base.
func TestParseV2ModelAPISelectsUntieredBaseCost(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cost    string
		wantIn  float64
		wantOut float64
	}{
		{name: "untiered first", cost: `[{"input":2,"output":10}]`, wantIn: 2, wantOut: 10},
		{name: "tiered first, untiered second", cost: `[{"tier":{"type":"context","size":272000},"input":4,"output":15},{"input":2,"output":10}]`, wantIn: 2, wantOut: 10},
		{name: "all tiered falls back to first", cost: `[{"tier":{"type":"context","size":272000},"input":4,"output":15}]`, wantIn: 4, wantOut: 15},
		{name: "no cost entries", cost: `[]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := `{"data":[{"id":"model","modelID":"model","providerID":"openai","name":"Model","enabled":true,"capabilities":{"tools":true},"variants":[],"cost":` + tt.cost + `}]}`
			providers, err := parseV2ModelAPI(strings.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			cost := providers["openai"].Models["model"].Cost
			if cost.Input != tt.wantIn || cost.Output != tt.wantOut {
				t.Fatalf("cost = %+v, want {%v %v}", cost, tt.wantIn, tt.wantOut)
			}
		})
	}
}

// TestParseV2ModelAPIRejectsNonAPIOutput pins the parser-level rejection of
// non-API output: anything that is not the JSON envelope fails as
// malformed_output (discovery-level failures fall back to the V1 stream).
func TestParseV2ModelAPIRejectsNonAPIOutput(t *testing.T) {
	for _, output := range []string{"Available models:\n- openai/gpt-5.5\n", "openai/\n", "openai/gpt-5.5\nplugin log\n"} {
		t.Run(output, func(t *testing.T) {
			_, err := parseV2ModelAPI(strings.NewReader(output))
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorMalformed {
				t.Fatalf("error = %v, want malformed_output", err)
			}
		})
	}
}

// TestParseV2ModelAPIRejectsMissingToolCapability pins the parser-level
// schema strictness: the explicit tools capability is required, so payloads
// without it (or with a wrong shape) fail as unsupported_schema.
func TestParseV2ModelAPIRejectsMissingToolCapability(t *testing.T) {
	for _, payload := range []string{
		`{"data":[{"id":"model","modelID":"model","providerID":"openai","enabled":true}]}`,
		`{"data":[{"id":"model","modelID":"model","providerID":"openai","enabled":true,"capabilities":{}}]}`,
		`{"data":[{"id":"model","modelID":"model","providerID":"openai","enabled":true,"capabilities":{"tools":"true"}}]}`,
	} {
		t.Run(payload, func(t *testing.T) {
			_, err := parseV2ModelAPI(strings.NewReader(payload))
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorUnsupportedSchema {
				t.Fatalf("error = %v, want unsupported_schema", err)
			}
		})
	}
}

func TestDiscoverCatalogRejectsUnknownVersionWithoutInvokingModels(t *testing.T) {
	calledModels := false
	_, err := DiscoverCatalogWithRunner(context.Background(), "project", func(_ context.Context, command Command) (io.Reader, error) {
		if strings.Join(command.Args, " ") == "--version" {
			return strings.NewReader("opencode v3.0.0\n"), nil
		}
		calledModels = true
		return strings.NewReader(""), nil
	})
	var catalogErr *CatalogError
	if !errors.As(err, &catalogErr) || catalogErr.Kind != CatalogErrorUnsupportedSchema || calledModels {
		t.Fatalf("error = %v, calledModels = %t", err, calledModels)
	}
}

func TestDiscoverCatalogRealV2(t *testing.T) {
	if testing.Short() || os.Getenv("GENTLE_AI_TEST_OPENCODE_V2") == "" {
		t.Skip("opt-in integration test requires an installed OpenCode V2 binary")
	}
	major, err := DetectRuntimeMajor(context.Background())
	if err != nil || major != RuntimeV2 {
		t.Skipf("OpenCode V2 unavailable: major = %v, error = %v", major, err)
	}
	// Model availability is location-scoped; an empty temporary directory can
	// legitimately return no models even when the repository has a catalog.
	projectDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	providers, err := DiscoverCatalog(context.Background(), projectDir)
	if err != nil {
		t.Fatalf("DiscoverCatalog(real V2) error = %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("real V2 model list contained no providers")
	}
	selectable := 0
	for _, provider := range providers {
		selectable += len(FilterModelsForSDD(provider))
	}
	if selectable == 0 {
		t.Fatal("real V2 API catalog contained no tool-capable picker models")
	}
}

func TestDiscoverCatalogMapsVerboseOutputAndProjectDirectory(t *testing.T) {
	var got Command
	runner := func(_ context.Context, command Command) (io.Reader, error) {
		got = command
		return strings.NewReader(verboseCatalog), nil
	}

	providers, err := discoverV1CatalogWithRunner(context.Background(), `C:\work\project`, runner)
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v", err)
	}
	if got.Path != "opencode" || got.Dir != `C:\work\project` || strings.Join(got.Args, " ") != "models --verbose" {
		t.Fatalf("command = %+v, want opencode models --verbose in project directory", got)
	}
	model := providers["custom"].Models["qwen/qwen3"]
	if !model.ToolCall || !model.Reasoning || model.Limit.Context != 32768 || model.Cost.Output != 0.8 || strings.Join(model.Variants, ",") != "low,high" {
		t.Fatalf("runtime model = %+v", model)
	}
	if _, ok := providers["other"].Models["plain"]; !ok {
		t.Fatal("missing second provider model")
	}
}

func TestDiscoverCatalogToleratesLogNoiseAroundRecords(t *testing.T) {
	noisy := "[skill-registry] skipping refresh: not a project root: /Users/someone/Desktop\n" +
		verboseCatalog +
		"[skill-registry] refresh done\n"
	providers, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
		return strings.NewReader(noisy), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v, want tolerated log preamble", err)
	}
	if _, ok := providers["custom"].Models["qwen/qwen3"]; !ok {
		t.Fatal("missing model after log preamble")
	}
	if _, ok := providers["other"].Models["plain"]; !ok {
		t.Fatal("missing trailing model with interleaved log noise")
	}
}

func TestDiscoverCatalogOrdersKnownEffortVariantsSemantically(t *testing.T) {
	out := "custom/model\n" +
		`{"id":"model","name":"Model","capabilities":{"toolcall":true},"variants":{"medium":{},"high":{},"low":{}}}` + "\n" +
		"custom/other\n" +
		`{"id":"other","name":"Other","capabilities":{"toolcall":true},"variants":{"zeta":{},"alpha":{}}}` + "\n"
	providers, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
		return strings.NewReader(out), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v", err)
	}
	if got := strings.Join(providers["custom"].Models["model"].Variants, ","); got != "low,medium,high" {
		t.Fatalf("effort variants = %q, want semantic low,medium,high order", got)
	}
	if got := strings.Join(providers["custom"].Models["other"].Variants, ","); got != "alpha,zeta" {
		t.Fatalf("unknown variants = %q, want sorted fallback", got)
	}
}

func TestSortVariants_ExtendedEffortAndSpeedVariants(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "extended effort levels all ordered by rank",
			input:    []string{"ultra", "max", "xhigh", "high", "medium", "low"},
			expected: []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		},
		{
			name:     "mixed effort and non-effort speed variants",
			input:    []string{"fast", "high", "ultra", "beta", "low", "alpha"},
			expected: []string{"low", "high", "ultra", "alpha", "beta", "fast"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variants := append([]string(nil), tt.input...)
			sortVariants(variants)
			if !reflect.DeepEqual(variants, tt.expected) {
				t.Fatalf("sortVariants(%v) = %v, want %v", tt.input, variants, tt.expected)
			}
		})
	}
}

func TestDiscoverCatalogParsesLargeOutputAboveOneMegabyte(t *testing.T) {
	// Generate ~1.5 MiB of valid models (3500 records * ~450 bytes each)
	payload := string(generateCatalogFixture(3500))
	if len(payload) <= 1<<20 {
		t.Fatalf("test payload size = %d, want > 1 MiB", len(payload))
	}

	providers, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
		return strings.NewReader(payload), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() large payload error = %v", err)
	}
	if len(providers["prov"].Models) != 3500 {
		t.Fatalf("parsed models count = %d, want 3500", len(providers["prov"].Models))
	}
}

func TestDiscoverCatalogDecoderReadAheadDoesNotDropSubsequentHeaders(t *testing.T) {
	// Compact consecutive JSON records where json.Decoder's internal buffer reads into the next header line
	var b strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "prov/model-%02d\n{\"id\":\"model-%02d\",\"capabilities\":{\"toolcall\":true}}\n", i, i)
	}
	providers, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
		return strings.NewReader(b.String()), nil
	})
	if err != nil {
		t.Fatalf("DiscoverCatalogWithRunner() error = %v", err)
	}
	if len(providers["prov"].Models) != 50 {
		t.Fatalf("parsed models = %d, want 50 (headers likely eaten by decoder read-ahead)", len(providers["prov"].Models))
	}
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("model-%02d", i)
		if _, ok := providers["prov"].Models[id]; !ok {
			t.Fatalf("missing model %q", id)
		}
	}
}

func TestDiscoverCatalogRejectsInvalidOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		kind CatalogErrorKind
	}{
		{"truncated JSON", "custom/model\n{\"id\":", CatalogErrorMalformed},
		{"unsupported record", "custom/model\n{\"name\":\"Model\"}", CatalogErrorUnsupportedSchema},
		{"missing tool capability", "custom/model\n{\"id\":\"model\",\"capabilities\":{}}", CatalogErrorUnsupportedSchema},
		{"incompatible tool capability", "custom/model\n{\"id\":\"model\",\"capabilities\":{\"toolcall\":\"true\"}}", CatalogErrorUnsupportedSchema},
		{"provider header mismatch", "custom/other\n{\"id\":\"model\",\"capabilities\":{\"toolcall\":true}}", CatalogErrorUnsupportedSchema},
		{"log noise only", "[skill-registry] skipping refresh: not a project root: /Users/someone\nsome other log line\n", CatalogErrorUnsupportedSchema},
		{"human readable model list", "Available models:\n- custom/model\n", CatalogErrorUnsupportedSchema},
		{"oversized output", strings.Repeat("x", maxCatalogOutput+1), CatalogErrorOutputTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
				return strings.NewReader(tt.out), nil
			})
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != tt.kind {
				t.Fatalf("error = %v, want %v", err, tt.kind)
			}
		})
	}
}

// TestProcessStreamReaderReadSurfacesExitStatusAfterEOF pins the Read
// contract: once the drained stream is exhausted, Read must reap the child
// and return its exit error, never swallow it as a plain io.EOF.
func TestProcessStreamReaderReadSurfacesExitStatusAfterEOF(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX exit-status fixture")
	}
	// Force a child with a non-zero exit status through sh.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: "sh", Args: []string{"-c", "exit 7"}})
	if err != nil {
		t.Fatalf("runCatalogCommand: %v", err)
	}
	var buf [256]byte
	for {
		_, readErr := r.Read(buf[:])
		if readErr != nil {
			var catalogErr *CatalogError
			if errors.As(readErr, &catalogErr) {
				if catalogErr.Kind != CatalogErrorCommandFailed {
					t.Fatalf("Read terminal error = %v, want command_failed", readErr)
				}
				return
			}
			var exitErr *exec.ExitError
			if errors.As(readErr, &exitErr) {
				if code := exitErr.ExitCode(); code != 7 {
					t.Fatalf("Read terminal exit code = %d, want 7", code)
				}
				return
			}
			if !errors.Is(readErr, io.EOF) {
				t.Fatalf("Read terminal error = %v, want the child's exit status", readErr)
			}
			t.Fatal("Read returned plain io.EOF; the child's non-zero exit was swallowed")
		}
	}
}

func TestRunCatalogCommandCancelsOverflowingChild(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "catalog-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte("package main\nimport (\"fmt\"; \"strings\")\nfunc main() { for { fmt.Println(strings.Repeat(\"x\", 1024)) } }\n"), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	limit := 128
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	limitReader := &countingLimitReader{r: r, limit: int64(limit), cancel: cancel}
	_, parseErr := parseVerboseCatalog(limitReader)

	var catalogErr *CatalogError
	if !errors.As(parseErr, &catalogErr) || catalogErr.Kind != CatalogErrorOutputTooLarge {
		t.Fatalf("parseErr = %v, want output_too_large", parseErr)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("overflowing helper ran for %v, want prompt cancellation", elapsed)
	}
}

func TestDiscoverCatalogClassifiesCommandFailuresAndEmptyCatalog(t *testing.T) {
	tests := []struct {
		name string
		err  error
		kind CatalogErrorKind
	}{
		{"missing binary", &exec.Error{Name: "opencode", Err: os.ErrNotExist}, CatalogErrorMissingBinary},
		{"path binary missing", &exec.Error{Name: "opencode", Err: exec.ErrNotFound}, CatalogErrorMissingBinary},
		{"non-zero exit", &exec.ExitError{}, CatalogErrorCommandFailed},
		{"timeout", context.DeadlineExceeded, CatalogErrorTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
				return nil, tt.err
			})
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != tt.kind {
				t.Fatalf("error = %v, want %v", err, tt.kind)
			}
		})
	}
	providers, err := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
		return strings.NewReader(""), nil
	})
	if err != nil || len(providers) != 0 {
		t.Fatalf("empty catalog = %v, %v; want empty successful catalog", providers, err)
	}
}

func TestMergeConfiguredCatalogKeepsRuntimeAuthoritative(t *testing.T) {
	runtimeCatalog := map[string]Provider{
		"runtime": {
			ID:   "runtime",
			Name: "Runtime Provider",
			URL:  "http://runtime.example/v1",
			Models: map[string]Model{
				"shared": {ID: "shared", Name: "Runtime Shared", ToolCall: true},
			},
		},
	}
	configuredCatalog := map[string]Provider{
		"runtime": {
			ID:   "runtime",
			Name: "Configured Provider",
			URL:  "http://configured.example/v1",
			Models: map[string]Model{
				"shared":    {ID: "shared", Name: "Configured Shared"},
				"file-only": {ID: "file-only", Name: "File Only", ToolCall: true},
			},
		},
		"custom": {
			ID:   "custom",
			Name: "Custom Provider",
			URL:  "http://custom.example/v1",
			Models: map[string]Model{
				"custom-model": {ID: "custom-model", Name: "Custom Model", ToolCall: true},
			},
		},
	}

	merged := MergeConfiguredCatalog(runtimeCatalog, configuredCatalog)
	if got := merged["runtime"].Name; got != "Runtime Provider" {
		t.Fatalf("runtime provider name = %q, want runtime value", got)
	}
	if got := merged["runtime"].URL; got != "http://runtime.example/v1" {
		t.Fatalf("runtime provider URL = %q, want runtime value", got)
	}
	if got := merged["runtime"].Models["shared"].Name; got != "Runtime Shared" {
		t.Fatalf("shared model = %q, want runtime value", got)
	}
	if _, ok := merged["runtime"].Models["file-only"]; !ok {
		t.Fatal("missing configured-only model on runtime provider")
	}
	if _, ok := merged["custom"].Models["custom-model"]; !ok {
		t.Fatal("missing configured-only provider/model")
	}
}

func TestMergeConfiguredCatalogFillsMissingRuntimeURL(t *testing.T) {
	runtimeCatalog := map[string]Provider{
		"lmstudio": {ID: "lmstudio", Name: "LM Studio", Models: map[string]Model{"runtime": {ID: "runtime", Name: "Runtime"}}},
	}
	configuredCatalog := map[string]Provider{
		"lmstudio": {ID: "lmstudio", URL: "http://localhost:1234/v1", Models: map[string]Model{}},
	}

	merged := MergeConfiguredCatalog(runtimeCatalog, configuredCatalog)
	if got := merged["lmstudio"].URL; got != "http://localhost:1234/v1" {
		t.Fatalf("merged URL = %q, want configured URL filling missing runtime metadata", got)
	}
}

func TestMergeConfiguredCatalogCopiesVariantsDefensively(t *testing.T) {
	runtimeCatalog := map[string]Provider{
		"p1": {
			ID: "p1",
			Models: map[string]Model{
				"m1": {ID: "m1", Variants: []string{"low", "high"}},
			},
		},
	}
	configuredCatalog := map[string]Provider{
		"p2": {
			ID: "p2",
			Models: map[string]Model{
				"m2": {ID: "m2", Variants: []string{"medium", "max"}},
			},
		},
	}

	merged := MergeConfiguredCatalog(runtimeCatalog, configuredCatalog)

	// Mutating variants in merged catalog must not mutate input catalog slices
	merged["p1"].Models["m1"].Variants[0] = "mutated"
	if runtimeCatalog["p1"].Models["m1"].Variants[0] != "low" {
		t.Fatalf("runtime model variants was mutated via merged copy")
	}

	merged["p2"].Models["m2"].Variants[0] = "mutated"
	if configuredCatalog["p2"].Models["m2"].Variants[0] != "medium" {
		t.Fatalf("configured model variants was mutated via merged copy")
	}
}

func TestMergeConfiguredCatalogMergesAndSortsVariants(t *testing.T) {
	runtimeCatalog := map[string]Provider{
		"openai": {
			ID: "openai",
			Models: map[string]Model{
				"gpt-5": {ID: "gpt-5", Name: "GPT-5", Variants: []string{"low", "high"}},
			},
		},
	}
	configuredCatalog := map[string]Provider{
		"openai": {
			ID: "openai",
			Models: map[string]Model{
				"gpt-5": {ID: "gpt-5", Variants: []string{"fast", "medium", "low"}},
			},
		},
	}

	merged := MergeConfiguredCatalog(runtimeCatalog, configuredCatalog)
	gotVariants := merged["openai"].Models["gpt-5"].Variants
	wantVariants := []string{"low", "medium", "high", "fast"}
	if !reflect.DeepEqual(gotVariants, wantVariants) {
		t.Fatalf("merged variants = %v, want %v", gotVariants, wantVariants)
	}
}

// TestDiscoverCatalogLiveHostIntegration runs against the real `opencode`
// binary when it is available and asserts only host-portable invariants: the
// command exits successfully and yields at least one provider with at least
// one model. Specific provider or model IDs vary per machine and must not be
// asserted here, or the test breaks on every other host and in CI.
func TestDiscoverCatalogLiveHostIntegration(t *testing.T) {
	if testing.Short() || os.Getenv("GENTLE_AI_TEST_LIVE_HOST") == "" {
		t.Skip("skipping live host integration test; set GENTLE_AI_TEST_LIVE_HOST=1 to run")
	}
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode binary not in PATH")
	}
	providers, err := DiscoverCatalog(context.Background(), ".")
	if err != nil {
		t.Fatalf("DiscoverCatalog() on live host failed: %v", err)
	}
	if len(providers) == 0 {
		t.Skip("live host reports an empty catalog; nothing to assert")
	}
	count := 0
	for _, p := range providers {
		if p.ID == "" {
			t.Errorf("provider has empty ID: %+v", p)
			continue
		}
		for _, m := range p.Models {
			if m.ID == "" {
				t.Errorf("provider %q contains a model with an empty ID", p.ID)
			}
		}
		count += len(p.Models)
	}
	if count == 0 {
		t.Skip("live host reports an empty model list; nothing to assert")
	}
	t.Logf("DiscoverCatalog() successfully discovered %d providers and %d models on live host", len(providers), count)
}

// TestCountingLimitReaderAllowsExactFitStream verifies that a valid catalog
// stream whose total size equals the configured limit parses completely
// instead of being rejected as output_too_large. Only streams that exceed the
// limit must fail; a catalog ending exactly at the defensive ceiling is valid.
func TestCountingLimitReaderAllowsExactFitStream(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 25; i++ {
		fmt.Fprintf(&b, "prov/model-%02d\n", i)
		fmt.Fprintf(&b, `{"id":"model-%02d","name":"Model %02d %s","capabilities":{"toolcall":true}}`+"\n", i, i, strings.Repeat("x", 40))
	}
	payload := b.String()

	providers, err := parseVerboseCatalog(&countingLimitReader{r: strings.NewReader(payload), limit: int64(len(payload))})
	if err != nil {
		t.Fatalf("parseVerboseCatalog() exact-fit catalog error = %v, want successful parse", err)
	}
	if got := len(providers["prov"].Models); got != 25 {
		t.Fatalf("parsed models = %d, want 25 (exact-fit catalog must parse completely)", got)
	}
}

// exitOnCloseReader mimics a process stream reader whose stream may carry a
// parse failure while the child exits non-zero: Close reports the child's
// wait error, mirroring processStreamReader.Close after the lifecycle fix.
type exitOnCloseReader struct {
	r       io.Reader
	waitErr error
}

func (e *exitOnCloseReader) Read(p []byte) (int, error) { return e.r.Read(p) }
func (e *exitOnCloseReader) Close() error               { return e.waitErr }
func (e *exitOnCloseReader) WaitError() error           { return e.waitErr }

// TestDiscoverCatalogPrefersCommandFailureOverParseClassification pins the
// classification precedence of issue #4043 review feedback: a non-zero child
// exit must surface as command_failed (or timeout) and must never be masked
// by a downstream parse classification. Only a genuine output overflow keeps
// its own category, matching the pre-streaming cmd.Run() behavior.
func TestDiscoverCatalogPrefersCommandFailureOverParseClassification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX exit-status fixture")
	}
	fixture := exec.Command("sh", "-c", "exit 7")
	if err := fixture.Run(); err == nil {
		t.Fatal("expected fixture command to exit non-zero")
	}
	exitErr := &exec.ExitError{ProcessState: fixture.ProcessState}

	tests := []struct {
		name string
		out  string
		want CatalogErrorKind
	}{
		{"malformed stream with non-zero exit", "custom/model\n{\"id\":", CatalogErrorCommandFailed},
		{"valid stream with non-zero exit", verboseCatalog, CatalogErrorCommandFailed},
		{"oversized stream with non-zero exit", strings.Repeat("x", maxCatalogOutput+1), CatalogErrorOutputTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(context.Context, Command) (io.Reader, error) {
				return &exitOnCloseReader{r: strings.NewReader(tt.out), waitErr: exitErr}, nil
			}
			_, err := discoverV1CatalogWithRunner(context.Background(), "project", runner)
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != tt.want {
				t.Fatalf("err = %v, want %v (child exit status must not be masked)", err, tt.want)
			}
		})
	}
}

// TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendant reproduces
// the inherited-pipe cancellation defect: a descendant of the child that
// inherits stdout and stderr must not extend discovery past the context
// deadline.
func TestRunCatalogCommandDeadlineNotBlockedByInheritingDescendant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX sleep descendant to inherit the pipes")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "descendant-helper")
	source := filepath.Join(dir, "main.go")
	src := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	descendant := exec.Command("sleep", "5")
	descendant.Stdout = os.Stdout
	descendant.Stderr = os.Stderr
	_ = descendant.Start()
	fmt.Println("custom/model")
	fmt.Println("{\"id\":\"model\",\"name\":\"Model\",\"capabilities\":{\"toolcall\":true}}")
	time.Sleep(25 * time.Millisecond)
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	started := time.Now()
	_, _ = io.ReadAll(r)
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("discovery stayed blocked for %v; want bounded by the 300ms deadline", elapsed)
	}
}

// TestRunCatalogCommandDeadlineNotBlockedByEscapedDescendant verifies that when
// a descendant escapes the child's Unix process group (via its own pgid) and
// inherits stdout, cancellation of the direct child still bounds discovery
// within the context deadline instead of blocking indefinitely on the escaped
// descendant's open pipe.
func TestRunCatalogCommandDeadlineNotBlockedByEscapedDescendant(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX process groups to test escaping the group")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "escaped-descendant-helper")
	source := filepath.Join(dir, "main.go")
	src := `package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func main() {
	descendant := exec.Command("sleep", "5")
	descendant.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	descendant.Stdout = os.Stdout
	descendant.Stderr = os.Stderr
	_ = descendant.Start()
	fmt.Println("custom/model")
	fmt.Println("{\"id\":\"model\",\"name\":\"Model\",\"capabilities\":{\"toolcall\":true}}")
	time.Sleep(25 * time.Millisecond)
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	started := time.Now()
	_, _ = io.ReadAll(r)
	if closer, ok := r.(io.Closer); ok {
		_ = closer.Close()
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("discovery stayed blocked for %v; want bounded by the 300ms deadline", elapsed)
	}
}

func TestBoundedPipeBufferPreservesNonEOFReadFailure(t *testing.T) {
	buf := newBoundedPipeBuffer(1024, nil)
	buf.append([]byte("hello world"))
	simulatedErr := errors.New("simulated pipe read failure")
	buf.finish(simulatedErr)

	chunk := make([]byte, 5)
	n, err := buf.Read(chunk)
	if err != nil || n != 5 || string(chunk[:n]) != "hello" {
		t.Fatalf("Read chunk 1 = (%d, %v, %q), want (5, nil, %q)", n, err, string(chunk[:n]), "hello")
	}

	rest := make([]byte, 100)
	n, err = buf.Read(rest)
	if err != nil || n != 6 || string(rest[:n]) != " world" {
		t.Fatalf("Read chunk 2 = (%d, %v, %q), want (6, nil, %q)", n, err, string(rest[:n]), " world")
	}

	// Buffer drained; next read must return the non-EOF readErr.
	n, err = buf.Read(rest)
	if !errors.Is(err, simulatedErr) {
		t.Fatalf("Read after drain err = %v, want %v", err, simulatedErr)
	}
}

// TestProcessStreamReaderWaitErrorJoinsStdoutDrain pins the os/exec pipe
// lifecycle contract: when parsing fails early while the child is still alive
// and writing, WaitError must join the stdout drain before reaping the child
// instead of calling Wait under a concurrent pipe reader. The helper emits a
// malformed record first (so the parse fails fast) and keeps writing noise
// for ~2s before exiting non-zero; discovery must complete promptly with
// command_failed (the child's exit wins over the parse classification) and
// must never panic or deadlock on the pipe.
func TestProcessStreamReaderWaitErrorJoinsStdoutDrain(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "earlyfail-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	source := filepath.Join(dir, "main.go")
	src := `package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Println("custom/model")
	fmt.Println("{broken json")
	for i := 0; i < 200; i++ {
		fmt.Println("noise line", i)
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(4)
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand() error = %v", err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}

	done := make(chan error, 1)
	go func() {
		_, discoverErr := discoverV1CatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
			return r, nil
		})
		done <- discoverErr
	}()

	select {
	case discoverErr := <-done:
		var catalogErr *CatalogError
		if !errors.As(discoverErr, &catalogErr) || catalogErr.Kind != CatalogErrorCommandFailed {
			t.Fatalf("err = %v, want command_failed (child exit must win over early parse failure)", discoverErr)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("DiscoverCatalogWithRunner blocked; WaitError did not join the stdout drain before reaping")
	}
}

// slowReader throttles consumption so the parser runs far slower than the
// child writes. OpenCode's catalog writer aborts when its stdout pipe drains
// too slowly, so discovery must drain the pipe in the background and let the
// consumer parse at its own pace.
type slowReader struct {
	r io.Reader
}

func (s *slowReader) Read(p []byte) (int, error) {
	time.Sleep(2 * time.Millisecond)
	return s.r.Read(p)
}

// TestDiscoverCatalogSurvivesSlowConsumer pins the background-drain contract:
// a consumer slower than the child's writer must still observe the complete
// catalog, because the drain goroutine keeps the pipe empty. Without the
// background drain this test truncates the catalog mid-record with
// malformed_output on hosts where OpenCode aborts slow stdout writers.
func TestDiscoverCatalogSurvivesSlowConsumer(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "slow-consumer-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	source := filepath.Join(dir, "main.go")
	src := `package main

import (
	"fmt"
)

func main() {
	for i := 0; i < 50; i++ {
		fmt.Printf("prov/model-%03d\n", i)
		fmt.Printf("{\"id\":\"model-%03d\",\"name\":\"Model %03d\",\"capabilities\":{\"toolcall\":true}}\n", i, i)
	}
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand: %v", err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	providers, err := parseVerboseCatalog(&countingLimitReader{r: &slowReader{r: r}, limit: maxCatalogOutput, cancel: cancel})
	if err != nil {
		t.Fatalf("slow-consumer parse error = %v, want complete catalog", err)
	}
	prov, ok := providers["prov"]
	if !ok {
		t.Fatal("slow consumer missing provider 'prov'")
	}
	if len(prov.Models) != 50 {
		t.Fatalf("slow consumer discovered %d models, want 50", len(prov.Models))
	}
}

// TestDiscoverCatalogLegacyVsNew pins the regression contract of issue #4042
// against a generated fixture: under the legacy 1 MiB ceiling a large catalog
// fails with output_too_large, while the current 16 MiB ceiling parses the
// same stream completely. No host fixture file is required.
func TestDiscoverCatalogLegacyVsNew(t *testing.T) {
	data := generateCatalogFixture(3500)

	// Legacy 1 MiB limit: must fail with CatalogErrorOutputTooLarge
	limitReaderLegacy := &countingLimitReader{r: bytes.NewReader(data), limit: 1 << 20}
	_, errLegacy := parseVerboseCatalog(limitReaderLegacy)
	var catErr *CatalogError
	if !errors.As(errLegacy, &catErr) || catErr.Kind != CatalogErrorOutputTooLarge {
		t.Fatalf("legacy limit (1 MiB) err = %v, want output_too_large", errLegacy)
	}

	// New 16 MiB limit: must succeed and parse all models
	limitReaderNew := &countingLimitReader{r: bytes.NewReader(data), limit: 16 << 20}
	providersNew, errNew := parseVerboseCatalog(limitReaderNew)
	if errNew != nil {
		t.Fatalf("new limit (16 MiB) err = %v, want success", errNew)
	}
	if got := len(providersNew["prov"].Models); got != 3500 {
		t.Fatalf("parsed models = %d, want 3500", got)
	}
}

func TestStderrPressure_NoDeadlock(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "stderr-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	source := filepath.Join(dir, "main.go")
	src := `package main
import (
	"fmt"
	"os"
	"strings"
)
func main() {
	// Write 500KB to stderr (far exceeding standard 64KB OS pipe buffer)
	for i := 0; i < 500; i++ {
		fmt.Fprintln(os.Stderr, strings.Repeat("E", 1024))
	}
	// Write valid catalog output to stdout
	fmt.Println("custom/model")
	fmt.Println("{\"id\":\"model\",\"name\":\"Model\",\"capabilities\":{\"toolcall\":true}}")
}
`
	if err := os.WriteFile(source, []byte(src), 0o600); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if err := exec.Command("go", "build", "-o", helper, source).Run(); err != nil {
		t.Fatalf("build helper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r, err := runCatalogCommand(ctx, Command{Path: helper})
	if err != nil {
		t.Fatalf("runCatalogCommand error = %v", err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}

	limitReader := &countingLimitReader{r: r, limit: 16 << 20, cancel: cancel}
	providers, parseErr := parseVerboseCatalog(limitReader)
	if parseErr != nil {
		t.Fatalf("parseVerboseCatalog error with heavy stderr: %v", parseErr)
	}
	if _, ok := providers["custom"].Models["model"]; !ok {
		t.Fatalf("model not found in providers: %+v", providers)
	}
	t.Logf("Stderr pressure scenario passed with 0 deadlock")
}

type oneByteReader struct {
	data []byte
	idx  int
}

func (o *oneByteReader) Read(p []byte) (int, error) {
	if o.idx >= len(o.data) {
		return 0, io.EOF
	}
	p[0] = o.data[o.idx]
	o.idx++
	return 1, nil
}

func TestChunkFragmentation_OneByteAtATime(t *testing.T) {
	raw := "noisy preamble\n" +
		"custom/m1\n" +
		"{\"id\":\"m1\",\"name\":\"M1\",\"capabilities\":{\"toolcall\":true}}\n" +
		"some interleaved log message\n" +
		"custom/m2\n" +
		"{\"id\":\"m2\",\"name\":\"M2\",\"capabilities\":{\"toolcall\":true},\"variants\":{\"high\":{}}}\n"

	reader := &oneByteReader{data: []byte(raw)}
	providers, err := parseVerboseCatalog(reader)
	if err != nil {
		t.Fatalf("parseVerboseCatalog(oneByteReader) failed: %v", err)
	}
	if len(providers["custom"].Models) != 2 {
		t.Fatalf("expected 2 models, got: %+v", providers["custom"].Models)
	}
	if providers["custom"].Models["m1"].ID != "m1" || providers["custom"].Models["m2"].ID != "m2" {
		t.Fatalf("unexpected models: %+v", providers["custom"].Models)
	}
	t.Logf("One byte chunk fragmentation test passed successfully")
}

// generateCatalogFixture returns a deterministic synthetic catalog payload of
// approximately the requested size by emitting valid model records.
func generateCatalogFixture(records int) []byte {
	var b strings.Builder
	for i := 0; i < records; i++ {
		fmt.Fprintf(&b, "prov/model-%d\n", i)
		fmt.Fprintf(&b, `{"id":"model-%d","name":"Model %d %s","capabilities":{"toolcall":true},"limit":{"context":128000},"variants":{"low":{},"medium":{},"high":{}}}`+"\n", i, i, strings.Repeat("x", 300))
	}
	return []byte(b.String())
}

func BenchmarkParseCatalogMemory_Synthesized1MB(b *testing.B) {
	data := generateCatalogFixture(3500)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, _ = parseVerboseCatalog(r)
	}
}

func BenchmarkParseCatalogMemory_Synthesized5MB(b *testing.B) {
	data := generateCatalogFixture(17500)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, _ = parseVerboseCatalog(r)
	}
}
