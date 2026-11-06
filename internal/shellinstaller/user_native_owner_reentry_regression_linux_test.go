//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIndependentNativeOwnerReentry(t *testing.T) {
	for _, name := range []string{"malformed", "expired", "extended", "stale", "custom-fallback"} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Dir(installStagePath(t))
			req := UserInstallRequest{Destination: filepath.Join(parent, "final"), Mode: "separate"}
			token, err := InspectUserInstall(req)
			if err != nil {
				t.Fatal(err)
			}
			deadline := strconv.FormatInt(time.Now().Add(time.Minute).UnixNano(), 10)
			switch name {
			case "malformed":
				deadline = "not-a-deadline"
			case "expired":
				deadline = strconv.FormatInt(time.Now().Add(-time.Second).UnixNano(), 10)
			case "extended":
				deadline = strconv.FormatInt(time.Now().Add(141*time.Second).UnixNano(), 10)
			case "stale":
				token = strings.Repeat("0", 64)
			}
			before := nativeChildFixtureStamp(t, parent)
			var out bytes.Buffer
			if name == "custom-fallback" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				self, selfErr := os.Executable()
				if selfErr != nil {
					t.Fatal(selfErr)
				}
				err = RunUserEntryReplacingOwner(ctx, self, []string{"install", req.Destination, "separate", "", "", token}, nil, &out, &out)
			} else {
				err = RunUserEntry(context.Background(), "", []string{"internal-native-own", deadline, req.Destination, "separate", "", "", token}, nil, &out, &out)
			}
			if err == nil || out.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
				t.Fatal("refusal changed state", err, out.String())
			}
		})
	}
}
