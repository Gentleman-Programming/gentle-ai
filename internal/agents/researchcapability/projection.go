package researchcapability

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v2/internal/model"
)

// declarationPrefix is the exact anchor every generated runtime research
// projection must carry exactly once. The authority validates the shipped
// assets against this grammar instead of trusting them independently (#4088).
const declarationPrefix = "Evidence grants: "

// declarationLinePattern pins the declaration body. Class order is fixed
// (documentation, then open-web), separators are single spaces, and the
// trailing period is required; anything else refuses.
var declarationLinePattern = regexp.MustCompile(`^documentation=\[([A-Za-z0-9@_.,-]*)\]; open-web=\[([A-Za-z0-9@_.,-]*)\]\.`)

// declarationTokenPattern is the grant alphabet. It deliberately excludes
// spaces and commas so `[A, B]`, empty tokens, and trailing commas cannot
// parse as valid declarations.
var declarationTokenPattern = regexp.MustCompile(`^[A-Za-z0-9@_.-]+$`)

// ToolBinding maps one canonical grant to the runtime-specific tool identity
// that exposes it. Bindings are ordered per agent, and that slice order is
// part of the contract: projections must never iterate a map to build a tool
// surface.
type ToolBinding struct {
	Grant Grant
	Tool  string
}

// adapterToolBindings is the explicit grant-to-tool mapping per runtime. Only
// tool identities listed here are evidence: Bash, generic MCP, persistence,
// and inherited tools are never bindings.
var adapterToolBindings = map[model.AgentID][]ToolBinding{
	model.AgentClaudeCode: {
		{Grant: GrantWebFetch, Tool: "WebFetch"},
		{Grant: GrantWebSearch, Tool: "WebSearch"},
	},
	model.AgentKiroIDE: {
		{Grant: GrantContext7, Tool: "@context7"},
	},
	model.AgentPi: {
		{Grant: GrantPiFetchContent, Tool: "fetch_content"},
		{Grant: GrantPiWebSearch, Tool: "web_search"},
		{Grant: GrantPiSourceCheck, Tool: "source_check"},
		{Grant: GrantPiGetSearchContent, Tool: "get_search_content"},
	},
	model.AgentOpenCode: {
		{Grant: GrantWebFetch, Tool: "webfetch"},
		{Grant: GrantWebSearch, Tool: "websearch"},
	},
	model.AgentKilocode: {
		{Grant: GrantWebFetch, Tool: "webfetch"},
		{Grant: GrantWebSearch, Tool: "websearch"},
	},
}

// AdapterToolBindings returns a defensive copy of one agent's ordered
// grant-to-tool mapping. Agents without bindings have no evidence tool surface
// and therefore deny every observed tool identity.
func AdapterToolBindings(agent model.AgentID) []ToolBinding {
	bindings := adapterToolBindings[agent]
	if len(bindings) == 0 {
		return nil
	}
	return append([]ToolBinding(nil), bindings...)
}

// AdapterToolIdentity resolves the runtime tool identity for one grant. The
// second result is false when the agent has no binding for that grant, which
// makes the grant unmappable rather than implicitly admitted.
func AdapterToolIdentity(agent model.AgentID, grant Grant) (string, bool) {
	for _, binding := range adapterToolBindings[agent] {
		if binding.Grant == grant {
			return binding.Tool, true
		}
	}
	return "", false
}

// Declaration renders the canonical evidence declaration for one agent.
// Denied agents render both classes empty. The output round-trips through
// ParseDeclaration and must equal the declaration embedded in the shipped
// runtime assets; assets are verified against this function, never edited to
// match it (#4088).
func Declaration(agent model.AgentID) string {
	capability, ok := ForAgent(agent)
	var documentation, openWeb []Grant
	if ok {
		documentation = capability.Grants[ClassDocumentation]
		openWeb = capability.Grants[ClassOpenWeb]
	}
	return declarationPrefix +
		"documentation=[" + joinDeclarationGrants(documentation) + "]" +
		"; open-web=[" + joinDeclarationGrants(openWeb) + "]."
}

