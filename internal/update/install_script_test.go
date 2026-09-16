package update

import (
	"bytes"
	"encoding/json"
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
		"prepend_go_env_pattern GONOSUMDB github.com/gentleman-programming/gentle-ai/v3",
		"prepend_go_env_pattern GOPRIVATE github.com/gentleman-programming/gentle-ai/v3",
		"prepend_go_env_pattern GONOPROXY github.com/gentleman-programming/gentle-ai/v3",
		"go install \"$go_package\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.sh is missing %q in beta go install proxy-bypass path", want)
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
		`Add-GoEnvPattern -Name "GONOSUMDB" -Pattern $module`,
		`Add-GoEnvPattern -Name "GOPRIVATE" -Pattern $module`,
		`Add-GoEnvPattern -Name "GONOPROXY" -Pattern $module`,
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

func TestWindowsInstallScriptGoChannels(t *testing.T) {
	for _, major := range []string{"", "3", "4", "5", "12"} {
		for _, channel := range []string{"stable", "beta", "nightly"} {
			t.Run(channel+"/v"+major, func(t *testing.T) {
				module, tag := "github.com/gentleman-programming/gentle-ai", "v1.2.3"
				if major != "" {
					module += "/v" + major
					tag = "v" + major + ".2.3"
				}
				mod := "// leading comment\n\nmodule \"" + module + "\" // trailing comment\n\ngo 1.99.0\n"
				calls, output, err := runPowerShellInstallerFixture(t, channel, module, tag, strings.Repeat("a", 40), mod, "", major == "")
				if err != nil {
					t.Fatalf("installer failed: %v\n%s\n%s", err, output, calls)
				}
				version, endpoint, env := tag, "releases/latest", "|github.com/acme/*|"+module
				if channel != "stable" {
					version, endpoint, env = strings.Repeat("a", 40), "commits/main", module+"|"+module+",github.com/acme/*|"+module
				}
				for _, want := range []string{
					"resolve https://api.github.com/repos/Gentleman-Programming/gentle-ai/" + endpoint,
					"metadata https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/" + version + "/go.mod",
					"install " + module + "/cmd/gentle-ai@" + version, "env " + env,
					"parser local|off|",
				} {
					if !strings.Contains(calls, want+"\n") {
						t.Errorf("missing %q in calls:\n%s", want, calls)
					}
				}
				for _, prefix := range []string{"resolve ", "metadata ", "install ", "parser "} {
					if strings.Count(calls, prefix) != 1 {
						t.Errorf("expected one %q:\n%s", prefix, calls)
					}
				}
			})
		}
	}
}

func TestWindowsInstallScriptGoFailures(t *testing.T) {
	const module = "github.com/gentleman-programming/gentle-ai/v5"
	for _, tt := range []struct{ name, channel, tag, sha, mod, fail string }{
		{name: "missing release", channel: "stable", tag: " "},
		{name: "malformed release", channel: "stable", tag: "v5.2.3/../../main"},
		{name: "missing sha", sha: " "}, {name: "malformed sha", sha: "main"},
		{name: "release HTTP failure", channel: "stable", fail: "resolve"},
		{name: "commit HTTP failure", fail: "resolve"}, {name: "metadata HTTP failure", fail: "metadata"},
		{name: "missing module", mod: "go 1.25.10\n"},
		{name: "duplicate module", mod: "module " + module + "\nmodule " + module + "\n"},
		{name: "malformed module", mod: "module \"unterminated\n"},
		{name: "lookalike repository", mod: "module github.com/gentleman-programming/gentle-ai-extra/v5\n"},
		{name: "regex lookalike", mod: "module githubXcom/gentleman-programming/gentle-ai/v5\n"},
		{name: "wrong case", mod: "module github.com/Gentleman-Programming/gentle-ai/v5\n"},
		{name: "trailing segment", mod: "module " + module + "/extra\n"},
		{name: "noncanonical major", mod: "module github.com/gentleman-programming/gentle-ai/v05\n"},
		{name: "v1 suffix", mod: "module github.com/gentleman-programming/gentle-ai/v1\n"},
		{name: "parser exit failure", fail: "parser"}, {name: "install exit failure", fail: "install"},
		{name: "parser failure with absent env", fail: "parser"},
		{name: "binary hold", fail: "binary"}, {name: "insecure hold", fail: "insecure"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.channel == "" {
				tt.channel = "beta"
			}
			if tt.tag == "" {
				tt.tag = "v5.2.3"
			}
			if tt.sha == "" {
				tt.sha = strings.Repeat("a", 40)
			}
			if tt.mod == "" {
				tt.mod = "module " + module + "\ngo 1.99.0\n"
			}
			calls, output, err := runPowerShellInstallerFixture(t, tt.channel, module, tt.tag, tt.sha, tt.mod, tt.fail, tt.name == "parser failure with absent env")
			if err == nil {
				t.Fatalf("unexpected success:\n%s\n%s", output, calls)
			}
			want := 0
			if tt.fail == "install" {
				want = 1
			}
			if strings.Count(calls, "install ") != want {
				t.Errorf("unexpected installs:\n%s", calls)
			}
			if (tt.fail == "binary" || tt.fail == "insecure") && (strings.Contains(calls, "resolve ") || strings.Contains(calls, "metadata ")) {
				t.Errorf("distribution hold made HTTP calls:\n%s", calls)
			}
		})
	}
}

