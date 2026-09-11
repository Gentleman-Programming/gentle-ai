package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/sddstatus"
)

func TestSDDStatusDoesNotPrepareConsentMarker(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedSDDStatusReadyChange(t, root, "marker-status", "- [ ] 1.1 Update `../source/main.go`\n")
	writeSDDStatusFile(t, filepath.Join(root, "source", "main.go"), "package source\n")

	var stdout bytes.Buffer
	if err := RunSDDStatus([]string{"marker-status", "--cwd", root, "--json"}, &stdout); err != nil {
		t.Fatalf("RunSDDStatus() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(changeRoot, ".gentle-ai-instance")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status prepared a marker: stat error = %v", err)
	}
	var status sddstatus.Status
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("decode status JSON: %v\n%s", err, stdout.String())
	}
	if status.Consent != nil {
		t.Fatalf("read-only status emitted usable consent: %#v", status.Consent)
	}
	if !strings.Contains(strings.Join(status.BlockedReasons, "\n"), `sdd-continue "marker-status"`) {
		t.Fatalf("status did not name explicit preparation: %v", status.BlockedReasons)
	}
}

func TestSDDContinuePlanningMarkerScopePreparesWhileApplyStaysBlocked(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedSDDStatusReadyChange(t, root, "marker-continue", "- [ ] 1.1 Update `../source/main.go`\n")
	writeSDDStatusFile(t, filepath.Join(root, "source", "main.go"), "package source\n")

	var stdout bytes.Buffer
	if err := RunSDDContinue([]string{"marker-continue", "--cwd", root, "--json"}, &stdout); err != nil {
		t.Fatalf("RunSDDContinue() error = %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(changeRoot, ".gentle-ai-instance"))
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(marker)), "sdd-") {
		t.Fatalf("continue did not prepare a marker: %q, %v", marker, err)
	}
	var status sddstatus.Status
	if err := json.Unmarshal(stdout.Bytes(), &status); err != nil {
		t.Fatalf("decode continue JSON: %v\n%s", err, stdout.String())
	}
	if status.Consent == nil || status.ApplyState != sddstatus.ApplyBlocked || status.NextRecommended == "apply" {
		t.Fatalf("continue status = consent %#v apply %q next %q, want consent and blocked apply", status.Consent, status.ApplyState, status.NextRecommended)
	}
}

func TestSDDContinueRefusesMarkerFilesystemFailure(t *testing.T) {
	root := t.TempDir()
	changeRoot := seedSDDStatusReadyChange(t, root, "marker-read-only", "- [ ] 1.1 Update `../source/main.go`\n")
	writeSDDStatusFile(t, filepath.Join(root, "source", "main.go"), "package source\n")
	markerPath := filepath.Join(changeRoot, ".gentle-ai-instance")
	if err := os.Mkdir(markerPath, 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	err := RunSDDContinue([]string{"marker-read-only", "--cwd", root, "--json"}, &stdout)
	if err == nil {
		t.Fatal("RunSDDContinue() error = nil, want marker filesystem refusal")
	}
	if stdout.Len() != 0 {
		t.Fatalf("continue published output after marker filesystem refusal: %s", stdout.String())
	}
	if info, statErr := os.Stat(markerPath); statErr != nil || !info.IsDir() {
		t.Fatalf("marker directory was changed: info=%#v err=%v", info, statErr)
	}
}

func consentCLISnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			snapshot[path] = "directory"
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[path] = string(payload)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestSDDCLIRepeatedStatusAndPreparationPreserveAuthority(t *testing.T) {
	repo := initReviewCLIRepo(t)
	outside := t.TempDir()
	root := seedSDDStatusReadyChange(t, repo, "snapshots", "- [ ] Update `"+outside+"/main.go`\n")
	args := []string{"snapshots", "--cwd", repo, "--json"}
	initial := consentCLISnapshot(t, repo)
	for _, present := range []bool{false, true} {
		if present {
			var output bytes.Buffer
			if err := RunSDDContinue(args, &output); err != nil {
				t.Fatal(err)
			}
		}
		before := consentCLISnapshot(t, repo)
		var previous []byte
		for i := 0; i < 3; i++ {
			var output bytes.Buffer
			if err := RunSDDStatus(args, &output); err != nil {
				t.Fatal(err)
			}
			var status sddstatus.Status
			if err := json.Unmarshal(output.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			if (status.Consent != nil) != present || status.ApplyState != sddstatus.ApplyBlocked || !reflect.DeepEqual(status.ActionContext.AllowedEditRoots, []string{repo}) {
				t.Fatalf("unexpected status: %#v", status)
			}
			if i > 0 && !bytes.Equal(previous, output.Bytes()) {
				t.Fatal("repeated status output drifted")
			}
			previous = append([]byte(nil), output.Bytes()...)
		}
		if !reflect.DeepEqual(before, consentCLISnapshot(t, repo)) {
			t.Fatal("status mutated marker/artifact/authority bytes")
		}
		if present {
			delete(before, filepath.Join(root, ".gentle-ai-instance"))
			if !reflect.DeepEqual(before, initial) {
				t.Fatal("preparation changed more than marker")
			}
		}
	}
}
