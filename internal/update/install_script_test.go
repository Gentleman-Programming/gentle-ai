package update

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWindowsInstallScriptHasNoUTF8BOM(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("scripts/install.ps1 starts with UTF-8 BOM; PowerShell irm | iex treats BOM+#Requires as an invalid command")
	}
}

func TestWindowsInstallScriptIsASCIIOnly(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	for i, b := range content {
		if b >= 0x80 {
			line := 1 + bytes.Count(content[:i], []byte("\n"))
			t.Fatalf("scripts/install.ps1 contains non-ASCII byte 0x%X at byte offset %d, line %d; Windows PowerShell 5.1 can misdecode UTF-8 without BOM when running powershell -File", b, i, line)
		}
	}
}

// TestWindowsInstallScriptHasNoUnsafeStringSubexpression guards against the
// PowerShell 5.1 parser failure reported in issue #849. Patterns like
// "($fileSize bytes)" inside a double-quoted string are read by Windows
// PowerShell 5.1 as an invalid subexpression and abort parsing before any code
// runs. Use the -f format operator instead, e.g. ("... {0} bytes" -f $fileSize).
func TestWindowsInstallScriptHasNoUnsafeStringSubexpression(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	// Match double-quoted strings, then flag any "($identifier <word>" inside
	// them. Scoping to quoted strings avoids false positives on real code such
	// as `foreach ($loc in $locations)`.
	stringLiteral := regexp.MustCompile(`"[^"]*"`)
	unsafeSubexpr := regexp.MustCompile(`\(\$[A-Za-z_][A-Za-z0-9_]*\s+[A-Za-z]`)

	for _, line := range bytes.Split(content, []byte("\n")) {
		for _, str := range stringLiteral.FindAll(line, -1) {
			if unsafeSubexpr.Match(str) {
				t.Errorf("scripts/install.ps1 contains an unsafe ($var word) string subexpression that breaks PowerShell 5.1 parsing: %s\nUse the -f format operator instead.", str)
			}
		}
	}
}

func TestInstallScriptBetaGoInstallBypassesPublicGoProxy(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)
	for _, want := range []string{
		"prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v2",
		"prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v2",
		"prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v2",
		"go install \"$go_package\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.sh is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai/v2 \\",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai/v2 \\",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai/v2 \\",
	} {
		if strings.Contains(script, clobber) {
			t.Fatalf("scripts/install.sh clobbers existing user env with %q; beta proxy bypass must preserve existing patterns", clobber)
		}
	}

	start := strings.Index(script, "prepend_go_env_pattern() {")
	if start == -1 {
		t.Fatal("scripts/install.sh is missing prepend_go_env_pattern function")
	}
	endMarker := "\n}\n\n# ============================================================================\n# Install via binary download"
	end := strings.Index(script[start:], endMarker)
	if end == -1 {
		t.Fatal("could not locate end of prepend_go_env_pattern function")
	}
	function := script[start : start+end+3]

	cmd := exec.Command("bash", "-c", function+`
GONOSUMDB=example.com/private
GOPRIVATE=github.com/acme/*
GONOPROXY=github.com/gentleman-programming/gentle-ai/v2
prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v2
prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v2
prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v2
printf '%s\n%s\n%s\n' "$GONOSUMDB" "$GOPRIVATE" "$GONOPROXY"
`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run prepend_go_env_pattern fixture: %v\noutput: %s", err, out)
	}

	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"github.com/gentleman-programming/gentle-ai/v2,example.com/private",
		"github.com/gentleman-programming/gentle-ai/v2,github.com/acme/*",
		"github.com/gentleman-programming/gentle-ai/v2",
	}, "\n")
	if got != want {
		t.Fatalf("prepend_go_env_pattern output = %q, want %q", got, want)
	}
}

