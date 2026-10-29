package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	catalogTimeout   = 15 * time.Second
	catalogWaitDelay = 250 * time.Millisecond
	maxCatalogOutput = 16 << 20
)

// Command describes the bounded OpenCode command used for runtime discovery.
type Command struct {
	Path        string
	Args        []string
	Dir         string
	OutputLimit int
	Env         []string
}

type CommandOutput struct{ Stdout, Stderr []byte }

type CommandRunner func(context.Context, Command) (io.Reader, error)

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

// Error returns the catalog error kind string as the error message. The error
// intentionally exposes only the category, never command output.
func (e *CatalogError) Error() string { return string(e.Kind) }

// DiscoverCatalog reads OpenCode's effective provider catalog for projectDir.
func DiscoverCatalog(ctx context.Context, projectDir string) (map[string]Provider, error) {
	return DiscoverCatalogWithRunner(ctx, projectDir, runCatalogCommand)
}

// waitErrorReader is the contract for stream readers that surface the child's
// wait error, mirroring catalogFileReader. It lets the discovery pipeline
// honor the child's exit status instead of masking it with a parse
// classification.
type waitErrorReader interface {
	io.Reader
	io.Closer
	WaitError() error
}

// DiscoverCatalogWithRunner parses the streamed catalog and classifies the
// outcome. The child's exit status takes precedence over downstream parse
// classification: a non-zero exit surfaces as command_failed (or timeout),
// never masked by malformed/unsupported_schema. The exception is a genuine
// output overflow, which keeps its own category — matching the pre-streaming
// cmd.Run() semantics where overflow was detected during cmd.Run() itself.
func DiscoverCatalogWithRunner(ctx context.Context, projectDir string, runner CommandRunner) (map[string]Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	version, err := runner(ctx, Command{Path: "opencode", Args: []string{"--version"}, Dir: projectDir})
	if err != nil {
		return nil, catalogCommandError(ctx, err)
	}
	major, versionErr := readCatalogVersion(ctx, version)
	if versionErr != nil {
		return nil, versionErr
	}
	args := []string{"models", "--verbose"}
	if major == RuntimeV2 {
		args = []string{"api", "get", "/api/model?location%5Bdirectory%5D=" + url.QueryEscape(projectDir)}
	}
	limit := maxCatalogOutput
	r, err := runner(ctx, Command{Path: "opencode", Args: args, Dir: projectDir})
	if err != nil {
		return nil, catalogCommandError(ctx, err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	limitReader := &countingLimitReader{r: r, limit: int64(limit), cancel: cancel}
	var providers map[string]Provider
	var parseErr error
	if major == RuntimeV2 {
		providers, parseErr = parseV2ModelAPI(limitReader)
	} else {
		providers, parseErr = parseVerboseCatalog(limitReader)
	}
	if parseErr != nil {
		var catalogErr *CatalogError
		if errors.As(parseErr, &catalogErr) && catalogErr.Kind == CatalogErrorOutputTooLarge {
			return nil, catalogErr
		}
		return nil, catalogCommandErrorWithRunnerWait(ctx, r, parseErr)
	}
	if waiter, ok := r.(waitErrorReader); ok {
		if waitErr := waiter.WaitError(); waitErr != nil {
			return nil, catalogCommandError(ctx, waitErr)
		}
	}
	return providers, nil
}

func readCatalogVersion(ctx context.Context, r io.Reader) (RuntimeMajor, error) {
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	data, err := io.ReadAll(io.LimitReader(r, 4097))
	if err != nil {
		return RuntimeUnknown, catalogCommandErrorWithRunnerWait(ctx, r, err)
	}
	if len(data) > 4096 {
		return RuntimeUnknown, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	if waiter, ok := r.(waitErrorReader); ok {
		if err := waiter.WaitError(); err != nil {
			return RuntimeUnknown, catalogCommandError(ctx, err)
		}
	}
	major := ParseRuntimeMajor(string(data))
	if major == RuntimeUnknown {
		return major, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
	}
	return major, nil
}

// parseV2ModelAPI consumes the location-scoped V2 API response. Its explicit
// tools capability is required; model IDs alone cannot establish suitability
// for the SDD picker.
func parseV2ModelAPI(r io.Reader) (map[string]Provider, error) {
	var response struct {
		Data []struct {
			ID           string `json:"id"`
			ProviderID   string `json:"providerID"`
			ModelID      string `json:"modelID"`
			Name         string `json:"name"`
			Family       string `json:"family"`
			Enabled      *bool  `json:"enabled"`
			Capabilities *struct {
				Tools     *bool `json:"tools"`
				Reasoning *bool `json:"reasoning"`
			} `json:"capabilities"`
			Variants []struct {
				ID string `json:"id"`
			} `json:"variants"`
			Limit ModelLimit  `json:"limit"`
			Cost  []ModelCost `json:"cost"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&response); err != nil {
		return nil, catalogJSONError(err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
		}
		return nil, catalogJSONError(err)
	}
	if response.Data == nil {
		return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
	}
	providers := map[string]Provider{}
	for _, raw := range response.Data {
		if raw.ProviderID == "" || raw.ModelID == "" || raw.ID == "" || raw.Capabilities == nil || raw.Capabilities.Tools == nil || raw.Enabled == nil {
			return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
		}
		if !*raw.Enabled {
			continue
		}
		provider := providers[raw.ProviderID]
		if provider.ID == "" {
			provider = Provider{ID: raw.ProviderID, Name: raw.ProviderID, Models: map[string]Model{}}
		}
		variants := make([]string, 0, len(raw.Variants))
		for _, variant := range raw.Variants {
			if variant.ID == "" {
				return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
			}
			variants = append(variants, variant.ID)
		}
		sortVariants(variants)
		model := Model{ID: raw.ID, Name: raw.Name, Family: raw.Family, ToolCall: *raw.Capabilities.Tools, Limit: raw.Limit, Variants: variants}
		if raw.Capabilities.Reasoning != nil {
			model.Reasoning = *raw.Capabilities.Reasoning
		}
		if len(raw.Cost) > 0 {
			model.Cost = raw.Cost[0]
		}
		provider.Models[raw.ID] = model
		providers[raw.ProviderID] = provider
	}
	return providers, nil
}

func catalogJSONError(err error) error {
	var catalogErr *CatalogError
	if errors.As(err, &catalogErr) {
		return catalogErr
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return &CatalogError{Kind: CatalogErrorUnsupportedSchema}
	}
	return &CatalogError{Kind: CatalogErrorMalformed}
}

// MergeConfiguredCatalog augments the runtime catalog with file-backed config
// data without replacing runtime-discovered providers or models.
func MergeConfiguredCatalog(runtimeCatalog, configuredCatalog map[string]Provider) map[string]Provider {
	merged := cloneCatalog(runtimeCatalog)
	for providerID, configuredProvider := range configuredCatalog {
		runtimeProvider, exists := merged[providerID]
		if !exists {
			merged[providerID] = cloneProvider(configuredProvider)
			continue
		}
		if runtimeProvider.URL == "" {
			runtimeProvider.URL = configuredProvider.URL
		}
		if runtimeProvider.Models == nil {
			runtimeProvider.Models = map[string]Model{}
		}
		for modelID, configuredModel := range configuredProvider.Models {
			if existingModel, exists := runtimeProvider.Models[modelID]; !exists {
				if len(configuredModel.Variants) > 0 {
					variants := make([]string, len(configuredModel.Variants))
					copy(variants, configuredModel.Variants)
					configuredModel.Variants = variants
				}
				runtimeProvider.Models[modelID] = configuredModel
			} else if len(configuredModel.Variants) > 0 {
				existingVariants := make(map[string]struct{}, len(existingModel.Variants))
				for _, v := range existingModel.Variants {
					existingVariants[v] = struct{}{}
				}
				mergedVariants := append([]string(nil), existingModel.Variants...)
				for _, v := range configuredModel.Variants {
					if _, ok := existingVariants[v]; !ok {
						existingVariants[v] = struct{}{}
						mergedVariants = append(mergedVariants, v)
					}
				}
				sortVariants(mergedVariants)
				existingModel.Variants = mergedVariants
				runtimeProvider.Models[modelID] = existingModel
			}
		}
		merged[providerID] = runtimeProvider
	}
	return merged
}

func cloneCatalog(catalog map[string]Provider) map[string]Provider {
	clone := make(map[string]Provider, len(catalog))
	for id, provider := range catalog {
		clone[id] = cloneProvider(provider)
	}
	return clone
}

func cloneProvider(provider Provider) Provider {
	provider.Models = cloneModels(provider.Models)
	return provider
}

func cloneModels(models map[string]Model) map[string]Model {
	clone := make(map[string]Model, len(models))
	for id, model := range models {
		if len(model.Variants) > 0 {
			variants := make([]string, len(model.Variants))
			copy(variants, model.Variants)
			model.Variants = variants
		}
		clone[id] = model
	}
	return clone
}

// catalogCommandErrorWithRunnerWait classifies a parse failure while honoring
// a completed child's wait error: when the child already exited non-zero, the
// exit status wins over the parse classification.
func catalogCommandErrorWithRunnerWait(ctx context.Context, r io.Reader, parseErr error) error {
	if waiter, ok := r.(waitErrorReader); ok {
		if waitErr := waiter.WaitError(); waitErr != nil {
			return catalogCommandError(ctx, waitErr)
		}
	}
	return catalogCommandError(ctx, parseErr)
}

// catalogCommandError maps a raw discovery failure to the typed CatalogError
// taxonomy. A CatalogError passes through unchanged; deadline or context
// expiry becomes CatalogErrorTimeout; a missing or unresolvable binary becomes
// CatalogErrorMissingBinary; anything else becomes CatalogErrorCommandFailed.
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

// parseVerboseCatalog parses the streamed `opencode models --verbose` output.
// It reads one catalog record at a time: a `provider/model` header line
// followed by a JSON object. Header-shaped log lines (plugin and hook output)
// are tolerated as noise, but if the stream contains only noise the parse
// fails closed with CatalogErrorUnsupportedSchema. A stream that exceeds the
// defensive byte ceiling fails with CatalogErrorOutputTooLarge.
func parseVerboseCatalog(r io.Reader) (map[string]Provider, error) {
	providers := map[string]Provider{}
	sawNoise := false
	s := newStreamBuffer(r)

	for {
		line, err := s.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		header := strings.TrimSpace(line)
		if !isCatalogHeader(header) {
			if header != "" {
				sawNoise = true
			}
			continue
		}

		nextByte, err := s.peekNonWhitespace()
		if err != nil {
			if errors.Is(err, io.EOF) {
				sawNoise = true
				break
			}
			return nil, err
		}
		if nextByte != '{' {
			// A log line containing a slash but not followed by JSON
			sawNoise = true
			continue
		}

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

		if err := s.decodeJSON(&raw); err != nil {
			var catalogErr *CatalogError
			if errors.As(err, &catalogErr) {
				return nil, catalogErr
			}
			var typeErr *json.UnmarshalTypeError
			if errors.As(err, &typeErr) {
				return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
			}
			return nil, &CatalogError{Kind: CatalogErrorMalformed}
		}

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
		provider.Models[raw.ID] = Model{
			ID:        raw.ID,
			Name:      raw.Name,
			Family:    raw.Family,
			ToolCall:  *raw.Capabilities.ToolCall,
			Reasoning: reasoning,
			Limit:     raw.Limit,
			Cost:      raw.Cost,
			Variants:  variants,
		}
		providers[providerID] = provider
	}

	if len(providers) == 0 && sawNoise {
		return nil, &CatalogError{Kind: CatalogErrorUnsupportedSchema}
	}
	return providers, nil
}

// streamBuffer is an incremental line-oriented reader over the command's
// stdout. It buffers small chunks, serves line reads, and lets a json.Decoder
// consume the remainder of a record through Read while recovering any
// read-ahead bytes so subsequent header lines are never dropped.
type streamBuffer struct {
	r   io.Reader
	buf []byte
	eof bool
}

// newStreamBuffer wraps r in an incremental catalog stream reader.
func newStreamBuffer(r io.Reader) *streamBuffer {
	return &streamBuffer{r: r}
}

// fill pulls one chunk from the underlying reader into the buffer. It returns
// io.EOF only when the stream is exhausted and no buffered bytes remain;
// trailing bytes received alongside EOF stay available for subsequent reads.
func (s *streamBuffer) fill() error {
	if s.eof {
		return io.EOF
	}
	var chunk [4096]byte
	n, err := s.r.Read(chunk[:])
	if n > 0 {
		s.buf = append(s.buf, chunk[:n]...)
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			s.eof = true
			if len(s.buf) == 0 {
				return io.EOF
			}
			return nil
		}
		return err
	}
	return nil
}

// readLine returns the next newline-terminated line, tolerating a final line
// without a trailing newline. Buffered bytes are consumed before pulling more
// data from the underlying stream.
func (s *streamBuffer) readLine() (string, error) {
	for {
		if idx := bytes.IndexByte(s.buf, '\n'); idx >= 0 {
			line := string(s.buf[:idx])
			s.buf = s.buf[idx+1:]
			return line, nil
		}
		if err := s.fill(); err != nil {
			if errors.Is(err, io.EOF) {
				if len(s.buf) > 0 {
					line := string(s.buf)
					s.buf = nil
					return line, nil
				}
				return "", io.EOF
			}
			return "", err
		}
	}
}

// peekNonWhitespace discards buffered whitespace and returns the next
// non-whitespace byte, blocking on the underlying stream when needed. It
// distinguishes a real catalog header (followed by `{`) from a header-shaped
// log line without breaking incremental streaming.
func (s *streamBuffer) peekNonWhitespace() (byte, error) {
	for {
		for i, b := range s.buf {
			if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
				s.buf = s.buf[i:]
				return b, nil
			}
		}
		s.buf = nil
		if err := s.fill(); err != nil {
			return 0, err
		}
	}
}

// Read satisfies io.Reader so a json.Decoder can consume a record body
// directly from the buffer, serving buffered read-ahead first and delegating
// to the underlying stream only when the buffer is empty.
func (s *streamBuffer) Read(p []byte) (int, error) {
	if len(s.buf) > 0 {
		n := copy(p, s.buf)
		s.buf = s.buf[n:]
		return n, nil
	}
	if s.eof {
		return 0, io.EOF
	}
	return s.r.Read(p)
}

// decodeJSON decodes one catalog record from the stream. The decoder's
// read-ahead bytes are recovered back into the buffer so the next header line
// — even on the same chunk — is never dropped between records.
func (s *streamBuffer) decodeJSON(v interface{}) error {
	dec := json.NewDecoder(s)
	if err := dec.Decode(v); err != nil {
		return err
	}
	buffered, err := io.ReadAll(dec.Buffered())
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	s.buf = append(buffered, s.buf...)
	return nil
}

// isCatalogHeader reports whether line has the `provider/model` header shape:
// non-empty, no whitespace, at least one slash, and not the start of a JSON
// block. Anything else on stdout is log noise, never a catalog record.
func isCatalogHeader(line string) bool {
	return line != "" && !strings.ContainsAny(line, " \t") && strings.Contains(line, "/") && line[0] != '{'
}

// effortRank orders the known reasoning-effort variant names semantically.
var effortRank = map[string]int{
	"low":    0,
	"medium": 1,
	"high":   2,
	"xhigh":  3,
	"max":    4,
	"ultra":  5,
}

// sortVariants orders ranked effort levels first by rank, followed by
// non-effort/speed variants in stable lexical order.
func sortVariants(variants []string) {
	sort.Slice(variants, func(i, j int) bool {
		rI, okI := effortRank[variants[i]]
		rJ, okJ := effortRank[variants[j]]
		if okI && okJ {
			return rI < rJ
		}
		if okI != okJ {
			return okI
		}
		return variants[i] < variants[j]
	})
}

// catalogSizeCheckInterval bounds how often the capture-file size monitor
// samples the child's output while it runs.
const catalogSizeCheckInterval = 100 * time.Millisecond

// runCatalogCommand starts the OpenCode catalog command and returns a reader
// over its captured stdout. The child writes to a regular temp file, never a
// pipe: OpenCode V2's API command can truncate its own response when stdout is
// a pipe while the identical command through a regular file parses cleanly
// (issue #5833), so discovery always uses the file transport. Stderr goes to
// /dev/null because CatalogError never exposes command output, and a size
// monitor cancels the child if the capture file grows past the defensive
// ceiling. The returned reader must be Closed to cancel the process, reap the
// child, and remove the capture file.
func runCatalogCommand(ctx context.Context, command Command) (io.Reader, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	cmd.WaitDelay = catalogWaitDelay
	afterStart, release := configureProcessGroup(cmd)

	capture, err := os.CreateTemp("", "gentle-ai-opencode-catalog-")
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Stdout = capture
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		_ = capture.Close()
		_ = os.Remove(capture.Name())
		if release != nil {
			release()
		}
		cancel()
		return nil, err
	}
	if afterStart != nil {
		afterStart()
	}
	// The child inherited its own descriptor for the capture file; the
	// parent's write handle is no longer needed. The reader reopens the path
	// so it reads from the beginning with an independent file offset.
	_ = capture.Close()

	reader := newCatalogFileReader(cmd, capture.Name(), release, cancel)
	// Reap the child in the background as soon as it exits so reads can wait
	// on either new data or the child's termination without blocking on a
	// synchronous Wait.
	go reader.reap()
	go reader.monitorSize()
	return reader, nil
}

// catalogFileReader serves the catalog child's stdout captured to a regular
// temp file. Reads stream whatever the child has written so far; at end of
// file with the child still running, the reader waits for either new data or
// the child's exit (bounded by context cancellation) before deciding the
// stream is complete. Once the child is reaped, a terminal read reports its
// wait error when it exited non-zero, io.EOF otherwise. The wait error is
// also available through WaitError for callers that stop reading early, so a
// non-zero exit is never masked by a downstream parse classification.
//
// Concurrency: the background reaper goroutine owns the child lifecycle and
// publishes completion exclusively through the childDone and sizeDone channel
// closes; consumers therefore read the child's wait error only after
// observing childDone (or via sync.Once), which gives the required
// happens-before ordering without extra locking.
type catalogFileReader struct {
	cmd     *exec.Cmd
	path    string
	release func()
	cancel  context.CancelFunc

	waitOnce sync.Once
	waitErr  error
	// childDone is closed once the child has been reaped.
	childDone chan struct{}
	// sizeDone is closed once the child has been reaped, stopping the size
	// monitor.
	sizeDone chan struct{}
	// grew carries a token whenever the monitor observes the capture file
	// grow, waking a reader blocked at end of file. Capacity one: signals
	// coalesce and each wake re-checks the file.
	grew      chan struct{}
	lastKnown int64 // monitor goroutine only
	file      *os.File
	// exhausted marks the capture file as removed; consumer goroutine only.
	exhausted bool
}

func newCatalogFileReader(cmd *exec.Cmd, path string, release func(), cancel context.CancelFunc) *catalogFileReader {
	return &catalogFileReader{
		cmd:       cmd,
		path:      path,
		release:   release,
		cancel:    cancel,
		childDone: make(chan struct{}),
		sizeDone:  make(chan struct{}),
		grew:      make(chan struct{}, 1),
	}
}

// monitorSize watches the capture file while the child runs. It cancels the
// child when the file grows past the defensive ceiling, preserving the
// write-side bound the in-memory pipe buffer enforced before the file
// transport, and signals growth so a reader parked at end of file wakes
// promptly.
func (r *catalogFileReader) monitorSize() {
	ticker := time.NewTicker(catalogSizeCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.sizeDone:
			return
		case <-ticker.C:
			info, err := os.Stat(r.path)
			if err != nil {
				continue
			}
			if info.Size() > maxCatalogOutput {
				r.cancel()
				return
			}
			if info.Size() > r.lastKnown {
				r.lastKnown = info.Size()
				select {
				case r.grew <- struct{}{}:
				default:
				}
			}
		}
	}
}

// reap joins the child exactly once, stopping the size monitor and waking
// parked readers. It returns the child's wait error, which is empty for a
// clean exit.
func (r *catalogFileReader) reap() error {
	r.waitOnce.Do(func() {
		r.waitErr = r.cmd.Wait()
		close(r.sizeDone)
		if r.release != nil {
			r.release()
		}
		close(r.childDone)
	})
	return r.waitErr
}

// Read serves captured stdout bytes. End of file is only final once the child
// has been reaped: while it runs, the reader waits for either new data or the
// child's exit before reporting the terminal state.
func (r *catalogFileReader) Read(p []byte) (int, error) {
	n, err := r.readFile(p)
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	for {
		select {
		case <-r.childDone:
			// The child is reaped and the capture can no longer grow: one
			// last read drains the tail, then the stream is final.
			n, err = r.readFile(p)
			if !errors.Is(err, io.EOF) {
				return n, err
			}
			r.cleanup()
			if r.waitErr != nil {
				return 0, r.waitErr
			}
			return 0, io.EOF
		case <-r.grew:
			n, err = r.readFile(p)
			if !errors.Is(err, io.EOF) {
				return n, err
			}
			// Stale growth signal; keep waiting for data or child exit.
		}
	}
}

// readFile opens the capture path lazily and serves bytes from an independent
// read handle, so the child's writes never move the reader's offset.
func (r *catalogFileReader) readFile(p []byte) (int, error) {
	if r.exhausted {
		return 0, io.EOF
	}
	if r.file == nil {
		f, err := os.Open(r.path)
		if err != nil {
			return 0, err
		}
		r.file = f
	}
	return r.file.Read(p)
}

// Close cancels the underlying command, reaps the child, and removes the
// capture file exactly once.
func (r *catalogFileReader) Close() error {
	r.cancel()
	_ = r.reap()
	r.cleanup()
	return nil
}

// WaitError reports the child's wait error, reaping the child first. It
// implements waitErrorReader so discovery can classify a non-zero exit even
// when the parse itself failed.
func (r *catalogFileReader) WaitError() error {
	_ = r.reap()
	return r.waitErr
}

// cleanup releases the read handle and the capture file. Removal is
// best-effort: an escaped descendant may still hold the file open, and on
// Windows that keeps the remove failing harmlessly.
func (r *catalogFileReader) cleanup() {
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	if !r.exhausted {
		_ = os.Remove(r.path)
		r.exhausted = true
	}
}

// countingLimitReader enforces the defensive stdout ceiling: a stream of at
// most `limit` bytes parses normally, while any additional byte trips
// CatalogErrorOutputTooLarge and cancels the underlying command. A stream that
// ends exactly at the limit is valid, so once the counter reaches the limit a
// single probe read distinguishes a clean EOF from a true overflow.
type countingLimitReader struct {
	r      io.Reader
	limit  int64
	read   int64
	cancel context.CancelFunc
}

// Read serves the next bytes while enforcing the ceiling: reads are clamped
// to the remaining allowance, and once the counter reaches the limit a single
// extra probe byte distinguishes a clean end-of-stream from a true overflow.
// Overflow cancels the underlying command immediately.
func (c *countingLimitReader) Read(p []byte) (int, error) {
	maxToRead := int64(len(p))
	if remaining := c.limit - c.read; maxToRead > remaining {
		maxToRead = remaining
	}
	if maxToRead <= 0 {
		var probe [1]byte
		n, err := c.r.Read(probe[:])
		c.read += int64(n)
		if n > 0 {
			if c.cancel != nil {
				c.cancel()
			}
			return 0, &CatalogError{Kind: CatalogErrorOutputTooLarge}
		}
		return 0, err
	}
	n, err := c.r.Read(p[:maxToRead])
	c.read += int64(n)
	return n, err
}
