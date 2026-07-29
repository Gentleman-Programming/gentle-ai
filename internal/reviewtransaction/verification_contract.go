package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	VerificationApplicabilitySchema = "gentle-ai.verification-applicability/v1"
	VerificationPlanRegistrySchema  = "gentle-ai.verification-plan-registry/v1"
	VerificationPlanSchema          = "gentle-ai.verification-plan/v1"
	VerificationResultRefSchema     = "gentle-ai.verification-result-ref/v1"
	CorrectionImpactClosureSchema   = "gentle-ai.correction-impact-closure/v1"
	VerificationClassifierVersion   = "gentle-ai.verification-classifier/v1"
)

type VerificationApplicabilityValue string

const (
	VerificationApplicable    VerificationApplicabilityValue = "applicable"
	VerificationNotApplicable VerificationApplicabilityValue = "not_applicable"
	VerificationUnknown       VerificationApplicabilityValue = "unknown"
)

type VerificationContentActivity string

const (
	VerificationContentPassive VerificationContentActivity = "passive"
	VerificationContentActive  VerificationContentActivity = "active"
	VerificationContentUnknown VerificationContentActivity = "unknown"
)

type VerificationApplicabilityReasonCode string

const (
	VerificationReasonPassiveDocument    VerificationApplicabilityReasonCode = "passive_document"
	VerificationReasonPassiveImage       VerificationApplicabilityReasonCode = "passive_image"
	VerificationReasonActiveSource       VerificationApplicabilityReasonCode = "active_source"
	VerificationReasonActiveConfig       VerificationApplicabilityReasonCode = "active_configuration"
	VerificationReasonActiveMDX          VerificationApplicabilityReasonCode = "active_mdx"
	VerificationReasonActiveOperational  VerificationApplicabilityReasonCode = "active_operational_document"
	VerificationReasonActiveRegistryPath VerificationApplicabilityReasonCode = "active_registry_path"
	VerificationReasonMixedContent       VerificationApplicabilityReasonCode = "mixed_content"
	VerificationReasonUnknownBinary      VerificationApplicabilityReasonCode = "unknown_binary"
	VerificationReasonUnknownContent     VerificationApplicabilityReasonCode = "unknown_content"
	VerificationReasonUnknownMode        VerificationApplicabilityReasonCode = "unknown_mode"
	VerificationReasonUnknownModeChange  VerificationApplicabilityReasonCode = "unknown_mode_change"
	VerificationReasonUnknownSubmodule   VerificationApplicabilityReasonCode = "unknown_submodule"
	VerificationReasonUnknownSymlink     VerificationApplicabilityReasonCode = "unknown_symlink"
)

type VerificationApplicabilityReason struct {
	Code VerificationApplicabilityReasonCode `json:"code"`
	Path string                              `json:"path,omitempty"`
}

// VerificationSubject is the minimum exact immutable subject required by
// proportional-verification contracts. UnbornHead is explicit because the
// historical snapshot identity intentionally did not bind it.
type VerificationSubject struct {
	Kind             TargetKind `json:"kind"`
	Projection       Projection `json:"projection,omitempty"`
	UnbornHead       bool       `json:"unborn_head,omitempty"`
	BaseTree         string     `json:"base_tree"`
	CandidateTree    string     `json:"candidate_tree"`
	PathsDigest      string     `json:"paths_digest"`
	SnapshotIdentity string     `json:"snapshot_identity"`
}

func VerificationSubjectFromSnapshot(snapshot Snapshot) (VerificationSubject, error) {
	subject := VerificationSubject{
		Kind: snapshot.Kind, Projection: snapshot.Projection, UnbornHead: snapshot.UnbornHead,
		BaseTree: snapshot.BaseTree, CandidateTree: snapshot.CandidateTree,
		PathsDigest: snapshot.PathsDigest, SnapshotIdentity: snapshot.Identity,
	}
	if err := subject.Validate(); err != nil {
		return VerificationSubject{}, err
	}
	return subject, nil
}

func (subject VerificationSubject) Validate() error {
	switch subject.Kind {
	case TargetCurrentChanges, TargetBaseDiff, TargetBaseWorkspaceOverlay, TargetExactRevision, TargetFixDiff:
	default:
		return fmt.Errorf("verification subject has unsupported target kind %q", subject.Kind)
	}
	projection, err := canonicalProjection(subject.Projection)
	if err != nil {
		return fmt.Errorf("verification subject: %w", err)
	}
	if projection != subject.Projection {
		return errors.New("verification subject projection must use its canonical representation")
	}
	if !validGitTree(subject.BaseTree) || !validGitTree(subject.CandidateTree) {
		return errors.New("verification subject requires valid base and candidate Git trees")
	}
	if !validSHA256(subject.PathsDigest) || !validSHA256(subject.SnapshotIdentity) {
		return errors.New("verification subject requires lowercase SHA-256 path and snapshot identities")
	}
	return nil
}

type VerificationApplicability struct {
	Schema            string                            `json:"schema"`
	ClassifierVersion string                            `json:"classifier_version"`
	Subject           VerificationSubject               `json:"subject"`
	PolicyHash        string                            `json:"policy_hash"`
	RegistryDigest    string                            `json:"registry_digest"`
	Decision          VerificationApplicabilityValue    `json:"decision"`
	ContentActivity   VerificationContentActivity       `json:"content_activity"`
	Reasons           []VerificationApplicabilityReason `json:"reasons"`
	EvidenceRefs      []string                          `json:"evidence_refs"`
	Digest            string                            `json:"digest"`
}

func (applicability VerificationApplicability) Validate() error {
	if applicability.Schema != VerificationApplicabilitySchema ||
		applicability.ClassifierVersion != VerificationClassifierVersion {
		return errors.New("unsupported verification applicability schema or classifier")
	}
	if err := applicability.Subject.Validate(); err != nil {
		return err
	}
	if !validSHA256(applicability.PolicyHash) || !validSHA256(applicability.RegistryDigest) {
		return errors.New("verification applicability requires policy and registry SHA-256 bindings")
	}
	if len(applicability.Reasons) == 0 || !verificationReasonsCanonical(applicability.Reasons) {
		return errors.New("verification applicability reasons must be non-empty and canonical")
	}
	hasPassive, hasActive, hasUnknown, hasMixed := false, false, false, false
	for _, reason := range applicability.Reasons {
		category, valid := verificationReasonCategory(reason)
		if !valid {
			return fmt.Errorf("invalid verification applicability reason %#v", reason)
		}
		switch category {
		case VerificationContentPassive:
			hasPassive = true
		case VerificationContentActive:
			hasActive = true
		case VerificationContentUnknown:
			hasUnknown = true
		default:
			hasMixed = true
		}
	}
	if !canonicalSHA256Refs(applicability.EvidenceRefs) {
		return errors.New("verification applicability evidence refs must be canonical lowercase SHA-256 identities")
	}
	switch applicability.Decision {
	case VerificationNotApplicable:
		if applicability.ContentActivity != VerificationContentPassive || !hasPassive || hasActive || hasUnknown || hasMixed {
			return errors.New("not-applicable verification must bind passive content")
		}
	case VerificationApplicable:
		if applicability.ContentActivity != VerificationContentActive || !hasActive || hasUnknown ||
			hasMixed != hasPassive {
			return errors.New("applicable verification must bind active content")
		}
	case VerificationUnknown:
		if applicability.ContentActivity != VerificationContentUnknown || !hasUnknown ||
			hasMixed != (hasPassive && hasActive) {
			return errors.New("unknown verification applicability must bind unknown content")
		}
	default:
		return fmt.Errorf("unsupported verification applicability %q", applicability.Decision)
	}
	want, err := verificationApplicabilityDigest(applicability)
	if err != nil {
		return err
	}
	if applicability.Digest != want {
		return errors.New("verification applicability digest does not match its canonical content")
	}
	return nil
}

