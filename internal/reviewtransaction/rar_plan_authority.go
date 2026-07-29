package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
)

const (
	RARPlanAuthoritySchema = "gentle-ai.rar-plan-authority/v1"

	VerificationFrozenPlanConsentSchema   = "gentle-ai.verification-frozen-plan-consent/v1"
	VerificationEffectAuthorizationSchema = "gentle-ai.verification-effect-authorization/v1"

	rarPlanAuthorityDigestDomain              = "gentle-ai.rar-plan-authority-digest/v1"
	verificationFrozenPlanConsentDigestDomain = "gentle-ai.verification-frozen-plan-consent-digest/v1"
	verificationEffectAuthorizationDomain     = "gentle-ai.verification-effect-authorization-digest/v1"
)

// RARPlanAuthority is the complete RAR-owned pre-execution plan preimage.
// AuthorityRef is also the PlanRevisionRef consumed by WorkRun. Repository
// identity and the full Snapshot are retained because the smaller Subject is
// intentionally insufficient to revalidate a live workspace by itself.
//
// Effects is the single place where all four independent verification
// dimensions are frozen together. It is a pointer because a plan that runs
// nothing has no effect to declare; an executable plan must declare all of
// them, and omission is rejected rather than defaulted to harmless.
type RARPlanAuthority struct {
	Schema             string                     `json:"schema"`
	AuthorityRef       string                     `json:"authority_ref"`
	RepositoryIdentity string                     `json:"repository_identity"`
	Snapshot           Snapshot                   `json:"snapshot"`
	Subject            VerificationSubject        `json:"subject"`
	Applicability      VerificationApplicability  `json:"applicability"`
	Registry           VerificationPlanRegistry   `json:"registry"`
	Plan               VerificationPlan           `json:"plan"`
	Effects            *VerificationEffectProfile `json:"effects,omitempty"`
}

// Validate proves canonical self-consistency. Only ResolvePlan additionally
// proves that the bound repository identity and exact Snapshot are still live.
// PolicyHash is owned by the emitted registry/plan contract; this package does
// not invent a second external policy owner.
func (authority RARPlanAuthority) Validate() error {
	if authority.Schema != RARPlanAuthoritySchema ||
		!validSHA256(authority.AuthorityRef) ||
		!validSHA256(authority.RepositoryIdentity) {
		return errors.New("invalid RAR plan authority identity")
	}
	subject, err := VerificationSubjectFromSnapshot(authority.Snapshot)
	if err != nil {
		return fmt.Errorf("validate RAR plan snapshot subject: %w", err)
	}
	if subject != authority.Subject ||
		authority.Applicability.Subject != authority.Subject ||
		authority.Plan.Subject != authority.Subject {
		return errors.New("RAR plan authority does not bind one exact snapshot subject")
	}
	if err := ValidateVerificationPlan(
		authority.Applicability,
		authority.Registry,
		authority.Plan,
	); err != nil {
		return fmt.Errorf("validate RAR plan contracts: %w", err)
	}
	if err := validatePlanEffectBinding(authority.Plan, authority.Effects); err != nil {
		return err
	}
	want, err := rarPlanAuthorityDigest(authority)
	if err != nil {
		return err
	}
	if authority.AuthorityRef != want {
		return errors.New("RAR plan authority ref does not match its canonical content")
	}
	return nil
}

// RARPlanPublication contains only the exact frozen snapshot and the complete
// RAR verification-plan contracts. It deliberately has no result, outcome,
// candidate-head, or caller-selected storage field.
type RARPlanPublication struct {
	Snapshot      Snapshot
	Applicability VerificationApplicability
	Registry      VerificationPlanRegistry
	Plan          VerificationPlan
	Effects       *VerificationEffectProfile
}

// validatePlanEffectBinding proves the authority binds all four independent
// dimensions. An executable plan must declare cost, mutation effect, and
// permission effect, and its declared cost must be the exact fold of the frozen
// obligations so a "quick" claim cannot hide a long obligation. A plan that
// runs nothing must not claim effects it can never produce.
func validatePlanEffectBinding(plan VerificationPlan, effects *VerificationEffectProfile) error {
	if plan.Applicability != VerificationApplicable {
		if effects != nil {
			return errors.New("verification plan that runs nothing cannot bind effect dimensions")
		}
		return nil
	}
	if effects == nil {
		return fmt.Errorf("%w: applicable verification plan effect profile", ErrVerificationDimensionMissing)
	}
	if err := effects.Validate(); err != nil {
		return fmt.Errorf("validate RAR plan effect dimensions: %w", err)
	}
	if effects.Applicability != plan.Applicability {
		return errors.New("RAR plan effect profile does not bind the exact plan applicability")
	}
	if cost := AggregateVerificationCost(plan.Obligations); effects.Cost != cost {
		return fmt.Errorf(
			"RAR plan effect cost %q does not bind the exact obligation cost %q",
			effects.Cost,
			cost,
		)
	}
	return nil
}

