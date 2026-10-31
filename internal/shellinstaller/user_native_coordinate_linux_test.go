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

func TestUserNativeFreshSelection(t *testing.T) {
	parent := filepath.Dir(installStagePath(t))
	destination := filepath.Join(parent, "destination")
	req := UserInstallRequest{Destination: destination, Mode: "separate"}
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{destination, "separate", "", "", token}
	before, err := userTreeStamp(parent)
	if err != nil {
		t.Fatal(err)
	}
	selected, fresh, err := userNativeFreshRequest(args)
	if err != nil || !fresh || selected.Destination != destination || selected.Confirmation != token {
		t.Fatal("confirmed fresh selection not routed", selected, fresh, err)
	}
	for _, confirmation := range []string{"", strings.Repeat("0", 64)} {
		changed := append([]string(nil), args...)
		changed[4] = confirmation
		if _, fresh, err := userNativeFreshRequest(changed); !fresh || err == nil {
			t.Fatal("missing or stale confirmation accepted", fresh, err)
		}
		var output bytes.Buffer
		if err := RunUserEntry(context.Background(), "", append([]string{"install"}, changed...), nil, &output, &output); err == nil || output.Len() != 0 {
			t.Fatal("public refusal reported success", err, output.String())
		}
	}
	if after, err := userTreeStamp(parent); err != nil || before != after {
		t.Fatal("selection or refusal prepared files", err)
	}
	shared := append([]string(nil), args...)
	shared[1] = "shared"
	if _, fresh, err := userNativeFreshRequest(nil); fresh || err == nil {
		t.Fatal("malformed empty selection accepted", fresh, err)
	}
	for _, unchanged := range [][]string{shared} {
		if _, fresh, err := userNativeFreshRequest(unchanged); fresh || err != nil {
			t.Fatal("legacy path intercepted", fresh, err)
		}
	}
	if after, err := userTreeStamp(parent); err != nil || before != after {
		t.Fatal("malformed or legacy selection prepared files", err)
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if _, fresh, err := userNativeFreshRequest(args); fresh || err != nil {
		t.Fatal("existing target intercepted instead of legacy resume", fresh, err)
	}
}

func TestUserNativeCoordinateRefusal(t *testing.T) {
	var output bytes.Buffer
	if err := userNativeCoordinate(context.Background(), "", nil, nil, &output, &output); err == nil || !strings.Contains(err.Error(), "invalid native coordinator arguments") {
		t.Fatal("missing coordinator arguments accepted", err)
	}
	kernelErr := UserKernelCheck()
	err := RunUserEntry(context.Background(), "", []string{"internal-native-coordinate"}, nil, &output, &output)
	if kernelErr != nil {
		if err == nil || err.Error() != kernelErr.Error() || output.Len() != 0 {
			t.Fatal("unqualified coordinator selector bypassed kernel admission", err, kernelErr, output.String())
		}
	} else if err == nil || !strings.Contains(err.Error(), "invalid native coordinator arguments") {
		t.Fatal("coordinator selector not dispatched", err)
	}
	if output.Len() != 0 {
		t.Fatal("refused coordinator reported success", output.String())
	}
}
