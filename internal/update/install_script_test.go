package update

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
		`prepend_go_env_pattern GONOSUMDB "$module"`,
		`prepend_go_env_pattern GOPRIVATE "$module"`,
		`prepend_go_env_pattern GONOPROXY "$module"`,
		"go install \"$go_package\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.sh is missing %q in beta go install proxy-bypass path", want)
		}
	}
	if !strings.Contains(script, "if ! (\n        if [ \"${CHANNEL}\" = \"beta\" ]; then") {
		t.Fatal("scripts/install.sh must scope beta-only Go environment exports to the go install subshell")
	}
	if !strings.Contains(script, "    ); then\n        fatal \"Failed to install via go install.") {
		t.Fatal("scripts/install.sh must preserve fatal handling when go install fails")
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

func TestInstallScriptGoChannels(t *testing.T) {
	const base = "github.com/gentleman-programming/gentle-ai"
	for _, major := range []string{"", "3", "4", "5", "12"} {
		for _, channel := range []string{"stable", "beta", "nightly"} {
			t.Run(channel+"/v"+major, func(t *testing.T) {
				module, tag := base, "v1.2.3"
				if major != "" {
					module += "/v" + major
					tag = "v" + major + ".2.3"
				}
				// Use the actual Go parser, including syntax a line-one parser misses.
				mod := "// leading comment\n\nmodule \"" + module + "\" // trailing comment\n\ngo 1.99.0\n"
				calls, output, err := runGoInstallerFixture(t, channel, module, tag, mod, "", "")
				if err != nil {
					t.Fatalf("installer failed: %v\n%s\n%s", err, output, calls)
				}
				version, gitCalls, releaseCalls := tag, 0, 1
				env := "env |github.com/acme/*|" + module
				if channel != "stable" {
					version, gitCalls, releaseCalls = strings.Repeat("a", 40), 1, 0
					env = "env " + module + "|" + module + ",github.com/acme/*|" + module
				}
				for _, want := range []string{
					"metadata https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/" + version + "/go.mod\n",
					"install " + module + "/cmd/gentle-ai@" + version + "\n", env + "\n",
				} {
					if !strings.Contains(calls, want) {
						t.Errorf("missing %q in calls:\n%s", want, calls)
					}
				}
				if channel != "stable" {
					wantPostEnv := "post-env |github.com/acme/*|" + module + "\n"
					if !strings.Contains(calls, wantPostEnv) {
						t.Errorf("beta Go environment leaked outside go install subshell: missing %q in calls:\n%s", wantPostEnv, calls)
					}
				}
				if strings.Count("\n"+calls, "\ngit ") != gitCalls || strings.Count(calls, "release\n") != releaseCalls || strings.Count(calls, "install ") != 1 {
					t.Errorf("unexpected channel resolution/install count:\n%s", calls)
				}
			})
		}
	}
}

func TestInstallScriptGoFailures(t *testing.T) {
	const module = "github.com/gentleman-programming/gentle-ai/v5"
	const valid = "module " + module + "\ngo 1.25.10\n"
	sha := strings.Repeat("a", 40)
	for _, tt := range []struct {
		name, mod, refs, fail string
	}{
		{name: "missing module", mod: "go 1.25.10\n"},
		{name: "duplicate module", mod: valid + "module " + module + "\n"},
		{name: "malformed module", mod: "module \"unterminated\n"},
		{name: "lookalike repository", mod: "module github.com/gentleman-programming/gentle-ai-extra/v5\n"},
		{name: "regex lookalike", mod: "module githubXcom/gentleman-programming/gentle-ai/v5\n"},
		{name: "trailing segment", mod: "module " + module + "/extra\n"},
		{name: "noncanonical major", mod: "module github.com/gentleman-programming/gentle-ai/v05\n"},
		{name: "missing ref", refs: "\n"},
		{name: "wrong ref", refs: sha + "\trefs/heads/other\n"},
		{name: "malformed sha", refs: "not-a-sha\trefs/heads/main\n"},
		{name: "duplicate ref", refs: strings.Repeat(sha+"\trefs/heads/main\n", 2)},
		{name: "git failure", fail: "git"},
		{name: "metadata failure", fail: "metadata"},
		{name: "parser failure", fail: "parser"},
		{name: "install failure", fail: "install"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mod := tt.mod
			if mod == "" {
				mod = valid
			}
			calls, output, err := runGoInstallerFixture(t, "beta", module, "v5.2.3", mod, tt.refs, tt.fail)
			if err == nil {
				t.Fatalf("installer unexpectedly succeeded:\n%s\n%s", output, calls)
			}
			wantInstalls := 0
			if tt.fail == "install" {
				wantInstalls = 1
			}
			if strings.Count(calls, "install ") != wantInstalls {
				t.Errorf("unexpected install count:\n%s", calls)
			}
		})
	}
}

