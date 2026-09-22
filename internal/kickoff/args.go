package kickoff

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v3/internal/multirole"
)

// maxChangeNameLength bounds a --change value to a plausible single path
// component. 300 characters, the adversarial vector task 8.1 names
// explicitly, sits comfortably above this bound.
const maxChangeNameLength = 255

// reservedWindowsNames is the closed set of device names Windows refuses as
// a file or directory name, regardless of extension (T-7).
var reservedWindowsNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// validateChangeName rejects a --change value that could escape
// openspec/changes/ once joined into a path, before any path is
// constructed (T-2, T-7). It is pure string validation over the raw flag
// value: it never touches the filesystem, and it is the single containment
// check ParseSealArgs, ParseGateRecordArgs, and ParseShowArgs all share, so
// a future flag never gets a second, driftable copy of the same rule
// (task 8.3).
func validateChangeName(name string) error {
	if name == "" {
		return fmt.Errorf("--change no puede estar vacio")
	}
	if len(name) > maxChangeNameLength {
		return fmt.Errorf("--change supera la longitud maxima de %d caracteres", maxChangeNameLength)
	}
	if strings.ContainsAny(name, "/\\:") {
		return fmt.Errorf("--change no puede contener separadores de ruta ni ':': %q", name)
	}
	if name == ".." || name == "." {
		return fmt.Errorf("--change no puede ser un segmento de navegacion: %q", name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("--change no puede ser una ruta absoluta: %q", name)
	}
	base := name
	if idx := strings.Index(name, "."); idx >= 0 {
		base = name[:idx]
	}
	// Windows strips trailing spaces from a file name before comparing it
	// against a reserved device name, so a bare first-dot split alone lets
	// "con " (trailing space, no extension) slip past this check even though
	// Windows treats it as the same reserved name as "con" (remediation,
	// post Phase 8: an independent validator found this gap).
	base = strings.TrimRight(base, " ")
	if reservedWindowsNames[strings.ToLower(base)] {
		return fmt.Errorf("--change no puede usar el nombre reservado de Windows %q", name)
	}
	return nil
}

// requireFlagValue returns args[i+1] as the value for a flag named name,
// and the index the caller's for-loop should resume from. It mirrors
// sddstatus.ParseCommandArgs' own --cwd handling (status.go:225): a missing
// value, or one that looks like another flag, is rejected before it is
// ever assigned.
func requireFlagValue(args []string, i int, name string) (string, int, error) {
	if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
		return "", i, fmt.Errorf("%s requiere un valor", name)
	}
	return args[i+1], i + 1, nil
}

// RoleFlag is one parsed --role <id>[:blocking|deferred|optional] value.
type RoleFlag struct {
	Role       string
	GatePolicy multirole.GatePolicy
}

// parseRoleFlag splits a --role value on its first ':' into a role
// identifier and an optional gate policy, defaulting to blocking when the
// policy segment is absent (design.md S5.7: "por defecto: fullstack:blocking"
// generalises to every --role value with no explicit policy).
func parseRoleFlag(value string) (RoleFlag, error) {
	role := value
	policy := multirole.PolicyBlocking
	if idx := strings.Index(value, ":"); idx >= 0 {
		role = value[:idx]
		policyValue := multirole.GatePolicy(value[idx+1:])
		if !validGatePolicies[policyValue] {
			return RoleFlag{}, fmt.Errorf("--role %q tiene una politica de compuerta desconocida %q", value, string(policyValue))
		}
		policy = policyValue
	}
	if role == "" {
		return RoleFlag{}, fmt.Errorf("--role %q no declara un identificador de rol", value)
	}
	return RoleFlag{Role: role, GatePolicy: policy}, nil
}

// SealArgs is the parsed, validated result of `axiom sdd kickoff seal`.
// Parsing is pure: it never touches the filesystem, and it never resolves
// --change against openspec/changes/ — that resolution belongs to the CLI
// adapter, which never uses it to create a new directory (task 8.2).
type SealArgs struct {
	CWD    string
	Change string
	Infer  bool

	FlowMode FlowMode

	ExecutionStyle       ExecutionStyle
	ExecutionStyleSource string // "explicit" | "session_pace"
	// ExecutionStyleOverride is true when both --execution-style and
	// --from-session-pace were given: the explicit flag wins, and this
	// records that as a deliberate override rather than a silent one
	// (D-03).
	ExecutionStyleOverride bool

	HandoffPolicy    HandoffPolicy
	Roles            []RoleFlag
	DeploymentTarget string
	JSON             bool
}

