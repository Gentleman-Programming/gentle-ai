/**
 * background-agents
 * Unified delegation system for OpenCode
 *
 * Replaces native `task` tool with persistent, async-first agent delegation.
 * All agent outputs are persisted to storage, orchestrator receives only key references.
 *
 * Based on oh-my-opencode by @code-yeongyu (MIT License)
 * https://github.com/code-yeongyu/oh-my-opencode
 *
 * Adapted from kdcokenny/opencode-background-agents (MIT License)
 * https://github.com/kdcokenny/opencode-background-agents
 *
 * Adaptations:
 * - Inlined kdco-primitives (types, getProjectId, logWarn, withTimeout, TimeoutError)
 * - Exported as `BackgroundAgents` (matching the Engram plugin convention)
 * - All imports resolved to available node_modules
 */

import * as crypto from "node:crypto"
import * as fs from "node:fs/promises"
import * as os from "node:os"
import * as path from "node:path"
import { stat } from "node:fs/promises"
import { type Plugin, type ToolContext, tool } from "@opencode-ai/plugin"
import type { createOpencodeClient } from "@opencode-ai/sdk"
import type { Event, Message, Part, TextPart } from "@opencode-ai/sdk"
import { adjectives, animals, colors, uniqueNamesGenerator } from "unique-names-generator"

// ==========================================
// INLINED: kdco-primitives/types
// ==========================================

export type OpencodeClient = ReturnType<typeof createOpencodeClient>

// ==========================================
// INLINED: kdco-primitives/with-timeout
// ==========================================

class TimeoutError extends Error {
  readonly name = "TimeoutError" as const
  readonly timeoutMs: number
  constructor(message: string, timeoutMs: number) {
    super(message)
    this.timeoutMs = timeoutMs
  }
}

async function withTimeout<T>(
  promise: Promise<T>,
  ms: number,
  message = "Operation timed out",
): Promise<T> {
  if (typeof ms !== "number" || ms < 0)
    throw new Error(`withTimeout: timeout must be a non-negative number, got ${ms}`)
  if (ms === 0) throw new TimeoutError(message, ms)
  let timeoutId: ReturnType<typeof setTimeout>
  return Promise.race([
    promise.finally(() => clearTimeout(timeoutId)),
    new Promise<never>((_, reject) => {
      timeoutId = setTimeout(() => {
        reject(new TimeoutError(message, ms))
      }, ms)
    }),
  ])
}

// ==========================================
// INLINED: kdco-primitives/log-warn
// ==========================================

function logWarn(
  client: OpencodeClient | undefined,
  service: string,
  message: string,
): void {
  if (!client) {
    console.warn(`[${service}] ${message}`)
    return
  }
  client.app.log({ body: { service, level: "warn", message } }).catch(() => {})
}

// ==========================================
// INLINED: kdco-primitives/get-project-id
// ==========================================

function hashPath(projectRoot: string): string {
  const hash = crypto.createHash("sha256").update(projectRoot).digest("hex")
  return hash.slice(0, 16)
}

async function getProjectId(projectRoot: string): Promise<string> {
  if (!projectRoot || typeof projectRoot !== "string") {
    throw new Error("getProjectId: projectRoot is required and must be a string")
  }
  const gitPath = path.join(projectRoot, ".git")
  const gitStat = await stat(gitPath).catch(() => null)
  if (!gitStat) return hashPath(projectRoot)

  let gitDir = gitPath
  if (gitStat.isFile()) {
    const content = await Bun.file(gitPath).text()
    const match = content.match(/^gitdir:\s*(.+)$/m)
    if (!match)
      throw new Error(`getProjectId: .git file exists but has invalid format at ${gitPath}`)
    const gitdirPath = match[1].trim()
    const resolvedGitdir = path.resolve(projectRoot, gitdirPath)
    const commondirPath = path.join(resolvedGitdir, "commondir")
    const commondirFile = Bun.file(commondirPath)
    if (await commondirFile.exists()) {
      const commondirContent = (await commondirFile.text()).trim()
      gitDir = path.resolve(resolvedGitdir, commondirContent)
    } else {
      gitDir = path.resolve(resolvedGitdir, "../..")
    }
    const gitDirStat = await stat(gitDir).catch(() => null)
    if (!gitDirStat?.isDirectory())
      throw new Error(`getProjectId: Resolved gitdir ${gitDir} is not a directory`)
  }

  const cacheFile = path.join(gitDir, "opencode")
  const cache = Bun.file(cacheFile)
  if (await cache.exists()) {
    const cached = (await cache.text()).trim()
    if (/^[a-f0-9]{40}$/i.test(cached) || /^[a-f0-9]{16}$/i.test(cached)) return cached
  }

  try {
    const proc = Bun.spawn(["git", "rev-list", "--max-parents=0", "--all"], {
      cwd: projectRoot,
      stdout: "pipe",
      stderr: "pipe",
      env: { ...process.env, GIT_DIR: undefined, GIT_WORK_TREE: undefined },
    })
    const exitCode = await withTimeout(proc.exited, 5000, "git rev-list timed out").catch((e) => {
      if (e instanceof TimeoutError) proc.kill()
      return 1
    })
    if (exitCode === 0) {
      const output = await new Response(proc.stdout).text()
      const roots = output
        .split("\n")
        .filter(Boolean)
        .map((x) => x.trim())
        .sort()
      if (roots.length > 0 && /^[a-f0-9]{40}$/i.test(roots[0])) {
        const projectId = roots[0]
        try {
          await Bun.write(cacheFile, projectId)
        } catch {}
        return projectId
      }
    }
  } catch {}
  return hashPath(projectRoot)
}

