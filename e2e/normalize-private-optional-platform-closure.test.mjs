import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { normalize } from './normalize-private-optional-platform-closure.mjs';

const keep = 'node_modules/keep';
const foreign = 'node_modules/keep/node_modules/foreign';
const later = 'node_modules/zforeign';
const platform = { os: 'linux', cpu: 'x64', libc: 'glibc' };
function fixture(run) {
  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'owned-normalize-'));
  const root = path.join(stage, 'project');
  fs.mkdirSync(root);
  const lock = { packages: {} };
  function add(key, rules = {}) {
    const name = key.split('node_modules/').at(-1);
    lock.packages[key] = { name, version: '1.0.0', optional: true, ...rules };
    fs.mkdirSync(path.join(root, key), { recursive: true });
    fs.writeFileSync(path.join(root, key, 'package.json'), JSON.stringify({ name, version: '1.0.0' }));
    fs.writeFileSync(path.join(root, key, 'README.md'), 'real package');
    fs.mkdirSync(path.join(root, key, 'bin'));
  }
  add(keep, { optional: false });
  add(foreign, { os: ['aix'], cpu: ['ppc64'] });
  add(later, { cpu: ['arm64'] });
  // Same physical collector contract as the installer: directories never follow links.
  function scan() {
    const actual = [];
    function directory(key) {
      assert.ok(fs.lstatSync(path.join(root, key)).isDirectory());
    }
    function walk(modules) {
      if (!fs.existsSync(path.join(root, modules))) return;
      directory(modules);
      for (const entry of fs.readdirSync(path.join(root, modules))) {
        if (entry === '.bin' || entry === '.package-lock.json') continue;
        const key = `${modules}/${entry}`;
        directory(key);
        const packages = entry.startsWith('@') ? fs.readdirSync(path.join(root, key)).map(x => `${key}/${x}`) : [key];
        for (const item of packages) {
          directory(item);
          actual.push(item);
          walk(`${item}/node_modules`);
        }
      }
    }
    walk('node_modules');
    return actual;
  }
  const context = { root, lock, expectedPaths: [keep], platform };
  const invoke = (ops = {}) => normalize({ ...context, actualPaths: scan() }, ops);
  const snapshot = () => [foreign, later].map(key => fs.readFileSync(path.join(root, key, 'package.json'), 'utf8'));
  try { run({ root, lock, add, scan, context, invoke, snapshot }); }
  finally { fs.rmSync(stage, { recursive: true, force: true }); }
}

