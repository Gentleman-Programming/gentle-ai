package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v3/internal/app"
)

// stubUpgradeSequence pins the two primitives of the upgrade->sync chain so the
// semantic rules of spec §2.3 run without a real binary upgrade or a real sync.
// syncCalls counts sync invocations so the skip rule can prove "sync no
// invocado".
func stubUpgradeSequence(
	t *testing.T,
	report app.UpgradeRunReport,
	reportErr error,
	syncResp *EcosystemActionResponse,
	syncErr error,
) *int {
	t.Helper()
	syncCalls := new(int)
	origReport := upgradeSequenceReportFn
	origSync := upgradeSequenceSyncFn
	t.Cleanup(func() {
		upgradeSequenceReportFn = origReport
		upgradeSequenceSyncFn = origSync
	})

	upgradeSequenceReportFn = func(context.Context, io.Writer) (app.UpgradeRunReport, error) {
		return report, reportErr
	}
	upgradeSequenceSyncFn = func(*Service) (*EcosystemActionResponse, error) {
		*syncCalls++
		return syncResp, syncErr
	}
	return syncCalls
}

func TestRunUpgradeSequence_BothPhasesSucceed(t *testing.T) {
	syncCalls := stubUpgradeSequence(t,
		app.UpgradeRunReport{Status: app.UpgradeStatusSucceeded, SelfToolName: "axiom"},
		nil,
		&EcosystemActionResponse{Success: true, Action: "sync", Message: "ok", Output: []string{"synced"}},
		nil,
	)

	svc := NewService(t.TempDir())
	resp, err := svc.RunUpgradeSequence()
	if err != nil {
		t.Fatalf("RunUpgradeSequence() error = %v", err)
	}
	if *syncCalls != 1 {
		t.Fatalf("sync invocations = %d, want 1", *syncCalls)
	}
	if !resp.Success {
		t.Errorf("success = false, want true")
	}
	if resp.Sequence != "upgrade->sync" {
		t.Errorf("sequence = %q, want upgrade->sync", resp.Sequence)
	}
	if resp.Phases == nil {
		t.Fatal("phases = nil, want consolidated report")
	}
	if resp.Phases.Upgrade.Status != "succeeded" {
		t.Errorf("phases.upgrade.status = %q, want succeeded", resp.Phases.Upgrade.Status)
	}
	if !resp.Phases.Sync.Executed {
		t.Errorf("phases.sync.executed = false, want true")
	}
	if resp.Phases.Sync.SkippedReason != "" {
		t.Errorf("phases.sync.skipped_reason = %q, want empty", resp.Phases.Sync.SkippedReason)
	}
	if !resp.Phases.Sync.Success {
		t.Errorf("phases.sync.success = false, want true")
	}
}

func TestRunUpgradeSequence_RestartRequiredSkipsSync(t *testing.T) {
	syncCalls := stubUpgradeSequence(t,
		app.UpgradeRunReport{
			Status:          app.UpgradeStatusSucceeded,
			SelfToolName:    "axiom",
			RestartRequired: true,
		},
		nil,
		nil, // sync must not be reached
		nil,
	)

	svc := NewService(t.TempDir())
	resp, err := svc.RunUpgradeSequence()
	if err != nil {
		t.Fatalf("RunUpgradeSequence() error = %v", err)
	}
	if *syncCalls != 0 {
		t.Fatalf("sync invocations = %d, want 0 (sync must not run after self-binary replacement)", *syncCalls)
	}
	if !resp.Phases.Upgrade.RestartRequired {
		t.Errorf("phases.upgrade.restart_required = false, want true")
	}
	if resp.Phases.Sync.Executed {
		t.Errorf("phases.sync.executed = true, want false")
	}
	if resp.Phases.Sync.SkippedReason != "restart-required" {
		t.Errorf("phases.sync.skipped_reason = %q, want restart-required", resp.Phases.Sync.SkippedReason)
	}
	// Rule §2.3.5: a restart-required skip is not a failure.
	if !resp.Success {
		t.Errorf("success = false, want true even with sync omitted for restart-required")
	}
}

func TestRunUpgradeSequence_UpgradeFailureSkipsSync(t *testing.T) {
	syncCalls := stubUpgradeSequence(t,
		app.UpgradeRunReport{Status: app.UpgradeStatusFailed},
		errors.New("upgrade failed for \"engram\": exit status 1"),
		nil,
		nil,
	)

	svc := NewService(t.TempDir())
	resp, err := svc.RunUpgradeSequence()
	if err != nil {
		t.Fatalf("RunUpgradeSequence() error = %v, want report (not 500)", err)
	}
	if *syncCalls != 0 {
		t.Fatalf("sync invocations = %d, want 0 (sync must not run after a fatal upgrade failure)", *syncCalls)
	}
	if resp.Phases.Upgrade.Status != "failed" {
		t.Errorf("phases.upgrade.status = %q, want failed", resp.Phases.Upgrade.Status)
	}
	if resp.Phases.Sync.Executed {
		t.Errorf("phases.sync.executed = true, want false")
	}
	if resp.Phases.Sync.SkippedReason != "upgrade-failed" {
		t.Errorf("phases.sync.skipped_reason = %q, want upgrade-failed", resp.Phases.Sync.SkippedReason)
	}
	if resp.Success {
		t.Errorf("success = true, want false when upgrade failed")
	}
}