// ==========================================
// READABLE ID GENERATION
// ==========================================

function generateReadableId(): string {
  return uniqueNamesGenerator({
    dictionaries: [adjectives, colors, animals],
    separator: "-",
    length: 3,
    style: "lowerCase",
  })
}

// ==========================================
// METADATA GENERATION (using small_model)
// ==========================================

interface GeneratedMetadata {
  title: string
  description: string
}

/**
 * Generate title and description from result content using small_model
 * Falls back to truncation if small_model unavailable
 */
async function generateMetadata(
  client: OpencodeClient,
  resultContent: string,
  parentID: string,
  debugLog: (msg: string) => Promise<void>,
): Promise<GeneratedMetadata> {
  const fallbackMetadata = (): GeneratedMetadata => {
    // Fallback: truncate first line/paragraph
    const firstLine =
      resultContent.split("\n").find((l) => l.trim().length > 0) || "Delegation result"
    const title = firstLine.slice(0, 30).trim() + (firstLine.length > 30 ? "..." : "")
    const description =
      resultContent.slice(0, 150).trim() + (resultContent.length > 150 ? "..." : "")
    return { title, description }
  }

  try {
    // Get config to check for small_model
    const config = await client.config.get()
    const configData = config.data as { small_model?: string } | undefined

    if (!configData?.small_model) {
      await debugLog("generateMetadata: No small_model configured, using fallback")
      return fallbackMetadata()
    }

    await debugLog(`generateMetadata: Using small_model ${configData.small_model}`)

    // Create a session for metadata generation
    const session = await client.session.create({
      body: {
        title: "Metadata Generation",
        parentID,
      },
    })

    if (!session.data?.id) {
      await debugLog("generateMetadata: Failed to create session")
      return fallbackMetadata()
    }

    // Prompt the small model for metadata
    const prompt = `Generate a title and description for this research result.

RULES:
- Title: 2-5 words, max 30 characters, sentence case
- Description: 2-3 sentences, max 150 characters, summarize key findings

RESULT CONTENT:
${resultContent.slice(0, 2000)}

Respond with ONLY valid JSON in this exact format:
{"title": "Your Title Here", "description": "Your description here."}`

    // Await prompt response directly with timeout safety net
    const PROMPT_TIMEOUT_MS = 30000
    const result = await Promise.race([
      client.session.prompt({
        path: { id: session.data.id },
        body: {
          parts: [{ type: "text", text: prompt }],
        },
      }),
      new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error("Prompt timeout after 30s")), PROMPT_TIMEOUT_MS),
      ),
    ])

    // Extract text from the response
    const responseParts = result.data?.parts as TextPart[] | undefined
    const textPart = responseParts?.find((p): p is TextPart => p.type === "text")
    if (!textPart) {
      await debugLog("generateMetadata: No text part in response")
      return fallbackMetadata()
    }

    // Parse JSON response
    const jsonMatch = textPart.text.match(/\{[\s\S]*\}/)
    if (!jsonMatch) {
      await debugLog(`generateMetadata: No JSON found in response: ${textPart.text}`)
      return fallbackMetadata()
    }

    const parsed = JSON.parse(jsonMatch[0]) as { title?: string; description?: string }
    if (!parsed.title || !parsed.description) {
      await debugLog("generateMetadata: Invalid JSON structure")
      return fallbackMetadata()
    }

    await debugLog(`generateMetadata: Generated title="${parsed.title}"`)
    return {
      title: parsed.title.slice(0, 30),
      description: parsed.description.slice(0, 150),
    }
  } catch (error) {
    await debugLog(
      `generateMetadata error: ${error instanceof Error ? error.message : "Unknown error"}`,
    )
    return fallbackMetadata()
  }
}

// ==========================================
// TYPE DEFINITIONS
// ==========================================

interface SessionMessageItem {
  info: Message
  parts: Part[]
}

interface AssistantSessionMessageItem {
  info: Message & { role: "assistant" }
  parts: Part[]
}

interface DelegationProgress {
  toolCalls: number
  lastUpdate: Date
  lastMessage?: string
  lastMessageAt?: Date
}

const MAX_RUN_TIME_MS = 15 * 60 * 1000 // 15 minutes

interface Delegation {
  id: string // Human-readable ID (e.g., "swift-amber-falcon")
  sessionID: string
  parentSessionID: string
  parentMessageID: string
  parentAgent: string
  prompt: string
  agent: string
  status: "running" | "complete" | "error" | "cancelled" | "timeout"
  startedAt: Date
  completedAt?: Date
  progress: DelegationProgress
  error?: string
  // Generated on completion by small_model
  title?: string
  description?: string
  result?: string
}

interface DelegateInput {
  parentSessionID: string
  parentMessageID: string
  parentAgent: string
  prompt: string
  agent: string
}

interface DelegationListItem {
  id: string
  status: string
  title?: string
  description?: string
  agent?: string
}

// ==========================================
// LOGGING HELPER
// ==========================================

/**
 * Create a structured logger that sends messages to OpenCode's log API.
 * Catches errors silently to avoid disrupting tool execution.
 */
function createLogger(client: OpencodeClient) {
  const log = (level: "debug" | "info" | "warn" | "error", message: string) =>
    client.app.log({ body: { service: "background-agents", level, message } }).catch(() => {})
  return {
    debug: (msg: string) => log("debug", msg),
    info: (msg: string) => log("info", msg),
    warn: (msg: string) => log("warn", msg),
    error: (msg: string) => log("error", msg),
  }
}

