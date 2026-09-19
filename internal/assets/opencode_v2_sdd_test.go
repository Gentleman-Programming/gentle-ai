package assets

import (
	"strings"
	"testing"
)

func TestOpenCodeV2SDDDispatch(t *testing.T) {
	runV2Plugin(t, "sdd-task-result-artifacts", `
const hooks={};let disposals=0,signal,wake;const queue=[];
const ctx={location:{directory:'/project',workspaceID:'one'},tool:{async hook(name,fn){await Promise.resolve();hooks[name]=fn;return {async dispose(){disposals++}}}},session:{async get(arg){if(Object.keys(arg).join()!=='sessionID')throw Error('V1 SDK shape');return {id:arg.sessionID,...(arg.sessionID==='child'?{parentID:'root'}:{})}}},event:{subscribe(opts){signal=opts.signal;signal.addEventListener('abort',()=>wake?.());return {[Symbol.asyncIterator]:async function*(){while(!signal.aborted){if(!queue.length)await new Promise(r=>wake=r);while(queue.length)yield queue.shift()}}}}}};
const cleanup=await plugin.setup(ctx);if(!hooks['execute.before']||!hooks['execute.after'])throw Error('registrations not awaited');
const reject=async(fn,why)=>{let failed=false;try{await fn()}catch{failed=true}if(!failed)throw Error(why)};
const request=()=>({tool:'subagent',sessionID:'root',id:'call',input:{agent:'sdd-apply',prompt:'Implement approved tasks',description:'Apply'}});
await reject(()=>hooks['execute.before'](request()),'missing preflight accepted');
const question={tool:'question',sessionID:'root',id:'q',input:{questions:[{question:'Gentle AI SDD preflight 1/3: Pace?',options:[]},{question:'Artifacts?',options:[]},{question:'PR?',options:[]}]}};
await hooks['execute.before'](question);
await reject(()=>hooks['execute.after']({...question,status:'completed',result:{output:{answers:[['please proceed'],['Engram'],['Auto']]}}}),'free text accepted');
for(const answers of [[['Automatic','Interactive'],['Engram'],['Auto']],[['Automátic'],['Engram'],['Auto']],[['Automatic'],['Engram']]]) {
 await reject(()=>hooks['execute.after']({...question,status:'completed',result:{output:{answers}}}),'non-exact domain accepted');
}
await hooks['execute.after']({...question,status:'completed',result:{output:{answers:[[' Automatic '],['Engram'],['Auto']]}}});
const call=request();await hooks['execute.before'](call);if(!call.input.prompt.startsWith('## SDD Session Preflight'))throw Error('authority missing');
await reject(()=>hooks['execute.before']({...request(),sessionID:'child'}),'child authority');
await hooks['execute.after']({...call,status:'completed',result:{output:{sessionID:'worker',status:'running',output:'started'}}});
await hooks['execute.before'](request());
await hooks['execute.after']({...call,status:'completed',result:{output:{sessionID:'worker',status:'completed',output:'raw final result'},content:'<subagent>formatted NOT parsed</subagent>'}});
await reject(()=>hooks['execute.after']({...call,status:'completed',result:{output:{sessionID:'worker',status:'completed',output:''},content:'fake complete prose'}}),'empty raw accepted');
await reject(()=>hooks['execute.before'](request()),'failure not latched');
queue.push({type:'session.deleted',location:ctx.location,data:{sessionID:'root'}});wake?.();await new Promise(r=>setImmediate(r));
await reject(()=>hooks['execute.before'](request()),'deletion retained authority');
const errorQuestion={...question,sessionID:'errorroot'};await hooks['execute.after']({...errorQuestion,status:'completed',result:{output:{answers:[['Automatic'],['Engram'],['Auto']]}}});
const errorCall={...request(),sessionID:'errorroot'};await hooks['execute.before'](errorCall);
await reject(()=>hooks['execute.after']({...errorCall,status:'error',error:{message:'provider failure'}}),'tool error ignored');
await reject(()=>hooks['execute.before']({...request(),sessionID:'errorroot'}),'tool failure not latched');
await cleanup();if(disposals!==2||!signal.aborted)throw Error('cleanup');
let cleaned=0;await reject(()=>plugin.setup({...ctx,tool:{async hook(name,fn){if(name==='execute.after')throw Error('registration failed');return {async dispose(){cleaned++}}}}}),'registration failure swallowed');if(cleaned!==1)throw Error('partial registration leaked');
`)
}

