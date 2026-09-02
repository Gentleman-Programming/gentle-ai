package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	catalogTimeout   = 15 * time.Second
	maxCatalogOutput = 16 << 20
)

// Command describes the bounded OpenCode command used for runtime discovery.
type Command struct {
	Path string
	Args []string
	Dir  string
}

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
// wait error on Close, mirroring processStreamReader. It lets the discovery
// pipeline honor the child's exit status instead of masking it with a parse
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
	limit := maxCatalogOutput
	r, err := runner(ctx, Command{Path: "opencode", Args: []string{"models", "--verbose"}, Dir: projectDir})
	if err != nil {
		return nil, catalogCommandError(ctx, err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	limitReader := &countingLimitReader{r: r, limit: int64(limit), cancel: cancel}
	providers, parseErr := parseVerboseCatalog(limitReader)
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

// runCatalogCommand starts `opencode models --verbose` and returns a reader
// over its stdout. Both pipes are drained concurrently into bounded state so
// a slow consumer can never make the child block or die on a full pipe:
// OpenCode's catalog writer aborts when its stdout pipe drains too slowly,
// which would otherwise truncate the catalog mid-record. Stdout is retained
// up to the defensive ceiling; stderr is discarded because CatalogError never
// exposes command output. The returned reader must be Closed to cancel the
// process and reap the child.
func runCatalogCommand(ctx context.Context, command Command) (io.Reader, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	configureProcessGroup(cmd)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		_ = stdoutPipe.Close()
		cancel()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = stdoutPipe.Close()
		_ = stderrPipe.Close()
		cancel()
		return nil, err
	}

	stream := newBoundedPipeBuffer(maxCatalogOutput, cancel)
	stderrDone := make(chan struct{})

	// Drain stdout at full speed into the bounded buffer.
	go func() {
		var chunk [8192]byte
		for {
			n, readErr := stdoutPipe.Read(chunk[:])
			if n > 0 {
				if overflow := stream.append(chunk[:n]); overflow {
					cancel()
					readErr = io.EOF
				}
			}
			if readErr != nil {
				break
			}
		}
		stream.finish()
	}()

	// Drain stderr concurrently so a chatty child can never fill its pipe and
	// deadlock. Discarded on purpose: CatalogError never exposes command
	// output, so there is nothing to retain.
	go func() {
		_, _ = io.Copy(io.Discard, stderrPipe)
		close(stderrDone)
	}()

	return &processStreamReader{
		cmd:        cmd,
		stream:     stream,
		stderrDone: stderrDone,
		cancel:     cancel,
	}, nil
}

// boundedPipeBuffer accumulates a child's stdout up to a defensive ceiling so
// the child can write at full speed while consumers parse at their own pace.
// Reads block until data is available or the drain goroutine finishes.
type boundedPipeBuffer struct {
	mu       sync.Mutex
	cond     *sync.Cond
	buf      []byte
	pos      int64
	finished bool
	overflow bool
	limit    int64
	cancel   context.CancelFunc
}

func newBoundedPipeBuffer(limit int64, cancel context.CancelFunc) *boundedPipeBuffer {
	b := &boundedPipeBuffer{limit: limit, cancel: cancel}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// append stores one drained chunk. It reports overflow — and stops accepting
// further bytes — once the ceiling is exceeded.
func (b *boundedPipeBuffer) append(data []byte) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.overflow {
		return true
	}
	if int64(len(b.buf))+int64(len(data)) > b.limit {
		b.overflow = true
		b.cond.Broadcast()
		return true
	}
	b.buf = append(b.buf, data...)
	b.cond.Broadcast()
	return false
}

// finish marks the drain as complete so blocked readers observe EOF.
func (b *boundedPipeBuffer) finish() {
	b.mu.Lock()
	b.finished = true
	b.cond.Broadcast()
	b.mu.Unlock()
}

// Read serves buffered bytes, blocking while the drain is still running and
// no data is available.
func (b *boundedPipeBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for b.pos == int64(len(b.buf)) && !b.finished {
		b.cond.Wait()
	}
	if b.overflow {
		if b.cancel != nil {
			b.cancel()
		}
		return 0, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	n := copy(p, b.buf[b.pos:])
	b.pos += int64(n)
	if b.pos == int64(len(b.buf)) && b.finished {
		if n == 0 {
			return 0, io.EOF
		}
		return n, nil
	}
	return n, nil
}

// processStreamReader owns the started child process: it exposes the drained
// stdout buffer as an io.Reader, drains stderr on a background goroutine, and
// on Close cancels the process and waits for both drains and the child.
type processStreamReader struct {
	cmd        *exec.Cmd
	stream     *boundedPipeBuffer
	stderrDone chan struct{}
	cancel     context.CancelFunc
	closed     bool
	waitErr    error
}

// Read serves drained stdout bytes; it returns the child's exit error after
// the stream is exhausted.
func (p *processStreamReader) Read(buf []byte) (int, error) {
	n, err := p.stream.Read(buf)
	if n > 0 {
		return n, nil
	}
	if err != nil {
		return n, err
	}
	p.closeAndReap()
	if p.waitErr != nil {
		return 0, p.waitErr
	}
	return 0, io.EOF
}

// Close cancels the underlying command and reaps the child process exactly
// once. Cancellation is honored even when descendants inherited the pipes:
// the child is reaped first (it dies with the context), and only then the
// stderr drain is awaited.
func (p *processStreamReader) Close() error {
	p.cancel()
	p.closeAndReap()
	return nil
}

// WaitError reports the child's wait error after the stream was reaped. It
// implements waitErrorReader so discovery can classify a non-zero exit even
// when the parse itself failed.
func (p *processStreamReader) WaitError() error {
	p.closeAndReap()
	return p.waitErr
}

// closeAndReap performs the idempotent shutdown sequence: reap the child and
// only afterwards wait for the stderr drain. The stdout drain goroutine ends
// on its own once the child's pipe is closed by Wait. Cancellation is not
// called here: the context that owns the child (Close or the deadline) drives
// it, and cancelling before Wait would make Wait report context cancellation
// instead of the child's real exit status.
func (p *processStreamReader) closeAndReap() {
	if p.closed {
		return
	}
	p.closed = true
	p.waitErr = p.cmd.Wait()
	<-p.stderrDone
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

// ParseCatalogForDiag is a temporary diagnostic entry point; remove before commit.
func ParseCatalogForDiag(r io.Reader) (map[string]Provider, error) {
	return parseVerboseCatalog(r)
}
