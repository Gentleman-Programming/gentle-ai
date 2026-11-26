package assets

import (
	"strings"
	"testing"
)

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
const runningTool=(call,input)=>({type:'tool',id:call.id,name:call.tool,state:{status:'running',input}});
const original=new WeakMap();
const make=async(location={directory:'/project',workspaceID:'one'},sdk={})=>{const hooks={};let disposed=0,signal,wake;const queue=[],parents=new Map();const cleanup=await plugin.setup({location,session:{async context({sessionID}){if(parents.has(sessionID))return parents.get(sessionID);return sdk.context?.(sessionID)},async get({sessionID}){return sdk.get?.(sessionID)},async wait({sessionID}){return sdk.wait?.(sessionID)}},shell:{async hook(name,fn){const invocation={env:{KEEP:'yes'}};await fn(invocation);if(invocation.env.GENTLE_AI_OPENCODE_RELAY_CONTRACT!=='gentle-ai.opencode-relay/v2-staged'||invocation.env.KEEP!=='yes')throw Error('shell declaration');return {async dispose(){disposed++}}}},tool:{async hook(name,fn){hooks[name]=name==='execute.before'?call=>{if(!original.has(call))original.set(call,{...call.input});const parent={type:'assistant',id:call.messageID,agent:call.agent,content:[runningTool(call,original.get(call))]};parents.set(call.sessionID,sdk.parent?sdk.parent(call,parent):[parent]);return fn(call)}:fn;return {async dispose(){disposed++}}}},event:{subscribe(opts){signal=opts.signal;signal.addEventListener('abort',()=>wake?.());return {[Symbol.asyncIterator]:async function*(){while(!signal.aborted){if(!queue.length)await new Promise(r=>wake=r);while(queue.length)yield queue.shift()}}}}}});return {hooks,cleanup,queue,event:async e=>{queue.push(e);wake?.();await new Promise(r=>setImmediate(r))},disposed:()=>disposed}};
const reject=async(fn)=>{let failed=false;try{await fn()}catch{failed=true}if(!failed)throw Error('expected refusal')};
const call=(id='call')=>({tool:'subagent',sessionID:'root',agent:'parent',messageID:'msg-'+id,id,input:{agent:'review-risk',prompt:'opaque binding'}});
const completed=c=>({...c,status:'completed',result:{output:{sessionID:'child',status:'completed',output:'RAW BYTES'},content:'NEVER PARSE WRAPPER'}});
const a=await make(),b=await make();const c=call();await a.hooks['execute.before'](c);await b.hooks['execute.before'](c);if(children.length!==1||c.input.prompt!=='GO PROMPT')throw Error('duplicate/process materialization');const result=completed(c);await a.hooks['execute.after'](result);await b.hooks['execute.after'](result);if(children[0].frames[1].output!=='RAW BYTES'||result.result.output.output!=='GO RESULT'||!children[0].killed)throw Error('opaque completion');
// Payload syntax is Go-owned: all reviewer roles use the same opaque wire.
const agents=['review-risk','review-resilience','review-readability','review-reliability','review-refuter','review-validator'];
const fence=String.fromCharCode(96).repeat(3);
const payloads=[
  ['valid JSON',' \r\n{\n\t"message": "café 🧪", "findings": []\n}  \n'],
  ['fenced JSON',fence+'json\n{"findings":[]}\n'+fence+'\n'],
  ['malformed JSON',' \n{"findings": [\t'],
  ['multiple objects','{"findings":[]}\n{"verdict":"pass"}'],
];
const parse=JSON.parse;
let payloadParseAttempts=0;
JSON.parse=(text,...args)=>{if(payloads.some(([,raw])=>raw===text))payloadParseAttempts++;return parse(text,...args)};
try {
  for(const agent of agents)for(const [name,raw] of payloads){
    const c=call(agent+':'+name);c.input.agent=agent;
    const originalPrompt=c.input.prompt;
    const count=children.length;
    await a.hooks['execute.before'](c);
    const child=children.at(-1);
    if(children.length!==count+1||child.frames[0].prompt!==originalPrompt||c.input.prompt!=='GO PROMPT')throw Error(agent+': prompt forwarding');
    // Go binds the dispatched host agent to the Task role; the prompt alone never selects it.
    if(child.frames[0].agent!==agent||Object.keys(child.frames[0]).sort().join()!=='agent,operation,prompt,schema')throw Error(agent+': start frame must name the dispatched agent');
    const result=completed(c);result.result.output.output=raw;
    await a.hooks['execute.after'](result);
    const frame=child.frames[1];
    if(child.frames.length!==2||frame.schema!=='gentle-ai.provider-transport/v1'||frame.operation!=='complete'||frame.nonce!=='opaque'||'error' in frame)throw Error(agent+': completion framing');
    if(typeof frame.output!=='string'||!Buffer.from(frame.output).equals(Buffer.from(raw)))throw Error(agent+': changed '+name);
    if(result.result.output.output!=='GO RESULT'||result.result.content!=='GO RESULT'||!child.killed)throw Error(agent+': native result substitution');
  }
  if(payloadParseAttempts!==0)throw Error('adapter attempted reviewer JSON parsing');
}finally{JSON.parse=parse}

