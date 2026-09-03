package installcommands

import "github.com/gentleman-programming/gentle-ai/v2/internal/system"

// NpmInstallCommands returns the display-only command for a global npm install.
// Android/Termux has no sudo privilege path, regardless of its package manager.
func NpmInstallCommands(profile system.PlatformProfile, packageName string) [][]string {
	commands := [][]string{{"npm", "install", "-g", "--ignore-scripts", packageName}}
	if profile.OS == "linux" && !profile.NpmWritable {
		commands[0] = append([]string{"sudo"}, commands[0]...)
	}
	return commands
}
