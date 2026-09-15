package sddstatus

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewtransaction"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/assets"
)

const freshV2RerunInstruction = "Start a fresh implementation state and rerun `gentle-ai sdd-status --contract gentle-ai.sdd-status/v2`."

func TestSDDStatusV2CleanBreak(t *testing.T) {
	t.Run("v2 is the sole default and v1 is refused read-only", func(t *testing.T) {
		if SchemaVersion != 2 {
			t.Fatalf("SchemaVersion = %d, want 2", SchemaVersion)
		}

		defaultArgs, err := ParseCommandArgs([]string{"thin"})
		if err != nil {
			t.Fatalf("ParseCommandArgs(default) error = %v", err)
		}
		if defaultArgs.Contract != "gentle-ai.sdd-status/v2" {
			t.Fatalf("default contract = %q, want gentle-ai.sdd-status/v2", defaultArgs.Contract)
		}

		v2Args, err := ParseCommandArgs([]string{"thin", "--contract", "gentle-ai.sdd-status/v2"})
		if err != nil {
			t.Fatalf("ParseCommandArgs(v2) error = %v", err)
		}
		if v2Args.Contract != "gentle-ai.sdd-status/v2" {
			t.Fatalf("v2 contract = %q, want gentle-ai.sdd-status/v2", v2Args.Contract)
		}

		_, err = ParseCommandArgs([]string{"thin", "--contract", "gentle-ai.sdd-status/v1"})
		if err == nil || err.Error() != "unsupported sdd-status contract \"gentle-ai.sdd-status/v1\". "+freshV2RerunInstruction {
			t.Fatalf("v1 refusal = %v, want one fresh-v2 rerun instruction", err)
		}
		afterRefusalDefaultArgs, err := ParseCommandArgs([]string{"thin"})
		if err != nil {
			t.Fatalf("ParseCommandArgs(default after v1 refusal) error = %v", err)
		}
		if !reflect.DeepEqual(afterRefusalDefaultArgs, defaultArgs) {
			t.Fatalf("v1 refusal changed default command parsing result: before=%#v after=%#v", defaultArgs, afterRefusalDefaultArgs)
		}
	})

	t.Run("tagged test corpus has no retired public v1 status pins", func(t *testing.T) {
		files, err := taggedStatusTestFiles()
		if err != nil {
			t.Fatalf("taggedStatusTestFiles() error = %v", err)
		}
		for _, file := range files {
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", file, err)
			}
			if pins := retiredV1StatusTestPins(string(content)); len(pins) > 0 {
				t.Errorf("build-tagged test %s retains retired public v1 status pins: %s", file, strings.Join(pins, ", "))
			}
		}
	})

	t.Run("projection has the exact v2 authority-free key sets", func(t *testing.T) {
		status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, nil, nil, "apply", nil)
		projected, err := ProjectStatusV2(status)
		if err != nil {
			t.Fatalf("v2 projection error = %v", err)
		}
		payload, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		assertExactJSONKeys(t, document, []string{
			"schemaName", "schemaVersion", "changeName", "artifactStore", "planningHome", "changeRoot",
			"artifactPaths", "contextFiles", "artifacts", "taskProgress", "dependencies", "applyState",
			"actionContext", "relationships", "nextRecommended", "blockedReasons",
			// notes is #4372's deliberate additive extension: the non-blocking
			// diagnostics channel that keeps `blockedReasons` a pure gate.
			"notes",
		})
		assertJSONNestedKeys(t, document, "artifactPaths", []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"})
		assertJSONNestedKeys(t, document, "contextFiles", []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"})
		assertJSONNestedKeys(t, document, "artifacts", []string{"proposal", "specs", "design", "tasks", "applyProgress", "verifyReport"})
		for _, forbidden := range []string{"remediationState", "reviewGate", "reviewTransaction", "reVerify", "runtimeStatus", "reviewPolicy", "reviewLedger", "reviewReceipt", "reviewBundle", "reviewContext", "reviewState", "lineageId", "generation", "fixBatch", "correctionBudget"} {
			if strings.Contains(string(payload), forbidden) {
				t.Fatalf("v2 projection retained authority key %q: %s", forbidden, payload)
			}
		}
	})

	t.Run("v2 preserves seven dependencies, three instruction groups, and opaque consent", func(t *testing.T) {
		change := "thin"
		status := baseStatus(ArtifactStoreOpenSpec, "/repo", nil, &change, nil, "apply", nil)
		instructions := renderPhaseInstructions(status)
		status.PhaseInstructions = &instructions
		consent := newEditAuthorityConsent(change, "/repo", []string{"/repo/internal"}, "sdd-opaque", "")
		status.Consent = &consent

		projected, err := ProjectStatusV2(status)
		if err != nil {
			t.Fatalf("ProjectStatusV2() error = %v", err)
		}
		payload, err := json.Marshal(projected)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatal(err)
		}
		assertJSONNestedKeys(t, document, "dependencies", []string{"proposal", "specs", "design", "tasks", "apply", "verify", "archive"})
		assertJSONNestedKeys(t, document, "phaseInstructions", []string{"apply", "verify", "archive"})
		if !bytes.Contains(payload, []byte("sdd-opaque")) {
			t.Fatalf("v2 projection lost the present opaque consent marker: %s", payload)
		}
		if _, ok := document["consent"]; !ok {
			t.Fatalf("v2 projection omitted present consent: %s", payload)
		}
	})

	t.Run("verified archive excludes review state and status reads preserve history", func(t *testing.T) {
		reviewEnabledHome(t)
		repo := initRuntimeLedgerRepo(t)
		changeRoot := seedReadyChange(t, repo, "thin", "- [x] 1.1 Work\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))
		write(t, filepath.Join(changeRoot, "reviews", "transaction.json"), "{\"retired\":true}\n")
		before := snapshotStatusReadTree(t, repo)
		for range 2 {
			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin"})
			if err != nil {
				t.Fatal(err)
			}
			if status.Dependencies.Archive != DependencyReady || status.NextRecommended != "archive" {
				t.Fatalf("archive=%q next=%q, want ready/archive", status.Dependencies.Archive, status.NextRecommended)
			}
			projection, err := ProjectStatusV2(status)
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"reviewOffer", "remediationState", "reviewGate", "reviewTransaction", "reVerify", "runtimeStatus"} {
				if strings.Contains(string(payload), `"`+key+`"`) {
					t.Errorf("SDD projection retained %q", key)
				}
			}
		}
		if after := snapshotStatusReadTree(t, repo); after != before {
			t.Fatal("status reads changed existing artifacts or authority")
		}
	})

	t.Run("unfinished tasks remain visible when explicit archive is available", func(t *testing.T) {
		repo := initRuntimeLedgerRepo(t)
		changeRoot := seedReadyChange(t, repo, "thin", "- [x] 1.1 Work\n- [ ] 1.2 Remaining\n")
		write(t, filepath.Join(changeRoot, "verify-report.md"), testVerifyEnvelope("pass", 0, 0, "1/1", "1/1", 0, 0))

		status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "thin"})
		if err != nil {
			t.Fatal(err)
		}
		if status.Dependencies.Archive != DependencyReady || status.NextRecommended != "apply" || status.TaskProgress.AllComplete {
			t.Fatalf("archive=%q next=%q, want optional archive while unfinished tasks still recommend apply", status.Dependencies.Archive, status.NextRecommended)
		}
	})

	t.Run("shipped assets and goldens no longer pin v1", func(t *testing.T) {
		contract := assets.MustRead("skills/_shared/sdd-status-contract.md")
		if strings.Contains(contract, "sdd-status/v1") || strings.Contains(contract, "Native status v1") {
			t.Fatalf("active status asset still pins v1")
		}
		if !strings.Contains(contract, "sdd-status/v2") {
			t.Fatal("active status asset does not advertise v2")
		}
		if golden := mustReadStatusGolden(t, "sdd-claude-cmd-gentle-sdd-status.golden"); strings.Contains(golden, "sdd-status/v1") || strings.Contains(golden, "Native status v1") {
			t.Fatal("generated status golden still pins v1")
		}
	})
}

