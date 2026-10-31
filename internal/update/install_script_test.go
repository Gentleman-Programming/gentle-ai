package update

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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
		"Add-GoEnvPattern -Name \"GONOSUMDB\" -Pattern \"github.com/gentleman-programming/gentle-ai/v4\"",
		"Add-GoEnvPattern -Name \"GOPRIVATE\" -Pattern \"github.com/gentleman-programming/gentle-ai/v4\"",
		"Add-GoEnvPattern -Name \"GONOPROXY\" -Pattern \"github.com/gentleman-programming/gentle-ai/v4\"",
		"& go install $goPackage",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("scripts/install.ps1 is missing %q in beta go install proxy-bypass path", want)
		}
	}

	for _, clobber := range []string{
		"$env:GONOSUMDB = \"github.com/gentleman-programming/gentle-ai/v4\"",
		"$env:GOPRIVATE = \"github.com/gentleman-programming/gentle-ai/v4\"",
		"$env:GONOPROXY = \"github.com/gentleman-programming/gentle-ai/v4\"",
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
	if major != "v4" {
		t.Errorf("current module major = %s, want v4", major)
	}

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

const installPathMark = "# gentle-ai installer"

// runInstallPathSetup sources scripts/install.sh in an isolated HOME and runs
// ensure_install_dir_on_path for dir with the given SHELL, OS and PATH.
func runInstallPathSetup(t *testing.T, home, shell, osName, path, dir string, extraEnv ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh targets macOS and Linux; Windows installs through install.ps1")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", `source "$1"; setup_colors; OS="$2"; ensure_install_dir_on_path "$3"`, "_", script, osName, dir)
	cmd.Env = append([]string{"HOME=" + home, "SHELL=" + shell, "PATH=" + path, "TERM=dumb"}, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ensure_install_dir_on_path failed: %v\noutput: %s", err, out)
	}
	return string(out)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	return string(content)
}

