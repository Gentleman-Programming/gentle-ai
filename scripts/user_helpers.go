package scripts

import "embed"

// The provisioner and authenticated locks are distinct from the runtime helpers.
//
//go:embed provision-gentle-shell-private-global.mjs user-global-graph.mjs user-locks/modern/package-lock.json user-locks/prior/package-lock.json
var userHelpers embed.FS

func ReadUserHelper() ([]byte, error) {
	return userHelpers.ReadFile("provision-gentle-shell-private-global.mjs")
}

// ReadUserAssets returns the provisioner, graph projection and acquisition locks.
// Runtime paths stay distinct from the bootstrap-helper inventory.
func ReadUserAssets() (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range []string{"provision-gentle-shell-private-global.mjs", "user-global-graph.mjs", "user-locks/modern/package-lock.json", "user-locks/prior/package-lock.json"} {
		data, err := userHelpers.ReadFile(name)
		if err != nil {
			return nil, err
		}
		if name == "provision-gentle-shell-private-global.mjs" {
			name = "provision.mjs"
		}
		files[name] = data
	}
	return files, nil
}
