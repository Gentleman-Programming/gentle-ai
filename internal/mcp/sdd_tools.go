package mcp

import (
	"fmt"
	"strings"
)

// DefaultSDDTools returns all pre-packaged SDD reasoning tools for MCP.
func DefaultSDDTools() []Tool {
	return []Tool{
		{
			Name:        "sdd_explore",
			Description: "Investigate ideas, codebase structure, architecture, and requirements before committing to a change.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"topic": {
						Type:        "string",
						Description: "The topic, feature idea, or problem to explore.",
					},
					"context": {
						Type:        "string",
						Description: "Optional background context, constraints, or specific files to inspect.",
					},
				},
				Required: []string{"topic"},
			},
		},
		{
			Name:        "sdd_review",
			Description: "Perform 4R review (Read, Reason, Reconcile, Report) or SDD artifact review.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"artifact": {
						Type:        "string",
						Description: "Optional SDD artifact content or path to review.",
					},
					"focus": {
						Type:        "string",
						Description: "Optional focus area for review (e.g. '4r', 'architecture', 'security', 'correctness').",
					},
				},
			},
		},
		{
			Name:        "sdd_propose",
			Description: "Formulate an SDD proposal outlining change intent, scope, risks, and success criteria.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"change": {
						Type:        "string",
						Description: "Description of the proposed change.",
					},
					"scope": {
						Type:        "string",
						Description: "Optional scope boundaries and non-goals.",
					},
				},
				Required: []string{"change"},
			},
		},
		{
			Name:        "sdd_spec",
			Description: "Draft an SDD requirement specification with user stories and acceptance criteria.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"feature": {
						Type:        "string",
						Description: "Feature name or target functionality to specify.",
					},
					"requirements": {
						Type:        "string",
						Description: "Optional specific requirements or user stories.",
					},
				},
				Required: []string{"feature"},
			},
		},
		{
			Name:        "sdd_design",
			Description: "Draft an SDD technical design document detailing module architecture and data models.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"spec": {
						Type:        "string",
						Description: "Specification or target system architecture to design.",
					},
					"architecture": {
						Type:        "string",
						Description: "Optional architectural constraints or component details.",
					},
				},
				Required: []string{"spec"},
			},
		},
		{
			Name:        "sdd_tasks",
			Description: "Break down an SDD design into an actionable, sequential task checklist.",
			InputSchema: ToolSchema{
				Type: "object",
				Properties: map[string]PropertySchema{
					"design": {
						Type:        "string",
						Description: "Design document or system plan to break into tasks.",
					},
					"phases": {
						Type:        "string",
						Description: "Optional execution phases or sequencing guidance.",
					},
				},
				Required: []string{"design"},
			},
		},
	}
}

func getStringArg(args map[string]interface{}, key string, required bool) (string, error) {
	if args == nil {
		if required {
			return "", &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("missing required string parameter '%s'", key),
			}
		}
		return "", nil
	}

	val, exists := args[key]
	if !exists || val == nil {
		if required {
			return "", &RPCError{
				Code:    ErrCodeInvalidParams,
				Message: fmt.Sprintf("missing required string parameter '%s'", key),
			}
		}
		return "", nil
	}

	strVal, ok := val.(string)
	if !ok {
		return "", &RPCError{
			Code:    ErrCodeInvalidParams,
			Message: fmt.Sprintf("parameter '%s' must be a string, got %T", key, val),
		}
	}

	if required && strings.TrimSpace(strVal) == "" {
		return "", &RPCError{
			Code:    ErrCodeInvalidParams,
			Message: fmt.Sprintf("required string parameter '%s' cannot be empty", key),
		}
	}

	return strVal, nil
}

// HandleSDDExplore executes the sdd_explore reasoning tool.
func HandleSDDExplore(args map[string]interface{}) (*ToolCallResult, error) {
	topic, err := getStringArg(args, "topic", true)
	if err != nil {
		return nil, err
	}
	ctxVal, err := getStringArg(args, "context", false)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("## SDD Exploration Protocol (Reasoning Tool)\n\n")
	sb.WriteString(fmt.Sprintf("**Topic**: %s\n", topic))
	if ctxVal != "" {
		sb.WriteString(fmt.Sprintf("**Context**: %s\n", ctxVal))
	}
	sb.WriteString("\n### Exploration Guidelines:\n")
	sb.WriteString("1. **Codebase Inspection**: Locate relevant packages, interfaces, and entry points.\n")
	sb.WriteString("2. **Contract Analysis**: Examine input/output data structures, errors, and side effects.\n")
	sb.WriteString("3. **Risk Assessment**: Identify architectural dependencies and potential failure points.\n")
	sb.WriteString("4. **Synthesis**: Document findings and prepare clear recommendations for proposal/design.\n")

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}

