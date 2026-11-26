import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

// Pi's MCP CLI connects without loading any packages or calling a model.
const [root, pkg = "/node_modules/@earendil-works/pi-coding-agent"] = process.argv.slice(2);
const cwd = join(root, "project"), agentDir = join(root, "agent");
const configPath = join(agentDir, "mcp.json");
// Mirror the optional Gentle AI integration; this fixture does not run the installer.
const config = JSON.stringify({ mcpServers: { codegraph: { command: "codegraph", args: ["serve", "--mcp"] } } });
writeFileSync(configPath, config, { encoding: "utf8", flag: "wx", mode: 0o600 });

const options = { cwd, encoding: "utf8", timeout: 15000 };
const report = JSON.parse(execFileSync(process.execPath, [join(pkg, "dist/bundle/cli.js"), "mcp", "list", "--json"], {
  ...options, env: { ...process.env, PI_CODING_AGENT_DIR: agentDir },
}));
assert.deepEqual(report.errors, []);
assert.equal(report.servers.length, 1);
const [server] = report.servers;
assert.equal(server.name, "codegraph");
assert.equal(server.scope, "global");
assert.equal(server.source, configPath);
assert.equal(server.enabled, true);
assert.equal(server.state, "connected");
assert.deepEqual(server.tools, ["codegraph_explore"]);
assert.equal(readFileSync(configPath, "utf8"), config);

const status = JSON.parse(execFileSync("codegraph", ["status", cwd, "--json"], options));
assert.equal(status.initialized, false);
assert.equal(status.lastIndexed, null);
assert.equal(status.projectPath, cwd);
assert.equal(existsSync(join(cwd, ".codegraph")), false, "MCP discovery must not index the project");
assert.equal(execFileSync("codegraph", ["--version"], options).trim(), "1.4.1");
// Keep the pass marker consumed by the tagged Go launcher fixture.
console.log("CODEGRAPH_PACKAGED_SDK_DISCOVERY_PASS mcp_tool=codegraph_explore actual_private_command=1.4.1 initialized=false lastIndexed=null no_model_call=true");