type VerificationCost string

const (
	VerificationCostQuick    VerificationCost = "quick"
	VerificationCostLong     VerificationCost = "long"
	VerificationCostVeryLong VerificationCost = "very_long"
	VerificationCostUnknown  VerificationCost = "unknown"
)

// VerificationMutationEffect answers what a check DOES to the workspace. It is
// deliberately independent of VerificationCost: a quick check can still destroy
// state and a very-long check can still be read-only, so folding effect into
// cost would let a fast destructive check pass as harmless.
type VerificationMutationEffect string

const (
	VerificationMutationReadOnly    VerificationMutationEffect = "read_only"
	VerificationMutationDestructive VerificationMutationEffect = "destructive"
)

// VerificationPermissionEffect answers what a check REACHES beyond the ordinary
// sandbox. It varies independently of both cost and mutation effect: a quick
// read-only check can still need credentials or network authority.
type VerificationPermissionEffect string

const (
	VerificationPermissionOrdinary  VerificationPermissionEffect = "ordinary"
	VerificationPermissionSensitive VerificationPermissionEffect = "permission_sensitive"
)

// VerificationDecision is the single proportional outcome resolved from the
// four independent dimensions. Rejected is the fail-closed sentinel; it is
// never a runnable state.
type VerificationDecision string

const (
	VerificationDecisionRejected               VerificationDecision = "rejected"
	VerificationDecisionRecordNotApplicable    VerificationDecision = "record_not_applicable"
	VerificationDecisionRecordEvidenceGap      VerificationDecision = "record_evidence_gap"
	VerificationDecisionAutoRun                VerificationDecision = "auto_run"
	VerificationDecisionFrozenPlanConsent      VerificationDecision = "frozen_plan_consent"
	VerificationDecisionImmediateAuthorization VerificationDecision = "immediate_authorization"
)

var (
	// ErrVerificationDimensionMissing marks an omitted dimension. Omission is
	// never read as a harmless default, because the whole point of modelling
	// effect separately is that "unstated" and "read-only" are not the same.
	ErrVerificationDimensionMissing = errors.New("verification effect dimension is missing")
	// ErrVerificationDimensionUnsupported marks a value outside its closed set.
	ErrVerificationDimensionUnsupported = errors.New("verification effect dimension is unsupported")
	// ErrVerificationAuthorizationStale marks consent or authorization that does
	// not bind the exact frozen plan it is presented against.
	ErrVerificationAuthorizationStale = errors.New("verification authorization does not bind the exact frozen plan")
)

// VerificationEffectProfile carries the four independent verification
// dimensions. Every dimension is a closed set and none of them is derivable
// from another, so all four must be stated explicitly.
type VerificationEffectProfile struct {
	Applicability    VerificationApplicabilityValue `json:"applicability"`
	Cost             VerificationCost               `json:"cost"`
	MutationEffect   VerificationMutationEffect     `json:"mutation_effect"`
	PermissionEffect VerificationPermissionEffect   `json:"permission_effect"`
}

func (profile VerificationEffectProfile) Validate() error {
	switch profile.Applicability {
	case VerificationApplicable, VerificationNotApplicable, VerificationUnknown:
	case "":
		return fmt.Errorf("%w: applicability", ErrVerificationDimensionMissing)
	default:
		return fmt.Errorf("%w: applicability %q", ErrVerificationDimensionUnsupported, profile.Applicability)
	}
	switch profile.Cost {
	case VerificationCostQuick, VerificationCostLong, VerificationCostVeryLong, VerificationCostUnknown:
	case "":
		return fmt.Errorf("%w: cost", ErrVerificationDimensionMissing)
	default:
		return fmt.Errorf("%w: cost %q", ErrVerificationDimensionUnsupported, profile.Cost)
	}
	switch profile.MutationEffect {
	case VerificationMutationReadOnly, VerificationMutationDestructive:
	case "":
		return fmt.Errorf("%w: mutation effect", ErrVerificationDimensionMissing)
	default:
		return fmt.Errorf("%w: mutation effect %q", ErrVerificationDimensionUnsupported, profile.MutationEffect)
	}
	switch profile.PermissionEffect {
	case VerificationPermissionOrdinary, VerificationPermissionSensitive:
	case "":
		return fmt.Errorf("%w: permission effect", ErrVerificationDimensionMissing)
	default:
		return fmt.Errorf("%w: permission effect %q", ErrVerificationDimensionUnsupported, profile.PermissionEffect)
	}
	return nil
}

// VerificationGate is the one resolved decision plus the two gates it implies.
// Cost consent and effect authorization are separate obligations because they
// answer different questions: consent pays for time, authorization admits an
// effect. A long destructive check owes both.
type VerificationGate struct {
	Decision                       VerificationDecision `json:"decision"`
	RequiresFrozenPlanConsent      bool                 `json:"requires_frozen_plan_consent"`
	RequiresImmediateAuthorization bool                 `json:"requires_immediate_authorization"`
}

// AllowsAutomaticRun is true only for the single unattended combination:
// applicable, quick, read-only, and ordinary.
func (gate VerificationGate) AllowsAutomaticRun() bool {
	return gate.Decision == VerificationDecisionAutoRun &&
		!gate.RequiresFrozenPlanConsent && !gate.RequiresImmediateAuthorization
}

// rejectedVerificationGate closes every gate as well as the decision so a
// caller that ignores the returned error still cannot start any work, whichever
// field it happens to inspect.
func rejectedVerificationGate() VerificationGate {
	return VerificationGate{
		Decision:                       VerificationDecisionRejected,
		RequiresFrozenPlanConsent:      true,
		RequiresImmediateAuthorization: true,
	}
}

