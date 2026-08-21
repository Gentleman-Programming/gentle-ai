package doctor

import (
	"context"
	"reflect"
)

// executor validates package-owned remedy evidence before any future execution.
type executor struct {
	registry         *registry
	platformDetector func() Platform
}

// preflightResult is an executor-bound, read-only preflight decision.
type preflightResult struct {
	executor   *executor
	id         RemedyID
	category   RemedyCategory
	actionMode ActionMode
	platform   Platform
}
type managedOperation interface {
	id() RemedyID
	preflight(context.Context, Platform) (preflightEvidence, error)
}
type preflightEvidence struct {
	id                  RemedyID
	category            RemedyCategory
	actionMode          ActionMode
	platform            Platform
	eligible            bool
	prerequisitesReady  bool
	ownershipVerified   bool
	containmentVerified bool
}
type registry struct{ operations map[RemedyID]managedOperation }

// policyError exposes only a stable, sanitized remedy-policy denial.
type policyError struct{ code policyReason }
type policyReason string

func (r policyReason) Error() string  { return string(r) }
func (e *policyError) Error() string  { return "remedy policy denied: " + string(e.code) }
func (e *policyError) Unwrap() error  { return e.code }
func (e *policyError) reason() string { return string(e.code) }
func newExecutorWithPlatform(detector func() Platform, operations ...managedOperation) (*executor, error) {
	if detector == nil {
		return nil, newPolicyError("invalid-operation")
	}
	registered, err := newRegistry(operations...)
	if err != nil {
		return nil, err
	}
	return &executor{registry: &registered, platformDetector: detector}, nil
}
func newRegistry(operations ...managedOperation) (registry, error) {
	registered := make(map[RemedyID]managedOperation, len(operations))
	for _, operation := range operations {
		id := operationID(operation)
		if id == "" {
			return registry{}, newPolicyError("invalid-operation")
		}
		if _, exists := registered[id]; exists {
			return registry{}, newPolicyError("duplicate-operation")
		}
		registered[id] = operation
	}
	return registry{operations: registered}, nil
}
func operationID(operation managedOperation) (id RemedyID) {
	if operation == nil {
		return ""
	}
	value := reflect.ValueOf(operation)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		if value.IsNil() {
			return ""
		}
	}
	defer func() {
		if recover() != nil {
			id = ""
		}
	}()
	return operation.id()
}

// preflight collects package-owned read-only evidence and validates it.
func (e *executor) preflight(ctx context.Context, id RemedyID) (preflightResult, error) {
	if e == nil || e.registry == nil {
		return preflightDenied("invalid-operation")
	}
	if contextState(ctx) != nil {
		return preflightDenied("context-canceled")
	}
	operation, ok := e.registry.operations[id]
	if !ok {
		return preflightDenied("unknown-remedy")
	}
	if operationID(operation) != id {
		return preflightDenied("id-mismatch")
	}
	remedy := NewRemedy(id, "")
	switch {
	case remedy.Category == "":
		return preflightDenied("unknown-remedy")
	case !remedy.Eligible:
		return preflightDenied("ineligible")
	case remedy.ActionMode != ActionConfirmation:
		return preflightDenied("manual-or-non-managed-action")
	}
	platform := e.detectPlatform()
	if !supportsPlatform(remedy, platform) {
		return preflightDenied("unsupported-platform")
	}
	evidence, err := runPreflight(operation, ctx, platform)
	if contextState(ctx) != nil {
		return preflightDenied("context-canceled")
	}
	if err != nil {
		return preflightDenied("preflight-failed")
	}
	if operationID(operation) != id {
		return preflightDenied("id-mismatch")
	}
	if err := validateEvidence(id, platform, evidence); err != nil {
		return preflightResult{}, err
	}
	return preflightResult{executor: e, id: id, category: evidence.category, actionMode: evidence.actionMode, platform: platform}, nil
}
func preflightDenied(reason string) (preflightResult, error) {
	return preflightResult{}, newPolicyError(reason)
}
func runPreflight(operation managedOperation, ctx context.Context, platform Platform) (e preflightEvidence, err error) {
	defer func() {
		if recover() != nil {
			err = newPolicyError("preflight-failed")
		}
	}()
	return operation.preflight(ctx, platform)
}
func validateEvidence(id RemedyID, platform Platform, evidence preflightEvidence) error {
	remedy := NewRemedy(id, "")
	checks := []struct {
		invalid bool
		reason  string
	}{
		{evidence.id != id, "id-mismatch"},
		{remedy.Category == "", "unknown-remedy"},
		{evidence.category != remedy.Category, "invalid-category"},
		{!remedy.Eligible || evidence.eligible != remedy.Eligible, "ineligible"},
		{evidence.actionMode != remedy.ActionMode || remedy.ActionMode != ActionConfirmation, "manual-or-non-managed-action"},
		{evidence.platform != platform || !supportsPlatform(remedy, platform), "unsupported-platform"},
		{!evidence.prerequisitesReady, "missing-prerequisites"},
		{!evidence.ownershipVerified, "ownership-unverified"},
		{!evidence.containmentVerified, "containment-unverified"},
	}
	for _, check := range checks {
		if check.invalid {
			return newPolicyError(check.reason)
		}
	}
	return nil
}
func supportsPlatform(remedy *Remedy, platform Platform) bool {
	for _, supported := range remedy.SupportedPlatforms {
		if supported == platform {
			return true
		}
	}
	return false
}
func contextState(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	return ctx.Err()
}
func (e *executor) detectPlatform() (platform Platform) {
	defer func() {
		if recover() != nil {
			platform = ""
		}
	}()
	if e.platformDetector != nil {
		platform = e.platformDetector()
	}
	return platform
}
func newPolicyError(reason string) *policyError { return &policyError{code: policyReason(reason)} }