func TestWindowsInstallScriptBetaGoInstallPreservesGoProxyBypassEnv(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.ps1")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	script := string(content)
	for _, want := range []string{
		"Add-GoEnvPattern -Name \"GONOSUMDB\" -Pattern \"github.com/gentleman-programming/gentle-ai/v2\"",
		"Add-GoEnvPattern -Name \"GOPRIVATE\" -Pattern \"github.com/gentleman-programming/gentle-ai/v2\"",
		"Add-GoEnvPattern -Name \"GONOPROXY\" -Pattern \"github.com/gentleman-programming/gentle-ai/v2\"",
		"& go install $goPackage",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.ps1 is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"$env:GONOSUMDB = \"github.com/gentleman-programming/gentle-ai/v2\"",
		"$env:GOPRIVATE = \"github.com/gentleman-programming/gentle-ai/v2\"",
		"$env:GONOPROXY = \"github.com/gentleman-programming/gentle-ai/v2\"",
	} {
		if strings.Contains(script, clobber) {
			t.Fatalf("scripts/install.ps1 clobbers existing user env with %q; beta proxy bypass must preserve existing patterns", clobber)
		}
	}

	start := strings.Index(script, "function Add-GoEnvPattern {")
	if start == -1 {
		t.Fatal("scripts/install.ps1 is missing Add-GoEnvPattern function")
	}
	endMarker := "\n}\n\nfunction Test-Installation"
	end := strings.Index(script[start:], endMarker)
	if end == -1 {
		t.Fatal("could not locate end of Add-GoEnvPattern function")
	}
	function := script[start : start+end+3]

	for _, want := range []string{
		"$current = [Environment]::GetEnvironmentVariable($Name, \"Process\")",
		"Set-Item -Path \"Env:$Name\" -Value $Pattern",
		"Set-Item -Path \"Env:$Name\" -Value (\"{0},{1}\" -f $Pattern, $current)",
		"if ($patterns -contains $Pattern) { return }",
	} {
		if !strings.Contains(function, want) {
			t.Fatalf("Add-GoEnvPattern does not preserve existing env patterns; missing %q", want)
		}
	}
}

// extractAtomicPublishBinary returns the body of atomic_publish_binary
// defined in scripts/install.sh so individual test cases can invoke it
// through bash -c under controlled PATH / filesystem conditions.
func extractAtomicPublishBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	script := string(content)
	start := strings.Index(script, "atomic_publish_binary() {")
	if start == -1 {
		t.Fatal("scripts/install.sh is missing atomic_publish_binary function (#1728)")
	}
	end := strings.Index(script[start:], "\n}\n")
	if end == -1 {
		t.Fatal("could not locate end of atomic_publish_binary function")
	}
	return script[start : start+end+2]
}

// runAtomicPublish invokes atomic_publish_binary in a fresh bash process
// with stubDir prepended to PATH (so cp/mv/chmod can be overridden by
// per-test fixtures) and the system PATH preserved. The helper's exit
// status is returned to the caller. src/dest are passed via env vars
// rather than positional args because Go's exec.Command assigns the
// first command argument to $0, not $1.
func runAtomicPublish(t *testing.T, function, stubDir, src, dest string) error {
	t.Helper()
	cmd := exec.Command("bash", "-c",
		`set -e
`+function+`
atomic_publish_binary "$ATOMIC_TEST_SRC" "$ATOMIC_TEST_DEST"`)
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ATOMIC_TEST_SRC="+src,
		"ATOMIC_TEST_DEST="+dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &runErr{msg: string(out), err: err}
	}
	return nil
}

type runErr struct {
	msg string
	err error
}

func (e *runErr) Error() string { return e.msg + ": " + e.err.Error() }
func (e *runErr) Unwrap() error { return e.err }

// writeFile is a small helper for seeding test fixtures.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

// fileDigest returns the SHA-256 of a file's contents.
func fileDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// stageDebris returns a sorted list of files in dir whose names start
// with base + ".staging." — the residue atomic_publish_binary is required
// to clean up on every failure path.
func stageDebris(t *testing.T, dir, base string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, base+".staging.*"))
	if err != nil {
		t.Fatalf("Glob(%q) error = %v", filepath.Join(dir, base+".staging.*"), err)
	}
	return matches
}

