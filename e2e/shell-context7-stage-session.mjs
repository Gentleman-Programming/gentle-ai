import assert from "node:assert/strict";
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const [pkg, root] = process.argv.slice(2);
const { createAgentSession, createMcpExtension, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } = await import(pathToFileURL(join(pkg, "dist/index.js")).href);
// Full physical preimages, including directory roots and every child.
function snapshot(path) {
  const info = lstatSync(path, { bigint: true });
  return [info.dev.toString(), info.ino.toString(), info.mode.toString(), info.mtimeNs.toString(),
    info.isDirectory() ? readdirSync(path).sort().map((name) => [name, snapshot(join(path, name))]) : readFileSync(path).toString("base64")];
}
let requests = 0;
const methods = [];
const server = createServer(async (request, response) => {
  requests++;
  if (request.method !== "POST") { response.writeHead(405).end(); return; }
  let bytes = "";
  for await (const chunk of request) {
    bytes += chunk;
    if (Buffer.byteLength(bytes) > 65536) { response.writeHead(413).end(); return; }
  }
  let call;
  try { call = JSON.parse(bytes); }
  catch { response.writeHead(400).end(); return; }
  methods.push(call.method);
  if (call.id === undefined) { response.writeHead(204).end(); return; }
  let result = {};
  if (call.method === "initialize") result = { protocolVersion: "2025-11-25", capabilities: { tools: {} }, serverInfo: { name: "project-override-fixture", version: "1" } };
  if (call.method === "tools/list") result = { tools: [{ name: "fixture_docs", description: "Local project fixture, not Context7", inputSchema: { type: "object", properties: {} } }] };
  response.writeHead(200, { "Content-Type": "application/json" });
  response.end(JSON.stringify({ jsonrpc: "2.0", id: call.id, result }));
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const url = `http://127.0.0.1:${server.address().port}`;
const shared = join(root, "shared-sentinel");
mkdirSync(shared, { mode: 0o700 });
writeFileSync(join(shared, "historical.txt"), "Shared historical malformed bytes\n", { mode: 0o600 });
const sharedBefore = snapshot(shared);
try {
  const rejected = await fetch(url, { method: "GET", signal: AbortSignal.timeout(2000) });
  assert.equal(rejected.status, 405);
  assert.equal(requests, 1);
  assert.deepEqual(methods, []);
  console.log("STAGE_HTTP_OBSERVER_PASS rejected-GET=405 total=1 rpc=0");
  for (const mode of ["off", "absent", "builtin-disabled", "project-trusted", "project-untrusted"]) {
    requests = 0;
    methods.length = 0;
    const target = join(root, mode), agent = join(target, "agent"), config = join(target, "config"), cwd = join(target, "project");
    const mcp = join(agent, "mcp.json");
    assert.equal(existsSync(mcp), mode !== "absent");
    const mcpBefore = mode === "absent" ? null : snapshot(mcp);
    if (mode !== "absent") {
      assert.equal(readFileSync(mcp, "utf8"), '{\n  "mcpServers": {\n    "context7": {\n      "url": "https://mcp.context7.com/mcp",\n      "enabled": false,\n      "exposure": "codemode"\n    }\n  }\n}\n');
      assert.equal(Number(lstatSync(mcp).mode) & 0o777, 0o600);
    }
    assert.equal(existsSync(join(config, "mcp.json")), false);
    const projectCase = mode.startsWith("project-");
    const projectTrusted = mode !== "project-untrusted";
    if (projectCase) {
      mkdirSync(join(cwd, ".pi"), { mode: 0o700 });
      // Local URL belongs only to the project: never replace the staged URL.
      writeFileSync(join(cwd, ".pi/mcp.json"), JSON.stringify({ mcpServers: { context7: { url, enabled: true, exposure: "direct" } } }), { mode: 0o600 });
    }
    const projectBefore = snapshot(cwd), configBefore = snapshot(config);
    process.env.HOME = join(target, "home");
    process.env.PI_CODING_AGENT_DIR = agent;
    process.env.GENTLE_PI_CONFIG_HOME = config;
    const settings = SettingsManager.inMemory(mode === "builtin-disabled" ? { extensions: ["-builtin:mcp"] } : {}, { projectTrusted });
    const modelRuntime = await ModelRuntime.create({ authPath: join(agent, "auth.json"), modelsStorePath: join(agent, "models-store.json"),
      modelsPath: null, allowModelNetwork: false, refreshOnCreate: false });
    const loader = new DefaultResourceLoader({ cwd, agentDir: agent, settingsManager: settings,
      noSkills: true, noPromptTemplates: true, noThemes: true,
      extensionFactories: [{ name: "mcp", builtin: true, replaceable: true, factory: createMcpExtension() }],
    });
    await loader.reload();
    assert.deepEqual(loader.getExtensions().errors, []);
    const { session } = await createAgentSession({ cwd, agentDir: agent, modelRuntime, resourceLoader: loader, settingsManager: settings,
      sessionManager: SessionManager.inMemory(), noTools: "builtin" });
    const notifications = [], errors = [];
    let runner;
    try {
      await session.bindExtensions({ onError(error) { errors.push(error); } });
      runner = session.extensionRunner;
      if (runner) {
        const ui = runner.getUIContext(), notify = ui.notify;
        runner.setUIContext({ ...ui, notify(message, type) { notifications.push([message, type]); } }, "print");
        assert.equal(ui.notify, notify, "original public UI unchanged");
        assert.equal(runner.createContext().isProjectTrusted(), projectTrusted);
      }
      if (mode === "builtin-disabled") {
        assert.equal(runner?.getCommand("mcp"), undefined, "do not prompt a missing command and dispatch to a model");
        assert.deepEqual(notifications, []);
      } else {
        assert.ok(runner.getCommand("mcp"));
        await session.prompt("/mcp");
        assert.equal(notifications.length, 1);
        const expected = mode === "absent"
          ? `No MCP servers configured. Add them to ${mcp} or .pi/mcp.json.`
          : mode === "project-trusted" ? "context7: connected, 1 tools (direct)" : "context7: disabled (codemode)";
        assert.deepEqual(notifications[0], [expected, "info"]);
        const beforeQuery = requests;
        await session.prompt("/mcp");
        assert.deepEqual(notifications, [[expected, "info"], [expected, "info"]]);
        assert.equal(requests, beforeQuery, "status query created no new transport");
      }
      const toolName = "mcp__context7__fixture_docs";
      if (mode === "project-trusted") {
        assert.ok(session.getToolDefinition(toolName));
        assert.ok(session.getCallableToolNames().includes(toolName));
        assert.equal(requests, 4, "project transport discovery only");
        assert.deepEqual(methods, ["initialize", "notifications/initialized", "tools/list"]);
      } else {
        assert.equal(session.getToolDefinition(toolName), undefined);
        assert.equal(requests, 0, "no project fixture traffic");
        assert.deepEqual(methods, []);
      }
      assert.deepEqual(errors, []);
      assert.deepEqual(session.messages, [], "no model prompt, assistant message or fabricated provenance");
      console.log(`STAGE_MANAGER_TEXT ${JSON.stringify({ mode, notifications, suppliedTrust: projectTrusted, trustSource: "SDK fixture", humanApproval: false })}`);
    } finally {
      runner?.setUIContext(undefined, "print");
      if (runner) await runner.emit({ type: "session_shutdown" });
      session.dispose();
    }
    assert.deepEqual(snapshot(cwd), projectBefore);
    assert.deepEqual(snapshot(config), configBefore);
    assert.equal(existsSync(join(config, "mcp.json")), false);
    assert.deepEqual(snapshot(shared), sharedBefore);
    if (mode === "absent") assert.equal(existsSync(mcp), false);
    else assert.deepEqual(snapshot(mcp), mcpBefore, "canonical official off policy unchanged");
    console.log(`FROZEN_PI_STAGE_SESSION_PASS mode=${mode} HTTP=${requests} RPC=${JSON.stringify(methods)} privateAgentFiles=${JSON.stringify(readdirSync(agent).sort())}`);
  }
} finally {
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
