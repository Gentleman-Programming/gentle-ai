package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/doctor"
	"github.com/gentleman-programming/gentle-ai/v4/internal/state"
)

func TestRunDoctor_StateErrorsPreserveState(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		readErr  error
		detail   string
		remedy   string
		remedyID doctor.RemedyID
		status   CheckStatus
	}{
		{name: "permission", payload: `{"installed_agents":["pi"]}`, readErr: os.ErrPermission, detail: "failed to read", remedy: "Inspect permissions and ownership", remedyID: "inspect-state-access", status: CheckStatusFail},
		{name: "other read error", payload: `{"installed_agents":["pi"]}`, readErr: fmt.Errorf("device read failure"), detail: "failed to read", remedy: "Inspect the file and parent directory", remedyID: "inspect-state-access", status: CheckStatusFail},
		{name: "invalid JSON", payload: "not-json", detail: "failed to parse", remedy: "Restore a valid backup or repair", remedyID: doctor.RemedyRepairState, status: CheckStatusFail},
		{name: "missing", readErr: os.ErrNotExist, detail: "state file not found", remedy: "Run 'gentle-ai install' to create initial state", remedyID: doctor.RemedyInstall, status: CheckStatusWarn},
		{name: "valid", payload: `{"installed_agents":[]}`, detail: "with no installed agents", remedy: "Run 'gentle-ai install' to configure agents", remedyID: doctor.RemedyInstall, status: CheckStatusWarn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			path := state.Path(home)
			if tt.payload != "" {
				if err := os.MkdirAll(home+"/.gentle-ai", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.payload), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			originalRead := doctorReadStateFn
			originalHome := osUserHomeDirDoctor
			originalLook := lookPathFn
			originalPaths := pathDirsFn
			originalBytes := availableBytesFn
			t.Cleanup(func() {
				doctorReadStateFn = originalRead
				osUserHomeDirDoctor = originalHome
				lookPathFn = originalLook
				pathDirsFn = originalPaths
				availableBytesFn = originalBytes
			})
			if tt.readErr != nil {
				doctorReadStateFn = func(string) (state.InstallState, error) {
					return state.InstallState{}, fmt.Errorf("read state: %w", &os.PathError{Op: "open", Path: path, Err: tt.readErr})
				}
			}
			osUserHomeDirDoctor = func() (string, error) { return home, nil }
			lookPathFn = func(string) (string, error) { return "", os.ErrNotExist }
			pathDirsFn = func() []string { return nil }
			availableBytesFn = func(string) (int64, error) { return 1024 * 1024 * 1024, nil }
			t.Setenv(engramHealthEnvVar, "")

			result := checkStateJSON(home)
			if result.Status != tt.status || !strings.Contains(result.Detail, tt.detail) {
				t.Errorf("state result = %+v, want %s / %q", result, tt.status, tt.detail)
			}
			if result.Remedy == nil || result.Remedy.ID != tt.remedyID || !strings.HasPrefix(result.Remedy.Description, tt.remedy) {
				t.Errorf("remedy = %+v, want %s / %q", result.Remedy, tt.remedyID, tt.remedy)
			}
			if tt.readErr != nil && tt.readErr != os.ErrNotExist {
				if !strings.Contains(result.Detail, path) || !strings.Contains(result.Detail, tt.readErr.Error()) {
					t.Errorf("lost read evidence: %s", result.Detail)
				}
				if result.Remedy != nil && (result.Remedy.Eligible || result.Remedy.ActionMode != doctor.ActionManualOnly) {
					t.Errorf("access remedy must remain manual: %+v", result.Remedy)
				}
			}
			var output bytes.Buffer
			if err := RunDoctor(context.Background(), &output); err != nil {
				t.Fatalf("RunDoctor: %v", err)
			}
			if !strings.Contains(output.String(), result.Detail) || (result.Remedy != nil && !strings.Contains(output.String(), result.Remedy.Description)) {
				t.Errorf("report missing state evidence/remedy:\n%s", output.String())
			}
			if strings.Contains(output.String(), "Delete or repair") {
				t.Errorf("destructive advice:\n%s", output.String())
			}
			after, err := os.ReadFile(path)
			if tt.payload == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("doctor created state: %s, %v", after, err)
				}
			} else if err != nil || string(after) != tt.payload {
				t.Fatalf("doctor changed state: %q, %v", after, err)
			}
		})
	}
}
