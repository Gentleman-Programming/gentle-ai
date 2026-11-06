import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import vm from 'node:vm';
import test from 'node:test';

const source = fs.readFileSync(new URL('../scripts/provision-gentle-shell-private-global.mjs', import.meta.url), 'utf8');
function uniqueSlice(start, end) {
  const offset = source.indexOf(start), finish = source.indexOf(end, offset + start.length);
  assert.ok(offset >= 0 && finish > offset, 'live publication seam missing');
  assert.equal(source.lastIndexOf(start), offset, 'ambiguous publication seam');
  return source.slice(offset, finish);
}
// Execute the actual publication block with real temporary files. This isolated
// primitive proof does not qualify npm acquisition or native supplier readback.
const definitions = uniqueSlice('const uid = process.getuid();', 'for (const value of [root, prefix, agent, finalPrefix, finalRoot])');
const publication = uniqueSlice('    const stage = path.join(agent, `.gentle-shell-settings-${process.pid}`);', '  } else {\n    const declaration = settings.packages?.some');
for (const failDirectorySync of [false, true]) {
  test(`shared publication ${failDirectorySync ? 'propagates post-rename sync failure' : 'syncs renamed settings before graph witness'}`, t => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'shared-settings-sync-'));
    fs.chmodSync(root, 0o700);
    t.after(() => fs.rmSync(root, { recursive: true, force: true }));
    const agent = path.join(root, 'agent'), state = path.join(root, 'state');
    fs.mkdirSync(agent, { mode: 0o700 });
    fs.mkdirSync(state, { mode: 0o700 });
    const settingsPath = path.join(agent, 'settings.json');
    fs.writeFileSync(settingsPath, '{"theme":"before"}\n', { mode: 0o600 });
    const events = [], descriptors = new Map();
    let renamed = false;
    const fault = Object.assign(new Error('injected agent directory sync failure'), { code: 'EIO' });
    const monitored = Object.create(fs);
    monitored.openSync = (...args) => {
      const fd = fs.openSync(...args);
      descriptors.set(fd, args[0]);
      return fd;
    };
    monitored.fsyncSync = fd => {
      const file = descriptors.get(fd);
      events.push(['sync', file, renamed]);
      if (failDirectorySync && renamed && file === agent) throw fault;
      return fs.fsyncSync(fd);
    };
    monitored.closeSync = fd => {
      events.push(['close', descriptors.get(fd), renamed]);
      descriptors.delete(fd);
      return fs.closeSync(fd);
    };
    monitored.renameSync = (from, to) => {
      fs.renameSync(from, to);
      renamed = true;
      events.push(['rename', to, true]);
    };
    const publish = vm.runInNewContext(`${definitions}\n(agent, settingsPath, settings, state, observed) => {\n${publication}\n}`, {
      fs: monitored, path, crypto, process, Buffer,
    });
    const settings = { theme: 'after', packages: ['existing', 'selected'], npmCommand: ['node', 'npm'] };
    let failure;
    try { publish(agent, settingsPath, settings, state, []); } catch (error) { failure = error; }
    if (failDirectorySync) {
      assert.equal(failure, fault, 'publication must propagate the post-rename failure');
      assert.equal(fs.existsSync(path.join(state, 'global-graph.json')), false, 'no successful graph witness after uncertain publication');
    } else {
      assert.equal(failure, undefined);
      const rename = events.findIndex(event => event[0] === 'rename');
      const synced = events.findIndex(event => event[0] === 'sync' && event[1] === agent && event[2]);
      const witness = events.findIndex(event => event[0] === 'sync' && event[1] === path.join(state, 'global-graph.json'));
      assert.ok(rename >= 0 && synced > rename && witness > synced, 'rename, agent-directory sync, then graph witness');
      assert.deepEqual(JSON.parse(fs.readFileSync(path.join(state, 'global-graph.json'))), []);
    }
    assert.deepEqual(JSON.parse(fs.readFileSync(settingsPath)), settings, 'failure after rename preserves changed settings for recovery');
    assert.equal(descriptors.size, 0, 'file and directory descriptors closed on every path');
    assert.equal(fs.readdirSync(agent).some(name => name.startsWith('.gentle-shell-settings-')), false);
  });
}
