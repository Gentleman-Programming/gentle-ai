// Package versions centralizes pinned npm package versions so Renovate can
// auto-PR bumps. The marker comments are machine-readable directives consumed
// by the customManager defined in renovate.json — keep them in the exact form
// `// renovate: datasource=<ds> depName=<name>` immediately above each const.
package versions

// renovate: datasource=npm depName=@anthropic-ai/claude-code
const ClaudeCode = "2.1.140"

// renovate: datasource=npm depName=@kilocode/cli
const Kilocode = "7.2.52"

// renovate: datasource=npm depName=opencode-ai
const OpenCode = "1.14.48"

// renovate: datasource=npm depName=@qwen-code/qwen-code
const QwenCode = "0.15.10"

// renovate: datasource=npm depName=@openai/codex
const Codex = "0.130.0"

// renovate: datasource=npm depName=@google/gemini-cli
const GeminiCLI = "0.41.2"

// renovate: datasource=npm depName=@upstash/context7-mcp
const Context7MCP = "2.2.5"

// renovate: datasource=npm depName=gentle-engram
const GentleEngram = "0.1.4"
