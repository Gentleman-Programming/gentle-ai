//go:build windows

package shellinstaller

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// userWindowsOverlayFixture extends the owned launch fixture with a complete
// stock gentle-pi package: source files, the native bundle custody and the
// dependency tree the overlay moves between the packages.
func userWindowsOverlayFixture(t *testing.T) string {
	t.Helper()
	root := userWindowsLaunchFixture(t)
	gentle := filepath.Join(root, "prefix", "node_modules", "gentle-pi")
	if err := os.RemoveAll(gentle); err != nil {
		t.Fatal(err)
	}
	userWindowsOverlayFixtureFile(t, root, "prefix/node_modules/gentle-pi/package.json", "stock package\n")
	userWindowsOverlayFixtureFile(t, root, "prefix/node_modules/gentle-pi/bin/gentle-shell.mjs", "stock shell entry\n")
	userWindowsOverlayFixtureFile(t, root, "prefix/node_modules/gentle-pi/.gentle-ai/v1.0.0/integrity.json", "{}\n")
	userWindowsOverlayFixtureFile(t, root, "prefix/node_modules/gentle-pi/.gentle-ai/v1.0.0/gentle-ai.exe", "stock native binary\n")
	userWindowsOverlayFixtureFile(t, root, "prefix/node_modules/gentle-pi/node_modules/dep/index.js", "stock dependency\n")
	return root
}

// userWindowsOverlayArchives adds the runtime/archives directory every
// published installation carries; the launch fixture omits it on purpose, but
// the overlay retains the Main archive there.
func userWindowsOverlayArchives(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "runtime", "archives"), 0700); err != nil {
		t.Fatal(err)
	}
}

