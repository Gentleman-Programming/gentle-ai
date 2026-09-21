package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gentleman-programming/gentle-ai/v3/internal/kickoff"
	"github.com/gentleman-programming/gentle-ai/v3/internal/workspace"
)

// RunSDDKickoff is the CLI entry point for `axiom sdd kickoff <seal|show>`
// (design.md S5.7). It is a thin I/O and formatting adapter: every decision
// (retro-seal inference, single-write sealing, archived-root refusal, role
// file naming) lives in internal/kickoff (task 9.4).
func RunSDDKickoff(args []string, stdout io.Writer) error {
	if hasGovernanceHelpFlag(args) {
		return renderSDDKickoffHelp(stdout)
	}
	if len(args) == 0 {
		return fmt.Errorf("kickoff requiere un subcomando: seal o show; ejecuta `axiom sdd kickoff --help`")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "seal":
		return runSDDKickoffSeal(rest, stdout)
	case "show":
		return runSDDKickoffShow(rest, stdout)
	default:
		return fmt.Errorf("subcomando %q no reconocido para kickoff; opciones: seal, show", sub)
	}
}

func renderSDDKickoffHelp(stdout io.Writer) error {
	_, _ = fmt.Fprintln(stdout, "Uso: axiom sdd kickoff <seal|show> [argumentos]")
	_, _ = fmt.Fprintln(stdout, "  seal --cwd <ruta> --change <nombre> (--execution-style continuous|checkpointed | --from-session-pace interactive|auto) --handoff-policy none|per_checkpoint [--role <id>[:blocking|deferred|optional] ...] [--deployment-target local|staging|production] [--json]")
	_, _ = fmt.Fprintln(stdout, "  seal --infer --cwd <ruta> --change <nombre> [--json]   (retro-sellado conservador, REQ-21.4)")
	_, _ = fmt.Fprintln(stdout, "  show --cwd <ruta> --change <nombre> [--json]")
	return nil
}

func runSDDKickoffSeal(args []string, stdout io.Writer) error {
	parsed, err := kickoff.ParseSealArgs(args)
	if err != nil {
		return err
	}
	workspaceRoot, changeRoot, err := resolveGovernanceChangeRoot(parsed.CWD, parsed.Change)
	if err != nil {
		return err
	}

	var sealing kickoff.Kickoff
	if parsed.Infer {
		designPath := filepath.Join(changeRoot, "design.md")
		wsConfig := loadWorkspaceConfigIfPresent(workspaceRoot)
		inferred, note, inferErr := kickoff.InferKickoff(designPath, wsConfig)
		if inferErr != nil {
			if note != "" {
				return fmt.Errorf("%s: %w", note, inferErr)
			}
			return inferErr
		}
		sealing = inferred
	} else {
		sealing = parsed.ToKickoff(parsed.Change)
	}
	sealing.Change = parsed.Change

	sealed, wrote, err := kickoff.Seal(changeRoot, sealing)
	if err != nil {
		return err
	}

	return renderKickoffSealResult(stdout, parsed.JSON, sealed, wrote)
}

func runSDDKickoffShow(args []string, stdout io.Writer) error {
	parsed, err := kickoff.ParseShowArgs(args)
	if err != nil {
		return err
	}
	_, changeRoot, err := resolveGovernanceChangeRoot(parsed.CWD, parsed.Change)
	if err != nil {
		return err
	}

	sealed, err := kickoff.Load(changeRoot)
	if err != nil {
		return err
	}

	if parsed.JSON {
		result := kickoffShowResultJSON{Sealed: sealed != nil}
		if sealed != nil {
			payload := toKickoffJSON(*sealed)
			result.Kickoff = &payload
		}
		return encodeJSON(stdout, result)
	}
	if sealed == nil {
		_, err := fmt.Fprintf(stdout, "El cambio %q no tiene kickoff sellado (sin kickoff sellado).\n", parsed.Change)
		return err
	}
	_, err = fmt.Fprintln(stdout, renderKickoffSummaryText(*sealed, false))
	return err
}