type Logger = ReturnType<typeof createLogger>

// ==========================================
// AGENT CAPABILITY DETECTION
// ==========================================

/**
 * Parse agent mode at boundary.
 * Returns trusted type indicating if agent is a sub-agent.
 */
async function parseAgentMode(
  client: OpencodeClient,
  agentName: string,
  log: Logger,
): Promise<{ isSubAgent: boolean }> {
  try {
    const result = await client.app.agents({})
    const agents = (result.data ?? []) as { name: string; mode?: string }[]
    const agent = agents.find((a) => a.name === agentName)
    return { isSubAgent: agent?.mode === "subagent" }
  } catch (error) {
    // Fail-safe: Agent list errors shouldn't block task calls
    // Fail-loud: Log for observability
    log.warn(
      `Agent list fetch failed for "${agentName}", assuming non-sub-agent: ${error instanceof Error ? error.message : String(error)}`,
    )
    return { isSubAgent: false }
  }
}

/**
 * Permission entry type: simple value or pattern object.
 * Matches CLI schema: z.union([z.enum(["ask", "allow", "deny"]), z.record(z.enum(...))])
 */
type PermissionEntry = "ask" | "allow" | "deny" | Record<string, "ask" | "allow" | "deny">

/**
 * Check if a permission entry denies access (Law 4: Fail Fast).
 * Handles both simple values ("deny") and pattern objects ({ "*": "deny" }).
 */
function isPermissionDenied(entry: PermissionEntry | undefined): boolean {
  if (entry === undefined) return false
  if (entry === "deny") return true
  if (typeof entry === "object" && entry["*"] === "deny") return true
  return false
}

/**
 * Parse agent write capability at boundary.
 * Returns trusted type indicating if agent is read-only.
 *
 * An agent is read-only when ALL of: edit, write, and bash are denied.
 * Permission schema supports both simple ("deny") and pattern ({ "*": "deny" }) values.
 */
async function parseAgentWriteCapability(
  client: OpencodeClient,
  agentName: string,
  log: Logger,
): Promise<{ isReadOnly: boolean }> {
  try {
    const config = await client.config.get()
    const configData = config.data as {
      agent?: Record<
        string,
        {
          permission?: Record<string, PermissionEntry>
        }
      >
    }
    const permission = configData?.agent?.[agentName]?.permission ?? {}

    const editDenied = isPermissionDenied(permission.edit)
    const writeDenied = isPermissionDenied(permission.write)
    const bashDenied = isPermissionDenied(permission.bash)

    return { isReadOnly: editDenied && writeDenied && bashDenied }
  } catch (error) {
    // Fail-safe: Config errors shouldn't block task calls
    // Fail-loud: Log for observability
    log.warn(
      `Config fetch failed for "${agentName}", assuming write-capable: ${error instanceof Error ? error.message : String(error)}`,
    )
    return { isReadOnly: false }
  }
}

/**
 * Resolve the model configured for a given agent.
 * Parses the "provider/model-id" format into the shape session.prompt() expects.
 * Returns undefined when no model is configured (caller falls back to default).
 */
async function resolveAgentModel(
  client: OpencodeClient,
  agentName: string,
  log: Logger,
): Promise<{ providerID: string; modelID: string } | undefined> {
  try {
    const config = await client.config.get()
    const configData = config.data as {
      agent?: Record<string, { model?: string }>
    } | undefined

    const modelStr = configData?.agent?.[agentName]?.model
    if (!modelStr) return undefined

    const slashIndex = modelStr.indexOf("/")
    if (slashIndex === -1) return undefined

    const providerID = modelStr.substring(0, slashIndex)
    const modelID = modelStr.substring(slashIndex + 1)

    await log.info(`resolveAgentModel: ${agentName} → ${providerID}/${modelID}`)
    return { providerID, modelID }
  } catch {
    return undefined
  }
}

/**
 * DELEGATION MANAGER
 */
class DelegationManager {
  private delegations: Map<string, Delegation> = new Map()
  private client: OpencodeClient
  private baseDir: string
  private log: Logger
  // Track pending delegations per parent session for batched notifications
  private pendingByParent: Map<string, Set<string>> = new Map()

  constructor(client: OpencodeClient, baseDir: string, log: Logger) {
    this.client = client
    this.baseDir = baseDir
    this.log = log
  }

  /**
   * Resolves the root session ID by walking up the parent chain.
   */
  async getRootSessionID(sessionID: string): Promise<string> {
    let currentID = sessionID
    // Prevent infinite loops with max depth
    for (let depth = 0; depth < 10; depth++) {
      try {
        const session = await this.client.session.get({
          path: { id: currentID },
        })

        if (!session.data?.parentID) {
          return currentID
        }

        currentID = session.data.parentID
      } catch {
        // If we can't fetch the session, assume current is root or best effort
        return currentID
      }
    }
    return currentID
  }

  /**
   * Get the delegations directory for a session scope (root session)
   */
  private async getDelegationsDir(sessionID: string): Promise<string> {
    const rootID = await this.getRootSessionID(sessionID)
    return path.join(this.baseDir, rootID)
  }

  /**
   * Ensure the delegations directory exists
   */
  private async ensureDelegationsDir(sessionID: string): Promise<string> {
    const dir = await this.getDelegationsDir(sessionID)
    await fs.mkdir(dir, { recursive: true })
    return dir
  }

