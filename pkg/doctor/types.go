package doctor

import (
	"context"
	"io"
	"time"

	"github.com/gentleman-programming/gentle-ai/pkg/doctor/fixer"
)

// Status represents the outcome of a health check.
type Status string

const (
	// StatusPass indica que la verificación pasó exitosamente.
	StatusPass Status = "PASS"
	// StatusWarn indica que la verificación pasó con advertencias.
	StatusWarn Status = "WARN"
	// StatusFail indica que la verificación falló y requiere atención.
	StatusFail Status = "FAIL"
	// StatusInfo indica información informativa sin impacto en salud.
	StatusInfo Status = "INFO"
	// StatusSkip indica que la verificación fue omitida.
	StatusSkip Status = "SKIP"
)

// Category groups related checks.
type Category string

const (
	// CategoryHardware agrupa verificaciones de hardware del sistema.
	CategoryHardware Category = "hardware"
	// CategorySoftware agrupa verificaciones de software y dependencias.
	CategorySoftware Category = "software"
	// CategoryConfig agrupa verificaciones de configuración del agente.
	CategoryConfig Category = "config"
)

// CheckResult is the outcome of a single health check.
type CheckResult struct {
	Name        string                 // Unique identifier: "hw/cpu/cores"
	Category    Category               // hw, sw, cfg
	Status      Status                 // PASS/WARN/FAIL/INFO/SKIP
	Summary     string                 // One-line human summary
	Detail      string                 // Optional detailed output
	Remediation *Remediation           // Optional fix hint (populated in FixMode)
	Duration    time.Duration          // Check execution time
	Metadata    map[string]string      // Free-form key/value for JSON
}

// Remediation provides OS-aware fix suggestions.
type Remediation struct {
	Description string            // Human-readable explanation
	Commands    map[string]string // GOOS -> shell command
	ManualSteps []string          // Fallback if no command
	Links       []string          // Documentation URLs
}

// DoctorReport aggregates all check results.
type DoctorReport struct {
	GeneratedAt time.Time
	Version     string
	GOOS        string
	GOARCH      string
	Results     []CheckResult
	Summary     Summary
	FixRegistry *fixer.FixerRegistry // For renderers to access fix suggestions
}

// Summary provides counts for exit code and rendering.
type Summary struct {
	Total     int
	Pass      int
	Warn      int
	Fail      int
	Info      int
	Skip      int
	Duration  time.Duration
}

// DoctorFlags holds CLI flags for the doctor command.
type DoctorFlags struct {
	JSONOutput  bool
	FixMode     bool
	Categories  []string
	Verbose     bool
	ConfigPaths []string
}

// Checker is the interface all check implementations must satisfy.
type Checker interface {
	Name() string
	Category() Category
	Run(ctx context.Context) []CheckResult
}

// Renderer formats the report for output.
type Renderer interface {
	Render(report DoctorReport) error
}

// RenderOptions configures renderer behavior.
type RenderOptions struct {
	Format      string // "text", "json", "tui"
	Verbose     bool
	Color       bool
	ShowPassed  bool
	ShowSkipped bool
	FixMode     bool
	Writer      io.Writer
}