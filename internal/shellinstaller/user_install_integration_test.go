package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The provision scripts stay the source of truth for the network-bound npm
// budget (NPM_NETWORK_TIMEOUT). The shellinstaller deadline bounds mirror
// that value; this parser keeps the mirror honest without duplicating drift.
func userScriptsNpmNetworkTimeout(t *testing.T) time.Duration {
	t.Helper()
	pattern := regexp.MustCompile(`NPM_NETWORK_TIMEOUT = (\d+);`)
	timeout := time.Duration(0)
	for _, script := range []string{
		"scripts/provision-gentle-shell-windows.mjs",
		"scripts/provision-gentle-shell-private-global.mjs",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", script))
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		match := pattern.FindSubmatch(data)
		if match == nil {
			t.Fatalf("NPM_NETWORK_TIMEOUT not declared in %s", script)
		}
		ms, err := strconv.ParseInt(string(match[1]), 10, 64)
		if err != nil {
			t.Fatalf("parse NPM_NETWORK_TIMEOUT in %s: %v", script, err)
		}
		if timeout == 0 {
			timeout = time.Duration(ms) * time.Millisecond
		} else if time.Duration(ms)*time.Millisecond != timeout {
			t.Fatalf("NPM_NETWORK_TIMEOUT diverges between provision scripts: %s vs %s", script, "previous")
		}
	}
	return timeout
}

// Both provision scripts must keep declaring the same network-bound npm
// budget; a divergence means one install channel can hang twice as long as
// the other before any external deadline fires.
func TestProvisionScriptsDeclareSharedNpmNetworkTimeout(t *testing.T) {
	if userScriptsNpmNetworkTimeout(t) <= 0 {
		t.Fatal("provision scripts declare no positive NPM_NETWORK_TIMEOUT")
	}
}

// Linux and Windows share one package; the internal entry must never accept
// a selector that the running platform would silently drop.
func TestUserInstallEntryChannelMatchesPlatform(t *testing.T) {
	base := []string{"/owned/shell", "separate", "", "", "confirmed"}
	req, err := UserInstallFromEntry(base)
	if err != nil || req.Destination != base[0] || req.Mode != base[1] || req.SharedPrefix != base[2] || req.SharedAgent != base[3] || req.Confirmation != base[4] {
		t.Fatalf("five-value entry = %+v, %v", req, err)
	}
	for _, channel := range []string{"stable", "main"} {
		req, err := UserInstallFromEntry(append(append([]string(nil), base...), "--channel", channel))
		if runtime.GOOS == "windows" {
			if err != nil || req.Destination != base[0] {
				t.Fatalf("Windows refused --channel %s: %+v, %v", channel, req, err)
			}
			continue
		}
		if err == nil || req != (UserInstallRequest{}) {
			t.Fatalf("non-Windows entry ignored --channel %s: %+v, %v", channel, req, err)
		}
	}
	for _, args := range [][]string{base[:4], append(append([]string(nil), base...), "--channel"), append(append([]string(nil), base...), "--unknown", "main")} {
		if req, err := UserInstallFromEntry(args); err == nil || req != (UserInstallRequest{}) {
			t.Fatalf("malformed entry %q returned %+v, %v", args, req, err)
		}
	}
}

func TestUserConfirmationKeepsLinuxFiveValueBinding(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows binds the channel as a sixth value; see user_channels_windows_test.go")
	}
	req := UserInstallRequest{Destination: "/owned/shell", Mode: "shared", SharedPrefix: "/owned/pi", SharedAgent: "/owned/agent"}
	data, _ := json.Marshal([]string{req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, "identity"})
	if got, want := userConfirmation(req, "identity"), fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
		t.Fatalf("Linux confirmation changed: %s, want %s", got, want)
	}
}

func TestUserInstallUnsupportedPlatformRefusesCommonAPI(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("platform has a native backend")
	}
	check := UserKernelCheck()
	if check == nil || !strings.Contains(check.Error(), "Linux amd64") || !strings.Contains(check.Error(), "macOS 14+ arm64") || !strings.Contains(check.Error(), "Windows 11 x64") {
		t.Fatalf("unsupported platform refusal is inaccurate: %v", check)
	}
	req := UserInstallRequest{Destination: t.TempDir() + "/shell", Mode: "separate"}
	token, inspectErr := InspectUserInstall(req)
	result, runErr := RunUserInstall(context.Background(), req)
	entryErr := RunUserEntry(context.Background(), "", []string{"install"}, nil, io.Discard, io.Discard)
	for _, err := range []error{inspectErr, runErr, entryErr} {
		if err == nil || err.Error() != check.Error() {
			t.Fatalf("common API bypassed the platform refusal: %v", err)
		}
	}
	if token != "" || result != (UserInstallResult{}) {
		t.Fatalf("unsupported platform returned authority: %q %+v", token, result)
	}
}
