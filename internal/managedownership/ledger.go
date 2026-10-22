// Package managedownership holds the hash ledger Gentle AI uses to prove it
// owns the exact bytes of a file it installed. It depends only on the standard
// library so any installer can share it without an import cycle.
package managedownership

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Version is the only ownership ledger version Read accepts.
const Version = 1

// Ledger maps a managed file name to the SHA-256 of the bytes Gentle AI
// installed under that name.
type Ledger struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

// Shape admits any caller-defined struct with the Ledger layout. Read decodes
// into the caller's own type, so JSON type errors keep naming that type.
type Shape interface {
	~struct {
		Version int               `json:"version"`
		Files   map[string]string `json:"files"`
	}
}

// Hash returns the lowercase hex SHA-256 recorded for installed bytes.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Read loads the ledger at path, accepting entries only for names. A missing
// ledger is not an error: it returns an empty ledger and false. Any other
// failure returns an empty ledger, false, and the error.
func Read[L Shape](path string, names []string) (L, bool, error) {
	ledger := L(Ledger{Version: Version, Files: make(map[string]string)})
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
	var parsed L
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ledger, false, fmt.Errorf("decode ownership ledger: %w", err)
	}
	fields := Ledger(parsed)
	if fields.Version != Version || fields.Files == nil {
		return ledger, false, fmt.Errorf("unsupported ownership ledger version or missing files")
	}
	allowed := make(map[string]bool, len(names))
	for _, name := range names {
		allowed[name] = true
	}
	for name, hash := range fields.Files {
		if !allowed[name] || len(hash) != 64 || strings.ToLower(hash) != hash {
			return ledger, false, fmt.Errorf("invalid ownership ledger entry %q", name)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return ledger, false, fmt.Errorf("invalid ownership hash for %q: %w", name, err)
		}
	}
	return parsed, true, nil
}
