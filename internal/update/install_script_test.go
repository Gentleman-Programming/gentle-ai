package update

import (
	"bytes"
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

	// Since #4689 the beta env patterns are derived from the module path
	// resolved from go.mod at the pinned main commit SHA, so the assertions
	// guard the derivation form, not a hard-coded /v3 literal.
	script := string(content)
	for _, want := range []string{
		"local env_pattern=\"${module}\"",
		"prepend_go_env_pattern GONOSUMDB \"${env_pattern}\"",
		"prepend_go_env_pattern GOPRIVATE \"${env_pattern}\"",
		"prepend_go_env_pattern GONOPROXY \"${env_pattern}\"",
		"go install \"$go_package\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.sh is missing %q in beta go install proxy-bypass path", want)
		}
	}

	// The derived pattern must come from the resolved module, never a
	// hard-coded owner/major literal (the #4689 regression class).
	for _, banned := range []string{
		"prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v",
		"prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v",
		"prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v",
	} {
		if strings.Contains(script, banned) {
			t.Fatalf("scripts/install.sh hard-codes the module major in the beta proxy-bypass patterns (%q); derive it from go.mod at the resolved ref", banned)
		}
	}

	for _, clobber := range []string{
		"GONOSUMDB=github.com/gentleman-programming/gentle-ai/v3 \\",
		"GOPRIVATE=github.com/gentleman-programming/gentle-ai/v3 \\",
		"GONOPROXY=github.com/gentleman-programming/gentle-ai/v3 \\",
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
GONOPROXY=github.com/gentleman-programming/gentle-ai/v3
prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v3
prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v3
prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v3
printf '%s\n%s\n%s\n' "$GONOSUMDB" "$GOPRIVATE" "$GONOPROXY"
`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run prepend_go_env_pattern fixture: %v\noutput: %s", err, out)
	}

	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"github.com/gentleman-programming/gentle-ai/v3,example.com/private",
		"github.com/gentleman-programming/gentle-ai/v3,github.com/acme/*",
		"github.com/gentleman-programming/gentle-ai/v3",
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
		"Add-GoEnvPattern -Name \"GONOSUMDB\" -Pattern \"github.com/gentleman-programming/gentle-ai/v3\"",
		"Add-GoEnvPattern -Name \"GOPRIVATE\" -Pattern \"github.com/gentleman-programming/gentle-ai/v3\"",
		"Add-GoEnvPattern -Name \"GONOPROXY\" -Pattern \"github.com/gentleman-programming/gentle-ai/v3\"",
		"& go install $goPackage",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.ps1 is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"$env:GONOSUMDB = \"github.com/gentleman-programming/gentle-ai/v3\"",
		"$env:GOPRIVATE = \"github.com/gentleman-programming/gentle-ai/v3\"",
		"$env:GONOPROXY = \"github.com/gentleman-programming/gentle-ai/v3\"",
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

// TestInstallScriptsGoInstallPackageMatchesModuleMajor guards the install
// scripts against the regression that shipped in v3.0.1: a module-major
// migration that rewrites the literal module string misses them. Since
// #4689 the Unix installer derives the module path (and its major) from
// go.mod at the resolved source ref, so its guard asserts that derivation;
// the PowerShell installer still pins the current major (until #4690) and
// is guarded against a migration that forgets it.
func TestInstallScriptsGoInstallPackageMatchesModuleMajor(t *testing.T) {
	goMod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	moduleLine := strings.SplitN(string(goMod), "\n", 2)[0]
	majorMatch := regexp.MustCompile(`^module github\.com/gentleman-programming/gentle-ai/(v[0-9]+)$`).FindStringSubmatch(strings.TrimSpace(moduleLine))
	if majorMatch == nil {
		t.Fatalf("go.mod module line %q does not carry a major version suffix", moduleLine)
	}
	major := majorMatch[1]

	cases := []struct {
		script  string
		pattern string
	}{
		// install.sh resolves the ref (release tag for stable, main commit
		// SHA for beta) and builds the package from the module declared at
		// that ref, so a future major bump needs no script change.
		{"install.sh", `local go_package="${module}/cmd/${BINARY_NAME}@${ref}"`},
		{"install.ps1", `$goPackage = "github.com/$($GITHUB_OWNER.ToLower())/$GITHUB_REPO/` + major + `/cmd/$BINARY_NAME@$version"`},
	}
	stale := regexp.MustCompile(`/v[0-9]+/cmd/`)
	for _, tc := range cases {
		content, err := os.ReadFile(filepath.Join("..", "..", "scripts", tc.script))
		if err != nil {
			t.Fatal(err)
		}
		text := string(content)
		if !strings.Contains(text, tc.pattern) {
			t.Errorf("scripts/%s must build the go install package on the %s module: missing %q", tc.script, major, tc.pattern)
		}
		for _, hit := range stale.FindAllString(text, -1) {
			if hit != "/"+major+"/cmd/" {
				t.Errorf("scripts/%s still references %s in a go install package path; go.mod is %s", tc.script, hit, major)
			}
		}
		if strings.Contains(text, "tags are v2.x") {
			t.Errorf("scripts/%s comment still describes v2.x tags", tc.script)
		}
	}
}

// TestInstallScriptUsesStagingAndRename (issue #1728): the install path must
// never `cp` directly to the destination; partial `cp` failures leave the
// destination binary truncated. The script stages next to the destination
// and renames in place so the previous binary either survives intact or is
// fully replaced.
func TestInstallScriptUsesStagingAndRename(t *testing.T) {
	content := readInstallScript(t)
	// Find any `cp` invocation that writes under the install dir variable.
	cpToDest := regexp.MustCompile(`cp\s+"\$\{?tmpdir`)
	if cpToDest.MatchString(content) {
		t.Errorf("scripts/install.sh still copies from tmpdir directly; use a same-directory staging file + POSIX rename")
	}
	// The rename path must use a local staging file, not a cross-directory mv.
	staging := regexp.MustCompile(`\.staging\.\$\$`)
	if !staging.MatchString(content) {
		t.Errorf("scripts/install.sh must stage with a same-directory .staging.$$ file")
	}
	mvInPlace := regexp.MustCompile(`mv -f -- "\$staging" "\$final"`)
	if !mvInPlace.MatchString(content) {
		t.Errorf("scripts/install.sh must rename staging -> final in place; got: %s", findMatches(content, regexp.MustCompile(`mv -f[^\n]*`)))
	}
}

