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

func TestDiscoverCatalogMapsVerboseOutputAndProjectDirectory(t *testing.T) {
	var got Command
	runner := func(_ context.Context, command Command) (io.Reader, error) {
		got = command
		return strings.NewReader(verboseCatalog), nil
	}

	providers, err := DiscoverCatalogWithRunner(context.Background(), `C:\work\project`, runner)
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
	providers, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
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
	providers, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
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

func TestDiscoverCatalogParsesLargeOutputAboveOneMegabyte(t *testing.T) {
	var b strings.Builder
	// Generate ~1.5 MiB of valid models (3500 records * ~450 bytes each)
	padding := strings.Repeat("x", 300)
	for i := 0; i < 3500; i++ {
		fmt.Fprintf(&b, "prov/model-%d\n", i)
		fmt.Fprintf(&b, `{"id":"model-%d","name":"Model %d %s","capabilities":{"toolcall":true},"limit":{"context":128000},"variants":{"low":{},"medium":{},"high":{}}}`+"\n", i, i, padding)
	}
	payload := b.String()
	if len(payload) <= 1<<20 {
		t.Fatalf("test payload size = %d, want > 1 MiB", len(payload))
	}

	providers, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
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
	providers, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
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
			_, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
				return strings.NewReader(tt.out), nil
			})
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != tt.kind {
				t.Fatalf("error = %v, want %v", err, tt.kind)
			}
		})
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
	r, err := runCatalogCommand(ctx, Command{Path: helper, OutputLimit: limit})
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
			_, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
				return nil, tt.err
			})
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Kind != tt.kind {
				t.Fatalf("error = %v, want %v", err, tt.kind)
			}
		})
	}
	providers, err := DiscoverCatalogWithRunner(context.Background(), "project", func(context.Context, Command) (io.Reader, error) {
		return strings.NewReader(""), nil
	})
	if err != nil || len(providers) != 0 {
		t.Fatalf("empty catalog = %v, %v; want empty successful catalog", providers, err)
	}
}

func TestDiscoverCatalogLiveHostIntegration(t *testing.T) {
	if _, err := exec.LookPath("opencode"); err != nil {
		t.Skip("opencode binary not in PATH")
	}
	providers, err := DiscoverCatalog(context.Background(), ".")
	if err != nil {
		t.Fatalf("DiscoverCatalog() on live host failed: %v", err)
	}
	if len(providers) == 0 {
		t.Fatal("DiscoverCatalog() returned 0 providers on this host")
	}
	count := 0
	for _, p := range providers {
		count += len(p.Models)
	}
	t.Logf("DiscoverCatalog() successfully discovered %d providers and %d models on live host", len(providers), count)

	// Verify model from beginning
	if p, ok := providers["opencode"]; !ok || p.Models["big-pickle"].ID != "big-pickle" {
		t.Errorf("expected model opencode/big-pickle from beginning of output, got provider=%+v", p)
	}
	// Verify model from end
	if p, ok := providers["zai"]; !ok || p.Models["glm-5v-turbo"].ID != "glm-5v-turbo" {
		t.Errorf("expected model zai/glm-5v-turbo from end of output, got provider=%+v", p)
	}
}

func TestDiscoverCatalogLegacyVsNew(t *testing.T) {
	data, err := os.ReadFile("/tmp/opencode_models.txt")
	if err != nil {
		t.Skip("skipping legacy vs new test, /tmp/opencode_models.txt not found")
	}

	// Legacy 1 MiB limit: must fail with CatalogErrorOutputTooLarge
	limitReaderLegacy := &countingLimitReader{r: bytes.NewReader(data), limit: 1 << 20}
	_, errLegacy := parseVerboseCatalog(limitReaderLegacy)
	var catErr *CatalogError
	if !errors.As(errLegacy, &catErr) || catErr.Kind != CatalogErrorOutputTooLarge {
		t.Fatalf("legacy limit (1 MiB) err = %v, want output_too_large", errLegacy)
	}
	t.Logf("Legacy 1 MiB limit correctly reproduced error: %v", errLegacy)

	// New 16 MiB limit: must succeed and parse all models
	limitReaderNew := &countingLimitReader{r: bytes.NewReader(data), limit: 16 << 20}
	providersNew, errNew := parseVerboseCatalog(limitReaderNew)
	if errNew != nil {
		t.Fatalf("new limit (16 MiB) err = %v, want success", errNew)
	}
	t.Logf("New 16 MiB limit successfully parsed %d providers", len(providersNew))
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

	r, err := runCatalogCommand(ctx, Command{Path: helper, OutputLimit: 16 << 20})
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

func BenchmarkParseCatalogMemory_RealData(b *testing.B) {
	data, err := os.ReadFile("/tmp/opencode_models.txt")
	if err != nil {
		b.Skip("real data file not found")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(data)
		_, _ = parseVerboseCatalog(r)
	}
}

func BenchmarkParseCatalogMemory_Synthesized5MB(b *testing.B) {
	data, err := os.ReadFile("/tmp/opencode_models.txt")
	if err != nil {
		b.Skip("real data file not found")
	}
	// Replicate real dataset 5 times to form ~5.5 MB
	var big bytes.Buffer
	for i := 0; i < 5; i++ {
		big.Write(data)
	}
	bigBytes := big.Bytes()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(bigBytes)
		_, _ = parseVerboseCatalog(r)
	}
}
