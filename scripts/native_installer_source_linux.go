//go:build linux && amd64

package scripts

import "embed"

// This is the selected local source carrier, not a published release asset.
//
//go:embed native-installer-source-8983.zip
var nativeInstallerSource embed.FS

// ReadNativeInstallerSource returns independent bytes of the fixed local carrier.
func ReadNativeInstallerSource() ([]byte, error) {
	return nativeInstallerSource.ReadFile("native-installer-source-8983.zip")
}
