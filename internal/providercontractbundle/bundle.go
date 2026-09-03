// Package providercontractbundle creates and verifies the data-only reviewer
// provider contract release archive.
package providercontractbundle

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
	"github.com/gentleman-programming/gentle-ai/v2/internal/reviewerprovider"
)

const (
	bundleSchema             = "gentle-ai.review-provider-contract-bundle/v1"
	contractSemverFile       = "contracts/review-provider-contract/CONTRACT_SEMVER"
	maxArchiveBytes    int64 = 8 << 20
	maxFileBytes       int64 = 4 << 20
	maxBundleBytes     int64 = 16 << 20
	maxManifestBytes   int64 = 64 << 10
	bundleFileMode           = 0o644
	// bundleFixedFileCount is README.md, manifest.json, and the three
	// schema/vector pairs every role contributes (2*3).
	bundleFixedFileCount = 8
	// maxBundleFileCount bounds the raw tar header pre-check before the
	// canonical inventory is known: the fixed files plus one entry per closed
	// orchestration runtime. It must grow only alongside
	// orchestrationRuntimeIdentities.
	maxBundleFileCount = bundleFixedFileCount + 1
)

var bundleTimestamp = time.Unix(0, 0).UTC()

var (
	// refusal:by-design input: an untrusted bundle never becomes an activation candidate.
	errInvalidBundle = errors.New("invalid provider contract bundle")
	// refusal:by-design input: an invalid local contract version cannot name a release asset.
	errInvalidContractSemver = errors.New("invalid provider contract semver")
	contractSemverPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

type fileReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type roleManifest struct {
	ID                   string        `json:"id"`
	RequestSchemaID      string        `json:"request_schema_id"`
	ResultSchemaID       string        `json:"result_schema_id"`
	RequiredCapabilities []string      `json:"required_capabilities"`
	Schema               fileReference `json:"schema"`
	Vector               fileReference `json:"vector"`
}

type orchestrationManifest struct {
	Runtime string        `json:"runtime"`
	File    fileReference `json:"file"`
}

type manifest struct {
	Schema              string                  `json:"schema"`
	ContractSemver      string                  `json:"contract_semver"`
	TransportCapability string                  `json:"transport_capability"`
	Runtimes            []string                `json:"runtimes"`
	README              fileReference           `json:"readme"`
	Roles               []roleManifest          `json:"roles"`
	Orchestration       []orchestrationManifest `json:"orchestration"`
}

// orchestrationRuntimeIdentities is deliberately closed, the same discipline
// reviewerprovider applies to registeredRuntimeIdentities: a runtime is added
// here only once it has no other delivery channel for the review execution
// contract inside Gentle AI's own installer (no system prompt, skill, or
// file-subagent surface to splice generic SDD composition into), so this
// bundle becomes that runtime's sole channel. Pi is first: its capability
// manifest advertises MCP only, so sdd.Inject is a no-op for it and the
// contract text renderBoundedReviewAssetBody produces for every other
// runtime never reaches gentle-pi any other way (issue #4056).
var orchestrationRuntimeIdentities = []string{"pi"}

// orchestrationEntry is one closed runtime's bound review execution contract,
// materialized once so Generate and Verify read the exact same bytes.
type orchestrationEntry struct {
	runtime string
	content []byte
}

// canonicalOrchestrationEntries renders the bound review execution contract
// for every closed orchestration runtime, in sorted runtime order. It is the
// single source both Generate and Verify read, so the shipped bytes can never
// drift from what sdd.ReviewExecutionContractFor actually produces.
func canonicalOrchestrationEntries() ([]orchestrationEntry, error) {
	runtimes := slices.Clone(orchestrationRuntimeIdentities)
	sort.Strings(runtimes)
	entries := make([]orchestrationEntry, 0, len(runtimes))
	for index, runtime := range runtimes {
		if index > 0 && runtimes[index-1] == runtime {
			return nil, fmt.Errorf("%w: orchestration runtime %q is duplicated", errInvalidBundle, runtime)
		}
		agent := model.AgentID(runtime)
		if !reviewerprovider.RegisteredRuntime(agent) {
			return nil, fmt.Errorf("%w: orchestration runtime %q is not a registered review runtime", errInvalidBundle, runtime)
		}
		content, err := sdd.ReviewExecutionContractFor(agent)
		if err != nil {
			return nil, fmt.Errorf("%w: orchestration runtime %q: %v", errInvalidBundle, runtime, err)
		}
		if strings.Contains(content, "{{GENTLE_AI_RUNTIME_AGENT_ID}}") {
			return nil, fmt.Errorf("%w: orchestration runtime %q left the runtime identity placeholder unbound", errInvalidBundle, runtime)
		}
		if runtime == string(model.AgentPi) {
			if !validPiFacadeLifecycle(content) {
				return nil, fmt.Errorf("%w: orchestration runtime %q does not use the complete facade-only lifecycle", errInvalidBundle, runtime)
			}
		} else {
			wantCommand := "--agent " + runtime
			if count := strings.Count(content, wantCommand); count != 1 {
				return nil, fmt.Errorf("%w: orchestration runtime %q contract names %q %d times, want 1", errInvalidBundle, runtime, wantCommand, count)
			}
		}
		entries = append(entries, orchestrationEntry{runtime: runtime, content: []byte(strings.TrimSpace(content) + "\n")})
	}
	return entries, nil
}

func validPiFacadeLifecycle(content string) bool {
	for _, required := range []string{
		"`gentle_review` with {\"operation\":\"inspect\"}",
		"`gentle_review` with operation `status`, the exact retained `lineageId`, and `workspaceRoot` only when needed",
		"`gentle_review_capture` for one current returned slot",
		"`gentle_review_capture_group` for the complete current reviewer group",
		"On `approved`, use bound facade STATUS to obtain or replay the exact provider-issued `acknowledge-approved` continuation, then execute it unchanged.",
		"Only the exact provider-issued acknowledgement continuation burns approved authority.",
		"`gentle_review` with operation `answer-consent` and the exact `consentBinding`",
		"resubmit the same exact binding with `reviewerRunAcknowledged: true`",
	} {
		if !strings.Contains(content, required) {
			return false
		}
	}
	// The user-owned kill switch (`gentle-ai review mode ...`) is ordinary CLI
	// with no facade operation, so the contract may legitimately name it; every
	// other raw "gentle-ai review " lifecycle route is still forbidden.
	stripped := strings.ReplaceAll(content, "gentle-ai review mode ", "")
	return !strings.Contains(stripped, "gentle-ai review ")
}

var bundleREADME = []byte(`# Gentle AI review provider contract

This data-only bundle describes the provider result contracts admitted by Gentle AI.

## Activation

1. Verify the signed release checksum manifest before using this archive.
2. Verify every listed file hash and the transport capability before activation.
3. Confirm your runtime identity appears in the manifest's registered runtimes before trusting the layout.
4. Pass the Go-materialized opaque prompt to the provider and return only raw output or an error.

## Orchestration

manifest.json's orchestration array lists, for closed runtimes only, one
orchestration/<runtime>.md file: the exact review execution contract text
Gentle AI's own installer would have spliced into that runtime's system
prompt, for a runtime whose adapter has no system prompt to splice it into.
It carries no executable content, same as every other file in this bundle: a
runtime mirrors and delivers the text as-is and still relies on Go for every
review decision, prompt, receipt, and delivery gate.

Go remains the admission authority for prompts, results, receipts, and delivery gates.
`)

// ReadContractSemver reads the one committed provider-contract version file.
func ReadContractSemver(filename string) (string, error) {
	payload, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("%w: read %s: %v", errInvalidContractSemver, filename, err)
	}
	semver := strings.TrimSpace(string(payload))
	if string(payload) != semver+"\n" || !contractSemverPattern.MatchString(semver) {
		return "", fmt.Errorf("%w: %s must contain exact MAJOR.MINOR.PATCH", errInvalidContractSemver, filename)
	}
	return semver, nil
}

