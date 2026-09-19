package telemetryruntime

import (
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
	"github.com/gentleman-programming/gentle-ai/v2/internal/opencode"
)

func TestManagedV2SelectionAndProvenance(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := ReconcileForMajorWithRollback(dir, opencode.RuntimeUnknown); err == nil {
		t.Fatal("unknown selected")
	}
	if _, err := Reconcile(dir); err != nil {
		t.Fatal(err)
	}
	_, rollback, err := ReconcileForMajorWithRollback(dir, opencode.RuntimeV2)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(ManagedPaths(dir)[0])
	if string(data) != assets.MustRead("opencode/plugins-v2/telemetry-runtime.ts") {
		t.Fatal("wrong major")
	}
	if err := CheckManaged(dir); err != nil {
		t.Fatal(err)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(ManagedPaths(dir)[0])
	if !strings.Contains(string(data), "telemetry-runtime/v1") {
		t.Fatal("rollback changed major")
	}
	if _, _, err := ReconcileForMajorWithRollback(dir, opencode.RuntimeV2); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(ManagedPaths(dir)[0], []byte("custom"), 0644)
	if _, _, err := ReconcileForMajorWithRollback(dir, opencode.RuntimeV2); err == nil {
		t.Fatal("custom bytes adopted")
	}
}