// loadWorkspaceConfigIfPresent mirrors the established
// cmd/axiom/main.go:728 idiom (runRoleList and its siblings): a missing or
// unreadable axiom.yaml is not fatal here — multirole.DetectRoles and
// InferKickoff both accept a nil *workspace.WorkspaceConfig.
func loadWorkspaceConfigIfPresent(workspaceRoot string) *workspace.WorkspaceConfig {
	cfgPath := filepath.Join(workspaceRoot, "axiom.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		return nil
	}
	cfg, _ := workspace.LoadConfig(cfgPath)
	return cfg
}

func renderKickoffSealResult(stdout io.Writer, asJSON bool, sealed kickoff.Kickoff, wrote bool) error {
	if asJSON {
		return encodeJSON(stdout, kickoffSealResultJSON{Wrote: wrote, Kickoff: toKickoffJSON(sealed)})
	}
	_, err := fmt.Fprintln(stdout, renderKickoffSummaryText(sealed, wrote))
	return err
}

func renderKickoffSummaryText(k kickoff.Kickoff, wrote bool) string {
	prefix := "Kickoff ya estaba sellado"
	if wrote {
		prefix = "Kickoff sellado"
	}
	roles := make([]string, 0, len(k.Config.Roles))
	for _, r := range k.Config.Roles {
		roles = append(roles, fmt.Sprintf("%s:%s", r.Role, r.GatePolicy))
	}
	return fmt.Sprintf("%s para %q: flow_mode=%s execution_style=%s handoff_policy=%s roles=%v deployment_target=%s sealed_by=%s",
		prefix, k.Change, k.Config.FlowMode, k.Config.ExecutionStyle, k.Config.HandoffPolicy, roles, k.Lifecycle.DeploymentTarget, k.SealedBy)
}

func encodeJSON(stdout io.Writer, value any) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// kickoffJSON, kickoffRoleJSON, kickoffSealResultJSON, and
// kickoffShowResultJSON are this verb's own --json shape: kickoff.Kickoff
// carries yaml tags, not json ones, so it is translated explicitly rather
// than encoded as-is (which would leak Go field-name casing into the CLI's
// JSON contract).
type kickoffRoleJSON struct {
	Role       string `json:"role"`
	GatePolicy string `json:"gatePolicy"`
	TasksFile  string `json:"tasksFile"`
	VerifyFile string `json:"verifyFile"`
}

type kickoffJSON struct {
	Schema           string            `json:"schema"`
	Change           string            `json:"change"`
	SealedAt         string            `json:"sealedAt"`
	SealedBy         string            `json:"sealedBy"`
	FlowMode         string            `json:"flowMode"`
	ExecutionStyle   string            `json:"executionStyle"`
	HandoffPolicy    string            `json:"handoffPolicy"`
	Roles            []kickoffRoleJSON `json:"roles"`
	DeploymentTarget string            `json:"deploymentTarget"`
}

func toKickoffJSON(k kickoff.Kickoff) kickoffJSON {
	roles := make([]kickoffRoleJSON, 0, len(k.Config.Roles))
	for _, r := range k.Config.Roles {
		roles = append(roles, kickoffRoleJSON{
			Role:       r.Role,
			GatePolicy: string(r.GatePolicy),
			TasksFile:  r.TasksFile,
			VerifyFile: r.VerifyFile,
		})
	}
	return kickoffJSON{
		Schema:           k.Schema,
		Change:           k.Change,
		SealedAt:         k.SealedAt.Format("2006-01-02T15:04:05Z07:00"),
		SealedBy:         k.SealedBy,
		FlowMode:         string(k.Config.FlowMode),
		ExecutionStyle:   string(k.Config.ExecutionStyle),
		HandoffPolicy:    string(k.Config.HandoffPolicy),
		Roles:            roles,
		DeploymentTarget: k.Lifecycle.DeploymentTarget,
	}
}

type kickoffSealResultJSON struct {
	Wrote   bool        `json:"wrote"`
	Kickoff kickoffJSON `json:"kickoff"`
}

type kickoffShowResultJSON struct {
	Sealed  bool         `json:"sealed"`
	Kickoff *kickoffJSON `json:"kickoff,omitempty"`
}