fixture(({ invoke, scan, root }) => {
  invoke();
  assert.deepEqual(scan(), [keep]);
  assert.equal(fs.readFileSync(path.join(root, keep, 'README.md'), 'utf8'), 'real package');
});
const rejectionCases = {
  'unsafe later plan preserves earlier candidate': ({ lock }) => { lock.packages[later].optional = false; },
  'optional must be boolean true': ({ lock }) => { lock.packages[later].optional = 'true'; },
  'applicable extra': ({ lock }) => { lock.packages[later].cpu = ['x64']; },
  'unknown extra': ({ lock }) => { delete lock.packages[later]; },
  'malformed platform array': ({ lock }) => { lock.packages[later].os = 'aix'; },
  'malformed later rule despite excluded OS': ({ lock }) => { lock.packages[later].os = ['aix']; lock.packages[later].libc = [false]; },
  'wrong name': ({ root }) => { fs.writeFileSync(path.join(root, later, 'package.json'), '{"name":"other","version":"1.0.0"}'); },
  'wrong version': ({ root }) => { fs.writeFileSync(path.join(root, later, 'package.json'), '{"name":"zforeign","version":"2.0.0"}'); },
  'invalid metadata JSON': ({ root }) => { fs.writeFileSync(path.join(root, later, 'package.json'), '{'); },
  'missing expected package': ({ context }) => { context.expectedPaths.push('node_modules/missing'); },
  'nested expected subtree': ({ context, add }) => { const child = `${later}/node_modules/child`; add(child); context.expectedPaths.push(child); },
  'root pin excluded': ({ add }) => { add('node_modules/gentle-pi', { os: ['aix'] }); },
  'required child inside removable parent': ({ add }) => { add(`${later}/node_modules/child`, { optional: false, os: ['aix'] }); },
  'unknown child inside removable parent': ({ add, lock }) => { const child = `${later}/node_modules/child`; add(child); delete lock.packages[child]; },
};
for (const [name, mutate] of Object.entries(rejectionCases)) {
  fixture(f => {
    mutate(f);
    const before = f.snapshot();
    let effects = 0;
    assert.throws(() => f.invoke({ remove() { effects++; } }), undefined, name);
    assert.equal(effects, 0, `${name}: zero effects`);
    assert.deepEqual(f.snapshot(), before, `${name}: preserved preimage`);
  });
}
for (const rules of [{ os: ['!linux'] }, { cpu: ['!x64'] }, { libc: ['musl'] }, { os: ['!any'] }]) {
  fixture(({ lock, invoke, scan }) => {
    lock.packages[later] = { ...lock.packages[later], cpu: undefined, ...rules };
    invoke();
    assert.deepEqual(scan(), [keep]);
  });
}
for (const rules of [{ os: ['any'] }, { cpu: [] }, { libc: ['glibc', '!musl'] }]) {
  fixture(({ lock, invoke }) => {
    lock.packages[later] = { ...lock.packages[later], cpu: undefined, ...rules };
    assert.throws(() => invoke(), /applicable/);
  });
}
for (const target of ['package', 'metadata', 'parent', 'root']) {
  fixture(f => {
    const key = target === 'root' ? f.root : path.join(f.root, target === 'parent' ? `${keep}/node_modules` : later);
    const original = target === 'metadata' ? path.join(key, 'package.json') : key;
    fs.renameSync(original, `${original}.held`);
    fs.symlinkSync(`${original}.held`, original);
    let effects = 0;
    assert.throws(() => f.invoke({ remove() { effects++; } }));
    assert.equal(effects, 0, `${target} symlink rejected before effects`);
  });
}
fixture(f => {
  fs.rmSync(path.join(f.root, later, 'package.json'));
  fs.mkdirSync(path.join(f.root, later, 'package.json'));
  assert.throws(() => f.invoke(), /metadata/);
});
fixture(f => {
  assert.throws(() => normalize({ ...f.context, actualPaths: [keep, '../escape'] }), /path/);
  assert.ok(fs.existsSync(path.join(f.root, foreign)));
});
fixture(f => {
  let effects = 0;
  assert.throws(() => f.invoke({
    beforeApply() { fs.appendFileSync(path.join(f.root, later, 'package.json'), ' '); },
    remove() { effects++; }
  }), /preimage/);
  assert.equal(effects, 0);
});
fixture(f => {
  fs.symlinkSync(f.root, path.join(f.root, foreign, 'bin', 'outside'));
  f.invoke();
  assert.ok(fs.existsSync(path.join(f.root, keep, 'package.json')), 'inner symlink is unlinked, not followed');
});
fixture(f => {
  f.add('node_modules/@scope/foreign', { libc: ['!glibc'] });
  f.add(`${later}/node_modules/child`, { os: ['aix'] });
  f.invoke();
  assert.deepEqual(f.scan(), [keep], 'scoped and nested foreign packages normalize');
});
for (const target of ['directory', 'metadata']) {
  fixture(f => {
    let effects = 0;
    assert.throws(() => f.invoke({
      beforeApply() {
        const original = path.join(f.root, later, target === 'metadata' ? 'package.json' : '');
        fs.renameSync(original, `${original}.held`);
        fs.symlinkSync(`${original}.held`, original);
      },
      remove() { effects++; }
    }));
    assert.equal(effects, 0, `fresh ${target} symlink: zero effects`);
  });
}
fixture(f => {
  const before = f.snapshot();
  assert.throws(() => f.invoke({ remove() { throw Error('first removal failure'); } }), /first removal/);
  assert.deepEqual(f.snapshot(), before, 'first-effect failure preserves packages');
});
fixture(f => {
  let calls = 0;
  assert.throws(() => f.invoke({ remove(key) {
    if (++calls === 2) throw Error('injected removal failure');
    fs.rmSync(key, { recursive: true });
  } }), /injected removal/);
  // Model the installer's EXIT trap: a failed private stage is never published.
  fs.rmSync(f.root, { recursive: true });
  assert.equal(fs.existsSync(f.root), false, 'rollback leaves no project residue');
});
fixture(f => {
  f.invoke({ remove() {} });
  assert.notDeepEqual(f.scan().sort(), f.context.expectedPaths.slice().sort(), 'postscan detects residue');
});
console.log('owned optional platform normalization source controls passed');
