# Capture verbatim Pi manager status in one SDK session

The private-policy probe captures the actual built-in `/mcp` notification using
public `ExtensionRunner.getUIContext()` and `setUIContext(context, "print")`.
It composes a new context from the SDK's public UI context with a recording
`notify`; neither the original UI nor the manager implementation is mutated.
The notification comes from the same session that loaded the source consumer,
connected to the offline fixture and executed its discovered tool callback.

## Checked boundaries

- On reports `context7: connected, 1 tools (direct)` from frozen Pi1.0.0.
- Off and absent contribute no registration; the manager reports no servers.
- A disabled same-name global file wins and names its override source.
- Disabled built-in MCP has no manager notification, despite registration.
- Repeating `/mcp` in the same session returns identical text without new HTTP.
- Shared/malformed policies stop before unsupported SDK store initialization;
  Shared bytes and private policy bytes/inodes/modes/mtimes stay unchanged.

These are exact presentation assertions, **not a production parser or stable
schema**. Normal status omits the endpoint and active source path. This fresh
SDK probe does not inspect an already running CLI/TUI session. Trusted-project
precedence, remote Context7 health, agent permissions, authentic new Gentle SDK
artifacts, installed-platform acceptance and Ready remain unqualified.
The stock CLI's empty print-mode output remains documented separately.

Run the existing private-policy mode of `TestUserContext7FrozenRegistration`
inside the asserted offline Guest. See `shell-context7-private-cli.md` for the
separate CLI probe and `shell-context7-registration.md` for callback boundaries.
