import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { existsSync, lstatSync, readFileSync, readdirSync, readlinkSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// Interpret the unmodified retained package with the authenticated SDK.
// No model prompt, tool invocation, installer, native override or child mode.
const [root, sdk] = process.argv.slice(2);
const main = join(root, "runtime/main-source");
assert.equal(existsSync(join(main, ".gentle-ai")), false, "this probe must not activate a native bundle");
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
assert.equal(JSON.parse(readFileSync(join(sdk, "package.json"))).version, "1.0.0");
const { createAgentSession, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } =
  await import(pathToFileURL(join(sdk, "dist/index.js")).href);
const settingsManager = SettingsManager.inMemory({ packages: [main], extensions: ["-builtin:mcp"] });
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
  console.log("MAIN_SDK_RESOURCES", JSON.stringify({
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
  console.log("MAIN_SDK_SESSION_PASS no_model_call=true no_tool_call=true native_bundle_absent=true");
} finally {
  if (session) {
    await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" });
    session.dispose();
  }
}
assert.deepEqual(errors, [], "actual Main shutdown reported errors");
console.log("MAIN_SDK_SHUTDOWN_PASS");
const projectAfter = projectInventory(cwd);
for (const [path, record] of Object.entries(projectBefore)) {
  assert.deepEqual(projectAfter[path], record, `Main changed existing private project entry ${path}`);
}
const created = Object.keys(projectAfter).filter((path) => !(path in projectBefore)).sort();
assert.deepEqual(created, [".atl", ".atl/.gitignore", ".atl/.skill-registry.cache.json", ".atl/skill-registry.md"]);
assert.equal(projectAfter[".atl"].mode & 0o777, 0o700);
for (const path of created.slice(1)) {
  assert.equal(projectAfter[path].mode & 0o170777, 0o100600);
}
assert.equal(readFileSync(join(cwd, ".atl/.gitignore"), "utf8"), "*\n");
assert.doesNotThrow(() => JSON.parse(readFileSync(join(cwd, ".atl/.skill-registry.cache.json"))));
console.log("MAIN_SDK_PROJECT_READBACK_PASS existing_entries_unchanged=true", JSON.stringify({ created }));
