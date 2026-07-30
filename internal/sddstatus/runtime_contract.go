package sddstatus

import "regexp"

const (
	RuntimeRequestIDPattern    = `^[a-z0-9][a-z0-9._-]{0,127}$`
	RuntimeRevisionPattern     = `^sha256:[a-f0-9]{64}$`
	RuntimeChangePattern       = `^[a-z0-9]+(?:-[a-z0-9]+)*$`
	RuntimeLineagePattern      = `^[a-z0-9]+(?:-[a-z0-9]+)*$`
	RuntimeDefaultAttemptLimit = 2
	RuntimeMaxAttemptLimit     = 100
	RuntimeDefaultChangedLines = 200
	RuntimeMaxChangedLines     = 1_000_000
	// Deprecated compatibility aliases retained for existing callers.
	DefaultRuntimeAttemptLimit  = RuntimeDefaultAttemptLimit
	DefaultRuntimeChangedLines  = RuntimeDefaultChangedLines
	RuntimeChangeLimit          = 96
	RuntimeLineageLimit         = 128
	RuntimeWorkUnitLimit        = 160
	RuntimeEvidenceGoalLimit    = 240
	RuntimeDiagnosisLimit       = 500
	RuntimeCleanupEvidenceLimit = 500
	RuntimeProcessEvidenceLimit = 500
	RuntimeResetReasonLimit     = 500
	RuntimeActorLimit           = 128
	MaxVerifyReportBytes        = 1 << 20
	RuntimeOutcomeRunning       = "running"
	RuntimeOutcomeFailed        = "failed"
	RuntimeOutcomeInterrupted   = "interrupted"
	RuntimeOutcomePassed        = "passed"

	RuntimeDispositionReused      = "reused"
	RuntimeDispositionInvalidated = "invalidated"

	VerifyVerdictPass             = "pass"
	VerifyVerdictPassWithWarnings = "pass_with_warnings"
	VerifyVerdictFail             = "fail"
)

var (
	runtimeRequestIDPattern = regexp.MustCompile(RuntimeRequestIDPattern)
	runtimeRevisionPattern  = regexp.MustCompile(RuntimeRevisionPattern)
	runtimeChangePattern    = regexp.MustCompile(RuntimeChangePattern)
	runtimeLineagePattern   = regexp.MustCompile(RuntimeLineagePattern)
	runtimeHashPattern      = regexp.MustCompile(RuntimeRevisionPattern)
)

func RuntimeTerminalOutcomes() []string {
	return []string{RuntimeOutcomeFailed, RuntimeOutcomeInterrupted, RuntimeOutcomePassed}
}
func RuntimeHarnessDispositions() []string {
	return []string{RuntimeDispositionReused, RuntimeDispositionInvalidated}
}

func VerifyVerdicts() []string {
	return []string{VerifyVerdictPass, VerifyVerdictPassWithWarnings, VerifyVerdictFail}
}

func VerifyReportEnvelopeFields() []string {
	return []string{"schema", "evidence_revision", "verdict", "blockers", "critical_findings", "requirements", "scenarios", "test_command", "test_exit_code", "test_output_hash", "build_command", "build_exit_code", "build_output_hash"}
}
