//go:build windows

package scripts

import (
	"embed"
	"errors"
)

// Stock Windows composition reuses the bounded portable SRI completion helper.
//
//go:embed provision-gentle-shell-windows.mjs complete-generated-lock-sri.mjs
var windowsUserHelpers embed.FS

func ReadWindowsUserHelper() ([]byte, error) {
	return windowsUserHelpers.ReadFile("provision-gentle-shell-windows.mjs")
}

func ReadPrivateHelper(name string) ([]byte, error) {
	if name != "complete-generated-lock-sri.mjs" {
		return nil, errors.New("unsupported Windows private helper")
	}
	return windowsUserHelpers.ReadFile(name)
}
