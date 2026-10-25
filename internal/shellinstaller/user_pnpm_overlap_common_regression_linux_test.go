//go:build linux && amd64 && shell_pnpm_publication

package shellinstaller

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

const independentOverlapSource = "/fixture/install-parent/.gentle-native-4277763020/producer"

func independentOverlapLocation(t *testing.T) NativeInstallationLocation {
	t.Helper()
	_, loc, err := userReadNativeReceipt(context.Background(), independentOverlapSource)
	if err != nil || loc == nil {
		t.Fatal(err)
	}
	return *loc
}
func independentOverlapHash(t *testing.T) string {
	t.Helper()
	m, err := userPNPMPayloadInventory(context.Background(), independentOverlapSource, userPNPMRoots)
	if err != nil || len(m) != 84345 {
		t.Fatal("source inventory", len(m), err)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func independentOverlapReceipt(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile(independentOverlapSource + "/native-installation.json")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
