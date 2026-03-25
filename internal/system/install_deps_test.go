package system

import (
	"strings"
	"testing"
)

func TestInstallHintGitDarwin(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	hint := installHintGit(profile)
	if hint != "brew install git" {
		t.Fatalf("installHintGit(darwin) = %q", hint)
	}
}

func TestInstallHintGitUbuntu(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt", LinuxDistro: "ubuntu"}
	hint := installHintGit(profile)
	if !strings.Contains(hint, "apt-get install") {
		t.Fatalf("installHintGit(ubuntu) = %q", hint)
	}
}

func TestInstallHintGitArch(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "pacman", LinuxDistro: "arch"}
	hint := installHintGit(profile)
	if !strings.Contains(hint, "pacman -S") {
		t.Fatalf("installHintGit(arch) = %q", hint)
	}
}

func TestInstallHintNodeDarwin(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	hint := installHintNode(profile)
	if hint != "brew install node" {
		t.Fatalf("installHintNode(darwin) = %q", hint)
	}
}

func TestInstallHintNodeUbuntu(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt", LinuxDistro: "ubuntu"}
	hint := installHintNode(profile)
	if !strings.Contains(hint, "nodesource") {
		t.Fatalf("installHintNode(ubuntu) = %q, want NodeSource URL", hint)
	}
}

func TestInstallHintNodeArch(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "pacman", LinuxDistro: "arch"}
	hint := installHintNode(profile)
	if !strings.Contains(hint, "pacman") || !strings.Contains(hint, "nodejs") {
		t.Fatalf("installHintNode(arch) = %q", hint)
	}
}

func TestInstallHintNodeFedora(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "dnf", LinuxDistro: LinuxDistroFedora}
	hint := installHintNode(profile)
	if !strings.Contains(hint, "rpm.nodesource.com") || !strings.Contains(hint, "dnf install -y nodejs") {
		t.Fatalf("installHintNode(fedora) = %q, want NodeSource LTS setup + dnf install", hint)
	}
}

func TestInstallHintBrew(t *testing.T) {
	hint := installHintBrew()
	if !strings.Contains(hint, "Homebrew") {
		t.Fatalf("installHintBrew() = %q, want Homebrew URL", hint)
	}
}

func TestInstallHintGoDarwin(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	hint := installHintGo(profile)
	if hint != "brew install go" {
		t.Fatalf("installHintGo(darwin) = %q", hint)
	}
}

func TestInstallHintGoUbuntu(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt", LinuxDistro: "ubuntu"}
	hint := installHintGo(profile)
	if !strings.Contains(hint, "apt-get install") {
		t.Fatalf("installHintGo(ubuntu) = %q", hint)
	}
}

func TestInstallCommandsForDepGitDarwin(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git darwin commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "brew" || cmds[0][1] != "install" || cmds[0][2] != "git" {
		t.Fatalf("git darwin command = %v", cmds[0])
	}
}

func TestInstallCommandsForDepGitUbuntu(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt", LinuxDistro: "ubuntu"}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git ubuntu commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "sudo" {
		t.Fatalf("git ubuntu command = %v, want sudo", cmds[0])
	}
}

func TestInstallCommandsForDepNodeUbuntuHasTwoSteps(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt", LinuxDistro: "ubuntu"}
	cmds := InstallCommandsForDep("node", profile)
	if len(cmds) != 2 {
		t.Fatalf("node ubuntu commands = %d, want 2 (nodesource setup + install)", len(cmds))
	}
}

func TestInstallCommandsForDepNodeFedoraHasTwoSteps(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "dnf", LinuxDistro: LinuxDistroFedora}
	cmds := InstallCommandsForDep("node", profile)
	if len(cmds) != 2 {
		t.Fatalf("node fedora commands = %d, want 2 (nodesource setup + install)", len(cmds))
	}
	if cmds[0][0] != "bash" || !strings.Contains(cmds[0][2], "rpm.nodesource.com/setup_lts.x") {
		t.Fatalf("node fedora step 1 = %v, want nodesource setup", cmds[0])
	}
	if cmds[1][0] != "sudo" || cmds[1][1] != "dnf" || cmds[1][4] != "nodejs" {
		t.Fatalf("node fedora step 2 = %v, want sudo dnf install -y nodejs", cmds[1])
	}
}

func TestInstallCommandsForDepNpmReturnsNil(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	cmds := InstallCommandsForDep("npm", profile)
	if cmds != nil {
		t.Fatalf("npm commands = %v, want nil (comes with node)", cmds)
	}
}

func TestInstallCommandsForDepBrewOnLinuxReturnsNil(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt"}
	cmds := InstallCommandsForDep("brew", profile)
	if cmds != nil {
		t.Fatalf("brew on linux = %v, want nil", cmds)
	}
}

func TestInstallCommandsForDepBrewOnDarwin(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	cmds := InstallCommandsForDep("brew", profile)
	if len(cmds) != 1 {
		t.Fatalf("brew darwin commands = %d, want 1", len(cmds))
	}
}

