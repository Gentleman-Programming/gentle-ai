package update

// Tools is the static registry of managed tools that can be checked for updates.
//
// InstallMethod controls which upgrade strategy the executor uses:
//   - InstallBrew: managed via homebrew (macOS/Linux with brew)
//   - InstallGoInstall: installed via `go install <GoImportPath>@version`
//   - InstallBinary: downloaded binary from GitHub Releases (atomic replace)
//
// For brew-managed platforms the executor picks brew regardless of the
// field here; InstallMethod represents the non-brew fallback strategy.
var Tools = []ToolInfo{
	{
		Name:          "gentle-ai",
		Owner:         "Gentleman-Programming",
		Repo:          "gentle-ai",
		DetectCmd:     nil, // version comes from build-time ldflags (app.Version)
		VersionPrefix: "v",
		// gentle-ai: brew on macOS, binary release download on Linux/Windows.
		// Self-upgrade of the running binary on Windows is deferred to Phase 2.
		InstallMethod: InstallBinary,
	},
	{
		Name:          "engram",
		Owner:         "Gentleman-Programming",
		Repo:          "engram",
		DetectCmd:     []string{"engram", "version"},
		VersionPrefix: "v",
		// engram: brew on macOS/Linux-brew, go-install elsewhere.
		InstallMethod: InstallGoInstall,
		GoImportPath:  "github.com/Gentleman-Programming/engram/cmd/engram",
	},
	{
		Name:          "gga",
		Owner:         "Gentleman-Programming",
		Repo:          "gentleman-guardian-angel",
		DetectCmd:     []string{"gga", "--version"},
		VersionPrefix: "v",
		// gga: brew on macOS, binary release download on Linux/Windows.
		InstallMethod: InstallBinary,
	},
}
