package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestRunArgsReviewValidateEnabledUsesUnmanagedDeliveryForEveryGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOCACHE", filepath.Join(home, "go-cache"))
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	if err := os.MkdirAll(os.Getenv("TMPDIR"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo := initAppReviewGateRepository(t)
	var mode bytes.Buffer
	if err := RunArgs([]string{"review", "mode", "enable", "--scope", "global", "--cwd", repo}, &mode); err != nil {
		t.Fatalf("enable receipt-driven development: %v\n%s", err, mode.String())
	}

	for _, gate := range []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"} {
		t.Run(gate, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunArgs([]string{"review", "validate", "--cwd", repo, "--gate", gate}, &output); err != nil {
				t.Fatalf("review validate --gate %s: %v\n%s", gate, err, output.String())
			}

			var result struct {
				Schema   string         `json:"schema"`
				Result   string         `json:"result"`
				Allowed  bool           `json:"allowed"`
				Action   string         `json:"action"`
				Reason   string         `json:"reason"`
				Context  map[string]any `json:"context"`
				Delivery string         `json:"delivery"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode output: %v\n%s", err, output.String())
			}
			if result.Schema != "gentle-ai.review-gate-result/v1" || result.Result != "invalidated" || result.Allowed ||
				result.Action != "repository-policy" || result.Delivery != "unmanaged" {
				t.Fatalf("non-deciding gate result = %#v\n%s", result, output.String())
			}
			if result.Reason != "review enforcement is enabled but no review authority governs this candidate, so delivery follows ordinary repository policy" {
				t.Fatalf("gate reason = %q", result.Reason)
			}
			if len(result.Context) != 1 || result.Context["gate"] != gate {
				t.Fatalf("gate context = %#v, want only gate %q", result.Context, gate)
			}
			compactOutput := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(strings.ToLower(output.String()))
			for _, forbidden := range []string{"\"result\":\"allow\"", "\"allowed\":true", "approved", "receipt", "lineage", "receipt_governed", "disabled"} {
				if strings.Contains(compactOutput, forbidden) {
					t.Fatalf("enabled unmanaged output contains forbidden %q:\n%s", forbidden, output.String())
				}
			}
		})
	}
}

func TestRunArgsReviewValidateNonDecidingNegotiatedAndLegacyRoutes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOCACHE", filepath.Join(home, "go-cache"))
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	if err := os.MkdirAll(os.Getenv("TMPDIR"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo := initAppReviewGateRepository(t)
	if err := RunArgs([]string{"review", "mode", "enable", "--scope", "global", "--cwd", repo}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	for _, contract := range []string{"gentle-ai.review-integration/v1", "gentle-ai.review-integration/v2"} {
		t.Run(contract, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunArgs([]string{"review", "validate", "--contract", contract, "--cwd", repo, "--gate", "pre-commit"}, &output); err != nil {
				t.Fatalf("negotiated review validate: %v\n%s", err, output.String())
			}
			var envelope struct {
				Schema    string          `json:"schema"`
				Contract  string          `json:"contract"`
				Operation string          `json:"operation"`
				Result    json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Contract != contract || envelope.Operation != "review.validate" {
				t.Fatalf("negotiated envelope = %#v", envelope)
			}
			var result map[string]any
			if err := json.Unmarshal(envelope.Result, &result); err != nil {
				t.Fatal(err)
			}
			if result["delivery"] != "unmanaged" || result["action"] != "repository-policy" || result["allowed"] != false {
				t.Fatalf("negotiated gate result = %s", envelope.Result)
			}
		})
	}

	requestPath := filepath.Join(repo, "request.json")
	if err := os.WriteFile(requestPath, []byte(`{"gate":"release"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var legacy bytes.Buffer
	if err := RunArgs([]string{"review-validate", "--cwd", repo, "--receipt", filepath.Join(repo, "missing-receipt.json"), "--request", requestPath}, &legacy); err != nil {
		t.Fatalf("legacy review-validate must not read receipt authority: %v\n%s", err, legacy.String())
	}
	var legacyResult map[string]any
	if err := json.Unmarshal(legacy.Bytes(), &legacyResult); err != nil {
		t.Fatal(err)
	}
	if legacyResult["delivery"] != "unmanaged" || legacyResult["context"].(map[string]any)["gate"] != "release" {
		t.Fatalf("legacy non-deciding result = %s", legacy.String())
	}

	if err := RunArgs([]string{"review", "mode", "disable", "--scope", "global", "--cwd", repo}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var disabled bytes.Buffer
	if err := RunArgs([]string{"review", "validate", "--cwd", repo, "--gate", "pre-commit"}, &disabled); err != nil {
		t.Fatalf("disabled review validate: %v\n%s", err, disabled.String())
	}
	var disabledResult map[string]any
	if err := json.Unmarshal(disabled.Bytes(), &disabledResult); err != nil {
		t.Fatal(err)
	}
	if disabledResult["delivery"] != "disabled/unmanaged" || disabledResult["allowed"] != false || disabledResult["result"] == "allow" {
		t.Fatalf("disabled non-deciding result = %s", disabled.String())
	}
}

func TestRunArgsReviewValidateDoesNotReadOrMutateAuthority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOCACHE", filepath.Join(home, "go-cache"))
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	if err := os.MkdirAll(os.Getenv("TMPDIR"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo := initAppReviewGateRepository(t)
	if err := RunArgs([]string{"review", "mode", "enable", "--scope", "global", "--cwd", repo}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	invoke := func(t *testing.T) []byte {
		t.Helper()
		var output bytes.Buffer
		if err := RunArgs([]string{"review", "validate", "--cwd", repo, "--gate", "pre-push"}, &output); err != nil {
			t.Fatalf("pre-push without an upstream: %v\n%s", err, output.String())
		}
		return output.Bytes()
	}
	baseline := invoke(t)

	lockPath := filepath.Join(repo, ".git", "gentle-ai", "review-transactions", "v2", "LOCK")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const residue = "interrupted authority lock residue\n"
	if err := os.WriteFile(lockPath, []byte(residue), 0o600); err != nil {
		t.Fatal(err)
	}
	withResidue := invoke(t)
	if !bytes.Equal(baseline, withResidue) {
		t.Fatalf("authority residue changed non-deciding gate output:\nbaseline:\n%s\nresidue:\n%s", baseline, withResidue)
	}
	payload, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != residue {
		t.Fatalf("review validate mutated authority residue: %q", payload)
	}
}

func TestRunArgsReviewValidateAllGatesRoutesAgreeAndMatchPublishedSchema(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOCACHE", filepath.Join(home, "go-cache"))
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	if err := os.MkdirAll(os.Getenv("TMPDIR"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo := initAppReviewGateRepository(t)
	if err := RunArgs([]string{"review", "mode", "enable", "--scope", "global", "--cwd", repo}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	schema := compileAppGateResultSchema(t)
	decode := func(t *testing.T, payload []byte) map[string]any {
		t.Helper()
		var result map[string]any
		if err := json.Unmarshal(payload, &result); err != nil {
			t.Fatalf("decode gate result: %v\\n%s", err, payload)
		}
		return result
	}
	validate := func(t *testing.T, payload []byte) {
		t.Helper()
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(document); err != nil {
			t.Fatalf("published gate schema rejected result: %v\\n%s", err, payload)
		}
	}

	for _, gate := range []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"} {
		t.Run(gate, func(t *testing.T) {
			var plain bytes.Buffer
			if err := RunArgs([]string{"review", "validate", "--cwd", repo, "--gate", gate}, &plain); err != nil {
				t.Fatalf("plain review validate: %v\\n%s", err, plain.String())
			}
			validate(t, plain.Bytes())
			want := decode(t, plain.Bytes())
			if gate == "pre-push" && want["delivery"] != "unmanaged" {
				t.Fatalf("empty/no-upstream pre-push result = %s", plain.String())
			}

			for _, contract := range []string{"gentle-ai.review-integration/v1", "gentle-ai.review-integration/v2"} {
				t.Run(contract, func(t *testing.T) {
					var output bytes.Buffer
					if err := RunArgs([]string{"review", "validate", "--contract", contract, "--cwd", repo, "--gate", gate}, &output); err != nil {
						t.Fatalf("negotiated review validate: %v\\n%s", err, output.String())
					}
					var envelope struct {
						Contract  string          `json:"contract"`
						Operation string          `json:"operation"`
						Result    json.RawMessage `json:"result"`
					}
					if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
						t.Fatal(err)
					}
					if envelope.Contract != contract || envelope.Operation != "review.validate" {
						t.Fatalf("negotiated review validate envelope = %#v", envelope)
					}
					validate(t, envelope.Result)
					if got := decode(t, envelope.Result); !reflect.DeepEqual(got, want) {
						t.Fatalf("negotiated %s result = %#v, want %#v", contract, got, want)
					}
				})
			}

			requestPath := filepath.Join(t.TempDir(), "legacy-request.json")
			if err := os.WriteFile(requestPath, []byte(`{"gate":"`+gate+`"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			var legacy bytes.Buffer
			if err := RunArgs([]string{"review-validate", "--cwd", repo, "--receipt", filepath.Join(repo, "missing-receipt.json"), "--request", requestPath}, &legacy); err != nil {
				t.Fatalf("legacy review-validate: %v\\n%s", err, legacy.String())
			}
			validate(t, legacy.Bytes())
			if got := decode(t, legacy.Bytes()); !reflect.DeepEqual(got, want) {
				t.Fatalf("legacy result = %#v, want %#v", got, want)
			}
		})
	}
}

func TestRunArgsReviewValidateDisabledAllGatesAreByteStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("GOCACHE", filepath.Join(home, "go-cache"))
	t.Setenv("TMPDIR", filepath.Join(home, "tmp"))
	if err := os.MkdirAll(os.Getenv("TMPDIR"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo := initAppReviewGateRepository(t)
	if err := RunArgs([]string{"review", "mode", "disable", "--scope", "global", "--cwd", repo}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	schema := compileAppGateResultSchema(t)
	for _, gate := range []string{"post-apply", "pre-commit", "pre-push", "pre-pr", "release"} {
		t.Run(gate, func(t *testing.T) {
			run := func() []byte {
				t.Helper()
				var output bytes.Buffer
				if err := RunArgs([]string{"review", "validate", "--cwd", repo, "--gate", gate}, &output); err != nil {
					t.Fatalf("disabled review validate: %v\\n%s", err, output.String())
				}
				return output.Bytes()
			}
			first, second := run(), run()
			if !bytes.Equal(first, second) {
				t.Fatalf("disabled gate output changed:\\nfirst:\\n%s\\nsecond:\\n%s", first, second)
			}
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(first))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(document); err != nil {
				t.Fatalf("published gate schema rejected disabled result: %v\\n%s", err, first)
			}
			var result map[string]any
			if err := json.Unmarshal(first, &result); err != nil {
				t.Fatal(err)
			}
			if result["delivery"] != "disabled/unmanaged" || result["allowed"] != false || result["result"] == "allow" ||
				len(result["context"].(map[string]any)) != 1 || result["context"].(map[string]any)["gate"] != gate {
				t.Fatalf("disabled result = %s", first)
			}
		})
	}
}

func compileAppGateResultSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "contracts", "review-integration", "v2", "schemas", "gate-result.schema.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(document["$id"].(string), document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(document["$id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func initAppReviewGateRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	command := exec.Command("git", "init", "--quiet", repo)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	return repo
}