// Execute the whole script with only local utilities and fail-closed command doubles.
// The only real Go operation allowed by the double is offline metadata parsing.
func runGoInstallerFixture(t *testing.T, channel, module, tag, mod, refs, fail string) (string, string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Bash installer supports macOS and Linux")
	}
	root := t.TempDir()
	bin, tmp := filepath.Join(root, "bin"), filepath.Join(root, "tmp")
	for _, dir := range []string{bin, tmp, filepath.Join(root, "telemetry")} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, text string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	if refs == "" {
		refs = strings.Repeat("a", 40) + "\trefs/heads/main\n"
	}
	write(filepath.Join(root, "module"), mod, 0o600)
	write(filepath.Join(root, "refs"), refs, 0o600)
	write(filepath.Join(root, "calls"), "", 0o600)
	// Prevent the real parser's telemetry sidecar from racing TempDir cleanup.
	write(filepath.Join(root, "telemetry", "mode"), "off", 0o600)
	write(filepath.Join(tmp, "unrelated"), "preserve", 0o600)
	for _, tool := range []string{"uname", "tr", "sed", "tail", "head", "mktemp", "rm"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"git": `
[ "$*" = "ls-remote --exit-code https://github.com/Gentleman-Programming/gentle-ai.git refs/heads/main" ] || exit 90
printf 'git %s\n' "$*" >> "$FIXTURE/calls"
[ "$FAIL" != git ] || exit 1
if [ -e "$FIXTURE/resolved" ]; then
    printf '%040d\trefs/heads/main\n' 1
else
    : > "$FIXTURE/resolved"
    /bin/cat "$FIXTURE/refs"
fi`,
		"curl": `
dest=""
while [ "$#" -gt 1 ]; do
    case "$1" in
        -sL|-sfL|-fsSL) shift ;;
        -o) dest="$2"; shift 2 ;;
        -w) [ "$2" = '\n%{http_code}' ] || exit 90; shift 2 ;;
        *) exit 90 ;;
    esac
done
case "$1" in
    https://api.github.com/repos/Gentleman-Programming/gentle-ai/releases/latest)
        printf 'release\n' >> "$FIXTURE/calls"
        printf '{"tag_name":"%s"}\n200' "$TAG" ;;
    https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/*/go.mod)
        printf 'metadata %s\n' "$1" >> "$FIXTURE/calls"
        [ "$FAIL" != metadata ] || exit 1
        [ -n "$dest" ] || exit 90
        /bin/cat "$FIXTURE/module" > "$dest" ;;
    *) exit 90 ;;
esac`,
		"go": `
case "$1" in
    mod)
        [ "$#" = 4 ] && [ "$2" = edit ] && [ "$3" = -json ] || exit 90
        [ "$FAIL" != parser ] || exit 1
        exec "$REAL_GO" "$@" ;;
    install)
        [ "$#" = 2 ] || exit 90
        printf 'install %s\nenv %s|%s|%s\n' "$2" "$GONOSUMDB" "$GOPRIVATE" "$GONOPROXY" >> "$FIXTURE/calls"
        [ "$FAIL" != install ] || exit 1 ;;
        env)
        [ "$#" = 2 ] && [ "$2" = GOBIN ] || exit 90
        printf 'post-env %s|%s|%s\n' "$GONOSUMDB" "$GOPRIVATE" "$GONOPROXY" >> "$FIXTURE/calls"
        printf '%s/bin\n' "$FIXTURE" ;;
    *) exit 90 ;;
esac`,
		"gentle-ai": `[ "$*" = version ] || exit 90; printf 'fixture version\n'`,
	} {
		write(filepath.Join(bin, name), "#!/bin/bash\nset -euo pipefail\n"+body+"\n", 0o700)
	}
	cmd := exec.Command("/bin/bash", "--noprofile", "--norc", "../../scripts/install.sh", "--method", "go", "--channel", channel)
	cmd.Env = []string{
		"PATH=" + bin, "HOME=" + root, "TMPDIR=" + tmp, "FIXTURE=" + root,
		"REAL_GO=" + filepath.Join(runtime.GOROOT(), "bin", "go"), "TAG=" + tag, "FAIL=" + fail,
		"GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOPROXY=off", "GOSUMDB=off",
		"TEST_TELEMETRY_DIR=" + filepath.Join(root, "telemetry"),
		"GONOSUMDB=", "GOPRIVATE=github.com/acme/*", "GONOPROXY=" + module,
	}
	out, err := cmd.CombinedOutput()
	calls, readErr := os.ReadFile(filepath.Join(root, "calls"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	entries, readErr := os.ReadDir(tmp)
	if readErr != nil || len(entries) != 1 || entries[0].Name() != "unrelated" {
		t.Errorf("metadata cleanup left unexpected files: %v, error: %v", entries, readErr)
	}
	return string(calls), string(out), err
}

// TestInstallScriptsGoInstallPackageMatchesModuleMajor guards the install
// scripts against the regression that shipped in v3.0.1. Check source contracts
// and exercise the dynamic installer with repository metadata. Derive the
// expected major from go.mod to cover future module migrations.
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
		{"install.sh", `local go_package="${module}/cmd/${BINARY_NAME}@${version}"`},
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
		if tc.script == "install.sh" {
			t.Run(tc.script, func(t *testing.T) {
				module, sha := "github.com/gentleman-programming/gentle-ai/"+major, strings.Repeat("a", 40)
				calls, output, err := runGoInstallerFixture(t, "beta", module, "", string(goMod), "", "")
				if err != nil {
					t.Fatalf("installer failed: %v\n%s\n%s", err, output, calls)
				}
				want := "install " + module + "/cmd/gentle-ai@" + sha + "\n"
				if !strings.Contains(calls, want) || strings.Count(calls, "install ") != 1 {
					t.Errorf("scripts/%s must install the module declared in go.mod: want %q, calls:\n%s", tc.script, want, calls)
				}
			})
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