// This mock only models a native transport refusal; Go tests prove the actual
// empty-output decoder contract. Empty strings must reach Go, not TS admission.
const unavailable=[
  ['failed hook',r=>{r.status='error';r.error={message:'host failure'}}],
  ['running child',r=>{r.result.output.status='running'}],
  ['failed child',r=>{r.result.output.status='error'}],
  ['missing child status',r=>{delete r.result.output.status}],
  ['missing child session',r=>{delete r.result.output.sessionID}],
  ['empty child session',r=>{r.result.output.sessionID=''}],
  ['missing raw output',r=>{delete r.result.output.output}],
  ['non-string raw output',r=>{r.result.output.output={findings:[]}}],
  ['empty raw output',r=>{r.result.output.output=''},''],
  ['whitespace raw output',r=>{r.result.output.output=' \r\n\t'},' \r\n\t'],
];
for(const agent of agents)for(const [name,mutate,raw] of unavailable){
  const c=call(agent+':'+name);c.input.agent=agent;
  await a.hooks['execute.before'](c);
  const child=children.at(-1);
  child.stdin.end=line=>{child.frames.push(JSON.parse(line));queueMicrotask(()=>child.emit('close',1))};
  const result=completed(c);mutate(result);
  await reject(()=>a.hooks['execute.after'](result));
  const frame=child.frames[1];
  if(child.frames.length!==2||frame.operation!=='complete'||frame.nonce!=='opaque'||!child.killed)throw Error(agent+': refusal lifecycle '+name);
  if(raw!==undefined){
    if(frame.output!==raw||'error' in frame)throw Error(agent+': empty bytes must reach Go');
  }else if('output' in frame||frame.error!=='opencode_task_host_output_unavailable')throw Error(agent+': unavailable output forwarded '+name);
  if(result.status==='completed'&&(result.result.output.status!=='unavailable'||result.result.output.reason!=='relay_unavailable'||result.result.content!=='opencode_review_transport_relay_refused (reason: relay_unavailable)'))throw Error(agent+': advisory escaped '+name);
  if(result.status==='error'&&result.error.message!=='host failure')throw Error('host failure was replaced');
}
await reject(()=>a.hooks['execute.after'](completed(c)));
const failed=call('orphan');await a.hooks['execute.before'](failed);await b.hooks['execute.before'](failed);const orphan=completed(failed);await a.cleanup();await reject(()=>b.hooks['execute.after'](orphan));if(JSON.stringify(orphan.result).includes('RAW BYTES'))throw Error('orphan advisory escaped');
// Recreate the owner after disposal for subsequent location and error cases.
const replacement=await make();a.hooks=replacement.hooks;a.cleanup=replacement.cleanup;a.event=replacement.event;a.disposed=replacement.disposed;
const normalSpawn=globalThis.__spawn;
globalThis.__spawn=(...args)=>{const child=normalSpawn(...args);child.stdin.write=()=>queueMicrotask(()=>child.emit('error',Error('native unavailable')));return child};
const denied=call('denied');await reject(()=>a.hooks['execute.before'](denied));if(!children.at(-1).killed||denied.input.prompt!=='opencode_review_transport_relay_refused')throw Error('start failure not contained');const deniedResult=completed(denied);await reject(()=>a.hooks['execute.after'](deniedResult));if(JSON.stringify(deniedResult).includes('RAW BYTES'))throw Error('failed start escaped');globalThis.__spawn=normalSpawn;