func TestInstallCommandsForDepUnknownReturnsNil(t *testing.T) {
	profile := PlatformProfile{OS: "darwin", PackageManager: "brew"}
	cmds := InstallCommandsForDep("unknown_tool", profile)
	if cmds != nil {
		t.Fatalf("unknown tool commands = %v, want nil", cmds)
	}
}

func TestInstallCommandsForDepGitArchUsesPacman(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "pacman", LinuxDistro: "arch"}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git arch commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "sudo" || cmds[0][1] != "pacman" {
		t.Fatalf("git arch command = %v, want sudo pacman", cmds[0])
	}
}

func TestInstallCommandsForDepGitFedoraUsesDnf(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "dnf", LinuxDistro: LinuxDistroFedora}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git fedora commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "sudo" || cmds[0][1] != "dnf" {
		t.Fatalf("git fedora command = %v, want sudo dnf", cmds[0])
	}
}

func TestFormatMissingDepsMessageAllPresent(t *testing.T) {
	report := DependencyReport{AllPresent: true}
	msg := FormatMissingDepsMessage(report)
	if !strings.Contains(msg, "All required dependencies are present") {
		t.Fatalf("unexpected message for all present: %q", msg)
	}
}

func TestFormatMissingDepsMessageWithMissing(t *testing.T) {
	report := DependencyReport{
		Dependencies: []Dependency{
			{Name: "node", Required: true, Installed: false, InstallHint: "brew install node"},
		},
		AllPresent:      false,
		MissingRequired: []string{"node"},
	}

	msg := FormatMissingDepsMessage(report)
	if !strings.Contains(msg, "node") || !strings.Contains(msg, "brew install node") {
		t.Fatalf("missing deps message = %q", msg)
	}
}

func TestInstallHintGitWindows(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	hint := installHintGit(profile)
	if hint != "winget install Git.Git" {
		t.Fatalf("installHintGit(windows) = %q", hint)
	}
}

func TestInstallHintNodeWindows(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	hint := installHintNode(profile)
	if hint != "winget install OpenJS.NodeJS.LTS" {
		t.Fatalf("installHintNode(windows) = %q", hint)
	}
}

func TestInstallHintGoWindows(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	hint := installHintGo(profile)
	if hint != "winget install GoLang.Go" {
		t.Fatalf("installHintGo(windows) = %q", hint)
	}
}

func TestInstallHintCurlWindows(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	hint := installHintCurl(profile)
	if !strings.Contains(hint, "pre-installed") {
		t.Fatalf("installHintCurl(windows) = %q, want pre-installed message", hint)
	}
}

func TestInstallCommandsForDepGitWindows(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git windows commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "winget" {
		t.Fatalf("git windows command = %v, want winget", cmds[0])
	}
}

func TestInstallCommandsForDepNodeWindows(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	cmds := InstallCommandsForDep("node", profile)
	if len(cmds) != 1 {
		t.Fatalf("node windows commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "winget" {
		t.Fatalf("node windows command = %v, want winget", cmds[0])
	}
}

func TestInstallCommandsForDepCurlWindowsReturnsNil(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	cmds := InstallCommandsForDep("curl", profile)
	if cmds != nil {
		t.Fatalf("curl on windows = %v, want nil (pre-installed)", cmds)
	}
}

func TestInstallCommandsForDepBrewOnWindowsReturnsNil(t *testing.T) {
	profile := PlatformProfile{OS: "windows", PackageManager: "winget"}
	cmds := InstallCommandsForDep("brew", profile)
	if cmds != nil {
		t.Fatalf("brew on windows = %v, want nil", cmds)
	}
}

func TestInstallCommandsFullMatrix(t *testing.T) {
	profiles := []PlatformProfile{
		{OS: "darwin", PackageManager: "brew"},
		{OS: "linux", PackageManager: "apt", LinuxDistro: "ubuntu"},
		{OS: "linux", PackageManager: "pacman", LinuxDistro: "arch"},
		{OS: "linux", PackageManager: "dnf", LinuxDistro: LinuxDistroFedora},
	}

	deps := []string{"git", "curl", "node", "go"}

	for _, profile := range profiles {
		for _, dep := range deps {
			t.Run(profile.OS+"/"+profile.PackageManager+"/"+dep, func(t *testing.T) {
				cmds := InstallCommandsForDep(dep, profile)
				if cmds == nil {
					t.Fatalf("InstallCommandsForDep(%q, %s/%s) = nil", dep, profile.OS, profile.PackageManager)
				}
				if len(cmds) == 0 {
					t.Fatalf("InstallCommandsForDep(%q) returned empty slice", dep)
				}
				for _, cmd := range cmds {
					if len(cmd) == 0 {
						t.Fatalf("empty command in sequence for %q", dep)
					}
				}
			})
		}
	}
}

// --- Tests for Nix install hints (R-NIX-004) ---

func TestInstallHintGitNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	hint := installHintGit(profile)
	if !strings.Contains(hint, "nix profile install") {
		t.Fatalf("installHintGit(nix flakes) = %q, want nix profile install", hint)
	}
}

func TestInstallHintGitNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	hint := installHintGit(profile)
	if !strings.Contains(hint, "nix-env -iA") {
		t.Fatalf("installHintGit(nix legacy) = %q, want nix-env -iA", hint)
	}
}

func TestInstallHintCurlNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	hint := installHintCurl(profile)
	if !strings.Contains(hint, "nix profile install") {
		t.Fatalf("installHintCurl(nix flakes) = %q, want nix profile install", hint)
	}
}

func TestInstallHintCurlNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	hint := installHintCurl(profile)
	if !strings.Contains(hint, "nix-env -iA") {
		t.Fatalf("installHintCurl(nix legacy) = %q, want nix-env -iA", hint)
	}
}

func TestInstallHintNodeNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	hint := installHintNode(profile)
	if !strings.Contains(hint, "nix profile install") || !strings.Contains(hint, "PATH") {
		t.Fatalf("installHintNode(nix flakes) = %q, want nix profile install with PATH warning", hint)
	}
}

func TestInstallHintNodeNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	hint := installHintNode(profile)
	if !strings.Contains(hint, "nix-env -iA") || !strings.Contains(hint, "PATH") {
		t.Fatalf("installHintNode(nix legacy) = %q, want nix-env -iA with PATH warning", hint)
	}
}

func TestInstallHintGoNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	hint := installHintGo(profile)
	if !strings.Contains(hint, "nix profile install") {
		t.Fatalf("installHintGo(nix flakes) = %q, want nix profile install", hint)
	}
}

func TestInstallHintGoNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	hint := installHintGo(profile)
	if !strings.Contains(hint, "nix-env -iA") {
		t.Fatalf("installHintGo(nix legacy) = %q, want nix-env -iA", hint)
	}
}

func TestInstallHintNix(t *testing.T) {
	hint := installHintNix()
	if !strings.Contains(hint, "nixos.org") || !strings.Contains(hint, "curl") {
		t.Fatalf("installHintNix() = %q, want curl installer from nixos.org", hint)
	}
}

// --- Tests for Nix install commands (R-NIX-005) ---

func TestInstallCommandsForDepGitNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git nix flakes commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix" || cmds[0][1] != "profile" || cmds[0][2] != "install" {
		t.Fatalf("git nix flakes command = %v, want nix profile install", cmds[0])
	}
}

func TestInstallCommandsForDepGitNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	cmds := InstallCommandsForDep("git", profile)
	if len(cmds) != 1 {
		t.Fatalf("git nix legacy commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix-env" || cmds[0][1] != "-iA" {
		t.Fatalf("git nix legacy command = %v, want nix-env -iA", cmds[0])
	}
}

func TestInstallCommandsForDepCurlNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	cmds := InstallCommandsForDep("curl", profile)
	if len(cmds) != 1 {
		t.Fatalf("curl nix flakes commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix" || cmds[0][1] != "profile" {
		t.Fatalf("curl nix flakes command = %v, want nix profile install", cmds[0])
	}
}

func TestInstallCommandsForDepNodeNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	cmds := InstallCommandsForDep("node", profile)
	if len(cmds) != 1 {
		t.Fatalf("node nix flakes commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix" || cmds[0][1] != "profile" {
		t.Fatalf("node nix flakes command = %v, want nix profile install", cmds[0])
	}
}

func TestInstallCommandsForDepNodeNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	cmds := InstallCommandsForDep("node", profile)
	if len(cmds) != 1 {
		t.Fatalf("node nix legacy commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix-env" || cmds[0][1] != "-iA" {
		t.Fatalf("node nix legacy command = %v, want nix-env -iA", cmds[0])
	}
}

func TestInstallCommandsForDepGoNixFlakes(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: true}
	cmds := InstallCommandsForDep("go", profile)
	if len(cmds) != 1 {
		t.Fatalf("go nix flakes commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix" || cmds[0][1] != "profile" {
		t.Fatalf("go nix flakes command = %v, want nix profile install", cmds[0])
	}
}

func TestInstallCommandsForDepGoNixLegacy(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "nix", NixFlakes: false}
	cmds := InstallCommandsForDep("go", profile)
	if len(cmds) != 1 {
		t.Fatalf("go nix legacy commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "nix-env" || cmds[0][1] != "-iA" {
		t.Fatalf("go nix legacy command = %v, want nix-env -iA", cmds[0])
	}
}

func TestInstallCommandsForDepNix(t *testing.T) {
	profile := PlatformProfile{OS: "linux", PackageManager: "apt"}
	cmds := InstallCommandsForDep("nix", profile)
	if len(cmds) != 1 {
		t.Fatalf("nix install commands = %d, want 1", len(cmds))
	}
	if cmds[0][0] != "bash" || !strings.Contains(cmds[0][2], "nixos.org/nix/install") {
		t.Fatalf("nix install command = %v, want bash curl installer", cmds[0])
	}
}
