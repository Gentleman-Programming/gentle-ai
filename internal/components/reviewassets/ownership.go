package reviewassets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OwnershipLedgerFilename identifies the native rendered-agent ownership record.
const OwnershipLedgerFilename = ".gentle-ai-native-agent-ownership.json"

const ownershipVersion = 1

type ownershipLedger struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

func installedHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readOwnership(path string, names []string) (ownershipLedger, bool, error) {
	ledger := ownershipLedger{Version: ownershipVersion, Files: make(map[string]string)}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ledger, false, nil
	}
	if err != nil {
		return ledger, false, fmt.Errorf("stat ownership ledger: %w", err)
	}
	if !info.Mode().IsRegular() {
		return ledger, false, fmt.Errorf("ownership ledger is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ledger, false, fmt.Errorf("read ownership ledger: %w", err)
	}
	parsed := ownershipLedger{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ledger, false, fmt.Errorf("decode ownership ledger: %w", err)
	}
	if parsed.Version != ownershipVersion || parsed.Files == nil {
		return ledger, false, fmt.Errorf("unsupported ownership ledger version or missing files")
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for name, hash := range parsed.Files {
		if !allowed[name] || len(hash) != 64 || strings.ToLower(hash) != hash {
			return ledger, false, fmt.Errorf("invalid ownership ledger entry %q", name)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return ledger, false, fmt.Errorf("invalid ownership hash for %q: %w", name, err)
		}
	}
	return parsed, true, nil
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