func TestOpenCodeV2ReviewRelay(t *testing.T) {
	source, _ := Read("opencode/plugins-v2/opencode-review-transport.ts")
	for _, forbidden := range []string{"ReviewerResultSchema", "repository_context", "capture-result", "TRANSPORT_ISOLATION_SYSTEM", "session.hook", "readFile", "subject_hash", "retry", "OPENCODE_DISABLE"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("adapter owns semantics: %s", forbidden)
		}
	}
	runV2Plugin(t, "opencode-review-transport", `
const {EventEmitter}=await import('node:events');const children=[];
globalThis.__spawn=(command,args,options)=>{if(command!=='gentle-ai'||args.join(' ')!=='review opencode-transport')throw Error('unexpected process');if(options.env?.GENTLE_AI_OPENCODE_RELAY_CONTRACT!=='gentle-ai.opencode-relay/v2-staged')throw Error('missing negative host declaration');const child=new EventEmitter();child.stdout=new EventEmitter();child.stderr=new EventEmitter();child.stdin=new EventEmitter();child.frames=[];child.kill=()=>{child.killed=true};child.stdin.write=(line)=>{child.frames.push(JSON.parse(line));queueMicrotask(()=>child.stdout.emit('data',Buffer.from(JSON.stringify({schema:'gentle-ai.provider-transport/v1',operation:'prompt',nonce:'opaque',prompt:'GO PROMPT'})+'\n')))};child.stdin.end=(line)=>{child.frames.push(JSON.parse(line));queueMicrotask(()=>child.stdout.emit('data',Buffer.from(JSON.stringify({schema:'gentle-ai.provider-transport/v1',operation:'result',output:'GO RESULT'})+'\n')))};children.push(child);return child};
const make=async(location={directory:'/project',workspaceID:'one'})=>{const hooks={};let disposed=0,signal,wake;const queue=[];const cleanup=await plugin.setup({location,shell:{async hook(name,fn){const invocation={env:{KEEP:'yes'}};await fn(invocation);if(invocation.env.GENTLE_AI_OPENCODE_RELAY_CONTRACT!=='gentle-ai.opencode-relay/v2-staged'||invocation.env.KEEP!=='yes')throw Error('shell declaration');return {async dispose(){disposed++}}}},tool:{async hook(name,fn){hooks[name]=fn;return {async dispose(){disposed++}}}},event:{subscribe(opts){signal=opts.signal;signal.addEventListener('abort',()=>wake?.());return {[Symbol.asyncIterator]:async function*(){while(!signal.aborted){if(!queue.length)await new Promise(r=>wake=r);while(queue.length)yield queue.shift()}}}}}});return {hooks,cleanup,queue,event:async e=>{queue.push(e);wake?.();await new Promise(r=>setImmediate(r))},disposed:()=>disposed}};
const reject=async(fn)=>{let failed=false;try{await fn()}catch{failed=true}if(!failed)throw Error('expected refusal')};
const call=(id='call')=>({tool:'subagent',sessionID:'root',id,input:{agent:'review-risk',prompt:'opaque binding'}});
const completed=c=>({...c,status:'completed',result:{output:{sessionID:'child',status:'completed',output:'RAW BYTES'},content:'NEVER PARSE WRAPPER'}});
const a=await make(),b=await make();const c=call();await a.hooks['execute.before'](c);await b.hooks['execute.before'](c);if(children.length!==1||c.input.prompt!=='GO PROMPT')throw Error('duplicate/process materialization');const result=completed(c);await a.hooks['execute.after'](result);await b.hooks['execute.after'](result);if(children[0].frames[1].output!=='RAW BYTES'||result.result.output.output!=='GO RESULT'||!children[0].killed)throw Error('opaque completion');
await reject(()=>a.hooks['execute.after'](completed(c)));
const failed=call('orphan');await a.hooks['execute.before'](failed);await b.hooks['execute.before'](failed);const orphan=completed(failed);await a.cleanup();await reject(()=>b.hooks['execute.after'](orphan));if(JSON.stringify(orphan.result).includes('RAW BYTES'))throw Error('orphan advisory escaped');
// Recreate the owner after disposal for subsequent location and error cases.
const replacement=await make();a.hooks=replacement.hooks;a.cleanup=replacement.cleanup;a.event=replacement.event;a.disposed=replacement.disposed;
const normalSpawn=globalThis.__spawn;
globalThis.__spawn=(...args)=>{const child=normalSpawn(...args);child.stdin.write=()=>queueMicrotask(()=>child.emit('error',Error('native unavailable')));return child};
const denied=call('denied');await reject(()=>a.hooks['execute.before'](denied));if(!children.at(-1).killed||denied.input.prompt!=='opencode_review_transport_relay_refused')throw Error('start failure not contained');const deniedResult=completed(denied);await reject(()=>a.hooks['execute.after'](deniedResult));if(JSON.stringify(deniedResult).includes('RAW BYTES'))throw Error('failed start escaped');globalThis.__spawn=normalSpawn;

for(const extra of [{background:true},{sessionID:'reuse'}]){const c=call('unsafe');Object.assign(c.input,extra);const count=children.length;await reject(()=>a.hooks['execute.before'](c));if(children.length!==count)throw Error('unsafe dispatch spawned')}
for(const status of ['running','error']){const c=call(status);await a.hooks['execute.before'](c);const result=completed(c);if(status==='error'){result.status='error';result.error={message:'failure'}}else result.result.output.status='running';await a.hooks['execute.after'](result);if(!children.at(-1).frames[1].error||children.at(-1).frames[1].output)throw Error('nonterminal forwarded as result')}
const other=await make({directory:'/project',workspaceID:'two'});const x=call('isolation');await a.hooks['execute.before'](x);await other.hooks['execute.before'](call('isolation'));const first=children.at(-2),second=children.at(-1);await a.event({type:'session.deleted',location:{directory:'/wrong',workspaceID:'one'},data:{sessionID:'root'}});if(first.killed)throw Error('foreign deletion');await a.event({type:'session.deleted',location:{directory:'/project',workspaceID:'one'},data:{sessionID:'root'}});if(!first.killed||second.killed)throw Error('location/session cleanup');await other.cleanup();if(!second.killed)throw Error('dispose relay');await a.cleanup();await b.cleanup();if(a.disposed()!==3)throw Error('hook cleanup');
let disposed=0;await reject(()=>plugin.setup({location:{directory:'/project'},tool:{async hook(name){if(name==='execute.after')throw Error('registration');return {async dispose(){disposed++}}}}}));if(disposed!==1)throw Error('partial registration leak');
`)
}
