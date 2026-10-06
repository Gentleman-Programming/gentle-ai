package reviewassets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v4/internal/managedownership"
)

// OwnershipLedgerFilename identifies the native rendered-agent ownership record.
const OwnershipLedgerFilename = ".gentle-ai-native-agent-ownership.json"

// The shared managed-ownership ledger reads and validates the native-agent
// ledger; these wrappers keep the installer reading as it did before the move.
const ownershipVersion = managedownership.Version

// ownershipLedger stays a reviewassets type so JSON decode errors keep naming it.
type ownershipLedger struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

func installedHash(data []byte) string { return managedownership.Hash(data) }

func readOwnership(path string, names []string) (ownershipLedger, bool, error) {
	return managedownership.Read[ownershipLedger](path, names)
}

func nativeFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("stat native agent %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("native agent is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read native agent %s: %w", path, err)
	}
	return data, true, nil
}

func ledgerPath(dir string) string { return filepath.Join(dir, OwnershipLedgerFilename) }