  /**
   * Delegate a task to an agent
   */
  async delegate(input: DelegateInput): Promise<Delegation> {
    // Generate readable ID
    const id = generateReadableId()
    await this.debugLog(`delegate() called, generated ID: ${id}`)

    // Check for ID collisions (regenerate if needed)
    let finalId = id
    let attempts = 0
    while (this.delegations.has(finalId) && attempts < 10) {
      finalId = generateReadableId()
      attempts++
    }
    if (this.delegations.has(finalId)) {
      throw new Error("Failed to generate unique delegation ID after 10 attempts")
    }

    // Validate agent exists before creating session
    const agentsResult = await this.client.app.agents({})
    const agents = (agentsResult.data ?? []) as {
      name: string
      description?: string
      mode?: string
    }[]
    const validAgent = agents.find((a) => a.name === input.agent)

    if (!validAgent) {
      const available = agents
        .filter((a) => a.mode === "subagent" || a.mode === "all" || !a.mode)
        .map((a) => `• ${a.name}${a.description ? ` - ${a.description}` : ""}`)
        .join("\n")

      throw new Error(
        `Agent "${input.agent}" not found.\n\nAvailable agents:\n${available || "(none)"}`,
      )
    }

    // NOTE: Read-only restriction removed — any sub-agent can use delegate.
    // Background delegations run in isolated sessions outside OpenCode's session tree.
    // The undo/branching system cannot track changes made in background sessions.
    // This is an accepted tradeoff for the ability to run sub-agents in parallel.

    // Create isolated session for delegation
    const sessionResult = await this.client.session.create({
      body: {
        title: `Delegation: ${finalId}`,
        parentID: input.parentSessionID,
      },
    })

    await this.debugLog(`session.create result: ${JSON.stringify(sessionResult.data)}`)

    if (!sessionResult.data?.id) {
      throw new Error("Failed to create delegation session")
    }

    const delegation: Delegation = {
      id: finalId,
      sessionID: sessionResult.data.id,
      parentSessionID: input.parentSessionID,
      parentMessageID: input.parentMessageID,
      parentAgent: input.parentAgent,
      prompt: input.prompt,
      agent: input.agent,
      status: "running",
      startedAt: new Date(),
      progress: {
        toolCalls: 0,
        lastUpdate: new Date(),
      },
    }

    await this.debugLog(`Created delegation ${delegation.id}`)
    this.delegations.set(delegation.id, delegation)

    // Track this delegation for batched notification
    const parentId = input.parentSessionID
    if (!this.pendingByParent.has(parentId)) {
      this.pendingByParent.set(parentId, new Set())
    }
    this.pendingByParent.get(parentId)?.add(delegation.id)
    await this.debugLog(
      `Tracking delegation ${delegation.id} for parent ${parentId}. Pending count: ${this.pendingByParent.get(parentId)?.size}`,
    )

    await this.debugLog(
      `Delegation added to map. Current delegations: ${Array.from(this.delegations.keys()).join(", ")}`,
    )

    // Set a timer for the global max run time
    setTimeout(() => {
      const current = this.delegations.get(delegation.id)
      if (current && current.status === "running") {
        this.handleTimeout(delegation.id)
      }
    }, MAX_RUN_TIME_MS + 5000) // Adding 5s buffer

    // Ensure delegations directory exists (early check)
    await this.ensureDelegationsDir(input.parentSessionID)

    // Resolve the agent's configured model so sub-agents use their own model, not the orchestrator's
    const agentModel = await resolveAgentModel(this.client, input.agent, this.log)

    // Fire the prompt (using prompt() instead of promptAsync() to properly initialize agent loop)
    // Agent param is critical for MCP tools - tells OpenCode which agent's config to use
    // Anti-recursion: disable nested delegations and state-modifying tools via tools config
    this.client.session
      .prompt({
        path: { id: delegation.sessionID },
        body: {
          agent: input.agent,
          ...(agentModel && { model: agentModel }),
          parts: [{ type: "text", text: input.prompt }],
          tools: {
            task: false,
            delegate: false,
            todowrite: false,
            plan_save: false,
          },
        },
      })
      .catch((error: Error) => {
        delegation.status = "error"
        delegation.error = error.message
        delegation.completedAt = new Date()
        this.persistOutput(delegation, `Error: ${error.message}`)
        this.notifyParent(delegation)
      })

    return delegation
  }

  /**
   * Handle delegation timeout
   */
  private async handleTimeout(delegationId: string): Promise<void> {
    const delegation = this.delegations.get(delegationId)
    if (!delegation || delegation.status !== "running") return

    await this.debugLog(`handleTimeout for delegation ${delegation.id}`)

    delegation.status = "timeout"
    delegation.completedAt = new Date()
    delegation.error = `Delegation timed out after ${MAX_RUN_TIME_MS / 1000}s`

    // Try to cancel the session
    try {
      await this.client.session.delete({
        path: { id: delegation.sessionID },
      })
    } catch {
      // Ignore
    }

    // Get whatever result was produced so far
    const result = await this.getResult(delegation)
    await this.persistOutput(delegation, `${result}\n\n[TIMEOUT REACHED]`)

    // Notify parent session
    await this.notifyParent(delegation)
  }

  /**
   * Wait for a delegation to complete (polling)
   */
  private async waitForCompletion(delegationId: string): Promise<void> {
    const pollInterval = 1000
    const startTime = Date.now()

    const delegation = this.delegations.get(delegationId)
    if (!delegation) return

    while (
      delegation.status === "running" &&
      Date.now() - startTime < MAX_RUN_TIME_MS + 10000 // Slightly more than global limit
    ) {
      await new Promise((resolve) => setTimeout(resolve, pollInterval))
    }
  }