func userWindowsOverlayFixtureFile(t *testing.T, root, relative, body string) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// userWindowsOverlayMainFixture builds a frozen Main snapshot whose source
// files keep the composition the overlay requires: package.json and the shell
// entry stay composition-identical, the SDK source is the Main-only delta.
func userWindowsOverlayMainFixture(t *testing.T, commit string) (userWindowsArtifact, []byte) {
	t.Helper()
	prefix := "gentle-shell-" + commit
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, member := range []struct{ name, body string }{
		{prefix + "/package.json", "main package\n"},
		{prefix + "/bin/gentle-shell.mjs", "main shell entry\n"},
		{prefix + "/sdk/main.mjs", "main sdk source\n"},
	} {
		file, err := writer.Create(member.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(member.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	artifact, err := userWindowsMainArtifact(commit, userWindowsSHA(data))
	if err != nil {
		t.Fatal(err)
	}
	return artifact, data
}

// userWindowsOverlaySeams stubs the overlay's verifier, provisioner and Main
// acquisition. The stubs record their calls; the fixture trees carry no real
// supplier archives and never execute Node.
func userWindowsOverlaySeams(t *testing.T, snapshot *userWindowsArtifact, data *[]byte, verifies *int, provisions *[]string, snapshotCalls *int) {
	t.Helper()
	previousVerify, previousProvision, previousSnapshot := userWindowsOverlayVerify, userWindowsOverlayProvision, userWindowsOverlayMainSnapshot
	userWindowsOverlayVerify = func(ctx context.Context, root string, _, _ io.Writer) error {
		*verifies++
		return nil
	}
	userWindowsOverlayProvision = func(ctx context.Context, root, action string, _, _ io.Writer) error {
		*provisions = append(*provisions, action)
		return nil
	}
	userWindowsOverlayMainSnapshot = func(ctx context.Context) (userWindowsArtifact, []byte, error) {
		*snapshotCalls++
		if snapshot == nil {
			return userWindowsArtifact{}, nil, errors.New("no Main snapshot expected")
		}
		return *snapshot, *data, nil
	}
	t.Cleanup(func() {
		userWindowsOverlayVerify, userWindowsOverlayProvision, userWindowsOverlayMainSnapshot = previousVerify, previousProvision, previousSnapshot
	})
}

func userWindowsOverlayRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func userWindowsOverlayRequest(root, channel string) UserInstallRequest {
	return UserInstallRequest{Destination: root, Mode: "separate", Channel: channel, Experience: &UserExperience{Persona: "gentleman", BackgroundSubagents: "on", SDKSource: channel}}
}

// The stable -> Main overlay swaps only the SDK package and rebinds the
// channel authority; every other component, preference file and binding stays
// byte-identical, and the stock package is retained at the tree root as the
// later Main -> Stable restore source.
func TestUserWindowsChannelOverlayStableToMain(t *testing.T) {
	root := userWindowsOverlayFixture(t)
	userWindowsOverlayArchives(t, root)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	commit := strings.Repeat("a", 40)
	artifact, data := userWindowsOverlayMainFixture(t, commit)
	verifies, snapshotCalls := 0, 0
	var provisions []string
	userWindowsOverlaySeams(t, &artifact, &data, &verifies, &provisions, &snapshotCalls)

	result, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "main"))
	if err != nil || !applied {
		t.Fatalf("stable -> Main overlay failed: applied=%v err=%v", applied, err)
	}
	if result.State != "channel-overlay" || result.Destination != root || result.Prefix != filepath.Join(root, "prefix") {
		t.Fatalf("overlay result differs: %+v", result)
	}
	gentle := filepath.Join(root, "prefix", "node_modules", "gentle-pi")
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "package.json")); got != "main package\n" {
		t.Fatalf("Main SDK package.json differs: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "sdk", "main.mjs")); got != "main sdk source\n" {
		t.Fatalf("Main-only source missing: %q", got)
	}
	// Native and dependency custody moved with the swap.
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, ".gentle-ai", "v1.0.0", "gentle-ai.exe")); got != "stock native binary\n" {
		t.Fatalf("native custody lost in the Main swap: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "node_modules", "dep", "index.js")); got != "stock dependency\n" {
		t.Fatalf("dependency custody lost in the Main swap: %q", got)
	}
	// The retained tree-root stock package is the Main -> Stable restore
	// source: complete except the custody that moved into the Main package.
	stock := filepath.Join(root, "stock-gentle-pi")
	if got := userWindowsOverlayRead(t, filepath.Join(stock, "package.json")); got != "stock package\n" {
		t.Fatalf("retained stock package.json differs: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(stock, "bin", "gentle-shell.mjs")); got != "stock shell entry\n" {
		t.Fatalf("retained stock shell entry differs: %q", got)
	}
	for _, relative := range []string{".gentle-ai", "node_modules"} {
		if _, err := os.Lstat(filepath.Join(stock, relative)); !os.IsNotExist(err) {
			t.Fatalf("retained stock package still holds %s custody: %v", relative, err)
		}
	}
	// Every other component stays byte-identical.
	if got := userWindowsOverlayRead(t, filepath.Join(root, "runtime", "node", "node.exe")); got != "fixture Node, never executed" {
		t.Fatalf("Node runtime was touched: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(root, "agent", "settings.json")); got != "owned agent state" {
		t.Fatalf("agent state was touched: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(root, "source", "package-lock.json")); got != "{\"lockfileVersion\":3}" {
		t.Fatalf("retained lock was touched: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(root, "bin", "pi.cmd")); got != string(userWindowsBinding(root, "pi")) {
		t.Fatalf("command binding was touched: %q", got)
	}
	// Authority rebind: canonical selection with only the channel changed and
	// a manifest that names the fresh Main snapshot.
	selection, err := userWindowsRead(filepath.Join(root, "selection.json"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := userWindowsSelection(selection, root)
	if err != nil || selected.Channel != "main" {
		t.Fatalf("selection was not rebound to Main: %+v %v", selected, err)
	}
	manifestRaw, err := userWindowsRead(filepath.Join(root, "manifest.json"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	var manifest userWindowsManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	canonical, _ := json.Marshal(manifest)
	if string(manifestRaw) != string(canonical)+"\n" {
		t.Fatal("rebound manifest is not canonical with a trailing newline")
	}
	if manifest.SelectionSHA != userWindowsSHA(selection) || manifest.MainCommit != commit || manifest.MainArchiveSHA != artifact.SHA {
		t.Fatalf("manifest authority differs: %+v", manifest)
	}
	mainJSON, err := userWindowsRead(filepath.Join(root, "main.json"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.Marshal(artifact)
	if string(mainJSON) != string(expected) {
		t.Fatal("retained Main descriptor differs from the acquired snapshot")
	}
	mainZIP, err := userWindowsRead(filepath.Join(root, "runtime", "archives", "main.zip"), 32<<20)
	if err != nil || string(mainZIP) != string(data) {
		t.Fatalf("retained Main archive differs: %v", err)
	}
	if verifies != 2 {
		t.Fatalf("overlay ran %d verifications; want the gate and the commit check", verifies)
	}
	if len(provisions) != 1 || provisions[0] != "overlay" {
		t.Fatalf("overlay provisioning differs: %v", provisions)
	}
	if snapshotCalls != 1 {
		t.Fatalf("Main snapshot acquired %d times", snapshotCalls)
	}
	// The rebound tree must satisfy the retained authority readback.
	if _, err := userWindowsManifestRead(root); err != nil {
		t.Fatalf("rebound manifest readback refused: %v", err)
	}
}

// A same-channel reconfigure never enters the overlay: the full
// retire-and-reinstall reconfigure stays responsible for component updates.
func TestUserWindowsChannelOverlaySameChannelStaysFullReconfigure(t *testing.T) {
	root := userWindowsOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	verifies, snapshotCalls := 0, 0
	var provisions []string
	userWindowsOverlaySeams(t, nil, nil, &verifies, &provisions, &snapshotCalls)
	_, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "stable"))
	if err != nil || applied {
		t.Fatalf("same-channel reconfigure took the overlay: applied=%v err=%v", applied, err)
	}
	if verifies != 0 || len(provisions) != 0 || snapshotCalls != 0 {
		t.Fatalf("same-channel reconfigure mutated the tree: verifies=%d provisions=%v snapshots=%d", verifies, provisions, snapshotCalls)
	}
	if _, err := os.Lstat(filepath.Join(root, "stock-gentle-pi")); !os.IsNotExist(err) {
		t.Fatal("same-channel reconfigure created the stock copy")
	}
}

// Main -> Stable restores the retained stock bytes exactly, drops the Main
// authority and quarantine, and leaves a canonical Stable installation.
func TestUserWindowsChannelOverlayMainToStable(t *testing.T) {
	root := userWindowsOverlayFixture(t)
	userWindowsOverlayArchives(t, root)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	commit := strings.Repeat("a", 40)
	artifact, data := userWindowsOverlayMainFixture(t, commit)
	verifies, snapshotCalls := 0, 0
	var provisions []string
	userWindowsOverlaySeams(t, &artifact, &data, &verifies, &provisions, &snapshotCalls)
	if _, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "main")); err != nil || !applied {
		t.Fatalf("stable -> Main setup overlay failed: %v", err)
	}

	_, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "stable"))
	if err != nil || !applied {
		t.Fatalf("Main -> Stable overlay failed: applied=%v err=%v", applied, err)
	}
	gentle := filepath.Join(root, "prefix", "node_modules", "gentle-pi")
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "package.json")); got != "stock package\n" {
		t.Fatalf("restored package.json differs: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "bin", "gentle-shell.mjs")); got != "stock shell entry\n" {
		t.Fatalf("restored shell entry differs: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, ".gentle-ai", "v1.0.0", "gentle-ai.exe")); got != "stock native binary\n" {
		t.Fatalf("restored native custody differs: %q", got)
	}
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "node_modules", "dep", "index.js")); got != "stock dependency\n" {
		t.Fatalf("restored dependency custody differs: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(gentle, "sdk", "main.mjs")); !os.IsNotExist(err) {
		t.Fatal("Main source survived the restore")
	}
	if _, err := os.Lstat(filepath.Join(root, "stock-gentle-pi")); !os.IsNotExist(err) {
		t.Fatal("retained stock copy survived the restore promotion")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "replaced-gentle-pi-") {
			t.Fatalf("Main quarantine survived the committed overlay: %s", entry.Name())
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "main.json")); !os.IsNotExist(err) {
		t.Fatal("Main descriptor survived the Stable restore")
	}
	if _, err := os.Lstat(filepath.Join(root, "runtime", "archives", "main.zip")); !os.IsNotExist(err) {
		t.Fatal("Main archive survived the Stable restore")
	}
	selection, err := userWindowsRead(filepath.Join(root, "selection.json"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := userWindowsSelection(selection, root)
	if err != nil || selected.Channel != "stable" {
		t.Fatalf("selection was not rebound to Stable: %+v %v", selected, err)
	}
	manifestRaw, err := userWindowsRead(filepath.Join(root, "manifest.json"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	var manifest userWindowsManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SelectionSHA != userWindowsSHA(selection) || manifest.MainCommit != "" || manifest.MainArchiveSHA != "" {
		t.Fatalf("Stable manifest authority differs: %+v", manifest)
	}
	if _, err := userWindowsManifestRead(root); err != nil {
		t.Fatalf("restored manifest readback refused: %v", err)
	}
	// Provisioning sequence: the Main overlay, then the pre-rebind
	// composition verify of the restored package. The integrity gate and the
	// commit verification of each overlay run through the tree verifier
	// (verifies), not the provisioner. The Main snapshot is acquired once.
	if len(provisions) != 2 || provisions[0] != "overlay" || provisions[1] != "verify" {
		t.Fatalf("Main -> Stable provisioning differs: %v", provisions)
	}
	if verifies != 4 {
		t.Fatalf("two overlays ran %d tree verifications; want a gate and a commit check each", verifies)
	}
	if snapshotCalls != 1 {
		t.Fatalf("Main -> Stable acquired the snapshot %d times", snapshotCalls)
	}
	// A restored Stable installation can overlay to Main again.
	if _, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "main")); err != nil || !applied {
		t.Fatalf("restored Stable tree refused the next overlay: %v", err)
	}
}

// The complete verification gate runs against the real tree before any byte
// moves: a failing gate refuses the overlay and leaves the installation
// exactly as it was.
func TestUserWindowsChannelOverlayRefusesFailingVerification(t *testing.T) {
	root := userWindowsOverlayFixture(t)
	t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
	commit := strings.Repeat("a", 40)
	artifact, data := userWindowsOverlayMainFixture(t, commit)
	// Only the Main acquisition is stubbed; the verifier keeps its real
	// implementation, which refuses this fixture (no retained archives).
	verifies, snapshotCalls := 0, 0
	var provisions []string
	userWindowsOverlaySeams(t, &artifact, &data, &verifies, &provisions, &snapshotCalls)
	userWindowsOverlayVerify = userWindowsVerify

	before, err := userWindowsRead(filepath.Join(root, "manifest.json"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	_, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "main"))
	if err == nil || !applied {
		t.Fatalf("failing verification did not refuse the overlay: applied=%v err=%v", applied, err)
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("refusal does not name the verification gate: %v", err)
	}
	if snapshotCalls != 0 {
		t.Fatal("failing gate still acquired the Main snapshot")
	}
	gentle := filepath.Join(root, "prefix", "node_modules", "gentle-pi")
	if got := userWindowsOverlayRead(t, filepath.Join(gentle, "package.json")); got != "stock package\n" {
		t.Fatalf("refused overlay mutated the SDK: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(root, "stock-gentle-pi")); !os.IsNotExist(err) {
		t.Fatal("refused overlay created the stock copy")
	}
	if _, err := os.Lstat(filepath.Join(root, "main.json")); !os.IsNotExist(err) {
		t.Fatal("refused overlay wrote the Main descriptor")
	}
	after, err := userWindowsRead(filepath.Join(root, "manifest.json"), 4096)
	if err != nil || string(after) != string(before) {
		t.Fatalf("refused overlay changed the manifest: %v", err)
	}
}

// A failure after the physical swap unwinds every compensated mutation: the
// previous valid Stable installation remains byte-identical. The fixture
// carries runtime/archives, so each failure is injected deliberately: a
// refused main.zip write (after main.json already exists) and a refused owned
// provision (after both Main authority files exist).
func TestUserWindowsChannelOverlayUnwindsAfterProvisionFailure(t *testing.T) {
	for _, tc := range []struct {
		name, failWrite, failAction string
		wantProvisions              int
	}{
		{name: "main.zip write refused", failWrite: "main.zip", wantProvisions: 0},
		{name: "provision refused", failAction: "overlay", wantProvisions: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := userWindowsOverlayFixture(t)
			userWindowsOverlayArchives(t, root)
			t.Setenv("GENTLE_AI_RECONFIGURE_TARGET", root)
			commit := strings.Repeat("a", 40)
			artifact, data := userWindowsOverlayMainFixture(t, commit)
			verifies, snapshotCalls := 0, 0
			var provisions []string
			userWindowsOverlaySeams(t, &artifact, &data, &verifies, &provisions, &snapshotCalls)
			previousWrite := userWindowsOverlayWrite
			t.Cleanup(func() { userWindowsOverlayWrite = previousWrite })
			var written []string
			userWindowsOverlayWrite = func(path string, body []byte) error {
				if tc.failWrite != "" && filepath.Base(path) == tc.failWrite {
					return errors.New("injected exclusive write refusal")
				}
				written = append(written, filepath.Base(path))
				return previousWrite(path, body)
			}
			userWindowsOverlayProvision = func(ctx context.Context, root, action string, _, _ io.Writer) error {
				provisions = append(provisions, action)
				if action == tc.failAction {
					return errors.New("owned provision refused")
				}
				return nil
			}
			manifestBefore := userWindowsOverlayRead(t, filepath.Join(root, "manifest.json"))
			gentle := filepath.Join(root, "prefix", "node_modules", "gentle-pi")

			_, applied, err := userWindowsChannelOverlay(context.Background(), userWindowsOverlayRequest(root, "main"))
			if err == nil || !applied {
				t.Fatalf("injected failure did not fail the overlay: applied=%v err=%v", applied, err)
			}
			if len(written) == 0 || written[0] != "main.json" {
				t.Fatalf("failure was not injected after the Main descriptor write: %v", written)
			}
			if len(provisions) != tc.wantProvisions {
				t.Fatalf("provisioning differs: %v", provisions)
			}
			if got := userWindowsOverlayRead(t, filepath.Join(gentle, "package.json")); got != "stock package\n" {
				t.Fatalf("unwound SDK package.json differs: %q", got)
			}
			if got := userWindowsOverlayRead(t, filepath.Join(gentle, "node_modules", "dep", "index.js")); got != "stock dependency\n" {
				t.Fatalf("unwound dependency custody differs: %q", got)
			}
			if _, err := os.Lstat(filepath.Join(gentle, "sdk", "main.mjs")); !os.IsNotExist(err) {
				t.Fatal("unwound SDK kept Main source")
			}
			if _, err := os.Lstat(filepath.Join(root, "stock-gentle-pi")); !os.IsNotExist(err) {
				t.Fatal("unwound overlay kept the stock copy")
			}
			if _, err := os.Lstat(filepath.Join(root, "main.json")); !os.IsNotExist(err) {
				t.Fatal("unwound overlay kept the Main descriptor")
			}
			if _, err := os.Lstat(filepath.Join(root, "runtime", "archives", "main.zip")); !os.IsNotExist(err) {
				t.Fatal("unwound overlay kept the Main archive")
			}
			if got := userWindowsOverlayRead(t, filepath.Join(root, "manifest.json")); got != manifestBefore {
				t.Fatal("unwound overlay changed the manifest")
			}
			selection, err := userWindowsRead(filepath.Join(root, "selection.json"), 4096)
			if err != nil {
				t.Fatal(err)
			}
			if selected, selectionErr := userWindowsSelection(selection, root); selectionErr != nil || selected.Channel != "stable" {
				t.Fatalf("unwound selection differs: %+v %v", selected, selectionErr)
			}
		})
	}
}
