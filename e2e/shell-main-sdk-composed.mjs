import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, lstatSync, readFileSync, readdirSync, readlinkSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// Qualify the unpublished SDK Main + exact stock supplier composition.
// Stock native/installer modules are authenticated; no installation, native
// process, model prompt, tool call, dev override or child mode.
const [root, sdk, selection] = process.argv.slice(2);
assert.ok(selection === undefined || selection === "selected" || selection === "worker");
if (selection === "worker") assert.equal(process.env.GENTLE_PI_NO_SKILL_REGISTRY, "1");
const main = join(root, "runtime/main-sdk");
const { resolveGentleAiBinary, gentleAiBinaryPath, PackageLocalGentleAiBinaryMissingError } =
  await import(pathToFileURL(join(main, "runtime/gentle-ai-binary.mjs")).href);
assert.equal(existsSync(join(root, "config/dev-binary.json")), false);
const binary = gentleAiBinaryPath(main, "linux");
assert.equal(resolveGentleAiBinary(main, "linux"), binary);
assert.equal(createHash("sha256").update(readFileSync(binary)).digest("hex"),
  "50ba217b5138c1a9c7d5bf2f79931b1bb89b89c4cf650dcd7ee037657c88158d");
assert.throws(() => resolveGentleAiBinary(main, "linux", () => Buffer.from("bad")),
  PackageLocalGentleAiBinaryMissingError);
console.log("COMPOSED_SDK_NATIVE_PASS package_local=true pinned=true no_process=true");
const cwd = join(root, "project"), agentDir = join(root, "agent");
function projectInventory(directory, relative = "") {
  const records = {};
  for (const entry of readdirSync(directory).sort()) {
    const path = join(directory, entry), name = join(relative, entry);
    const stat = lstatSync(path);
    records[name] = {
      mode: stat.mode, ino: stat.ino, size: stat.size, mtime: stat.mtimeMs,
      content: stat.isFile() ? createHash("sha256").update(readFileSync(path)).digest("hex") :
        stat.isSymbolicLink() ? readlinkSync(path) : "directory",
    };
    if (stat.isDirectory()) Object.assign(records, projectInventory(path, name));
  }
  return records;
}
const projectBefore = projectInventory(cwd);
const agentBefore = projectInventory(agentDir);
assert.equal(JSON.parse(readFileSync(join(sdk, "package.json"))).version, "1.0.0");
const { createAgentSession, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } =
  await import(pathToFileURL(join(sdk, "dist/index.js")).href);
const settingsManager = selection !== undefined ? SettingsManager.create(cwd, agentDir) :
  SettingsManager.inMemory({ packages: [main], extensions: ["-builtin:mcp"] });
assert.deepEqual(settingsManager.getPackages(), [main]);
assert.deepEqual(settingsManager.drainErrors(), []);
if (selection !== undefined) {
  assert.deepEqual(settingsManager.getGlobalSettings(), JSON.parse(readFileSync(join(agentDir, "settings.json"))));
  console.log("COMPOSED_SDK_SELECTED_SETTINGS_READBACK file_backed=true package_override=false");
}
const modelRuntime = await ModelRuntime.create({
  authPath: join(root, "config/auth.json"),
  modelsStorePath: join(root, "config/models-store.json"),
  modelsPath: null, allowModelNetwork: false, refreshOnCreate: false,
});
const loader = new DefaultResourceLoader({ cwd, agentDir, settingsManager });
const errors = [];
let session;
try {
  await loader.reload();
  const loaded = loader.getExtensions();
  console.log("COMPOSED_SDK_RESOURCES", JSON.stringify({
    extensions: loaded.extensions.map((extension) => extension.path),
    errors: loaded.errors, warnings: loaded.warnings ?? [],
  }));
  assert.deepEqual(loaded.errors, [], "actual Main package did not load with frozen SDK");
  assert.deepEqual(loaded.warnings ?? [], []);
  assert.equal(loaded.extensions.length, 20);
  assert.ok(loader.getSkills().skills.length > 0);
  assert.ok(loader.getPrompts().prompts.length > 0);
  assert.ok(loader.getThemes().themes.length > 0);
  assert.ok(loaded.extensions.some((extension) => extension.path === join(main, "extensions/gentle-ai.ts")));
  assert.ok(loaded.extensions.every((extension) => extension.path.startsWith(main + "/")));
  ({ session } = await createAgentSession({
    cwd, agentDir, settingsManager, resourceLoader: loader, modelRuntime,
    sessionManager: SessionManager.inMemory(cwd), noTools: "builtin",
  }));
  await session.bindExtensions({ onError: (error) => errors.push(error) });
  assert.deepEqual(errors, [], "actual Main session_start reported errors");
  for (const tool of ["gentle_review", "gentle_review_capture", "codegraph", "todo"]) {
    assert.ok(session.getToolDefinition(tool), `actual Main did not register ${tool}`);
  }
  assert.equal(session.isStreaming, false);
  console.log("COMPOSED_SDK_SESSION_PASS no_model_call=true no_tool_call=true stock_native_bundle=true");
} finally {
  if (session) {
    await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" });
    session.dispose();
  }
}
assert.deepEqual(errors, [], "actual Main shutdown reported errors");
console.log("COMPOSED_SDK_SHUTDOWN_PASS");
const agentAfter = projectInventory(agentDir);
assert.equal(lstatSync(agentDir).uid, 1000);
assert.equal(lstatSync(agentDir).mode & 0o177777, 0o40700);
for (const [path, record] of Object.entries(agentBefore)) {
  assert.deepEqual(agentAfter[path], record, `Main changed existing agent entry ${path}`);
}
const agents = ["gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker", "jd-fix-agent",
  "jd-judge-a", "jd-judge-b", "review-readability", "review-reliability", "review-resilience", "review-risk"];
