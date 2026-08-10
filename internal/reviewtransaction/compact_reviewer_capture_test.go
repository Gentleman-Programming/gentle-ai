package reviewtransaction

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type compactReviewerCaptureFixture struct {
	store   CompactStore
	state   CompactState
	request CompactAdmittedReviewerResultRequest
	path    string
}

func newCompactReviewerCaptureFixture(
	t *testing.T,
	lineage string,
) compactReviewerCaptureFixture {
	t.Helper()
	repo := initSnapshotRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "internal"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, "internal", "a.go")
	secondPath := filepath.Join(repo, "internal", "b.go")
	if err := os.WriteFile(
		path,
		[]byte("package internal\n\nfunc Value() int { return 1 }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		secondPath,
		[]byte("package internal\n\nfunc SecondValue() int { return 1 }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	gitSnapshot(t, repo, "add", "--", "internal/a.go", "internal/b.go")
	gitSnapshot(t, repo, "commit", "-m", "add go fixture")
	if err := os.WriteFile(
		path,
		[]byte("package internal\n\nfunc Value() int { return 2 }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		secondPath,
		[]byte("package internal\n\nfunc SecondValue() int { return 2 }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	state := newCompactTestState(t, repo, lineage)
	if len(state.SelectedLenses) != 1 ||
		state.SelectedLenses[0] != LensReliability {
		t.Fatalf("fixture lenses = %v", state.SelectedLenses)
	}
	store, err := CompactAuthoritativeStore(
		context.Background(),
		repo,
		lineage,
	)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.Replace("", "review/start", state)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := (SnapshotBuilder{Repo: repo}).FrozenCandidateContext(
		context.Background(),
		state.InitialSnapshot,
	)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := NewArtifactSubject(
		state,
		revision,
		frozen,
		LensReliability,
		0,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	inspection := ArtifactInspection{
		Status: ArtifactInspectionCompleted,
		Paths:  append([]string(nil), state.InitialSnapshot.Paths...),
	}
	result := LensResult{
		Lens:     LensReliability,
		Findings: []Finding{},
		Evidence: []string{
			"inspected internal/a.go:1 against the complete frozen candidate",
		},
	}
	raw, err := json.Marshal(compactProviderReviewerResult{
		SubjectHash: subject.SubjectHash,
		Inspection:  inspection,
		Lens:        subject.Lens,
		Findings:    result.Findings,
		Evidence:    result.Evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	return compactReviewerCaptureFixture{
		store: store,
		state: state,
		request: CompactAdmittedReviewerResultRequest{
			ExpectedRevision:          revision,
			TargetIdentity:            state.InitialSnapshot.Identity,
			FrozenContext:             frozen,
			ArtifactSubject:           subject,
			Inspection:                inspection,
			Result:                    result,
			CandidateCausalFindingIDs: []string{},
			RawPayload:                raw,
		},
		path: filepath.Join(
			store.Dir,
			CompactReviewerResultsDir,
			"00-"+LensReliability+".json",
		),
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultPublishesDurableExactReplay(
	t *testing.T,
) {
	fixture := newCompactReviewerCaptureFixture(
		t,
		"native-admitted-reviewer",
	)
	got, err := fixture.store.CaptureAdmittedReviewerResult(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultHash == "" ||
		got.ResultHash != LensResultHash(got) ||
		got.Lens != LensReliability {
		t.Fatalf("admitted result = %#v", got)
	}
	payload, digest, err := readCompactReviewerArtifact(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if digest != compactPreservedPayloadDigest(payload) {
		t.Fatalf("artifact digest = %q", digest)
	}
	var envelope compactAdmittedReviewerResult
	decodeStrictReviewerCaptureJSON(t, payload, &envelope)
	if envelope.Schema != AdmittedReviewerResultSchema ||
		envelope.Subject != fixture.request.ArtifactSubject ||
		envelope.Admission.Validate(fixture.request.ArtifactSubject) != nil {
		t.Fatalf("admitted envelope = %#v", envelope)
	}
	var provider compactProviderReviewerResult
	decodeStrictReviewerCaptureJSON(t, envelope.Result, &provider)
	if provider.SubjectHash != fixture.request.ArtifactSubject.SubjectHash ||
		provider.Inspection.Status != ArtifactInspectionCompleted ||
		provider.Lens != LensReliability {
		t.Fatalf("canonical provider result = %#v", provider)
	}
	reAdmitted, ok := reAdmitCompactReviewerResult(
		t.Context(),
		envelope,
		fixture.request.ArtifactSubject,
		fixture.request.FrozenContext,
	)
	if !ok || reAdmitted.ResultHash != got.ResultHash {
		t.Fatalf("re-admitted result = %#v, %t", reAdmitted, ok)
	}
	beforeArtifact := append([]byte(nil), payload...)
	beforeDigest, err := os.ReadFile(fixture.path + ".sha256")
	if err != nil {
		t.Fatal(err)
	}

	replayed, err := fixture.store.CaptureAdmittedReviewerResult(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	afterArtifact, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	afterDigest, err := os.ReadFile(fixture.path + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ResultHash != got.ResultHash ||
		!bytes.Equal(beforeArtifact, afterArtifact) ||
		!bytes.Equal(beforeDigest, afterDigest) {
		t.Fatal("exact replay changed admitted reviewer artifacts")
	}
	if err := os.Remove(fixture.path + ".sha256"); err != nil {
		t.Fatal(err)
	}
	recovered, err := fixture.store.CaptureAdmittedReviewerResult(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatalf("recover missing digest sidecar: %v", err)
	}
	recoveredDigest, err := os.ReadFile(fixture.path + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ResultHash != got.ResultHash ||
		!bytes.Equal(recoveredDigest, beforeDigest) {
		t.Fatal("exact replay did not recover the missing digest sidecar")
	}
	record, err := fixture.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != fixture.request.ExpectedRevision ||
		record.State.State != StateReviewing {
		t.Fatalf("capture mutated compact authority = %#v", record)
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultNormalizesInspectionPathsBeforePersisting(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "capture-reordered-inspection-replay")
	first, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}

	reordered := fixture.request
	reordered.Inspection.Paths = []string{
		fixture.request.Inspection.Paths[1],
		fixture.request.Inspection.Paths[0],
	}
	replayed, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), reordered)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("reordered complete inspection replay = %#v, %v; want %#v, nil", replayed, err, first)
	}
	afterReplay, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterReplay, persisted) {
		t.Fatal("reordered complete inspection replay changed the persisted result")
	}

	conflicting := reordered
	conflicting.Result.Evidence = append([]string(nil), reordered.Result.Evidence...)
	conflicting.Result.Evidence = append(conflicting.Result.Evidence, "independent verification inspected internal/b.go:1")
	raw, err := json.Marshal(compactProviderReviewerResult{
		SubjectHash: conflicting.ArtifactSubject.SubjectHash,
		Inspection:  conflicting.Inspection,
		Lens:        conflicting.ArtifactSubject.Lens,
		Findings:    conflicting.Result.Findings,
		Evidence:    conflicting.Result.Evidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	conflicting.RawPayload = append(raw, '\n')
	if _, err := fixture.store.CaptureAdmittedReviewerResult(t.Context(), conflicting); err == nil || !strings.Contains(err.Error(), "different canonical bytes") {
		t.Fatalf("conflicting provider payload error = %v", err)
	}
	afterConflict, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterConflict, persisted) {
		t.Fatal("conflicting provider payload changed the persisted result")
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultConvergesAfterExactReplayLockTimeout(t *testing.T) {
	fixture := newCompactReviewerCaptureFixture(t, "capture-timeout-replay")
	want, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	held, err := acquireStoreLock(fixture.store.lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	reordered := fixture.request
	reordered.Inspection.Paths = []string{
		fixture.request.Inspection.Paths[1],
		fixture.request.Inspection.Paths[0],
	}

	got, err := fixture.store.CaptureAdmittedReviewerResult(context.Background(), reordered)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("reordered complete inspection replay behind held store lock = %#v, %v; want %#v", got, err, want)
	}

	payload, _, err := readCompactReviewerArtifact(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope compactAdmittedReviewerResult
	decodeStrictReviewerCaptureJSON(t, payload, &envelope)
	alternateAdmission := envelope.Admission
	alternateAdmission.RawSHA256 = payloadSHA256(append([]byte("different transport\n"), fixture.request.RawPayload...))
	if result, found, err := fixture.store.resolveAdmittedReviewerResult(
		context.Background(), fixture.request.ExpectedRevision, fixture.request.TargetIdentity,
		fixture.request.FrozenContext, fixture.request.ArtifactSubject, &alternateAdmission,
	); err == nil || found || !reflect.DeepEqual(result, LensResult{}) {
		t.Fatalf("different raw authority resolved as exact replay: result=%#v found=%t err=%v", result, found, err)
	}
}

func TestCompactStoreResolveAdmittedReviewerResultIsExactAndReadOnly(
	t *testing.T,
) {
	fixture := newCompactReviewerCaptureFixture(
		t,
		"resolve-native-admitted-reviewer",
	)
	stateBefore, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	missing, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	)
	if err != nil || found || !reflect.DeepEqual(missing, LensResult{}) {
		t.Fatalf("missing admitted result = %#v, %t, %v", missing, found, err)
	}
	if _, err := os.Lstat(filepath.Dir(fixture.path)); !errors.Is(
		err,
		fs.ErrNotExist,
	) {
		t.Fatalf("read-only miss created reviewer directory: %v", err)
	}
	want, err := fixture.store.CaptureAdmittedReviewerResult(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatal(err)
	}
	artifactBefore, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	digestBefore, err := os.ReadFile(fixture.path + ".sha256")
	if err != nil {
		t.Fatal(err)
	}

	got, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	)
	if err != nil || !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved admitted result = %#v, %t, %v", got, found, err)
	}
	stateAfter, err := os.ReadFile(fixture.store.StatePath())
	if err != nil {
		t.Fatal(err)
	}
	artifactAfter, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	digestAfter, err := os.ReadFile(fixture.path + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stateBefore, stateAfter) ||
		!bytes.Equal(artifactBefore, artifactAfter) ||
		!bytes.Equal(digestBefore, digestAfter) {
		t.Fatal("resolver changed compact authority or reviewer artifacts")
	}
}

func TestCompactStoreResolveAdmittedReviewerResultFailsClosed(
	t *testing.T,
) {
	fixture := newCompactReviewerCaptureFixture(
		t,
		"resolve-admitted-reviewer-refusal",
	)
	if _, err := fixture.store.CaptureAdmittedReviewerResult(
		context.Background(),
		fixture.request,
	); err != nil {
		t.Fatal(err)
	}
	if _, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		verificationTestHash("different-review-target"),
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	); err == nil || found {
		t.Fatalf("mismatched target = found %t, error %v", found, err)
	}
	tamperedFrozen := fixture.request.FrozenContext
	tamperedFrozen.ChangedPathManifest = append(
		[]ChangedPathManifestEntry(nil),
		tamperedFrozen.ChangedPathManifest...,
	)
	tamperedFrozen.ChangedPathManifest[0].ModeOnly = true
	if _, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		tamperedFrozen,
		fixture.request.ArtifactSubject,
	); err == nil || found {
		t.Fatalf("tampered frozen context = found %t, error %v", found, err)
	}

	payload, err := os.ReadFile(fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(
		append([]byte(nil), bytes.TrimSuffix(payload, []byte("}\n"))...),
		[]byte(",\"unexpected\":true}\n")...,
	)
	if err := os.WriteFile(fixture.path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		fixture.path+".sha256",
		[]byte(compactPreservedPayloadDigest(payload)+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if _, found, err := fixture.store.ResolveAdmittedReviewerResult(
		context.Background(),
		fixture.request.ExpectedRevision,
		fixture.request.TargetIdentity,
		fixture.request.FrozenContext,
		fixture.request.ArtifactSubject,
	); err == nil || found {
		t.Fatalf("unknown-field artifact = found %t, error %v", found, err)
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultRejectsUnsafeOrConflictingSlots(
	t *testing.T,
) {
	tests := []struct {
		name       string
		arrange    func(*testing.T, compactReviewerCaptureFixture)
		wantUnsafe bool
	}{
		{
			name: "different immutable artifact",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				writePrivateReviewerCaptureFile(
					t,
					fixture.path,
					[]byte("different\n"),
					0o600,
				)
			},
		},
		{
			name: "conflicting digest sidecar",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				writePrivateReviewerCaptureFile(
					t,
					fixture.path+".sha256",
					[]byte("sha256:"+strings.Repeat("9", 64)+"\n"),
					0o600,
				)
			},
		},
		{
			name: "symlinked artifact",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				if runtime.GOOS == "windows" {
					t.Skip("symlink creation requires optional Windows privileges")
				}
				if _, err := createPrivateRARDirectory(
					filepath.Dir(fixture.path),
				); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("outside\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, fixture.path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
			wantUnsafe: true,
		},
		{
			name: "hardlinked artifact",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				if _, err := createPrivateRARDirectory(
					filepath.Dir(fixture.path),
				); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("outside\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, fixture.path); err != nil {
					t.Skipf("hardlink unavailable: %v", err)
				}
			},
			wantUnsafe: true,
		},
		{
			name: "hardlinked digest sidecar",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				if _, err := createPrivateRARDirectory(
					filepath.Dir(fixture.path),
				); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("outside\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(target, fixture.path+".sha256"); err != nil {
					t.Skipf("hardlink unavailable: %v", err)
				}
			},
			wantUnsafe: true,
		},
		{
			name: "symlinked reviewer directory",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				if runtime.GOOS == "windows" {
					t.Skip("symlink creation requires optional Windows privileges")
				}
				target := t.TempDir()
				if err := os.Symlink(
					target,
					filepath.Dir(fixture.path),
				); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			},
			wantUnsafe: true,
		},
		{
			name: "group-readable artifact",
			arrange: func(t *testing.T, fixture compactReviewerCaptureFixture) {
				if runtime.GOOS == "windows" {
					t.Skip("POSIX mode fixture")
				}
				writePrivateReviewerCaptureFile(
					t,
					fixture.path,
					[]byte("unsafe\n"),
					0o640,
				)
			},
			wantUnsafe: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCompactReviewerCaptureFixture(
				t,
				"capture-refusal-"+strings.ReplaceAll(test.name, " ", "-"),
			)
			test.arrange(t, fixture)
			_, err := fixture.store.CaptureAdmittedReviewerResult(
				context.Background(),
				fixture.request,
			)
			if err == nil {
				t.Fatal("unsafe/conflicting capture succeeded")
			}
			if test.wantUnsafe && !errors.Is(err, errUnsafeRARAuthorityPath) {
				t.Fatalf("unsafe capture error = %v", err)
			}
			if _, statErr := os.Lstat(fixture.path + ".sha256"); test.name != "conflicting digest sidecar" &&
				test.name != "hardlinked digest sidecar" &&
				!errors.Is(statErr, fs.ErrNotExist) {
				t.Fatalf("refused capture published digest sidecar: %v", statErr)
			}
			if test.name == "conflicting digest sidecar" {
				if _, statErr := os.Lstat(fixture.path); !errors.Is(statErr, fs.ErrNotExist) {
					t.Fatalf("sidecar conflict published artifact: %v", statErr)
				}
			}
		})
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultRejectsCallerDerivedContext(
	t *testing.T,
) {
	fixture := newCompactReviewerCaptureFixture(
		t,
		"capture-rederive-frozen-context",
	)
	tampered := fixture.request
	tampered.FrozenContext.ChangedPathManifest = append(
		[]ChangedPathManifestEntry(nil),
		tampered.FrozenContext.ChangedPathManifest...,
	)
	tampered.FrozenContext.ChangedPathManifest[0].ModeOnly = true
	if _, err := fixture.store.CaptureAdmittedReviewerResult(
		context.Background(),
		tampered,
	); err == nil || !strings.Contains(
		err.Error(),
		"does not match repository authority",
	) {
		t.Fatalf("caller-derived frozen context error = %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(fixture.path)); !errors.Is(
		err,
		fs.ErrNotExist,
	) {
		t.Fatalf("context refusal published reviewer directory: %v", err)
	}
}

func TestCompactStoreCaptureRefusesInvalidLocationsWithoutConsumingTheSlot(t *testing.T) {
	for _, tt := range []struct {
		location string
		reason   FindingLocationErrorReason
	}{
		{"internal/a.go:1-2,4", FindingLocationLineNotInteger},
		{"internal/a.go:3-2", FindingLocationErrorReason("range_must_be_ascending")},
		{"internal/a.go:0-1", FindingLocationLineNotPositive},
		{"internal/a.go:" + strings.Repeat("9", 64), FindingLocationErrorReason("line_overflows_integer")},
		{"internal/a.go:9223372036854775808", FindingLocationErrorReason("line_overflows_integer")},
		{"internal/a.go:18446744073709551615", FindingLocationErrorReason("line_overflows_integer")},
		{"internal/a.go", FindingLocationExpectedPathAndLine},
	} {
		t.Run(tt.location, func(t *testing.T) {
			fixture := newCompactReviewerCaptureFixture(t, "capture-invalid-location")
			before, err := fixture.store.Load()
			if err != nil {
				t.Fatal(err)
			}
			fixture.request.Result.Findings = []Finding{{
				ID: "R3-001", Lens: "reliability", Location: tt.location, Severity: "WARNING",
				Claim: "invalid location", ProofRefs: []string{"internal/a.go:1"},
			}}
			if _, err = fixture.store.CaptureAdmittedReviewerResult(t.Context(), fixture.request); err == nil {
				t.Fatal("CaptureAdmittedReviewerResult() succeeded")
			} else {
				var locationErr *FindingLocationError
				if !errors.As(err, &locationErr) || locationErr.Reason != tt.reason {
					t.Fatalf("CaptureAdmittedReviewerResult() error = %v; want %q", err, tt.reason)
				}
			}
			after, err := fixture.store.Load()
			if err != nil || !reflect.DeepEqual(after, before) {
				t.Fatalf("refused capture changed authority: before=%#v after=%#v err=%v", before, after, err)
			}
			if _, err := os.Lstat(fixture.path); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("refused capture consumed reviewer slot: %v", err)
			}
		})
	}
}

func TestCompactStoreCaptureAdmittedReviewerResultSerializesConcurrentReplayAndConflict(
	t *testing.T,
) {
	t.Run("exact replay", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(
			t,
			"capture-concurrent-replay",
		)
		const attempts = 8
		var wait sync.WaitGroup
		errorsByAttempt := make([]error, attempts)
		hashes := make([]string, attempts)
		for index := 0; index < attempts; index++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				result, err := fixture.store.CaptureAdmittedReviewerResult(
					context.Background(),
					fixture.request,
				)
				errorsByAttempt[index] = err
				hashes[index] = result.ResultHash
			}(index)
		}
		wait.Wait()
		for index := range errorsByAttempt {
			if errorsByAttempt[index] != nil ||
				hashes[index] == "" ||
				hashes[index] != hashes[0] {
				t.Fatalf(
					"concurrent replay[%d] = hash %q, error %v",
					index,
					hashes[index],
					errorsByAttempt[index],
				)
			}
		}
		if _, _, err := readCompactReviewerArtifact(fixture.path); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("different raw authority", func(t *testing.T) {
		fixture := newCompactReviewerCaptureFixture(
			t,
			"capture-concurrent-conflict",
		)
		alternate := fixture.request
		alternate.RawPayload = append(
			[]byte("review transport prefix\n"),
			alternate.RawPayload...,
		)
		requests := []CompactAdmittedReviewerResultRequest{
			fixture.request,
			alternate,
		}
		var wait sync.WaitGroup
		results := make([]error, len(requests))
		for index := range requests {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				_, results[index] = fixture.store.CaptureAdmittedReviewerResult(
					context.Background(),
					requests[index],
				)
			}(index)
		}
		wait.Wait()
		successes, conflicts := 0, 0
		for _, err := range results {
			switch {
			case err == nil:
				successes++
			case strings.Contains(
				err.Error(),
				"different canonical bytes",
			):
				conflicts++
			default:
				t.Fatalf("concurrent conflict error = %v", err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf(
				"concurrent conflict = %d success, %d conflict",
				successes,
				conflicts,
			)
		}
	})
}

func decodeStrictReviewerCaptureJSON(
	t *testing.T,
	payload []byte,
	value any,
) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("extra JSON value: %v", err)
	}
}

func writePrivateReviewerCaptureFile(
	t *testing.T,
	path string,
	payload []byte,
	mode fs.FileMode,
) {
	t.Helper()
	if _, err := createPrivateRARDirectory(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, mode); err != nil {
		t.Fatal(err)
	}
}
