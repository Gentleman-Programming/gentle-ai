import { describe, expect, mock, test } from "bun:test"

mock.module("@opencode-ai/plugin", () => ({
  tool: Object.assign(() => ({}), { schema: { string: () => ({ describe: () => ({}) }) } }),
}))

mock.module("unique-names-generator", () => ({
  default: {
    adjectives: [],
    animals: [],
    colors: [],
    uniqueNamesGenerator: () => "mocked-name",
  },
}))

const { __testables } = await import("./background-agents")

function createClientWithTaskPermission(
  permission: Record<string, "ask" | "allow" | "deny"> | undefined,
) {
  return {
    config: {
      get: async () => ({
        data: {
          agent: {
            "sdd-orchestrator": {
              permission: permission ? { task: permission } : {},
            },
          },
        },
      }),
    },
  }
}

describe("resolvePermissionDecision", () => {
  test("prioritizes exact match over wildcard pattern", () => {
    const decision = __testables.resolvePermissionDecision(
      {
        "*": "deny",
        "sdd-*": "allow",
        "sdd-apply-free": "deny",
      },
      "sdd-apply-free",
    )

    expect(decision).toBe("deny")
  })

  test("matches wildcard patterns", () => {
    const decision = __testables.resolvePermissionDecision(
      {
        "*": "deny",
        "sdd-*-free": "allow",
      },
      "sdd-apply-free",
    )

    expect(decision).toBe("allow")
  })

  test("falls back to star rule when nothing else matches", () => {
    const decision = __testables.resolvePermissionDecision(
      {
        "*": "deny",
      },
      "sdd-apply",
    )

    expect(decision).toBe("deny")
  })
})

describe("getSuggestedAgentName", () => {
  test("suggests the matching default-profile agent", () => {
    const suggestion = __testables.getSuggestedAgentName(
      {
        "*": "deny",
        "sdd-apply": "allow",
      },
      "sdd-apply-free",
    )

    expect(suggestion).toBe("sdd-apply")
  })

  test("suggests the matching suffixed-profile agent", () => {
    const suggestion = __testables.getSuggestedAgentName(
      {
        "*": "deny",
        "sdd-apply-free": "allow",
      },
      "sdd-apply",
    )

    expect(suggestion).toBe("sdd-apply-free")
  })
})

describe("assertDelegationAllowed", () => {
  test("allows delegation when target agent is explicitly allowed", async () => {
    const client = createClientWithTaskPermission({
      "*": "deny",
      "sdd-apply": "allow",
    })

    await expect(
      __testables.assertDelegationAllowed(
        client as never,
        "sdd-orchestrator",
        "sdd-apply",
      ),
    ).resolves.toBeUndefined()
  })

  test("rejects delegation outside the parent routing policy with suggestion", async () => {
    const client = createClientWithTaskPermission({
      "*": "deny",
      "sdd-apply": "allow",
    })

    await expect(
      __testables.assertDelegationAllowed(
        client as never,
        "sdd-orchestrator",
        "sdd-apply-free",
      ),
    ).rejects.toThrow('Try "sdd-apply" instead.')
  })

  test("keeps backward compatibility when parent has no task policy", async () => {
    const client = createClientWithTaskPermission(undefined)

    await expect(
      __testables.assertDelegationAllowed(
        client as never,
        "sdd-orchestrator",
        "sdd-apply-free",
      ),
    ).resolves.toBeUndefined()
  })
})
