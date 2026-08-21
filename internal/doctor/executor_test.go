package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecutorConstructionAndZeroValues(t *testing.T) {
	valid := validSyncOperation()
	var typedNil *fakeOperation
	tests := []struct {
		name       string
		detector   func() Platform
		operations []managedOperation
		want       string
	}{
		{"nil detector", nil, nil, "invalid-operation"},
		{"nil operation", linuxDetector, []managedOperation{nil}, "invalid-operation"},
		{"typed nil operation", linuxDetector, []managedOperation{typedNil}, "invalid-operation"},
		{"operation ID panic", linuxDetector, []managedOperation{&fakeOperation{panicID: true}}, "invalid-operation"},
		{"duplicate operation ID", linuxDetector, []managedOperation{valid, valid}, "duplicate-operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newExecutorWithPlatform(tt.detector, tt.operations...)
			wantPolicyReason(t, err, tt.want)
		})
	}
	var zero executor
	_, err := zero.preflight(context.Background(), RemedySync)
	wantPolicyReason(t, err, "invalid-operation")
}

func TestPreflightCanonicalPolicyPrecedesPlatformDetection(t *testing.T) {
	tests := []struct {
		name      string
		id        RemedyID
		detector  func() Platform
		operation managedOperation
		want      string
	}{
		{"unknown on supported platform", RemedyID("unknown"), linuxDetector, validSyncOperation(), "unknown-remedy"},
		{"unknown on unsupported platform", RemedyID("unknown"), unsupportedDetector, validSyncOperation(), "unknown-remedy"},
		{"unknown on panicking detector", RemedyID("unknown"), panickingDetector, validSyncOperation(), "unknown-remedy"},
		{"ineligible on supported platform", RemedyInstall, linuxDetector, ineligibleOperation(), "ineligible"},
		{"ineligible on unsupported platform", RemedyInstall, unsupportedDetector, ineligibleOperation(), "ineligible"},
		{"ineligible on panicking detector", RemedyInstall, panickingDetector, ineligibleOperation(), "ineligible"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := newExecutorWithPlatform(tt.detector, tt.operation)
			if err != nil {
				t.Fatalf("constructor error = %v", err)
			}
			_, err = executor.preflight(context.Background(), tt.id)
			wantPolicyReason(t, err, tt.want)
		})
	}
}

func TestPreflightReturnsCanonicalSyncClassification(t *testing.T) {
	executor := newTestExecutor(t, validSyncOperation())
	result, err := executor.preflight(context.Background(), RemedySync)
	if err != nil {
		t.Fatalf("preflight() error = %v", err)
	}
	if result.executor != executor || result.id != RemedySync || result.category != RemedyCategoryConfiguration || result.actionMode != ActionConfirmation || result.platform != PlatformLinux {
		t.Fatalf("preflight() = %#v, want executor-owned canonical sync classification", result)
	}
}

func TestPreflightValidatesEvidenceAndPlatform(t *testing.T) {
	tests := []struct {
		name     string
		change   func(*preflightEvidence)
		detector func() Platform
		want     string
	}{
		{"ID mismatch", func(e *preflightEvidence) { e.id = RemedyInstall }, linuxDetector, "id-mismatch"},
		{"invalid category", func(e *preflightEvidence) { e.category = RemedyCategoryService }, linuxDetector, "invalid-category"},
		{"automatic action", func(e *preflightEvidence) { e.actionMode = ActionAutomatic }, linuxDetector, "manual-or-non-managed-action"},
		{"manual action", func(e *preflightEvidence) { e.actionMode = ActionManualOnly }, linuxDetector, "manual-or-non-managed-action"},
		{"unsupported evidence platform", func(e *preflightEvidence) { e.platform = PlatformWindows }, linuxDetector, "unsupported-platform"},
		{"ineligible evidence", func(e *preflightEvidence) { e.eligible = false }, linuxDetector, "ineligible"},
		{"missing prerequisites", func(e *preflightEvidence) { e.prerequisitesReady = false }, linuxDetector, "missing-prerequisites"},
		{"unverified ownership", func(e *preflightEvidence) { e.ownershipVerified = false }, linuxDetector, "ownership-unverified"},
		{"unverified containment", func(e *preflightEvidence) { e.containmentVerified = false }, linuxDetector, "containment-unverified"},
		{"unsupported platform", nil, unsupportedDetector, "unsupported-platform"},
		{"detector panic", nil, panickingDetector, "unsupported-platform"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := validEvidence(RemedySync, ActionConfirmation)
			if tt.change != nil {
				tt.change(&evidence)
			}
			executor, err := newExecutorWithPlatform(tt.detector, &fakeOperation{operationID: RemedySync, evidence: evidence})
			if err != nil {
				t.Fatalf("constructor error = %v", err)
			}
			_, err = executor.preflight(context.Background(), RemedySync)
			wantPolicyReason(t, err, tt.want)
		})
	}
}