func taggedStatusTestFiles() ([]string, error) {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		return nil, err
	}
	var tagged []string
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if hasGoBuildConstraint(string(content)) {
			tagged = append(tagged, file)
		}
	}
	sort.Strings(tagged)
	return tagged, nil
}

func hasGoBuildConstraint(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "//go:build "), strings.HasPrefix(line, "// +build "):
			return true
		case strings.HasPrefix(line, "//"):
			continue
		default:
			return false
		}
	}
	return false
}

func retiredV1StatusTestPins(content string) []string {
	pins := make([]string, 0, 3)
	if strings.Contains(content, "ProjectStatusV1") {
		pins = append(pins, "ProjectStatusV1")
		if strings.Contains(content, "ReviewGate") {
			pins = append(pins, "ReviewGate status projection")
		}
	}
	if strings.Contains(content, "gentle-ai.sdd-status/v1") {
		pins = append(pins, "contract v1")
	}
	return pins
}

func snapshotStatusReadTree(t *testing.T, root string) string {
	t.Helper()
	var entries []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "." {
			return nil
		}
		// Git may update administrative files while status resolves repository
		// context. The read-only status contract covers planning artifacts and
		// authority, not Git's platform-specific bookkeeping.
		if relative == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			entries = append(entries, "dir:"+relative)
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		entries = append(entries, fmt.Sprintf("file:%s:%x", relative, digest))
		return nil
	}); err != nil {
		t.Fatalf("snapshotStatusReadTree(%s): %v", root, err)
	}
	sort.Strings(entries)
	return strings.Join(entries, "\n")
}

