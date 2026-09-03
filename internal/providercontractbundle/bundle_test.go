package providercontractbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v2/internal/components/sdd"
	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

func TestGenerateIsDeterministicAndVerifiable(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	for _, directory := range []string{first, second} {
		if err := Generate(directory, "1.0.0"); err != nil {
			t.Fatalf("Generate(%s): %v", directory, err)
		}
	}
	firstFiles := readGeneratedFiles(t, first)
	secondFiles := readGeneratedFiles(t, second)
	wantNames := []string{
		"README.md",
		"manifest.json",
		"orchestration/pi.md",
		"schemas/lens.schema.json",
		"schemas/refuter.schema.json",
		"schemas/targeted-validator.schema.json",
		"vectors/lens.json",
		"vectors/refuter.json",
		"vectors/targeted-validator.json",
	}
	if got := sortedFileNames(firstFiles); !equalStrings(got, wantNames) {
		t.Fatalf("generated files = %v, want %v", got, wantNames)
	}
	for _, name := range wantNames {
		if !bytes.Equal(firstFiles[name], secondFiles[name]) {
			t.Fatalf("generated %s differs between runs", name)
		}
		info, err := os.Stat(filepath.Join(first, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		// Windows cannot represent 0644: Chmod only honors the write bit and
		// Stat reports 0666. Bundle determinism is content-level (asserted
		// above) and published archives build on Linux; the staging mode is
		// best-effort metadata, so the expectation is per-OS (#3229).
		wantMode := fs.FileMode(bundleFileMode)
		if runtime.GOOS == "windows" {
			wantMode = 0o666
		}
		if info.Mode().Perm() != wantMode || !info.ModTime().Equal(bundleTimestamp) {
			t.Fatalf("generated %s metadata = %s %s, want %04o %s", name, info.Mode(), info.ModTime(), wantMode, bundleTimestamp)
		}
	}
	if err := VerifyStaging(first); err != nil {
		t.Fatalf("VerifyStaging(%s): %v", first, err)
	}
	archives := []string{
		filepath.Join(t.TempDir(), "first.tar.gz"),
		filepath.Join(t.TempDir(), "second.tar.gz"),
	}
	writeArchive(t, archives[0], firstFiles, nil, false)
	writeArchive(t, archives[1], secondFiles, nil, false)
	if err := VerifyArchive(archives[0]); err != nil {
		t.Fatalf("VerifyArchive(%s): %v", archives[0], err)
	}
	firstArchive, err := os.ReadFile(archives[0])
	if err != nil {
		t.Fatal(err)
	}
	secondArchive, err := os.ReadFile(archives[1])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstArchive, secondArchive) {
		t.Fatal("independent generated staging archives differ")
	}
}

func TestVerifyArchiveRejectsUnsafeTarEntries(t *testing.T) {
	files, err := generatedFiles("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		duplicate bool
		mutate    func(map[string][]byte, *tar.Header)
	}{
		{name: "parent traversal", mutate: func(_ map[string][]byte, header *tar.Header) { header.Name = "../README.md" }},
		{name: "absolute path", mutate: func(_ map[string][]byte, header *tar.Header) { header.Name = "/README.md" }},
		{name: "dot path", mutate: func(_ map[string][]byte, header *tar.Header) { header.Name = "." }},
		{name: "backslash path", mutate: func(_ map[string][]byte, header *tar.Header) { header.Name = `schemas\lens.schema.json` }},
		{name: "noncanonical path", mutate: func(_ map[string][]byte, header *tar.Header) { header.Name = "schemas//lens.schema.json" }},
		{name: "duplicate path", duplicate: true, mutate: func(_ map[string][]byte, _ *tar.Header) {}},
		{name: "symlink", mutate: func(_ map[string][]byte, header *tar.Header) {
			header.Typeflag = tar.TypeSymlink
			header.Linkname = "README.md"
		}},
		{name: "hard link", mutate: func(_ map[string][]byte, header *tar.Header) {
			header.Typeflag = tar.TypeLink
			header.Linkname = "README.md"
		}},
		{name: "directory", mutate: func(_ map[string][]byte, header *tar.Header) { header.Typeflag = tar.TypeDir }},
		{name: "fifo", mutate: func(_ map[string][]byte, header *tar.Header) { header.Typeflag = tar.TypeFifo }},
		{name: "character device", mutate: func(_ map[string][]byte, header *tar.Header) { header.Typeflag = tar.TypeChar }},
		{name: "block device", mutate: func(_ map[string][]byte, header *tar.Header) { header.Typeflag = tar.TypeBlock }},
		{name: "unknown type", mutate: func(_ map[string][]byte, header *tar.Header) { header.Typeflag = 'Z' }},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "unsafe.tar.gz")
			copy := cloneFiles(files)
			header := &tar.Header{Name: "README.md"}
			test.mutate(copy, header)
			writeArchive(t, archive, copy, header, test.duplicate)
			if err := VerifyArchive(archive); err == nil {
				t.Fatal("VerifyArchive accepted an unsafe archive")
			}
		})
	}
}

