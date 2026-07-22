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

type sddParamConfig struct {
	key        string
	label      string
	required   bool
	defaultVal string
}

type sddToolConfig struct {
	title           string
	params          []sddParamConfig
	guidelinesTitle string
	guidelines      []string
}

func handleSDDTool(cfg sddToolConfig, args map[string]interface{}) (*ToolCallResult, error) {
	var sb strings.Builder
	sb.WriteString(cfg.title)

	for _, p := range cfg.params {
		val, err := getStringArg(args, p.key, p.required)
		if err != nil {
			return nil, err
		}
		if val == "" && p.defaultVal != "" {
			val = p.defaultVal
		}
		if val != "" {
			sb.WriteString(fmt.Sprintf("**%s**: %s\n", p.label, val))
		}
	}

	if cfg.guidelinesTitle != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", cfg.guidelinesTitle))
	}

	for _, g := range cfg.guidelines {
		sb.WriteString(fmt.Sprintf("%s\n", g))
	}

	return &ToolCallResult{
		Content: []TextContent{{Type: "text", Text: sb.String()}},
		IsError: false,
	}, nil
}

var (
	sddExploreConfig = sddToolConfig{
		title: "## SDD Exploration Protocol (Reasoning Tool)\n\n",
		params: []sddParamConfig{
			{key: "topic", label: "Topic", required: true},
			{key: "context", label: "Context", required: false},
		},
		guidelinesTitle: "### Exploration Guidelines:",
		guidelines: []string{
			"1. **Codebase Inspection**: Locate relevant packages, interfaces, and entry points.",
			"2. **Contract Analysis**: Examine input/output data structures, errors, and side effects.",
			"3. **Risk Assessment**: Identify architectural dependencies and potential failure points.",
			"4. **Synthesis**: Document findings and prepare clear recommendations for proposal/design.",
		},
	}

	sddReviewConfig = sddToolConfig{
		title: "## SDD 4R Review Protocol (Reasoning Tool)\n\n",
		params: []sddParamConfig{
			{key: "artifact", label: "Target Artifact", required: false},
			{key: "focus", label: "Focus Area", required: false, defaultVal: "4r"},
		},
		guidelinesTitle: "### 4R Protocol Steps:",
		guidelines: []string{
			"- **Read**: Inspect source code and artifacts thoroughly without assumptions or line skipping.",
			"- **Reason**: Evaluate correctness, invariants, edge cases, error paths, and non-functional requirements.",
			"- **Reconcile**: Validate alignment with design specifications, interfaces, and project guidelines.",
			"- **Report**: Provide actionable, precise, and objective review feedback with line citations.",
		},
	}

	sddProposeConfig = sddToolConfig{
		title: "## SDD Proposal Protocol\n\n",
		params: []sddParamConfig{
			{key: "change", label: "Proposed Change", required: true},
			{key: "scope", label: "Scope & Boundaries", required: false},
		},
		guidelinesTitle: "### Guidelines:",
		guidelines: []string{
			"- Define problem statement and motivation.",
			"- Outline key architectural decisions and alternative approaches considered.",
			"- List success criteria and explicit non-goals.",
		},
	}

	sddSpecConfig = sddToolConfig{
		title: "## SDD Specification Protocol\n\n",
		params: []sddParamConfig{
			{key: "feature", label: "Feature", required: true},
			{key: "requirements", label: "Requirements", required: false},
		},
		guidelinesTitle: "### Guidelines:",
		guidelines: []string{
			"- Formulate detailed functional requirements and acceptance criteria.",
			"- Define edge cases, error conditions, and API contracts.",
		},
	}

	sddDesignConfig = sddToolConfig{
		title: "## SDD Technical Design Protocol\n\n",
		params: []sddParamConfig{
			{key: "spec", label: "Target Spec", required: true},
			{key: "architecture", label: "Architecture", required: false},
		},
		guidelinesTitle: "### Guidelines:",
		guidelines: []string{
			"- Detail package layout, data structures, and function signatures.",
			"- Document concurrency, state management, and error handling strategies.",
		},
	}

	sddTasksConfig = sddToolConfig{
		title: "## SDD Task Breakdown Protocol\n\n",
		params: []sddParamConfig{
			{key: "design", label: "Design Reference", required: true},
			{key: "phases", label: "Phases", required: false},
		},
		guidelinesTitle: "### Guidelines:",
		guidelines: []string{
			"- Create incremental, testable tasks for implementation.",
			"- Mark tasks with verification criteria and dependencies.",
		},
	}
)

// HandleSDDExplore executes the sdd_explore reasoning tool.
func HandleSDDExplore(args map[string]interface{}) (*ToolCallResult, error) {
	return handleSDDTool(sddExploreConfig, args)
}

// HandleSDDReview executes the sdd_review reasoning tool.
func HandleSDDReview(args map[string]interface{}) (*ToolCallResult, error) {
	return handleSDDTool(sddReviewConfig, args)
}

// HandleSDDPropose executes the sdd_propose reasoning tool.
func HandleSDDPropose(args map[string]interface{}) (*ToolCallResult, error) {
	return handleSDDTool(sddProposeConfig, args)
}

// HandleSDDSpec executes the sdd_spec reasoning tool.
func HandleSDDSpec(args map[string]interface{}) (*ToolCallResult, error) {
	return handleSDDTool(sddSpecConfig, args)
}

// HandleSDDDesign executes the sdd_design reasoning tool.
func HandleSDDDesign(args map[string]interface{}) (*ToolCallResult, error) {
	return handleSDDTool(sddDesignConfig, args)
}

// HandleSDDTasks executes the sdd_tasks reasoning tool.
func HandleSDDTasks(args map[string]interface{}) (*ToolCallResult, error) {
	return handleSDDTool(sddTasksConfig, args)
}
