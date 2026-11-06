import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

// Actual public SDK registration + installed-command fixture, no model call,
// assistant provenance, indexing or installed-platform qualification.
const [root] = process.argv.slice(2);
const pkg = "/node_modules/@earendil-works/pi-coding-agent";
const extension = "/node_modules/gentle-pi/extensions/codegraph-tools.ts";
assert.equal(createHash("sha256").update(readFileSync(extension)).digest("hex"), "7d499bc7c70e9b7812bc03c373aafe766472ace6b95d036e0a585438ba35d0f7");
const { createAgentSession, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } = await import(pathToFileURL(join(pkg,"dist/index.js")).href);
const cwd = join(root,"project"), agentDir = join(root,"agent"), config = join(root,"config");
const settingsManager = SettingsManager.inMemory({ packages:["/node_modules/gentle-pi"], extensions:["-builtin:mcp"] });
const modelRuntime = await ModelRuntime.create({authPath:join(config,"auth.json"),modelsStorePath:join(config,"models-store.json"),modelsPath:null,allowModelNetwork:false,refreshOnCreate:false});
let session;
try {
  const loader = new DefaultResourceLoader({cwd,agentDir,settingsManager});
  await loader.reload();
  ({session} = await createAgentSession({cwd,agentDir,settingsManager,resourceLoader:loader,modelRuntime,sessionManager:SessionManager.inMemory(),noTools:"builtin"}));
  await session.bindExtensions({});
  assert.deepEqual(loader.getExtensions().errors,[]);
  assert.ok(session.getToolDefinition("codegraph"),"actual packaged SDK did not register CodeGraph");
  assert.equal(execFileSync("codegraph",["--version"],{cwd,encoding:"utf8"}).trim(),"1.4.1");
  const status = JSON.parse(execFileSync("codegraph",["status",cwd,"--json"],{cwd,encoding:"utf8"}));
  assert.equal(status.initialized,false);
  assert.equal(status.lastIndexed,null);
  assert.equal(status.projectPath,cwd);
  console.log("CODEGRAPH_PACKAGED_SDK_DISCOVERY_PASS tool_registered=true actual_private_command=1.4.1 initialized=false lastIndexed=null no_model_call=true");
} finally {
  if (session) {
    await session.extensionRunner.emit({type:"session_shutdown"});
    session.dispose();
  }
}
