//go:build darwin

package shellinstaller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// darwinPrivateParent returns an owned 0700 parent spelled through the /var
// system link, so every selection below needs canonicalization.
func darwinPrivateParent(t *testing.T) (raw, canonical string) {
	t.Helper()
	raw = t.TempDir()
	if err := os.Chmod(raw, 0700); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	if canonical == raw {
		t.Skip("TMPDIR is already canonical; this control needs a /var or /tmp spelling")
	}
	return raw, canonical
}

func darwinEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestDarwinSeparateSelectionCanonicalizesOnce(t *testing.T) {
	raw, canonical := darwinPrivateParent(t)
	rawReq := UserInstallRequest{Destination: filepath.Join(raw, "shell"), Mode: "separate"}
	canonicalReq := UserInstallRequest{Destination: filepath.Join(canonical, "shell"), Mode: "separate"}
	for _, req := range []UserInstallRequest{rawReq, canonicalReq} {
		if err := ValidateUserInstall(req); err != nil {
			t.Fatalf("Separate selection %q refused: %v", req.Destination, err)
		}
	}
	rawToken, rawErr := InspectUserInstall(rawReq)
	canonicalToken, canonicalErr := InspectUserInstall(canonicalReq)
	if rawErr != nil || canonicalErr != nil || rawToken == "" || rawToken != canonicalToken {
		t.Fatalf("raw and canonical spellings must bind one physical selection: %q %v / %q %v", rawToken, rawErr, canonicalToken, canonicalErr)
	}
	if names := darwinEntries(t, canonical); len(names) != 0 {
		t.Fatalf("validation or inspection had effects: %v", names)
	}
}

func TestDarwinSharedAndRecoverRefuseBeforeEffects(t *testing.T) {
	_, parent := darwinPrivateParent(t)
	req := UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "shared",
		SharedPrefix: filepath.Join(parent, "prefix"), SharedAgent: filepath.Join(parent, "agent")}
	bundle := filepath.Join(req.SharedPrefix, "lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js")
	for _, dir := range []string{filepath.Dir(bundle), req.SharedAgent} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(bundle, []byte("stock"), 0600); err != nil {
		t.Fatal(err)
	}
	before := strings.Join(darwinEntries(t, parent), ",")
	const shared = "Shared mode is not yet available on macOS"
	_, inspectErr := InspectUserInstall(req)
	_, previewErr := PreviewUserInstall(req, "")
	_, runErr := RunUserInstall(context.Background(), req)
	entryErr := RunUserEntry(context.Background(), "", []string{"install", req.Destination, req.Mode, req.SharedPrefix, req.SharedAgent, "f"}, nil, nil, nil)
	for name, err := range map[string]error{"validate": ValidateUserInstall(req), "inspect": inspectErr, "preview": previewErr, "run": runErr, "entry": entryErr} {
		if err == nil || !strings.Contains(err.Error(), shared) || !strings.Contains(err.Error(), "gentle-ai shell install --mode separate") {
			t.Fatalf("%s: Shared must refuse with the precise macOS message: %v", name, err)
		}
	}
	err := RunUserEntry(context.Background(), "", []string{"recover", req.Destination, "inspect"}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "shell recover is not yet available on macOS") || !strings.Contains(err.Error(), "gentle-ai shell --help") {
		t.Fatalf("recover must refuse with the precise macOS message: %v", err)
	}
	if after := strings.Join(darwinEntries(t, parent), ","); after != before {
		t.Fatalf("refused Shared or recover changed the parent: %q -> %q", before, after)
	}
}