func TestInstallScriptPathSetupChoosesStartupFilePerShell(t *testing.T) {
	const basePath = "/usr/bin:/bin"
	shLine := func(dir string) string { return `export PATH="` + dir + `:$PATH" ` + installPathMark }

	tests := []struct {
		name     string
		shell    string
		os       string
		existing []string // startup files (relative to HOME) that exist before the run
		zdotdir  string   // relative to HOME; empty keeps ZDOTDIR unset
		want     string   // relative to HOME
		fish     bool
	}{
		{name: "zsh uses ~/.zshrc", shell: "/bin/zsh", os: "darwin", want: ".zshrc"},
		{name: "zsh honors ZDOTDIR", shell: "/usr/bin/zsh", os: "linux", zdotdir: "zdot", want: "zdot/.zshrc"},
		{name: "bash on macOS creates ~/.bash_profile when none exists", shell: "/bin/bash", os: "darwin", want: ".bash_profile"},
		{name: "bash on macOS keeps an existing ~/.profile", shell: "/bin/bash", os: "darwin", existing: []string{".profile"}, want: ".profile"},
		{name: "bash on macOS prefers ~/.bash_login over ~/.profile", shell: "/bin/bash", os: "darwin", existing: []string{".bash_login", ".profile"}, want: ".bash_login"},
		{name: "bash on macOS prefers ~/.bash_profile", shell: "/bin/bash", os: "darwin", existing: []string{".bash_profile", ".bash_login", ".profile"}, want: ".bash_profile"},
		{name: "bash on Linux uses ~/.bashrc", shell: "/bin/bash", os: "linux", existing: []string{".profile"}, want: ".bashrc"},
		{name: "fish uses its own conf.d file", shell: "/usr/bin/fish", os: "linux", want: ".config/fish/conf.d/gentle-ai.fish", fish: true},
		{name: "other shells use ~/.profile", shell: "/bin/dash", os: "linux", want: ".profile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".local", "bin")
			for _, name := range tt.existing {
				if err := os.WriteFile(filepath.Join(home, name), []byte("# user content\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var extra []string
			if tt.zdotdir != "" {
				if err := os.MkdirAll(filepath.Join(home, tt.zdotdir), 0o755); err != nil {
					t.Fatal(err)
				}
				extra = append(extra, "ZDOTDIR="+filepath.Join(home, tt.zdotdir))
			}

			out := runInstallPathSetup(t, home, tt.shell, tt.os, basePath, dir, extra...)

			profile := filepath.Join(home, tt.want)
			line := shLine(dir)
			if tt.fish {
				line = "set -gx PATH '" + dir + "' $PATH " + installPathMark
			}
			want := line + "\n"
			if slices.Contains(tt.existing, tt.want) {
				want = "# user content\n" + want
			}
			if got := readFileString(t, profile); got != want {
				t.Fatalf("%s content = %q, want %q", tt.want, got, want)
			}
			current := `Current terminal: export PATH="` + dir + `:$PATH"`
			if tt.fish {
				current = "Current terminal: set -gx PATH '" + dir + "' $PATH"
			}
			for _, msg := range []string{
				current,
				"Future terminals: open a new terminal",
				"PATH was added to " + profile,
			} {
				if !strings.Contains(out, msg) {
					t.Errorf("output missing %q\noutput: %s", msg, out)
				}
			}
			for _, name := range tt.existing {
				if name != tt.want && readFileString(t, filepath.Join(home, name)) != "# user content\n" {
					t.Errorf("%s was modified; only %s may change", name, tt.want)
				}
			}

			again := runInstallPathSetup(t, home, tt.shell, tt.os, basePath, dir, extra...)
			if got := readFileString(t, profile); got != want {
				t.Fatalf("second run changed %s: %q, want %q", tt.want, got, want)
			}
			for _, msg := range []string{current, "Future terminals: open a new terminal", "PATH is already configured in " + profile} {
				if !strings.Contains(again, msg) {
					t.Errorf("second run output missing %q\noutput: %s", msg, again)
				}
			}
		})
	}
}

func TestInstallScriptPathSetupWritesNothingWhenDirIsOnPath(t *testing.T) {
	for _, entry := range []string{"%s", "%s/"} {
		home := t.TempDir()
		dir := filepath.Join(home, ".local", "bin")
		path := "/usr/bin:" + strings.Replace(entry, "%s", dir, 1) + ":/bin"
		out := runInstallPathSetup(t, home, "/bin/zsh", "darwin", path, dir)
		if strings.TrimSpace(out) != "" {
			t.Errorf("PATH entry %q: expected no output, got %q", entry, out)
		}
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("PATH entry %q: HOME must stay untouched, found %v", entry, entries)
		}
	}
}

func TestInstallScriptPathSetupRefusesSymlinkedStartupFile(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "bin")
	target := filepath.Join(home, "dotfiles-zshrc")
	if err := os.WriteFile(target, []byte("# managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(home, ".zshrc")
	if err := os.Symlink(target, profile); err != nil {
		t.Fatal(err)
	}

	out := runInstallPathSetup(t, home, "/bin/zsh", "darwin", "/usr/bin:/bin", dir)

	if got := readFileString(t, target); got != "# managed\n" {
		t.Fatalf("symlink target was written through: %q", got)
	}
	if info, err := os.Lstat(profile); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink (err=%v)", profile, err)
	}
	line := `export PATH="` + dir + `:$PATH" ` + installPathMark
	for _, msg := range []string{
		"Add this line to " + profile + ", then open a new terminal:\n  " + line,
		`Current terminal: export PATH="` + dir + `:$PATH"`,
	} {
		if !strings.Contains(out, msg) {
			t.Errorf("output missing %q\noutput: %s", msg, out)
		}
	}
	for _, claim := range []string{"PATH was added", "Future terminals"} {
		if strings.Contains(out, claim) {
			t.Errorf("output claims %q although nothing was written\noutput: %s", claim, out)
		}
	}
}

func TestInstallScriptPathSetupRefusesSymlinkedFolderUnderHome(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "bin")
	real := filepath.Join(home, "dotfiles-config")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, ".config")); err != nil {
		t.Fatal(err)
	}

	out := runInstallPathSetup(t, home, "/usr/bin/fish", "linux", "/usr/bin:/bin", dir)

	entries, err := os.ReadDir(real)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("wrote through a symlinked ~/.config: %v", entries)
	}
	if strings.Contains(out, "PATH was added") {
		t.Errorf("output claims PATH was added through a symlinked folder\noutput: %s", out)
	}
}

func TestInstallScriptPathSetupPutsDirOnProcessPathWhenAlreadyConfigured(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "bin")
	runInstallPathSetup(t, home, "/bin/zsh", "darwin", "/usr/bin:/bin", dir)

	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", `source "$1"; setup_colors; OS=darwin; ensure_install_dir_on_path "$2" >/dev/null; printf '%s' "$PATH"`, "_", script, dir)
	cmd.Env = []string{"HOME=" + home, "SHELL=/bin/zsh", "PATH=/usr/bin:/bin", "TERM=dumb"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("second run failed: %v\noutput: %s", err, out)
	}
	if got, want := string(out), dir+":/usr/bin:/bin"; got != want {
		t.Fatalf("process PATH after already-configured run = %q, want %q", got, want)
	}
}