func TestRunUpgradeSequence_SyncFailureKeepsUpgradeSuccess(t *testing.T) {
	syncCalls := stubUpgradeSequence(t,
		app.UpgradeRunReport{Status: app.UpgradeStatusSucceeded},
		nil,
		&EcosystemActionResponse{Success: false, Action: "sync", Error: "sync exploded"},
		nil,
	)

	svc := NewService(t.TempDir())
	resp, err := svc.RunUpgradeSequence()
	if err != nil {
		t.Fatalf("RunUpgradeSequence() error = %v", err)
	}
	if *syncCalls != 1 {
		t.Fatalf("sync invocations = %d, want 1", *syncCalls)
	}
	if !resp.Phases.Sync.Executed {
		t.Errorf("phases.sync.executed = false, want true")
	}
	if resp.Success {
		t.Errorf("success = true, want false when an executed phase failed")
	}
}

func TestRunUpgradeSequence_NoReportAtAllIsInternalServerError(t *testing.T) {
	// Empty report status + error means the service produced no report at all
	// (spec §2.4): that is the only 500 case.
	stubUpgradeSequence(t, app.UpgradeRunReport{}, errors.New("infrastructure failure"), nil, nil)

	svc := NewService(t.TempDir())
	resp, err := svc.RunUpgradeSequence()
	if err == nil {
		t.Fatal("RunUpgradeSequence() error = nil, want failure when no report can be produced")
	}
	if resp == nil || resp.Success {
		t.Fatalf("resp = %#v, want unsuccessful response", resp)
	}
}

func TestEcosystemUpgradeHTTPMethods(t *testing.T) {
	stubUpgradeSequence(t,
		app.UpgradeRunReport{Status: app.UpgradeStatusSucceeded},
		nil,
		&EcosystemActionResponse{Success: true, Action: "sync"},
		nil,
	)

	svc := NewService(t.TempDir())
	router := NewServer(svc).Router()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/ecosystem/upgrade", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/ecosystem/upgrade = %d, want 405", method, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ecosystem/upgrade", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/ecosystem/upgrade = %d, want 200", rr.Code)
	}

	var decoded EcosystemActionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if decoded.Sequence != "upgrade->sync" {
		t.Errorf("sequence = %q, want upgrade->sync", decoded.Sequence)
	}
	if decoded.Phases == nil {
		t.Fatal("phases = nil in JSON response")
	}
}

func TestEcosystemUpgradeSequenceDTOKeepsRequiredFields(t *testing.T) {
	// Required fields inside phases travel WITHOUT omitempty (D-07): consumers
	// must see them even at zero value.
	raw, err := json.Marshal(EcosystemPhases{
		Upgrade: UpgradePhaseReport{},
		Sync:    SyncPhaseReport{},
	})
	if err != nil {
		t.Fatalf("marshal phases: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal phases: %v", err)
	}

	for _, phase := range []string{"upgrade", "sync"} {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(decoded[phase], &fields); err != nil {
			t.Fatalf("unmarshal phases.%s: %v", phase, err)
		}
		required := []string{"success"}
		if phase == "upgrade" {
			required = append(required, "status", "restart_required", "manual_hint")
		} else {
			required = append(required, "executed", "skipped_reason")
		}
		for _, key := range required {
			if _, ok := fields[key]; !ok {
				t.Errorf("phases.%s missing required field %q: %s", phase, key, decoded[phase])
			}
		}
	}

	// Consumers that do not know phases must read exactly the previous document.
	legacy, err := json.Marshal(EcosystemActionResponse{
		Success: true,
		Action:  "sync",
		Message: "Sincronización completada exitosamente",
	})
	if err != nil {
		t.Fatalf("marshal legacy response: %v", err)
	}
	var legacyFields map[string]json.RawMessage
	if err := json.Unmarshal(legacy, &legacyFields); err != nil {
		t.Fatalf("unmarshal legacy response: %v", err)
	}
	for _, key := range []string{"sequence", "phases"} {
		if _, ok := legacyFields[key]; ok {
			t.Errorf("legacy response unexpectedly contains %q: %s", key, legacy)
		}
	}
}

func TestEcosystemSyncEndpointContractUnchanged(t *testing.T) {
	svc := NewService(t.TempDir())
	router := NewServer(svc).Router()

	req := httptest.NewRequest(http.MethodPost, "/api/ecosystem/sync", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Fatalf("POST /api/ecosystem/sync = %d, want 200 or 500", rr.Code)
	}
	if strings.Contains(rr.Body.String(), `"sequence"`) {
		t.Errorf("sync endpoint must not populate sequence: %s", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/ecosystem/sync", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/ecosystem/sync = %d, want 405", rr.Code)
	}
}