func TestDarwinEntryPointsFollowKernelGate(t *testing.T) {
	if reflect.ValueOf(userKernelGate).Pointer() != reflect.ValueOf(userDarwinKernelCheck).Pointer() {
		t.Fatal("production kernel gate is not userDarwinKernelCheck")
	}
	_, parent := darwinPrivateParent(t)
	req := UserInstallRequest{Destination: filepath.Join(parent, "shell"), Mode: "separate"}
	token, err := InspectUserInstall(req)
	if err != nil {
		t.Fatal(err)
	}
	req.Confirmation = token
	refused := errors.New("injected kernel refusal")
	original := userKernelGate
	t.Cleanup(func() { userKernelGate = original })
	userKernelGate = func() error { return refused }
	if _, err := RunUserInstall(context.Background(), req); !errors.Is(err, refused) {
		t.Fatalf("installation ran past a refused kernel: %v", err)
	}
	for _, verb := range []string{"check", "internal-check", "install", "launch", "recover", "unknown"} {
		if err := RunUserEntry(context.Background(), "", []string{verb, req.Destination, req.Mode, "", "", token}, nil, nil, nil); !errors.Is(err, refused) {
			t.Fatalf("%s ran past a refused kernel: %v", verb, err)
		}
	}
	if names := darwinEntries(t, parent); len(names) != 0 {
		t.Fatalf("refused kernel left effects: %v", names)
	}
	userKernelGate = func() error { return nil }
	for _, verb := range []string{"check", "internal-check"} {
		if err := RunUserEntry(context.Background(), "", []string{verb}, nil, nil, nil); err != nil {
			t.Fatalf("%s refused on a qualified kernel: %v", verb, err)
		}
	}
	if err := RunUserEntry(context.Background(), "", []string{"unknown"}, nil, nil, nil); err == nil {
		t.Fatal("unknown entry accepted")
	}
	if err := RunUserEntry(context.Background(), "", nil, nil, nil, nil); err == nil {
		t.Fatal("empty entry accepted")
	}
}

func TestDarwinStagePathAcceptsOnlyTheExactReport(t *testing.T) {
	const destination = "/private/tmp/owned/runtime/node"
	const stage = "/private/tmp/owned/runtime/.gentle-node-stage.Ab3dE9gZ/node"
	if got, err := userStagePath(userStageReport+stage+"\n", destination); err != nil || got != stage {
		t.Fatalf("exact report refused: %q %v", got, err)
	}
	for name, output := range map[string]string{
		"empty":            "",
		"unterminated":     userStageReport + stage,
		"two reports":      userStageReport + stage + "\n" + userStageReport + stage + "\n",
		"blank tail":       userStageReport + stage + "\n\n",
		"crlf":             userStageReport + stage + "\r\n",
		"leading output":   "x" + userStageReport + stage + "\n",
		"published report": "Node bootstrap only: Node=24.18.0 npm=11.16.0 package-install=not-run launch=not-run Ready=false\n",
		"short suffix":     userStageReport + "/private/tmp/owned/runtime/.gentle-node-stage.Ab3dE9g/node\n",
		"long suffix":      userStageReport + "/private/tmp/owned/runtime/.gentle-node-stage.Ab3dE9gZZ/node\n",
		"symbol suffix":    userStageReport + "/private/tmp/owned/runtime/.gentle-node-stage.Ab3dE9g-/node\n",
		"foreign parent":   userStageReport + "/private/tmp/other/runtime/.gentle-node-stage.Ab3dE9gZ/node\n",
		"traversal":        userStageReport + "/private/tmp/owned/runtime/../runtime/.gentle-node-stage.Ab3dE9gZ/node\n",
		"double slash":     userStageReport + "/private/tmp/owned/runtime//.gentle-node-stage.Ab3dE9gZ/node\n",
		"other leaf":       userStageReport + "/private/tmp/owned/runtime/.gentle-node-stage.Ab3dE9gZ/nodes\n",
		"trailing slash":   userStageReport + stage + "/\n",
		"nested":           userStageReport + stage + "/node\n",
		"other prefix":     userStageReport + "/private/tmp/owned/runtime/.gentle-user-stage.Ab3dE9gZ/node\n",
		"relative":         userStageReport + "runtime/.gentle-node-stage.Ab3dE9gZ/node\n",
	} {
		if got, err := userStagePath(output, destination); err == nil {
			t.Fatalf("%s: report accepted as %q", name, got)
		}
	}
}

type darwinStage struct {
	runtime, container, stage, destination, report, nodeSHA string
	nodeSize                                                int64
}

