//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func nativeChildFixtureStamp(t *testing.T, root string) string {
	t.Helper()
	entries := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, cause error) error {
		if cause != nil {
			return cause
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := privateStamp(info)
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(data))
		}
		entries[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func TestUserNativeChildPublicDeadlineRefusal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, deadline := range []string{
		"bad",
		strconv.FormatInt(time.Now().Add(-time.Second).UnixNano(), 10),
		strconv.FormatInt(time.Now().Add(141*time.Second).UnixNano(), 10),
	} {
		before := nativeChildFixtureStamp(t, root)
		var output bytes.Buffer
		args := []string{"internal-native-produce", root, root + "/stage", deadline,
			root + "/destination", "separate", "", "", strings.Repeat("0", 64)}
		err := RunUserEntry(context.Background(), "", args, nil, &output, &output)
		if err == nil || output.Len() != 0 || before != nativeChildFixtureStamp(t, root) {
			t.Fatal("invalid child input changed state", err, output.String())
		}
	}
}

// The isolated-manager harness supplies a fresh fixture per qualification run.
func TestUserNativeChildPrivateRefusal(t *testing.T) {
	scenario := os.Getenv("GENTLE_CHILD_ENTRY_SCENARIO")
	if scenario == "" {
		t.Skip("requires isolated private manager and adversarial child fixture")
	}
	switch scenario {
	case "source", "node", "confirmation", "plain-rw":
	default:
		t.Fatal("unknown private child scenario", scenario)
	}
	ctx := context.Background()
	root := "/fixture/native-worker"
	before := nativeChildFixtureStamp(t, root)
	if _, err := userNativeOperation(ctx); err != nil {
		t.Fatal("actual operation membership failed", err)
	}
	boundary := userNativeExecutionCheck(root, "/run/user/1000")
	if (boundary == nil) != (scenario != "plain-rw") {
		t.Fatal("wrong actual boundary", scenario, boundary)
	}
	var output bytes.Buffer
	args := []string{"internal-native-produce", root, "/fixture/final-parent/stage",
		strconv.FormatInt(time.Now().Add(30*time.Second).UnixNano(), 10),
		"/fixture/final-parent/destination", "separate", "", "", strings.Repeat("0", 64)}
	err := RunUserEntry(ctx, "", args, nil, &output, &output)
	if err == nil || output.Len() != 0 {
		t.Fatal("refused child reported success", scenario, err, output.String())
	}
	if scenario == "source" && !errors.Is(err, errNativeSourceSnapshot) {
		t.Fatal("source gate not reached", err)
	}
	if scenario == "node" && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("missing Node gate not reached", err)
	}
	if scenario == "confirmation" {
		var failure *PrivateRuntimeError
		if !errors.As(err, &failure) || failure.Cause == nil ||
			!strings.Contains(failure.Cause.Error(), "physical selection confirmation differs") {
			t.Fatal("confirmation gate not reached", err)
		}
	}
	if before != nativeChildFixtureStamp(t, root) {
		t.Fatal("refused child changed producer", scenario)
	}
	for _, path := range []string{root + "/data", root + "/runtime/native-bootstrap",
		"/fixture/final-parent/stage", "/fixture/final-parent/destination"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("refused child prepared original or published", path, err)
		}
	}
	t.Log("ACTUAL_CHILD_REFUSAL", scenario, err)
}