func assertExactJSONKeys(t *testing.T, document map[string]json.RawMessage, expected []string) {
	t.Helper()
	want := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		want[key] = struct{}{}
	}
	for key := range document {
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected JSON key %q", key)
		}
		delete(want, key)
	}
	for key := range want {
		t.Fatalf("missing JSON key %q", key)
	}
}

func assertJSONNestedKeys(t *testing.T, document map[string]json.RawMessage, key string, expected []string) {
	t.Helper()
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(document[key], &nested); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	assertExactJSONKeys(t, nested, expected)
}

func mustReadStatusGolden(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestConsentPreparationRefusesMalformedOrEscapedMarker(t *testing.T) {
	for _, scenario := range []string{"empty", "partial", "escaped-change", "escaped-planning", "marker-symlink"} {
		t.Run(scenario, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			outside := t.TempDir()
			root := seedReadyChange(t, repo, "consent-negative", "- [ ] Update `"+outside+"/main.go`\n")
			marker := filepath.Join(root, changeInstanceMarkerFile)
			switch scenario {
			case "empty":
				write(t, marker, "")
			case "partial":
				write(t, marker, "sdd-")
			case "marker-symlink":
				write(t, filepath.Join(outside, "marker"), "sdd-"+strings.Repeat("a", 32)+"\n")
				if err := os.Symlink(filepath.Join(outside, "marker"), marker); err != nil {
					t.Fatal(err)
				}
			case "escaped-change", "escaped-planning":
				path := root
				if scenario == "escaped-planning" {
					path = filepath.Join(repo, "openspec")
				}
				moved := filepath.Join(outside, "moved")
				if err := os.Rename(path, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, path); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshotStatusReadTree(t, outside)
			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "consent-negative"})
			if scenario == "empty" && err == nil {
				t.Error("empty persisted marker was accepted by status")
			}
			if err == nil {
				err = PrepareChangeInstanceConsent(status)
			}
			if err == nil && (status.ChangeRoot != nil || status.Consent != nil || status.ApplyState != ApplyBlocked) {
				t.Fatal("unsafe preparation succeeded")
			}
			if after := snapshotStatusReadTree(t, outside); after != before {
				t.Fatal("unsafe preparation wrote outside planning")
			}
		})
	}
}

