# A fresh SDK session cannot qualify MCP permission execution

The actual Pi1.0.0 session rejects public nested tool execution when it has no
assistant message: `No assistant message issued this call`. This occurs before
`tool_call` permission hooks, not because a permission hook approved or denied
an effect. The existing direct MCP callback remains transport evidence only.

## Probe and limits

The private-policy probe uses public `ExtensionRunner.createToolContext()` and
`ExtensionToolContext.executeTool()`. Its `host-nested-probe` ID is explicitly
host-chosen, **not a model-issued parent call**. The discovered direct MCP tool
is callable; public session messages contain no assistant response before or
after the attempt. Startup initialization/discovery traffic is acknowledged.
The SDK's native result has `isError: true`; no additional HTTP or `tools/call`
occurs, and the registered `tool_call`/`tool_result` observers record zero calls.
Their zero counts do not qualify those hooks' functionality or authorization.
The existing direct callback then reaches the fixture without exercising them.

No model response, transcript import, credentials, human confirmation or native
result is fabricated. A genuine model-issued call in the same legitimate
session, or a future documented top-level host-dispatch API, is still required
for positive permission-path qualification. A blocked `tool_call` need not
reach `tool_result`; those events must be checked independently in that future
flow. Actual Context7 service health and installed Ready remain unqualified.

Run the existing private-policy mode of `TestUserContext7FrozenRegistration`
inside the asserted offline Guest. See `shell-context7-manager-text.md` for the
separate same-session presentation readback and its endpoint/schema limits.