// Generate materializes the exact eight root-relative regular files consumed by
// the GoReleaser meta archive. It never writes an archive itself.
func Generate(outputDir, contractSemver string) error {
	if !contractSemverPattern.MatchString(contractSemver) {
		return fmt.Errorf("%w: %q", errInvalidContractSemver, contractSemver)
	}
	files, err := generatedFiles(contractSemver)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(outputDir); err != nil {
		return fmt.Errorf("%w: clear staging directory: %v", errInvalidBundle, err)
	}
	for _, name := range sortedFileNames(files) {
		filename := filepath.Join(outputDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			return fmt.Errorf("%w: create staging directory: %v", errInvalidBundle, err)
		}
		if err := os.WriteFile(filename, files[name], 0o644); err != nil {
			return fmt.Errorf("%w: write %s: %v", errInvalidBundle, name, err)
		}
		if err := os.Chmod(filename, bundleFileMode); err != nil {
			return fmt.Errorf("%w: normalize mode for %s: %v", errInvalidBundle, name, err)
		}
		if err := os.Chtimes(filename, bundleTimestamp, bundleTimestamp); err != nil {
			return fmt.Errorf("%w: normalize timestamp for %s: %v", errInvalidBundle, name, err)
		}
	}
	return nil
}

func generatedFiles(contractSemver string) (map[string][]byte, error) {
	contracts, err := canonicalContracts()
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{"README.md": slices.Clone(bundleREADME)}
	roles := make([]roleManifest, 0, len(contracts))
	for _, contract := range contracts {
		schemaPath := path.Join("schemas", contract.ID+".schema.json")
		vectorPath := path.Join("vectors", contract.ID+".json")
		vector, err := canonicalVector(contract.ResultSchema)
		if err != nil {
			return nil, fmt.Errorf("%w: %s vector: %v", errInvalidBundle, contract.ID, err)
		}
		files[schemaPath] = slices.Clone(contract.ResultSchema)
		files[vectorPath] = vector
		capabilities := slices.Clone(contract.RequiredCapabilities)
		sort.Strings(capabilities)
		roles = append(roles, roleManifest{
			ID:                   contract.ID,
			RequestSchemaID:      contract.RequestSchemaID,
			ResultSchemaID:       contract.ResultSchemaID,
			RequiredCapabilities: capabilities,
			Schema:               fileReference{Path: schemaPath, SHA256: hash(files[schemaPath])},
			Vector:               fileReference{Path: vectorPath, SHA256: hash(files[vectorPath])},
		})
	}
	orchestrationSources, err := canonicalOrchestrationEntries()
	if err != nil {
		return nil, err
	}
	orchestration := make([]orchestrationManifest, 0, len(orchestrationSources))
	for _, entry := range orchestrationSources {
		filePath := path.Join("orchestration", entry.runtime+".md")
		files[filePath] = entry.content
		orchestration = append(orchestration, orchestrationManifest{
			Runtime: entry.runtime,
			File:    fileReference{Path: filePath, SHA256: hash(files[filePath])},
		})
	}
	manifestPayload, err := json.MarshalIndent(manifest{
		Schema:              bundleSchema,
		ContractSemver:      contractSemver,
		TransportCapability: reviewerprovider.TransportCapability,
		Runtimes:            reviewerprovider.RegisteredRuntimeIdentities(),
		README:              fileReference{Path: "README.md", SHA256: hash(files["README.md"])},
		Roles:               roles,
		Orchestration:       orchestration,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: encode manifest: %v", errInvalidBundle, err)
	}
	files["manifest.json"] = append(manifestPayload, '\n')
	if want := bundleFixedFileCount + len(orchestrationSources); len(files) != want {
		return nil, fmt.Errorf("%w: generated inventory has %d files, want %d", errInvalidBundle, len(files), want)
	}
	return files, nil
}

func canonicalContracts() ([]reviewerprovider.Contract, error) {
	contracts := reviewerprovider.Contracts()
	if len(contracts) != 3 {
		return nil, fmt.Errorf("%w: canonical registry has %d roles, want 3", errInvalidBundle, len(contracts))
	}
	sort.Slice(contracts, func(left, right int) bool { return contracts[left].ID < contracts[right].ID })
	for index, contract := range contracts {
		if contract.ID == "" || contract.RequestSchemaID == "" || contract.ResultSchemaID == "" || len(contract.ResultSchema) == 0 || len(contract.RequiredCapabilities) == 0 {
			return nil, fmt.Errorf("%w: canonical role %d is incomplete", errInvalidBundle, index)
		}
		if index > 0 && contracts[index-1].ID == contract.ID {
			return nil, fmt.Errorf("%w: canonical role %q is duplicated", errInvalidBundle, contract.ID)
		}
	}
	return contracts, nil
}

func canonicalVector(schema []byte) ([]byte, error) {
	var document struct {
		Examples []json.RawMessage `json:"examples"`
	}
	if err := json.Unmarshal(schema, &document); err != nil || len(document.Examples) == 0 {
		return nil, errors.New("canonical result schema has no example") // refusal:by-design input: a vector must remain canonical to the registry.
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(document.Examples[0], &object); err != nil || object == nil {
		return nil, errors.New("canonical result schema example is not an object") // refusal:by-design input: provider vectors are JSON objects.
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, document.Examples[0]); err != nil {
		return nil, err
	}
	return append([]byte(compact.String()), '\n'), nil
}

// VerifyArchive checks a complete release archive without extracting it to disk.
func VerifyArchive(filename string) error {
	info, err := os.Lstat(filename)
	if err != nil {
		return fmt.Errorf("%w: inspect archive: %v", errInvalidBundle, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxArchiveBytes {
		return fmt.Errorf("%w: archive is not a bounded regular file", errInvalidBundle)
	}
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("%w: open archive: %v", errInvalidBundle, err)
	}
	defer file.Close()
	compressed := bufio.NewReader(file)
	gzipReader, err := gzip.NewReader(compressed)
	if err != nil {
		return fmt.Errorf("%w: open gzip stream: %v", errInvalidBundle, err)
	}
	gzipReader.Multistream(false)
	expanded, err := io.ReadAll(io.LimitReader(gzipReader, maxBundleBytes+1))
	if err != nil {
		return fmt.Errorf("%w: read gzip stream: %v", errInvalidBundle, err)
	}
	if int64(len(expanded)) > maxBundleBytes {
		return fmt.Errorf("%w: expanded tar exceeds size limit", errInvalidBundle)
	}
	if err := gzipReader.Close(); err != nil {
		return fmt.Errorf("%w: close gzip stream: %v", errInvalidBundle, err)
	}
	if _, err := compressed.ReadByte(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: archive has trailing compressed data", errInvalidBundle)
		}
		return fmt.Errorf("%w: read archive trailer: %v", errInvalidBundle, err)
	}
	if err := validateRawTarHeaders(expanded); err != nil {
		return fmt.Errorf("%w: %v", errInvalidBundle, err)
	}

	entries := make(map[string][]byte, 8)
	tarReader := tar.NewReader(bytes.NewReader(expanded))
	var totalBytes int64
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return fmt.Errorf("%w: read tar entry: %v", errInvalidBundle, nextErr)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("%w: %q is not a regular file", errInvalidBundle, header.Name)
		}
		if !canonicalArchivePath(header.Name) {
			return fmt.Errorf("%w: archive path %q is unsafe", errInvalidBundle, header.Name)
		}
		if _, exists := entries[header.Name]; exists {
			return fmt.Errorf("%w: archive path %q is duplicated", errInvalidBundle, header.Name)
		}
		if header.Size < 0 || header.Size > maxFileBytes || totalBytes > maxBundleBytes-header.Size {
			return fmt.Errorf("%w: archive entry %q exceeds size limits", errInvalidBundle, header.Name)
		}
		payload, readErr := io.ReadAll(io.LimitReader(tarReader, header.Size+1))
		if readErr != nil || int64(len(payload)) != header.Size {
			return fmt.Errorf("%w: read archive entry %q", errInvalidBundle, header.Name)
		}
		totalBytes += header.Size
		entries[header.Name] = payload
	}
	return validateEntries(entries)
}