// TestInstallScriptAtomicPublishBinaryShape guards the helper's structural
// invariants: staging lives in dest's directory, mode is set before
// publication, and the rename happens only after a successful stage.
func TestInstallScriptAtomicPublishBinaryShape(t *testing.T) {
	path := filepath.Join("..", "..", "scripts", "install.sh")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	script := string(content)

	for _, want := range []string{
		// Acceptance 1: staging lives in dest's directory.
		`staging="$(mktemp "${dest}.staging.XXXXXX")"`,
		// Acceptance 2: executable mode set before publication.
		`chmod +x -- "$staging"`,
		// Acceptance 3: same-filesystem rename publishes.
		`mv -- "$staging" "$dest"`,
		// Acceptance 4: failure of any stage returns 1.
		`return 1`,
		// Acceptance 5: every failure path removes the staging file.
		`rm -f -- "$staging" 2>/dev/null || true`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("atomic_publish_binary is missing required acceptance pattern %q", want)
		}
	}

	// install_binary must invoke the helper rather than the unsafe cp
	// over the live destination. The pre-fix call site was a direct
	// `cp "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"`
	// followed by chmod; the post-fix path goes through the helper.
	if strings.Contains(script,
		`cp "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}" 2>/dev/null`,
	) {
		t.Error("scripts/install.sh install_binary still does a direct cp over the live destination; #1728 not fixed")
	}
	if !strings.Contains(script, `atomic_publish_binary "${tmpdir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"`) {
		t.Error("scripts/install.sh install_binary must call atomic_publish_binary for the binary publish (#1728)")
	}
}

// TestInstallScriptAtomicPublishBinaryFirstInstall covers acceptance 6
// (first installation): the helper creates the destination when it does
// not yet exist and writes the candidate's bytes into it.
func TestInstallScriptAtomicPublishBinaryFirstInstall(t *testing.T) {
	binDir := t.TempDir()
	dir := t.TempDir()
	src := filepath.Join(dir, "candidate")
	dest := filepath.Join(dir, "gentle-ai")
	candidateBytes := []byte("first-install-payload")
	writeFile(t, src, candidateBytes)

	if err := runAtomicPublish(t, extractAtomicPublishBinary(t), binDir, src, dest); err != nil {
		t.Fatalf("atomic_publish_binary returned error on first install: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", dest, err)
	}
	if !bytes.Equal(got, candidateBytes) {
		t.Errorf("dest contents = %q, want %q", got, candidateBytes)
	}
	if debris := stageDebris(t, dir, "gentle-ai"); len(debris) != 0 {
		t.Errorf("first install left staging debris: %v", debris)
	}
}

// TestInstallScriptAtomicPublishBinaryReplacement covers acceptance 6
// (successful replacement): an existing destination is atomically
// overwritten with the candidate's bytes, and the executable bit is
// preserved on the published file.
func TestInstallScriptAtomicPublishBinaryReplacement(t *testing.T) {
	binDir := t.TempDir()
	dir := t.TempDir()
	src := filepath.Join(dir, "candidate")
	dest := filepath.Join(dir, "gentle-ai")
	oldBytes := []byte("old-binary-bytes-do-not-match")
	candidateBytes := []byte("new-binary-bytes-after-replace")
	writeFile(t, src, candidateBytes)
	writeFile(t, dest, oldBytes)
	// Mark the destination executable so we can verify mode survival.
	if err := os.Chmod(dest, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", dest, err)
	}

	if err := runAtomicPublish(t, extractAtomicPublishBinary(t), binDir, src, dest); err != nil {
		t.Fatalf("atomic_publish_binary returned error on replacement: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", dest, err)
	}
	if !bytes.Equal(got, candidateBytes) {
		t.Errorf("dest contents = %q, want %q", got, candidateBytes)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", dest, err)
	}
	if st.Mode()&0o111 == 0 {
		t.Errorf("dest lost executable mode after replacement: mode = %v", st.Mode())
	}
	if debris := stageDebris(t, dir, "gentle-ai"); len(debris) != 0 {
		t.Errorf("replacement left staging debris: %v", debris)
	}
}