// ResolveVerificationGate is total over the closed cross product of the four
// dimensions. Precedence is deliberate: a plan that runs nothing cannot be
// promoted into work by an effect, and an unknown dimension is recorded as an
// evidence gap rather than guessed, so it also runs nothing and needs no
// authorization. Only once work is genuinely planned do effect and cost apply,
// and effect then overrides cost: a quick destructive check is still gated.
func ResolveVerificationGate(profile VerificationEffectProfile) (VerificationGate, error) {
	if err := profile.Validate(); err != nil {
		return rejectedVerificationGate(), fmt.Errorf("resolve verification gate: %w", err)
	}
	switch profile.Applicability {
	case VerificationNotApplicable:
		return VerificationGate{Decision: VerificationDecisionRecordNotApplicable}, nil
	case VerificationUnknown:
		return VerificationGate{Decision: VerificationDecisionRecordEvidenceGap}, nil
	}
	if profile.Cost == VerificationCostUnknown {
		return VerificationGate{Decision: VerificationDecisionRecordEvidenceGap}, nil
	}
	expensive := profile.Cost == VerificationCostLong || profile.Cost == VerificationCostVeryLong
	sensitive := profile.MutationEffect == VerificationMutationDestructive ||
		profile.PermissionEffect == VerificationPermissionSensitive
	switch {
	case sensitive:
		return VerificationGate{
			Decision:                       VerificationDecisionImmediateAuthorization,
			RequiresFrozenPlanConsent:      expensive,
			RequiresImmediateAuthorization: true,
		}, nil
	case expensive:
		return VerificationGate{
			Decision:                  VerificationDecisionFrozenPlanConsent,
			RequiresFrozenPlanConsent: true,
		}, nil
	default:
		return VerificationGate{Decision: VerificationDecisionAutoRun}, nil
	}
}

// AggregateVerificationCost folds obligation costs into the single plan-level
// cost dimension. Unknown dominates every other value, and an empty or
// unsupported input is unknown too, because an unmeasurable obligation makes
// the plan an evidence gap rather than a cheap check.
func AggregateVerificationCost(obligations []VerificationObligation) VerificationCost {
	if len(obligations) == 0 {
		return VerificationCostUnknown
	}
	rank := map[VerificationCost]int{
		VerificationCostQuick: 1, VerificationCostLong: 2, VerificationCostVeryLong: 3,
	}
	highest := VerificationCostQuick
	for _, obligation := range obligations {
		level, known := rank[obligation.Cost]
		if !known {
			return VerificationCostUnknown
		}
		if level > rank[highest] {
			highest = obligation.Cost
		}
	}
	return highest
}

// VerificationObligation contains refs, never shell text. ArgvRef identifies
// an immutable literal-argv value; RequirementRef identifies the exact check
// obligation. CapabilityRef names the provider capability needed to execute it.
type VerificationObligation struct {
	ID              string           `json:"id"`
	RequirementRef  string           `json:"requirement_ref"`
	ArgvRef         string           `json:"argv_ref"`
	CapabilityRef   string           `json:"capability_ref"`
	Cost            VerificationCost `json:"cost"`
	MandatoryGlobal bool             `json:"mandatory_global,omitempty"`
}

func (obligation VerificationObligation) validate() error {
	if !validVerificationID(obligation.ID) || !validVerificationID(obligation.CapabilityRef) {
		return errors.New("verification obligation requires canonical ID and capability refs")
	}
	if !validSHA256(obligation.RequirementRef) || !validSHA256(obligation.ArgvRef) {
		return errors.New("verification obligation requires exact requirement and argv SHA-256 refs")
	}
	switch obligation.Cost {
	case VerificationCostQuick, VerificationCostLong, VerificationCostVeryLong, VerificationCostUnknown:
	default:
		return fmt.Errorf("unsupported verification cost %q", obligation.Cost)
	}
	return nil
}

type VerificationPlanRegistry struct {
	Schema           string                   `json:"schema"`
	PolicyHash       string                   `json:"policy_hash"`
	OperationalPaths []string                 `json:"operational_paths"`
	Obligations      []VerificationObligation `json:"obligations"`
	Digest           string                   `json:"digest"`
}

