import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// Consumer E2E, not installed-platform qualification. Files passed here were
// written by the native stage writer, never by a duplicate fixture codec.
const [root, persona, background, frozenPackage] = process.argv.slice(2);
const sdk = frozenPackage ?? "/sdk";
if (frozenPackage) {
  const manifest = JSON.parse(readFileSync(join(sdk, "package.json"), "utf8"));
  assert.equal(manifest.name, "gentle-pi");
  assert.equal(manifest.version, "4.0.0");
}
process.env.GENTLE_PI_CONFIG_HOME = join(root, "config");
process.env.HOME = join(root, "home");
const cwd = join(root, "project");
const personal = join(root, "home/.pi/gentle-ai");
mkdirSync(cwd, { recursive: true });
mkdirSync(personal, { recursive: true });
const canary = "personal historical bytes\n";
writeFileSync(join(personal, "persona.json"), canary);
const { resolveBackgroundSubagentsPolicy } = await import(pathToFileURL(join(sdk, "lib/background-subagents-policy.ts")).href);
const { createGentleAiExtension } = await import(pathToFileURL(join(sdk, "extensions/gentle-ai.ts")).href);
const personaFile = join(root, "config/persona.json");
const personaPreimage = existsSync(personaFile) ? readFileSync(personaFile) : null;
const backgroundFile = join(root, "config/background-subagents.json");
const backgroundPreimage = readFileSync(backgroundFile);
const handlers = new Map();
const api = {
  events: new EventEmitter(),
  on: (name, handler) => handlers.set(name, handler),
  registerCommand() {}, registerTool() {}, registerFlag() {},
  getFlag: () => false, getActiveTools: () => [], getAllTools: () => [],
};
createGentleAiExtension({
  nativeReviewCli: null, childStandingReviewPermissionClient: null,
  resolveTelemetryTriggerBinary() { throw new Error("no installed-native telemetry in this source test"); },
})(api);
async function actualPersonaPrompt(expected) {
  const options = { appendSystemPrompt: "" };
  await handlers.get("before_agent_start")({ systemPromptOptions: options }, {
    cwd, hasUI: false, mode: "interactive",
    sessionManager: { getSessionId: () => "source-experience" },
  });
  assert.ok(options.appendSystemPrompt.includes(`Current persona mode: ${expected}`), options.appendSystemPrompt);
}
await actualPersonaPrompt(persona === "unmanaged" ? "gentleman" : persona);
let result = resolveBackgroundSubagentsPolicy(cwd);
assert.equal(result.policy, background);
assert.equal(result.source, "global_file");
assert.equal(result.malformed, false);
assert.equal(result.globalFile, join(root, "config/background-subagents.json"));
const project = join(cwd, ".pi/gentle-ai");
mkdirSync(project, { recursive: true });
const override = background === "on" ? "off" : "on";
writeFileSync(join(project, "background-subagents.json"), JSON.stringify({ schema: "gentle-pi.background-subagents/v1", policy: override }));
writeFileSync(join(project, "persona.json"), '{"mode":"neutral"}');
await actualPersonaPrompt("neutral");
result = resolveBackgroundSubagentsPolicy(cwd);
assert.equal(result.policy, override);
assert.equal(result.source, "project_file");
writeFileSync(join(project, "background-subagents.json"), "{malformed historical bytes");
result = resolveBackgroundSubagentsPolicy(cwd);
assert.equal(result.policy, "off");
assert.equal(result.source, "project_file");
assert.equal(result.malformed, true);
assert.equal(readFileSync(join(personal, "persona.json"), "utf8"), canary);
assert.deepEqual(existsSync(personaFile) ? readFileSync(personaFile) : null, personaPreimage);
assert.deepEqual(readFileSync(backgroundFile), backgroundPreimage);
assert.equal(readFileSync(join(project, "persona.json"), "utf8"), '{"mode":"neutral"}');
assert.equal(readFileSync(join(project, "background-subagents.json"), "utf8"), "{malformed historical bytes");
console.log(`CANONICAL_${frozenPackage ? "PACKAGE" : "SOURCE"}_PASS persona=${persona} background=${background} actual-handler=true project-precedence=true personal-preserved=true`);