// Gate resolves the single proportional decision for this exact frozen plan.
// A plan that runs nothing is resolved from applicability alone; it declares no
// cost or effect, so none is invented for it here either.
func (authority RARPlanAuthority) Gate() (VerificationGate, error) {
	if err := authority.Validate(); err != nil {
		return rejectedVerificationGate(), fmt.Errorf("resolve RAR plan gate: %w", err)
	}
	switch authority.Plan.Applicability {
	case VerificationNotApplicable:
		return VerificationGate{Decision: VerificationDecisionRecordNotApplicable}, nil
	case VerificationUnknown:
		return VerificationGate{Decision: VerificationDecisionRecordEvidenceGap}, nil
	}
	return ResolveVerificationGate(*authority.Effects)
}

// VerificationFrozenPlanConsent is the single human consent for an expensive
// frozen plan. It binds the exact authority and plan so a resume replays that
// plan instead of regenerating it or asking a second time.
type VerificationFrozenPlanConsent struct {
	Schema       string                    `json:"schema"`
	AuthorityRef string                    `json:"authority_ref"`
	PlanDigest   string                    `json:"plan_digest"`
	Effects      VerificationEffectProfile `json:"effects"`
	ConsentRef   string                    `json:"consent_ref"`
}

// NewVerificationFrozenPlanConsent records consent only for a plan whose gate
// actually asks for it. Nothing else can manufacture a consent record.
func NewVerificationFrozenPlanConsent(authority RARPlanAuthority) (VerificationFrozenPlanConsent, error) {
	gate, err := authority.Gate()
	if err != nil {
		return VerificationFrozenPlanConsent{}, err
	}
	if !gate.RequiresFrozenPlanConsent {
		return VerificationFrozenPlanConsent{}, errors.New("verification plan does not require frozen-plan consent")
	}
	consent := VerificationFrozenPlanConsent{
		Schema:       VerificationFrozenPlanConsentSchema,
		AuthorityRef: authority.AuthorityRef,
		PlanDigest:   authority.Plan.Digest,
		Effects:      *authority.Effects,
	}
	if consent.ConsentRef, err = verificationFrozenPlanConsentDigest(consent); err != nil {
		return VerificationFrozenPlanConsent{}, err
	}
	return consent, nil
}

// ResumeFrozenVerificationPlan returns the identical frozen plan and the gate
// that remains. Consent is already spent, so it is never asked again; an
// immediate-authorization obligation deliberately survives the resume because
// it answers a different question than cost.
func ResumeFrozenVerificationPlan(
	authority RARPlanAuthority,
	consent VerificationFrozenPlanConsent,
) (VerificationPlan, VerificationGate, error) {
	gate, err := authority.Gate()
	if err != nil {
		return VerificationPlan{}, rejectedVerificationGate(), err
	}
	if consent.Schema != VerificationFrozenPlanConsentSchema {
		return VerificationPlan{}, rejectedVerificationGate(),
			errors.New("unsupported verification frozen-plan consent schema")
	}
	if consent.AuthorityRef != authority.AuthorityRef || consent.PlanDigest != authority.Plan.Digest {
		return VerificationPlan{}, rejectedVerificationGate(),
			fmt.Errorf("%w: frozen-plan consent", ErrVerificationAuthorizationStale)
	}
	if authority.Effects == nil || consent.Effects != *authority.Effects {
		return VerificationPlan{}, rejectedVerificationGate(),
			fmt.Errorf("%w: consent effect dimensions", ErrVerificationAuthorizationStale)
	}
	want, err := verificationFrozenPlanConsentDigest(consent)
	if err != nil {
		return VerificationPlan{}, rejectedVerificationGate(), err
	}
	if consent.ConsentRef != want {
		return VerificationPlan{}, rejectedVerificationGate(),
			errors.New("verification frozen-plan consent ref does not match its canonical content")
	}
	if !gate.RequiresFrozenPlanConsent {
		return VerificationPlan{}, rejectedVerificationGate(),
			errors.New("verification plan does not require frozen-plan consent")
	}
	resumed := VerificationGate{
		Decision:                       VerificationDecisionAutoRun,
		RequiresImmediateAuthorization: gate.RequiresImmediateAuthorization,
	}
	if resumed.RequiresImmediateAuthorization {
		resumed.Decision = VerificationDecisionImmediateAuthorization
	}
	return authority.Plan, resumed, nil
}