// Native PowerShell doubles run the unchanged whole-script entrypoint. Only Go's
// offline metadata parser is real; no install, application, or HTTP request runs.
func runPowerShellInstallerFixture(t *testing.T, channel, module, tag, sha, mod, fail string, absent bool) (string, string, error) {
	t.Helper()
	if testing.Short() {
		t.Skip("PowerShell subprocess fixture")
	}
	var shell string
	names := []string{"pwsh"}
	if runtime.GOOS == "windows" {
		names = []string{"powershell", "pwsh"}
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			shell = path
			break
		}
	}
	if shell == "" {
		t.Skip("PowerShell is unavailable")
	}
	root := t.TempDir()
	tmp := filepath.Join(root, "tmp")
	for _, dir := range []string{tmp, filepath.Join(root, "telemetry")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("module", mod)
	write("calls", "")
	write("tmp/unrelated", "preserve")
	write("telemetry/mode", "off")
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	goName := "go"
	if runtime.GOOS == "windows" {
		goName += ".exe"
	}
	config, err := json.Marshal(map[string]any{"Channel": channel, "Module": module, "Tag": tag, "SHA": sha, "Fail": fail, "Absent": absent, "Script": script, "Go": filepath.Join(runtime.GOROOT(), "bin", goName)})
	if err != nil {
		t.Fatal(err)
	}
	write("config.json", string(config))
	write("fixture.ps1", `
$ErrorActionPreference = 'Stop'
$cfg = Get-Content -LiteralPath (Join-Path $env:FIXTURE 'config.json') -Raw | ConvertFrom-Json
function Log([string]$Text) { Add-Content -LiteralPath (Join-Path $env:FIXTURE 'calls') -Value $Text -Encoding UTF8 }
function Parser-State {
    $values = foreach ($name in @('GOTOOLCHAIN','GOWORK','GOFLAGS')) {
        $value = [Environment]::GetEnvironmentVariable($name, 'Process')
        if ($null -eq $value) { '<absent>' } else { $value }
    }
    $values -join '|'
}
foreach ($name in @('GOTOOLCHAIN','GOWORK','GOFLAGS')) { Remove-Item "Env:$name" -ErrorAction SilentlyContinue }
if (-not $cfg.Absent) { $env:GOTOOLCHAIN = 'auto'; $env:GOWORK = '/missing-workspace'; $env:GOFLAGS = '-mod=vendor' }
Remove-Item Env:GONOSUMDB -ErrorAction SilentlyContinue
$env:GOPRIVATE = 'github.com/acme/*'; $env:GONOPROXY = $cfg.Module
function chcp { if (($args -join ' ') -ne '65001') { throw 'unexpected chcp' } }
function Invoke-RestMethod {
    param([string]$Uri)
    Log ('resolve ' + $Uri)
    if ($cfg.Fail -eq 'resolve') { throw 'fixture HTTP failure' }
    switch -CaseSensitive ($Uri) {
        'https://api.github.com/repos/Gentleman-Programming/gentle-ai/releases/latest' { return @{tag_name=$cfg.Tag} }
        'https://api.github.com/repos/Gentleman-Programming/gentle-ai/commits/main' {
            $sha = $cfg.SHA
            if ($script:resolved) { $sha = 'b' * 40 }
            $script:resolved = $true
            return @{sha=$sha}
        }
        default { throw ('unexpected HTTP: ' + $Uri) }
    }
}
function Invoke-WebRequest {
    param([string]$Uri, [switch]$UseBasicParsing, [string]$OutFile)
    Log ('metadata ' + $Uri)
    if (-not $UseBasicParsing -or -not $OutFile -or $Uri -cnotmatch '\Ahttps://raw\.githubusercontent\.com/Gentleman-Programming/gentle-ai/[^/]+/go\.mod\z') { throw 'unexpected metadata request' }
    if ($cfg.Fail -eq 'metadata') { throw 'fixture download failure' }
    Copy-Item -LiteralPath (Join-Path $env:FIXTURE 'module') -Destination $OutFile
}
function go {
    $global:LASTEXITCODE = 0
    switch ($args[0]) {
        'mod' {
            if ($args.Count -ne 4 -or $args[1] -ne 'edit' -or $args[2] -ne '-json') { throw 'unexpected parser' }
            Log ('parser ' + $env:GOTOOLCHAIN + '|' + $env:GOWORK + '|' + $env:GOFLAGS)
            if ($env:GOTOOLCHAIN -ne 'local' -or $env:GOWORK -ne 'off' -or $env:GOFLAGS) { throw 'parser is not offline' }
            if ($cfg.Fail -eq 'parser') { $global:LASTEXITCODE = 1; return '{"Module":{"Path":"github.com/gentleman-programming/gentle-ai/v5"}}' }
            & $cfg.Go @args
            $global:LASTEXITCODE = $LASTEXITCODE
        }
        'install' {
            if ($args.Count -ne 2) { throw 'unexpected install arguments' }
            Log ('install ' + $args[1]); Log ('env ' + $env:GONOSUMDB + '|' + $env:GOPRIVATE + '|' + $env:GONOPROXY)
            Log ('install-state ' + (Parser-State))
            if ($cfg.Fail -eq 'install') { $global:LASTEXITCODE = 1 }
        }
        'env' {
            if ($args.Count -ne 2 -or $args[1] -ne 'GOBIN') { throw 'unexpected go env' }
            Join-Path $env:FIXTURE 'bin'
        }
        default { throw 'unexpected Go operation' }
    }
}
$options = @{ Method='go'; Channel=$cfg.Channel }
if ($cfg.Fail -eq 'binary') { $options.Method = 'binary' }
if ($cfg.Fail -eq 'insecure') { $options.Insecure = $true }
try { & $cfg.Script @options; exit $LASTEXITCODE }
finally { Log ('final-state ' + (Parser-State)) }
`)
	cmd := exec.Command(shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", filepath.Join(root, "fixture.ps1"))
	// Preserve only OS process essentials, never credentials or Go/user configuration.
	for _, name := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATHEXT"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(shell), "HOME="+root, "USERPROFILE="+root, "TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp, "FIXTURE="+root, "GOPROXY=off", "GOSUMDB=off", "GOENV=off", "TEST_TELEMETRY_DIR="+filepath.Join(root, "telemetry"))
	out, runErr := cmd.CombinedOutput()
	data, err := os.ReadFile(filepath.Join(root, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n")
	state := "auto|/missing-workspace|-mod=vendor"
	if absent {
		state = "<absent>|<absent>|<absent>"
	}
	if !strings.Contains(calls, "final-state "+state+"\n") {
		t.Errorf("parser environment was not restored:\n%s\n%s", calls, out)
	}
	if strings.Contains(calls, "install ") && !strings.Contains(calls, "install-state "+state+"\n") {
		t.Errorf("install inherited parser overrides:\n%s", calls)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil || len(entries) != 1 || entries[0].Name() != "unrelated" {
		t.Errorf("metadata cleanup: %v, %v", entries, err)
	}
	if sentinel, err := os.ReadFile(filepath.Join(tmp, "unrelated")); err != nil || string(sentinel) != "preserve" {
		t.Errorf("cleanup changed unrelated sentinel: %q, %v", sentinel, err)
	}
	return calls, string(out), runErr
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
		{"install.sh", `local go_package="github.com/${owner_lc}/${GITHUB_REPO}/` + major + `/cmd/${BINARY_NAME}@${version}"`},
		{"install.ps1", `$goPackage = "$module/cmd/$BINARY_NAME@$version"`},
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
		if tc.script == "install.ps1" {
			t.Run(tc.script, func(t *testing.T) {
				module, sha := "github.com/gentleman-programming/gentle-ai/"+major, strings.Repeat("a", 40)
				calls, output, err := runPowerShellInstallerFixture(t, "beta", module, "", sha, string(goMod), "", false)
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