func TestVerifyArchiveAcceptsTypeRegA(t *testing.T) {
	files, err := generatedFiles("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "regular-a.tar.gz")
	writeArchive(t, archive, files, &tar.Header{Name: "README.md", Typeflag: tar.TypeRegA}, false)
	if err := VerifyArchive(archive); err != nil {
		t.Fatalf("VerifyArchive rejected TypeRegA: %v", err)
	}
}

func TestVerifyArchiveRejectsHiddenOrAmbiguousArchiveContent(t *testing.T) {
	files, err := generatedFiles("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		write func(*testing.T, string, map[string][]byte)
	}{
		{
			name: "duplicate manifest key",
			write: func(t *testing.T, archive string, files map[string][]byte) {
				var document map[string]any
				if err := json.Unmarshal(files["manifest.json"], &document); err != nil {
					t.Fatal(err)
				}
				payload, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				files["manifest.json"] = append([]byte(`{"schema":"wrong",`), payload[1:]...)
				writeArchive(t, archive, files, nil, false)
			},
		},
		{
			name: "nested duplicate manifest key",
			write: func(t *testing.T, archive string, files map[string][]byte) {
				payload := bytes.Replace(files["manifest.json"], []byte(`"readme": {`), []byte(`"readme": {"path":"wrong",`), 1)
				if bytes.Equal(payload, files["manifest.json"]) {
					t.Fatal("could not create nested duplicate manifest key")
				}
				files["manifest.json"] = payload
				writeArchive(t, archive, files, nil, false)
			},
		},
		{
			name: "array-contained duplicate manifest key",
			write: func(t *testing.T, archive string, files map[string][]byte) {
				payload := bytes.Replace(files["manifest.json"], []byte(`"id": "lens",`), []byte(`"id": "wrong",`+"\n"+`      "id": "lens",`), 1)
				if bytes.Equal(payload, files["manifest.json"]) {
					t.Fatal("could not create array-contained duplicate manifest key")
				}
				files["manifest.json"] = payload
				writeArchive(t, archive, files, nil, false)
			},
		},
		{
			name: "PAX extended header",
			write: func(t *testing.T, archive string, files map[string][]byte) {
				writeArchiveWithPAX(t, archive, files, 128)
			},
		},
		{
			name: "concatenated gzip member",
			write: func(t *testing.T, archive string, files map[string][]byte) {
				writeArchive(t, archive, files, nil, false)
				appendGzipZeroMember(t, archive, 64<<20)
			},
		},
		{
			name: "trailing compressed data",
			write: func(t *testing.T, archive string, files map[string][]byte) {
				writeArchive(t, archive, files, nil, false)
				appendRawData(t, archive, []byte("not a gzip member"))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "ambiguous.tar.gz")
			test.write(t, archive, cloneFiles(files))
			if err := VerifyArchive(archive); err == nil {
				t.Fatal("VerifyArchive accepted hidden or ambiguous archive content")
			}
		})
	}
}

func TestVerifyArchiveRejectsDamagedAndExpandedStreams(t *testing.T) {
	files, err := generatedFiles("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		write func(*testing.T, string)
	}{
		{
			name: "gzip checksum failure",
			write: func(t *testing.T, archive string) {
				writeArchive(t, archive, cloneFiles(files), nil, false)
				mutateArchive(t, archive, func(payload []byte) []byte {
					payload[len(payload)-1] ^= 0xff
					return payload
				})
			},
		},
		{
			name: "truncated gzip stream",
			write: func(t *testing.T, archive string) {
				writeArchive(t, archive, cloneFiles(files), nil, false)
				mutateArchive(t, archive, func(payload []byte) []byte { return payload[:len(payload)-1] })
			},
		},
		{
			name: "expanded regular payload limit",
			write: func(t *testing.T, archive string) {
				entries := make([]rawTarEntry, 0, 5)
				for index := 0; index < 5; index++ {
					entries = append(entries, rawTarEntry{
						name:     fmt.Sprintf("payload-%d", index),
						typeflag: tar.TypeReg,
						payload:  make([]byte, maxFileBytes),
					})
				}
				writeRawTarArchive(t, archive, entries)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "damaged.tar.gz")
			test.write(t, archive)
			if err := VerifyArchive(archive); err == nil {
				t.Fatal("VerifyArchive accepted a damaged or oversized stream")
			}
		})
	}
}

