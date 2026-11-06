# Published Pi SDK: registration and direct MCP callback

`TestUserContext7FrozenRegistration` uses the Linux-only
`shell_context7_registration` tag inside the existing asserted offline Guest.
It verifies and privately extracts the committed Pi1.0.0 archive, reuses its
read-only stock dependencies, and runs the actual SDK/resource loader.

The real `registerMcpServer` API, built-in MCP factory, `bindExtensions` startup
and `/mcp` command establish connection and tool discovery without a model.
The real registered tool definition then executes `tools/call` and returns the
fixture result. This **direct callback** does not certify the agent permission
pipeline, an installed CLI, or the actual Context7 service.

Controls cover no registration, a same-name disabled Shared config overriding
registration, and disabled built-in MCP. All three make zero server requests.
The Shared agent tree remains unchanged; explicit SDK model/credential/log
options confine initialization to private storage. The connected case records
creation of private `mcp-auth.json`, rather than pretending there were no writes.

**Do not infer that a private registration alone preserves Shared scope.**
Default SDK storage was observed creating `auth.json`, `models-store.json` and
`mcp-auth.json` in the Shared agent directory. The scoped SDK options in this
fixture are not proof that the ordinary CLI exposes the same binding. The MCP
credential-store implementation is imported from its exact checksum-matched
archive module, not a guaranteed package-root export. A production consumer must
resolve those boundaries before Context7 can become an installed Ready claim.

The Go wrapper reuses the test-local Linux process-group cancellation and bounded
pipe-drain helper. The original60-second context and every source assertion stay
intact. This recovery does not execute the SDK, contact Context7, or establish
escaped-descendant containment, agent permission approval or installed readiness.