func joinDeclarationGrants(grants []Grant) string {
	values := make([]string, len(grants))
	for index, grant := range grants {
		values[index] = string(grant)
	}
	return strings.Join(values, ",")
}

// ParseDeclaration extracts the canonical evidence declaration from runtime
// projection text. It is deliberately strict: exactly one marker, exactly the
// documented grammar, and a boundary after the trailing period. Any deviation
// refuses instead of guessing, because a lenient parse would let an
// unvalidated projection masquerade as the authority (#4088).
func ParseDeclaration(text string) (map[Class][]Grant, bool) {
	index := strings.Index(text, declarationPrefix)
	if index < 0 {
		return nil, false
	}
	if strings.Contains(text[index+len(declarationPrefix):], declarationPrefix) {
		return nil, false
	}
	segment := text[index+len(declarationPrefix):]
	match := declarationLinePattern.FindStringSubmatchIndex(segment)
	if match == nil {
		return nil, false
	}
	if match[1] < len(segment) && !isDeclarationBoundary(segment[match[1]]) {
		return nil, false
	}
	documentation, ok := parseDeclarationList(segment[match[2]:match[3]])
	if !ok {
		return nil, false
	}
	openWeb, ok := parseDeclarationList(segment[match[4]:match[5]])
	if !ok {
		return nil, false
	}
	return map[Class][]Grant{
		ClassDocumentation: documentation,
		ClassOpenWeb:       openWeb,
	}, true
}