func TestVerifyArchiveRejectsOversizedPAXMetadata(t *testing.T) {
	files, err := generatedFiles("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "oversized-pax.tar.gz")
	writeArchiveWithPAX(t, archive, files, 17<<20)
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("PAX archive was empty")
	}
	if err := VerifyArchive(archive); err == nil {
		t.Fatal("VerifyArchive accepted PAX metadata")
	}
}

func TestReadContractSemverRejectsNonCanonicalInput(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "CONTRACT_SEMVER")
	for _, value := range []string{"v1.0.0\n", "1.0\n", "01.0.0\n", "1.0.0"} {
		if err := os.WriteFile(filename, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadContractSemver(filename); err == nil {
			t.Fatalf("ReadContractSemver accepted %q", value)
		}
	}
}

func readGeneratedFiles(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	err := filepath.WalkDir(directory, func(filename string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(directory, filename)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = payload
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func writeArchive(t *testing.T, filename string, files map[string][]byte, override *tar.Header, duplicate bool) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	gzipWriter.Name = ""
	gzipWriter.Comment = ""
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	names := sortedFileNames(files)
	for _, name := range names {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(files[name])), Typeflag: tar.TypeReg}
		if override != nil && name == "README.md" {
			header.Typeflag = override.Typeflag
			header.Name = override.Name
			header.Linkname = override.Linkname
			if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
				header.Size = 0
			}
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := tarWriter.Write(files[name]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if duplicate {
		payload := files["README.md"]
		if err := tarWriter.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeArchiveWithPAX(t *testing.T, filename string, files map[string][]byte, metadataSize int) {
	t.Helper()
	metadata := paxRecord("comment", string(bytes.Repeat([]byte("x"), metadataSize)))
	entries := []rawTarEntry{{name: "PaxHeader", typeflag: tar.TypeXHeader, payload: metadata}}
	for _, name := range sortedFileNames(files) {
		entries = append(entries, rawTarEntry{name: name, typeflag: tar.TypeReg, payload: files[name]})
	}
	writeRawTarArchive(t, filename, entries)
}

type rawTarEntry struct {
	name     string
	typeflag byte
	payload  []byte
}

func writeRawTarArchive(t *testing.T, filename string, entries []rawTarEntry) {
	t.Helper()
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	var payload bytes.Buffer
	for _, entry := range entries {
		writeRawTarEntry(t, &payload, entry.name, entry.typeflag, entry.payload)
	}
	payload.Write(make([]byte, 1024))
	if _, err := io.Copy(gzipWriter, &payload); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeRawTarEntry(t *testing.T, destination io.Writer, name string, typeflag byte, payload []byte) {
	t.Helper()
	header := make([]byte, 512)
	copy(header[0:100], name)
	copy(header[100:108], []byte("0000644\x00"))
	copy(header[108:116], []byte("0000000\x00"))
	copy(header[116:124], []byte("0000000\x00"))
	copy(header[124:136], []byte(fmt.Sprintf("%011o\x00", len(payload))))
	copy(header[136:148], []byte("00000000000\x00"))
	for index := 148; index < 156; index++ {
		header[index] = ' '
	}
	header[156] = typeflag
	copy(header[257:263], []byte("ustar\x00"))
	copy(header[263:265], []byte("00"))
	checksum := 0
	for _, value := range header {
		checksum += int(value)
	}
	copy(header[148:156], []byte(fmt.Sprintf("%06o\x00 ", checksum)))
	if _, err := destination.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, err := destination.Write(payload); err != nil {
		t.Fatal(err)
	}
	if padding := len(payload) % 512; padding != 0 {
		if _, err := destination.Write(make([]byte, 512-padding)); err != nil {
			t.Fatal(err)
		}
	}
}

func paxRecord(key, value string) []byte {
	content := key + "=" + value + "\n"
	for length := len(content) + 2; ; length++ {
		record := fmt.Sprintf("%d %s", length, content)
		if len(record) == length {
			return []byte(record)
		}
	}
}

func appendGzipZeroMember(t *testing.T, filename string, expandedBytes int) {
	t.Helper()
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	chunk := make([]byte, 1<<20)
	for written := 0; written < expandedBytes; {
		next := min(len(chunk), expandedBytes-written)
		if _, err := gzipWriter.Write(chunk[:next]); err != nil {
			t.Fatal(err)
		}
		written += next
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func appendRawData(t *testing.T, filename string, payload []byte) {
	t.Helper()
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func mutateArchive(t *testing.T, filename string, mutate func([]byte) []byte) {
	t.Helper()
	payload, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	payload = mutate(payload)
	if err := os.WriteFile(filename, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func cloneFiles(files map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(files))
	for name, payload := range files {
		result[name] = append([]byte(nil), payload...)
	}
	return result
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}

// TestGeneratedOrchestrationEntryCarriesTheBoundPiContract locks the content
// gentle-pi mirrors (issue #4056): the orchestration file must be Pi's exact
// bound review execution contract, with the runtime placeholder resolved and
// its capture command named exactly once.
func TestGeneratedOrchestrationEntryCarriesTheBoundPiContract(t *testing.T) {
	files, err := generatedFiles("1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	content, found := files["orchestration/pi.md"]
	if !found {
		t.Fatal("generated files are missing orchestration/pi.md")
	}
	text := string(content)
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("orchestration/pi.md does not end with a trailing newline")
	}
	if strings.Contains(text, "{{GENTLE_AI_RUNTIME_AGENT_ID}}") {
		t.Fatal("orchestration/pi.md left the runtime identity placeholder unbound")
	}
	for _, want := range []string{
		"`gentle_review` with {\"operation\":\"inspect\"}",
		"`gentle_review` with operation `status`, the exact retained `lineageId`, and `workspaceRoot` only when needed",
		"`gentle_review_capture_group`",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("orchestration/pi.md missing Pi facade route %q", want)
		}
	}
	if strings.Contains(text, "gentle-ai review status") {
		t.Fatal("orchestration/pi.md exposes raw STATUS")
	}
	if !strings.Contains(text, "## Entry rule") {
		t.Fatal("orchestration/pi.md is missing the `## Entry rule` heading")
	}
}

// TestGenerateThenVerifyRoundTripsTheOrchestrationManifestEntry is the
// generate-then-verify roundtrip strict TDD requires: the manifest must carry
// a sorted orchestration entry naming pi and its file reference, and Verify
// must accept the archive built from exactly those bytes.
func TestPiFacadeLifecycleValidation(t *testing.T) {
	valid, err := sdd.ReviewExecutionContractFor(model.AgentPi)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		content string
		valid   bool
	}{
		{name: "complete facade lifecycle", content: valid, valid: true},
		{name: "missing facade status", content: strings.Replace(valid, "`gentle_review` with operation `status`, the exact retained `lineageId`, and `workspaceRoot` only when needed", "", 1)},
		{name: "missing single capture", content: strings.ReplaceAll(valid, "`gentle_review_capture`", "")},
		{name: "missing group capture", content: strings.ReplaceAll(valid, "`gentle_review_capture_group`", "")},
		{name: "missing approved acknowledgement", content: strings.Replace(valid, "Only the exact provider-issued acknowledgement continuation burns approved authority.", "", 1)},
		{name: "missing public facade acknowledgement operation", content: strings.Replace(valid, "`acknowledge-approved` continuation", "`replacement` continuation", 1), valid: false},
		{name: "missing answer-consent route", content: strings.Replace(valid, "`gentle_review` with operation `answer-consent` and the exact `consentBinding`", "", 1), valid: false},
		{name: "missing forecast acknowledgement", content: strings.Replace(valid, "resubmit the same exact binding with `reviewerRunAcknowledged: true`", "", 1), valid: false},
		{name: "user-owned mode switch is allowed", content: valid + "\ngentle-ai review mode enable --scope global\n", valid: true},
		{name: "raw status", content: valid + "\ngentle-ai review status\n"},
		{name: "raw capture", content: valid + "\ngentle-ai review capture-result\n"},
		{name: "raw acknowledgement", content: valid + "\ngentle-ai review acknowledge-approved\n"},
		{name: "raw recover", content: valid + "\ngentle-ai review recover --lineage x\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := validPiFacadeLifecycle(test.content); got != test.valid {
				t.Fatalf("validPiFacadeLifecycle() = %t, want %t", got, test.valid)
			}
		})
	}
}

func TestGenerateThenVerifyRoundTripsTheOrchestrationManifestEntry(t *testing.T) {
	directory := t.TempDir()
	if err := Generate(directory, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	files := readGeneratedFiles(t, directory)
	var decoded manifest
	if err := json.Unmarshal(files["manifest.json"], &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Orchestration) != 1 {
		t.Fatalf("manifest orchestration entries = %d, want 1", len(decoded.Orchestration))
	}
	entry := decoded.Orchestration[0]
	if entry.Runtime != "pi" {
		t.Fatalf("manifest orchestration runtime = %q, want %q", entry.Runtime, "pi")
	}
	if entry.File.Path != "orchestration/pi.md" || hash(files["orchestration/pi.md"]) != entry.File.SHA256 {
		t.Fatalf("manifest orchestration file reference = %+v, does not match generated content", entry.File)
	}

	archive := filepath.Join(t.TempDir(), "bundle.tar.gz")
	writeArchive(t, archive, files, nil, false)
	if err := VerifyArchive(archive); err != nil {
		t.Fatalf("VerifyArchive: %v", err)
	}
	if err := VerifyStaging(directory); err != nil {
		t.Fatalf("VerifyStaging: %v", err)
	}
}

// TestVerifyArchiveRejectsATamperedOrchestrationFile proves the orchestration
// entry is protected by the same sha256 binding as every other bundle file.
func TestVerifyArchiveRejectsATamperedOrchestrationFile(t *testing.T) {
	files, err := generatedFiles("1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	tampered := cloneFiles(files)
	tampered["orchestration/pi.md"] = append(tampered["orchestration/pi.md"], '\n')
	archive := filepath.Join(t.TempDir(), "tampered-orchestration.tar.gz")
	writeArchive(t, archive, tampered, nil, false)
	if err := VerifyArchive(archive); err == nil {
		t.Fatal("VerifyArchive accepted a tampered orchestration/pi.md byte")
	}
}

// TestVerifyArchiveRejectsAManifestMissingTheOrchestrationEntry proves an old
// 1.1.0-shaped manifest (no orchestration section) is refused once the bundle
// carries orchestration/pi.md: VerifyArchive is only ever run against the
// archive generated from the same commit's CONTRACT_SEMVER (release preflight
// and the README's manual-inspection instructions), so there is no caller
// depending on the older layout staying acceptable.
func TestVerifyArchiveRejectsAManifestMissingTheOrchestrationEntry(t *testing.T) {
	files, err := generatedFiles("1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	stripped := cloneFiles(files)
	delete(stripped, "orchestration/pi.md")
	var decoded map[string]any
	if err := json.Unmarshal(files["manifest.json"], &decoded); err != nil {
		t.Fatal(err)
	}
	delete(decoded, "orchestration")
	payload, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	stripped["manifest.json"] = append(payload, '\n')
	archive := filepath.Join(t.TempDir(), "missing-orchestration.tar.gz")
	writeArchive(t, archive, stripped, nil, false)
	if err := VerifyArchive(archive); err == nil {
		t.Fatal("VerifyArchive accepted a manifest without the orchestration entry")
	}
}

// TestVerifyArchiveRejectsAnUnregisteredOrchestrationRuntime proves the
// orchestration runtime is checked against the closed registered-runtime
// identity list, the same way every other runtime-scoped claim in this bundle
// is, rather than trusted from the manifest bytes alone.
func TestVerifyArchiveRejectsAnUnregisteredOrchestrationRuntime(t *testing.T) {
	files, err := generatedFiles("1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	tampered := cloneFiles(files)
	payload := bytes.Replace(tampered["manifest.json"], []byte(`"runtime": "pi"`), []byte(`"runtime": "not-a-runtime"`), 1)
	if bytes.Equal(payload, tampered["manifest.json"]) {
		t.Fatal("could not rewrite the orchestration runtime identity")
	}
	tampered["manifest.json"] = payload
	archive := filepath.Join(t.TempDir(), "unregistered-orchestration-runtime.tar.gz")
	writeArchive(t, archive, tampered, nil, false)
	if err := VerifyArchive(archive); err == nil {
		t.Fatal("VerifyArchive accepted an unregistered orchestration runtime")
	}
}
