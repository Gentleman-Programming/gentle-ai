import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { lstatSync, readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// The Go caller authenticates/extracts the frozen archive and runs the actual
// stage preparer. No synthetic assistant, model definition, or user messages.
const [pkg, root] = process.argv.slice(2);
// Source-supplied closure declared at cleanup27ae; not a published package.
// The caller supplies these files read-only; this is not a mutation custody proof.
const sourceClosure = [
  ["/sdk/extensions/strict-tdd.ts", "d2a73347b850d1e7e8f0f04be1f4d7d512ffa8be467ef1a7375de2a78b02375c"],
  ["/sdk/lib/agent-home.ts", "4cf2363d319e30e2b8bb5e2dbe58239c2397798331fbbe11f1675f30ee595511"],
  ["/sdk/lib/append-system-prompt.ts", "e0849bd13e30da8b1ec55e1b0a5a1fcac186ececeb43f84511d02467fe2fede3"],
];
for (const [path, expected] of sourceClosure) {
  assert.ok(lstatSync(path).isFile(), `supplied source is not a regular file: ${path}`);
  assert.equal(createHash("sha256").update(readFileSync(path)).digest("hex"), expected, `supplied source mismatch: ${path}`);
}
const { createAgentSession, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } = await import(pathToFileURL(join(pkg, "dist/index.js")).href);
function tree(path) {
  return readdirSync(path).sort().map((name) => {
    const absolute = join(path, name), info = lstatSync(absolute);
    return [name, info.mode, info.ino, info.mtimeMs, info.isDirectory() ? tree(absolute) : readFileSync(absolute).toString("base64")];
  });
}
for (const choice of ["on", "off", "absent"]) {
  const target = join(root, choice), config = join(target, "config"), agent = join(target, "agent"), cwd = join(target, "project");
  const beforeInit = tree(target);
  const path = join(config, "strict-tdd.json");
  if (choice === "absent") assert.ok(!readdirSync(config).includes("strict-tdd.json"));
  else {
    assert.equal(readFileSync(path, "utf8"), '{\n  "schema": "gentle-pi.strict-tdd/v1",\n  "policy": "' + choice + '"\n}\n');
    assert.equal(lstatSync(path).mode & 0o777, 0o600);
  }
  assert.ok(!readdirSync(config).includes("persona.json"), "unmanaged acquired a persona");
  const previous = process.env.GENTLE_PI_CONFIG_HOME;
  process.env.GENTLE_PI_CONFIG_HOME = config;
  let session;
  try {
    const modelRuntime = await ModelRuntime.create({ authPath: join(config, "auth.json"), modelsStorePath: join(config, "models-store.json"), modelsPath: null, allowModelNetwork: false, refreshOnCreate: false });
    const settings = SettingsManager.inMemory({}, { projectTrusted: false });
    const loader = new DefaultResourceLoader({ cwd, agentDir: agent, settingsManager: settings, additionalExtensionPaths: ["/sdk/extensions/strict-tdd.ts"], noSkills: true, noPromptTemplates: true, noThemes: true });
    await loader.reload();
    assert.deepEqual(loader.getExtensions().errors, []);
    ({ session } = await createAgentSession({ cwd, agentDir: agent, resourceLoader: loader, modelRuntime, settingsManager: settings, sessionManager: SessionManager.inMemory(), noTools: "builtin" }));
    await session.bindExtensions({});
    const afterInit = tree(target);
    // Disclose actual SDK initialization effects instead of ignoring config.
    const beforeConfig = beforeInit.find((entry) => entry[0] === "config");
    const afterConfig = afterInit.find((entry) => entry[0] === "config");
    const beforeNames = beforeConfig[4].map((entry) => entry[0]);
    const created = afterConfig[4].filter((entry) => !beforeNames.includes(entry[0]));
    assert.deepEqual(created.map((entry) => entry[0]), ["auth.json"]);
    assert.equal(beforeConfig[1], afterConfig[1]);
    assert.equal(beforeConfig[2], afterConfig[2]);
    assert.ok(afterConfig[3] >= beforeConfig[3]);
    for (const entry of created) {
      assert.equal(entry[1] & 0o777, 0o600);
      console.log(`OWNED_SDK_INITIALIZATION choice=${choice} file=${entry[0]} bytes=${Buffer.from(entry[4], "base64").length}`);
    }
    const expected = structuredClone(beforeInit);
    expected.find((entry) => entry[0] === "config")[3] = afterConfig[3];
    expected.find((entry) => entry[0] === "config")[4].push(...created);
    expected.find((entry) => entry[0] === "config")[4].sort((a, b) => a[0].localeCompare(b[0]));
    assert.deepEqual(afterInit, expected, "SDK startup altered pre-existing stage bytes/metadata or wrote elsewhere");
    const errors = [];
    session.extensionRunner.onError((error) => errors.push(error));
    const result = await session.extensionRunner.emitBeforeAgentStart("Canonical stage readback, not an agent run", undefined, { cwd, selectedTools: [], appendSystemPrompt: "ordinary baseline" });
    const options = result.systemPromptOptions;
    assert.equal(options.forceSystemPrompt, undefined);
    if (choice === "on") {
      assert.ok(options.appendSystemPrompt.startsWith("ordinary baseline"));
      assert.match(options.appendSystemPrompt, /observed behavioral RED before changing production code/);
      assert.ok(!options.appendSystemPrompt.includes("malformed or unreadable"));
    } else assert.equal(options.appendSystemPrompt, "ordinary baseline");
    assert.deepEqual(errors, []);
    assert.deepEqual(result.messages, []);
    assert.deepEqual(session.messages, []);
    assert.equal(session.isStreaming, false);
    assert.deepEqual(tree(target), afterInit, "policy callback changed owned bytes/mode/inode/mtime");
    console.log(`FROZEN_PI_STRICT_TDD_STAGE_PASS choice=${choice} callback-consumed=true additional-policy=${choice === "on"} messages=0 provider-request-issued=false`);
  } finally {
    try {
      if (session) await session.extensionRunner.emit({ type: "session_shutdown" });
    } finally {
      try { session?.dispose(); }
      finally {
        if (previous === undefined) delete process.env.GENTLE_PI_CONFIG_HOME;
        else process.env.GENTLE_PI_CONFIG_HOME = previous;
      }
    }
  }
}
