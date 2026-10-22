//go:build linux && amd64

package shellinstaller

import (
	"errors"
	"os"
	"path/filepath"
)

// Preserve only the already-bounded preparation failure output. Raw child
// diagnostics can be sensitive: never add them to errors or terminal output.
func userNativePrepareDiagnostic(root, operation, output string, cause error) error {
	if operation != "prepare" || output == "" || cause == nil {
		return cause
	}
	data := []byte(output)
	if len(data) > 4096 {
		data = data[len(data)-4096:]
	}
	file, err := os.OpenFile(filepath.Join(root, "native-prepare.stderr"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.Join(cause, err)
	}
	_, writeErr := file.Write(data)
	return errors.Join(cause, writeErr, file.Sync(), file.Close())
}