func isDeclarationBoundary(char byte) bool {
	switch char {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func parseDeclarationList(raw string) ([]Grant, bool) {
	if raw == "" {
		return []Grant{}, true
	}
	tokens := strings.Split(raw, ",")
	grants := make([]Grant, 0, len(tokens))
	seen := make(map[Grant]bool, len(tokens))
	for _, token := range tokens {
		if !declarationTokenPattern.MatchString(token) {
			return nil, false
		}
		grant := Grant(token)
		if seen[grant] {
			return nil, false
		}
		seen[grant] = true
		grants = append(grants, grant)
	}
	return grants, true
}

// RuntimeProjection is one generated runtime research projection: declared
// evidence grants plus the evidence tool identities the projection exposes.
type RuntimeProjection struct {
	Agent          model.AgentID
	Declared       map[Class][]Grant // nil when no inline declaration
	HasDeclaration bool
	// Allowlist=true: the tool surface lists exactly the available tools
	// (absence denies; Claude/Kiro/Cursor/Kimi frontmatter).
	// Allowlist=false: tools are default-open and must be explicitly denied
	// (OpenCode/Kilocode permission maps).
	Allowlist    bool
	AllowedTools []string
	DeniedTools  []string
}

// VerifyProjection fails closed when a generated runtime research projection
// does not exactly project the canonical capability authority. It returns only
// an error: it never produces claims or grants, so a mismatch can only stop
// generation.
func VerifyProjection(projection RuntimeProjection) error {
	canonical, ok := ForAgent(projection.Agent)
	if !ok {
		canonical = Capability{}
	}
	if projection.HasDeclaration {
		if err := verifyDeclaredClasses(projection, canonical); err != nil {
			return err
		}
	} else {
		if canonicalDeclaresGrants(canonical) {
			return fmt.Errorf("agent %s: canonical research capability declares grants but the projection carries no declaration", projection.Agent)
		}
		if len(projection.Declared) != 0 {
			return fmt.Errorf("agent %s: projection without a declaration must not carry declared grants", projection.Agent)
		}
	}
	expected, err := expectedToolIdentities(projection.Agent, canonical)
	if err != nil {
		return err
	}
	allowed, err := verifyAllowedTools(projection, expected)
	if err != nil {
		return err
	}
	if projection.Allowlist {
		if len(projection.DeniedTools) != 0 {
			return fmt.Errorf("agent %s: allowlist projection carries denied tools %v, but allowlist formats have no deny channel", projection.Agent, projection.DeniedTools)
		}
		return nil
	}
	return verifyDeniedTools(projection, expected, allowed)
}

func verifyDeclaredClasses(projection RuntimeProjection, canonical Capability) error {
	for class := range projection.Declared {
		if class != ClassDocumentation && class != ClassOpenWeb {
			return fmt.Errorf("agent %s: unknown declared research class %q", projection.Agent, class)
		}
	}
	for _, class := range []Class{ClassDocumentation, ClassOpenWeb} {
		want := canonical.Grants[class]
		got := projection.Declared[class]
		if !sameGrants(got, want) {
			return fmt.Errorf("agent %s: declared %s grants %v do not exact-match canonical grants %v", projection.Agent, class, got, want)
		}
	}
	return nil
}

func canonicalDeclaresGrants(canonical Capability) bool {
	for _, grants := range canonical.Grants {
		if len(grants) > 0 {
			return true
		}
	}
	return false
}

func expectedToolIdentities(agent model.AgentID, canonical Capability) (map[string]bool, error) {
	expected := make(map[string]bool)
	for class, grants := range canonical.Grants {
		for _, grant := range grants {
			tool, ok := AdapterToolIdentity(agent, grant)
			if !ok {
				return nil, fmt.Errorf("agent %s: canonical grant %q in class %s has no adapter tool binding", agent, grant, class)
			}
			expected[tool] = true
		}
	}
	return expected, nil
}

// verifyAllowedTools checks the projected allow surface and returns it as a
// set for the default-open denial check.
func verifyAllowedTools(projection RuntimeProjection, expected map[string]bool) (map[string]bool, error) {
	allowed := make(map[string]bool, len(projection.AllowedTools))
	for _, tool := range projection.AllowedTools {
		if allowed[tool] {
			return nil, fmt.Errorf("agent %s: duplicate allowed evidence tool %q", projection.Agent, tool)
		}
		if !isKnownToolIdentity(projection.Agent, tool) {
			return nil, fmt.Errorf("agent %s: allowed evidence tool %q has no adapter binding", projection.Agent, tool)
		}
		allowed[tool] = true
	}
	for tool := range expected {
		if !allowed[tool] {
			return nil, fmt.Errorf("agent %s: canonical evidence tool %q is missing from the projected tool surface", projection.Agent, tool)
		}
	}
	for tool := range allowed {
		if !expected[tool] {
			return nil, fmt.Errorf("agent %s: projected evidence tool %q is not declared by the canonical capability", projection.Agent, tool)
		}
	}
	return allowed, nil
}

func verifyDeniedTools(projection RuntimeProjection, expected, allowed map[string]bool) error {
	denied := make(map[string]bool, len(projection.DeniedTools))
	for _, tool := range projection.DeniedTools {
		if !isKnownToolIdentity(projection.Agent, tool) {
			return fmt.Errorf("agent %s: denied evidence tool %q has no adapter binding", projection.Agent, tool)
		}
		if denied[tool] {
			return fmt.Errorf("agent %s: duplicate denied evidence tool %q", projection.Agent, tool)
		}
		if allowed[tool] {
			return fmt.Errorf("agent %s: evidence tool %q is both allowed and denied", projection.Agent, tool)
		}
		denied[tool] = true
	}
	for _, binding := range AdapterToolBindings(projection.Agent) {
		if expected[binding.Tool] {
			continue
		}
		if !denied[binding.Tool] {
			return fmt.Errorf("agent %s: default-open projection must explicitly deny evidence tool %q", projection.Agent, binding.Tool)
		}
	}
	return nil
}

func isKnownToolIdentity(agent model.AgentID, tool string) bool {
	for _, binding := range AdapterToolBindings(agent) {
		if binding.Tool == tool {
			return true
		}
	}
	return false
}

// ToolDecision is one evidence tool identity and whether the canonical
// capability admits that identity for the agent.
type ToolDecision struct {
	Tool    string
	Allowed bool
}

// EvidenceToolDecisions projects the canonical capability onto the agent's
// ordered tool bindings. Generation and tests both consume this function so a
// generated surface cannot drift from the authority (#4088).
func EvidenceToolDecisions(agent model.AgentID) []ToolDecision {
	bindings := AdapterToolBindings(agent)
	if len(bindings) == 0 {
		return nil
	}
	canonical, ok := ForAgent(agent)
	decisions := make([]ToolDecision, 0, len(bindings))
	for _, binding := range bindings {
		allowed := false
		if ok {
			for _, grants := range canonical.Grants {
				for _, grant := range grants {
					if grant == binding.Grant {
						allowed = true
					}
				}
			}
		}
		decisions = append(decisions, ToolDecision{Tool: binding.Tool, Allowed: allowed})
	}
	return decisions
}

// MarkdownProjection extracts the research projection from one rendered
// markdown sub-agent file (Claude, Kiro, Cursor, Kimi). The declaration is
// mandatory: an agent that cannot declare its evidence grants has no
// projection to verify.
func MarkdownProjection(agent model.AgentID, content string) (RuntimeProjection, error) {
	declared, ok := ParseDeclaration(content)
	if !ok {
		return RuntimeProjection{}, fmt.Errorf("agent %s: markdown research projection is missing exactly one valid evidence declaration", agent)
	}
	projection := RuntimeProjection{
		Agent:          agent,
		Declared:       declared,
		HasDeclaration: true,
		Allowlist:      true,
	}
	toolsLine, toolsCount, err := findToolsLine(content)
	if err != nil {
		return RuntimeProjection{}, fmt.Errorf("agent %s: %w", agent, err)
	}
	if toolsCount == 0 {
		// A runtime with bindings but no tools line still has an empty
		// projected surface; VerifyProjection rejects it when the canonical
		// matrix declares grants.
		return projection, nil
	}
	if len(AdapterToolBindings(agent)) == 0 {
		return RuntimeProjection{}, fmt.Errorf("agent %s: unexpected tools line %q for a runtime without evidence bindings", agent, toolsLine)
	}
	allowed, err := parseToolsLineTokens(agent, toolsLine)
	if err != nil {
		return RuntimeProjection{}, fmt.Errorf("agent %s: %w", agent, err)
	}
	for _, tool := range allowed {
		if !isKnownToolIdentity(agent, tool) {
			return RuntimeProjection{}, fmt.Errorf("agent %s: projected evidence tool %q has no adapter binding", agent, tool)
		}
	}
	projection.AllowedTools = allowed
	return projection, nil
}

// findToolsLine locates the single frontmatter tools line. Only the initial
// frontmatter block is the tool surface the runtime loads: a `tools:` line in
// the body is ignored rather than trusted, and a document without a closed
// frontmatter block refuses. Within the block the line must start with
// `tools:` and a space or tab, so `exclude_tools:` and bare `tools:` do not
// match; more than one match is ambiguous and refuses.
func findToolsLine(content string) (string, int, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != "---" {
		return "", 0, fmt.Errorf("missing markdown frontmatter")
	}
	count := 0
	value := ""
	closed := false
	for _, line := range lines[1:] {
		line = strings.TrimSuffix(line, "\r")
		if line == "---" {
			closed = true
			break
		}
		if !strings.HasPrefix(line, "tools:") {
			continue
		}
		rest := line[len("tools:"):]
		if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		count++
		value = strings.TrimSpace(rest)
	}
	if !closed {
		return "", 0, fmt.Errorf("unterminated markdown frontmatter")
	}
	if count > 1 {
		return "", count, fmt.Errorf("found %d tools lines, want exactly one", count)
	}
	return value, count, nil
}

func parseToolsLineTokens(agent model.AgentID, value string) ([]string, error) {
	switch agent {
	case model.AgentClaudeCode:
		return parseCommaSpaceTokens(value)
	case model.AgentKiroIDE:
		return parseJSONToolArray(value)
	default:
		return nil, fmt.Errorf("no markdown tools grammar is defined for this runtime")
	}
}

func parseCommaSpaceTokens(value string) ([]string, error) {
	tokens := strings.Split(value, ", ")
	seen := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		if token == "" || strings.TrimSpace(token) != token {
			return nil, fmt.Errorf("tools value %q is not a comma-space separated token list", value)
		}
		if seen[token] {
			return nil, fmt.Errorf("tools value %q repeats token %q", value, token)
		}
		seen[token] = true
	}
	return tokens, nil
}

func parseJSONToolArray(value string) ([]string, error) {
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("tools value %q is not a JSON string array", value)
	}
	var tools []string
	if err := json.Unmarshal([]byte(value), &tools); err != nil {
		return nil, fmt.Errorf("tools value %q is not a JSON string array: %w", value, err)
	}
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool == "" {
			return nil, fmt.Errorf("tools value %q contains an empty token", value)
		}
		if seen[tool] {
			return nil, fmt.Errorf("tools value %q repeats token %q", value, tool)
		}
		seen[tool] = true
	}
	return tools, nil
}

