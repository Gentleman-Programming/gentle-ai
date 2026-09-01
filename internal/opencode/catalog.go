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
	"time"
)

const (
	catalogTimeout   = 15 * time.Second
	maxCatalogOutput = 16 << 20
	maxStderrOutput  = 64 << 10
)

// Command describes the bounded OpenCode command used for runtime discovery.
type Command struct {
	Path        string
	Args        []string
	Dir         string
	OutputLimit int
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

func (e *CatalogError) Error() string { return string(e.Kind) }

// DiscoverCatalog reads OpenCode's effective provider catalog for projectDir.
func DiscoverCatalog(ctx context.Context, projectDir string) (map[string]Provider, error) {
	return DiscoverCatalogWithRunner(ctx, projectDir, runCatalogCommand)
}

// DiscoverCatalogWithRunner permits deterministic command-boundary tests.
func DiscoverCatalogWithRunner(ctx context.Context, projectDir string, runner CommandRunner) (map[string]Provider, error) {
	ctx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()
	limit := maxCatalogOutput
	r, err := runner(ctx, Command{Path: "opencode", Args: []string{"models", "--verbose"}, Dir: projectDir, OutputLimit: limit})
	if err != nil {
		return nil, catalogCommandError(ctx, err)
	}
	if closer, ok := r.(io.Closer); ok {
		defer closer.Close()
	}
	limitReader := &countingLimitReader{r: r, limit: int64(limit), cancel: cancel}
	providers, parseErr := parseVerboseCatalog(limitReader)
	if parseErr != nil {
		return nil, catalogCommandError(ctx, parseErr)
	}
	return providers, nil
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

type streamBuffer struct {
	r   io.Reader
	buf []byte
	eof bool
}

func newStreamBuffer(r io.Reader) *streamBuffer {
	return &streamBuffer{r: r}
}

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

func runCatalogCommand(ctx context.Context, command Command) (io.Reader, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir

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

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		var buf [maxStderrOutput]byte
		for {
			_, readErr := stderrPipe.Read(buf[:])
			if readErr != nil {
				break
			}
		}
	}()

	return &processStreamReader{
		cmd:        cmd,
		stdout:     stdoutPipe,
		stderrDone: stderrDone,
		cancel:     cancel,
	}, nil
}

type processStreamReader struct {
	cmd        *exec.Cmd
	stdout     io.ReadCloser
	stderrDone chan struct{}
	cancel     context.CancelFunc
	closed     bool
	waitErr    error
	eof        bool
}

func (p *processStreamReader) Read(buf []byte) (int, error) {
	if p.eof {
		if p.waitErr != nil {
			return 0, p.waitErr
		}
		return 0, io.EOF
	}
	n, err := p.stdout.Read(buf)
	if err != nil {
		p.eof = true
		p.closeAndReap()
		if err == io.EOF {
			if p.waitErr != nil {
				return n, p.waitErr
			}
			return n, io.EOF
		}
	}
	return n, err
}

func (p *processStreamReader) Close() error {
	p.cancel()
	p.closeAndReap()
	return nil
}

func (p *processStreamReader) closeAndReap() {
	if p.closed {
		return
	}
	p.closed = true
	_ = p.stdout.Close()
	<-p.stderrDone
	p.waitErr = p.cmd.Wait()
}

type countingLimitReader struct {
	r      io.Reader
	limit  int64
	read   int64
	cancel context.CancelFunc
}

func (c *countingLimitReader) Read(p []byte) (int, error) {
	if c.read >= c.limit {
		if c.cancel != nil {
			c.cancel()
		}
		return 0, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	maxToRead := int64(len(p))
	if remaining := c.limit - c.read; maxToRead > remaining {
		maxToRead = remaining
	}
	n, err := c.r.Read(p[:maxToRead])
	c.read += int64(n)
	if c.read >= c.limit {
		if c.cancel != nil {
			c.cancel()
		}
		return n, &CatalogError{Kind: CatalogErrorOutputTooLarge}
	}
	return n, err
}