// VerificationEffectAuthorization admits one destructive or
// permission-sensitive obligation immediately before its effect. It binds the
// exact frozen plan, so replaying it against any other plan is rejected.
type VerificationEffectAuthorization struct {
	Schema           string                    `json:"schema"`
	AuthorityRef     string                    `json:"authority_ref"`
	PlanDigest       string                    `json:"plan_digest"`
	ObligationID     string                    `json:"obligation_id"`
	Effects          VerificationEffectProfile `json:"effects"`
	AuthorizationRef string                    `json:"authorization_ref"`
}

// NewVerificationEffectAuthorization issues authority only for a planned
// obligation of a plan whose gate demands immediate authorization. An ordinary
// read-only plan cannot obtain one, so the record itself proves the effect.
func NewVerificationEffectAuthorization(
	authority RARPlanAuthority,
	obligationID string,
) (VerificationEffectAuthorization, error) {
	gate, err := authority.Gate()
	if err != nil {
		return VerificationEffectAuthorization{}, err
	}
	if !gate.RequiresImmediateAuthorization {
		return VerificationEffectAuthorization{}, errors.New("verification plan does not require immediate authorization")
	}
	if !containsSorted(verificationObligationIDs(authority.Plan.Obligations), obligationID) {
		return VerificationEffectAuthorization{}, fmt.Errorf(
			"verification obligation %q is not part of the frozen plan",
			obligationID,
		)
	}
	authorization := VerificationEffectAuthorization{
		Schema:       VerificationEffectAuthorizationSchema,
		AuthorityRef: authority.AuthorityRef,
		PlanDigest:   authority.Plan.Digest,
		ObligationID: obligationID,
		Effects:      *authority.Effects,
	}
	if authorization.AuthorizationRef, err = verificationEffectAuthorizationDigest(authorization); err != nil {
		return VerificationEffectAuthorization{}, err
	}
	return authorization, nil
}

// ValidateVerificationEffectAuthorization rejects stale and cross-bound
// authority before any effect can start. Because the authority ref is content
// addressed, a superseded plan produces a different ref and the earlier
// authorization can never be reused against it.
func ValidateVerificationEffectAuthorization(
	authority RARPlanAuthority,
	authorization VerificationEffectAuthorization,
) error {
	gate, err := authority.Gate()
	if err != nil {
		return err
	}
	if !gate.RequiresImmediateAuthorization {
		return errors.New("verification plan does not require immediate authorization")
	}
	if authorization.Schema != VerificationEffectAuthorizationSchema {
		return errors.New("unsupported verification effect authorization schema")
	}
	if authorization.AuthorityRef != authority.AuthorityRef ||
		authorization.PlanDigest != authority.Plan.Digest {
		return fmt.Errorf("%w: effect authorization", ErrVerificationAuthorizationStale)
	}
	if authority.Effects == nil || authorization.Effects != *authority.Effects {
		return fmt.Errorf("%w: authorization effect dimensions", ErrVerificationAuthorizationStale)
	}
	if !containsSorted(verificationObligationIDs(authority.Plan.Obligations), authorization.ObligationID) {
		return fmt.Errorf(
			"verification obligation %q is not part of the frozen plan",
			authorization.ObligationID,
		)
	}
	want, err := verificationEffectAuthorizationDigest(authorization)
	if err != nil {
		return err
	}
	if authorization.AuthorizationRef != want {
		return errors.New("verification effect authorization ref does not match its canonical content")
	}
	return nil
}

func verificationFrozenPlanConsentDigest(consent VerificationFrozenPlanConsent) (string, error) {
	consent.ConsentRef = ""
	return verificationContractDigest(verificationFrozenPlanConsentDigestDomain, consent)
}

func verificationEffectAuthorizationDigest(authorization VerificationEffectAuthorization) (string, error) {
	authorization.AuthorizationRef = ""
	return verificationContractDigest(verificationEffectAuthorizationDomain, authorization)
}

