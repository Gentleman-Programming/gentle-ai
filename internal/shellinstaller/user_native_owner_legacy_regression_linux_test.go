//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndependentNativeOwnerLegacy(t *testing.T) {
	for _, name := range []string{"standard-files", "custom-streams", "shared", "existing"} {
		t.Run(name, func(t *testing.T) {
			parent := filepath.Dir(installStagePath(t))
			req := UserInstallRequest{Destination: filepath.Join(parent, "final"), Mode: "separate"}
			if name == "shared" {
				req.Mode = "shared"
			}
			if name == "existing" {
				if err := os.Mkdir(req.Destination, 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := nativeChildFixtureStamp(t, parent)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			token := strings.Repeat("0", 64)
			if name == "standard-files" || name == "custom-streams" {
				var inspectErr error
				token, inspectErr = InspectUserInstall(req)
				if inspectErr != nil {
					t.Fatal(inspectErr)
				}
			}
			args := []string{"install", req.Destination, req.Mode, "", "", token}
			var output bytes.Buffer
			var err error
			if name == "custom-streams" {
				err = RunUserEntry(ctx, "", args, nil, &output, &output)
			} else {
				err = RunUserEntry(ctx, "", args, os.Stdin, os.Stdout, os.Stderr)
			}
			if err == nil || output.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
				t.Fatal("canceled programmatic entry failed preservation", err, output.String())
			}
			t.Logf("LEGACY_COMPAT %s %T %s", name, err, err.Error())
		})
	}
}
