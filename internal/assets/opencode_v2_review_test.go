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
const make=async(location={directory:'/project',workspaceID:'one'})=>{const hooks={};let disposed=0,signal,wake;const queue=[];const cleanup=await plugin.setup({location,shell:{async hook(name,fn){const invocation={env:{KEEP:'yes'}};await fn(invocation);if(invocation.env.GENTLE_AI_OPENCODE_RELAY_CONTRACT!=='gentle-ai.opencode-relay/v2-staged'||invocation.env.KEEP!=='yes')throw Error('shell declaration');return {async dispose(){disposed++}}}},tool:{async hook(name,fn){hooks[name]=fn;return {async dispose(){disposed++}}}},event:{subscribe(opts){signal=opts.signal;signal.addEventListener('abort',()=>wake?.());return {[Symbol.asyncIterator]:async function*(){while(!signal.aborted){if(!queue.length)await new Promise(r=>wake=r);while(queue.length)yield queue.shift()}}}}}});return {hooks,cleanup,queue,event:async e=>{queue.push(e);wake?.();await new Promise(r=>setImmediate(r))},disposed:()=>disposed}};
const reject=async(fn)=>{let failed=false;try{await fn()}catch{failed=true}if(!failed)throw Error('expected refusal')};
const call=(id='call')=>({tool:'subagent',sessionID:'root',id,input:{agent:'review-risk',prompt:'opaque binding'}});
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
  if(result.status==='completed'&&(result.result.output.status!=='unavailable'||result.result.content!=='opencode_review_transport_relay_refused'))throw Error(agent+': advisory escaped '+name);
  if(result.status==='error'&&result.error.message!=='host failure')throw Error('host failure was replaced');
}
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