  /**
   * Handle session.idle event - called when a session becomes idle
   */
  async handleSessionIdle(sessionID: string): Promise<void> {
    const delegation = this.findBySession(sessionID)
    if (!delegation || delegation.status !== "running") return

    await this.debugLog(`handleSessionIdle for delegation ${delegation.id}`)

    delegation.status = "complete"
    delegation.completedAt = new Date()

    // Get the result
    const result = await this.getResult(delegation)
    delegation.result = result

    // Generate title and description using small model
    const metadata = await generateMetadata(
      this.client,
      result,
      delegation.sessionID,
      (msg) => this.debugLog(msg),
    )
    delegation.title = metadata.title
    delegation.description = metadata.description

    // Persist output with generated metadata
    await this.persistOutput(delegation, result)

    // Notify parent session
    await this.notifyParent(delegation)
  }

  /**
   * Get the result from a delegation's session
   */
  private async getResult(delegation: Delegation): Promise<string> {
    try {
      const messages = await this.client.session.messages({
        path: { id: delegation.sessionID },
      })

      const messageData = messages.data as SessionMessageItem[] | undefined

      if (!messageData || messageData.length === 0) {
        await this.debugLog(`getResult: No messages found for session ${delegation.sessionID}`)
        return `Delegation "${delegation.description}" completed but produced no output.`
      }

      await this.debugLog(
        `getResult: Found ${messageData.length} messages. Roles: ${messageData.map((m) => m.info.role).join(", ")}`,
      )

      // Find the last message from the assistant/model
      const isAssistantMessage = (m: SessionMessageItem): m is AssistantSessionMessageItem =>
        m.info.role === "assistant"

      const assistantMessages = messageData.filter(isAssistantMessage)

      if (assistantMessages.length === 0) {
        await this.debugLog(
          `getResult: No assistant messages found in ${JSON.stringify(messageData.map((m) => ({ role: m.info.role, keys: Object.keys(m) })))}`,
        )
        return `Delegation "${delegation.description}" completed but produced no assistant response.`
      }

      const lastMessage = assistantMessages[assistantMessages.length - 1]

      // Extract text parts from the message
      const isTextPart = (p: Part): p is TextPart => p.type === "text"
      const textParts = lastMessage.parts.filter(isTextPart)

      if (textParts.length === 0) {
        await this.debugLog(
          `getResult: No text parts found in message: ${JSON.stringify(lastMessage)}`,
        )
        return `Delegation "${delegation.description}" completed but produced no text content.`
      }

      return textParts.map((p) => p.text).join("\n")
    } catch (error) {
      await this.debugLog(
        `getResult error: ${error instanceof Error ? error.message : "Unknown error"}`,
      )
      return `Delegation "${delegation.description}" completed but result could not be retrieved: ${
        error instanceof Error ? error.message : "Unknown error"
      }`
    }
  }

  /**
   * Persist delegation output to storage
   */
  private async persistOutput(delegation: Delegation, content: string): Promise<void> {
    try {
      // Ensure we resolve the root session ID of the PARENT session for storage
      const dir = await this.ensureDelegationsDir(delegation.parentSessionID)
      const filePath = path.join(dir, `${delegation.id}.md`)

      // Use title/description if available (generated by small model), otherwise fallback
      const title = delegation.title || delegation.id
      const description = delegation.description || "(No description generated)"

      const header = `# ${title}

${description}

**ID:** ${delegation.id}
**Agent:** ${delegation.agent}
**Status:** ${delegation.status}
**Started:** ${delegation.startedAt.toISOString()}
**Completed:** ${delegation.completedAt?.toISOString() || "N/A"}

---

`
      await fs.writeFile(filePath, header + content, "utf8")
      await this.debugLog(`Persisted output to ${filePath}`)
    } catch (error) {
      await this.debugLog(
        `Failed to persist output: ${error instanceof Error ? error.message : "Unknown error"}`,
      )
    }
  }

  /**
   * Notify parent session that delegation is complete.
   * Uses batching: individual notifications are silent (noReply: true),
   * but when ALL delegations for a parent session complete, triggers a response.
   */
  private async notifyParent(delegation: Delegation): Promise<void> {
    try {
      const statusText = delegation.status === "complete" ? "complete" : delegation.status

      // Mark this delegation as complete in the pending tracker
      const pendingSet = this.pendingByParent.get(delegation.parentSessionID)
      if (pendingSet) {
        pendingSet.delete(delegation.id)
      }

      // Check if ALL delegations for this parent are now complete
      const allComplete = !pendingSet || pendingSet.size === 0

      // Clean up if all complete
      if (allComplete && pendingSet) {
        this.pendingByParent.delete(delegation.parentSessionID)
      }

      // Always send the completed delegation notification first (compact — full result is on disk)
      const completionNotification = `[TASK NOTIFICATION]
ID: ${delegation.id}
Status: ${statusText}
Use delegation_read(id) to retrieve the full result.`

      await this.client.session.prompt({
        path: { id: delegation.parentSessionID },
        body: {
          noReply: true,
          agent: delegation.parentAgent,
          parts: [{ type: "text", text: completionNotification }],
        },
      })

      // If all delegations complete, send a minimal completion notice that triggers response
      if (allComplete) {
        const allCompleteNotification = `[TASK NOTIFICATION] All delegations complete.`

        await this.client.session.prompt({
          path: { id: delegation.parentSessionID },
          body: {
            noReply: false,
            agent: delegation.parentAgent,
            parts: [{ type: "text", text: allCompleteNotification }],
          },
        })
      }

      await this.debugLog(
        `Notified parent session ${delegation.parentSessionID} (allComplete=${allComplete}, remaining=${pendingSet?.size || 0})`,
      )
    } catch (error) {
      await this.debugLog(
        `Failed to notify parent: ${error instanceof Error ? error.message : "Unknown error"}`,
      )
    }
  }