// PublishPlan validates the exact live snapshot, constructs its owner
// content-addressed authority, and durably publishes it in the repository's
// private Git-common-dir RAR store. Exact replay is idempotent.
func (repository *RARAuthorityRepository) PublishPlan(
	ctx context.Context,
	request RARPlanPublication,
) (RARPlanAuthority, error) {
	if err := ctx.Err(); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	subject, err := VerificationSubjectFromSnapshot(request.Snapshot)
	if err != nil {
		return RARPlanAuthority{}, err
	}
	if request.Applicability.Subject != subject || request.Plan.Subject != subject {
		return RARPlanAuthority{}, errors.New("RAR plan publication does not bind the exact snapshot subject")
	}
	if err := ValidateVerificationPlan(
		request.Applicability,
		request.Registry,
		request.Plan,
	); err != nil {
		return RARPlanAuthority{}, fmt.Errorf("validate RAR plan publication: %w", err)
	}
	if err := validatePlanEffectBinding(request.Plan, request.Effects); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateLivePlanSnapshot(ctx, request.Snapshot); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateNativePlanContracts(
		ctx,
		request.Snapshot,
		request.Applicability,
		request.Registry,
		request.Plan,
	); err != nil {
		return RARPlanAuthority{}, err
	}

	authority := RARPlanAuthority{
		Schema:             RARPlanAuthoritySchema,
		RepositoryIdentity: repository.identity.RepositoryIdentity,
		Snapshot:           request.Snapshot,
		Subject:            subject,
		Applicability:      request.Applicability,
		Registry:           request.Registry,
		Plan:               request.Plan,
		Effects:            request.Effects,
	}
	authority.AuthorityRef, err = rarPlanAuthorityDigest(authority)
	if err != nil {
		return RARPlanAuthority{}, err
	}
	if err := authority.Validate(); err != nil {
		return RARPlanAuthority{}, err
	}
	payload, err := canonicalRARPlanAuthorityPayload(authority)
	if err != nil {
		return RARPlanAuthority{}, err
	}

	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := ensureRARRepositoryRoot(repository.identity.GitCommonDir, repository.root, true); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := ensurePrivateRARDirectoryTree(repository.root, repository.planObjectsRoot(), true); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	lock, err := acquireRARAuthorityLock(ctx, filepath.Join(repository.root, "LOCK"))
	if err != nil {
		// One lock-exhaustion shape is not a failure: equivalent publishers
		// serialized behind the exclusive RAR lock, the bounded wait elapsed,
		// and the winner already published this exact content-addressed
		// authority. The plan object is immutable and published by atomic
		// no-replace rename, so a byte-identical read-back proves this
		// caller's publication already happened; it then passes the same
		// identity-and-liveness gate the winner passes after publishing, and
		// returns the identical authority instead of a timeout for work that
		// succeeded (1872).
		//
		// Everything else keeps the typed error: cancellation, a missing
		// object, different bytes at the address, a replaced repository
		// identity, or a snapshot that is no longer live all mean there is
		// nothing proven to converge on.
		if errors.Is(err, ErrAuthorityLockTimeout) {
			existing, readErr := readPrivateRARFile(repository.planObjectPath(authority.AuthorityRef))
			if readErr == nil && bytes.Equal(existing, payload) &&
				repository.validateIdentity(ctx) == nil &&
				repository.validateLivePlanSnapshot(ctx, request.Snapshot) == nil &&
				repository.validateIdentity(ctx) == nil {
				return authority, nil
			}
		}
		return RARPlanAuthority{}, err
	}
	defer lock.release()
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateLivePlanSnapshot(ctx, request.Snapshot); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateNativePlanContracts(
		ctx,
		request.Snapshot,
		request.Applicability,
		request.Registry,
		request.Plan,
	); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := publishPrivateRARImmutable(repository.planObjectPath(authority.AuthorityRef), payload); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	// A concurrent workspace mutation cannot turn a stale publication into a
	// successful issuance. The immutable orphan remains harmless and retry-safe.
	if err := repository.validateLivePlanSnapshot(ctx, request.Snapshot); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	return authority, nil
}

