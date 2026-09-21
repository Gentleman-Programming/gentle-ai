package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCodeV2TelemetryLifecycle(t *testing.T) {
	runV2Plugin(t, "telemetry-runtime", `
const emitted=[], children=[]; let now=1000; Date.now=()=>now;
let sweep;const originalInterval=globalThis.setInterval;globalThis.setInterval=(fn,ms)=>{sweep=fn;return originalInterval(fn,ms)};
globalThis.__exec=(cmd,args,opts,cb)=>{ const child={stdin:{on(){},end(body){emitted.push(JSON.parse(body))}},kill(){this.killed=true}}; children.push(child); return child };
let deliver, signal; const queue=[]; let wake;
const ctx={location:{directory:"/project",workspaceID:"ws"},event:{subscribe(opts){signal=opts.signal; signal.addEventListener("abort",()=>wake?.()); return {[Symbol.asyncIterator]:async function*(){while(!signal.aborted){if(!queue.length) await new Promise(r=>wake=r); while(queue.length) yield queue.shift()}}}}}};
const cleanup=await plugin.setup(ctx);
const tick=()=>new Promise(r=>setImmediate(r));
const event=async(type,id,extra={},location=ctx.location)=>{queue.push({type,created:now,location,data:{sessionID:"SECRET-SESSION",assistantMessageID:id,...extra}});wake?.(); await tick()};
const start=(id,location)=>event("session.step.started",id,{agent:"sdd-apply",model:{providerID:"openai",id:"gpt-5.6-sol",variant:"high"}},location);
const end=(id,location)=>event("session.step.ended",id,{tokens:{input:10,output:2,reasoning:1,cache:{read:3,write:0}}},location);
await start("SECRET-MESSAGE"); now+=5; await end("SECRET-MESSAGE"); await end("SECRET-MESSAGE");
if(emitted.length!==1||emitted[0].info.selectedEffort!=="high")throw Error("completion/duplicate");
if(JSON.stringify(emitted).includes("SECRET")||JSON.stringify(emitted).includes("/project"))throw Error("identity escaped");
await start("workspace"); await end("workspace",{directory:"/project",workspaceID:"other"}); if(emitted.length!==1)throw Error("workspace leak");
await start("mismatch"); await end("mismatch",{directory:"/other",workspaceID:"ws"}); if(emitted.length!==1)throw Error("location leak");
await end("unknown"); await start("expired"); now+=600001; sweep(); await end("expired"); if(emitted.length!==1)throw Error("expiry/unmatched");
await start("veto");process.env.DO_NOT_TRACK="1";await end("veto");process.env.DO_NOT_TRACK="0";await end("veto");if(emitted.length!==1)throw Error("veto state retained");
await start("failed"); await event("session.step.failed","failed",{error:{type:"APIError",message:"SECRET-ERROR",status:429},tokens:{input:7}}); if(emitted.length!==2||emitted[1].info.error.data.statusCode!==429)throw Error("failure");
if(JSON.stringify(emitted).includes("SECRET"))throw Error("error identity escaped");
await start("malformed");await event("session.step.ended","malformed",{tokens:{input:{sessionID:"SECRET-NESTED"},output:2}});if(JSON.stringify(emitted).includes("SECRET"))throw Error("nested token privacy");
for(let i=0;i<257;i++)await start("bounded"+i); await end("bounded256"); if(emitted.length!==3)throw Error("capacity");
for(let i=0;i<40;i++)await end("bounded"+i); if(emitted.length!==32)throw Error("in flight bound");
await cleanup(); if(!signal.aborted||children.some(x=>!x.killed))throw Error("cleanup");
await end("bounded100"); if(emitted.length!==32)throw Error("disposed");
`)
}

func runV2Plugin(t *testing.T, name, harness string) {
	t.Helper()
	source, err := Read("opencode/plugins-v2/" + name + ".ts")
	if err != nil {
		t.Fatal(err)
	}
	if name == "telemetry-runtime" {
		for _, forbidden := range []string{"node:fs", "console.", "ctx.session.", "fetch(", "process.cwd("} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("telemetry adapter contains forbidden IO: %s", forbidden)
			}
		}
	}
	source = strings.Replace(source, `import { spawn } from "node:child_process"`, `const spawn = (...args: any[]) => (globalThis as any).__spawn(...args)`, 1)
	source = strings.Replace(source, `import { Plugin } from "@opencode/plugin"`, `const Plugin = { define: (value: any) => value }`, 1)
	source = strings.Replace(source, `import { execFile } from "node:child_process"`, `const execFile = (...args: any[]) => (globalThis as any).__exec(...args)`, 1)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin.mts"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "harness.mjs"), []byte("import plugin from './plugin.mts'\n"+harness), 0600); err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	cmd := exec.Command(node, "--experimental-strip-types", filepath.Join(dir, "harness.mjs"))
	cmd.Env = append(os.Environ(), "DO_NOT_TRACK=0", "GENTLE_AI_TELEMETRY=1", "CI=0", "GITHUB_ACTIONS=0", "HOME="+dir, "XDG_CONFIG_HOME="+dir, "XDG_DATA_HOME="+dir)
	if out, err := cmd.CombinedOutput(); err != nil || strings.Contains(string(out), "SECRET") {
		t.Fatalf("V2 harness: %v\n%s", err, out)
	}
}