// Go's refused frame names one bounded reason; only allow-listed codes reach the
// parent, and any other relay text collapses to relay_unavailable.
// An optional cause beside the reason reaches the parent only when it is an
// allow-listed code; an absent or unknown cause keeps the reason-only refusal.
const refusedSpawn=(stage,reason,cause)=>(...args)=>{const child=normalSpawn(...args);const frame={schema:'gentle-ai.provider-transport/v1',operation:'refused',error:reason};if(cause!==undefined)frame.cause=cause;const refuse=()=>queueMicrotask(()=>{child.stdout.emit('data',Buffer.from(JSON.stringify(frame)+'\n'));child.emit('close',1)});if(stage==='start')child.stdin.write=line=>{child.frames.push(JSON.parse(line));refuse()};else child.stdin.end=line=>{child.frames.push(JSON.parse(line));refuse()};return child};
for(const [stage,reason,want,cause,wantCause] of [['start','agent_mismatch','agent_mismatch'],['start','stale_authority','stale_authority'],['start','/Users/someone/RAW CHILD TEXT','relay_unavailable'],['complete','output_refused','output_refused'],['complete','provider_failed','provider_failed'],['complete','RAW BYTES','relay_unavailable'],['complete','output_refused','output_refused','proof_path_out_of_scope','proof_path_out_of_scope'],['complete','output_refused','output_refused','role_capture_failed','role_capture_failed'],['complete','output_refused','output_refused','RAW BYTES at /Users/someone'],['complete','output_refused','output_refused',{nested:'RAW'}],['complete','RAW BYTES','relay_unavailable','proof_path_out_of_scope','proof_path_out_of_scope']]){
  globalThis.__spawn=refusedSpawn(stage,reason,cause);
  const c=call(stage+':'+want+':'+wantCause);let thrown;
  try{await a.hooks['execute.before'](c)}catch(error){thrown=error}
  if(stage==='start'&&(!thrown||thrown.reason!==want||thrown.message!=='opencode_review_transport_relay_refused (reason: '+want+')'||c.input.prompt!=='opencode_review_transport_relay_refused'))throw Error(stage+': before refusal reason '+reason+' '+thrown?.message);
  const result=completed(c);let after;
  try{await a.hooks['execute.after'](result)}catch(error){after=error}
  const text='opencode_review_transport_relay_refused (reason: '+want+(wantCause?', cause: '+wantCause:'')+')';
  if(!after||after.reason!==want||result.result.output.reason!==want||result.result.output.code!=='opencode_review_transport_relay_refused'||result.result.content!==text||after.message!==text)throw Error(stage+': parent refusal reason '+reason+' '+JSON.stringify(result.result));
  if(wantCause?(after.cause!==wantCause||result.result.output.cause!==wantCause):('cause' in after||'cause' in result.result.output))throw Error(stage+': parent refusal cause '+JSON.stringify(cause)+' '+JSON.stringify(result.result));
  if(JSON.stringify(result.result).includes('RAW')||JSON.stringify(result.result).includes('/Users/'))throw Error('relay text escaped');
}
globalThis.__spawn=normalSpawn;
for(const extra of [{background:true},{sessionID:'reuse'}]){const c=call('dispatch');Object.assign(c.input,extra);let thrown;try{await a.hooks['execute.before'](c)}catch(error){thrown=error}if(thrown?.reason!=='dispatch_refused')throw Error('dispatch refusal reason');const result=completed(c);await reject(()=>a.hooks['execute.after'](result));if(result.result.output.reason!=='dispatch_refused')throw Error('dispatch parent reason')}
for(const extra of [{background:true},{sessionID:'reuse'}]){const c=call('unsafe');Object.assign(c.input,extra);const count=children.length;await reject(()=>a.hooks['execute.before'](c));if(children.length!==count)throw Error('unsafe dispatch spawned')}
for(const status of ['running','error']){const c=call(status);await a.hooks['execute.before'](c);const result=completed(c);if(status==='error'){result.status='error';result.error={message:'failure'}}else result.result.output.status='running';await a.hooks['execute.after'](result);if(!children.at(-1).frames[1].error||children.at(-1).frames[1].output)throw Error('nonterminal forwarded as result')}
const other=await make({directory:'/project',workspaceID:'two'});const x=call('isolation');await a.hooks['execute.before'](x);await other.hooks['execute.before'](call('isolation'));const first=children.at(-2),second=children.at(-1);await a.event({type:'session.deleted',location:{directory:'/wrong',workspaceID:'one'},data:{sessionID:'root'}});if(first.killed)throw Error('foreign deletion');await a.event({type:'session.deleted',location:{directory:'/project',workspaceID:'one'},data:{sessionID:'root'}});if(!first.killed||second.killed)throw Error('location/session cleanup');await other.cleanup();if(!second.killed)throw Error('dispose relay');await a.cleanup();await b.cleanup();if(a.disposed()!==3)throw Error('hook cleanup');
let disposed=0;await reject(()=>plugin.setup({location:{directory:'/project'},tool:{async hook(name){if(name==='execute.after')throw Error('registration');return {async dispose(){disposed++}}}}}));if(disposed!==1)throw Error('partial registration leak');

