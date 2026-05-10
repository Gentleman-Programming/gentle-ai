package update

// UpdateStatus represents the outcome of a single tool version check.
type UpdateStatus string

const (
	UpToDate        UpdateStatus = "up-to-date"
	UpdateAvailable UpdateStatus = "update-available"
	NotInstalled    UpdateStatus = "not-installed"
	// RegisteredNotMaterialized means an OpenCode community plugin is listed in
	// ~/.config/opencode/tui.json, but OpenCode has not yet materialized it under
	// ~/.config/opencode/node_modules/<pkg>/package.json.
	RegisteredNotMaterialized UpdateStatus = "registered-not-materialized"
	VersionUnknown            UpdateStatus = "version-unknown"
	CheckFailed               UpdateStatus = "check-failed"
	// DevBuild is used when the installed version is the sentinel "dev" string,
	// indicating a source-built binary. Such builds are not auto-targeted for upgrade.
	DevBuild UpdateStatus = "dev-build"
)

// InstallMethod describes how a managed tool is installed on the current platform.
// Used by the upgrade executor to choose the correct upgrade strategy.
type InstallMethod string

const (
	InstallBrew      InstallMethod = "brew"
	InstallGoInstall InstallMethod = "go-install"
	InstallBinary    InstallMethod = "binary"
	// InstallScript downloads and executes the project's install.sh via pipe.
	// Used for tools that distribute via shell scripts rather than pre-built binaries
	// (e.g., GGA which has no release binary assets).
	InstallScript InstallMethod = "script"
	// InstallOpenCodePlugin is a manual upgrade method: Gentle AI registers the
	// package in tui.json, and OpenCode owns package resolution on restart/reload.
	InstallOpenCodePlugin InstallMethod = "opencode-plugin"
)

// ToolInfo describes a managed tool that can be checked for updates.
type ToolInfo struct {
	Name          string        // human-readable name (e.g., "gentle-ai")
	Owner         string        // GitHub repository owner
	Repo          string        // GitHub repository name
	DetectCmd     []string      // command to detect installed version; nil = use build var
	VersionPrefix string        // prefix to strip from version output (e.g., "v")
	InstallMethod InstallMethod // how this tool is installed (used by upgrade executor)
	GoImportPath  string        // for go-install tools (e.g. "github.com/.../cmd/engram")
	NpmPackage    string        // for OpenCode community plugins installed in ~/.config/opencode/node_modules
}

// UpdateResult holds the result of checking a single tool for updates.
type UpdateResult struct {
	Tool             ToolInfo
	InstalledVersion string
	LatestVersion    string
	Status           UpdateStatus
	ReleaseURL       string
	UpdateHint       string
	Err              error
}
