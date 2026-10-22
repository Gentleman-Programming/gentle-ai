import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const helper = fileURLToPath(new URL('../scripts/provision-gentle-shell-private-global.mjs', import.meta.url));
const sha = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const identity = target => { const stat = fs.lstatSync(target, { bigint: true }); return `${stat.dev}:${stat.ino}`; };
function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shared-recovery-'));
  fs.chmodSync(root, 0o700);
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of ['state', 'prefix', 'agent', 'state/prefix.preimage', 'state/agent.preimage', 'runtime/node/bin', 'runtime/node/lib/node_modules/npm/bin']) {
    fs.mkdirSync(path.join(root, name), { recursive: true, mode: 0o700 });
  }
  // Unused physical files: restore must never spawn Node/npm or use the network.
  for (const name of ['runtime/node/bin/node', 'runtime/node/lib/node_modules/npm/bin/npm-cli.js']) {
    fs.writeFileSync(path.join(root, name), 'unused', { mode: 0o600 });
  }
  const prefix = path.join(root, 'prefix'), agent = path.join(root, 'agent');
  const savedSettings = '{"packages":["existing"],"npmCommand":"stock-npm"}';
  const hashes = {};
  for (const [name, file, bytes] of [['prefix.preimage', 'original', 'saved'], ['agent.preimage', 'settings.json', savedSettings]]) {
    fs.writeFileSync(path.join(root, 'state', name, file), bytes, { mode: 0o600 });
    hashes[name] = sha(JSON.stringify([['', 'dir', 0o700], [file, 'file', 0o600, sha(bytes)]]));
  }
  const selectionPath = path.join(root, 'state/selection.json');
  fs.writeFileSync(selectionPath, JSON.stringify({ mode: 'shared', prefix, agent, finalPrefix: prefix, finalRoot: root, prefixSHA: hashes['prefix.preimage'], agentSHA: hashes['agent.preimage'] }), { mode: 0o600 });
  const binding = { selectionSHA: sha(fs.readFileSync(selectionPath)), rootID: identity(root), prefixID: identity(prefix), agentID: identity(agent) };
  return { root, prefix, agent, selectionPath, binding, savedSettings };
}
function restore(f) {
  return spawnSync(process.execPath, [helper, f.root, f.prefix, f.agent, f.prefix, f.root, 'shared', 'restore', JSON.stringify(f.binding)], {
    env: { PATH: '/usr/bin:/bin', HOME: f.root }, timeout: 10000, encoding: 'utf8', maxBuffer: 4096,
  });
}

test('restore quarantines broken live trees and restores saved files', t => {
  const f = fixture(t);
  fs.symlinkSync('missing', path.join(f.prefix, 'dangling'));
  fs.writeFileSync(path.join(f.agent, 'settings.json'), '{"npmCommand":"missing/runtime"}', { mode: 0o600 });
  const large = fs.openSync(path.join(f.agent, 'large'), 'wx', 0o600);
  fs.ftruncateSync(large, 33554433);
  fs.closeSync(large);
  const result = restore(f);
  assert.equal(result.status, 0, result.stderr);
  assert.equal(fs.readFileSync(path.join(f.prefix, 'original'), 'utf8'), 'saved');
  assert.equal(fs.readFileSync(path.join(f.agent, 'settings.json'), 'utf8'), f.savedSettings);
  const entries = fs.readdirSync(path.join(f.root, 'state'));
  const prefixQ = entries.find(name => name.startsWith('prefix.preimage.quarantine-'));
  const agentQ = entries.find(name => name.startsWith('agent.preimage.quarantine-'));
  assert.equal(fs.readlinkSync(path.join(f.root, 'state', prefixQ, 'dangling')), 'missing');
  assert.equal(fs.statSync(path.join(f.root, 'state', agentQ, 'large')).size, 33554433);
  assert.equal(fs.readFileSync(path.join(f.root, 'state', agentQ, 'settings.json'), 'utf8'), '{"npmCommand":"missing/runtime"}');
  assert.equal(restore(f).status, 0);
  assert.deepEqual(fs.readdirSync(path.join(f.root, 'state')).sort(), entries.sort());
});

test('a corrupt second preimage refuses before either target changes', t => {
  const f = fixture(t);
  fs.writeFileSync(path.join(f.prefix, 'current'), 'keep', { mode: 0o600 });
  fs.writeFileSync(path.join(f.root, 'state/agent.preimage/settings.json'), 'corrupt');
  assert.notEqual(restore(f).status, 0);
  assert.equal(fs.readFileSync(path.join(f.prefix, 'current'), 'utf8'), 'keep');
  assert.equal(fs.readdirSync(path.join(f.root, 'state')).filter(name => name.includes('quarantine')).length, 0);
});

test('changed selection or replaced root invalidates confirmed authority', t => {
  for (const change of ['selection', 'root']) {
    const f = fixture(t);
    fs.writeFileSync(path.join(f.agent, 'current'), 'keep', { mode: 0o600 });
    if (change === 'selection') fs.appendFileSync(f.selectionPath, '\n');
    else { fs.renameSync(f.prefix, f.prefix + '.old'); fs.mkdirSync(f.prefix, { mode: 0o700 }); }
    assert.notEqual(restore(f).status, 0);
    assert.equal(fs.readFileSync(path.join(f.agent, 'current'), 'utf8'), 'keep');
    assert.equal(fs.readdirSync(path.join(f.root, 'state')).filter(name => name.includes('quarantine')).length, 0);
  }
});
