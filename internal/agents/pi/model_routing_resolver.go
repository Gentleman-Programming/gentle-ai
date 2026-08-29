package pi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	cmd := exec.CommandContext(ctx, bin)
	cmd.Stdin = bytes.NewReader(req)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.Bytes(), ee.ExitCode(), nil
		}
		return nil, 0, err
	}
	b := out.Bytes()
	if len(b) > MaxModelRoutingResponseBytes {
		return nil, 0, &RoutingError{Kind: "invalid-json", Cause: errors.New("response too large")}
	}
	return b, 0, nil
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
		if s, ok := v.(string); ok && (strings.HasPrefix(s, "npm:") || strings.HasPrefix(s, "git:") || strings.HasPrefix(s, "local:")) {
			return s
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