// HandleSDDReview executes the sdd_review reasoning tool.
func HandleSDDReview(args map[string]interface{}) (*ToolCallResult, error) {
	artifact, err := getStringArg(args, "artifact", false)
	if err != nil {
		return nil, err
	}
	focus, err := getStringArg(args, "focus", false)
	if err != nil {
		return nil, err
	}
	if focus == "" {
		focus = "4r"
	}

	var sb strings.Builder
	sb.WriteString("## SDD 4R Review Protocol (Reasoning Tool)\n\n")
	if artifact != "" {
		sb.WriteString(fmt.Sprintf("**Target Artifact**: %s\n", artifact))
	}
	sb.WriteString(fmt.Sprintf("**Focus Area**: %s\n\n", focus))
	sb.WriteString("### 4R Protocol Steps:\n")
	sb.WriteString("- **Read**: Inspect source code and artifacts thoroughly without assumptions or line skipping.\n")
	sb.WriteString("- **Reason**: Evaluate correctness, invariants, edge cases, error paths, and non-functional requirements.\n")
	sb.WriteString("- **Reconcile**: Validate alignment with design specifications, interfaces, and project guidelines.\n")
	sb.WriteString("- **Report**: Provide actionable, precise, and objective review feedback with line citations.\n")

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}

// HandleSDDPropose executes the sdd_propose reasoning tool.
func HandleSDDPropose(args map[string]interface{}) (*ToolCallResult, error) {
	change, err := getStringArg(args, "change", true)
	if err != nil {
		return nil, err
	}
	scope, err := getStringArg(args, "scope", false)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("## SDD Proposal Protocol\n\n")
	sb.WriteString(fmt.Sprintf("**Proposed Change**: %s\n", change))
	if scope != "" {
		sb.WriteString(fmt.Sprintf("**Scope & Boundaries**: %s\n", scope))
	}
	sb.WriteString("\n### Guidelines:\n")
	sb.WriteString("- Define problem statement and motivation.\n")
	sb.WriteString("- Outline key architectural decisions and alternative approaches considered.\n")
	sb.WriteString("- List success criteria and explicit non-goals.\n")

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}

// HandleSDDSpec executes the sdd_spec reasoning tool.
func HandleSDDSpec(args map[string]interface{}) (*ToolCallResult, error) {
	feature, err := getStringArg(args, "feature", true)
	if err != nil {
		return nil, err
	}
	reqsVal, err := getStringArg(args, "requirements", false)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("## SDD Specification Protocol\n\n")
	sb.WriteString(fmt.Sprintf("**Feature**: %s\n", feature))
	if reqsVal != "" {
		sb.WriteString(fmt.Sprintf("**Requirements**: %s\n", reqsVal))
	}
	sb.WriteString("\n### Guidelines:\n")
	sb.WriteString("- Formulate detailed functional requirements and acceptance criteria.\n")
	sb.WriteString("- Define edge cases, error conditions, and API contracts.\n")

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}

// HandleSDDDesign executes the sdd_design reasoning tool.
func HandleSDDDesign(args map[string]interface{}) (*ToolCallResult, error) {
	spec, err := getStringArg(args, "spec", true)
	if err != nil {
		return nil, err
	}
	archVal, err := getStringArg(args, "architecture", false)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("## SDD Technical Design Protocol\n\n")
	sb.WriteString(fmt.Sprintf("**Target Spec**: %s\n", spec))
	if archVal != "" {
		sb.WriteString(fmt.Sprintf("**Architecture**: %s\n", archVal))
	}
	sb.WriteString("\n### Guidelines:\n")
	sb.WriteString("- Detail package layout, data structures, and function signatures.\n")
	sb.WriteString("- Document concurrency, state management, and error handling strategies.\n")

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}

// HandleSDDTasks executes the sdd_tasks reasoning tool.
func HandleSDDTasks(args map[string]interface{}) (*ToolCallResult, error) {
	design, err := getStringArg(args, "design", true)
	if err != nil {
		return nil, err
	}
	phasesVal, err := getStringArg(args, "phases", false)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("## SDD Task Breakdown Protocol\n\n")
	sb.WriteString(fmt.Sprintf("**Design Reference**: %s\n", design))
	if phasesVal != "" {
		sb.WriteString(fmt.Sprintf("**Phases**: %s\n", phasesVal))
	}
	sb.WriteString("\n### Guidelines:\n")
	sb.WriteString("- Create incremental, testable tasks for implementation.\n")
	sb.WriteString("- Mark tasks with verification criteria and dependencies.\n")

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}