func NewVerificationPlanRegistry(policyHash string, operationalPaths []string, obligations []VerificationObligation) (VerificationPlanRegistry, error) {
	paths, err := canonicalPaths(operationalPaths)
	if err != nil {
		return VerificationPlanRegistry{}, fmt.Errorf("canonical registry paths: %w", err)
	}
	entries := append([]VerificationObligation{}, obligations...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	registry := VerificationPlanRegistry{
		Schema: VerificationPlanRegistrySchema, PolicyHash: policyHash,
		OperationalPaths: paths, Obligations: entries,
	}
	digest, err := verificationRegistryDigest(registry)
	if err != nil {
		return VerificationPlanRegistry{}, err
	}
	registry.Digest = digest
	if err := registry.Validate(); err != nil {
		return VerificationPlanRegistry{}, err
	}
	return registry, nil
}

func (registry VerificationPlanRegistry) Validate() error {
	if registry.Schema != VerificationPlanRegistrySchema || !validSHA256(registry.PolicyHash) {
		return errors.New("unsupported verification registry schema or policy binding")
	}
	if registry.OperationalPaths == nil {
		return errors.New("verification registry operational paths must be an explicit array")
	}
	paths, err := canonicalPaths(registry.OperationalPaths)
	if err != nil || !equalStrings(paths, registry.OperationalPaths) {
		return errors.New("verification registry operational paths must be canonical")
	}
	if registry.Obligations == nil {
		return errors.New("verification registry obligations must be an explicit array")
	}
	for index, obligation := range registry.Obligations {
		if err := obligation.validate(); err != nil {
			return fmt.Errorf("verification obligation %d: %w", index, err)
		}
		if index > 0 && registry.Obligations[index-1].ID >= obligation.ID {
			return errors.New("verification registry obligations must be sorted with unique IDs")
		}
	}
	want, err := verificationRegistryDigest(registry)
	if err != nil {
		return err
	}
	if registry.Digest != want {
		return errors.New("verification registry digest does not match its canonical content")
	}
	return nil
}

type VerificationPlan struct {
	Schema              string                         `json:"schema"`
	Subject             VerificationSubject            `json:"subject"`
	PolicyHash          string                         `json:"policy_hash"`
	RegistryDigest      string                         `json:"registry_digest"`
	ApplicabilityDigest string                         `json:"applicability_digest"`
	Applicability       VerificationApplicabilityValue `json:"applicability"`
	Obligations         []VerificationObligation       `json:"obligations"`
	Digest              string                         `json:"digest"`
}

// BuildVerificationPlan freezes the complete owner-issued registry projection.
// Callers cannot select or omit obligation IDs. Registries are scope-specific;
// RAR is the authority that constructs them. Unknown and not-applicable
// decisions intentionally issue no work.
func BuildVerificationPlan(applicability VerificationApplicability, registry VerificationPlanRegistry) (VerificationPlan, error) {
	if err := applicability.Validate(); err != nil {
		return VerificationPlan{}, err
	}
	if err := registry.Validate(); err != nil {
		return VerificationPlan{}, err
	}
	if applicability.PolicyHash != registry.PolicyHash || applicability.RegistryDigest != registry.Digest {
		return VerificationPlan{}, errors.New("verification applicability and registry bindings differ")
	}
	selected := []VerificationObligation{}
	if applicability.Decision == VerificationApplicable {
		if len(registry.Obligations) == 0 {
			return VerificationPlan{}, errors.New("applicable verification requires at least one exact obligation")
		}
		selected = append(selected, registry.Obligations...)
	}
	plan := VerificationPlan{
		Schema: VerificationPlanSchema, Subject: applicability.Subject,
		PolicyHash: applicability.PolicyHash, RegistryDigest: registry.Digest,
		ApplicabilityDigest: applicability.Digest, Applicability: applicability.Decision,
		Obligations: selected,
	}
	digest, err := verificationPlanDigest(plan)
	if err != nil {
		return VerificationPlan{}, err
	}
	plan.Digest = digest
	if err := ValidateVerificationPlan(applicability, registry, plan); err != nil {
		return VerificationPlan{}, err
	}
	return plan, nil
}

// validateStructure checks only canonical self-consistency. Authority-sensitive
// callers must use ValidateVerificationPlan with the exact applicability and
// registry preimages; a digest alone cannot prove registry membership.
func (plan VerificationPlan) validateStructure() error {
	if plan.Schema != VerificationPlanSchema {
		return errors.New("unsupported verification plan schema")
	}
	if err := plan.Subject.Validate(); err != nil {
		return err
	}
	if !validSHA256(plan.PolicyHash) || !validSHA256(plan.RegistryDigest) || !validSHA256(plan.ApplicabilityDigest) {
		return errors.New("verification plan requires exact policy, registry, and applicability bindings")
	}
	if plan.Obligations == nil {
		return errors.New("verification plan obligations must be an explicit array")
	}
	switch plan.Applicability {
	case VerificationApplicable:
		if len(plan.Obligations) == 0 {
			return errors.New("applicable verification plan requires obligations")
		}
	case VerificationNotApplicable, VerificationUnknown:
		if len(plan.Obligations) != 0 {
			return errors.New("non-executable verification applicability cannot carry obligations")
		}
	default:
		return fmt.Errorf("unsupported verification plan applicability %q", plan.Applicability)
	}
	for index, obligation := range plan.Obligations {
		if err := obligation.validate(); err != nil {
			return fmt.Errorf("verification plan obligation %d: %w", index, err)
		}
		if index > 0 && plan.Obligations[index-1].ID >= obligation.ID {
			return errors.New("verification plan obligations must be sorted with unique IDs")
		}
	}
	want, err := verificationPlanDigest(plan)
	if err != nil {
		return err
	}
	if plan.Digest != want {
		return errors.New("verification plan digest does not match its canonical content")
	}
	return nil
}

// ValidateVerificationPlan proves that a canonical plan was derived from the
// exact native applicability decision and registry, including every global
// obligation. It intentionally requires the authority preimages.
func ValidateVerificationPlan(
	applicability VerificationApplicability,
	registry VerificationPlanRegistry,
	plan VerificationPlan,
) error {
	if err := applicability.Validate(); err != nil {
		return err
	}
	if err := registry.Validate(); err != nil {
		return err
	}
	if err := plan.validateStructure(); err != nil {
		return err
	}
	if applicability.PolicyHash != registry.PolicyHash || applicability.RegistryDigest != registry.Digest {
		return errors.New("verification applicability and registry bindings differ")
	}
	if plan.Subject != applicability.Subject || plan.PolicyHash != applicability.PolicyHash ||
		plan.RegistryDigest != registry.Digest || plan.ApplicabilityDigest != applicability.Digest ||
		plan.Applicability != applicability.Decision {
		return errors.New("verification plan does not bind the exact applicability and registry")
	}
	if applicability.Decision == VerificationApplicable {
		if len(plan.Obligations) != len(registry.Obligations) {
			return errors.New("verification plan must contain the complete owner-issued registry")
		}
		for index, obligation := range plan.Obligations {
			if obligation != registry.Obligations[index] {
				return fmt.Errorf("verification plan obligation %q is not the exact registry obligation", obligation.ID)
			}
		}
	}
	return nil
}

type VerificationAggregate string

const (
	VerificationAggregateNotRequired VerificationAggregate = "not_required"
	VerificationAggregateComplete    VerificationAggregate = "complete"
	VerificationAggregateFailed      VerificationAggregate = "failed"
	VerificationAggregatePartial     VerificationAggregate = "partial"
	VerificationAggregateUnavailable VerificationAggregate = "unavailable"
)

// VerificationResultRef binds an external typed result to the exact RAR plan.
// ResultRef and EvidenceRefs are opaque immutable SHA-256 identities; RAR never
// imports the evidence-provider package.
type VerificationResultRef struct {
	Schema               string                `json:"schema"`
	ResultRef            string                `json:"result_ref"`
	Subject              VerificationSubject   `json:"subject"`
	PolicyHash           string                `json:"policy_hash"`
	PlanDigest           string                `json:"plan_digest"`
	ApplicabilityDigest  string                `json:"applicability_digest"`
	Aggregate            VerificationAggregate `json:"aggregate"`
	CompletedObligations []string              `json:"completed_obligations"`
	EvidenceRefs         []string              `json:"evidence_refs"`
}

func (result VerificationResultRef) Validate() error {
	if result.Schema != VerificationResultRefSchema || !validSHA256(result.ResultRef) {
		return errors.New("unsupported verification result-ref schema or immutable result ref")
	}
	if err := result.Subject.Validate(); err != nil {
		return err
	}
	if !validSHA256(result.PolicyHash) || !validSHA256(result.PlanDigest) || !validSHA256(result.ApplicabilityDigest) {
		return errors.New("verification result ref requires exact policy, plan, and applicability bindings")
	}
	if !canonicalVerificationIDs(result.CompletedObligations) || !canonicalSHA256Refs(result.EvidenceRefs) {
		return errors.New("verification result ref arrays must be canonical")
	}
	switch result.Aggregate {
	case VerificationAggregateNotRequired, VerificationAggregateComplete, VerificationAggregateFailed,
		VerificationAggregatePartial, VerificationAggregateUnavailable:
	default:
		return fmt.Errorf("unsupported verification aggregate %q", result.Aggregate)
	}
	return nil
}

func ValidateVerificationResultRef(
	applicability VerificationApplicability,
	registry VerificationPlanRegistry,
	plan VerificationPlan,
	result VerificationResultRef,
) error {
	if err := ValidateVerificationPlan(applicability, registry, plan); err != nil {
		return err
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if result.Subject != plan.Subject || result.PolicyHash != plan.PolicyHash ||
		result.PlanDigest != plan.Digest || result.ApplicabilityDigest != plan.ApplicabilityDigest {
		return errors.New("verification result ref does not bind the exact plan subject and policy")
	}
	planned := verificationObligationIDs(plan.Obligations)
	for _, completed := range result.CompletedObligations {
		if !containsSorted(planned, completed) {
			return fmt.Errorf("verification result completed unplanned obligation %q", completed)
		}
	}
	switch result.Aggregate {
	case VerificationAggregateNotRequired:
		if plan.Applicability != VerificationNotApplicable || len(planned) != 0 ||
			len(result.CompletedObligations) != 0 {
			return errors.New("not-required verification requires a native not-applicable empty plan")
		}
	case VerificationAggregateComplete:
		if plan.Applicability != VerificationApplicable {
			return errors.New("complete verification requires a native applicable plan")
		}
		if !equalStrings(planned, result.CompletedObligations) {
			return errors.New("complete verification must complete every planned obligation exactly")
		}
	default:
		if plan.Applicability != VerificationApplicable {
			return errors.New("unknown or not-applicable verification cannot publish an execution result")
		}
	}
	return nil
}

type CorrectionImpactClosure struct {
	Schema                     string                `json:"schema"`
	OriginalSubject            VerificationSubject   `json:"original_subject"`
	CorrectedSubject           VerificationSubject   `json:"corrected_subject"`
	OriginalPlanDigest         string                `json:"original_plan_digest"`
	CorrectedPlanDigest        string                `json:"corrected_plan_digest"`
	OriginalPolicyHash         string                `json:"original_policy_hash"`
	CorrectedPolicyHash        string                `json:"corrected_policy_hash"`
	OriginalRegistryDigest     string                `json:"original_registry_digest"`
	CorrectedRegistryDigest    string                `json:"corrected_registry_digest"`
	AffectedObligations        []string              `json:"affected_obligations"`
	MandatoryGlobalObligations []string              `json:"mandatory_global_obligations"`
	RerunObligations           []string              `json:"rerun_obligations"`
	Result                     VerificationResultRef `json:"result"`
	Digest                     string                `json:"digest"`
}

// BuildCorrectionImpactClosure deliberately has no caller-supplied affected
// set. Until owner-derived dependency graphs land, RAR takes the conservative
// safe closure: every corrected obligation is affected and rerun.
func BuildCorrectionImpactClosure(
	originalApplicability VerificationApplicability,
	originalRegistry VerificationPlanRegistry,
	originalPlan VerificationPlan,
	correctedApplicability VerificationApplicability,
	correctedRegistry VerificationPlanRegistry,
	correctedPlan VerificationPlan,
	result VerificationResultRef,
) (CorrectionImpactClosure, error) {
	if err := ValidateVerificationPlan(originalApplicability, originalRegistry, originalPlan); err != nil {
		return CorrectionImpactClosure{}, err
	}
	if err := ValidateVerificationPlan(correctedApplicability, correctedRegistry, correctedPlan); err != nil {
		return CorrectionImpactClosure{}, err
	}
	if err := ValidateVerificationResultRef(correctedApplicability, correctedRegistry, correctedPlan, result); err != nil {
		return CorrectionImpactClosure{}, err
	}
	affectedIDs := verificationObligationIDs(correctedPlan.Obligations)
	mandatory := make([]string, 0)
	for _, obligation := range correctedPlan.Obligations {
		if obligation.MandatoryGlobal {
			mandatory = append(mandatory, obligation.ID)
		}
	}
	rerun := append([]string{}, affectedIDs...)
	closure := CorrectionImpactClosure{
		Schema:          CorrectionImpactClosureSchema,
		OriginalSubject: originalPlan.Subject, CorrectedSubject: correctedPlan.Subject,
		OriginalPlanDigest: originalPlan.Digest, CorrectedPlanDigest: correctedPlan.Digest,
		OriginalPolicyHash: originalPlan.PolicyHash, CorrectedPolicyHash: correctedPlan.PolicyHash,
		OriginalRegistryDigest: originalPlan.RegistryDigest, CorrectedRegistryDigest: correctedPlan.RegistryDigest,
		AffectedObligations:        affectedIDs,
		MandatoryGlobalObligations: mandatory, RerunObligations: rerun, Result: result,
	}
	digest, err := correctionImpactClosureDigest(closure)
	if err != nil {
		return CorrectionImpactClosure{}, err
	}
	closure.Digest = digest
	if err := ValidateCorrectionImpactClosure(
		originalApplicability,
		originalRegistry,
		originalPlan,
		correctedApplicability,
		correctedRegistry,
		correctedPlan,
		closure,
	); err != nil {
		return CorrectionImpactClosure{}, err
	}
	return closure, nil
}

func ValidateCorrectionImpactClosure(
	originalApplicability VerificationApplicability,
	originalRegistry VerificationPlanRegistry,
	originalPlan VerificationPlan,
	correctedApplicability VerificationApplicability,
	correctedRegistry VerificationPlanRegistry,
	correctedPlan VerificationPlan,
	closure CorrectionImpactClosure,
) error {
	if err := ValidateVerificationPlan(originalApplicability, originalRegistry, originalPlan); err != nil {
		return err
	}
	if err := ValidateVerificationPlan(correctedApplicability, correctedRegistry, correctedPlan); err != nil {
		return err
	}
	if closure.Schema != CorrectionImpactClosureSchema {
		return errors.New("unsupported correction-impact closure schema")
	}
	if originalPlan.Subject == correctedPlan.Subject {
		return errors.New("correction-impact closure requires a changed immutable subject")
	}
	if closure.OriginalSubject != originalPlan.Subject || closure.CorrectedSubject != correctedPlan.Subject ||
		closure.OriginalPlanDigest != originalPlan.Digest || closure.CorrectedPlanDigest != correctedPlan.Digest ||
		closure.OriginalPolicyHash != originalPlan.PolicyHash ||
		closure.CorrectedPolicyHash != correctedPlan.PolicyHash ||
		closure.OriginalRegistryDigest != originalPlan.RegistryDigest ||
		closure.CorrectedRegistryDigest != correctedPlan.RegistryDigest {
		return errors.New("correction-impact closure does not bind the exact plans")
	}
	if !canonicalVerificationIDs(closure.AffectedObligations) ||
		!canonicalVerificationIDs(closure.MandatoryGlobalObligations) ||
		!canonicalVerificationIDs(closure.RerunObligations) {
		return errors.New("correction-impact obligation sets must be canonical")
	}
	originalPlanned := verificationObligationIDs(originalPlan.Obligations)
	correctedPlanned := verificationObligationIDs(correctedPlan.Obligations)
	if correctedPlan.Applicability == VerificationApplicable {
		for _, id := range originalPlanned {
			if !containsSorted(correctedPlanned, id) {
				return fmt.Errorf("corrected verification plan drops original obligation %q", id)
			}
		}
	}
	mandatory := make([]string, 0)
	for _, obligation := range correctedPlan.Obligations {
		if obligation.MandatoryGlobal {
			mandatory = append(mandatory, obligation.ID)
		}
	}
	if !equalStrings(correctedPlanned, closure.AffectedObligations) ||
		!equalStrings(mandatory, closure.MandatoryGlobalObligations) ||
		!equalStrings(correctedPlanned, closure.RerunObligations) {
		return errors.New("correction-impact closure must rerun the exact conservative corrected plan")
	}
	switch correctedPlan.Applicability {
	case VerificationApplicable:
		if closure.Result.Aggregate != VerificationAggregateComplete {
			return errors.New("applicable correction closure requires a complete result")
		}
	case VerificationNotApplicable:
		if closure.Result.Aggregate != VerificationAggregateNotRequired {
			return errors.New("not-applicable correction closure requires a not-required result")
		}
	default:
		return errors.New("unknown corrected applicability cannot close a correction")
	}
	if err := ValidateVerificationResultRef(
		correctedApplicability,
		correctedRegistry,
		correctedPlan,
		closure.Result,
	); err != nil {
		return err
	}
	want, err := correctionImpactClosureDigest(closure)
	if err != nil {
		return err
	}
	if closure.Digest != want {
		return errors.New("correction-impact closure digest does not match its canonical content")
	}
	return nil
}

// ClassifyVerificationApplicability derives a native, versioned decision from
// the immutable snapshot and a validated RAR registry. It does not ask a model
// whether verification is necessary.
func (builder SnapshotBuilder) ClassifyVerificationApplicability(
	ctx context.Context,
	snapshot Snapshot,
	registry VerificationPlanRegistry,
	evidenceRefs []string,
) (VerificationApplicability, error) {
	if err := registry.Validate(); err != nil {
		return VerificationApplicability{}, err
	}
	if err := builder.ValidateEvidence(ctx, snapshot); err != nil {
		return VerificationApplicability{}, fmt.Errorf("validate verification subject: %w", err)
	}
	subject, err := VerificationSubjectFromSnapshot(snapshot)
	if err != nil {
		return VerificationApplicability{}, err
	}
	if !canonicalSHA256Refs(evidenceRefs) {
		return VerificationApplicability{}, errors.New("verification evidence refs must be canonical lowercase SHA-256 identities")
	}
	stats, err := builder.DiffStats(ctx, snapshot)
	if err != nil {
		return VerificationApplicability{}, err
	}
	repo, err := builder.repositoryRoot(ctx)
	if err != nil {
		return VerificationApplicability{}, err
	}
	isolation, cleanup, err := isolatedImmutableTreeGit(ctx, repo)
	if err != nil {
		return VerificationApplicability{}, fmt.Errorf("prepare isolated verification classifier: %w", err)
	}
	defer cleanup()
	registryPaths := make(map[string]struct{}, len(registry.OperationalPaths))
	for _, logicalPath := range registry.OperationalPaths {
		registryPaths[logicalPath] = struct{}{}
	}
	reasons := make([]VerificationApplicabilityReason, 0, len(stats)+1)
	hasPassive, hasActive, hasUnknown := false, false, false
	for _, stat := range stats {
		activity, reason, classifyErr := builder.classifyVerificationStat(ctx, snapshot, stat, registryPaths, repo, isolation)
		if classifyErr != nil {
			return VerificationApplicability{}, classifyErr
		}
		reasons = append(reasons, reason)
		switch activity {
		case VerificationContentPassive:
			hasPassive = true
		case VerificationContentActive:
			hasActive = true
		default:
			hasUnknown = true
		}
	}
	if len(stats) == 0 {
		hasUnknown = true
		reasons = append(reasons, VerificationApplicabilityReason{Code: VerificationReasonUnknownContent})
	}
	if hasPassive && hasActive {
		reasons = append(reasons, VerificationApplicabilityReason{Code: VerificationReasonMixedContent})
	}
	reasons = canonicalVerificationReasons(reasons)
	decision, activity := VerificationNotApplicable, VerificationContentPassive
	switch {
	case hasUnknown:
		decision, activity = VerificationUnknown, VerificationContentUnknown
	case hasActive:
		decision, activity = VerificationApplicable, VerificationContentActive
	}
	applicability := VerificationApplicability{
		Schema: VerificationApplicabilitySchema, ClassifierVersion: VerificationClassifierVersion,
		Subject: subject, PolicyHash: registry.PolicyHash, RegistryDigest: registry.Digest,
		Decision: decision, ContentActivity: activity, Reasons: reasons,
		EvidenceRefs: append([]string{}, evidenceRefs...),
	}
	digest, err := verificationApplicabilityDigest(applicability)
	if err != nil {
		return VerificationApplicability{}, err
	}
	applicability.Digest = digest
	if err := applicability.Validate(); err != nil {
		return VerificationApplicability{}, err
	}
	return applicability, nil
}

func (builder SnapshotBuilder) classifyVerificationStat(
	ctx context.Context,
	snapshot Snapshot,
	stat DiffStat,
	registryPaths map[string]struct{},
	repo string,
	isolation []string,
) (VerificationContentActivity, VerificationApplicabilityReason, error) {
	reason := VerificationApplicabilityReason{Path: stat.Path}
	if stat.ModeOnly {
		reason.Code = VerificationReasonUnknownModeChange
		return VerificationContentUnknown, reason, nil
	}
	for _, mode := range []string{stat.OldMode, stat.NewMode} {
		switch mode {
		case "", "000000", "100644":
		case "120000":
			reason.Code = VerificationReasonUnknownSymlink
			return VerificationContentUnknown, reason, nil
		case "160000":
			reason.Code = VerificationReasonUnknownSubmodule
			return VerificationContentUnknown, reason, nil
		default:
			reason.Code = VerificationReasonUnknownMode
			return VerificationContentUnknown, reason, nil
		}
	}
	if _, active := registryPaths[stat.Path]; active {
		reason.Code = VerificationReasonActiveRegistryPath
		return VerificationContentActive, reason, nil
	}
	if stat.Generated || isOperationalVerificationDocumentPath(stat.Path) {
		reason.Code = VerificationReasonActiveOperational
		return VerificationContentActive, reason, nil
	}
	if isConfigurationReviewPath(stat.Path) {
		reason.Code = VerificationReasonActiveConfig
		return VerificationContentActive, reason, nil
	}
	extension := asciiLower(path.Ext(stat.Path))
	if extension == ".mdx" {
		if stat.Binary {
			reason.Code = VerificationReasonUnknownBinary
			return VerificationContentUnknown, reason, nil
		}
		static, err := verificationMDXIsStatic(ctx, snapshot, stat, repo, isolation)
		if err != nil {
			return "", VerificationApplicabilityReason{}, err
		}
		if static {
			reason.Code = VerificationReasonPassiveDocument
			return VerificationContentPassive, reason, nil
		}
		reason.Code = VerificationReasonActiveMDX
		return VerificationContentActive, reason, nil
	}
	if isPassiveDocumentPath(stat.Path) {
		if stat.Binary {
			reason.Code = VerificationReasonUnknownBinary
			return VerificationContentUnknown, reason, nil
		}
		activity, err := verificationDocumentActivity(ctx, snapshot, stat, repo, isolation)
		if err != nil {
			return "", VerificationApplicabilityReason{}, err
		}
		switch activity {
		case VerificationContentPassive:
			reason.Code = VerificationReasonPassiveDocument
		case VerificationContentActive:
			reason.Code = VerificationReasonActiveSource
		default:
			reason.Code = VerificationReasonUnknownContent
		}
		return activity, reason, nil
	}
	if isRasterImageExtension(extension) {
		reason.Code = VerificationReasonPassiveImage
		return VerificationContentPassive, reason, nil
	}
	if extension == ".svg" {
		if stat.Binary {
			reason.Code = VerificationReasonUnknownBinary
			return VerificationContentUnknown, reason, nil
		}
		passive, err := verificationSVGIsPassive(ctx, snapshot, stat, repo, isolation)
		if err != nil {
			return "", VerificationApplicabilityReason{}, err
		}
		if passive {
			reason.Code = VerificationReasonPassiveImage
			return VerificationContentPassive, reason, nil
		}
		reason.Code = VerificationReasonActiveSource
		return VerificationContentActive, reason, nil
	}
	if stat.Binary {
		reason.Code = VerificationReasonUnknownBinary
		return VerificationContentUnknown, reason, nil
	}
	if isKnownActiveVerificationPath(stat.Path) {
		reason.Code = VerificationReasonActiveSource
		return VerificationContentActive, reason, nil
	}
	reason.Code = VerificationReasonUnknownContent
	return VerificationContentUnknown, reason, nil
}

func verificationMDXIsStatic(ctx context.Context, snapshot Snapshot, stat DiffStat, repo string, isolation []string) (bool, error) {
	for _, version := range []struct {
		tree string
		mode string
	}{{snapshot.BaseTree, stat.OldMode}, {snapshot.CandidateTree, stat.NewMode}} {
		if version.mode == "" || version.mode == "000000" {
			continue
		}
		content, err := runGitIsolated(ctx, repo, isolation, nil, "cat-file", "blob", version.tree+":"+stat.Path)
		if err != nil {
			return false, fmt.Errorf("read immutable MDX %q: %w", stat.Path, err)
		}
		if !isStaticMDXDocument(content) {
			return false, nil
		}
	}
	return true, nil
}

func verificationDocumentActivity(
	ctx context.Context,
	snapshot Snapshot,
	stat DiffStat,
	repo string,
	isolation []string,
) (VerificationContentActivity, error) {
	for _, version := range []struct {
		tree string
		mode string
	}{{snapshot.BaseTree, stat.OldMode}, {snapshot.CandidateTree, stat.NewMode}} {
		if version.mode == "" || version.mode == "000000" {
			continue
		}
		content, err := runGitIsolated(ctx, repo, isolation, nil, "cat-file", "blob", version.tree+":"+stat.Path)
		if err != nil {
			return "", fmt.Errorf("read immutable document %q: %w", stat.Path, err)
		}
		activity := verificationDocumentContentActivity(path.Ext(asciiLower(stat.Path)), content)
		if activity != VerificationContentPassive {
			return activity, nil
		}
	}
	return VerificationContentPassive, nil
}

// verificationDocumentContentActivity recognizes only inert human-facing
// prose. Executable HTML, templating, and file-inclusion directives are active
// even when hidden behind a passive-looking extension. Invalid text is unknown.
func verificationDocumentContentActivity(extension string, content []byte) VerificationContentActivity {
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return VerificationContentUnknown
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	inFence := false
	var fence string
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		lower := asciiLower(line)
		if extension == ".md" || extension == "" {
			if marker := markdownFenceMarker(line); marker != "" {
				if !inFence {
					inFence, fence = true, marker
				} else if marker == fence {
					inFence, fence = false, ""
				}
				continue
			}
			if inFence {
				continue
			}
		}
		if documentLineIsActive(extension, lower) {
			return VerificationContentActive
		}
	}
	if inFence {
		return VerificationContentUnknown
	}
	return VerificationContentPassive
}

func markdownFenceMarker(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			count := 0
			for count < len(trimmed) && trimmed[count] == marker[0] {
				count++
			}
			if count >= 3 {
				return strings.Repeat(marker[:1], count)
			}
		}
	}
	return ""
}