func TestInstallScriptPathSetupKeepsFileWithoutTrailingNewline(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "bin")
	profile := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(profile, []byte("alias ll='ls -l'"), 0o644); err != nil {
		t.Fatal(err)
	}

	runInstallPathSetup(t, home, "/bin/bash", "linux", "/usr/bin:/bin", dir)

	want := "alias ll='ls -l'\nexport PATH=\"" + dir + ":$PATH\" " + installPathMark + "\n"
	if got := readFileString(t, profile); got != want {
		t.Fatalf("~/.bashrc = %q, want %q", got, want)
	}
}

func TestInstallScriptPathSetupRefusesUnsafeDir(t *testing.T) {
	for _, tc := range []struct{ shell, dir, current string }{
		{"/bin/zsh", `/opt/we"ird/bin`, `export PATH='/opt/we"ird/bin':"$PATH"`},
		{"/bin/zsh", "/opt/$HOME/bin", `export PATH='/opt/$HOME/bin':"$PATH"`},
		{"/bin/zsh", "/opt/back`tick/bin", "export PATH='/opt/back`tick/bin':\"$PATH\""},
		{"/usr/bin/fish", "/opt/it's/bin", `set -gx PATH '/opt/it\'s/bin' $PATH`},
		{"/usr/bin/fish", `/opt/back\slash/bin`, `set -gx PATH '/opt/back\\slash/bin' $PATH`},
	} {
		home := t.TempDir()
		out := runInstallPathSetup(t, home, tc.shell, "linux", "/usr/bin:/bin", tc.dir)
		profile := filepath.Join(home, ".zshrc")
		if strings.HasSuffix(tc.shell, "fish") {
			profile = filepath.Join(home, ".config", "fish", "conf.d", "gentle-ai.fish")
		}
		for _, msg := range []string{
			"Current terminal: " + tc.current,
			"Add this line to " + profile + ", then open a new terminal:\n  " + tc.current + " " + installPathMark,
		} {
			if !strings.Contains(out, msg) {
				t.Errorf("%s %q: output missing %q\noutput: %s", tc.shell, tc.dir, msg, out)
			}
		}
		if strings.Contains(out, "Future terminals") {
			t.Errorf("%s %q: output claims future terminals work\noutput: %s", tc.shell, tc.dir, out)
		}
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("%s %q: unsafe dir was written to a startup file: %v", tc.shell, tc.dir, entries)
		}
		if strings.Contains(out, "PATH was added") {
			t.Errorf("%s %q: output claims PATH was added\noutput: %s", tc.shell, tc.dir, out)
		}
	}
}

func TestInstallScriptInstallMethodsUsePathSetup(t *testing.T) {
	content := readInstallScript(t)
	for _, fn := range []string{"install_go() {", "install_binary() {"} {
		start := strings.Index(content, fn)
		if start == -1 {
			t.Fatalf("scripts/install.sh is missing %s", fn)
		}
		end := strings.Index(content[start:], "\n}\n")
		body := content[start : start+end]
		if !strings.Contains(body, "ensure_install_dir_on_path ") {
			t.Errorf("%s must call ensure_install_dir_on_path", fn)
		}
	}
	for _, stale := range []string{"Add this to your shell profile", "is not in your PATH\""} {
		if strings.Contains(content, stale) {
			t.Errorf("scripts/install.sh still prints the old manual PATH hint %q", stale)
		}
	}
}

func readWindowsInstallScript(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install.ps1"))
	if err != nil {
		t.Fatalf("ReadFile install.ps1: %v", err)
	}
	return string(content)
}

// powershellFunction returns the source of `function <name> {` up to its
// closing brace at column zero.
func powershellFunction(t *testing.T, script, name string) string {
	t.Helper()
	start := strings.Index(script, "function "+name+" {")
	if start == -1 {
		t.Fatalf("scripts/install.ps1 is missing function %s", name)
	}
	end := strings.Index(script[start:], "\n}\n")
	if end == -1 {
		t.Fatalf("could not locate end of function %s", name)
	}
	return script[start : start+end+3]
}

func TestWindowsInstallScriptPersistsUserPath(t *testing.T) {
	script := readWindowsInstallScript(t)

	get := powershellFunction(t, script, "Get-UserEnv")
	for _, want := range []string{`OpenSubKey("Environment")`, "[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames"} {
		if !strings.Contains(get, want) {
			t.Errorf("Get-UserEnv must read HKCU\\Environment without expanding names; missing %q", want)
		}
	}
	set := powershellFunction(t, script, "Set-UserEnv")
	for _, want := range []string{`OpenSubKey("Environment", $true)`, "GetValueKind($Key)", "[Microsoft.Win32.RegistryValueKind]::ExpandString", "Publish-EnvChange"} {
		if !strings.Contains(set, want) {
			t.Errorf("Set-UserEnv must keep the value kind and broadcast; missing %q", want)
		}
	}
	publish := powershellFunction(t, script, "Publish-EnvChange")
	for _, want := range []string{"SendMessageTimeout", "0x1a", `"Environment"`, "[IntPtr]0xffff"} {
		if !strings.Contains(publish, want) {
			t.Errorf("Publish-EnvChange must broadcast WM_SETTINGCHANGE for Environment; missing %q", want)
		}
	}
	install := powershellFunction(t, script, "Install-ViaGo")
	if !strings.Contains(install, "Add-InstallDirToPath -Directory $gobin") {
		t.Error("Install-ViaGo must persist GOBIN with Add-InstallDirToPath")
	}
	if !strings.Contains(powershellFunction(t, script, "Add-InstallDirToPath"), "Add-UserPathEntry -Directory $Directory") {
		t.Error("Add-InstallDirToPath must persist the directory with Add-UserPathEntry")
	}
	if strings.Contains(script, "Add it to your PATH environment variable.") {
		t.Error("scripts/install.ps1 still prints the old manual PATH hint")
	}
}

