package commandcode

import "path/filepath"

// ConfigPath returns the path to ~/.commandcode, the Command Code global config
// directory.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".commandcode")
}
