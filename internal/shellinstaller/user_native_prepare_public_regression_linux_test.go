//go:build linux && amd64 && shell_native_runtime

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func independentPrepareState(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		st, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s:%o:", rel, st.Mode())
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprint(h, target)
		} else if st.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, readErr := io.Copy(h, f)
			return errors.Join(readErr, f.Close())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func independentPrepareWorker(t *testing.T, kind string) (*NativeInstallerRuntime, string, string) {
	t.Helper()
	ctx := context.Background()
	parent := filepath.Dir(installStagePath(t))
	root, source := filepath.Join(parent, "root"), filepath.Join(parent, "source")
	if err := userPrepareInstallStage(ctx, root, "separate", nil); err != nil {
		t.Fatal(err)
	}
	nodeDir := filepath.Join(root, "runtime/node/bin")
	if err := os.MkdirAll(nodeDir, 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/cp", "--no-dereference", "/opt/node/bin/node", nodeDir+"/node")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := os.Chmod(nodeDir+"/node", 0700); err != nil {
		t.Fatal(err)
	}
	scripts := filepath.Join(source, "scripts")
	if err := os.MkdirAll(scripts, 0700); err != nil {
		t.Fatal(err)
	}
	factory := `export function createInstallerRuntime(){return {collectPlan:async()=>({plan:{fixture:true}}),runInstall:async()=>({outcome:"blocked",reason:"fixture",completed:[]})}}`
	if kind == "collect" {
		factory = `export function createInstallerRuntime(){return {collectPlan:async()=>{throw new Error("PRIVATE_DIAGNOSTIC_MARKER")}}}`
	}
	bootstrap := `export async function bootstrap(){throw new Error("PRIVATE_DIAGNOSTIC_MARKER")}`
	if kind == "deadline" {
		bootstrap = `export async function bootstrap(){process.stderr.write("PRIVATE_DIAGNOSTIC_MARKER\n");await new Promise(()=>setInterval(()=>{},1000))}`
	}
	for name, body := range map[string]string{"installer-runtime.mjs": factory, "installer-downloads.mjs": bootstrap} {
		if err := os.WriteFile(filepath.Join(scripts, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	w, err := NewNativeInstallerRuntime(ctx, root, source)
	if err != nil {
		t.Fatal(err)
	}
	return w, root, source
}

func TestIndependentPreparePublicBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("requires authenticated private Node")
	}
	if err := UserKernelCheck(); err != nil {
		t.Fatal(err)
	}
	baseline := os.Getenv("INDEPENDENT_PREPARE_BASELINE") == "1"
	proof := map[string]any{}
	t.Run("failure-and-refusal", func(t *testing.T) {
		w, root, source := independentPrepareWorker(t, "failure")
		before := independentPrepareState(t, filepath.Dir(root))
		sourceBefore := independentPrepareState(t, source)
		for _, canceled := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			if canceled {
				cancel()
			}
			_, err := w.Prepare(ctx, canceled)
			cancel()
			var typed *PrivateRuntimeError
			want := "refused"
			if canceled {
				want = "canceled"
			}
			if !errors.As(err, &typed) || typed.Kind != want || before != independentPrepareState(t, filepath.Dir(root)) {
				t.Fatal("preexecution refusal mutated state", err)
			}
		}
		_, err := w.Prepare(context.Background(), true)
		var exit *exec.ExitError
		var typed *PrivateRuntimeError
		if err == nil || !errors.As(err, &exit) || !errors.As(err, &typed) || typed.Kind != "nonzero" || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC_MARKER") {
			t.Fatal("public failure privacy/taxonomy", err)
		}
		proof["nonzeroSignature"] = err.Error()
		receipt := root + "/native-prepare.stderr"
		data, readErr := os.ReadFile(receipt)
		if baseline {
			if !os.IsNotExist(readErr) {
				t.Fatal("baseline unexpectedly retained diagnostic", readErr)
			}
		} else {
			st, e := os.Lstat(receipt)
			if e != nil || readErr != nil || st.Mode() != 0600 || len(data) > 4096 || !strings.Contains(string(data), "PRIVATE_DIAGNOSTIC_MARKER") {
				t.Fatal("missing private diagnostic", e, readErr)
			}
			_, err = w.Prepare(context.Background(), true)
			if !errors.As(err, &exit) || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC_MARKER") {
				t.Fatal("repeat changed failure", err)
			}
			again, e := os.ReadFile(receipt)
			if e != nil || string(again) != string(data) {
				t.Fatal("first diagnostic overwritten", e)
			}
			if err := os.Remove(receipt); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(filepath.Dir(root), "outside-marker")
			if err := os.WriteFile(outside, []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, receipt); err != nil {
				t.Fatal(err)
			}
			_, err = w.Prepare(context.Background(), true)
			if !errors.As(err, &exit) || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC_MARKER") {
				t.Fatal("symlink changed failure", err)
			}
			marker, e := os.ReadFile(outside)
			if e != nil || string(marker) != "preserved" {
				t.Fatal("symlink target overwritten", e)
			}
		}
		plan, err := w.invokeWithEnvironment(context.Background(), "collect", nil, false, nil)
		if err != nil || string(plan) != `{"fixture":true}` {
			t.Fatal("Collect changed", err)
		}
		result, err := w.invokeWithEnvironment(context.Background(), "run", plan, true, nil)
		if err != nil || string(result) != `{"outcome":"blocked","reason":"fixture","completed":[]}` {
			t.Fatal("Run changed", err)
		}
		if sourceBefore != independentPrepareState(t, source) {
			t.Fatal("fake source changed")
		}
		proof["collectRunUnchanged"] = true
	})
	t.Run("deadline", func(t *testing.T) {
		w, root, source := independentPrepareWorker(t, "deadline")
		sourceBefore := independentPrepareState(t, source)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := w.Prepare(ctx, true)
		var typed *PrivateRuntimeError
		if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &typed) || typed.Kind != "deadline" || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC_MARKER") {
			t.Fatal("deadline classification/privacy", err)
		}
		data, e := os.ReadFile(root + "/native-prepare.stderr")
		if baseline {
			if !os.IsNotExist(e) {
				t.Fatal(e)
			}
		} else if e != nil || len(data) > 4096 || !strings.Contains(string(data), "PRIVATE_DIAGNOSTIC_MARKER") {
			t.Fatal("deadline diagnostic missing", e)
		}
		if sourceBefore != independentPrepareState(t, source) {
			t.Fatal("deadline source changed")
		}
		proof["deadlineSignature"] = err.Error()
	})
	t.Run("nonprepare-failure", func(t *testing.T) {
		w, root, source := independentPrepareWorker(t, "collect")
		before := independentPrepareState(t, filepath.Dir(root))
		_, err := w.invokeWithEnvironment(context.Background(), "collect", nil, false, nil)
		var exit *exec.ExitError
		if !errors.As(err, &exit) || strings.Contains(err.Error(), "PRIVATE_DIAGNOSTIC_MARKER") {
			t.Fatal("Collect failure changed", err)
		}
		if _, err := os.Lstat(root + "/native-prepare.stderr"); !os.IsNotExist(err) {
			t.Fatal("Collect wrote diagnostic", err)
		}
		if before != independentPrepareState(t, filepath.Dir(root)) {
			t.Fatal("Collect failure mutated state", source)
		}
		proof["collectFailureSignature"] = err.Error()
	})
	b, err := json.MarshalIndent(proof, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/report/prepare-proof.json", b, 0600); err != nil {
		t.Fatal(err)
	}
}