  /**
   * Read a delegation's output by ID. Blocks if the delegation is still running.
   */
  async readOutput(sessionID: string, id: string): Promise<string> {
    // Try to find the file
    let filePath: string | undefined
    try {
      const dir = await this.getDelegationsDir(sessionID)
      filePath = path.join(dir, `${id}.md`)
      // Check if file exists
      await fs.access(filePath)
      return await fs.readFile(filePath, "utf8")
    } catch {
      // File doesn't exist yet, continue to check memory
    }

    // Check if it's currently running in memory
    const delegation = this.delegations.get(id)
    if (delegation) {
      if (delegation.status === "running") {
        await this.debugLog(`readOutput: waiting for delegation ${delegation.id} to complete`)
        await this.waitForCompletion(delegation.id)

        // Re-check after waiting
        const dir = await this.getDelegationsDir(sessionID)
        filePath = path.join(dir, `${id}.md`)
        try {
          return await fs.readFile(filePath, "utf8")
        } catch {
          // Still failed to read
        }

        // If still no file after waiting (e.g. error/timeout/cancel)
        const updated = this.delegations.get(id)
        if (updated && updated.status !== "running") {
          const title = updated.title || updated.id
          return `Delegation "${title}" ended with status: ${updated.status}. ${updated.error || ""}`
        }
      }
    }

    throw new Error(
      `Delegation "${id}" not found.\n\nUse delegation_list() to see available delegations.`,
    )
  }