// ParseSealArgs parses and validates `axiom sdd kickoff seal` arguments
// (design.md S5.7). With --infer, none of execution-style, handoff-policy,
// or role is required: InferKickoff derives all three from the existing
// tree (REQ-21.4, S8.1).
func ParseSealArgs(args []string) (SealArgs, error) {
	parsed := SealArgs{FlowMode: FlowSDD}
	var executionStyleExplicit, sessionPaceGiven bool
	var sessionPaceValue string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.JSON = true
		case "--infer":
			parsed.Infer = true
		case "--cwd":
			value, next, err := requireFlagValue(args, i, "--cwd")
			if err != nil {
				return SealArgs{}, err
			}
			parsed.CWD = value
			i = next
		case "--change":
			value, next, err := requireFlagValue(args, i, "--change")
			if err != nil {
				return SealArgs{}, err
			}
			if err := validateChangeName(value); err != nil {
				return SealArgs{}, err
			}
			parsed.Change = value
			i = next
		case "--flow-mode":
			value, next, err := requireFlagValue(args, i, "--flow-mode")
			if err != nil {
				return SealArgs{}, err
			}
			mode := FlowMode(value)
			if !validFlowModes[mode] {
				return SealArgs{}, fmt.Errorf("--flow-mode desconocido %q", value)
			}
			parsed.FlowMode = mode
			i = next
		case "--execution-style":
			value, next, err := requireFlagValue(args, i, "--execution-style")
			if err != nil {
				return SealArgs{}, err
			}
			style := ExecutionStyle(value)
			if !validExecutionStyles[style] {
				return SealArgs{}, fmt.Errorf("--execution-style desconocido %q", value)
			}
			parsed.ExecutionStyle = style
			parsed.ExecutionStyleSource = "explicit"
			executionStyleExplicit = true
			i = next
		case "--from-session-pace":
			value, next, err := requireFlagValue(args, i, "--from-session-pace")
			if err != nil {
				return SealArgs{}, err
			}
			if value != "interactive" && value != "auto" {
				return SealArgs{}, fmt.Errorf("--from-session-pace desconocido %q; opciones: interactive, auto", value)
			}
			sessionPaceGiven = true
			sessionPaceValue = value
			i = next
		case "--handoff-policy":
			value, next, err := requireFlagValue(args, i, "--handoff-policy")
			if err != nil {
				return SealArgs{}, err
			}
			policy := HandoffPolicy(value)
			if !validHandoffPolicies[policy] {
				return SealArgs{}, fmt.Errorf("--handoff-policy desconocido %q", value)
			}
			parsed.HandoffPolicy = policy
			i = next
		case "--role":
			value, next, err := requireFlagValue(args, i, "--role")
			if err != nil {
				return SealArgs{}, err
			}
			roleFlag, err := parseRoleFlag(value)
			if err != nil {
				return SealArgs{}, err
			}
			parsed.Roles = append(parsed.Roles, roleFlag)
			i = next
		case "--deployment-target":
			value, next, err := requireFlagValue(args, i, "--deployment-target")
			if err != nil {
				return SealArgs{}, err
			}
			if !validDeploymentTargets[value] {
				return SealArgs{}, fmt.Errorf("--deployment-target desconocido %q", value)
			}
			parsed.DeploymentTarget = value
			i = next
		default:
			return SealArgs{}, fmt.Errorf("bandera desconocida %q para kickoff seal", arg)
		}
	}

	if parsed.Change == "" {
		return SealArgs{}, fmt.Errorf("kickoff seal requiere --change")
	}

	if parsed.Infer {
		return parsed, nil
	}

	if !executionStyleExplicit && !sessionPaceGiven {
		return SealArgs{}, fmt.Errorf("kickoff seal requiere --execution-style o --from-session-pace (REQ-21.5: no se asume un valor por defecto)")
	}
	if sessionPaceGiven {
		derived := ExecutionCheckpointed
		if sessionPaceValue == "auto" {
			derived = ExecutionContinuous
		}
		if executionStyleExplicit {
			parsed.ExecutionStyleOverride = true
		} else {
			parsed.ExecutionStyle = derived
			parsed.ExecutionStyleSource = "session_pace"
		}
	}
	if parsed.HandoffPolicy == "" {
		return SealArgs{}, fmt.Errorf("kickoff seal requiere --handoff-policy")
	}
	if len(parsed.Roles) == 0 {
		parsed.Roles = []RoleFlag{{Role: "fullstack", GatePolicy: multirole.PolicyBlocking}}
	}
	if parsed.DeploymentTarget == "" {
		parsed.DeploymentTarget = "local"
	}

	return parsed, nil
}

