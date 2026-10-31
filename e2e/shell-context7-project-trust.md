# Project MCP precedence under explicitly supplied SDK trust

A disabled same-name project entry wins over the private extension registration
only when the SDK session trusts the project. This probe uses the public
`SettingsManager.inMemory(settings, { projectTrusted })` API on disposable
fixtures. It does **not** claim CLI trust-store consultation or human approval.

## Observed cases

| Supplied SDK trust | Actual manager text | Transport effect |
| --- | --- | --- |
| `true` | Disabled, codemode exposure, project file identified as overriding the registration | Zero HTTP; tool unavailable |
| `false` | Connected, direct exposure; project entry ignored | Initialization/discovery only; no `tools/call` |

Both cases keep the private registration enabled and the private agent's
canonical `mcp.json` absent. A disabled `context7` entry exists only in the
owned workspace's `.pi/mcp.json`. The public session context reports the actual
supplied trust state. Whole project/private-policy/Shared trees preserve
bytes, inodes, modes and mtimes. Repeating `/mcp` changes neither text nor HTTP
count. The original global-shadow case remains a separate control.

The SDK defaults to trusted when this option is omitted; ordinary CLI startup
performs a different trust-resolution flow. Neither fixture branch is a saved
CLI decision, human consent or permission-pipeline qualification. No tool or
model is invoked in these two added cases. Manager text remains presentation,
not a stable endpoint/schema API or installed Ready receipt.

Run the existing private-policy mode of `TestUserContext7FrozenRegistration`
inside the asserted offline Guest. Its exact nine-case observer must pass;
the separate four-case registration and three-case CLI modes remain unchanged.

These recovered assertions remain unexecuted during reconciliation. The nine
private cases reuse the existing bounded wrapper; no fresh model/human grant,
CLI trust-store decision, installed permission execution or Ready is claimed.
