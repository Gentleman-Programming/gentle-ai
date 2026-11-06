package scripts

import "embed"

// The provisioner and authenticated locks are distinct from the runtime helpers.
// gentle-pi-suppliers.json is the reviewed gentle-pi release list (S11): the
// only copy in the repository, embedded on every OS, read live from the
// repository main branch at check/update time with this copy as fallback. The
// Stable install no longer provisions from user-locks/modern (S19): the
// modern lock remains embedded as the legacy-installation verification
// authority and the prior lock serves the retained prior cohort.
//
//go:embed provision-gentle-shell-private-global.mjs user-global-graph.mjs user-main-bindings.mjs user-locks/modern/package-lock.json user-locks/prior/package-lock.json gentle-pi-suppliers.json
var userHelpers embed.FS

// ReadReviewedSuppliers returns the embedded reviewed gentle-pi supplier list.
func ReadReviewedSuppliers() ([]byte, error) {
	return userHelpers.ReadFile("gentle-pi-suppliers.json")
}

func ReadUserHelper() ([]byte, error) {
	return userHelpers.ReadFile("provision-gentle-shell-private-global.mjs")
}

// ReadUserAssets returns the provisioner, graph projection and acquisition locks.
// Runtime paths stay distinct from the bootstrap-helper inventory.
func ReadUserAssets() (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range []string{"provision-gentle-shell-private-global.mjs", "user-global-graph.mjs", "user-main-bindings.mjs", "user-locks/modern/package-lock.json", "user-locks/prior/package-lock.json"} {
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
