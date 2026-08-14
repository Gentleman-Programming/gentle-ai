package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// These bytes were created by the v2.4.0-rc.1 binary in the #2839 RED run.
// The bench cannot create this state itself because the current writer rightly
// refuses it, so it verifies every copied record before exercising recovery.
//
//go:embed testdata/consecutive-rescope-rc1/HEAD testdata/consecutive-rescope-rc1/provenance.json testdata/consecutive-rescope-rc1/records/*.json
var consecutiveRescopeRC1 embed.FS

const (
	rc1ConsecutiveRescopeChange      = "consecutive-rescope-repair"
	rc1ConsecutiveRescopeFixtureRoot = "/tmp/opencode/2839-repro"
	rc1ConsecutiveRescopeHead        = "sha256:da84114f95f9d1674cd23a3d06f5d92a3d5d36a029d5d40931500b91854ec622"
	consecutiveRescopeRepairPrefix   = "run this repair command:\n"
)

type rc1ConsecutiveRescopeManifest struct {
	Schema            string            `json:"schema"`
	PublicTag         string            `json:"public_tag"`
	Commit            string            `json:"commit"`
	GeneratorCommands []string          `json:"generator_commands"`
	OperationShape    []string          `json:"operation_shape"`
	Records           map[string]string `json:"records"`
}

var rc1ConsecutiveRescopeGeneratorCommands = []string{
	"test ! -e " + rc1ConsecutiveRescopeFixtureRoot,
	"mkdir -p " + rc1ConsecutiveRescopeFixtureRoot,
	"git clone --no-checkout . " + rc1ConsecutiveRescopeFixtureRoot + "/source",
	"git -C " + rc1ConsecutiveRescopeFixtureRoot + "/source checkout 3d1e673553c9afb0bf91a710121f415d6a7e4ed1",
	"go -C " + rc1ConsecutiveRescopeFixtureRoot + "/source build -trimpath -o " + rc1ConsecutiveRescopeFixtureRoot + "/gentle-ai ./cmd/gentle-ai",
	"mkdir " + rc1ConsecutiveRescopeFixtureRoot + "/repo",
	"git -C " + rc1ConsecutiveRescopeFixtureRoot + "/repo init -b main -q",
	"git -C " + rc1ConsecutiveRescopeFixtureRoot + "/repo config user.name Fixture",
	"git -C " + rc1ConsecutiveRescopeFixtureRoot + "/repo config user.email fixture@example.invalid",
	"git -C " + rc1ConsecutiveRescopeFixtureRoot + "/repo config commit.gpgsign false",
	"git -C " + rc1ConsecutiveRescopeFixtureRoot + `/repo commit --allow-empty -qm "fixture baseline"`,
	"(cd " + rc1ConsecutiveRescopeFixtureRoot + " && ./gentle-ai sdd-attempt begin --cwd " + rc1ConsecutiveRescopeFixtureRoot + "/repo --change consecutive-rescope-repair --expected-revision= --request-id begin-a --work-unit objective-a --evidence-goal prove-a --max-attempts 2 --max-changed-lines 20)",
	"(cd " + rc1ConsecutiveRescopeFixtureRoot + ` && ./gentle-ai sdd-attempt finish --cwd ` + rc1ConsecutiveRescopeFixtureRoot + `/repo --change consecutive-rescope-repair --expected-revision "$(./gentle-ai sdd-attempt status --cwd ` + rc1ConsecutiveRescopeFixtureRoot + `/repo --change consecutive-rescope-repair | jq -r '.revision')" --request-id finish-a-failed --outcome failed --evidence-revision sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --diagnosis "intentional failed zero-drift attempt" --harness-disposition reused --cleanup-evidence clean --process-evidence reproduced)`,
	"(cd " + rc1ConsecutiveRescopeFixtureRoot + ` && ./gentle-ai sdd-attempt rescope --cwd ` + rc1ConsecutiveRescopeFixtureRoot + `/repo --change consecutive-rescope-repair --expected-revision "$(./gentle-ai sdd-attempt status --cwd ` + rc1ConsecutiveRescopeFixtureRoot + `/repo --change consecutive-rescope-repair | jq -r '.revision')" --request-id rescope-a-b --work-unit objective-b --evidence-goal prove-b --max-attempts 1 --max-changed-lines 10 --reason "narrow A to B" --actor fixture-maintainer)`,
	"(cd " + rc1ConsecutiveRescopeFixtureRoot + ` && ./gentle-ai sdd-attempt rescope --cwd ` + rc1ConsecutiveRescopeFixtureRoot + `/repo --change consecutive-rescope-repair --expected-revision "$(./gentle-ai sdd-attempt status --cwd ` + rc1ConsecutiveRescopeFixtureRoot + `/repo --change consecutive-rescope-repair | jq -r '.revision')" --request-id rescope-b-c --work-unit objective-c --evidence-goal prove-c --max-attempts 1 --max-changed-lines 5 --reason "narrow B to C" --actor fixture-maintainer)`,
}

var rc1ConsecutiveRescopeOperationShape = []string{
	"attempt/begin objective A",
	"attempt/finish failed zero-drift objective A",
	"objective/rescope A to B",
	"objective/rescope B to C",
}