// 2.4.2 acknowledges the tool before the child turn has finished. The SDK
// fixture stores the original calling tool input, independently of hook input.
const running=(c,id)=>({...c,status:'completed',result:{output:{sessionID:id,status:'running',output:'RUNNING ACK'},content:'RUNNING WRAPPER'}});
const user=(text='GO PROMPT')=>({id:'user',type:'user',text:'You are a subagent spawned by another session.\n'+text});
const assistant=(agent,text='RAW',finish='stop')=>({id:'assistant',type:'assistant',agent,time:{completed:3},finish,content:[{type:'reasoning',text:'NOT OUTPUT'},{type:'text',text}]});
const idle=(outcome='succeeded')=>({id:'idle',type:'idle',outcome});
const bound=(id,c)=>({id,parentID:c.sessionID,agent:c.input.agent});
const childTurn=(c,text)=>[user(),assistant(c.input.agent,text),idle()];
const pause=()=>{let resolve;const waiting=new Promise(r=>resolve=r);return {waiting,resolve}};
const lateContext=pause(), deleting=await make(undefined,{parent:(_c,p)=>lateContext.waiting.then(()=>[p])});
const deletedCall=call('deleted-during-lookup'), beforeDelete=children.length;
const deletedBefore=deleting.hooks['execute.before'](deletedCall).then(()=>null,error=>error);
await deleting.event({type:'session.deleted',location:{directory:'/project',workspaceID:'one'},data:{sessionID:deletedCall.sessionID}});
lateContext.resolve();
const deletionRefusal=await deletedBefore;
if(deletionRefusal?.message!=='opencode_review_transport_relay_refused (reason: relay_unavailable)'||deletedCall.input.prompt!=='opencode_review_transport_relay_refused'||children.length!==beforeDelete)throw Error('deleted parent dispatched or was not refused');
await deleting.cleanup();
const calls=new Map(), turns=new Map(), waits=new Map();
const fixture={parent:(c,p)=>[p,{...p,id:'unrelated',content:[runningTool(c,{...c.input,background:true})]}],get:id=>calls.get(id),context:id=>turns.get(id),wait:id=>{const gate=waits.get(id);gate.enter.resolve();return gate.finish.waiting}};
const staged=await make(undefined,fixture), duplicate=await make(undefined,fixture);
const begin=async(c,id,options={})=>{
  await staged.hooks['execute.before'](c);
  const relay=children.at(-1);
  calls.set(id,options.identity??bound(id,c));turns.set(id,options.messages??childTurn(c));
  const gate={enter:pause(),finish:pause()};waits.set(id,gate);
  return { relay, gate, result:running(c,id) };
};
const background=await make(undefined,{parent:(c,p)=>{p.content[0].state.input={...c.input,background:true};return [p]}});
const before=children.length, hidden=call('stripped-background');
await reject(()=>background.hooks['execute.before'](hidden));
const hiddenResult=running(hidden,'unstarted');await reject(()=>background.hooks['execute.after'](hiddenResult));
if(children.length!==before||hiddenResult.result.output.reason!=='dispatch_refused')throw Error('stripped explicit background dispatched');
await background.cleanup();
for(const mutate of [p=>p.id='wrong-message',p=>p.agent='wrong-parent-agent',p=>p.content[0].id='wrong-call',p=>p.content[0].name='wrong-tool',p=>p.content[0].state.input={agent:'review-refuter',prompt:'opaque binding'},p=>p.content[0].state.input={agent:'review-risk',prompt:'different prompt'},p=>p.content.push(p.content[0])]){
  const mismatch=await make(undefined,{parent:(c,p)=>{mutate(p);return [p]}});
  await reject(()=>mismatch.hooks['execute.before'](call('mismatched-parent')));
  await mismatch.cleanup();
}
if(children.length!==before)throw Error('unbound parent dispatched');

