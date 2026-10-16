//go:build linux

package shellinstaller

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	assets "github.com/gentleman-programming/gentle-ai/v4/scripts"
)

// Exact role captures, not literal presence: missing or duplicate roles fail closed.
func userPinEqual(source []byte, pattern string, want ...string) bool {
	matches := regexp.MustCompile(pattern).FindAllSubmatch(source, 2)
	if len(matches) != 1 || len(matches[0]) != len(want)+1 {
		return false
	}
	for i, value := range want {
		if string(matches[0][i+1]) != value {
			return false
		}
	}
	return true
}

func userPinSource(t *testing.T, path string) []byte {
	t.Helper()
	// The retained Guest harness runs compiled tests outside the package directory.
	_, filename, _, ok := runtime.Caller(0)
	if ok && filepath.IsAbs(filename) {
		path = filepath.Join(filepath.Dir(filename), path)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > 1<<20 {
		t.Fatalf("bounded public source %s: read=%v close=%v bytes=%d", path, readErr, closeErr, len(data))
	}
	return data
}

func TestUserPinParity(t *testing.T) {
	files, err := assets.ReadUserAssets()
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := assets.ReadPrivateHelper("bootstrap-gentle-shell-private-node.sh")
	if err != nil {
		t.Fatal(err)
	}
	python := userPinSource(t, "../../e2e/shell-linux-user-install-guest.py")
	workflow := userPinSource(t, "../../.github/workflows/shell-linux-first-ci.yml")
	// Deliberately exclude the frozen 3.7 job; only the live user laboratory matters.
	job := regexp.MustCompile(`(?ms)^  user-vm-laboratory:\n(.*)\z`).FindSubmatch(workflow)
	if len(job) != 2 {
		t.Fatal("live user-vm-laboratory job absent")
	}
	cases := []struct {
		name    string
		source  []byte
		pattern string
		want    []string
	}{
		{"bootstrap/archive", bootstrap, `(?m)^    Linux\)\n        accepted=([a-f0-9]{64}) bytes=57224421\n        sha256=sha256sum$`, []string{privateColdSHA}},
		{"guest/archive", python, `(?m)^NODE_SHA = '([a-f0-9]{64})'$`, []string{privateColdSHA}},
		{"guest/archive-check", python, `(?m)^                require\(len\(raw\) == 57224421 and hashlib.sha256\(raw\).hexdigest\(\) == NODE_SHA, 'independent fixture Node pin'\)$`, nil},
		{"workflow/user-archive", job[1], `(?m)^          printf '%s  %s\\n' ([a-f0-9]{64}) "\$RUNNER_TEMP/node.data" \| sha256sum -c -$`, []string{privateColdSHA}},
		{"provision/suppliers", files["provision.mjs"], `(?m)^  for \(const \[relative, expected\] of \[\n    \['scripts/gentle-ai-installer.mjs', '([a-f0-9]{64})'\],\n    \['runtime/gentle-ai-binary.mjs', '([a-f0-9]{64})'\],\n  \]\) \{\n    if \(digest\(read\(path.join\(supplier, relative\)\)\) !== expected\) reject\('stock supplier pin'\);$`, []string{userInstallerSHA, privateNativeResolverSHA}},
		{"provision/native", files["provision.mjs"], `(?m)^    const manifest = '([^'\n]+)';\n    if \(binary.length !== 17109176 \|\| digest\(binary\) !== '([a-f0-9]{64})' \|\| !read\(path.join\(version, 'integrity.json'\)\).equals\(Buffer.from\(manifest\)\)\)`, []string{strings.ReplaceAll(userNativeManifest, "\n", `\n`), userBinarySHA}},
		{"guest/native", python, `(?m)^    require\(hashlib.sha256\(native.read_bytes\(\)\).hexdigest\(\) == '([a-f0-9]{64})', 'authenticated native bytes differ'\)$`, []string{userBinarySHA}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !userPinEqual(tc.source, tc.pattern, tc.want...) {
				t.Fatal("live pin role differs from compiled Go authority (or is missing/ambiguous)")
			}
			if len(tc.want) == 0 {
				return
			}
			// Mutate each actual role; an appended dead canonical literal cannot rescue it.
			indices := regexp.MustCompile(tc.pattern).FindSubmatchIndex(tc.source)
			for i, pin := range tc.want {
				mutated := append([]byte(nil), tc.source...)
				mutated[indices[2+2*i]] ^= 1
				mutated = append(mutated, []byte("\n// unused pin: "+pin+"\n")...)
				if userPinEqual(mutated, tc.pattern, tc.want...) || userPinEqual(nil, tc.pattern, tc.want...) {
					t.Fatal("mutated or missing live role accepted")
				}
			}
		})
	}
}

func TestUserFrozenRootPinParity(t *testing.T) {
	files, err := assets.ReadUserAssets()
	if err != nil {
		t.Fatal(err)
	}
	python := userPinSource(t, "../../e2e/shell-linux-user-install-guest.py")
	initial := regexp.MustCompile(`(?ms)^    pins = \{\n(.*?)^    \}\n    run\(\[str\(wrapper\), 'add', '-g',`).FindSubmatch(python)
	if len(initial) != 2 {
		t.Fatal("live pnpm root acquisition tuple map absent")
	}
	// Compiled live locks are root identity authority; do not invent independent Go SRI pins.
	for _, profile := range []string{"modern", "prior"} {
		t.Run(profile, func(t *testing.T) {
			var lock struct {
				Packages map[string]struct {
					Version, Integrity string
					Dependencies       map[string]string
				}
			}
			if err := json.Unmarshal(files["user-locks/"+profile+"/package-lock.json"], &lock); err != nil {
				t.Fatal(err)
			}
			if len(lock.Packages[""].Dependencies) != 5 {
				t.Fatal("live root declarations absent")
			}
			for name, version := range lock.Packages[""].Dependencies {
				entry := lock.Packages["node_modules/"+name]
				source, pattern := initial[1], `(?m)^        '`+regexp.QuoteMeta(name)+`': \('([^']+)', '([^']+)'\),$`
				if profile == "modern" && name == "@earendil-works/pi-coding-agent" {
					source, pattern = python, `(?m)^    pins\['@earendil-works/pi-coding-agent'\] = \('([^']+)', '([^']+)'\)$`
				}
				if entry.Version != version || entry.Integrity == "" || !userPinEqual(source, pattern, version, entry.Integrity) {
					t.Errorf("%s live root tuple/declaration differs from compiled %s lock", name, profile)
				}
			}
		})
	}
}
