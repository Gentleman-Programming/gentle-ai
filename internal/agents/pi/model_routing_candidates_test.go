package pi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func routingPathName(t *testing.T) string {
	if runtime.GOOS != "windows" {
		return packageBinName
	}
	t.Setenv("PATHEXT", ".EXE;.CMD")
	return packageBinName + ".EXE"
}
func writeRoutingPackage(t *testing.T, root, manifest string) string {
	t.Helper()
	target := writeTarget(t, root, "bin/"+packageBinName, 0o755)
	writeManifest(t, root, manifest)
	return target
}
func canonicalRoutingPath(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	must(t, err)
	return filepath.Clean(got)
}
func routingCause[T error](err error) bool {
	var cause T
	return errors.As(err, &cause)
}
func assertCandidateError(t *testing.T, err error, kind, cause string) {
	t.Helper()
	var candidate *CandidateError
	if !errors.As(err, &candidate) || candidate.Kind != kind || candidate.Source == "" || candidate.Path == "" {
		t.Fatalf("error = %T %v; want CandidateError kind=%q with provenance", err, err, kind)
	}
	ok := map[string]bool{"settings": routingCause[*SettingsError](err), "source": routingCause[*SourceError](err), "manifest": routingCause[*ManifestError](err), "bin": routingCause[*BinError](err)}[cause]
	if !ok {
		t.Fatalf("error = %v; want %s cause", err, cause)
	}
}
func routingFixture(t *testing.T) (base, cwd, agent string) {
	t.Helper()
	base = t.TempDir()
	cwd, agent = filepath.Join(base, "project"), filepath.Join(base, "agent")
	must(t, os.MkdirAll(cwd, 0o755))
	return
}