// ToKickoff assembles the sealed document body an explicit (non --infer)
// `axiom sdd kickoff seal` invocation requests. Role file naming reuses
// defaultRoleArtifactFiles, the exact same convention InferKickoff's
// retro-seal already applies (types.go), so an explicit seal and an
// inferred one can never name a role's tasks/verify files differently.
// change is assigned separately from the parsed flags because ParseSealArgs
// never resolves it against the filesystem (task 8.2).
func (a SealArgs) ToKickoff(change string) Kickoff {
	roles := make([]KickoffRole, 0, len(a.Roles))
	for _, r := range a.Roles {
		tasksFile, verifyFile := defaultRoleArtifactFiles(r.Role)
		roles = append(roles, KickoffRole{
			Role:       r.Role,
			GatePolicy: r.GatePolicy,
			TasksFile:  tasksFile,
			VerifyFile: verifyFile,
		})
	}
	return Kickoff{
		Schema:   KickoffSchemaV1,
		Change:   change,
		SealedBy: "cli",
		Config: FlowConfig{
			FlowMode:             a.FlowMode,
			ExecutionStyle:       a.ExecutionStyle,
			ExecutionStyleSource: a.ExecutionStyleSource,
			HandoffPolicy:        a.HandoffPolicy,
			Roles:                roles,
		},
		Lifecycle: Lifecycle{DeploymentTarget: a.DeploymentTarget, PostArchivePolicy: "bug_only"},
	}
}

// GateRecordArgs is the parsed, validated result of `axiom sdd gate
// record`. The four evidence-related fields (EvidenceKind, Commit, BaseRef,
// Evidence) are only ever populated together with a non-empty EvidenceKind
// (design.md S5.7); a gate approval that declares none of them behaves
// exactly as it did before Phase 21.
type GateRecordArgs struct {
	CWD      string
	Change   string
	Gate     GateKey
	Decision GateDecision
	Reason   string
	Actor    string
	JSON     bool

	// EvidenceKind classifies the integration evidence this decision
	// attaches, when any. Commit feeds the local ancestry check for
	// EvidencePRMerged (the only kind Axiom can verify); BaseRef is the
	// branch it must be an ancestor of, defaulting to "main" when omitted;
	// Evidence is the free-text reference an operator supplies for
	// EvidenceDeployment/EvidenceAttestation, neither of which Axiom can
	// check locally (D-13, T-11).
	EvidenceKind EvidenceKind
	Commit       string
	BaseRef      string
	Evidence     string
}

// validateGateKey checks --gate against the closed vocabulary: the four
// fixed keys, or a role-apply:<rol> key with a non-empty role segment.
// Whether that role actually belongs to the sealed roster is a stateful
// check the CLI adapter makes after loading kickoff.yaml (Phase 10); this
// function only checks shape.
func validateGateKey(value string) (GateKey, error) {
	key := GateKey(value)
	if fixedGateKeys[key] {
		return key, nil
	}
	if strings.HasPrefix(value, roleApplyGatePrefix) {
		if strings.TrimPrefix(value, roleApplyGatePrefix) == "" {
			return "", fmt.Errorf("--gate %q no declara un nombre de rol tras %q", value, roleApplyGatePrefix)
		}
		return key, nil
	}
	return "", fmt.Errorf("--gate %q desconocido; opciones: spec, design, tasks, role-apply:<rol>, integration", value)
}