func documentLineIsActive(extension, lower string) bool {
	commonMarkers := []string{
		"<script", "</script", "javascript:", "onload=", "onclick=", "onerror=",
		"{%", "{{", "<iframe", "<object", "<embed",
	}
	for _, marker := range commonMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if verificationHTMLHandlerPattern.MatchString(lower) {
		return true
	}
	for _, marker := range []string{
		"srcdoc=", "srcdoc =", "http-equiv=", "http-equiv =",
		"<link", "<style", "<form", "<input", "<button",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	switch extension {
	case ".adoc":
		for _, prefix := range []string{
			"include::", "ifdef::", "ifndef::", "ifeval::", "eval::", "pass::", "pass:",
		} {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
		if lower == "++++" {
			return true
		}
	case ".rst":
		for _, prefix := range []string{
			".. include::", ".. literalinclude::", ".. raw::",
		} {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
	default:
		for _, prefix := range []string{
			"import ", "export ", "!include ", "include::",
		} {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
	}
	return false
}

var verificationHTMLHandlerPattern = regexp.MustCompile(`(?:^|[[:space:]<])on[a-z0-9_-]+[[:space:]]*=`)

func verificationSVGIsPassive(ctx context.Context, snapshot Snapshot, stat DiffStat, repo string, isolation []string) (bool, error) {
	for _, version := range []struct {
		tree string
		mode string
	}{{snapshot.BaseTree, stat.OldMode}, {snapshot.CandidateTree, stat.NewMode}} {
		if version.mode == "" || version.mode == "000000" {
			continue
		}
		content, err := runGitIsolated(ctx, repo, isolation, nil, "cat-file", "blob", version.tree+":"+stat.Path)
		if err != nil {
			return false, fmt.Errorf("read immutable SVG %q: %w", stat.Path, err)
		}
		if !isPassiveSVG(content) {
			return false, nil
		}
	}
	return true, nil
}

func isPassiveSVG(content []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	sawRoot := false
	for {
		token, err := decoder.Token()
		if err != nil {
			return errors.Is(err, io.EOF) && sawRoot
		}
		switch item := token.(type) {
		case xml.Directive:
			return false
		case xml.ProcInst:
			if !strings.EqualFold(item.Target, "xml") {
				return false
			}
		case xml.StartElement:
			if !sawRoot {
				if !strings.EqualFold(item.Name.Local, "svg") {
					return false
				}
				sawRoot = true
			}
			switch asciiLower(item.Name.Local) {
			case "script", "foreignobject", "iframe", "object", "embed", "style":
				return false
			}
			for _, attribute := range item.Attr {
				name := asciiLower(attribute.Name.Local)
				value := strings.TrimSpace(asciiLower(attribute.Value))
				if strings.HasPrefix(name, "on") || strings.Contains(value, "javascript:") ||
					strings.Contains(value, "expression(") || strings.Contains(value, "url(http:") ||
					strings.Contains(value, "url(https:") || strings.Contains(value, "url(//") {
					return false
				}
				if (name == "href" || name == "src") && value != "" && !strings.HasPrefix(value, "#") {
					return false
				}
			}
		}
	}
}

func isPassiveDocumentPath(logicalPath string) bool {
	lower := asciiLower(logicalPath)
	if isOperationalVerificationDocumentPath(lower) {
		return false
	}
	switch path.Ext(lower) {
	case ".md", ".rst", ".adoc":
		return true
	case "":
		return strings.HasPrefix(path.Base(lower), "readme")
	default:
		return false
	}
}

func isOperationalVerificationDocumentPath(logicalPath string) bool {
	if isOperationalMarkdownPath(logicalPath) {
		return true
	}
	lower := asciiLower(logicalPath)
	for _, segment := range strings.Split(lower, "/") {
		stem := strings.TrimSuffix(segment, path.Ext(segment))
		for _, token := range strings.FieldsFunc(stem, func(char rune) bool {
			return (char < 'a' || char > 'z') && (char < '0' || char > '9')
		}) {
			switch token {
			case "policy", "policies", "rule", "rules":
				return true
			}
		}
	}
	return false
}

func isRasterImageExtension(extension string) bool {
	switch extension {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".avif":
		return true
	default:
		return false
	}
}

func isKnownActiveVerificationPath(logicalPath string) bool {
	extension := asciiLower(path.Ext(logicalPath))
	if _, ok := semanticSourceExtensions[extension]; ok {
		return true
	}
	switch extension {
	case ".css", ".scss", ".sass", ".less", ".html", ".htm", ".sql", ".graphql",
		".proto", ".xml", ".wasm", ".vue", ".svelte", ".lua", ".pl", ".swift":
		return true
	default:
		return false
	}
}

func verificationReasonsCanonical(reasons []VerificationApplicabilityReason) bool {
	return equalVerificationReasons(canonicalVerificationReasons(reasons), reasons)
}

func canonicalVerificationReasons(reasons []VerificationApplicabilityReason) []VerificationApplicabilityReason {
	result := append([]VerificationApplicabilityReason(nil), reasons...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Code != result[j].Code {
			return result[i].Code < result[j].Code
		}
		return result[i].Path < result[j].Path
	})
	canonical := result[:0]
	for _, reason := range result {
		if reason.Path != "" {
			logicalPath, err := normalizeLogicalPath(reason.Path)
			if err != nil || logicalPath != reason.Path {
				continue
			}
		}
		if len(canonical) == 0 || canonical[len(canonical)-1] != reason {
			canonical = append(canonical, reason)
		}
	}
	return canonical
}

func verificationReasonCategory(reason VerificationApplicabilityReason) (VerificationContentActivity, bool) {
	var category VerificationContentActivity
	switch reason.Code {
	case VerificationReasonPassiveDocument, VerificationReasonPassiveImage:
		category = VerificationContentPassive
	case VerificationReasonActiveSource, VerificationReasonActiveConfig, VerificationReasonActiveMDX,
		VerificationReasonActiveOperational, VerificationReasonActiveRegistryPath:
		category = VerificationContentActive
	case VerificationReasonUnknownBinary, VerificationReasonUnknownMode, VerificationReasonUnknownModeChange,
		VerificationReasonUnknownSubmodule, VerificationReasonUnknownSymlink:
		category = VerificationContentUnknown
	case VerificationReasonUnknownContent:
		category = VerificationContentUnknown
	case VerificationReasonMixedContent:
		return "", reason.Path == ""
	default:
		return "", false
	}
	if reason.Code != VerificationReasonUnknownContent && reason.Path == "" {
		return "", false
	}
	return category, true
}

func equalVerificationReasons(left, right []VerificationApplicabilityReason) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validVerificationID(value string) bool {
	if value == "" || len(value) > 160 || value != strings.TrimSpace(value) {
		return false
	}
	for index, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' ||
			char == '.' || char == '_' || char == '-' || char == '/' || char == ':'
		if !valid || index == 0 && !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}

func canonicalVerificationIDs(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if !validVerificationID(value) || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func canonicalSHA256Refs(values []string) bool {
	if values == nil {
		return false
	}
	for index, value := range values {
		if !validSHA256(value) || index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func verificationObligationIDs(obligations []VerificationObligation) []string {
	ids := make([]string, len(obligations))
	for index, obligation := range obligations {
		ids[index] = obligation.ID
	}
	return ids
}

func containsSorted(values []string, value string) bool {
	index := sort.SearchStrings(values, value)
	return index < len(values) && values[index] == value
}

func sortedUnion(left, right []string) []string {
	values := append(append([]string(nil), left...), right...)
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func verificationApplicabilityDigest(value VerificationApplicability) (string, error) {
	value.Digest = ""
	return verificationContractDigest("gentle-ai.verification-applicability-digest/v1", value)
}

func verificationRegistryDigest(value VerificationPlanRegistry) (string, error) {
	value.Digest = ""
	return verificationContractDigest("gentle-ai.verification-plan-registry-digest/v1", value)
}

func verificationPlanDigest(value VerificationPlan) (string, error) {
	value.Digest = ""
	return verificationContractDigest("gentle-ai.verification-plan-digest/v1", value)
}

func correctionImpactClosureDigest(value CorrectionImpactClosure) (string, error) {
	value.Digest = ""
	return verificationContractDigest("gentle-ai.correction-impact-closure-digest/v1", value)
}

func verificationContractDigest(domain string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
