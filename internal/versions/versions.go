// Package versions centralizes pinned external package versions so Renovate can
// auto-PR bumps. The marker comments are machine-readable directives consumed
// by the customManager defined in renovate.json — keep them in the exact form
// `// renovate: datasource=<ds> depName=<name>` immediately above each const.
//
// ClaudeCode, Kilocode, QwenCode, Codex, and GeminiCLI pins were removed here:
// gentle-ai no longer installs agent runtimes on the user's behalf (see
// agentInstallStep in internal/cli/run.go), and the display-only refusal
// commands that used to read these pins now advise "latest" instead, since a
// human runs them and a frozen version goes stale as soon as a newer release
// ships. OpenCode keeps its pin below: it is a real, separate owner (a CI
// workflow install and the organic-runtime E2E), unrelated to the refusal
// text gentle-ai prints when OpenCode isn't detected.
package versions

// renovate: datasource=npm depName=opencode-ai
const OpenCode = "1.18.10"

// renovate: datasource=npm depName=@upstash/context7-mcp
const Context7MCP = "2.2.5"

// renovate: datasource=github-releases depName=Gentleman-Programming/gentleman-guardian-angel
const GGAVersion = "2.10.1"
