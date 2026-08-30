package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type RoutingError struct{ Kind, Path string; Cause error }

func (e *RoutingError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("routing error (%s) at %q: %v", e.Kind, e.Path, e.Cause)
	}
	return fmt.Sprintf("routing error (%s) at %q", e.Kind, e.Path)
}
func (e *RoutingError) Unwrap() error { return e.Cause }

type ModelRoutingClient struct {
	Bin     string
	Timeout time.Duration
	Runner  func(ctx context.Context, bin string, req []byte) ([]byte, int, error)
}

type wireRequest struct {
	Contract string          `json:"contract"`
	Op       string          `json:"op"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type CapabilitiesResponse struct {
	Contract string `json:"contract"`
	Ok       bool   `json:"ok"`
}
type InspectRequest struct{ Path string `json:"path"` }
type InspectResponse struct{ Contract string `json:"contract"` }
type ValidateResponse struct{ Contract string `json:"contract"` }
type ApplyResponse struct{ Contract string `json:"contract"` }

func (c *ModelRoutingClient) do(ctx context.Context, op string, payload json.RawMessage) ([]byte, error) {
	if c.Bin == "" {
		return nil, &RoutingError{Kind: "missing", Path: c.Bin}
	}
	data, err := json.Marshal(wireRequest{Contract: modelRoutingContract, Op: op, Payload: payload})
	if err != nil {
		return nil, &RoutingError{Kind: "invalid-json", Path: c.Bin, Cause: err}
	}
	if len(data) > MaxModelRoutingResponseBytes {
		return nil, &RoutingError{Kind: "invalid-json", Cause: errors.New("request too large")}
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if c.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	runner := c.Runner
	if runner == nil {
		runner = modelRoutingRunner
	}
	if c.Runner == nil {
		if err := reStatBin(c.Bin); err != nil {
			return nil, err
		}
	}
	out, exit, err := runner(runCtx, c.Bin, data)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || runCtx.Err() != nil || strings.Contains(strings.ToLower(err.Error()), "deadline") || strings.Contains(strings.ToLower(err.Error()), "timeout") {
			return nil, &RoutingError{Kind: "timeout", Path: c.Bin, Cause: err}
		}
		return nil, &RoutingError{Kind: "probe-failed", Path: c.Bin, Cause: err}
	}
	if len(out) > MaxModelRoutingResponseBytes {
		return nil, &RoutingError{Kind: "invalid-json", Path: c.Bin, Cause: errors.New("response too large")}
	}
	if buf, _ := io.ReadAll(io.LimitReader(bytes.NewReader(out), int64(MaxModelRoutingResponseBytes+1))); len(buf) > MaxModelRoutingResponseBytes {
		return nil, &RoutingError{Kind: "invalid-json", Path: c.Bin, Cause: errors.New("response too large")}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, &RoutingError{Kind: "invalid-json", Path: c.Bin, Cause: err}
	}
	cr, ok := raw["contract"]
	if !ok {
		return nil, &RoutingError{Kind: "unsupported-contract", Path: c.Bin}
	}
	var contract string
	if err := json.Unmarshal(cr, &contract); err != nil || contract != modelRoutingContract {
		return nil, &RoutingError{Kind: "unsupported-contract", Path: c.Bin}
	}
	if exit != 0 {
		return nil, &RoutingError{Kind: "probe-failed", Path: c.Bin, Cause: fmt.Errorf("exit %d", exit)}
	}
	return out, nil
}
func (c *ModelRoutingClient) Capabilities(ctx context.Context) (*CapabilitiesResponse, error) {
	out, err := c.do(ctx, "capabilities", nil)
	if err != nil {
		return nil, err
	}
	var r CapabilitiesResponse
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, &RoutingError{Kind: "invalid-json", Cause: err}
	}
	return &r, nil
}
func (c *ModelRoutingClient) Inspect(ctx context.Context, req InspectRequest) (*InspectResponse, error) {
	p, _ := json.Marshal(req)
	out, err := c.do(ctx, "inspect", p)
	if err != nil {
		return nil, err
	}
	var r InspectResponse
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, &RoutingError{Kind: "invalid-json", Cause: err}
	}
	return &r, nil
}
func (c *ModelRoutingClient) Validate(ctx context.Context, draft json.RawMessage) (*ValidateResponse, error) {
	out, err := c.do(ctx, "validate", draft)
	if err != nil {
		return nil, err
	}
	var r ValidateResponse
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, &RoutingError{Kind: "invalid-json", Cause: err}
	}
	return &r, nil
}
func (c *ModelRoutingClient) Apply(ctx context.Context, draft json.RawMessage) (*ApplyResponse, error) {
	out, err := c.do(ctx, "apply", draft)
	if err != nil {
		return nil, err
	}
	var r ApplyResponse
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, &RoutingError{Kind: "invalid-json", Cause: err}
	}
	return &r, nil
}