func TestEnumerateModelRoutingCandidatesPATHOrderAndFiltering(t *testing.T) {
	base, cwd, _ := routingFixture(t)
	ambient := filepath.Join(base, "ambient")
	must(t, os.MkdirAll(ambient, 0o755))
	must(t, os.MkdirAll(filepath.Join(ambient, "relative"), 0o755))
	name := routingPathName(t)
	ambientRelative := filepath.Join(ambient, "relative", name)
	ambientEmpty := filepath.Join(ambient, name)
	must(t, os.WriteFile(ambientRelative, []byte("ambient"), 0o755))
	must(t, os.WriteFile(ambientEmpty, []byte("ambient"), 0o755))
	firstDir, relativeDir, emptyDir := filepath.Join(base, "first"), filepath.Join(cwd, "relative"), cwd
	first := filepath.Join(firstDir, name)
	relative := filepath.Join(relativeDir, name)
	empty := filepath.Join(emptyDir, name)
	for _, path := range []string{first, relative, empty} {
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte("candidate"), 0o755))
	}
	missingDir, directoryDir, nonexecDir := filepath.Join(base, "missing"), filepath.Join(base, "directory"), filepath.Join(base, "nonexec")
	must(t, os.MkdirAll(filepath.Join(directoryDir, name), 0o755))
	if runtime.GOOS != "windows" {
		must(t, os.MkdirAll(nonexecDir, 0o755))
		must(t, os.WriteFile(filepath.Join(nonexecDir, name), []byte("no"), 0o644))
	}
	aliasDir := filepath.Join(base, "alias")
	pathEntries := []string{firstDir, "relative", "", missingDir, directoryDir}
	if runtime.GOOS != "windows" {
		pathEntries = append(pathEntries, nonexecDir)
	}
	if runtime.GOOS != "windows" {
		must(t, os.MkdirAll(aliasDir, 0o755))
		must(t, os.Symlink(first, filepath.Join(aliasDir, name)))
		pathEntries = append(pathEntries, aliasDir)
	}
	t.Setenv("PATH", strings.Join(pathEntries, string(os.PathListSeparator)))
	t.Chdir(ambient)
	beforePATH, beforeWD, beforeTree := os.Getenv("PATH"), func() string { got, _ := os.Getwd(); return got }(), snapshotPiTree(t, base)
	got, err := EnumerateModelRoutingCandidates(cwd+string(filepath.Separator)+".", filepath.Join(base, "agent"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{canonicalRoutingPath(t, first), canonicalRoutingPath(t, relative), canonicalRoutingPath(t, empty)}
	if len(got) != len(want) {
		t.Fatalf("candidates = %#v; want %d candidates", got, len(want))
	}
	for i, candidate := range got {
		if candidate.Path != want[i] || candidate.Source != "PATH" {
			t.Fatalf("candidate[%d] = %#v; want PATH %q", i, candidate, want[i])
		}
	}
	afterWD, _ := os.Getwd()
	if os.Getenv("PATH") != beforePATH || afterWD != beforeWD || snapshotPiTree(t, base) != beforeTree {
		t.Fatal("enumeration changed PATH, cwd, or fixture filesystem")
	}
}

func TestEnumerateModelRoutingCandidatesPackageSourcesAndOrdering(t *testing.T) {
	cases := []struct {
		name, source string
		scope        PackageSourceScope
		root         func(string, string) string
	}{
		{"project npm", "npm:gentle-pi@1.2.3", SettingsScopeProject, func(c, a string) string { return filepath.Join(c, ".pi", "npm", "node_modules", "gentle-pi") }},
		{"user npm", "npm:gentle-pi", SettingsScopeUser, func(c, a string) string { return filepath.Join(a, "npm", "node_modules", "gentle-pi") }},
		{"project git", "git:github.com/team/gentle-pi@v1", SettingsScopeProject, func(c, a string) string { return filepath.Join(c, ".pi", "git", "github.com", "team", "gentle-pi") }},
		{"user git", "git:github.com/team/gentle-pi", SettingsScopeUser, func(c, a string) string { return filepath.Join(a, "git", "github.com", "team", "gentle-pi") }},
		{"project local", "./models", SettingsScopeProject, func(c, a string) string { return filepath.Join(c, ".pi", "models") }},
		{"user local", "./models", SettingsScopeUser, func(c, a string) string { return filepath.Join(a, "models") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, cwd, agent := routingFixture(t)
			root := tc.root(cwd, agent)
			bin := writeRoutingPackage(t, root, `{"name":"gentle-pi-models","bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
			settings := filepath.Join(agent, "settings.json")
			if tc.scope == SettingsScopeProject {
				settings = filepath.Join(cwd, ".pi", "settings.json")
			}
			writePiSettings(t, settings, `{"packages":[{"source":"`+tc.source+`","name":"gentle-pi"}]}`)
			t.Setenv("PATH", filepath.Join(base, "missing"))
			got, err := EnumerateModelRoutingCandidates(cwd, agent)
			if err != nil || len(got) != 1 || got[0] != (ModelRoutingCandidate{canonicalRoutingPath(t, bin), tc.source}) {
				t.Fatalf("candidates = %#v, error = %v; want package candidate", got, err)
			}
		})
	}

	base, cwd, agent := routingFixture(t)
	pathBin := writeRoutingPackage(t, filepath.Join(base, "path"), `{"name":"gentle-pi-models","bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
	packageRoot := filepath.Join(base, "package")
	packageBin := writeRoutingPackage(t, packageRoot, `{"name":"gentle-pi-models","bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
	writePiSettings(t, filepath.Join(cwd, ".pi", "settings.json"), `{"packages":[{"source":"`+packageRoot+`","name":"gentle-pi"}]}`)
	t.Setenv("PATH", filepath.Dir(pathBin))
	got, err := EnumerateModelRoutingCandidates(cwd, agent)
	if err != nil || len(got) != 2 || got[0] != (ModelRoutingCandidate{canonicalRoutingPath(t, pathBin), "PATH"}) || got[1] != (ModelRoutingCandidate{canonicalRoutingPath(t, packageBin), packageRoot}) {
		t.Fatalf("ordered candidates = %#v, error = %v; want PATH then package", got, err)
	}
	t.Setenv("PATH", strings.Join([]string{filepath.Dir(pathBin), filepath.Dir(packageBin)}, string(os.PathListSeparator)))
	got, err = EnumerateModelRoutingCandidates(cwd, agent)
	if err != nil || len(got) != 2 || got[1].Source != "PATH" {
		t.Fatalf("duplicate candidates = %#v, error = %v; want package once", got, err)
	}
}

func TestEnumerateModelRoutingCandidatesExplicitConfigurationFailsClosed(t *testing.T) {
	cases := []struct {
		name, settings, manifest, want string
	}{
		{"malformed settings", `{`, "", "settings"},
		{"malformed root", `{"packages":[{"source":"npm:","name":"gentle-pi"}]}`, "", "source"},
		{"malformed manifest", `{"packages":[{"source":"./package","name":"gentle-pi"}]}`, `{`, "manifest"},
		{"malformed bin", `{"packages":[{"source":"./package","name":"gentle-pi"}]}`, `{"name":"gentle-pi-models","bin":true}`, "bin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base, cwd, agent := routingFixture(t)
			pathBin := writeRoutingPackage(t, filepath.Join(base, "path"), `{"name":"gentle-pi-models","bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
			settingsPath := filepath.Join(cwd, ".pi", "settings.json")
			writePiSettings(t, settingsPath, tc.settings)
			if tc.manifest != "" {
				root := filepath.Join(cwd, ".pi", "package")
				writeTarget(t, root, "bin/"+packageBinName, 0o755)
				writeManifest(t, root, tc.manifest)
			}
			t.Setenv("PATH", filepath.Dir(pathBin))
			_, err := EnumerateModelRoutingCandidates(cwd, agent)
			switch tc.want {
			case "settings":
				assertCandidateError(t, err, "settings", "settings")
			case "source":
				assertCandidateError(t, err, "package-root", "source")
			case "manifest":
				assertCandidateError(t, err, "package-bin", "manifest")
			case "bin":
				assertCandidateError(t, err, "package-bin", "bin")
			}
		})
	}
}

func TestEnumerateModelRoutingCandidatesNotConfiguredInputAndAgentConvention(t *testing.T) {
	base, cwd, agent := routingFixture(t)
	bad := filepath.Join(base, "bad")
	must(t, os.MkdirAll(filepath.Join(bad, packageBinName), 0o755))
	var notConfigured *NotConfiguredError
	var candidate *CandidateError
	var err error
	for _, path := range []string{filepath.Join(base, "missing"), bad} {
		t.Setenv("PATH", path)
		_, err = EnumerateModelRoutingCandidates(cwd, agent)
		if !errors.As(err, &candidate) || !errors.As(err, &notConfigured) || !errors.Is(err, ErrPackageNotConfigured) || !strings.Contains(err.Error(), "PATH") {
			t.Fatalf("all-candidate error = %T %v; want actionable PATH and settings causes", err, err)
		}
	}

	bin := writeRoutingPackage(t, filepath.Join(agent, "models"), `{"name":"gentle-pi-models","bin":{"gentle-pi-models":"bin/gentle-pi-models"}}`)
	writePiSettings(t, filepath.Join(agent, "settings.json"), `{"packages":[{"source":"./models","name":"gentle-pi"}]}`)
	t.Setenv("PI_CODING_AGENT_DIR", agent)
	t.Setenv("PATH", filepath.Join(base, "missing"))
	beforePATH, beforeAgent := os.Getenv("PATH"), os.Getenv("PI_CODING_AGENT_DIR")
	got, err := EnumerateModelRoutingCandidates(cwd, "")
	if err != nil || len(got) != 1 || got[0].Path != canonicalRoutingPath(t, bin) || got[0].Source != "./models" || os.Getenv("PATH") != beforePATH || os.Getenv("PI_CODING_AGENT_DIR") != beforeAgent {
		t.Fatalf("agent convention result = %#v, error = %v; want configured user candidate and unchanged env", got, err)
	}

	_, err = EnumerateModelRoutingCandidates("relative", agent)
	if !errors.As(err, &candidate) || !errors.Is(err, ErrInvalidPiRoot) {
		t.Fatalf("invalid cwd error = %T %v; want CandidateError wrapping ErrInvalidPiRoot", err, err)
	}
}
