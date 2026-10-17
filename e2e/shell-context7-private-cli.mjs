import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync, readdirSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { join } from "node:path";

const [pkg, root] = process.argv.slice(2);
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
  if (call.method === "initialize") result = { protocolVersion: "2025-11-25", capabilities: { tools: {} }, serverInfo: { name: "private-cli-fixture", version: "1" } };
  if (call.method === "tools/list") result = { tools: [{ name: "fixture_docs", description: "Local fixture, not Context7", inputSchema: { type: "object", properties: {} } }] };
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
  for (const mode of ["on", "off", "shadowed"]) {
    requests = 0;
    methods.length = 0;
    const base = join(root, mode), target = join(base, "target"), shared = join(base, "shared");
    mkdirSync(base);
    mkdirSync(target);
    mkdirSync(shared);
    for (const name of ["home", "agent", "config", "workspace"]) mkdirSync(join(target, name));
    writeFileSync(join(shared, "historical.txt"), "Shared immutable bytes\n", { mode: 0o600 });
    const agent = join(target, "agent"), config = join(target, "config");
    writeFileSync(join(config, "mcp.json"), JSON.stringify({ mcpServers: { context7: { url, exposure: "codemode", enabled: mode !== "off" } } }), { mode: 0o600 });
    if (mode === "shadowed") writeFileSync(join(agent, "mcp.json"), JSON.stringify({ mcpServers: { context7: { url, enabled: false } } }), { mode: 0o600 });
    const sharedBefore = tree(shared), policyBefore = tree(config);
    const globalPath = join(agent, "mcp.json");
    const globalBefore = mode === "shadowed" ? { bytes: readFileSync(globalPath), info: statSync(globalPath) } : undefined;
    const { stdout, stderr } = await new Promise((resolve, reject) => {
      const child = execFile("/opt/node/bin/node", [join(pkg, "dist/bundle/cli.js"), "--extension", "/sdk/extensions/private-context7.ts", "--no-session", "--print", "/mcp"], {
        cwd: join(target, "workspace"), timeout: 15000, killSignal: "SIGKILL", maxBuffer: 1048576,
        env: { HOME: join(target, "home"), PI_CODING_AGENT_DIR: agent, GENTLE_PI_CONFIG_HOME: config, PATH: "/opt/node/bin:/usr/bin:/bin", GENTLE_AI_TELEMETRY_DISABLE: "1" },
      }, (error, stdout, stderr) => {
        if (error) reject(error);
        else resolve({ stdout, stderr });
      });
      // Print mode consumes piped stdin before startup; provide a real EOF.
      child.stdin.end();
    });
    console.log(`PRIVATE_CLI_READBACK mode=${mode} stdout=${JSON.stringify(stdout)} stderr=${JSON.stringify(stderr)} HTTP=${requests} RPC=${JSON.stringify(methods)}`);
    // Frozen print mode drops /mcp notifications: this is NOT a status receipt.
    assert.equal(stdout, "");
    assert.equal(stderr, "");
    if (mode === "on") {
      assert.ok(methods.includes("initialize") && methods.includes("tools/list"), "CLI did not initialize/discover the private registration");
      assert.ok(requests >= 3);
    } else {
      assert.equal(requests, 0, "off/shadowed CLI contacted the private server");
    }
    assert.deepEqual(tree(shared), sharedBefore, "CLI changed Shared files");
    assert.deepEqual(tree(config), policyBefore, "CLI rewrote private policy");
    if (globalBefore) {
      const after = statSync(globalPath);
      assert.deepEqual(readFileSync(globalPath), globalBefore.bytes);
      assert.equal(after.ino, globalBefore.info.ino);
      assert.equal(after.mode, globalBefore.info.mode);
      assert.equal(after.mtimeMs, globalBefore.info.mtimeMs);
    }
    console.log(`FROZEN_PI_PRIVATE_CLI_PASS mode=${mode} HTTP=${requests} Shared-preserved=true`);
  }
} finally {
  server.closeAllConnections();
  await new Promise((resolve) => server.close(resolve));
}