func TestOpenCodeV2CatalogAndRegistry(t *testing.T) {
	legacy, err := Read("opencode/plugins/skill-registry.ts")
	if err != nil {
		t.Fatal(err)
	}
	native, err := Read("opencode/plugins-v2/skill-registry.ts")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(legacy, "\n") {
		if strings.HasPrefix(line, "const PROJECT_MARKERS =") && !strings.Contains(native, line) {
			t.Fatal("V2 project marker guard diverged")
		}
	}

	runV2Plugin(t, "model-variants", `
const fs=await import('node:fs/promises');const path=await import('node:path');const os=await import('node:os');const crypto=await import('node:crypto');
const root=await fs.mkdtemp(path.join(os.tmpdir(),'catalog-v2-'));process.env.HOME=root;process.env.USERPROFILE=root;
let signal,wake;const queue=[];let revision=0;let subscribed=false;
const location={directory:'/project',workspaceID:'one'};
const ctx={location,model:{async list(){if(!subscribed)throw Error("snapshot before subscription");return {location:{directory:location.directory},data:[{providerID:'openai',id:'model',variants:[{id:revision?'high':'low'}]}]}}},event:{subscribe(opts){signal=opts.signal;signal.addEventListener('abort',()=>wake?.());return {[Symbol.asyncIterator]:async function*(){subscribed=true;while(!signal.aborted){if(!queue.length)await new Promise(r=>wake=r);while(queue.length)yield queue.shift()}}}}}};
// #4846: a fixed 30ms sleep assumes machine speed, not the plugin's async
// cache-write behavior. Windows runners are ~20x slower because Defender scans
// every temp write, so poll the real condition until a bounded deadline instead
// of racing a hard-coded delay. The deadline stays well inside the package
// timeout (10m default; the Windows shard passes -timeout=30m), and a genuine
// hang still fails with a diagnostic naming the condition that never became true.
const waitFor=async(predicate,{timeout,interval,label})=>{const deadline=Date.now()+timeout;let last;for(;;){try{if(await predicate())return}catch(error){last=error}if(Date.now()>=deadline)throw Error(label+' did not become true within '+timeout+'ms'+(last?': '+last:''));await new Promise(r=>setTimeout(r,interval))}};
const cleanup=await plugin.setup(ctx);
const dir=path.join(root,'.gentle-ai','cache','opencode-v2');const read=async()=>JSON.parse(await fs.readFile(path.join(dir,(await fs.readdir(dir))[0]),'utf8'));
await waitFor(async()=>(await read()).openai.model[0]==='low',{timeout:30000,interval:25,label:'initial catalog cache write'});
if((await read()).openai.model[0]!=='low')throw Error('catalog');
revision=1;queue.push({type:'model.updated',location});wake?.();
await waitFor(async()=>(await read()).openai.model[0]==='high',{timeout:30000,interval:25,label:'catalog refresh to high'});
if((await read()).openai.model[0]!=='high')throw Error('refresh');
if(await fs.stat(path.join(root,'.gentle-ai','cache','model-variants.json')).then(()=>true,()=>false))throw Error('legacy cache overwritten');await cleanup();if(!signal.aborted)throw Error('catalog disposal');
const second=await plugin.setup({location:{...location,workspaceID:'two'},model:ctx.model,event:{subscribe(opts){return {[Symbol.asyncIterator]:async function*(){await new Promise(r=>opts.signal.addEventListener('abort',r))}}}}});
await waitFor(async()=>(await fs.readdir(dir)).filter(name=>name.endsWith('.json')).length===2,{timeout:30000,interval:25,label:'second workspace cache entry'});
if((await fs.readdir(dir)).length!==2)throw Error('workspace caches collide');await second();await fs.rm(root,{recursive:true});
`)
	runV2Plugin(t, "skill-registry", `
const fs=await import('node:fs/promises');const os=await import('node:os');const path=await import('node:path');const root=await fs.mkdtemp(path.join(os.tmpdir(),'registry-v2-'));await fs.mkdir(path.join(root,'.git'));
const calls=[];globalThis.__exec=(cmd,args,opts,cb)=>{const child={kill(){this.killed=true}};calls.push({cmd,args,opts,child});return child};
const cleanup=await plugin.setup({location:{directory:root,project:{directory:'/wrong'}}});if(calls.length!==1||calls[0].args.at(-1)!==root||calls[0].opts.cwd!==root)throw Error('registry location');await cleanup();if(!calls[0].child.killed)throw Error('registry child cleanup');await plugin.setup({location:{directory:os.homedir()}});if(calls.length!==1)throw Error('home guard');await fs.rm(root,{recursive:true});
`)
}