// darwinStageFixture writes a bootstrap-shaped stage whose inventory lists
// every file, with a test Node pin standing in for the real binary.
func darwinStageFixture(t *testing.T, mutate func(stage string, listing map[string]string)) darwinStage {
	t.Helper()
	_, parent := darwinPrivateParent(t)
	fixture := darwinStage{runtime: filepath.Join(parent, "runtime")}
	fixture.container = filepath.Join(fixture.runtime, userStagePrefix+"Ab3dE9gZ")
	fixture.stage = filepath.Join(fixture.container, "node")
	fixture.destination = filepath.Join(fixture.runtime, "node")
	fixture.report = userStageReport + fixture.stage + "\n"
	files := map[string]string{"bin/node": "#!/bin/sh\nfixture node\n", "lib/node_modules/npm/bin/npm-cli.js": "npm\n", "BOOTSTRAP-PROVENANCE": userStageProvenance}
	for relative, data := range files {
		path := filepath.Join(fixture.stage, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.nodeSize = int64(len(files["bin/node"]))
	fixture.nodeSHA = fmt.Sprintf("%x", sha256.Sum256([]byte(files["bin/node"])))
	listing := map[string]string{}
	for relative, data := range files {
		listing[relative] = fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	}
	if mutate != nil {
		mutate(fixture.stage, listing)
	}
	var lines []string
	for relative, digest := range listing {
		lines = append(lines, digest+"  ./"+relative)
	}
	sort.Strings(lines)
	if err := os.WriteFile(filepath.Join(fixture.stage, userStageInventory), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestDarwinAdoptStagePublishesOnlyTheVerifiedStage(t *testing.T) {
	fixture := darwinStageFixture(t, nil)
	before, err := os.Stat(fixture.stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := userAdoptStage(context.Background(), fixture.report, fixture.destination, fixture.nodeSize, fixture.nodeSHA); err != nil {
		t.Fatalf("verified stage refused: %v", err)
	}
	after, err := os.Stat(fixture.destination)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("destination is not the verified stage: %v", err)
	}
	if names := darwinEntries(t, fixture.runtime); len(names) != 1 || names[0] != "node" {
		t.Fatalf("empty stage container not removed: %v", names)
	}
}

func TestDarwinAdoptStageRefusesForeignOrChangedStages(t *testing.T) {
	write := func(path, data string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
	}
	for name, tc := range map[string]struct {
		mutate func(stage string, listing map[string]string)
		after  func(fixture darwinStage)
		pin    string
		kind   string
	}{
		"changed file": {after: func(f darwinStage) {
			write(filepath.Join(f.stage, "lib/node_modules/npm/bin/npm-cli.js"), "evil\n", 0700)
		}, kind: "source"},
		"unlisted file": {after: func(f darwinStage) { write(filepath.Join(f.stage, "lib/extra.js"), "x", 0600) }, kind: "source"},
		"missing file": {mutate: func(_ string, listing map[string]string) {
			listing["lib/absent.js"] = strings.Repeat("0", 64)
		}, kind: "source"},
		"symlink": {after: func(f darwinStage) {
			if err := os.Symlink("bin/node", filepath.Join(f.stage, "link")); err != nil {
				t.Fatal(err)
			}
		}, kind: "source"},
		"node pin": {pin: strings.Repeat("a", 64), kind: "source"},
		"provenance": {after: func(f darwinStage) {
			write(filepath.Join(f.stage, "BOOTSTRAP-PROVENANCE"), "accepted-archive-sha256=0\n", 0700)
		}, kind: "source"},
		"malformed listing": {mutate: func(_ string, listing map[string]string) {
			listing["../escape"] = strings.Repeat("0", 64)
		}, kind: "source"},
		"open container": {after: func(f darwinStage) {
			if err := os.Chmod(f.container, 0755); err != nil {
				t.Fatal(err)
			}
		}, kind: "filesystem"},
		"container sibling": {after: func(f darwinStage) { write(filepath.Join(f.container, "diagnostic"), "", 0600) }, kind: "preimage"},
		"second stage": {after: func(f darwinStage) {
			if err := os.Mkdir(filepath.Join(f.runtime, userStagePrefix+"Zz9yX8wV"), 0700); err != nil {
				t.Fatal(err)
			}
		}, kind: "preimage"},
		"occupied destination": {after: func(f darwinStage) {
			if err := os.Mkdir(f.destination, 0700); err != nil {
				t.Fatal(err)
			}
		}, kind: "preimage"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := darwinStageFixture(t, tc.mutate)
			if tc.after != nil {
				tc.after(fixture)
			}
			pin := fixture.nodeSHA
			if tc.pin != "" {
				pin = tc.pin
			}
			_, destinationBefore := os.Lstat(fixture.destination)
			err := userAdoptStage(context.Background(), fixture.report, fixture.destination, fixture.nodeSize, pin)
			var failure *PrivateRuntimeError
			if !errors.As(err, &failure) || failure.Kind != tc.kind {
				t.Fatalf("refusal kind = %v, want %s", err, tc.kind)
			}
			if _, statErr := os.Lstat(filepath.Join(fixture.stage, "bin/node")); statErr != nil {
				t.Fatalf("refused stage was not kept intact: %v", statErr)
			}
			if _, destinationAfter := os.Lstat(fixture.destination); (destinationBefore == nil) != (destinationAfter == nil) {
				t.Fatal("refusal changed the destination")
			}
		})
	}
}

func TestDarwinPublishErrorsAreClassified(t *testing.T) {
	uncertain := privateError("uncertain", errors.New("sync"))
	for err, want := range map[error]string{syscall.EEXIST: "preimage", syscall.ENOTSUP: "filesystem", syscall.EXDEV: "filesystem", uncertain: "uncertain"} {
		var failure *PrivateRuntimeError
		if got := userPublishError(err); !errors.As(got, &failure) || failure.Kind != want || !errors.Is(got, err) {
			t.Fatalf("%v classified as %v, want %s", err, got, want)
		}
	}
	// The publication itself is RENAME_EXCL: an existing destination survives.
	fixture := darwinStageFixture(t, nil)
	if err := os.Mkdir(fixture.destination, 0700); err != nil {
		t.Fatal(err)
	}
	if err := userPublishError(userPublishStage(fixture.stage, fixture.destination)); !errors.Is(err, syscall.EEXIST) {
		t.Fatalf("publication over an existing destination = %v, want EEXIST", err)
	}
	if names := darwinEntries(t, fixture.destination); len(names) != 0 {
		t.Fatalf("publication entered the existing destination: %v", names)
	}
}

func TestDarwinOwnedRunSupervisesWithLimitsAndBoundedOutput(t *testing.T) {
	dir := t.TempDir()
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	run := func(timeout time.Duration, env []string, script string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
		cmd.Dir, cmd.Env = dir, env
		return userOwnedRun(ctx, cmd, cancel)
	}
	output, err := run(30*time.Second, []string{"PATH=/usr/bin:/bin"}, "pwd -P; ulimit -f; umask; ulimit -t; echo $$ $(ps -o pgid= -p $$)")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if err != nil || len(lines) != 5 || lines[0] != physical || lines[1] != "4194304" || lines[2] != "0077" || lines[3] == "unlimited" {
		t.Fatalf("owned command missed its directory or limits: %q %v", output, err)
	}
	if ids := strings.Fields(lines[4]); len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("owned command is not its own process group leader: %q", lines[4])
	}
	kind := func(err error) string {
		var failure *PrivateRuntimeError
		if errors.As(err, &failure) {
			return failure.Kind
		}
		return fmt.Sprint(err)
	}
	if output, err := run(30*time.Second, nil, "echo partial; exit 3"); kind(err) != "nonzero" || output != "partial\n" {
		t.Fatalf("nonzero exit = %q %v", output, err)
	}
	if _, err := run(30*time.Second, nil, "head -c 5000 /dev/zero | tr '\\0' x; sleep 30"); kind(err) != "output" {
		t.Fatalf("unbounded output = %v", err)
	}
	if _, err := run(300*time.Millisecond, nil, "sleep 30"); kind(err) != "deadline" {
		t.Fatalf("deadline = %v", err)
	}
	marker := filepath.Join(dir, "ran")
	if _, err := run(30*time.Second, []string{"BASH_ENV=/dev/null"}, "touch "+marker); kind(err) != "refused" {
		t.Fatalf("limits-altering environment = %v", err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("refused command ran: %v", err)
	}
}

func TestDarwinLaunchLimitsExecInPlaceWithoutSecondGroup(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "ulimit -f; umask; ulimit -t; echo $$ $(ps -o pgid= -p $$)")
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	limited, err := userLaunchLimits(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != userLimitsShell || len(cmd.Args) != 7 || cmd.Args[3] != userLimitsName || cmd.Args[4] != "/bin/sh" || cmd.Args[5] != "-c" {
		t.Fatalf("launch is not wrapped by the limits shell: %q %q", cmd.Path, cmd.Args)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := userLaunchGroup(cmd, nil, limited); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 4 || lines[0] != "4194304" || lines[1] != "0077" || lines[2] != "unlimited" {
		t.Fatalf("launched command missed its limits: %q", stdout.String())
	}
	if ids := strings.Fields(lines[3]); len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("exec in place lost the launch group: %q", lines[3])
	}
	refused := exec.Command("/usr/bin/true")
	refused.Env = []string{"BASH_FUNC_ulimit%%=() { :; }"}
	if _, err := userLaunchLimits(refused); err == nil || refused.Path != "/usr/bin/true" || refused.ExtraFiles != nil {
		t.Fatalf("limits-altering launch environment accepted: %v", err)
	}
}

// A launched Pi that exits 125 keeps its status; a wrapper that stops before
// attesting its limits (here its chain is replaced by the bare refusal exit)
// is a precise refusal.
func TestDarwinLaunchLimitsTellRefusalFromExit125(t *testing.T) {
	run := func(refuse bool) error {
		cmd := exec.CommandContext(context.Background(), "/bin/sh", "-c", "exit 125")
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		limited, err := userLaunchLimits(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if refuse {
			cmd.Args[2] = "exit 125"
		}
		return userLaunchGroup(cmd, nil, limited)
	}
	var exit *exec.ExitError
	var failure *PrivateRuntimeError
	if err := run(false); !errors.As(err, &exit) || exit.ExitCode() != 125 || errors.As(err, &failure) {
		t.Fatalf("launched exit 125 = %v, want the plain command status", err)
	}
	if err := run(true); !errors.As(err, &failure) || failure.Kind != "refused" {
		t.Fatalf("unattested wrapper exit 125 = %v, want a refusal", err)
	}
}

func TestDarwinExecutableAndTrustedTmpArePhysical(t *testing.T) {
	self, err := userExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if physical, err := filepath.EvalSymlinks(self); err != nil || physical != self {
		t.Fatalf("supervisor path %q is not physical: %q %v", self, physical, err)
	}
	dir, err := os.MkdirTemp(userTrustedTmp, "gentle-supervisor-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	supervisor := filepath.Join(dir, "gentle-ai")
	if err := os.WriteFile(supervisor, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := userSupervisorSHA(context.Background(), supervisor); err != nil {
		t.Fatalf("supervisor below the root sticky %s refused: %v", userTrustedTmp, err)
	}
}

// The darwin system pool delegates to the platform verifier and lists no
// subjects; acquisition must still get a verifying client, never an insecure one.
func TestDarwinColdClientUsesPlatformRoots(t *testing.T) {
	client, err := privateColdClient()
	if err != nil {
		t.Fatalf("cold client refused on darwin: %v", err)
	}
	defer client.CloseIdleConnections()
	config := client.Transport.(*http.Transport).TLSClientConfig
	if config == nil || config.InsecureSkipVerify || config.RootCAs == nil || config.MinVersion != tls.VersionTLS12 || config.ServerName != "nodejs.org" {
		t.Fatalf("cold client TLS configuration weakened: %+v", config)
	}
}

// /dev/null has no terminal ioctls: darwin answers ENODEV where Linux answers
// ENOTTY, and both must mean "not a terminal" to the launch path.
func TestDarwinNullStdinIsNotATerminal(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if _, err := userTermios(int(null.Fd())); !errors.Is(err, syscall.ENOTTY) {
		t.Fatalf("termios on %s = %v, want ENOTTY", os.DevNull, err)
	}
	cmd := exec.CommandContext(context.Background(), "/usr/bin/true")
	restore, err := userForeground(cmd, null)
	if err != nil || restore() != nil || cmd.SysProcAttr.Foreground {
		t.Fatalf("null stdin was treated as a terminal: %v", err)
	}
}
