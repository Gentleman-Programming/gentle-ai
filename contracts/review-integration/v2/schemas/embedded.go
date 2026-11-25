package reviewschemas

import _ "embed"

//go:embed intended-untracked-selection.schema.json
var intendedUntrackedSelection string

// IntendedUntrackedSelection returns the published intended-untracked selection schema.
func IntendedUntrackedSelection() string { return intendedUntrackedSelection }
