import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync, statSync, readdirSync } from "node:fs";
import { createServer } from "node:http";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const [pkg, root] = process.argv.slice(2);
const { createAgentSession, createMcpExtension, DefaultResourceLoader, SessionManager, SettingsManager } = await import(pathToFileURL(join(pkg, "dist/index.js")).href);
let requests = 0;
const methods = [];
const server = createServer(async (request, response) => {
  requests++;
  if (request.method !== "POST") { response.writeHead(405).end(); return; }
  let bytes = "";
  for await (const chunk of request) {
    bytes += chunk;
    if (bytes.length > 65536) { response.writeHead(413).end(); return; }
  }
  const call = JSON.parse(bytes);
  methods.push(call.method);
  if (call.id === undefined) { response.writeHead(204).end(); return; }
  let result = {};
  if (call.method === "initialize") result = { protocolVersion: "2025-11-25", capabilities: { tools: {} }, serverInfo: { name: "private-policy-fixture", version: "1" } };
  if (call.method === "tools/list") result = { tools: [{ name: "fixture_docs", description: "Local fixture, not Context7 service", inputSchema: { type: "object", properties: {} } }] };
  if (call.method === "tools/call") {
    assert.equal(call.params.name, "fixture_docs");
    result = { content: [{ type: "text", text: "PRIVATE_POLICY_CALLBACK_RESULT" }] };
  }
  response.writeHead(200, { "Content-Type": "application/json" });
  response.end(JSON.stringify({ jsonrpc: "2.0", id: call.id, result }));
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const url = `http://127.0.0.1:${server.address().port}`;
function tree(path) {
  return readdirSync(path).sort().map((name) => {
    const full = join(path, name), info = statSync(full);
    return [name, info.mode, info.ino, info.mtimeMs, info.isDirectory() ? tree(full) : readFileSync(full).toString("base64")];
  });
}
try {
  for (const mode of ["on", "off", "absent", "shadowed", "builtin-disabled", "shared-refused", "malformed"]) {
    requests = 0;
    methods.length = 0;
    const base = join(root, mode), target = join(base, "target"), shared = join(base, "shared");
    mkdirSync(base);
    mkdirSync(target);
    mkdirSync(shared);
    for (const name of ["agent", "home", "config", "workspace"]) mkdirSync(join(target, name));
    writeFileSync(join(shared, "historical.txt"), "Shared historical bytes\n", { mode: 0o600 });
    const before = tree(shared), config = join(target, "config");
    const agent = mode === "shared-refused" ? shared : join(target, "agent");
    process.env.PI_CODING_AGENT_DIR = agent;
    process.env.HOME = join(target, "home");
    process.env.GENTLE_PI_CONFIG_HOME = config;
    if (mode !== "absent") {
      writeFileSync(join(config, "mcp.json"), mode === "malformed" ? "{broken" : JSON.stringify({ mcpServers: { context7: { url, exposure: "direct", enabled: mode !== "off" } } }), { mode: 0o600 });
    }
    if (mode === "shadowed") writeFileSync(join(agent, "mcp.json"), JSON.stringify({ mcpServers: { context7: { url, enabled: false } } }), { mode: 0o600 });
    const policyBefore = tree(config);
    const settings = SettingsManager.inMemory(mode === "builtin-disabled" ? { extensions: ["-builtin:mcp"] } : {});
    let api;
    const loader = new DefaultResourceLoader({ cwd: join(target, "workspace"), agentDir: agent, settingsManager: settings, noSkills: true, noPromptTemplates: true, noThemes: true,
      additionalExtensionPaths: ["/sdk/extensions/private-context7.ts"],
      extensionFactories: [
        { name: "mcp", builtin: true, replaceable: true, factory: createMcpExtension() },
        (pi) => { api = pi; },
      ],
    });
    await loader.reload();
    if (mode === "shared-refused" || mode === "malformed") {
      assert.equal(loader.getExtensions().errors.length, 1);
      assert.match(loader.getExtensions().errors[0].error, /private Context7 policy/);
      // Stop before SDK services initialize stores in an unsupported agent dir.
      assert.equal(requests, 0);
    } else {
      assert.deepEqual(loader.getExtensions().errors, []);
      const { session } = await createAgentSession({ cwd: join(target, "workspace"), agentDir: agent, resourceLoader: loader, settingsManager: settings, sessionManager: SessionManager.inMemory(), noTools: "builtin" });
      try {
        const errors = [];
        const notifications = [];
        await session.bindExtensions({ onError(error) { errors.push(error); } });
        const runner = session.extensionRunner;
        const hostUI = runner.getUIContext();
        const originalNotify = hostUI.notify;
        // Compose a new public UI context; do not mutate the host or manager.
        runner.setUIContext({
          ...hostUI,
          notify(message, type) { notifications.push([message, type]); },
        }, "print");
        assert.equal(hostUI.notify, originalNotify, "the original UI remains unchanged");
        if (mode !== "builtin-disabled") await session.prompt("/mcp");
        if (mode === "builtin-disabled") {
          assert.deepEqual(notifications, [], "disabled manager has no status to capture");
          assert.equal(errors.length, 1);
          assert.match(errors[0].error, /no loaded extension connects MCP servers/);
        } else {
          assert.deepEqual(errors, []);
        }
        assert.equal(api.getMcpServers().length, ["on", "shadowed", "builtin-disabled"].includes(mode) ? 1 : 0);
        if (mode !== "builtin-disabled") {
          assert.equal(notifications.length, 1, "capture the same-session manager notification");
          const [message, type] = notifications[0];
          assert.equal(type, "info");
          assert.ok(!message.includes(url), "manager text is not endpoint readback");
          if (mode === "on") assert.equal(message, "context7: connected, 1 tools (direct)");
          if (mode === "off" || mode === "absent") {
            assert.equal(message, `No MCP servers configured. Add them to ${join(agent, "mcp.json")} or .pi/mcp.json.`);
          }
          if (mode === "shadowed") {
            assert.equal(message, `context7: disabled (codemode)\noverridden: "context7" registered by /sdk/extensions/private-context7.ts is overridden by "context7" in ${join(agent, "mcp.json")}`);
          }
          console.log(`FROZEN_PI_MANAGER_TEXT ${JSON.stringify({ mode, message, type })}`);
        }
        const tool = session.getToolDefinition("mcp__context7__fixture_docs");
        if (mode === "on") {
          assert.ok(tool);
          const result = await tool.execute("fixture-call", {}, AbortSignal.timeout(2000), undefined, session.extensionRunner.createContext());
          assert.match(JSON.stringify(result), /PRIVATE_POLICY_CALLBACK_RESULT/);
          assert.ok(methods.includes("initialize") && methods.includes("tools/list") && methods.includes("tools/call"));
          assert.equal(readFileSync(join(agent, "mcp-auth.json"), "utf8"), "{}");
          assert.equal(statSync(join(agent, "mcp-auth.json")).mode & 0o777, 0o600);
        } else {
          assert.equal(tool, undefined);
          assert.equal(requests, 0);
        }
        if (mode !== "builtin-disabled") {
          const beforeQuery = requests;
          const first = notifications[0];
          await session.prompt("/mcp");
          assert.deepEqual(notifications, [first, first], "repeat the same manager status");
          assert.equal(requests, beforeQuery, "status query did not open another connection");
        }
      } finally {
        session.extensionRunner.setUIContext(undefined, "print");
        await session.extensionRunner.emit({ type: "session_shutdown" });
        session.dispose();
      }
    }
    assert.deepEqual(tree(shared), before, "Shared files changed");
    assert.deepEqual(tree(config), policyBefore, "private policy was rewritten");
    console.log(`FROZEN_PI_PRIVATE_POLICY_PASS mode=${mode} HTTP=${requests} RPC=${JSON.stringify(methods)} Shared-preserved=true`);
  }
} finally {
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