func validateRawTarHeaders(payload []byte) error {
	const blockSize = 512
	var entries, zeroBlocks int
	for offset := 0; ; {
		if len(payload)-offset < blockSize {
			return errors.New("tar stream ends before a complete header") // refusal:by-design input: a release archive must contain complete tar headers.
		}
		header := payload[offset : offset+blockSize]
		offset += blockSize
		if bytes.Equal(header, make([]byte, blockSize)) {
			zeroBlocks++
			if zeroBlocks >= 2 {
				for _, value := range payload[offset:] {
					if value != 0 {
						return errors.New("tar stream has data after its terminator") // refusal:by-design input: only zero padding may follow a tar terminator.
					}
				}
				return nil
			}
			continue
		}
		if zeroBlocks != 0 {
			return errors.New("tar stream has an incomplete terminator") // refusal:by-design input: a release archive must end with two zero tar blocks.
		}
		if header[156] != tar.TypeReg && header[156] != tar.TypeRegA {
			return fmt.Errorf("tar metadata type %q is forbidden", header[156])
		}
		size, err := rawTarSize(header[124:136])
		if err != nil || size > maxFileBytes {
			return fmt.Errorf("tar entry exceeds size limits: %v", err)
		}
		entries++
		if entries > maxBundleFileCount {
			return errors.New("tar has too many entries") // refusal:by-design input: a provider contract archive has a fixed, closed file count.
		}
		padding := (blockSize - size%blockSize) % blockSize
		if size > int64(len(payload)-offset) || padding > int64(len(payload)-offset)-size {
			return errors.New("tar stream ends before a complete entry") // refusal:by-design input: each tar entry must be complete.
		}
		offset += int(size + padding)
	}
}

