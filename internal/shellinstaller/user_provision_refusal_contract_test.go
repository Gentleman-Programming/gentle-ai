package shellinstaller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The staged Windows provision helper refuses a package whose acquired
// package.json identity differs from the lock record. A refusal that names
// only "acquired identity" gives the operator neither the expected nor the
// acquired values and cannot be acted on; this contract keeps that refusal
// carrying the relative path, the expected name@version and the acquired
// name@version, exactly as the installer's main-overlay philosophy requires
// ("any refusal names the exact expected and staged values").
func TestWindowsUserHelperAcquiredIdentityRefusalNamesValues(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "provision-gentle-shell-windows.mjs"))
	if err != nil {
		t.Fatalf("read provision helper: %v", err)
	}
	line := ""
	for _, current := range strings.Split(string(data), "\n") {
		if strings.Contains(current, "acquired identity") {
			line = current
			break
		}
	}
	if line == "" {
		t.Fatal("provision helper has no acquired-identity refusal")
	}
	for _, want := range []string{
		"reject(`acquired identity: ${relative}",
		"expected ${name}@${record.version}",
		"acquired ${identity.name}@${identity.version}`)",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("acquired-identity refusal is missing %q: %s", want, strings.TrimSpace(line))
		}
	}
}