// ResolvePlan resolves only the owner-issued AuthorityRef (PlanRevisionRef),
// then revalidates repository identity and the stored exact live snapshot. It
// accepts no caller result, outcome, policy projection, or candidate head.
func (repository *RARAuthorityRepository) ResolvePlan(
	ctx context.Context,
	authorityRef string,
) (RARPlanAuthority, error) {
	if err := ctx.Err(); err != nil {
		return RARPlanAuthority{}, err
	}
	if !validSHA256(authorityRef) {
		return RARPlanAuthority{}, errors.New("invalid RAR plan authority ref")
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := ensureRARRepositoryRoot(repository.identity.GitCommonDir, repository.root, false); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := ensurePrivateRARDirectoryTree(repository.root, repository.planObjectsRoot(), false); err != nil {
		return RARPlanAuthority{}, err
	}
	payload, err := readPrivateRARFile(repository.planObjectPath(authorityRef))
	if err != nil {
		return RARPlanAuthority{}, err
	}
	authority, err := parseRARPlanAuthority(payload)
	if err != nil {
		return RARPlanAuthority{}, fmt.Errorf("%w: %v", ErrRARAuthorityCorrupt, err)
	}
	if authority.AuthorityRef != authorityRef {
		return RARPlanAuthority{}, fmt.Errorf("%w: plan authority lookup binding mismatch", ErrRARAuthorityCorrupt)
	}
	if authority.RepositoryIdentity != repository.identity.RepositoryIdentity {
		return RARPlanAuthority{}, fmt.Errorf("%w: repository identity changed", ErrRARAuthorityStale)
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateLivePlanSnapshot(ctx, authority.Snapshot); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateNativePlanContracts(
		ctx,
		authority.Snapshot,
		authority.Applicability,
		authority.Registry,
		authority.Plan,
	); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := repository.validateIdentity(ctx); err != nil {
		return RARPlanAuthority{}, err
	}
	return authority, nil
}

func (repository *RARAuthorityRepository) validateLivePlanSnapshot(
	ctx context.Context,
	snapshot Snapshot,
) error {
	if repository == nil {
		return errors.New("RAR authority repository is not initialized")
	}
	if err := (SnapshotBuilder{Repo: repository.identity.RepositoryRoot}).ValidateLiveSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("%w: exact plan snapshot is no longer live: %v", ErrRARAuthorityStale, err)
	}
	return nil
}

func (repository *RARAuthorityRepository) validateNativePlanContracts(
	ctx context.Context,
	snapshot Snapshot,
	applicability VerificationApplicability,
	registry VerificationPlanRegistry,
	plan VerificationPlan,
) error {
	if repository == nil {
		return errors.New("RAR authority repository is not initialized")
	}
	builder := SnapshotBuilder{Repo: repository.identity.RepositoryRoot}
	nativeApplicability, err := builder.ClassifyVerificationApplicability(
		ctx,
		snapshot,
		registry,
		applicability.EvidenceRefs,
	)
	if err != nil {
		return fmt.Errorf("derive native RAR plan applicability: %w", err)
	}
	if !reflect.DeepEqual(nativeApplicability, applicability) {
		return errors.New("RAR plan applicability differs from the native exact-snapshot classification")
	}
	nativePlan, err := BuildVerificationPlan(nativeApplicability, registry)
	if err != nil {
		return fmt.Errorf("derive native RAR verification plan: %w", err)
	}
	if !reflect.DeepEqual(nativePlan, plan) {
		return errors.New("RAR verification plan differs from the complete native registry projection")
	}
	return nil
}

func (repository *RARAuthorityRepository) planObjectsRoot() string {
	return filepath.Join(repository.root, "plan-objects")
}

func (repository *RARAuthorityRepository) planObjectPath(authorityRef string) string {
	return filepath.Join(repository.planObjectsRoot(), hashPathComponent(authorityRef)+".json")
}

func canonicalRARPlanAuthorityPayload(authority RARPlanAuthority) ([]byte, error) {
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(authority)
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}

func parseRARPlanAuthority(payload []byte) (RARPlanAuthority, error) {
	var authority RARPlanAuthority
	if err := decodeStrictRARJSON(payload, &authority); err != nil {
		return RARPlanAuthority{}, err
	}
	if err := authority.Validate(); err != nil {
		return RARPlanAuthority{}, err
	}
	canonical, err := canonicalRARPlanAuthorityPayload(authority)
	if err != nil || !bytes.Equal(payload, canonical) {
		return RARPlanAuthority{}, errors.New("RAR plan authority is not canonical")
	}
	return authority, nil
}

func rarPlanAuthorityDigest(authority RARPlanAuthority) (string, error) {
	authority.AuthorityRef = ""
	payload, err := json.Marshal(authority)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(rarPlanAuthorityDigestDomain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(payload)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