func rawTarSize(field []byte) (int64, error) {
	value := strings.Trim(string(field), " \x00")
	if value == "" {
		return 0, nil
	}
	for _, digit := range value {
		if digit < '0' || digit > '7' {
			return 0, fmt.Errorf("tar size is not octal")
		}
	}
	return strconv.ParseInt(value, 8, 64)
}

// VerifyStaging validates a generated staging directory by first writing no
// files outside it: callers use this to prove the release hook's exact bundle
// inventory before GoReleaser packages it.
func VerifyStaging(directory string) error {
	entries := make(map[string][]byte, 8)
	err := filepath.WalkDir(directory, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("staging entry %q is not a regular file", filename)
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if !canonicalArchivePath(name) {
			return fmt.Errorf("staging path %q is unsafe", name)
		}
		payload, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		if int64(len(payload)) > maxFileBytes {
			return fmt.Errorf("staging entry %q exceeds size limit", name)
		}
		entries[name] = payload
		return nil
	})
	if err != nil {
		return fmt.Errorf("%w: read staging directory: %v", errInvalidBundle, err)
	}
	return validateEntries(entries)
}

func canonicalArchivePath(name string) bool {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	return true
}

func validateEntries(entries map[string][]byte) error {
	manifestPayload, found := entries["manifest.json"]
	if !found {
		return fmt.Errorf("%w: manifest.json is missing", errInvalidBundle)
	}
	if int64(len(manifestPayload)) > maxManifestBytes {
		return fmt.Errorf("%w: manifest.json exceeds size limit", errInvalidBundle)
	}
	var decoded manifest
	if err := decodeStrictJSONObject(manifestPayload, &decoded); err != nil {
		return fmt.Errorf("%w: decode manifest: %v", errInvalidBundle, err)
	}
	if decoded.Schema != bundleSchema || !contractSemverPattern.MatchString(decoded.ContractSemver) || decoded.TransportCapability != reviewerprovider.TransportCapability {
		return fmt.Errorf("%w: manifest identity is unsupported", errInvalidBundle)
	}
	if !slices.Equal(decoded.Runtimes, reviewerprovider.RegisteredRuntimeIdentities()) {
		return fmt.Errorf("%w: manifest runtime inventory does not match the registered runtime identities", errInvalidBundle)
	}
	contracts, err := canonicalContracts()
	if err != nil {
		return err
	}
	expected := map[string]struct{}{"README.md": {}, "manifest.json": {}}
	if err := validateFileReference(entries, decoded.README, "README.md", bundleREADME); err != nil {
		return err
	}
	if len(decoded.Roles) != len(contracts) {
		return fmt.Errorf("%w: manifest role inventory is incomplete", errInvalidBundle)
	}
	for index, contract := range contracts {
		role := decoded.Roles[index]
		if index > 0 && decoded.Roles[index-1].ID >= role.ID {
			return fmt.Errorf("%w: manifest roles are not strictly sorted", errInvalidBundle)
		}
		if role.ID != contract.ID || role.RequestSchemaID != contract.RequestSchemaID || role.ResultSchemaID != contract.ResultSchemaID {
			return fmt.Errorf("%w: manifest role %q does not match the canonical registry", errInvalidBundle, role.ID)
		}
		capabilities := slices.Clone(contract.RequiredCapabilities)
		sort.Strings(capabilities)
		if !slices.Equal(role.RequiredCapabilities, capabilities) {
			return fmt.Errorf("%w: manifest role %q capabilities differ", errInvalidBundle, role.ID)
		}
		schemaPath := path.Join("schemas", contract.ID+".schema.json")
		vectorPath := path.Join("vectors", contract.ID+".json")
		if err := validateFileReference(entries, role.Schema, schemaPath, contract.ResultSchema); err != nil {
			return err
		}
		vector, vectorErr := canonicalVector(contract.ResultSchema)
		if vectorErr != nil {
			return fmt.Errorf("%w: canonical vector: %v", errInvalidBundle, vectorErr)
		}
		if err := validateFileReference(entries, role.Vector, vectorPath, vector); err != nil {
			return err
		}
		if err := validateSchemaID(entries[schemaPath], contract.ResultSchemaID); err != nil {
			return err
		}
		expected[schemaPath] = struct{}{}
		expected[vectorPath] = struct{}{}
	}
	orchestrationSources, err := canonicalOrchestrationEntries()
	if err != nil {
		return err
	}
	if len(decoded.Orchestration) != len(orchestrationSources) {
		return fmt.Errorf("%w: manifest orchestration inventory is incomplete", errInvalidBundle)
	}
	for index, source := range orchestrationSources {
		entry := decoded.Orchestration[index]
		if index > 0 && decoded.Orchestration[index-1].Runtime >= entry.Runtime {
			return fmt.Errorf("%w: manifest orchestration entries are not strictly sorted", errInvalidBundle)
		}
		if entry.Runtime != source.runtime {
			return fmt.Errorf("%w: manifest orchestration runtime %q does not match the canonical registry", errInvalidBundle, entry.Runtime)
		}
		if !reviewerprovider.RegisteredRuntime(model.AgentID(entry.Runtime)) {
			return fmt.Errorf("%w: manifest orchestration runtime %q is not a registered review runtime", errInvalidBundle, entry.Runtime)
		}
		filePath := path.Join("orchestration", entry.Runtime+".md")
		if err := validateFileReference(entries, entry.File, filePath, source.content); err != nil {
			return err
		}
		expected[filePath] = struct{}{}
	}
	if len(entries) != len(expected) {
		return fmt.Errorf("%w: archive inventory has %d files, want %d", errInvalidBundle, len(entries), len(expected))
	}
	for name := range entries {
		if _, found := expected[name]; !found {
			return fmt.Errorf("%w: archive has unexpected file %q", errInvalidBundle, name)
		}
	}
	return nil
}