func TestWindowsInstallScriptAddUserPathEntryBehavior(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not installed")
	}
	add := powershellFunction(t, readWindowsInstallScript(t), "Add-UserPathEntry")
	harness := add + `
$script:writes = 0
function Get-UserEnv { param([string]$Key) return $script:reg }
function Set-UserEnv { param([string]$Key, [string]$Value) $script:reg = $Value; $script:writes++ }
$sep = [IO.Path]::PathSeparator
$env:GOTEST_ROOT = 'C:\go'
$cases = @(
  @{ reg = 'C:\a;%USERPROFILE%\x'; dir = 'C:\go\bin' },
  @{ reg = $null; dir = 'C:\go\bin' },
  @{ reg = 'C:\a;c:\GO\BIN\'; dir = 'C:\go\bin' },
  @{ reg = '%GOTEST_ROOT%\bin;C:\a'; dir = 'C:\go\bin' }
)
foreach ($c in $cases) {
  $script:reg = $c.reg
  $script:writes = 0
  $env:PATH = ('/usr/bin' + $sep + '/bin')
  $result = Add-UserPathEntry -Directory $c.dir
  $sessionFirst = $env:PATH -eq ($c.dir + $sep + '/usr/bin' + $sep + '/bin')
  Write-Output ('{0}|{1}|{2}|{3}' -f $result, $script:reg, $script:writes, $sessionFirst)
}
`
	harnessPath := filepath.Join(t.TempDir(), "harness.ps1")
	if err := os.WriteFile(harnessPath, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput()
	if err != nil {
		t.Fatalf("pwsh harness failed: %v\noutput: %s", err, out)
	}
	got := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(out), "\r\n", "\n")), "\n")
	want := []string{
		`added|C:\go\bin;C:\a;%USERPROFILE%\x|1|True`,
		`added|C:\go\bin|1|True`,
		`present|C:\a;c:\GO\BIN\|0|True`,
		`present|%GOTEST_ROOT%\bin;C:\a|0|True`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Add-UserPathEntry results:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestWindowsInstallScriptSkipsDirAlreadyOnSessionPath pins S6 on Windows: a
// directory the session already resolves (Machine PATH, $PROFILE) is neither
// written to HKCU nor reported.
func TestWindowsInstallScriptSkipsDirAlreadyOnSessionPath(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not installed")
	}
	script := readWindowsInstallScript(t)
	harness := powershellFunction(t, script, "Add-UserPathEntry") + powershellFunction(t, script, "Add-InstallDirToPath") + `
function Get-UserEnv { param([string]$Key) return 'C:\a' }
function Set-UserEnv { param([string]$Key, [string]$Value) $script:writes++ }
function Write-Success { param([string]$Message) Write-Host $Message }
$sep = [IO.Path]::PathSeparator
foreach ($session in @('/Users/u/go/bin', '/USERS/U/GO/BIN')) {
  $script:writes = 0
  $env:PATH = ($session + $sep + '/usr/bin')
  $before = $env:PATH
  $output = (Add-InstallDirToPath -Directory '/Users/u/go/bin' *>&1 | Out-String).Trim()
  Write-Output ('writes={0}|output=[{1}]|unchanged={2}' -f $script:writes, $output, ($env:PATH -eq $before))
}
`
	harnessPath := filepath.Join(t.TempDir(), "harness.ps1")
	if err := os.WriteFile(harnessPath, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", harnessPath).CombinedOutput()
	if err != nil {
		t.Fatalf("pwsh harness failed: %v\noutput: %s", err, out)
	}
	got := strings.TrimSpace(strings.ReplaceAll(string(out), "\r\n", "\n"))
	want := "writes=0|output=[]|unchanged=True\nwrites=0|output=[]|unchanged=True"
	if got != want {
		t.Fatalf("dir already on session PATH:\n%s\nwant:\n%s", got, want)
	}
}
