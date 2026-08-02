package reviewtransaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSelectReviewLensesOwnsDeterministicZeroOneFourPolicy(t *testing.T) {
	tests := []struct {
		name       string
		assessment RiskAssessment
		focus      string
		want       []string
		wantErr    bool
	}{
		{name: "low is zero", assessment: RiskAssessment{Level: RiskLow}, focus: "irrelevant", want: []string{}},
		{name: "medium risk", assessment: RiskAssessment{Level: RiskMedium}, focus: " risk ", want: []string{LensRisk}},
		{name: "medium resilience", assessment: RiskAssessment{Level: RiskMedium}, focus: "resilience", want: []string{LensResilience}},
		{name: "medium readability", assessment: RiskAssessment{Level: RiskMedium}, focus: "readability", want: []string{LensReadability}},
		{name: "medium reliability", assessment: RiskAssessment{Level: RiskMedium}, focus: "reliability", want: []string{LensReliability}},
		{
			name: "high is canonical four", assessment: RiskAssessment{Level: RiskHigh}, focus: "irrelevant",
			want: []string{LensRisk, LensResilience, LensReadability, LensReliability},
		},
		{
			name: "large docs force readability", assessment: RiskAssessment{
				Level: RiskMedium, DominantLens: LensReadability,
			}, focus: "risk", want: []string{LensReadability},
		},
		{name: "medium rejects focus", assessment: RiskAssessment{Level: RiskMedium}, focus: "security", wantErr: true},
		{
			name: "dominant still validates focus", assessment: RiskAssessment{
				Level: RiskMedium, DominantLens: LensReadability,
			}, focus: "security", wantErr: true,
		},
		{
			name: "rejects invalid dominant binding", assessment: RiskAssessment{
				Level: RiskHigh, DominantLens: LensReadability,
			}, focus: "risk", wantErr: true,
		},
		{name: "rejects risk", assessment: RiskAssessment{Level: "critical"}, focus: "risk", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectReviewLenses(tt.assessment, tt.focus)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SelectReviewLenses() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SelectReviewLenses() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestClassifyVerificationApplicabilityFromImmutableSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string][]byte
		registryPaths []string
		wantDecision  VerificationApplicabilityValue
		wantActivity  VerificationContentActivity
		wantReasons   []VerificationApplicabilityReason
	}{
		{
			name:         "ordinary markdown is passive",
			files:        map[string][]byte{"docs/guide.md": []byte("# Guide\nHuman prose.\n")},
			wantDecision: VerificationNotApplicable, wantActivity: VerificationContentPassive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonPassiveDocument, Path: "docs/guide.md"}},
		},
		{
			name:         "markdown executable html is active",
			files:        map[string][]byte{"docs/guide.md": []byte("# Guide\n<script>run()</script>\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveSource, Path: "docs/guide.md"}},
		},
		{
			name:         "markdown html event handler is active",
			files:        map[string][]byte{"docs/guide.md": []byte("# Guide\n<img src=x onmouseover=run()>\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveSource, Path: "docs/guide.md"}},
		},
		{
			name:         "markdown fenced script remains inert prose",
			files:        map[string][]byte{"docs/guide.md": []byte("# Guide\n```html\n<script>example()</script>\n```\n")},
			wantDecision: VerificationNotApplicable, wantActivity: VerificationContentPassive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonPassiveDocument, Path: "docs/guide.md"}},
		},
		{
			name:         "asciidoc include is active",
			files:        map[string][]byte{"docs/guide.adoc": []byte("= Guide\ninclude::generated/config.adoc[]\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveSource, Path: "docs/guide.adoc"}},
		},
		{
			name:         "rst raw directive is active",
			files:        map[string][]byte{"docs/guide.rst": []byte("Guide\n=====\n.. raw:: html\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveSource, Path: "docs/guide.rst"}},
		},
		{
			name:         "ordinary raster image is passive even when binary",
			files:        map[string][]byte{"docs/poster.png": []byte("\x89PNG\r\n\x1a\n\x00payload")},
			wantDecision: VerificationNotApplicable, wantActivity: VerificationContentPassive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonPassiveImage, Path: "docs/poster.png"}},
		},
		{
			name:         "static svg is passive",
			files:        map[string][]byte{"docs/diagram.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`)},
			wantDecision: VerificationNotApplicable, wantActivity: VerificationContentPassive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonPassiveImage, Path: "docs/diagram.svg"}},
		},
		{
			name:         "active svg is applicable",
			files:        map[string][]byte{"docs/diagram.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveSource, Path: "docs/diagram.svg"}},
		},
		{
			name:         "agents contract is active",
			files:        map[string][]byte{"AGENTS.md": []byte("# Agent instructions\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveOperational, Path: "AGENTS.md"}},
		},
		{
			name:         "skill contract is active",
			files:        map[string][]byte{"skills/review/SKILL.md": []byte("# Review skill\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveOperational, Path: "skills/review/SKILL.md"}},
		},
		{
			name:         "policy markdown is active",
			files:        map[string][]byte{"docs/release-policy.md": []byte("# Policy\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveOperational, Path: "docs/release-policy.md"}},
		},
		{
			name:         "prompt document is active",
			files:        map[string][]byte{"prompts/system.txt": []byte("Follow the contract.\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveOperational, Path: "prompts/system.txt"}},
		},
		{
			name:         "workflow is active",
			files:        map[string][]byte{".github/workflows/ci.yml": []byte("jobs: {}\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveOperational, Path: ".github/workflows/ci.yml"}},
		},
		{
			name:         "configuration is active",
			files:        map[string][]byte{"config/app.toml": []byte("enabled = true\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveConfig, Path: "config/app.toml"}},
		},
		{
			name:         "static mdx is passive",
			files:        map[string][]byte{"book/chapter.mdx": []byte("# Chapter\n\n```tsx\n<Component />\n```\n")},
			wantDecision: VerificationNotApplicable, wantActivity: VerificationContentPassive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonPassiveDocument, Path: "book/chapter.mdx"}},
		},
		{
			name:         "active mdx is applicable",
			files:        map[string][]byte{"book/chapter.mdx": []byte("export const answer = 42\n\n# Chapter\n")},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveMDX, Path: "book/chapter.mdx"}},
		},
		{
			name:          "registry loaded ordinary doc is active",
			files:         map[string][]byte{"docs/reference.md": []byte("# Reference\n")},
			registryPaths: []string{"docs/reference.md"},
			wantDecision:  VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonActiveRegistryPath, Path: "docs/reference.md"}},
		},
		{
			name: "mixed content fails closed as applicable",
			files: map[string][]byte{
				"docs/guide.md":   []byte("# Guide\n"),
				"internal/app.go": []byte("package internal\n"),
			},
			wantDecision: VerificationApplicable, wantActivity: VerificationContentActive,
			wantReasons: []VerificationApplicabilityReason{
				{Code: VerificationReasonActiveSource, Path: "internal/app.go"},
				{Code: VerificationReasonMixedContent},
				{Code: VerificationReasonPassiveDocument, Path: "docs/guide.md"},
			},
		},
		{
			name:         "unknown content fails closed",
			files:        map[string][]byte{"assets/model.opaque": []byte("unknown\n")},
			wantDecision: VerificationUnknown, wantActivity: VerificationContentUnknown,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonUnknownContent, Path: "assets/model.opaque"}},
		},
		{
			name:         "unknown binary fails closed",
			files:        map[string][]byte{"assets/model.bin": []byte("\x00\x01\x02")},
			wantDecision: VerificationUnknown, wantActivity: VerificationContentUnknown,
			wantReasons: []VerificationApplicabilityReason{{Code: VerificationReasonUnknownBinary, Path: "assets/model.bin"}},
		},
		{
			name: "mixed with unknown remains unknown",
			files: map[string][]byte{
				"assets/model.opaque": []byte("unknown\n"),
				"docs/guide.md":       []byte("# Guide\n"),
				"internal/app.go":     []byte("package internal\n"),
			},
			wantDecision: VerificationUnknown, wantActivity: VerificationContentUnknown,
			wantReasons: []VerificationApplicabilityReason{
				{Code: VerificationReasonActiveSource, Path: "internal/app.go"},
				{Code: VerificationReasonMixedContent},
				{Code: VerificationReasonPassiveDocument, Path: "docs/guide.md"},
				{Code: VerificationReasonUnknownContent, Path: "assets/model.opaque"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			paths := make([]string, 0, len(tt.files))
			for logicalPath, content := range tt.files {
				fullPath := filepath.Join(repo, filepath.FromSlash(logicalPath))
				if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fullPath, content, 0o644); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, logicalPath)
			}
			snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
				Kind: TargetCurrentChanges, IntendedUntracked: paths,
			})
			if err != nil {
				t.Fatal(err)
			}
			registry := verificationTestRegistry(t, tt.registryPaths)
			builder := SnapshotBuilder{Repo: repo}
			got, err := builder.ClassifyVerificationApplicability(context.Background(), snapshot, registry, []string{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Decision != tt.wantDecision || got.ContentActivity != tt.wantActivity ||
				!reflect.DeepEqual(got.Reasons, tt.wantReasons) {
				t.Fatalf("ClassifyVerificationApplicability() = %q/%q/%#v, want %q/%q/%#v",
					got.Decision, got.ContentActivity, got.Reasons,
					tt.wantDecision, tt.wantActivity, tt.wantReasons)
			}
			again, err := builder.ClassifyVerificationApplicability(context.Background(), snapshot, registry, []string{})
			if err != nil || !reflect.DeepEqual(got, again) {
				t.Fatalf("classifier is not deterministic: first %#v, second %#v, err %v", got, again, err)
			}
		})
	}
}

func TestVerificationApplicabilityFailsClosedForModes(t *testing.T) {
	tests := []struct {
		name string
		stat DiffStat
		code VerificationApplicabilityReasonCode
	}{
		{name: "mode-only", stat: DiffStat{Path: "docs/guide.md", ModeOnly: true, OldMode: "100644", NewMode: "100755"}, code: VerificationReasonUnknownModeChange},
		{name: "executable", stat: DiffStat{Path: "docs/guide.md", OldMode: "100644", NewMode: "100755"}, code: VerificationReasonUnknownMode},
		{name: "symlink", stat: DiffStat{Path: "docs/guide.md", OldMode: "000000", NewMode: "120000"}, code: VerificationReasonUnknownSymlink},
		{name: "submodule", stat: DiffStat{Path: "vendor/module", OldMode: "000000", NewMode: "160000"}, code: VerificationReasonUnknownSubmodule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			activity, reason, err := (SnapshotBuilder{}).classifyVerificationStat(
				context.Background(), Snapshot{}, tt.stat, map[string]struct{}{}, "", nil,
			)
			if err != nil || activity != VerificationContentUnknown || reason.Code != tt.code {
				t.Fatalf("classifyVerificationStat() = %q/%#v/%v, want unknown/%q", activity, reason, err, tt.code)
			}
		})
	}
}

func TestVerificationPlanResultAndCorrectionClosureBindings(t *testing.T) {
	registry := verificationTestRegistry(t, []string{"AGENTS.md"})
	originalApplicability := verificationTestApplicability(t, registry, strings.Repeat("1", 40), verificationTestHash("original"))
	originalPlan, err := BuildVerificationPlan(originalApplicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	if got := verificationObligationIDs(originalPlan.Obligations); !reflect.DeepEqual(got, []string{"global", "unit"}) {
		t.Fatalf("BuildVerificationPlan() obligations = %v", got)
	}
	correctedApplicability := verificationTestApplicability(t, registry, strings.Repeat("2", 40), verificationTestHash("corrected"))
	correctedPlan, err := BuildVerificationPlan(correctedApplicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	result := VerificationResultRef{
		Schema: VerificationResultRefSchema, ResultRef: verificationTestHash("result"),
		Subject: correctedPlan.Subject, PolicyHash: correctedPlan.PolicyHash,
		PlanDigest: correctedPlan.Digest, ApplicabilityDigest: correctedPlan.ApplicabilityDigest,
		Aggregate: VerificationAggregateComplete, CompletedObligations: []string{"global", "unit"},
		EvidenceRefs: []string{verificationTestHash("evidence")},
	}
	if err := ValidateVerificationResultRef(correctedApplicability, registry, correctedPlan, result); err != nil {
		t.Fatal(err)
	}
	closure, err := BuildCorrectionImpactClosure(
		originalApplicability,
		registry,
		originalPlan,
		correctedApplicability,
		registry,
		correctedPlan,
		result,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(closure.MandatoryGlobalObligations, []string{"global"}) ||
		!reflect.DeepEqual(closure.RerunObligations, []string{"global", "unit"}) {
		t.Fatalf("correction closure = %#v", closure)
	}

	t.Run("result cannot cross subject", func(t *testing.T) {
		tampered := result
		tampered.Subject = originalPlan.Subject
		if err := ValidateVerificationResultRef(correctedApplicability, registry, correctedPlan, tampered); err == nil {
			t.Fatal("ValidateVerificationResultRef() accepted a result for the original subject")
		}
	})
	t.Run("complete must satisfy exact plan", func(t *testing.T) {
		tampered := result
		tampered.CompletedObligations = []string{"global"}
		if err := ValidateVerificationResultRef(correctedApplicability, registry, correctedPlan, tampered); err == nil {
			t.Fatal("ValidateVerificationResultRef() accepted an incomplete complete result")
		}
	})
	t.Run("closure cannot omit mandatory global", func(t *testing.T) {
		tampered := closure
		tampered.MandatoryGlobalObligations = []string{}
		tampered.RerunObligations = []string{"unit"}
		tampered.Digest, _ = correctionImpactClosureDigest(tampered)
		if err := ValidateCorrectionImpactClosure(
			originalApplicability,
			registry,
			originalPlan,
			correctedApplicability,
			registry,
			correctedPlan,
			tampered,
		); err == nil {
			t.Fatal("ValidateCorrectionImpactClosure() accepted omitted mandatory-global rerun")
		}
	})
	t.Run("digest detects tampering", func(t *testing.T) {
		tampered := closure
		tampered.AffectedObligations = []string{}
		if err := ValidateCorrectionImpactClosure(
			originalApplicability,
			registry,
			originalPlan,
			correctedApplicability,
			registry,
			correctedPlan,
			tampered,
		); err == nil {
			t.Fatal("ValidateCorrectionImpactClosure() accepted stale digest")
		}
	})
}

func TestBuildVerificationPlanIssuesNoWorkForNativeNonApplicableOrUnknown(t *testing.T) {
	registry := verificationTestRegistry(t, nil)
	for _, decision := range []VerificationApplicabilityValue{VerificationNotApplicable, VerificationUnknown} {
		t.Run(string(decision), func(t *testing.T) {
			applicability := verificationTestApplicability(t, registry, strings.Repeat("3", 40), verificationTestHash(string(decision)))
			applicability.Decision = decision
			if decision == VerificationNotApplicable {
				applicability.ContentActivity = VerificationContentPassive
				applicability.Reasons = []VerificationApplicabilityReason{{Code: VerificationReasonPassiveDocument, Path: "README.md"}}
			} else {
				applicability.ContentActivity = VerificationContentUnknown
				applicability.Reasons = []VerificationApplicabilityReason{{Code: VerificationReasonUnknownContent, Path: "asset.opaque"}}
			}
			applicability.Digest, _ = verificationApplicabilityDigest(applicability)
			plan, err := BuildVerificationPlan(applicability, registry)
			if err != nil {
				t.Fatalf("BuildVerificationPlan() error = %v", err)
			}
			if plan.Applicability != decision {
				t.Fatalf("plan applicability = %q, want %q", plan.Applicability, decision)
			}
			if len(plan.Obligations) != 0 {
				t.Fatal("BuildVerificationPlan() issued work for a non-executable applicability decision")
			}
			notRequired := VerificationResultRef{
				Schema: VerificationResultRefSchema, ResultRef: verificationTestHash("not-required-" + string(decision)),
				Subject: plan.Subject, PolicyHash: plan.PolicyHash, PlanDigest: plan.Digest,
				ApplicabilityDigest: plan.ApplicabilityDigest, Aggregate: VerificationAggregateNotRequired,
				CompletedObligations: []string{}, EvidenceRefs: []string{},
			}
			err = ValidateVerificationResultRef(applicability, registry, plan, notRequired)
			if decision == VerificationNotApplicable && err != nil {
				t.Fatalf("native not-applicable result rejected: %v", err)
			}
			if decision == VerificationUnknown && err == nil {
				t.Fatal("unknown applicability was relabeled not_required")
			}
		})
	}
}

func TestVerificationPlanAuthorityRejectsRegistryShrinkAndForgery(t *testing.T) {
	registry := verificationTestRegistry(t, []string{})
	applicability := verificationTestApplicability(
		t,
		registry,
		strings.Repeat("4", 40),
		verificationTestHash("authority"),
	)
	plan, err := BuildVerificationPlan(applicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	if got := verificationObligationIDs(plan.Obligations); !reflect.DeepEqual(got, []string{"global", "unit"}) {
		t.Fatalf("mandatory-global union = %v", got)
	}

	t.Run("omitted mandatory global", func(t *testing.T) {
		tampered := plan
		tampered.Obligations = append([]VerificationObligation{}, plan.Obligations[1:]...)
		tampered.Digest, _ = verificationPlanDigest(tampered)
		if err := ValidateVerificationPlan(applicability, registry, tampered); err == nil {
			t.Fatal("ValidateVerificationPlan() accepted a plan without the registry global")
		}
	})
	t.Run("forged registry obligation", func(t *testing.T) {
		tampered := plan
		tampered.Obligations = append([]VerificationObligation{}, plan.Obligations...)
		tampered.Obligations[1].RequirementRef = verificationTestHash("forged")
		tampered.Digest, _ = verificationPlanDigest(tampered)
		if err := ValidateVerificationPlan(applicability, registry, tampered); err == nil {
			t.Fatal("ValidateVerificationPlan() accepted a non-registry obligation")
		}
	})
}

func TestCorrectionClosureCannotShrinkOrTrustCallerImpact(t *testing.T) {
	registry := verificationTestRegistry(t, []string{})
	originalApplicability := verificationTestApplicability(
		t,
		registry,
		strings.Repeat("5", 40),
		verificationTestHash("shrink-original"),
	)
	originalPlan, err := BuildVerificationPlan(originalApplicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	correctedApplicability := verificationTestApplicability(
		t,
		registry,
		strings.Repeat("6", 40),
		verificationTestHash("shrink-corrected"),
	)
	shrunkPlan, err := BuildVerificationPlan(correctedApplicability, registry)
	if err != nil {
		t.Fatal(err)
	}
	shrunkPlan.Obligations = append([]VerificationObligation{}, shrunkPlan.Obligations[:1]...)
	shrunkPlan.Digest, _ = verificationPlanDigest(shrunkPlan)
	result := VerificationResultRef{
		Schema: VerificationResultRefSchema, ResultRef: verificationTestHash("shrunk-result"),
		Subject: shrunkPlan.Subject, PolicyHash: shrunkPlan.PolicyHash, PlanDigest: shrunkPlan.Digest,
		ApplicabilityDigest: shrunkPlan.ApplicabilityDigest, Aggregate: VerificationAggregateComplete,
		CompletedObligations: []string{"global"}, EvidenceRefs: []string{},
	}
	if _, err := BuildCorrectionImpactClosure(
		originalApplicability,
		registry,
		originalPlan,
		correctedApplicability,
		registry,
		shrunkPlan,
		result,
	); err == nil {
		t.Fatal("BuildCorrectionImpactClosure() accepted a corrected plan that dropped an original obligation")
	}
}

func TestVerificationContractsRejectNonCanonicalHashPreimages(t *testing.T) {
	registry := verificationTestRegistry(t, []string{})
	t.Run("explicit workspace projection", func(t *testing.T) {
		applicability := verificationTestApplicability(
			t,
			registry,
			strings.Repeat("7", 40),
			verificationTestHash("projection"),
		)
		applicability.Subject.Projection = ProjectionWorkspace
		applicability.Digest, _ = verificationApplicabilityDigest(applicability)
		if err := applicability.Validate(); err == nil {
			t.Fatal("VerificationApplicability.Validate() accepted non-canonical workspace projection")
		}
	})
	t.Run("null operational paths", func(t *testing.T) {
		tampered := registry
		tampered.OperationalPaths = nil
		tampered.Digest, _ = verificationRegistryDigest(tampered)
		if err := tampered.Validate(); err == nil {
			t.Fatal("VerificationPlanRegistry.Validate() accepted null operational_paths")
		}
	})
}

func TestSnapshotsEqualExactIncludesEverySnapshotField(t *testing.T) {
	snapshot := Snapshot{
		Kind: TargetCurrentChanges, Projection: ProjectionStaged, UnbornHead: true,
		BaseTree: strings.Repeat("1", 40), CandidateTree: strings.Repeat("2", 40),
		PathsDigest: verificationTestHash("paths"), IntendedUntracked: []string{},
		IntendedUntrackedProof: verificationTestHash("untracked"), LedgerIDs: []string{"R1"},
		Paths: []string{"README.md"}, Identity: verificationTestHash("snapshot"),
	}
	if !SnapshotsEqualExact(snapshot, snapshot) {
		t.Fatal("SnapshotsEqualExact() rejected identical snapshots")
	}
	changed := snapshot
	changed.UnbornHead = false
	if SnapshotsEqualExact(snapshot, changed) {
		t.Fatal("SnapshotsEqualExact() ignored UnbornHead")
	}
	fields := []func(*Snapshot){
		func(value *Snapshot) { value.Kind = TargetBaseDiff },
		func(value *Snapshot) { value.Projection = "" },
		func(value *Snapshot) { value.BaseTree = strings.Repeat("3", 40) },
		func(value *Snapshot) { value.CandidateTree = strings.Repeat("4", 40) },
		func(value *Snapshot) { value.PathsDigest = verificationTestHash("other-paths") },
		func(value *Snapshot) { value.IntendedUntracked = []string{"new.txt"} },
		func(value *Snapshot) { value.IntendedUntrackedProof = verificationTestHash("other-proof") },
		func(value *Snapshot) { value.LedgerIDs = []string{"R2"} },
		func(value *Snapshot) { value.Paths = []string{"docs/guide.md"} },
		func(value *Snapshot) { value.Identity = verificationTestHash("other-snapshot") },
	}
	for index, mutate := range fields {
		changed = snapshot
		mutate(&changed)
		if SnapshotsEqualExact(snapshot, changed) {
			t.Fatalf("SnapshotsEqualExact() ignored field mutation %d", index)
		}
	}
}

func TestVerificationClassifierExecutableModeIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git executable-bit transitions are POSIX-only")
	}
	repo := initSnapshotRepo(t)
	gitSnapshot(t, repo, "config", "core.filemode", "true")
	if err := os.Chmod(filepath.Join(repo, "tracked.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := (SnapshotBuilder{Repo: repo}).Build(context.Background(), Target{
		Kind: TargetCurrentChanges, IntendedUntracked: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := (SnapshotBuilder{Repo: repo}).ClassifyVerificationApplicability(
		context.Background(), snapshot, verificationTestRegistry(t, nil), []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision != VerificationUnknown || got.Reasons[0].Code != VerificationReasonUnknownModeChange {
		t.Fatalf("executable mode applicability = %#v", got)
	}
}

// TestResolveVerificationGateCoversEveryDimensionCrossProduct proves the four
// dimensions are resolved independently. Cost never implies effect: the table
// below deliberately contains a quick destructive row and a very-long
// read-only row that resolve to different gates.
func TestResolveVerificationGateCoversEveryDimensionCrossProduct(t *testing.T) {
	t.Parallel()
	autoRun := VerificationGate{Decision: VerificationDecisionAutoRun}
	consentOnly := VerificationGate{
		Decision:                  VerificationDecisionFrozenPlanConsent,
		RequiresFrozenPlanConsent: true,
	}
	authorizationOnly := VerificationGate{
		Decision:                       VerificationDecisionImmediateAuthorization,
		RequiresImmediateAuthorization: true,
	}
	consentAndAuthorization := VerificationGate{
		Decision:                       VerificationDecisionImmediateAuthorization,
		RequiresFrozenPlanConsent:      true,
		RequiresImmediateAuthorization: true,
	}
	gap := VerificationGate{Decision: VerificationDecisionRecordEvidenceGap}
	notApplicable := VerificationGate{Decision: VerificationDecisionRecordNotApplicable}

	type effectKey struct {
		cost       VerificationCost
		mutation   VerificationMutationEffect
		permission VerificationPermissionEffect
	}
	applicableGates := map[effectKey]VerificationGate{
		{VerificationCostQuick, VerificationMutationReadOnly, VerificationPermissionOrdinary}:        autoRun,
		{VerificationCostQuick, VerificationMutationReadOnly, VerificationPermissionSensitive}:       authorizationOnly,
		{VerificationCostQuick, VerificationMutationDestructive, VerificationPermissionOrdinary}:     authorizationOnly,
		{VerificationCostQuick, VerificationMutationDestructive, VerificationPermissionSensitive}:    authorizationOnly,
		{VerificationCostLong, VerificationMutationReadOnly, VerificationPermissionOrdinary}:         consentOnly,
		{VerificationCostLong, VerificationMutationReadOnly, VerificationPermissionSensitive}:        consentAndAuthorization,
		{VerificationCostLong, VerificationMutationDestructive, VerificationPermissionOrdinary}:      consentAndAuthorization,
		{VerificationCostLong, VerificationMutationDestructive, VerificationPermissionSensitive}:     consentAndAuthorization,
		{VerificationCostVeryLong, VerificationMutationReadOnly, VerificationPermissionOrdinary}:     consentOnly,
		{VerificationCostVeryLong, VerificationMutationReadOnly, VerificationPermissionSensitive}:    consentAndAuthorization,
		{VerificationCostVeryLong, VerificationMutationDestructive, VerificationPermissionOrdinary}:  consentAndAuthorization,
		{VerificationCostVeryLong, VerificationMutationDestructive, VerificationPermissionSensitive}: consentAndAuthorization,
		{VerificationCostUnknown, VerificationMutationReadOnly, VerificationPermissionOrdinary}:      gap,
		{VerificationCostUnknown, VerificationMutationReadOnly, VerificationPermissionSensitive}:     gap,
		{VerificationCostUnknown, VerificationMutationDestructive, VerificationPermissionOrdinary}:   gap,
		{VerificationCostUnknown, VerificationMutationDestructive, VerificationPermissionSensitive}:  gap,
	}

	applicabilities := []VerificationApplicabilityValue{
		VerificationApplicable, VerificationNotApplicable, VerificationUnknown,
	}
	costs := []VerificationCost{
		VerificationCostQuick, VerificationCostLong, VerificationCostVeryLong, VerificationCostUnknown,
	}
	mutations := []VerificationMutationEffect{
		VerificationMutationReadOnly, VerificationMutationDestructive,
	}
	permissions := []VerificationPermissionEffect{
		VerificationPermissionOrdinary, VerificationPermissionSensitive,
	}

	resolved := 0
	for _, applicability := range applicabilities {
		for _, cost := range costs {
			for _, mutation := range mutations {
				for _, permission := range permissions {
					profile := VerificationEffectProfile{
						Applicability: applicability, Cost: cost,
						MutationEffect: mutation, PermissionEffect: permission,
					}
					want := notApplicable
					switch applicability {
					case VerificationUnknown:
						want = gap
					case VerificationApplicable:
						known, ok := applicableGates[effectKey{cost, mutation, permission}]
						if !ok {
							t.Fatalf("cross-product combination %#v is unhandled by the expectation table", profile)
						}
						want = known
					}
					got, err := ResolveVerificationGate(profile)
					if err != nil {
						t.Fatalf("ResolveVerificationGate(%#v) error = %v", profile, err)
					}
					if got != want {
						t.Fatalf("ResolveVerificationGate(%#v) = %#v, want %#v", profile, got, want)
					}
					resolved++
				}
			}
		}
	}
	if resolved != len(applicabilities)*len(costs)*len(mutations)*len(permissions) {
		t.Fatalf("resolved %d cross-product combinations, want %d",
			resolved, len(applicabilities)*len(costs)*len(mutations)*len(permissions))
	}
}

// TestQuickDestructiveVerificationStillRequiresImmediateAuthorization is the
// headline invariant the old two-dimension model could not express.
func TestQuickDestructiveVerificationStillRequiresImmediateAuthorization(t *testing.T) {
	t.Parallel()
	gate, err := ResolveVerificationGate(VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostQuick,
		MutationEffect: VerificationMutationDestructive, PermissionEffect: VerificationPermissionOrdinary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gate.RequiresImmediateAuthorization {
		t.Fatalf("quick destructive gate = %#v, want immediate authorization", gate)
	}
	if gate.Decision != VerificationDecisionImmediateAuthorization || gate.AllowsAutomaticRun() {
		t.Fatalf("quick destructive gate = %#v, want a blocked non-automatic decision", gate)
	}
}

func TestVeryLongReadOnlyVerificationNeedsConsentButNotAuthorization(t *testing.T) {
	t.Parallel()
	gate, err := ResolveVerificationGate(VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostVeryLong,
		MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != VerificationDecisionFrozenPlanConsent ||
		!gate.RequiresFrozenPlanConsent || gate.RequiresImmediateAuthorization {
		t.Fatalf("very-long read-only gate = %#v, want consent without immediate authorization", gate)
	}
}

func TestQuickReadOnlyOrdinaryVerificationRunsAutomatically(t *testing.T) {
	t.Parallel()
	gate, err := ResolveVerificationGate(VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostQuick,
		MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !gate.AllowsAutomaticRun() || gate.RequiresFrozenPlanConsent || gate.RequiresImmediateAuthorization {
		t.Fatalf("quick read-only ordinary gate = %#v, want an automatic run", gate)
	}
}

// TestUnknownVerificationDimensionRecordsGapWithoutLoop proves an unknown
// dimension is an explicit recorded gap, never an invented result and never a
// retry: repeated resolution is idempotent and asks for nothing.
func TestUnknownVerificationDimensionRecordsGapWithoutLoop(t *testing.T) {
	t.Parallel()
	profiles := []VerificationEffectProfile{
		{
			Applicability: VerificationUnknown, Cost: VerificationCostQuick,
			MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
		},
		{
			Applicability: VerificationApplicable, Cost: VerificationCostUnknown,
			MutationEffect: VerificationMutationDestructive, PermissionEffect: VerificationPermissionSensitive,
		},
	}
	for _, profile := range profiles {
		first, err := ResolveVerificationGate(profile)
		if err != nil {
			t.Fatalf("ResolveVerificationGate(%#v) error = %v", profile, err)
		}
		if first.Decision != VerificationDecisionRecordEvidenceGap {
			t.Fatalf("ResolveVerificationGate(%#v) = %#v, want a recorded evidence gap", profile, first)
		}
		if first.RequiresFrozenPlanConsent || first.RequiresImmediateAuthorization || first.AllowsAutomaticRun() {
			t.Fatalf("evidence gap %#v started work or a prompt", first)
		}
		second, err := ResolveVerificationGate(profile)
		if err != nil || second != first {
			t.Fatalf("repeated resolution diverged: %#v, %#v, %v", first, second, err)
		}
	}
}

func TestNotApplicableVerificationPlansAndRunsNothing(t *testing.T) {
	t.Parallel()
	gate, err := ResolveVerificationGate(VerificationEffectProfile{
		Applicability: VerificationNotApplicable, Cost: VerificationCostVeryLong,
		MutationEffect: VerificationMutationDestructive, PermissionEffect: VerificationPermissionSensitive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gate.Decision != VerificationDecisionRecordNotApplicable ||
		gate.RequiresFrozenPlanConsent || gate.RequiresImmediateAuthorization || gate.AllowsAutomaticRun() {
		t.Fatalf("not-applicable gate = %#v, want a recorded outcome with no work", gate)
	}
}

// TestVerificationEffectProfileFailsClosedForMissingOrUnknownDimensions proves
// a caller that ignores the returned error still cannot run anything.
func TestVerificationEffectProfileFailsClosedForMissingOrUnknownDimensions(t *testing.T) {
	t.Parallel()
	complete := VerificationEffectProfile{
		Applicability: VerificationApplicable, Cost: VerificationCostQuick,
		MutationEffect: VerificationMutationReadOnly, PermissionEffect: VerificationPermissionOrdinary,
	}
	tests := []struct {
		name    string
		mutate  func(VerificationEffectProfile) VerificationEffectProfile
		missing bool
	}{
		{"missing applicability", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.Applicability = ""
			return p
		}, true},
		{"missing cost", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.Cost = ""
			return p
		}, true},
		{"missing mutation effect", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.MutationEffect = ""
			return p
		}, true},
		{"missing permission effect", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.PermissionEffect = ""
			return p
		}, true},
		{"unsupported applicability", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.Applicability = "maybe"
			return p
		}, false},
		{"unsupported cost", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.Cost = "instant"
			return p
		}, false},
		{"unsupported mutation effect", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.MutationEffect = "harmless"
			return p
		}, false},
		{"unsupported permission effect", func(p VerificationEffectProfile) VerificationEffectProfile {
			p.PermissionEffect = "trusted"
			return p
		}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			profile := test.mutate(complete)
			if err := profile.Validate(); err == nil {
				t.Fatalf("Validate() accepted %#v", profile)
			} else if test.missing && !errors.Is(err, ErrVerificationDimensionMissing) {
				t.Fatalf("Validate() error = %v, want ErrVerificationDimensionMissing", err)
			} else if !test.missing && !errors.Is(err, ErrVerificationDimensionUnsupported) {
				t.Fatalf("Validate() error = %v, want ErrVerificationDimensionUnsupported", err)
			}
			gate, err := ResolveVerificationGate(profile)
			if err == nil {
				t.Fatalf("ResolveVerificationGate() accepted %#v", profile)
			}
			if gate.AllowsAutomaticRun() || gate.Decision != VerificationDecisionRejected {
				t.Fatalf("rejected gate = %#v, want a fail-closed rejection", gate)
			}
			if !gate.RequiresFrozenPlanConsent || !gate.RequiresImmediateAuthorization {
				t.Fatalf("rejected gate = %#v, want every gate closed for error-ignoring callers", gate)
			}
		})
	}
}

func TestAggregateVerificationCostFoldsObligationsWithUnknownDominating(t *testing.T) {
	t.Parallel()
	obligation := func(id string, cost VerificationCost) VerificationObligation {
		return VerificationObligation{
			ID: id, RequirementRef: verificationTestHash(id + "-requirement"),
			ArgvRef: verificationTestHash(id + "-argv"), CapabilityRef: "host.exec", Cost: cost,
		}
	}
	tests := []struct {
		name        string
		obligations []VerificationObligation
		want        VerificationCost
	}{
		{"empty", nil, VerificationCostUnknown},
		{"quick only", []VerificationObligation{obligation("a", VerificationCostQuick)}, VerificationCostQuick},
		{"quick and long", []VerificationObligation{
			obligation("a", VerificationCostQuick), obligation("b", VerificationCostLong),
		}, VerificationCostLong},
		{"long and very long", []VerificationObligation{
			obligation("a", VerificationCostLong), obligation("b", VerificationCostVeryLong),
		}, VerificationCostVeryLong},
		{"unknown dominates", []VerificationObligation{
			obligation("a", VerificationCostVeryLong), obligation("b", VerificationCostUnknown),
		}, VerificationCostUnknown},
		{"unsupported is unknown", []VerificationObligation{
			obligation("a", VerificationCost("instant")),
		}, VerificationCostUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := AggregateVerificationCost(test.obligations); got != test.want {
				t.Fatalf("AggregateVerificationCost() = %q, want %q", got, test.want)
			}
		})
	}
}

func verificationTestRegistry(t *testing.T, operationalPaths []string) VerificationPlanRegistry {
	t.Helper()
	registry, err := NewVerificationPlanRegistry(
		verificationTestHash("policy"),
		operationalPaths,
		[]VerificationObligation{
			{
				ID: "unit", RequirementRef: verificationTestHash("unit-requirement"),
				ArgvRef: verificationTestHash("unit-argv"), CapabilityRef: "host.exec",
				Cost: VerificationCostQuick,
			},
			{
				ID: "global", RequirementRef: verificationTestHash("global-requirement"),
				ArgvRef: verificationTestHash("global-argv"), CapabilityRef: "host.exec",
				Cost: VerificationCostLong, MandatoryGlobal: true,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func verificationTestApplicability(t *testing.T, registry VerificationPlanRegistry, candidateTree, identity string) VerificationApplicability {
	t.Helper()
	applicability := VerificationApplicability{
		Schema: VerificationApplicabilitySchema, ClassifierVersion: VerificationClassifierVersion,
		Subject: VerificationSubject{
			Kind: TargetCurrentChanges, BaseTree: strings.Repeat("a", 40),
			CandidateTree: candidateTree, PathsDigest: verificationTestHash("paths"),
			SnapshotIdentity: identity,
		},
		PolicyHash: registry.PolicyHash, RegistryDigest: registry.Digest,
		Decision: VerificationApplicable, ContentActivity: VerificationContentActive,
		Reasons:      []VerificationApplicabilityReason{{Code: VerificationReasonActiveSource, Path: "internal/app.go"}},
		EvidenceRefs: []string{},
	}
	var err error
	applicability.Digest, err = verificationApplicabilityDigest(applicability)
	if err != nil {
		t.Fatal(err)
	}
	if err := applicability.Validate(); err != nil {
		t.Fatal(err)
	}
	return applicability
}

func verificationTestHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