func decodeStrictJSONObject(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := rejectDuplicateJSONKeys(decoder); err != nil {
		return err
	}
	decoder = json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func rejectDuplicateJSONKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return errors.New("JSON document must be an object") // refusal:by-design input: a manifest must be one JSON object.
	}
	if err := rejectDuplicateJSONObjectKeys(decoder); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("multiple JSON values") // refusal:by-design input: a manifest must be one JSON object.
	}
	return nil
}

func rejectDuplicateJSONObjectKeys(decoder *json.Decoder) error {
	keys := map[string]struct{}{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("JSON object key is not a string") // refusal:by-design input: JSON object keys must be strings.
		}
		if _, duplicate := keys[key]; duplicate {
			return fmt.Errorf("duplicate JSON key %q", key)
		}
		keys[key] = struct{}{}
		if err := rejectDuplicateJSONValueKeys(decoder); err != nil {
			return err
		}
	}
	_, err := decoder.Token()
	return err
}

func rejectDuplicateJSONValueKeys(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return rejectDuplicateJSONObjectKeys(decoder)
	case '[':
		for decoder.More() {
			if err := rejectDuplicateJSONValueKeys(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func validateFileReference(entries map[string][]byte, reference fileReference, expectedPath string, expectedPayload []byte) error {
	if reference.Path != expectedPath || !canonicalArchivePath(reference.Path) || !validSHA256(reference.SHA256) {
		return fmt.Errorf("%w: file reference %q is invalid", errInvalidBundle, expectedPath)
	}
	payload, found := entries[reference.Path]
	if !found || hash(payload) != reference.SHA256 || !slices.Equal(payload, expectedPayload) {
		return fmt.Errorf("%w: file reference %q does not match", errInvalidBundle, expectedPath)
	}
	return nil
}

func validateSchemaID(payload []byte, expectedID string) error {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil || document == nil {
		return fmt.Errorf("%w: schema is not a JSON object", errInvalidBundle)
	}
	var schemaID string
	if err := json.Unmarshal(document["$id"], &schemaID); err != nil || schemaID != expectedID {
		return fmt.Errorf("%w: schema ID does not match the canonical contract", errInvalidBundle)
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("multiple JSON values") // refusal:by-design input: a manifest must be one JSON document.
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hash(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func sortedFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