var rc1ConsecutiveRescopeExpectedRecords = map[string]string{
	"00357f75e9bd3b44b2e1a752fb22476547041deb9b062a246c9f21c70d225640.json": "sha256:00357f75e9bd3b44b2e1a752fb22476547041deb9b062a246c9f21c70d225640",
	"2d5661e29641c65b4a0da2ddfe9d94e2ab0b429e3c1093d7400a7142d1c325bb.json": "sha256:2d5661e29641c65b4a0da2ddfe9d94e2ab0b429e3c1093d7400a7142d1c325bb",
	"da84114f95f9d1674cd23a3d06f5d92a3d5d36a029d5d40931500b91854ec622.json": "sha256:da84114f95f9d1674cd23a3d06f5d92a3d5d36a029d5d40931500b91854ec622",
	"ff5759db66fe3beed65d5ae132e066457cfa81695673ebd778a9b7d7bcc96abd.json": "sha256:ff5759db66fe3beed65d5ae132e066457cfa81695673ebd778a9b7d7bcc96abd",
}

func rc1ConsecutiveRescopeRecords() (map[string]string, error) {
	return rc1ConsecutiveRescopeRecordsFrom(consecutiveRescopeRC1)
}

func rc1ConsecutiveRescopeRecordsFrom(fixture fs.FS) (map[string]string, error) {
	payload, err := fs.ReadFile(fixture, "testdata/consecutive-rescope-rc1/provenance.json")
	if err != nil {
		return nil, err
	}
	var manifest rc1ConsecutiveRescopeManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, fmt.Errorf("parse RC fixture provenance: %w", err)
	}
	if manifest.Schema != "gentle-ai.sdd-runtime-fixture-provenance/v1" || manifest.PublicTag != "v2.4.0-rc.1" || manifest.Commit != "3d1e673553c9afb0bf91a710121f415d6a7e4ed1" {
		return nil, errors.New("invalid bounded RC fixture provenance")
	}
	if !slices.Equal(manifest.OperationShape, rc1ConsecutiveRescopeOperationShape) {
		return nil, errors.New("RC fixture provenance refuses a different ordered operation sequence")
	}
	if !slices.Equal(manifest.GeneratorCommands, rc1ConsecutiveRescopeGeneratorCommands) {
		return nil, errors.New("RC fixture provenance refuses different generator commands")
	}
	if len(manifest.Records) != len(rc1ConsecutiveRescopeExpectedRecords) {
		return nil, errors.New("RC fixture provenance refuses a record set different from the source-controlled expected records")
	}
	records := make(map[string]string, len(rc1ConsecutiveRescopeExpectedRecords))
	for name, expected := range rc1ConsecutiveRescopeExpectedRecords {
		if manifest.Records[name] != expected {
			return nil, errors.New("RC fixture provenance refuses a record set different from the source-controlled expected records")
		}
		payload, err := fs.ReadFile(fixture, "testdata/consecutive-rescope-rc1/records/"+name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(payload)
		if expected != "sha256:"+hex.EncodeToString(sum[:]) || expected != "sha256:"+strings.TrimSuffix(name, ".json") {
			return nil, fmt.Errorf("RC fixture %s does not match provenance", name)
		}
		records[name] = expected
	}
	return records, nil
}

func rc1ConsecutiveRescopeStore(sandbox *Sandbox) error {
	if err := sandbox.initRepo(sandbox.Repo); err != nil {
		return err
	}
	if err := sandbox.write(filepath.Join(sandbox.Repo, "README.md"), "# RC fixture\n"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "add", "README.md"); err != nil {
		return err
	}
	if err := sandbox.git(sandbox.Repo, "commit", "-qm", "fixture baseline"); err != nil {
		return err
	}
	common, err := gitCommonDir(sandbox, sandbox.Repo)
	if err != nil {
		return err
	}
	records, err := rc1ConsecutiveRescopeRecords()
	if err != nil {
		return err
	}
	destination := filepath.Join(common, "gentle-ai", "sdd-runtime", "v1", rc1ConsecutiveRescopeChange)
	for _, path := range []string{"HEAD"} {
		payload, err := consecutiveRescopeRC1.ReadFile("testdata/consecutive-rescope-rc1/" + path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, path)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, payload, 0o644); err != nil {
			return err
		}
	}
	for name := range records {
		payload, err := consecutiveRescopeRC1.ReadFile("testdata/consecutive-rescope-rc1/records/" + name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, "records", name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, payload, 0o644); err != nil {
			return err
		}
	}
	sandbox.Scratch["rc1-consecutive-rescope-record"] = filepath.Join(destination, "records", strings.TrimPrefix(rc1ConsecutiveRescopeHead, "sha256:")+".json")
	return nil
}

