//go:build linux || darwin

package shellinstaller

import (
	"bytes"
	"encoding/json"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// Independent literals: every POSIX platform pins the same release versions;
// only the platform spelling of each official asset differs.
func TestUnixPinVersionParity(t *testing.T) {
	platform, ok := map[string]struct{ node, release, triple string }{
		"linux":  {"linux-x64", "linux_amd64", "x86_64-unknown-linux-musl"},
		"darwin": {"darwin-arm64", "darwin_arm64", "aarch64-apple-darwin"},
	}[runtime.GOOS]
	if !ok {
		t.Fatalf("no pinned platform for %s", runtime.GOOS)
	}
	hex := regexp.MustCompile(`^[a-f0-9]{64}$`)
	if privateColdURL != "https://nodejs.org"+privateColdPath || privateColdPath != "/dist/v24.18.0/node-v24.18.0-"+platform.node+".tar.gz" {
		t.Fatalf("Node archive pin %q is not the official v24.18.0 %s asset", privateColdURL, platform.node)
	}
	if privateColdSize <= 0 || !hex.MatchString(privateColdSHA) || privateNativeNodeSize <= 0 || !hex.MatchString(privateNativeNodeSHA) {
		t.Fatal("Node archive or member pin shape differs")
	}
	var manifest struct {
		Version, Asset, AssetSha256, BinarySha256 string
	}
	if err := json.Unmarshal([]byte(userNativeManifest), &manifest); err != nil || !strings.HasSuffix(userNativeManifest, "}\n") {
		t.Fatalf("native manifest is not one JSON line: %v", err)
	}
	if manifest.Version != "4.0.0" || manifest.Asset != "gentle-ai_4.0.0_"+platform.release+".tar.gz" || !hex.MatchString(manifest.AssetSha256) || manifest.BinarySha256 != userBinarySHA || !hex.MatchString(userBinarySHA) {
		t.Fatalf("gentle-ai release pin differs: %+v", manifest)
	}
	want := []userToolSource{
		{name: "fd", repo: "sharkdp/fd", tag: "v10.5.0", stem: "fd-v10.5.0-" + platform.triple},
		{name: "rg", repo: "BurntSushi/ripgrep", tag: "15.2.0", stem: "ripgrep-15.2.0-" + platform.triple},
	}
	if len(userToolSources) != len(want) {
		t.Fatalf("tool sources = %d, want %d", len(userToolSources), len(want))
	}
	for i, source := range userToolSources {
		if source.name != want[i].name || source.repo != want[i].repo || source.tag != want[i].tag || source.stem != want[i].stem || source.size <= 0 || !hex.MatchString(source.pin) {
			t.Fatalf("tool source %d = %+v, want release %+v", i, source, want[i])
		}
	}
}

// The bootstrap re-verifies the same archive the Go acquisition accepted.
func TestUnixBootstrapArchivePin(t *testing.T) {
	bootstrap, err := assets.ReadPrivateHelper("bootstrap-gentle-shell-private-node.sh")
	if err != nil {
		t.Fatal(err)
	}
	kernel := map[string]string{"linux": "Linux", "darwin": "Darwin"}[runtime.GOOS]
	matches := regexp.MustCompile(`(?m)^    `+kernel+`\)\n        accepted=([a-f0-9]{64}) bytes=([0-9]+)$`).FindAllSubmatch(bootstrap, -1)
	if len(matches) != 1 || string(matches[0][1]) != privateColdSHA || string(matches[0][2]) != strconv.FormatInt(privateColdSize, 10) {
		t.Fatalf("bootstrap %s archive pin differs from the compiled Node pin", kernel)
	}
	for _, check := range []string{
		"    test \"$(wc -c < \"$1\" | tr -d ' ')\" = \"$bytes\" || fail 'archive size differs'\n",
		"    test \"$($sha256 \"$1\" | cut -d ' ' -f1)\" = \"$accepted\" || fail 'archive hash differs'\n",
	} {
		if bytes.Count(bootstrap, []byte(check)) != 1 {
			t.Fatalf("bootstrap archive check absent: %q", check)
		}
	}
}