func TestPreflightFailsClosedForContextAndOperation(t *testing.T) {
	secret := errors.New("private preflight details")
	duringContext, cancel := context.WithCancel(context.Background())
	duringOperation := validSyncOperation()
	duringOperation.cancel = cancel
	tests := []struct {
		name      string
		ctx       context.Context
		operation *fakeOperation
		want      string
	}{
		{"nil context", nil, validSyncOperation(), "context-canceled"},
		{"canceled before preflight", canceledContext(), validSyncOperation(), "context-canceled"},
		{"preflight error", context.Background(), &fakeOperation{operationID: RemedySync, evidence: validEvidence(RemedySync, ActionConfirmation), err: secret}, "preflight-failed"},
		{"preflight panic", context.Background(), &fakeOperation{operationID: RemedySync, evidence: validEvidence(RemedySync, ActionConfirmation), panicPreflight: true}, "preflight-failed"},
		{"canceled during preflight", duringContext, duringOperation, "context-canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := newTestExecutor(t, tt.operation)
			_, err := executor.preflight(tt.ctx, RemedySync)
			wantPolicyReason(t, err, tt.want)
			if tt.name == "preflight error" && (errors.Unwrap(err).Error() != "preflight-failed" || errors.Is(err, secret) || strings.Contains(err.Error(), secret.Error())) {
				t.Fatalf("policy error exposed operation cause: %v", err)
			}
		})
	}
}

func TestPreflightRechecksOperationIdentity(t *testing.T) {
	op := validSyncOperation()
	executor := newTestExecutor(t, op)
	op.changeDuringPreflight = true
	_, err := executor.preflight(context.Background(), RemedySync)
	wantPolicyReason(t, err, "id-mismatch")

	op = validSyncOperation()
	executor = newTestExecutor(t, op)
	op.panicID = true
	_, err = executor.preflight(context.Background(), RemedySync)
	wantPolicyReason(t, err, "id-mismatch")
}

type fakeOperation struct {
	operationID                                    RemedyID
	evidence                                       preflightEvidence
	err                                            error
	cancel                                         context.CancelFunc
	panicID, panicPreflight, changeDuringPreflight bool
}

func (f *fakeOperation) id() RemedyID {
	if f.panicID {
		panic("private operation ID")
	}
	return f.operationID
}
func (f *fakeOperation) preflight(_ context.Context, _ Platform) (preflightEvidence, error) {
	if f.cancel != nil {
		f.cancel()
	}
	if f.changeDuringPreflight {
		f.operationID = RemedyInstall
	}
	if f.panicPreflight {
		panic("private preflight")
	}
	return f.evidence, f.err
}

func validEvidence(id RemedyID, mode ActionMode) preflightEvidence {
	return preflightEvidence{id: id, category: RemedyCategoryConfiguration, actionMode: mode, platform: PlatformLinux, eligible: true, prerequisitesReady: true, ownershipVerified: true, containmentVerified: true}
}
func validSyncOperation() *fakeOperation {
	return &fakeOperation{operationID: RemedySync, evidence: validEvidence(RemedySync, ActionConfirmation)}
}
func ineligibleOperation() *fakeOperation { return &fakeOperation{operationID: RemedyInstall} }
func linuxDetector() Platform             { return PlatformLinux }
func unsupportedDetector() Platform       { return Platform("plan9") }
func panickingDetector() Platform         { panic("private detector") }
func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
func newTestExecutor(t *testing.T, operation managedOperation) *executor {
	t.Helper()
	executor, err := newExecutorWithPlatform(linuxDetector, operation)
	if err != nil {
		t.Fatalf("constructor error = %v", err)
	}
	return executor
}
func wantPolicyReason(t *testing.T, err error, want string) {
	t.Helper()
	var policy *policyError
	if err == nil || !errors.As(err, &policy) || policy.reason() != want {
		t.Fatalf("error = %v, want policyError reason %q", err, want)
	}
}
