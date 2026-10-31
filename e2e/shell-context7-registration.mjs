import assert from "node:assert/strict";
import { mkdirSync, readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// Published SDK characterization, not the future private-policy consumer.
const [pkg, root] = process.argv.slice(2);
const { createAgentSession, createMcpExtension, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } = await import(pathToFileURL(join(pkg, "dist/index.js")).href);
// Implementations of the public credential-option types in this exact archive.
const { FileAuthStorageBackend } = await import(pathToFileURL(join(pkg, "dist/core/auth-storage.js")).href);
const { McpOAuthCredentialStore } = await import(pathToFileURL(join(pkg, "dist/extensions/mcp/oauth.js")).href);
const methods = [];
let requests = 0;
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
  if (call.method === "initialize") result = { protocolVersion: "2025-11-25", capabilities: { tools: {} }, serverInfo: { name: "offline-registration-fixture", version: "1" } };
  if (call.method === "tools/list") result = { tools: [{ name: "fixture_docs", description: "Local fixture, not Context7", inputSchema: { type: "object", properties: {} } }] };
  if (call.method === "tools/call") {
    assert.equal(call.params.name, "fixture_docs");
    result = { content: [{ type: "text", text: "ACTUAL_MCP_CALLBACK_RESULT" }] };
  }
  response.writeHead(200, { "Content-Type": "application/json" });
  response.end(JSON.stringify({ jsonrpc: "2.0", id: call.id, result }));
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const url = `http://127.0.0.1:${server.address().port}`;
function tree(path) {
  return readdirSync(path).sort().map((name) => {
    const absolute = join(path, name), info = statSync(absolute);
    return [name, info.mode, info.ino, info.mtimeMs, info.isDirectory() ? tree(absolute) : readFileSync(absolute).toString("base64")];
  });
}
try {
  // Prove the observer counts rejected traffic, not only decoded JSON-RPC.
  const rejected = await fetch(url, { method: "GET", signal: AbortSignal.timeout(2000) });
  assert.equal(rejected.status, 405);
  assert.equal(requests, 1);
  assert.deepEqual(methods, []);
  console.log("HTTP_REQUEST_ACCOUNTING_PASS rejected-GET=405 total=1 rpc=0");
  for (const mode of ["on", "off", "shadowed", "builtin-disabled"]) {
    methods.length = 0;
    requests = 0;
    const base = join(root, mode), target = join(base, "target"), agent = join(base, "shared-agent"), cwd = join(base, "workspace");
    for (const path of [join(target, "config"), agent, cwd]) mkdirSync(path, { recursive: true, mode: 0o700 });
    writeFileSync(join(agent, "historical.txt"), "Shared historical bytes\n", { mode: 0o600 });
    if (mode === "shadowed") writeFileSync(join(agent, "mcp.json"), JSON.stringify({ mcpServers: { context7: { url, enabled: false } } }), { mode: 0o600 });
    const before = tree(agent);
    process.env.PI_CODING_AGENT_DIR = agent;
    process.env.GENTLE_PI_CONFIG_HOME = join(target, "config");
    // Default SDK storage initializes files in the agent directory. Explicit
    // public SDK options keep these files inside the private target instead.
    const config = join(target, "config");
    const modelRuntime = await ModelRuntime.create({ authPath: join(config, "auth.json"), modelsStorePath: join(config, "models-store.json"), modelsPath: null, allowModelNetwork: false, refreshOnCreate: false });
    const credentials = new McpOAuthCredentialStore(new FileAuthStorageBackend(join(config, "mcp-auth.json")), config);
    const targetBefore = tree(target);
    let registrationApi;
    const settings = SettingsManager.inMemory(mode === "builtin-disabled" ? { extensions: ["-builtin:mcp"] } : {});
    const loader = new DefaultResourceLoader({
      cwd, agentDir: agent, settingsManager: settings,
      extensionFactories: [
        (pi) => {
          if (mode !== "off") pi.registerMcpServer("context7", { url, exposure: "codemode" });
          registrationApi = pi;
        },
        { name: "mcp", builtin: true, replaceable: true, factory: createMcpExtension({ credentials, logPath: join(config, "mcp.log") }) },
      ],
    });
    await loader.reload();
    const { session } = await createAgentSession({ cwd, agentDir: agent, resourceLoader: loader, settingsManager: settings, modelRuntime, sessionManager: SessionManager.inMemory(), noTools: "builtin" });
    try {
      await session.bindExtensions({});
      // The real MCP command waits for pending connections. No model request.
      if (mode !== "builtin-disabled") await session.prompt("/mcp");
      const registrations = registrationApi.getMcpServers();
      assert.deepEqual(loader.getExtensions().errors, []);
      const name = "mcp__context7__fixture_docs";
      const tool = session.getToolDefinition(name);
      assert.equal(registrations.length, mode === "off" ? 0 : 1);
      if (mode === "on") {
        assert.ok(tool, "actual built-in MCP did not register its discovered tool");
        const result = await tool.execute("direct-source-callback", {}, AbortSignal.timeout(2000), undefined, session.extensionRunner.createContext());
        assert.ok(result.content.some((item) => item.type === "text" && item.text === "ACTUAL_MCP_CALLBACK_RESULT"));
        assert.ok(methods.includes("initialize") && methods.includes("tools/list") && methods.includes("tools/call"));
        assert.ok(requests >= 4, "actual handshake and tool call were not observed");
      } else {
        assert.equal(tool, undefined);
        assert.deepEqual(methods, []);
        assert.equal(requests, 0, "disabled/shadowed server received HTTP traffic");
      }
      assert.equal(session.getActiveToolNames().includes("write"), false);
    } finally {
      await session.extensionRunner.emit({ type: "session_shutdown" });
      session.dispose();
    }
    assert.deepEqual(tree(agent), before, "Shared agent files changed");
    const after = tree(target);
    if (mode === "on") {
      // The real credential store initializes its explicitly private file on
      // first connection. Capture it, do not hide it or claim zero writes.
      const auth = statSync(join(config, "mcp-auth.json"));
      assert.equal(auth.mode & 0o777, 0o600);
      assert.equal(readFileSync(join(config, "mcp-auth.json"), "utf8"), "{}");
      const expected = structuredClone(targetBefore);
      assert.equal(after[0][2], expected[0][2]);
      assert.ok(after[0][3] >= expected[0][3]);
      expected[0][3] = after[0][3]; // owned directory mtime changes on creation
      expected[0][4].push(["mcp-auth.json", auth.mode, auth.ino, auth.mtimeMs, "e30="]);
      assert.deepEqual(after, expected, "unexpected private file/metadata mutation");
    } else {
      assert.deepEqual(after, targetBefore, "disabled private config changed");
    }
    console.log(`FROZEN_PI_REGISTRATION_PASS mode=${mode} requests=${requests} methods=${JSON.stringify(methods)} actual-tool-callback=${mode === "on"} shared-tree-preserved=true private-auth-initialized=${mode === "on"}`);
  }
} finally {
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