// TestInstallScriptAtomicPublishBinaryPartialCopyFailure covers
// acceptance 4 and 6 (partial-copy failure): when cp fails after
// truncating the destination, the previous executable must remain
// byte-for-byte unchanged and no staging debris must remain. Acceptance
// 7 requires this exact regression: preservation of the old digest and
// absence of staging debris.
func TestInstallScriptAtomicPublishBinaryPartialCopyFailure(t *testing.T) {
	binDir := t.TempDir()
	dir := t.TempDir()

	// Stub cp that simulates the unsafe behaviour: it truncates the
	// destination to a partial write and exits non-zero. The helper
	// invokes cp with "-- src staging", so the destination argument is
	// the LAST positional. This mirrors the reproduction in #1728 — except
	// the unsafe behaviour also corrupts the live destination, which the
	// helper must avoid.
	cpStub := `#!/usr/bin/env bash
# args: ... src dest  (cp -- src dest; "--" is $1)
dest=""
for arg in "$@"; do
    dest="$arg"
done
printf 'partial-new-binary' > "$dest"
exit 74
`
	if err := os.WriteFile(filepath.Join(binDir, "cp"), []byte(cpStub), 0o755); err != nil {
		t.Fatalf("WriteFile(cp stub) error = %v", err)
	}

	src := filepath.Join(dir, "candidate")
	dest := filepath.Join(dir, "gentle-ai")
	oldBytes := []byte("OLD-EXECUTABLE-BYTES-PRESERVED-ON-FAILURE")
	writeFile(t, src, []byte("ignored-candidate-bytes"))
	writeFile(t, dest, oldBytes)
	oldDigest := fileDigest(t, dest)

	err := runAtomicPublish(t, extractAtomicPublishBinary(t), binDir, src, dest)
	if err == nil {
		t.Fatal("atomic_publish_binary succeeded despite cp stub returning exit 74")
	}

	// Acceptance 4: existing destination unchanged byte-for-byte.
	gotDigest := fileDigest(t, dest)
	if gotDigest != oldDigest {
		t.Errorf("dest digest changed after partial-copy failure: got %s, want %s", gotDigest, oldDigest)
	}
	gotBytes, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", dest, err)
	}
	if !bytes.Equal(gotBytes, oldBytes) {
		t.Errorf("dest contents changed after partial-copy failure: got %q, want %q", gotBytes, oldBytes)
	}

	// Acceptance 5 and 7: no staging debris left behind.
	if debris := stageDebris(t, dir, "gentle-ai"); len(debris) != 0 {
		t.Errorf("partial-copy failure left staging debris: %v", debris)
	}
}

// TestInstallScriptAtomicPublishBinaryRenameFailure covers acceptance 4
// and 6 (rename failure): when the staging file exists but mv cannot
// publish it (e.g. cross-device, EACCES on rename of a read-only inode),
// the helper must remove the staging file and leave the destination
// byte-for-byte unchanged.
func TestInstallScriptAtomicPublishBinaryRenameFailure(t *testing.T) {
	binDir := t.TempDir()
	dir := t.TempDir()

	// Stub mv that always exits non-zero. This exercises the rename
	// failure branch; the helper must clean up the staging file and
	// preserve the existing destination.
	mvStub := `#!/usr/bin/env bash
exit 75
`
	if err := os.WriteFile(filepath.Join(binDir, "mv"), []byte(mvStub), 0o755); err != nil {
		t.Fatalf("WriteFile(mv stub) error = %v", err)
	}

	src := filepath.Join(dir, "candidate")
	dest := filepath.Join(dir, "gentle-ai")
	oldBytes := []byte("OLD-EXECUTABLE-BYTES-PRESERVED-ON-RENAME-FAILURE")
	writeFile(t, src, []byte("would-be-new-bytes"))
	writeFile(t, dest, oldBytes)
	oldDigest := fileDigest(t, dest)

	err := runAtomicPublish(t, extractAtomicPublishBinary(t), binDir, src, dest)
	if err == nil {
		t.Fatal("atomic_publish_binary succeeded despite mv stub returning exit 75")
	}

	gotDigest := fileDigest(t, dest)
	if gotDigest != oldDigest {
		t.Errorf("dest digest changed after rename failure: got %s, want %s", gotDigest, oldDigest)
	}
	if debris := stageDebris(t, dir, "gentle-ai"); len(debris) != 0 {
		t.Errorf("rename failure left staging debris: %v", debris)
	}
}
