//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUserNativeCoordinateDeadlineRefusal(t *testing.T) {
	parent := filepath.Dir(installStagePath(t))
	dest := parent + "/destination"
	req := UserInstallRequest{Destination: dest, Mode: "separate"}
	token, e := InspectUserInstall(req)
	if e != nil {
		t.Fatal(e)
	}
	args := []string{"install", dest, "separate", "", "", token}
	before := nativeChildFixtureStamp(t, parent)
	var out bytes.Buffer
	for _, deadline := range []string{"invalid", strconv.FormatInt(time.Now().Add(-time.Second).UnixNano(), 10), strconv.FormatInt(time.Now().Add(141*time.Second).UnixNano(), 10)} {
		entry := append([]string{"internal-native-coordinate", parent, parent + "-stage", deadline}, args[1:]...)
		if e := RunUserEntry(context.Background(), "", entry, nil, &out, &out); e == nil || out.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
			t.Fatal("coordinator deadline refusal changed state", e)
		}
	}
}
func TestUserNativeCoordinatePrivate(t *testing.T) {
	if testing.Short() {
		t.Skip("requires isolated private manager")
	}
	mode := os.Getenv("GENTLE_COORDINATE_VERIFY")
	if mode == "" {
		t.Skip("requires isolated private manager")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if mode == "child" {
		operation, e := userNativeOperation(ctx)
		if e != nil {
			t.Fatal(e)
		}
		expected, e := os.ReadFile("/fixture/expected-operation")
		if e != nil || operation.group.Path != string(expected) {
			t.Fatal("coordinator is not in owner's actual aggregate", operation, e)
		}
		if e := userNativeCoordinatorCheck(); e != nil {
			t.Fatal(e)
		}
		base := "/fixture/selection"
		deadline := strconv.FormatInt(time.Now().Add(20*time.Second).UnixNano(), 10)
		req := UserInstallRequest{Destination: base + "/destination", Mode: "separate"}
		token, e := InspectUserInstall(req)
		if e != nil {
			t.Fatal(e)
		}
		for _, tt := range []struct{ name, workspace, stage string }{{"prefix", base + "/wrong", base + "/wrong-stage"}, {"stage", base + "/.gentle-native-empty", base + "/bad-stage"}, {"nonempty", base + "/.gentle-native-filled", base + "/.gentle-native-filled-stage"}, {"symlink", base + "/.gentle-native-link", base + "/.gentle-native-link-stage"}} {
			t.Run(tt.name, func(t *testing.T) {
				before := nativeChildFixtureStamp(t, base)
				var out bytes.Buffer
				args := []string{"internal-native-coordinate", tt.workspace, tt.stage, deadline, req.Destination, "separate", "", "", token}
				e := RunUserEntry(ctx, "", args, nil, &out, &out)
				if e == nil || out.Len() != 0 || before != nativeChildFixtureStamp(t, base) {
					t.Fatal("invalid coordinator preimage changed state", e, out.String())
				}
			})
		}
		t.Log("ACTUAL_COORDINATOR_AGGREGATE_AND_PATH_REFUSAL", operation.group.Path)
		return
	}
	if e := userNativeCoordinatorCheck(); e != nil {
		t.Fatal(e)
	}
	base := "/fixture/selection"
	for _, p := range []string{base, base + "/wrong", base + "/.gentle-native-empty", base + "/.gentle-native-filled"} {
		if e := os.Mkdir(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(base+"/.gentle-native-filled/marker", []byte("preserve"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(base+"/.gentle-native-empty", base+"/.gentle-native-link"); e != nil {
		t.Fatal(e)
	}
	operation, e := newUserNativeSlice(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile("/fixture/expected-operation", []byte(operation.group.Path), 0600); e != nil {
		t.Fatal(e)
	}
	plan, e := newUserNativeServicePlan(ctx, operation, nil)
	if e != nil {
		t.Fatal(e)
	}
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	env := append(append([]string{}, operation.env...), "GENTLE_COORDINATE_VERIFY=child")
	args, e := plan.arguments("coordinate-guard-probe", false, self, "/fixture", []string{"-test.run=^TestUserNativeCoordinatePrivate$", "-test.timeout=110s", "-test.v"}, env)
	if e != nil {
		t.Fatal(e)
	}
	command := []string{}
	for _, arg := range args {
		if arg != "shell" {
			command = append(command, arg)
		}
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/systemd-run", command...)
	cmd.Env = operation.env
	output, e := cmd.CombinedOutput()
	t.Log(string(output))
	if e != nil || !strings.Contains(string(output), "ACTUAL_COORDINATOR_AGGREGATE_AND_PATH_REFUSAL") {
		t.Fatal("actual coordinator probe failed", e)
	}
	if e := operation.settle(ctx); e != nil {
		t.Fatal(e)
	}
	j, e := json.Marshal(map[string]string{"actualAggregate": operation.group.Path, "deadline": strconv.FormatInt(time.Now().Add(20*time.Second).UnixNano(), 10)})
	if e != nil {
		t.Fatal(e)
	}
	t.Log(string(j))
}

// GGA ON has no PNPM producer; the request must use the legacy installer that
// owns acquisition, installation and its own setup instructions.
func TestUserNativeFreshRequestRoutesGGA(t *testing.T) {
	parent := filepath.Dir(installStagePath(t))
	dest := parent + "/gga-destination"
	profile := DefaultUserExperience()
	profile.GGA, profile.GGAProvider = "on", "codex"
	req := UserInstallRequest{Destination: dest, Mode: "separate", Experience: &profile}
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := profile.Encode()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{dest, "separate", "", "", token, "--experience", encoded}
	_, fresh, err := userNativeFreshRequest(args)
	if err != nil || fresh {
		t.Fatalf("GGA ON did not route to the legacy installer: fresh=%v err=%v", fresh, err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatal("routing inspection changed the destination")
	}
}
