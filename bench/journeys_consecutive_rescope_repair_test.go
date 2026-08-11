package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRC1ConsecutiveRescopeProvenanceMatchesFixture(t *testing.T) {
	records, err := rc1ConsecutiveRescopeRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 {
		t.Fatalf("provenance record count = %d, want 4", len(records))
	}
}

func TestPrintedConsecutiveRescopeRepairArgumentsPreservesLiteralBackticks(t *testing.T) {
	workspace := "/tmp/work`space"
	change := "repair`change"
	stderr := "published consecutive-rescope record sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa is unreadable under normal replay; " + consecutiveRescopeRepairPrefix +
		"gentle-ai sdd-attempt repair --cwd '" + workspace + "' --change '" + change + "'"

	got, err := printedConsecutiveRescopeRepairArguments(stderr)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sdd-attempt", "repair", "--cwd", workspace, "--change", change}
	if !slices.Equal(got, want) {
		t.Fatalf("parsed repair command = %#v, want %#v", got, want)
	}
}

func TestRC1ConsecutiveRescopeProvenanceRefusesSameLengthOperationMutation(t *testing.T) {
	fixture := mutatedRC1ConsecutiveRescopeProvenance(t, func(manifest *rc1ConsecutiveRescopeManifest) {
		manifest.OperationShape[3] = "objective/rescope B to D"
	})
	if _, err := rc1ConsecutiveRescopeRecordsFrom(fixture); err == nil || !strings.Contains(err.Error(), "refuses a different ordered operation sequence") {
		t.Fatalf("same-length operation mutation = %v, want observable ordered-sequence refusal", err)
	}
}

func TestRC1ConsecutiveRescopeProvenanceRefusesGeneratorCommandMutation(t *testing.T) {
	fixture := mutatedRC1ConsecutiveRescopeProvenance(t, func(manifest *rc1ConsecutiveRescopeManifest) {
		manifest.GeneratorCommands[1] = "go build -o gentle-ai ./cmd/gentle-ai"
	})
	if _, err := rc1ConsecutiveRescopeRecordsFrom(fixture); err == nil || !strings.Contains(err.Error(), "refuses different generator commands") {
		t.Fatalf("generator command mutation = %v, want observable generator-command refusal", err)
	}
}

func TestRC1ConsecutiveRescopeGeneratorCommandsPinExactHistoricalWorkspace(t *testing.T) {
	const root = "/tmp/opencode/2839-repro"
	want := []string{
		"test ! -e " + root,
		"mkdir -p " + root,
		"git clone --no-checkout . " + root + "/source",
		"git -C " + root + "/source checkout 3d1e673553c9afb0bf91a710121f415d6a7e4ed1",
		"go -C " + root + "/source build -trimpath -o " + root + "/gentle-ai ./cmd/gentle-ai",
		"mkdir " + root + "/repo",
		"git -C " + root + "/repo init -b main -q",
		"git -C " + root + "/repo config user.name Fixture",
		"git -C " + root + "/repo config user.email fixture@example.invalid",
		"git -C " + root + "/repo config commit.gpgsign false",
		"git -C " + root + `/repo commit --allow-empty -qm "fixture baseline"`,
		"(cd " + root + " && ./gentle-ai sdd-attempt begin --cwd " + root + "/repo --change consecutive-rescope-repair --expected-revision= --request-id begin-a --work-unit objective-a --evidence-goal prove-a --max-attempts 2 --max-changed-lines 20)",
		"(cd " + root + ` && ./gentle-ai sdd-attempt finish --cwd ` + root + `/repo --change consecutive-rescope-repair --expected-revision "$(./gentle-ai sdd-attempt status --cwd ` + root + `/repo --change consecutive-rescope-repair | jq -r '.revision')" --request-id finish-a-failed --outcome failed --evidence-revision sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --diagnosis "intentional failed zero-drift attempt" --harness-disposition reused --cleanup-evidence clean --process-evidence reproduced)`,
		"(cd " + root + ` && ./gentle-ai sdd-attempt rescope --cwd ` + root + `/repo --change consecutive-rescope-repair --expected-revision "$(./gentle-ai sdd-attempt status --cwd ` + root + `/repo --change consecutive-rescope-repair | jq -r '.revision')" --request-id rescope-a-b --work-unit objective-b --evidence-goal prove-b --max-attempts 1 --max-changed-lines 10 --reason "narrow A to B" --actor fixture-maintainer)`,
		"(cd " + root + ` && ./gentle-ai sdd-attempt rescope --cwd ` + root + `/repo --change consecutive-rescope-repair --expected-revision "$(./gentle-ai sdd-attempt status --cwd ` + root + `/repo --change consecutive-rescope-repair | jq -r '.revision')" --request-id rescope-b-c --work-unit objective-c --evidence-goal prove-c --max-attempts 1 --max-changed-lines 5 --reason "narrow B to C" --actor fixture-maintainer)`,
	}
	if !slices.Equal(rc1ConsecutiveRescopeGeneratorCommands, want) {
		t.Fatalf("generator commands = %#v, want %#v", rc1ConsecutiveRescopeGeneratorCommands, want)
	}
}