func runPrintedConsecutiveRescopeRepair(r *journeyRun) error {
	records, err := rc1ConsecutiveRescopeRecords()
	if err != nil {
		return err
	}
	recordDir := filepath.Dir(r.sandbox.Scratch["rc1-consecutive-rescope-record"])
	beforeRecords := map[string][]byte{}
	for name := range records {
		beforeRecords[name], err = os.ReadFile(filepath.Join(recordDir, name))
		if err != nil {
			return err
		}
	}
	headBefore, err := os.ReadFile(filepath.Join(filepath.Dir(recordDir), "HEAD"))
	if err != nil {
		return err
	}
	before := r.run([]string{"sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", rc1ConsecutiveRescopeChange}, false)
	if before.ExitCode == 0 || !strings.Contains(before.Stderr, rc1ConsecutiveRescopeHead) {
		return fmt.Errorf("RC-shaped poisoned status = exit %d, stderr %q", before.ExitCode, firstLine(before.Stderr))
	}
	args, err := printedConsecutiveRescopeRepairArguments(before.Stderr)
	if err != nil {
		return fmt.Errorf("printed repair command: %w", err)
	}
	repaired := r.run(args, false)
	if repaired.ExitCode != 0 {
		return fmt.Errorf("printed repair command exit=%d: %s", repaired.ExitCode, firstLine(repaired.Stderr))
	}
	var repairedStatus struct {
		Revision         string `json:"revision"`
		DecisionRequired bool   `json:"decision_required"`
		NextAction       string `json:"next_action"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(repaired.Stdout)), &repairedStatus); err != nil || !repairedStatus.DecisionRequired || repairedStatus.NextAction != "reset" {
		return fmt.Errorf("repair result = %#v, parse=%v", repairedStatus, err)
	}
	entries, err := os.ReadDir(recordDir)
	if err != nil || len(entries) != len(beforeRecords)+1 {
		return fmt.Errorf("repair record count = %d, err=%v", len(entries), err)
	}
	headAfter, err := os.ReadFile(filepath.Join(filepath.Dir(recordDir), "HEAD"))
	if err != nil || bytes.Equal(headBefore, headAfter) {
		return fmt.Errorf("repair HEAD changed=%t, err=%v", !bytes.Equal(headBefore, headAfter), err)
	}
	for name, want := range beforeRecords {
		after, err := os.ReadFile(filepath.Join(recordDir, name))
		if err != nil || !bytes.Equal(after, want) {
			return fmt.Errorf("repair changed preserved RC record %s: %v", name, err)
		}
	}
	status := r.run([]string{"sdd-attempt", "status", "--cwd", r.sandbox.Repo, "--change", rc1ConsecutiveRescopeChange}, false)
	if status.ExitCode != 0 || !strings.Contains(status.Stdout, `"last_repair"`) || !strings.Contains(status.Stdout, `"decision_required": true`) || !strings.Contains(status.Stdout, `"next_action": "reset"`) {
		return fmt.Errorf("status after printed repair = exit %d, stdout %q, stderr %q", status.ExitCode, firstLine(status.Stdout), firstLine(status.Stderr))
	}
	reset := r.run([]string{"sdd-attempt", "reset", "--cwd", r.sandbox.Repo, "--change", rc1ConsecutiveRescopeChange, "--expected-revision", repairedStatus.Revision, "--request-id", "bench-2839-reset", "--actor", "bench-maintainer", "--reason", "reset repaired exhausted objective"}, false)
	if reset.ExitCode != 0 || !strings.Contains(reset.Stdout, `"next_action": "begin"`) {
		return fmt.Errorf("reset after repair = exit %d, stdout %q, stderr %q", reset.ExitCode, firstLine(reset.Stdout), firstLine(reset.Stderr))
	}
	var resetStatus struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(reset.Stdout)), &resetStatus); err != nil {
		return fmt.Errorf("parse reset after repair: %w", err)
	}
	if begin := r.run([]string{"sdd-attempt", "begin", "--cwd", r.sandbox.Repo, "--change", rc1ConsecutiveRescopeChange, "--expected-revision", resetStatus.Revision, "--request-id", "bench-2839-begin", "--work-unit", "objective-b", "--evidence-goal", "prove-b", "--max-attempts", "1", "--max-changed-lines", "10"}, false); begin.ExitCode != 0 {
		return fmt.Errorf("begin after reset = exit %d: %s", begin.ExitCode, firstLine(begin.Stderr))
	}
	return nil
}

func printedConsecutiveRescopeRepairArguments(stderr string) ([]string, error) {
	_, command, found := strings.Cut(stderr, consecutiveRescopeRepairPrefix)
	if !found {
		return nil, errors.New("poisoned status named no runnable repair command")
	}
	return printedCommandArguments(command)
}

func consecutiveRescopeRepairJourneys() []Journey {
	return []Journey{{
		ID:     "j81-rc1-consecutive-rescope-repair-executes-printed-command",
		Title:  "RC-created poison: status names and executes the audited repair without rewriting C",
		Source: "issue #2839; fixture extracted byte-for-byte from the v2.4.0-rc.1 RED run",
		Steps: []Step{
			{Name: "fixture: exact RC-created consecutive-rescope records", Fixture: rc1ConsecutiveRescopeStore},
			{Name: "run printed repair, audited reset, then begin", Composite: runPrintedConsecutiveRescopeRepair},
		},
	}}
}