// TestInstallScriptUsesExplicitMode (issue #1728): chmod +x inherits the
// caller umask (0711 under umask 022, 0700 under umask 077). The install path
// must use `install -m 0755` so the destination mode is the intended 0755
// regardless of caller umask.
func TestInstallScriptUsesExplicitMode(t *testing.T) {
	content := readInstallScript(t)
	if strings.Contains(content, "chmod +x \"${install_dir}/${BINARY_NAME}\"") {
		t.Error("scripts/install.sh must not chmod +x the destination binary; use install -m 0755")
	}
	if !strings.Contains(content, "install -m 0755") {
		t.Error("scripts/install.sh must call `install -m 0755` to publish the binary")
	}
}

// TestInstallScriptSudoUsesPositionalArgs (issue #1728): the sudo path must
// not interpolate the destination path into a sudo bash -c program; a path
// containing $(...) or backticks would be re-interpreted as commands by the
// invoking user's shell.
func TestInstallScriptSudoUsesPositionalArgs(t *testing.T) {
	content := readInstallScript(t)
	// Old vulnerable form: sudo cp ... "${install_dir}/${BINARY_NAME}"
	badCp := regexp.MustCompile(`sudo\s+cp\s+`)
	if badCp.MatchString(content) {
		t.Error("scripts/install.sh still uses sudo cp; pass paths as positional args to a fixed shell body")
	}
	// The new safe form passes paths as positional args: sudo -- bash -c '...' _ "$1" ...
	safeSudo := regexp.MustCompile(`sudo -- bash -c '[^']*' _ "`)
	if !safeSudo.MatchString(content) {
		t.Error("scripts/install.sh must pass sudo paths as positional args: `sudo -- bash -c '...' _ \"$1\" ...`")
	}
}

// TestInstallScriptHasStagingCleanupTrap (issue #1728): a TERM during the
// install must clean up the destination-local staging file; otherwise the
// previous binary can be left in a partial state and debris remains next
// to it.
func TestInstallScriptHasStagingCleanupTrap(t *testing.T) {
	content := readInstallScript(t)
	trapLine := regexp.MustCompile(`trap\s+'rm -f -- "\$staging"`).MatchString(content)
	if !trapLine {
		t.Error("scripts/install.sh must register `trap 'rm -f -- \"$staging\"' EXIT TERM INT` to clean up the staging file on any exit")
	}
}

func readInstallScript(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatalf("ReadFile install.sh: %v", err)
	}
	return string(content)
}

func findMatches(text string, re *regexp.Regexp) []string {
	matches := re.FindAllString(text, -1)
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches
}
