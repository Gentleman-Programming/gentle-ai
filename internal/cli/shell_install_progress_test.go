package cli

import (
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/shellinstaller"
)

// The Windows provisioner emits fixed stderr phase lines for its slow stock
// sub-phases. shellProgressWriter must surface each as a detail of the
// provisioning step (4) without disturbing the ordered step transitions.
func TestShellProgressWriterWindowsProvisionSubPhases(t *testing.T) {
	var reports []shellinstaller.UserInstallProgress
	log := &boundedWorkerLog{limit: 8192}
	w := &shellProgressWriter{log: log, report: func(p shellinstaller.UserInstallProgress) { reports = append(reports, p) }}
	subPhases := []string{"provision-lock", "provision-ci", "provision-copy", "provision-rebuild", "provision-compare", "provision-native"}
	for _, phase := range subPhases {
		if _, err := w.Write([]byte("Windows install phase: " + phase + "\n")); err != nil {
			t.Fatalf("write %s: %v", phase, err)
		}
	}
	for index, phase := range subPhases {
		got := reports[index]
		if got.Index != 4 || got.Total != 5 || got.State != "detail" || got.Detail != phase {
			t.Fatalf("sub-phase %s mapped to %+v, want provisioning detail", phase, got)
		}
	}
	if w.step != 2 {
		t.Fatalf("sub-phases alone must not advance the ordered step; step=%d", w.step)
	}
	reports = nil
	for _, line := range []string{"Windows install phase: provision-ready stock", "Windows install phase: publish"} {
		if _, err := w.Write([]byte(line + "\n")); err != nil {
			t.Fatalf("write %q: %v", line, err)
		}
	}
	want := []shellinstaller.UserInstallProgress{
		{Index: 4, Total: 5, State: "done"},
		{Index: 5, Total: 5, State: "running"},
		{Index: 5, Total: 5, State: "detail", Detail: "provision-ready stock"},
		{Index: 5, Total: 5, State: "detail", Detail: "publish"},
	}
	if len(reports) != len(want) {
		t.Fatalf("ready/publish produced %+v, want %v", reports, want)
	}
	for i, expected := range want {
		if reports[i] != expected {
			t.Fatalf("ready/publish report %d = %+v, want %+v", i, reports[i], expected)
		}
	}
	if !strings.Contains(log.String(), "Windows install phase: provision-copy") {
		t.Fatal("phase lines must reach the bounded worker log for failure diagnostics")
	}
	reports = nil
	if _, err := w.Write([]byte("unrelated worker noise\n")); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 0 {
		t.Fatalf("non-phase line produced reports: %+v", reports)
	}
}
