//go:build linux && amd64

package shellinstaller

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserNativeLegacySignature(t *testing.T) {
	parent := t.TempDir()
	for _, n := range []string{"prefix", "agent", "resume"} {
		if e := os.Mkdir(filepath.Join(parent, n), 0700); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(parent+"/resume/installation.json", []byte(`{"schema":0}`), 0600); e != nil {
		t.Fatal(e)
	}
	results := map[string]string{}
	for name, args := range map[string][]string{"shared": {parent + "/fresh", "shared", parent + "/prefix", parent + "/agent", strings.Repeat("0", 64)}, "resume": {parent + "/resume", "separate", "", "", strings.Repeat("0", 64)}} {
		before := nativeChildFixtureStamp(t, parent)
		var out bytes.Buffer
		e := RunUserEntry(context.Background(), "", append([]string{"install"}, args...), nil, &out, &out)
		if e == nil || out.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
			t.Fatal("legacy refusal changed state", name, e, out.String())
		}
		results[name] = strings.ReplaceAll(e.Error(), parent, "<fixture>")
	}
	j, e := json.Marshal(results)
	if e != nil {
		t.Fatal(e)
	}
	t.Log("LEGACY_SIGNATURE", string(j))
}

func TestUserNativeCanceledFreshDispatch(t *testing.T) {
	parent := filepath.Dir(installStagePath(t))
	destination := filepath.Join(parent, "destination")
	req := UserInstallRequest{Destination: destination, Mode: "separate"}
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"install", destination, "separate", "", "", token}
	before := nativeChildFixtureStamp(t, parent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := RunUserEntry(ctx, "", args, nil, &output, &output); err == nil ||
		output.Len() != 0 || before != nativeChildFixtureStamp(t, parent) {
		t.Fatal("canceled fresh request prepared state", err, output.String())
	}
}