const firstCall=call('first'), secondCall=call('second');secondCall.input.agent='review-reliability';
const firstRun=await begin(firstCall,'child-first',{messages:[user(),assistant(firstCall.input.agent,'INTERIM','tool-calls'),assistant(firstCall.input.agent,'FIRST'),idle()]});
await duplicate.hooks['execute.before'](firstCall);
const secondRun=await begin(secondCall,'child-second',{messages:childTurn(secondCall,'SECOND')});
await reject(()=>staged.hooks['execute.after'](completed({...firstCall,messageID:'wrong-parent-turn'})));
if(firstRun.relay.frames.length!==1)throw Error('wrong calling message reached Go');
const firstDone=staged.hooks['execute.after'](firstRun.result);
const secondDone=staged.hooks['execute.after'](secondRun.result);
await Promise.race([Promise.all([firstRun.gate.enter.waiting,secondRun.gate.enter.waiting]),Promise.all([firstDone,secondDone]).then(()=>{throw Error('hook did not wait for either child')})]);
if(firstRun.relay.frames.length!==1||secondRun.relay.frames.length!==1)throw Error('running acknowledgement reached Go');
const sameCall=running(firstCall,'child-first');await reject(()=>staged.hooks['execute.after'](sameCall));
secondRun.gate.finish.resolve();await secondDone;
if(secondRun.relay.frames[1].output!=='SECOND'||secondRun.result.result.output.status!=='completed'||secondRun.result.result.content!=='GO RESULT'||firstRun.relay.frames.length!==1)throw Error('reversed second completion');
firstRun.gate.finish.resolve();await firstDone;await duplicate.hooks['execute.after'](firstRun.result);
await reject(()=>staged.hooks['execute.after'](running(firstCall,'child-first')));
if(firstRun.relay.frames.length!==2||firstRun.relay.frames[1].output!=='FIRST'||firstRun.result.result.content!=='GO RESULT')throw Error('original relay must capture once');

const failCase=async(name,options={},modify=()=>{})=>{
  const run=await begin(call(name),'child-'+name,options);modify(run.result);run.gate.finish.resolve();
  run.relay.stdin.end=line=>{run.relay.frames.push(JSON.parse(line));queueMicrotask(()=>run.relay.stdout.emit('data',Buffer.from(JSON.stringify({schema:'gentle-ai.provider-transport/v1',operation:'refused',error:'provider_failed'})+'\n')))};
  await reject(()=>staged.hooks['execute.after'](run.result));
  if(run.relay.frames.length!==2||'output' in run.relay.frames[1]||run.result.result.output.reason!=='provider_failed'||!run.relay.killed)throw Error('not a bounded provider failure: '+name);
};
for(const [name,identity] of [['child-id',{id:'another',parentID:'root',agent:'review-risk'}],['parent',{id:'child-parent',parentID:'unrelated',agent:'review-risk'}],['agent',{id:'child-agent',parentID:'root',agent:'review-refuter'}]])await failCase(name,{identity});
await failCase('hook-prompt',{},r=>{r.input={...r.input,prompt:'different prompt'}});
for(const [name,messages] of [
  ['child-prompt',[user('GO PROMPT malicious suffix'),assistant('review-risk'),idle()]],
  ['wrong-author',[user(),assistant('review-refuter'),idle()]],
  ['followup',[user(),assistant('review-risk'),idle(),user(),assistant('review-risk'),idle()]],
  ['extra-response',[user(),assistant('review-risk'),assistant('review-risk'),idle()]],
  ['idle-not-final',[user(),assistant('review-risk'),idle(),user()]],
  ['failed',[user(),assistant('review-risk'),idle('failed')]],
  ['aborted',[user(),assistant('review-risk'),idle('aborted')]],
  ['assistant-error',[user(),{...assistant('review-risk'),error:{message:'failed'}},idle()]],
  ['missing-idle',[user(),assistant('review-risk')]],
  ['nonterminal',[user(),assistant('review-risk','RAW','tool-calls'),idle()]],
])await failCase(name,{messages});

for(const type of ['relay disconnection','parent deletion','child deletion','plugin disposal']){
  const id='child-'+type,c=call(type);
  const run=await begin(c,id);
  const done=reject(()=>staged.hooks['execute.after'](run.result));
  await Promise.race([run.gate.enter.waiting,done.then(()=>{throw Error('hook did not wait for '+type)})]);
  if(type==='relay disconnection')run.relay.emit('close',1);
  else if(type==='plugin disposal')await staged.cleanup();
  else await staged.event({type:'session.deleted',location:{directory:'/project',workspaceID:'one'},data:{sessionID:type==='parent deletion'?c.sessionID:id}});
  await done;run.gate.finish.resolve();await new Promise(r=>setImmediate(r));
  if(run.relay.frames.length!==1||!run.relay.killed||run.result.result.output.reason!=='relay_unavailable')throw Error('not cleaned without capture: '+type);
}
await duplicate.cleanup();
`)
}