// ParseGateRecordArgs parses and validates `axiom sdd gate record`
// arguments (design.md S5.7).
func ParseGateRecordArgs(args []string) (GateRecordArgs, error) {
	var parsed GateRecordArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.JSON = true
		case "--cwd":
			value, next, err := requireFlagValue(args, i, "--cwd")
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.CWD = value
			i = next
		case "--change":
			value, next, err := requireFlagValue(args, i, "--change")
			if err != nil {
				return GateRecordArgs{}, err
			}
			if err := validateChangeName(value); err != nil {
				return GateRecordArgs{}, err
			}
			parsed.Change = value
			i = next
		case "--gate":
			value, next, err := requireFlagValue(args, i, "--gate")
			if err != nil {
				return GateRecordArgs{}, err
			}
			key, err := validateGateKey(value)
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.Gate = key
			i = next
		case "--decision":
			value, next, err := requireFlagValue(args, i, "--decision")
			if err != nil {
				return GateRecordArgs{}, err
			}
			decision := GateDecision(value)
			if !validGateDecisions[decision] {
				return GateRecordArgs{}, fmt.Errorf("--decision desconocida %q; opciones: approved, rejected", value)
			}
			parsed.Decision = decision
			i = next
		case "--reason":
			value, next, err := requireFlagValue(args, i, "--reason")
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.Reason = value
			i = next
		case "--actor":
			value, next, err := requireFlagValue(args, i, "--actor")
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.Actor = value
			i = next
		case "--evidence-kind":
			value, next, err := requireFlagValue(args, i, "--evidence-kind")
			if err != nil {
				return GateRecordArgs{}, err
			}
			kind := EvidenceKind(value)
			if !validEvidenceKinds[kind] {
				return GateRecordArgs{}, fmt.Errorf("--evidence-kind desconocido %q; opciones: pr_merged, deployment, attestation", value)
			}
			parsed.EvidenceKind = kind
			i = next
		case "--commit":
			value, next, err := requireFlagValue(args, i, "--commit")
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.Commit = value
			i = next
		case "--base-ref":
			value, next, err := requireFlagValue(args, i, "--base-ref")
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.BaseRef = value
			i = next
		case "--evidence":
			value, next, err := requireFlagValue(args, i, "--evidence")
			if err != nil {
				return GateRecordArgs{}, err
			}
			parsed.Evidence = value
			i = next
		default:
			return GateRecordArgs{}, fmt.Errorf("bandera desconocida %q para gate record", arg)
		}
	}

	if parsed.Change == "" {
		return GateRecordArgs{}, fmt.Errorf("gate record requiere --change")
	}
	if parsed.Gate == "" {
		return GateRecordArgs{}, fmt.Errorf("gate record requiere --gate")
	}
	if parsed.Decision == "" {
		return GateRecordArgs{}, fmt.Errorf("gate record requiere --decision")
	}
	if parsed.Decision == DecisionRejected && strings.TrimSpace(parsed.Reason) == "" {
		return GateRecordArgs{}, fmt.Errorf("gate record --decision rejected requiere --reason (REQ-21.12 exige registrar el motivo)")
	}
	if parsed.EvidenceKind == EvidencePRMerged {
		if strings.TrimSpace(parsed.Commit) == "" {
			return GateRecordArgs{}, fmt.Errorf("gate record --evidence-kind pr_merged requiere --commit")
		}
		if parsed.BaseRef == "" {
			parsed.BaseRef = "main"
		}
	}

	return parsed, nil
}

// ShowArgs is the parsed, validated result shared by `axiom sdd kickoff
// show` and `axiom sdd gate show`: both only ever need to locate a change
// and pick an output format.
type ShowArgs struct {
	CWD    string
	Change string
	JSON   bool
}

// ParseShowArgs parses and validates a `show` subcommand's arguments.
func ParseShowArgs(args []string) (ShowArgs, error) {
	var parsed ShowArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			parsed.JSON = true
		case "--cwd":
			value, next, err := requireFlagValue(args, i, "--cwd")
			if err != nil {
				return ShowArgs{}, err
			}
			parsed.CWD = value
			i = next
		case "--change":
			value, next, err := requireFlagValue(args, i, "--change")
			if err != nil {
				return ShowArgs{}, err
			}
			if err := validateChangeName(value); err != nil {
				return ShowArgs{}, err
			}
			parsed.Change = value
			i = next
		default:
			return ShowArgs{}, fmt.Errorf("bandera desconocida %q para show", arg)
		}
	}
	if parsed.Change == "" {
		return ShowArgs{}, fmt.Errorf("show requiere --change")
	}
	return parsed, nil
}
