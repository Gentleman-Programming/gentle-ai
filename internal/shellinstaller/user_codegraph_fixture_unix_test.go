//go:build linux || darwin

package shellinstaller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

type codeGraphFixtureMember struct {
	name string
	data []byte
}

// codeGraphFixtureArchive builds one npm-shaped archive and the pin that
// authenticates it exactly as the production pins do.
func codeGraphFixtureArchive(t *testing.T, name string, node int64, members []codeGraphFixtureMember) ([]byte, userCodeGraphSource) {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	source := userCodeGraphSource{name: name, node: node}
	for _, member := range members {
		if err := writer.WriteHeader(&tar.Header{Name: member.name, Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len(member.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(member.data); err != nil {
			t.Fatal(err)
		}
		source.unpacked += int64(len(member.data))
		source.entries++
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	hash := sha512.Sum512(data)
	source.size, source.sri = int64(len(data)), "sha512-"+base64.StdEncoding.EncodeToString(hash[:])
	return data, source
}

// codeGraphFixtures replaces the pinned sources and the network producer with
// small archives of the host's real package names for the test's lifetime.
func codeGraphFixtures(t *testing.T) [][]byte {
	t.Helper()
	platform := userCodeGraphSources[1].name
	const node = 4096
	main, mainSource := codeGraphFixtureArchive(t, "codegraph", 0, []codeGraphFixtureMember{
		{"package/package.json", []byte(`{"name":"@colbymchenry/codegraph","version":"1.4.1"}`)},
		{"package/npm-shim.js", []byte("// shim\n")},
	})
	bundle, bundleSource := codeGraphFixtureArchive(t, platform, node, []codeGraphFixtureMember{
		{"package/package.json", []byte(`{"name":"@colbymchenry/` + platform + `","version":"1.4.1"}`)},
		{"package/node", bytes.Repeat([]byte{0x7f}, node)},
		{"package/lib/dist/bin/codegraph.js", []byte("console.log('1.4.1')\n")},
	})
	sources, acquire := userCodeGraphSources, userCodeGraphAcquire
	t.Cleanup(func() { userCodeGraphSources, userCodeGraphAcquire = sources, acquire })
	userCodeGraphSources = []userCodeGraphSource{mainSource, bundleSource}
	return [][]byte{main, bundle}
}

func codeGraphFixtureStage(t *testing.T, profile *UserExperience) string {
	t.Helper()
	root := installStagePath(t)
	if err := userPrepareInstallStage(context.Background(), root, "separate", profile); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

// The qualified host stages the exact pinned library tree, omits the vendored
// Node, retains the authenticated sources and admits it on readback.
func TestUserCodeGraphProvisionStagesFixtureLibrary(t *testing.T) {
	if !userCodeGraphPlatform() {
		t.Skip("CodeGraph library is qualified on Linux amd64 and macOS arm64")
	}
	archives := codeGraphFixtures(t)
	platform := userCodeGraphSources[1].name
	acquired := 0
	userCodeGraphAcquire = func(context.Context) ([][]byte, error) {
		acquired++
		return archives, nil
	}
	profile := &UserExperience{Persona: "gentleman", BackgroundSubagents: "on", StrictTDD: "on", GlobalRDD: "on", Context7: "on", CodeGraph: "on", GGA: "off"}
	root := codeGraphFixtureStage(t, profile)
	ctx := context.Background()
	if err := userCodeGraphProvision(ctx, root, "separate", profile); err != nil {
		t.Fatalf("qualified CodeGraph provisioning refused: %v", err)
	}
	if acquired != 1 {
		t.Fatalf("CodeGraph acquired %d times", acquired)
	}
	library := filepath.Join(root, "runtime/codegraph")
	for name, want := range map[string][]byte{
		"node_modules/@colbymchenry/codegraph/package.json":                     []byte(`{"name":"@colbymchenry/codegraph","version":"1.4.1"}`),
		"node_modules/@colbymchenry/" + platform + "/lib/dist/bin/codegraph.js": []byte("console.log('1.4.1')\n"),
		"sources/codegraph.0.tgz-part":                                          archives[0],
		"sources/" + platform + ".0.tgz-part":                                   archives[1],
	} {
		path := filepath.Join(library, name)
		data, err := os.ReadFile(path)
		info, statErr := os.Lstat(path)
		if err != nil || statErr != nil || !bytes.Equal(data, want) || info.Mode() != 0600 {
			t.Fatalf("staged member %s differs: %v %v %v", name, err, statErr, info)
		}
	}
	if _, err := os.Lstat(filepath.Join(library, "node_modules/@colbymchenry/"+platform+"/node")); !os.IsNotExist(err) {
		t.Fatal("vendored Node was materialized", err)
	}
	if info, err := os.Lstat(filepath.Join(library, "node_modules/@colbymchenry")); err != nil || info.Mode() != os.ModeDir|0700 {
		t.Fatalf("library directory is not private: %v %v", info, err)
	}
	if err := userStageCodeGraphExecutable(ctx, root, root); err != nil {
		t.Fatalf("CodeGraph command refused: %v", err)
	}
	if err := userCodeGraphReadback(ctx, root); err != nil {
		t.Fatalf("staged library failed its retained readback: %v", err)
	}
	if err := userCodeGraphProvision(ctx, root, "separate", profile); err == nil {
		t.Fatal("occupied library was replaced")
	}
	member := filepath.Join(library, "node_modules/@colbymchenry/"+platform+"/lib/dist/bin/codegraph.js")
	if err := os.WriteFile(member, []byte("console.log('tampered')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := userCodeGraphReadback(ctx, root); err == nil {
		t.Fatal("readback admitted a tampered member")
	}
}

// A tampered archive is refused before any library directory exists.
func TestUserCodeGraphProvisionRefusesTamperedArchive(t *testing.T) {
	if !userCodeGraphPlatform() {
		t.Skip("CodeGraph library is qualified on Linux amd64 and macOS arm64")
	}
	archives := codeGraphFixtures(t)
	for which := range archives {
		bad := append([][]byte(nil), archives...)
		bad[which] = bytes.Clone(archives[which])
		bad[which][len(bad[which])/2] ^= 1
		userCodeGraphAcquire = func(context.Context) ([][]byte, error) { return bad, nil }
		profile := &UserExperience{Persona: "unmanaged", BackgroundSubagents: "off", CodeGraph: "on"}
		root := codeGraphFixtureStage(t, profile)
		if err := userCodeGraphProvision(context.Background(), root, "separate", profile); err == nil {
			t.Fatalf("tampered archive %d admitted", which)
		}
		if _, err := os.Lstat(filepath.Join(root, "runtime/codegraph")); !os.IsNotExist(err) {
			t.Fatalf("tampered archive %d materialized a library: %v", which, err)
		}
	}
}