func TestConsentPreparationConcurrentWinnerAndReadOnlySnapshots(t *testing.T) {
	repo := initRuntimeLedgerRepo(t)
	outside := t.TempDir()
	root := seedReadyChange(t, repo, "winner", "- [ ] Update `"+outside+"/main.go`\n")
	options := ResolveOptions{CWD: repo, ChangeName: "winner"}
	before := snapshotStatusReadTree(t, repo)
	status, err := Resolve(options)
	if err != nil {
		t.Fatal(err)
	}
	if status.Consent != nil {
		t.Fatal("absent marker emitted consent")
	}
	for i := 0; i < 3; i++ {
		if _, err := Resolve(options); err != nil {
			t.Fatal(err)
		}
	}
	if snapshotStatusReadTree(t, repo) != before {
		t.Fatal("absent-marker status changed artifacts or authority")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- PrepareChangeInstanceConsent(status) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	marker, err := readChangeInstanceMarker(root)
	if err != nil || marker == "" {
		t.Fatalf("winner %q: %v", marker, err)
	}
	afterPreparation := snapshotStatusReadTree(t, repo)
	var unchangedEntries []string
	for _, entry := range strings.Split(afterPreparation, "\n") {
		if !strings.HasPrefix(entry, "file:openspec/changes/winner/"+changeInstanceMarkerFile+":") {
			unchangedEntries = append(unchangedEntries, entry)
		}
	}
	if strings.Join(unchangedEntries, "\n") != before {
		t.Fatal("initial preparation changed more than its marker")
	}
	for i := 0; i < 3; i++ {
		current, err := Resolve(options)
		if err != nil {
			t.Fatal(err)
		}
		if current.Consent == nil || !strings.Contains(current.Consent.Choices[0].Invocation, marker) {
			t.Fatal("consent did not bind persisted winner")
		}
		if current.ApplyState != ApplyBlocked || !reflect.DeepEqual(current.ActionContext.AllowedEditRoots, status.ActionContext.AllowedEditRoots) {
			t.Fatal("preparation granted source roots")
		}
		if err := PrepareChangeInstanceConsent(current); err != nil {
			t.Fatal(err)
		}
	}
	if snapshotStatusReadTree(t, repo) != afterPreparation {
		t.Fatal("present-marker status/reuse changed marker, artifacts or authority")
	}
	if afterPreparation == before {
		t.Fatal("preparation did not persist winner")
	}
	store := mustRuntimeStore(t, repo, "winner")
	ledger, err := store.Status()
	if err != nil || ledger.Revision != "" || len(ledger.GrantedRoots) != 0 {
		t.Fatalf("preparation changed authority: %#v %v", ledger, err)
	}
}

func TestConsentPublicationFailuresEmitNoIdentity(t *testing.T) {
	for _, scenario := range []string{"before-write-error", "publish-error", "missing-readback", "empty-readback", "partial-readback", "unreadable-readback", "replacement"} {
		t.Run(scenario, func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			outside := t.TempDir()
			root := seedReadyChange(t, repo, "publication", "- [ ] Update `"+outside+"/main.go`\n")
			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "publication"})
			if err != nil {
				t.Fatal(err)
			}
			original := publishChangeInstanceMarker
			t.Cleanup(func() { publishChangeInstanceMarker = original })
			publishChangeInstanceMarker = func(source, destination string) error {
				switch scenario {
				case "before-write-error":
					return errors.New("injected publication failure")
				case "missing-readback":
					return nil
				case "publish-error":
					if err := reviewtransaction.PublishFileNoReplace(source, destination); err != nil {
						return err
					}
					return errors.New("injected uncertain publication")
				case "empty-readback":
					write(t, destination, "")
				case "partial-readback":
					write(t, destination, "sdd-")
				case "unreadable-readback":
					return os.Mkdir(destination, 0755)
				case "replacement":
					// Move the observed change root out of the active changes tree before
					// creating its replacement. This keeps the identity-race fixture
					// independent of Windows' handling of a renamed current-root sibling.
					movedRoot := filepath.Join(t.TempDir(), "publication-old")
					if err := os.Rename(root, movedRoot); err != nil {
						return err
					}
					if err := os.Mkdir(root, 0755); err != nil {
						return err
					}
					write(t, destination, "sdd-"+strings.Repeat("b", 32)+"\n")
				}
				return nil
			}
			if err := PrepareChangeInstanceConsent(status); err == nil {
				t.Fatal("uncertain publication accepted for consent")
			}
			if status.Consent != nil {
				t.Fatal("failure emitted consent")
			}
		})
	}
}