func TestRC1ConsecutiveRescopeProvenanceRefusesHistoricalWorkspaceDrift(t *testing.T) {
	fixture := mutatedRC1ConsecutiveRescopeProvenance(t, func(manifest *rc1ConsecutiveRescopeManifest) {
		manifest.GeneratorCommands[11] = strings.Replace(manifest.GeneratorCommands[11], rc1ConsecutiveRescopeFixtureRoot+"/repo", "/tmp/opencode/other/repo", 1)
	})
	if _, err := rc1ConsecutiveRescopeRecordsFrom(fixture); err == nil || !strings.Contains(err.Error(), "refuses different generator commands") {
		t.Fatalf("historical workspace drift = %v, want observable generator-command refusal", err)
	}
}

func TestRC1ConsecutiveRescopeProvenanceRefusesRecordMapMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*rc1ConsecutiveRescopeManifest)
	}{
		{
			name: "missing expected record",
			mutate: func(manifest *rc1ConsecutiveRescopeManifest) {
				delete(manifest.Records, strings.TrimPrefix(rc1ConsecutiveRescopeHead, "sha256:")+".json")
			},
		},
		{
			name: "extra record",
			mutate: func(manifest *rc1ConsecutiveRescopeManifest) {
				manifest.Records["extra.json"] = "sha256:extra"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := mutatedRC1ConsecutiveRescopeProvenance(t, test.mutate)
			if _, err := rc1ConsecutiveRescopeRecordsFrom(fixture); err == nil || !strings.Contains(err.Error(), "source-controlled expected records") {
				t.Fatalf("record map mutation = %v, want source-controlled record-set refusal", err)
			}
		})
	}
}

func TestRC1ConsecutiveRescopeProvenanceRefusesUnexpectedRecordReplacement(t *testing.T) {
	var name string
	payload := []byte("unexpected immutable record")
	fixture := mutatedRC1ConsecutiveRescopeProvenance(t, func(manifest *rc1ConsecutiveRescopeManifest) {
		delete(manifest.Records, strings.TrimPrefix(rc1ConsecutiveRescopeHead, "sha256:")+".json")
		sum := sha256.Sum256(payload)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		name = strings.TrimPrefix(digest, "sha256:") + ".json"
		manifest.Records[name] = digest
	})
	fixture["testdata/consecutive-rescope-rc1/records/"+name] = &fstest.MapFile{Data: payload}
	if _, err := rc1ConsecutiveRescopeRecordsFrom(fixture); err == nil || !strings.Contains(err.Error(), "source-controlled expected records") {
		t.Fatalf("unexpected record replacement = %v, want source-controlled record-set refusal", err)
	}
}

func mutatedRC1ConsecutiveRescopeProvenance(t *testing.T, mutate func(*rc1ConsecutiveRescopeManifest)) fstest.MapFS {
	t.Helper()
	payload, err := consecutiveRescopeRC1.ReadFile("testdata/consecutive-rescope-rc1/provenance.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest rc1ConsecutiveRescopeManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	mutate(&manifest)
	payload, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"testdata/consecutive-rescope-rc1/provenance.json": &fstest.MapFile{Data: payload},
	}
	entries, err := consecutiveRescopeRC1.ReadDir("testdata/consecutive-rescope-rc1/records")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := "testdata/consecutive-rescope-rc1/records/" + entry.Name()
		record, err := consecutiveRescopeRC1.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[path] = &fstest.MapFile{Data: record}
	}
	return files
}
