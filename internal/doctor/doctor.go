// Package doctor defines presentation-independent health check results and execution.
package doctor

import "context"

// CheckID is a stable identifier for a health check.
type CheckID string

const (
	CheckStateJSON       CheckID = "state:json"
	CheckEngramReachable CheckID = "engram:reachable"
	CheckDiskSpace       CheckID = "disk:space"
)

// ToolCheckID returns the stable check identifier for a tool binary.
func ToolCheckID(tool string) CheckID { return CheckID("tool:" + tool) }

// Status is the outcome of a health check.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// RemedyID identifies a remedy that a future executor may implement.
type RemedyID string

const (
	RemedyInstallTool      RemedyID = "install-tool"
	RemedyRemoveDuplicates RemedyID = "remove-duplicate-tools"
	RemedyInstall          RemedyID = "install"
	RemedyRepairState      RemedyID = "repair-state"
	RemedySync             RemedyID = "sync"
	RemedyStartEngram      RemedyID = "start-engram"
	RemedyInspectEngram    RemedyID = "inspect-engram"
	RemedyFreeDiskSpace    RemedyID = "free-disk-space"
)

// RemedyCategory groups remedies by the resource they concern.
type RemedyCategory string

const (
	RemedyCategoryInstall       RemedyCategory = "install"
	RemedyCategoryConfiguration RemedyCategory = "configuration"
	RemedyCategoryEnvironment   RemedyCategory = "environment"
	RemedyCategoryService       RemedyCategory = "service"
	RemedyCategoryStorage       RemedyCategory = "storage"
)

// ActionMode describes the interaction required before a remedy may run.
type ActionMode string

const (
	ActionAutomatic    ActionMode = "automatic"
	ActionConfirmation ActionMode = "confirmation"
	ActionManualOnly   ActionMode = "manual-only"
)

// Platform identifies an operating environment supported by a remedy.
type Platform string

const (
	PlatformWindows Platform = "windows"
	PlatformMacOS   Platform = "macos"
	PlatformLinux   Platform = "linux"
	PlatformWSL     Platform = "wsl"
)

// Remedy describes a possible response without performing it.
type Remedy struct {
	ID                 RemedyID
	Description        string
	Category           RemedyCategory
	ActionMode         ActionMode
	Eligible           bool
	EligibilityReason  string
	SupportedPlatforms []Platform
}

// NewRemedy classifies a remedy without making it executable.
func NewRemedy(id RemedyID, description string) *Remedy {
	r := &Remedy{ID: id, Description: description, ActionMode: ActionManualOnly, SupportedPlatforms: supportedPlatforms()}
	switch id {
	case RemedyInstallTool, RemedyInstall:
		r.Category, r.EligibilityReason = RemedyCategoryInstall, "no bounded managed install was identified"
	case RemedyRemoveDuplicates:
		r.Category, r.EligibilityReason = RemedyCategoryEnvironment, "binary ownership is unknown"
	case RemedyRepairState:
		r.Category, r.EligibilityReason = RemedyCategoryConfiguration, "no safe recovery source was identified"
	case RemedySync:
		r.Category, r.ActionMode, r.Eligible = RemedyCategoryConfiguration, ActionConfirmation, true
		r.EligibilityReason = "requires managed state and existing sync safeguards"
	case RemedyStartEngram, RemedyInspectEngram:
		r.Category, r.EligibilityReason = RemedyCategoryService, "external process and log ownership is unknown"
	case RemedyFreeDiskSpace:
		r.Category, r.EligibilityReason = RemedyCategoryStorage, "disk cleanup is user-owned"
	default:
		r.EligibilityReason = "remedy is not allowlisted"
	}
	return r
}

func supportedPlatforms() []Platform {
	return []Platform{PlatformWindows, PlatformMacOS, PlatformLinux, PlatformWSL}
}

// Result contains evidence produced by one check.
type Result struct {
	Name   CheckID
	Status Status
	Detail string
	Remedy *Remedy
}

// Report preserves checks in execution order.
type Report struct {
	Checks []Result
}

// Check is a read-only diagnostic operation.
type Check struct {
	ID  CheckID
	Run func(context.Context) Result
}

// Runner executes checks deterministically in declaration order.
type Runner struct {
	Checks []Check
}

// Run executes every check in declaration order and isolates panics per check.
func (r Runner) Run(ctx context.Context) Report {
	report := Report{Checks: make([]Result, 0, len(r.Checks))}
	for _, check := range r.Checks {
		result := runCheck(ctx, check)
		result.Name = check.ID
		report.Checks = append(report.Checks, result)
	}
	return report
}

func runCheck(ctx context.Context, check Check) (result Result) {
	defer func() {
		if recover() != nil {
			result = Result{Status: StatusFail, Detail: "check failed unexpectedly"}
		}
	}()
	return check.Run(ctx)
}