// OpenCodeProjection extracts the research projection from one OpenCode agent
// entry. The entry either carries an inline declaration or references the
// shared research prompt file; every evidence binding must have an explicit
// allow/deny decision in the permission map, because OpenCode tools are
// default-open.
func OpenCodeProjection(agent model.AgentID, agentEntry map[string]any) (RuntimeProjection, error) {
	if agentEntry == nil {
		return RuntimeProjection{}, fmt.Errorf("agent %s: research agent entry is missing", agent)
	}
	if _, exists := agentEntry["tools"]; exists {
		return RuntimeProjection{}, fmt.Errorf("agent %s: deprecated tools key is not a valid research projection; evidence decisions belong in permission", agent)
	}
	prompt, ok := agentEntry["prompt"].(string)
	if !ok {
		return RuntimeProjection{}, fmt.Errorf("agent %s: research prompt has type %T, want string", agent, agentEntry["prompt"])
	}
	projection := RuntimeProjection{Agent: agent}
	if strings.Contains(prompt, declarationPrefix) {
		declared, parsed := ParseDeclaration(prompt)
		if !parsed {
			return RuntimeProjection{}, fmt.Errorf("agent %s: research prompt carries an invalid evidence declaration", agent)
		}
		projection.Declared = declared
		projection.HasDeclaration = true
	} else if !isSharedResearchPromptRef(prompt) {
		return RuntimeProjection{}, fmt.Errorf("agent %s: research prompt is neither a valid evidence declaration nor the shared research prompt reference", agent)
	}
	permission, ok := agentEntry["permission"].(map[string]any)
	if !ok {
		return RuntimeProjection{}, fmt.Errorf("agent %s: research permission has type %T, want an object", agent, agentEntry["permission"])
	}
	for _, binding := range AdapterToolBindings(agent) {
		raw, exists := permission[binding.Tool]
		if !exists {
			return RuntimeProjection{}, fmt.Errorf("agent %s: permission is missing an explicit decision for evidence tool %q", agent, binding.Tool)
		}
		decision, ok := raw.(string)
		if !ok || (decision != "allow" && decision != "deny") {
			return RuntimeProjection{}, fmt.Errorf("agent %s: permission for evidence tool %q = %#v, want \"allow\" or \"deny\"", agent, binding.Tool, raw)
		}
		if decision == "allow" {
			projection.AllowedTools = append(projection.AllowedTools, binding.Tool)
		} else {
			projection.DeniedTools = append(projection.DeniedTools, binding.Tool)
		}
	}
	return projection, nil
}

func isSharedResearchPromptRef(prompt string) bool {
	trimmed := strings.TrimSpace(prompt)
	if !strings.HasPrefix(trimmed, "{file:") || !strings.HasSuffix(trimmed, "}") {
		return false
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(trimmed, "{file:"), "}")
	if inner == "" || strings.ContainsAny(inner, "{}") {
		return false
	}
	normalized := strings.ReplaceAll(inner, `\`, "/")
	return strings.HasSuffix(normalized, "prompts/sdd/sdd-research.md")
}