  /**
   * List all delegations for a session
   */
  async listDelegations(sessionID: string): Promise<DelegationListItem[]> {
    const results: DelegationListItem[] = []

    // Add in-memory delegations that match this session (or parent)
    for (const delegation of this.delegations.values()) {
      results.push({
        id: delegation.id,
        status: delegation.status,
        title: delegation.title || "(generating...)",
        description: delegation.description || "(generating...)",
      })
    }

    // Check filesystem for persisted delegations
    try {
      const dir = await this.getDelegationsDir(sessionID)
      const files = await fs.readdir(dir)

      for (const file of files) {
        if (file.endsWith(".md")) {
          const id = file.replace(".md", "")
          // Deduplicate: prioritize in-memory status
          if (!results.find((r) => r.id === id)) {
            // Try to read title, agent, description from file
            let title = "(loaded from storage)"
            let description = ""
            let agent: string | undefined
            try {
              const filePath = path.join(dir, file)
              const content = await fs.readFile(filePath, "utf8")
              const titleMatch = content.match(/^# (.+)$/m)
              if (titleMatch) title = titleMatch[1]
              const agentMatch = content.match(/^\*\*Agent:\*\* (.+)$/m)
              if (agentMatch) agent = agentMatch[1]
              // Get first paragraph after title as description
              const lines = content.split("\n")
              if (lines.length > 2 && lines[2]) {
                description = lines[2].slice(0, 150)
              }
            } catch {
              // Ignore read errors
            }
            results.push({
              id,
              status: "complete",
              title,
              description,
              agent,
            })
          }
        }
      }
    } catch {
      // Directory may not exist yet
    }

    return results
  }

  /**
   * Delete a delegation by id (cancels if running, removes from storage)
   * Used internally for cleanup (timeout, etc.)
   */
  async deleteDelegation(sessionID: string, id: string): Promise<boolean> {
    // Find delegation by id
    let delegationId: string | undefined
    for (const [dId, d] of this.delegations) {
      if (d.id === id) {
        delegationId = dId
        break
      }
    }

    if (delegationId) {
      const delegation = this.delegations.get(delegationId)
      if (delegation?.status === "running") {
        try {
          await this.client.session.delete({
            path: { id: delegation.sessionID },
          })
        } catch {
          // Session may already be deleted
        }
        delegation.status = "cancelled"
        delegation.completedAt = new Date()
      }
      this.delegations.delete(delegationId)
    }

    // Remove from filesystem
    try {
      const dir = await this.getDelegationsDir(sessionID)
      const filePath = path.join(dir, `${id}.md`)
      await fs.unlink(filePath)
      return true
    } catch {
      return false
    }
  }

  /**
   * Find a delegation by its session ID
   */
  findBySession(sessionID: string): Delegation | undefined {
    return Array.from(this.delegations.values()).find((d) => d.sessionID === sessionID)
  }

  /**
   * Handle message events for progress tracking
   */
  handleMessageEvent(sessionID: string, messageText?: string): void {
    const delegation = this.findBySession(sessionID)
    if (!delegation || delegation.status !== "running") return

    delegation.progress.lastUpdate = new Date()
    if (messageText) {
      delegation.progress.lastMessage = messageText
      delegation.progress.lastMessageAt = new Date()
    }
  }

  /**
   * Get count of pending delegations for a parent session
   */
  getPendingCount(parentSessionID: string): number {
    const pendingSet = this.pendingByParent.get(parentSessionID)
    return pendingSet ? pendingSet.size : 0
  }

  /**
   * Get all currently running delegations (in-memory only)
   */
  getRunningDelegations(): Delegation[] {
    return Array.from(this.delegations.values()).filter((d) => d.status === "running")
  }

  /**
   * Get recent completed delegations for compaction injection
   */
  async getRecentCompletedDelegations(
    sessionID: string,
    limit: number = 10,
  ): Promise<DelegationListItem[]> {
    const all = await this.listDelegations(sessionID)
    return all.filter((d) => d.status !== "running").slice(-limit)
  }

  /**
   * Log debug messages
   */
  async debugLog(msg: string): Promise<void> {
    // Only log if debug is enabled (could be env var or static const)
    // For now, mirroring previous behavior but writing to the new baseDir/debug.log
    const timestamp = new Date().toISOString()
    const line = `${timestamp}: ${msg}\n`
    const debugFile = path.join(this.baseDir, "background-agents-debug.log")

    try {
      await fs.appendFile(debugFile, line, "utf8")
    } catch {
      // Ignore errors, try to ensure dir once if it fails?
      // Simpler to just ignore for debug logs
    }
  }
}

// ==========================================
// TOOL CREATORS
// ==========================================

interface DelegateArgs {
  prompt: string
  agent: string
}

function createDelegate(manager: DelegationManager): ReturnType<typeof tool> {
  return tool({
    description: `Delegate a task to an agent. Returns immediately with a readable ID.

Use this for:
- Research tasks (will be auto-saved)
- Parallel work that can run in background
- Any task where you want persistent, retrievable output

On completion, a compact notification arrives with the ID and status only.
Use \`delegation_read(id)\` to retrieve the full result. Results are persisted to disk and survive compaction.`,
    args: {
      prompt: tool.schema
        .string()
        .describe("The full detailed prompt for the agent. Must be in English."),
      agent: tool.schema
        .string()
        .describe(
          'Agent to delegate to: "explore" (codebase search), "researcher" (external research), "scribe" (docs/commits), or "general".',
        ),
    },
    async execute(args: DelegateArgs, toolCtx: ToolContext): Promise<string> {
      if (!toolCtx?.sessionID) {
        return "❌ delegate requires sessionID. This is a system error."
      }
      if (!toolCtx?.messageID) {
        return "❌ delegate requires messageID. This is a system error."
      }

      try {
        const delegation = await manager.delegate({
          parentSessionID: toolCtx.sessionID,
          parentMessageID: toolCtx.messageID,
          parentAgent: toolCtx.agent,
          prompt: args.prompt,
          agent: args.agent,
        })

        // Get total active count for this parent session
        const pendingSet = manager.getPendingCount(toolCtx.sessionID)
        const totalActive = pendingSet

        let response = `Delegation started: ${delegation.id}\nAgent: ${args.agent}`
        if (totalActive > 1) {
          response += `\n\n${totalActive} delegations now active.`
        }
        response += `\nYou WILL be notified when ${totalActive > 1 ? "ALL complete" : "complete"}. Do NOT poll.`

        return response
      } catch (error) {
        // Return validation errors as guidance, not exceptions
        return `❌ Delegation failed:\n\n${error instanceof Error ? error.message : "Unknown error"}`
      }
    },
  })
}

function createDelegationRead(manager: DelegationManager): ReturnType<typeof tool> {
  return tool({
    description: `Read the output of a delegation by its ID.
Use this to retrieve results from delegated tasks if the inline notification was lost during compaction.`,
    args: {
      id: tool.schema.string().describe("The delegation ID (e.g., 'elegant-blue-tiger')"),
    },
    async execute(args: { id: string }, toolCtx: ToolContext): Promise<string> {
      if (!toolCtx?.sessionID) {
        return "❌ delegation_read requires sessionID. This is a system error."
      }

      return await manager.readOutput(toolCtx.sessionID, args.id)
    },
  })
}

function createDelegationList(manager: DelegationManager): ReturnType<typeof tool> {
  return tool({
    description: `List all delegations for the current session.
Shows both running and completed delegations.`,
    args: {},
    async execute(_args: Record<string, never>, toolCtx: ToolContext): Promise<string> {
      if (!toolCtx?.sessionID) {
        return "❌ delegation_list requires sessionID. This is a system error."
      }

      const delegations = await manager.listDelegations(toolCtx.sessionID)

      if (delegations.length === 0) {
        return "No delegations found for this session."
      }

      const lines = delegations.map((d) => {
        const titlePart = d.title ? ` | ${d.title}` : ""
        const descPart = d.description ? `\n  → ${d.description}` : ""
        return `- **${d.id}**${titlePart} [${d.status}]${descPart}`
      })

      return `## Delegations\n\n${lines.join("\n")}`
    },
  })
}

// ==========================================
// DELEGATION RULES (injected into system prompt)
// ==========================================

const DELEGATION_RULES = `<task-notification>
<delegation-system>

## Async Background Delegation

You have tools for parallel background work:
- \`delegate(prompt, agent)\` - Launch background task, returns ID immediately
- \`delegation_read(id)\` - Retrieve completed result
- \`delegation_list()\` - List delegations (use sparingly)

## When to Use delegate vs task

| Tool | Behavior | Use When |
|------|----------|----------|
| \`delegate\` | Async, background, persisted to disk | You want to continue working while it runs |
| \`task\` | Synchronous, blocks until complete | You need the result before continuing |

Any agent can be used with \`delegate\`. Results survive context compaction.

## How It Works

1. Call \`delegate(prompt, agent)\` with a detailed prompt and agent name
2. Continue productive work while it runs in the background
3. Receive a \`<task-notification>\` when complete (compact: ID + status only)
4. Use \`delegation_read(id)\` to retrieve the full result when needed

## Critical Constraints

**NEVER poll \`delegation_list\` to check completion.**
You WILL be notified via \`<task-notification>\`. Polling wastes tokens.

**NEVER wait idle.** Always have productive work while delegations run.

**NOTE:** Background delegations run in isolated sessions. Changes made by write-capable
agents in background sessions are NOT tracked by OpenCode's undo/branching system.

</delegation-system>
</task-notification>`

// ==========================================
// COMPACTION CONTEXT FORMATTING
// ==========================================

interface DelegationForContext {
  id: string
  agent?: string
  title?: string
  description?: string
  status: string
  startedAt?: Date
  prompt?: string
}

/**
 * Format delegation context for injection during compaction.
 * Includes running delegations with notification reminder (only when running exist),
 * and recent completed delegations with full descriptions.
 */
function formatDelegationContext(
  running: DelegationForContext[],
  completed: DelegationForContext[],
): string {
  const sections: string[] = ["<delegation-context>"]

  // Running delegations (if any)
  if (running.length > 0) {
    sections.push("## Running Delegations")
    sections.push("")
    for (const d of running) {
      sections.push(`### \`${d.id}\`${d.agent ? ` (${d.agent})` : ""}`)
      if (d.startedAt) {
        sections.push(`**Started:** ${d.startedAt.toISOString()}`)
      }
      if (d.prompt) {
        const truncatedPrompt = d.prompt.length > 200 ? `${d.prompt.slice(0, 200)}...` : d.prompt
        sections.push(`**Prompt:** ${truncatedPrompt}`)
      }
      sections.push("")
    }

    // Only include reminder when there ARE running delegations
    sections.push(
      "> **Note:** You WILL be notified via a **Task Notification** blockquote when delegations complete.",
    )
    sections.push("> Do NOT poll `delegation_list` - continue productive work.")
    sections.push("")
  }

  // Completed delegations (recent) — compact: just ID + status, full output is on disk
  if (completed.length > 0) {
    sections.push("## Recent Completed Delegations")
    sections.push("")
    for (const d of completed) {
      sections.push(`- \`${d.id}\` [${d.status}]`)
    }
    sections.push("")
    sections.push("> Use `delegation_read(id)` to get full output for any completed delegation.")
    sections.push("")
  }

  sections.push("## Retrieval")
  sections.push('Use `delegation_read("id")` to access full delegation output.')
  sections.push("</delegation-context>")

  return sections.join("\n")
}

// ==========================================
// PLUGIN EXPORT
// ==========================================

/**
 * Expected input for experimental.chat.system.transform hook.
 */
interface SystemTransformInput {
  agent?: string
  sessionID?: string
}

export const BackgroundAgents: Plugin = async (ctx) => {
  const { client, directory } = ctx

  // Create logger early for all components
  const log = createLogger(client as OpencodeClient)

  // Project-level storage directory (shared across sessions)
  // Uses git root commit hash for cross-worktree consistency
  const projectId = await getProjectId(directory)
  const baseDir = path.join(os.homedir(), ".local", "share", "opencode", "delegations", projectId)

  // Ensure base directory exists (for debug logs etc)
  await fs.mkdir(baseDir, { recursive: true })

  const manager = new DelegationManager(client as OpencodeClient, baseDir, log)

  await manager.debugLog("BackgroundAgents initialized with delegation system")

  return {
    tool: {
      delegate: createDelegate(manager),
      delegation_read: createDelegationRead(manager),
      delegation_list: createDelegationList(manager),
    },

    // NOTE: tool.execute.before hook for task/delegate routing removed.
    // All agents can use both `delegate` (background, async, persisted) and `task` (native, synchronous).
    // The agent chooses based on whether it needs async background execution or synchronous results.

    // Inject delegation rules into system prompt
    "experimental.chat.system.transform": async (_input: SystemTransformInput, output) => {
      const combined = [...output.system, DELEGATION_RULES].join("\n\n---\n\n")
      output.system = [combined]
    },

    // Compaction hook - inject delegation context for context recovery
    "experimental.session.compacting": async (
      input: { sessionID: string },
      output: { context: string[]; prompt?: string },
    ) => {
      const rootSessionID = await manager.getRootSessionID(input.sessionID)

      // Get running delegations for this session tree
      const running = manager
        .getRunningDelegations()
        .filter(
          (d) =>
            d.parentSessionID === input.sessionID || d.parentSessionID === rootSessionID,
        )
        .map((d) => ({
          id: d.id,
          agent: d.agent,
          title: d.title,
          description: d.description,
          status: d.status,
          startedAt: d.startedAt,
          prompt: d.prompt,
        }))

      // Get recent completed delegations (last 10)
      const allDelegations = await manager.listDelegations(input.sessionID)
      const completed = allDelegations
        .filter((d) => d.status !== "running")
        .slice(-10)
        .map((d) => ({
          id: d.id,
          agent: d.agent,
          title: d.title,
          description: d.description,
          status: d.status,
        }))

      // Early exit if nothing to inject
      if (running.length === 0 && completed.length === 0) return

      output.context.push(formatDelegationContext(running, completed))
    },

    // Event hook
    event: async ({ event }: { event: Event }): Promise<void> => {
      if (event.type === "session.idle") {
        const sessionID = event.properties.sessionID
        const delegation = manager.findBySession(sessionID)
        if (delegation) {
          await manager.handleSessionIdle(sessionID)
        }
      }

      if (event.type === "message.updated") {
        const sessionID = event.properties.info.sessionID
        if (sessionID) {
          manager.handleMessageEvent(sessionID)
        }
      }
    },
  }
}

export default BackgroundAgents
