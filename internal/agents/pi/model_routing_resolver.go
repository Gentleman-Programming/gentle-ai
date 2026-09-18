package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const modelRoutingContract = "gentle-pi.model-routing/v1"
const MaxModelRoutingResponseBytes = 64 << 10

type Target int

const (TargetProject Target = iota; TargetGlobal)

var modelRoutingLookPath = exec.LookPath
var modelRoutingReadFile = os.ReadFile
var modelRoutingRunner = defaultModelRoutingRunner

func defaultModelRoutingRunner(ctx context.Context, bin string, req []byte) ([]byte, int, error) {
	if err := reStatBin(bin); err != nil {
		return nil, 0, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, bin)
	cmd.Stdin = bytes.NewReader(req)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, err
	}
	if err := cmd.Start(); err != nil {
		if runCtx.Err() != nil {
			return nil, 0, runCtx.Err()
		}
		return nil, 0, err
	}
	limit := int64(MaxModelRoutingResponseBytes + 1)
	type readRes struct {
		data []byte
		err  error
	}
	ch := make(chan readRes, 1)
	go func() {
		data, rerr := io.ReadAll(io.LimitReader(stdout, limit))
		ch <- readRes{data, rerr}
	}()
	var out []byte
	var readErr error
	select {
	case <-runCtx.Done():
		// Close the pipe reader to unblock io.ReadAll when a child process
		// has inherited stdout and keeps it open after the direct command is
		// killed by context cancellation. Without this, ReadAll blocks on EOF
		// even though the parent context is done.
		_ = stdout.Close()
		// Drain the reader goroutine to avoid leak, then reap the direct child.
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
		}
		_ = cmd.Wait()
		return nil, 0, runCtx.Err()
	case res := <-ch:
		out = res.data
		readErr = res.err
	}
	if readErr != nil {
		cancel()
		_ = cmd.Wait()
		if runCtx.Err() != nil {
			return nil, 0, runCtx.Err()
		}
		return nil, 0, readErr
	}
	if int64(len(out)) > int64(MaxModelRoutingResponseBytes) {
		cancel()
		_ = cmd.Wait()
		return nil, 0, &RoutingError{Kind: "invalid-json", Cause: errors.New("response too large")}
	}
	if err := cmd.Wait(); err != nil {
		if runCtx.Err() != nil {
			return nil, 0, runCtx.Err()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, ee.ExitCode(), nil
		}
		return nil, 0, err
	}
	return out, 0, nil
}
func reStatBin(p string) error {
	fi, err := os.Stat(p)
	if err != nil {
		return &RoutingError{Kind: "missing", Path: p, Cause: err}
	}
	if !fi.Mode().IsRegular() {
		return &RoutingError{Kind: "non-regular", Path: p}
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0 {
		return &RoutingError{Kind: "non-executable", Path: p}
	}
	return nil
}
func packageRootForSource(s, ad string) (string, error) {
	switch {
	case strings.HasPrefix(s, "npm:"):
		n := strings.TrimPrefix(s, "npm:")
		if i := strings.LastIndex(n, "@"); i > 0 {
			n = n[:i]
		}
		n = strings.TrimSpace(n)
		if n == "" || strings.Contains(n, "..") {
			return "", &RoutingError{Kind: "malformed", Path: s}
		}
		return filepath.Join(ad, "packages", "npm", filepath.FromSlash(n)), nil
	case strings.HasPrefix(s, "git:"):
		r := strings.TrimSpace(strings.TrimPrefix(s, "git:"))
		if r == "" {
			return "", &RoutingError{Kind: "malformed", Path: s}
		}
		return filepath.Join(ad, "packages", "git", filepath.FromSlash(r)), nil
	case strings.HasPrefix(s, "local:"):
		p := strings.TrimSpace(strings.TrimPrefix(s, "local:"))
		if p == "" {
			return "", &RoutingError{Kind: "malformed", Path: s}
		}
		if filepath.IsAbs(p) {
			return filepath.Clean(p), nil
		}
		return filepath.Join(ad, filepath.FromSlash(p)), nil
	}
	return "", &RoutingError{Kind: "malformed", Path: s}
}
func effectiveAgentDir(a string) string {
	if s := strings.TrimSpace(a); s != "" {
		return s
	}
	if v := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); v != "" {
		return v
	}
	if h, _ := os.UserHomeDir(); h != "" {
		return AgentConfigPath(h)
	}
	return a
}
func readSourceFromFile(p string) string {
	b, err := modelRoutingReadFile(p)
	if err != nil {
		return ""
	}
	var o map[string]any
	if err := json.Unmarshal(b, &o); err != nil {
		return ""
	}
	for _, v := range piPackagesAsSlice(o["packages"]) {
		identity := piPackageIdentity(v)
		if strings.HasPrefix(identity, "npm:") || strings.HasPrefix(identity, "git:") || strings.HasPrefix(identity, "local:") {
			return identity
		}
	}
	return ""
}
func selectedPackageSource(cwd, ad string) string {
	for _, p := range []string{filepath.Join(cwd, ".pi", "agent", "settings.json"), filepath.Join(cwd, ".pi", "settings.json")} {
		if s := readSourceFromFile(p); s != "" {
			return s
		}
	}
	if a := effectiveAgentDir(ad); a != "" {
		if s := readSourceFromFile(filepath.Join(a, "settings.json")); s != "" {
			return s
		}
	}
	return ""
}
func probeBin(ctx context.Context, bin string) (bool, error) {
	req, _ := json.Marshal(map[string]string{"contract": modelRoutingContract, "op": "capabilities"})
	pc, cancel := ctx, func() {}
	if _, ok := ctx.Deadline(); !ok {
		pc, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}
	_ = cancel
	out, exit, err := modelRoutingRunner(pc, bin, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || pc.Err() != nil {
			return false, &RoutingError{Kind: "timeout", Path: bin, Cause: err}
		}
		return false, &RoutingError{Kind: "probe-failed", Path: bin, Cause: err}
	}
	if len(out) > MaxModelRoutingResponseBytes {
		return false, &RoutingError{Kind: "invalid-json", Path: bin, Cause: errors.New("response too large")}
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(out, &raw); err != nil {
		return false, &RoutingError{Kind: "invalid-json", Path: bin, Cause: err}
	}
	cr, ok := raw["contract"]
	if !ok {
		return false, &RoutingError{Kind: "unsupported-contract", Path: bin}
	}
	var c string
	if err := json.Unmarshal(cr, &c); err != nil || c != modelRoutingContract {
		return false, &RoutingError{Kind: "unsupported-contract", Path: bin}
	}
	if exit != 0 {
		return false, &RoutingError{Kind: "probe-failed", Path: bin, Cause: fmt.Errorf("exit %d", exit)}
	}
	return true, nil
}
func ResolveModelRoutingExecutable(ctx context.Context, cwd, ad string, t Target) (string, error) {
	if bin, err := modelRoutingLookPath(packageBinName); err == nil && bin != "" {
		if err := reStatBin(bin); err == nil {
			if ok, pe := probeBin(ctx, bin); ok {
				return bin, nil
			} else if pe != nil {
				var re *RoutingError
				if errors.As(pe, &re) && re.Kind == "timeout" {
					return "", pe
				}
			}
		}
	}
	src := selectedPackageSource(cwd, ad)
	if t == TargetGlobal {
		if a := effectiveAgentDir(ad); a != "" {
			if u := readSourceFromFile(filepath.Join(a, "settings.json")); u != "" {
				if proj := readSourceFromFile(filepath.Join(cwd, ".pi", "agent", "settings.json")); proj != "" && u != proj {
					src = u
				} else if proj2 := readSourceFromFile(filepath.Join(cwd, ".pi", "settings.json")); proj2 != "" && u != proj2 {
					src = u
				}
			}
		}
	}
	if src == "" {
		return "", &RoutingError{Kind: "missing", Path: cwd}
	}
	root, err := packageRootForSource(src, effectiveAgentDir(ad))
	if err != nil {
		return "", err
	}
	bin, err := ResolvePackageBin(root)
	if err != nil {
		return "", err
	}
	if err := reStatBin(bin); err != nil {
		return "", err
	}
	if ok, pe := probeBin(ctx, bin); ok {
		return bin, nil
	} else if pe != nil {
		return "", pe
	}
	return "", &RoutingError{Kind: "missing", Path: cwd}
}

var _ = bytes.NewReader
var _ = time.Second