const assets = new Map(agents.map((name) => [`agents/${name}.md`, `assets/agents/${name}.md`]));
assets.set("chains/4r-review.chain.md", "assets/chains/4r-review.chain.md");
for (const name of ["strict-tdd", "strict-tdd-verify"]) {
  assets.set(`gentle-ai/support/${name}.md`, `assets/support/${name}.md`);
}
const directories = ["agents", "chains", "gentle-agents", "gentle-agents/catalog", "gentle-agents/presence",
  "gentle-agents/transport", "gentle-agents/transport/presence", "gentle-ai", "gentle-ai/support"];
const manifestPath = "gentle-ai/managed-assets.json";
const agentCreated = Object.keys(agentAfter).filter((path) => !(path in agentBefore)).sort();
assert.deepEqual(agentCreated, [...directories, ...assets.keys(), manifestPath].sort());
const hashes = {};
for (const [target, source] of assets) {
  assert.deepEqual(readFileSync(join(agentDir, target)), readFileSync(join(main, source)));
  hashes[target] = createHash("sha256").update(readFileSync(join(main, source))).digest("hex");
}
assert.deepEqual(JSON.parse(readFileSync(join(agentDir, manifestPath))), { schemaVersion: 1, assets: hashes });
for (const path of agentCreated) {
  const stat = lstatSync(join(agentDir, path));
  assert.equal(stat.uid, 1000);
  assert.equal(stat.mode & 0o177777, directories.includes(path) ? 0o40700 : 0o100600);
  if (!directories.includes(path)) assert.equal(stat.nlink, 1);
}
console.log("COMPOSED_SDK_AGENT_READBACK_PASS prior_entries_unchanged=true package_assets_exact=true", JSON.stringify({ agentCreated }));
const projectAfter = projectInventory(cwd);
for (const [path, record] of Object.entries(projectBefore)) {
  assert.deepEqual(projectAfter[path], record, `Main changed existing private project entry ${path}`);
}
const created = Object.keys(projectAfter).filter((path) => !(path in projectBefore)).sort();
if (selection === "worker") {
  assert.deepEqual(created, [], "owned worker disabled project skill-registry writes");
} else {
  assert.deepEqual(created, [".atl", ".atl/.gitignore", ".atl/.skill-registry.cache.json", ".atl/skill-registry.md"]);
  assert.equal(projectAfter[".atl"].mode & 0o177777, 0o40700);
  for (const path of created.slice(1)) {
    assert.equal(projectAfter[path].mode & 0o177777, 0o100600);
  }
  assert.equal(readFileSync(join(cwd, ".atl/.gitignore"), "utf8"), "*\n");
  assert.doesNotThrow(() => JSON.parse(readFileSync(join(cwd, ".atl/.skill-registry.cache.json"))));
}
console.log("COMPOSED_SDK_PROJECT_READBACK_PASS existing_entries_unchanged=true", JSON.stringify({ created }));