func TestConsentPreparationUnwritableAndUnreadable(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		t.Run(fmt.Sprint(unreadable), func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows does not enforce POSIX chmod access restrictions")
			}
			repo := initRuntimeLedgerRepo(t)
			outside := t.TempDir()
			root := seedReadyChange(t, repo, "permissions", "- [ ] Update `"+outside+"/main.go`\n")
			status, err := Resolve(ResolveOptions{CWD: repo, ChangeName: "permissions"})
			if err != nil {
				t.Fatal(err)
			}
			path, mode := root, os.FileMode(0500)
			if unreadable {
				path = filepath.Join(root, changeInstanceMarkerFile)
				write(t, path, "sdd-"+strings.Repeat("a", 32)+"\n")
				mode = 0
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0700) })
			if err := PrepareChangeInstanceConsent(status); err == nil {
				t.Fatal("inaccessible preparation succeeded")
			}
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".gentle-ai-instance") && !unreadable {
					t.Fatalf("unwritable entry published %s", entry.Name())
				}
			}
		})
	}
}

func TestConsentPreparationEngramAndNullableDiscovery(t *testing.T) {
	for _, selected := range []bool{false, true} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			repo := initRuntimeLedgerRepo(t)
			write(t, filepath.Join(repo, "openspec", "config.yaml"), "sdd:\n  artifact_store: engram\n")
			mkdir(t, filepath.Join(repo, ".engram"))
			runRuntimeLedgerGit(t, repo, "remote", "add", "origin", "git@github.com:Gentleman-Programming/gentle-ai.git")
			var observations []engramObservation
			options := ResolveOptions{CWD: repo, IncludeInstructions: true}
			if selected {
				options.ChangeName = "remote"
				observations = engramPlanningRoute("remote", "propose")
			}
			restore := stubEngramExport(t, observations)
			defer restore()
			before := snapshotStatusReadTree(t, repo)
			status, err := Resolve(options)
			if err != nil {
				t.Fatal(err)
			}
			if status.ArtifactStore != ArtifactStoreEngram || status.Consent != nil || (selected && ptrValue(status.ChangeRoot) != "engram:sdd/remote") || (!selected && status.ChangeRoot != nil) {
				t.Fatalf("invented filesystem authority: %#v", status)
			}
			if selected != (status.ChangeName != nil) {
				t.Fatal("wrong nullable selection")
			}
			if err := PrepareChangeInstanceConsent(status); err != nil {
				t.Fatal(err)
			}
			if _, err := ProjectStatusV2(status); err != nil {
				t.Fatal(err)
			}
			if snapshotStatusReadTree(t, repo) != before {
				t.Fatal("Engram preparation mutated filesystem")
			}
		})
	}
}
