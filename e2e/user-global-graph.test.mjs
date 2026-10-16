import assert from 'node:assert/strict';
import fs from 'node:fs';
import crypto from 'node:crypto';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import vm from 'node:vm';
import { fileURLToPath, pathToFileURL } from 'node:url';
import test from 'node:test';
import { projectGlobalGraph } from '../scripts/user-global-graph.mjs';

// Execute the current provisioner's last comparison with semantic evidence in
// memory. No acquisition, supplier code, native execution or physical proof.
const helperSource = fs.readFileSync(new URL('../scripts/provision-gentle-shell-private-global.mjs', import.meta.url), 'utf8');
const comparison = helperSource.slice(helperSource.indexOf('    const declaration = settings.packages?'),
  helperSource.lastIndexOf('\n  }\n  if (!read(lockPath)'));
async function compareGraph(observed, expected, lock, retainedPaths) {
  const root = fileURLToPath(new URL('../scripts/', import.meta.url));
  const finalPrefix = '/fixture/prefix', finalRoot = '/fixture/root', packageRoot = '/fixture/prefix/lib/node_modules/gentle-pi';
  const witness = Buffer.from(`${JSON.stringify(expected)}\n`);
  const context = vm.createContext({
    root, finalPrefix, finalRoot, packageRoot, path, pathToFileURL, Buffer,
    observed, lock, retained: { paths: retainedPaths }, preparing: false,
    settings: { packages: [packageRoot], npmCommand: [path.join(finalRoot, 'runtime/node/bin/node'),
      path.join(finalRoot, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'), '--prefix', finalPrefix] },
    graphPath: '/fixture/graph.json',
    read: name => { assert.equal(name, '/fixture/graph.json'); return witness; },
    writeExclusive: () => { assert.fail('verification must not rewrite the graph witness'); },
    reject: message => { throw Error(message); },
  });
  await new vm.Script(`(async () => { ${comparison}\n })()`, {
    filename: 'current-provisioner-comparison.mjs',
    importModuleDynamically: vm.constants.USE_MAIN_CONTEXT_DEFAULT_LOADER,
  }).runInContext(context);
  assert.equal(witness.toString(), `${JSON.stringify(expected)}\n`);
}

for (const profile of ['modern', 'prior']) {
  test(`${profile}: current helper compares 297 authenticated rows against the original 272-row witness`, async () => {
    const lock = JSON.parse(fs.readFileSync(new URL(`../scripts/user-locks/${profile}/package-lock.json`, import.meta.url)));
    const row = key => ({ directory: `lib/${key}`, name: lock.packages[key].name ?? key.split('node_modules/').at(-1),
      version: lock.packages[key].version, integrity: lock.packages[key].integrity });
    const matches = (rule, value) => !rule || (!rule.includes(`!${value}`) && !rule.includes('!any') &&
      (!rule.some(item => !item.startsWith('!')) || rule.includes(value) || rule.includes('any')));
    const applicable = record => matches(record.os, 'linux') && matches(record.cpu, 'x64') && matches(record.libc, 'glibc');
    const keys = Object.keys(lock.packages).filter(Boolean);
    const expected = keys.filter(key => !lock.packages[key].optional || applicable(lock.packages[key])).map(row);
    const retained = keys.filter(key => key.includes('/node_modules/@esbuild/') && lock.packages[key].optional && !applicable(lock.packages[key]));
    const actual = [...expected, ...retained.map(row)];
    assert.equal(keys.length, 315);
    assert.equal(expected.length, 272);
    assert.equal(actual.length, 297);
    const preimage = JSON.stringify({ actual, expected, lock, retained });
    await compareGraph(actual, expected, lock, retained);
    assert.equal(JSON.stringify({ actual, expected, lock, retained }), preimage);
    await compareGraph(expected, expected, lock, retained);
    await assert.rejects(compareGraph(actual.slice(1), expected, lock, retained), /global graph changed/);
    const mutated = structuredClone(actual);
    mutated.at(-1).integrity = `sha512-${Buffer.alloc(64).toString('base64')}`;
    await assert.rejects(compareGraph(mutated, expected, lock, retained), /global graph changed/);
  });
}

// This pure projection follows, never replaces, the provisioner's physical,
// retained-inventory, full-source-byte, range, bin, native and settings checks.
const optionalPath = 'node_modules/@earendil-works/pi-coding-agent/node_modules/@esbuild/darwin-arm64';
function fixture(profile) {
  const lock = JSON.parse(fs.readFileSync(new URL(`../scripts/user-locks/${profile}/package-lock.json`, import.meta.url)));
  const row = key => ({ directory: `lib/${key}`, name: lock.packages[key].name ?? key.split('node_modules/').at(-1),
    version: lock.packages[key].version, integrity: lock.packages[key].integrity });
  assert.equal(lock.packages[optionalPath].optional, true);
  assert.deepEqual(lock.packages[optionalPath].os, ['darwin']);
  const expected = Object.keys(lock.packages[''].dependencies).map(name => row(`node_modules/${name}`));
  return { lock, row, expected, extra: row(optionalPath), retained: [optionalPath] };
}

for (const profile of ['modern', 'prior']) {
  test(`${profile}: known retained optional appears or disappears without graph drift`, () => {
    const f = fixture(profile);
    for (const index of [0, 2, f.expected.length]) {
      const actual = [...f.expected.slice(0, index), f.extra, ...f.expected.slice(index)];
      const before = JSON.stringify({ actual, lock: f.lock, retained: f.retained });
      assert.deepEqual(projectGlobalGraph(actual, f.lock, f.retained), f.expected);
      assert.equal(JSON.stringify({ actual, lock: f.lock, retained: f.retained }), before, 'projection must not modify evidence');
    }
    assert.deepEqual(projectGlobalGraph(f.expected, f.lock, f.retained), f.expected);
  });

  test(`${profile}: complete declared platform graph tolerates all retained esbuild extras`, () => {
    const f = fixture(profile);
    // Declarative corpus coverage, not npm/runtime acquisition proof.
    const matches = (rule, value) => !rule || (!rule.includes(`!${value}`) &&
      !rule.includes('!any') && (!rule.some(item => !item.startsWith('!')) || rule.includes(value) || rule.includes('any')));
    const applicable = entry => matches(entry.os, 'linux') && matches(entry.cpu, 'x64') && matches(entry.libc, 'glibc');
    const keys = Object.keys(f.lock.packages).filter(Boolean);
    const expected = keys.filter(key => !f.lock.packages[key].optional || applicable(f.lock.packages[key])).map(f.row);
    const retained = keys.filter(key => key.includes('/node_modules/@esbuild/') &&
      f.lock.packages[key].optional === true && !applicable(f.lock.packages[key]));
    const actual = [...expected, ...retained.map(f.row)];
    assert.equal(expected.length, 272);
    assert.equal(actual.length, 297);
    assert.deepEqual(projectGlobalGraph(actual, f.lock, retained), expected);
  });

  const mutations = {
    'unknown name': row => { row.name = 'foreign'; },
    'unknown version': row => { row.version = '9.9.9'; },
    'changed integrity': row => { row.integrity = `sha512-${Buffer.alloc(64).toString('base64')}`; },
    'relocated same identity': row => { row.directory = 'lib/node_modules/@esbuild/darwin-arm64'; },
    'directory alias': row => { row.directory = `lib/./${optionalPath}`; },
    'escaping directory': row => { row.directory = `lib/../${optionalPath}`; },
  };
  for (const [scenario, mutate] of Object.entries(mutations)) {
    test(`${profile}: ${scenario} remains visible to the graph seal`, () => {
      const f = fixture(profile);
      mutate(f.extra);
      const actual = [...f.expected, f.extra];
      assert.deepEqual(projectGlobalGraph(actual, f.lock, f.retained), actual);
      assert.notDeepEqual(projectGlobalGraph(actual, f.lock, f.retained), f.expected);
    });
  }

  test(`${profile}: unretained optional and required nodes cannot disappear`, () => {
    const f = fixture(profile);
    const actual = [...f.expected, f.extra];
    assert.deepEqual(projectGlobalGraph(actual, f.lock, []), actual);
    f.lock.packages[optionalPath].optional = false;
    assert.deepEqual(projectGlobalGraph(actual, f.lock, f.retained), actual);
    // Even a forged optional flag/retained list cannot hide a selected root.
    const rootPath = 'node_modules/typebox';
    f.lock.packages[rootPath].optional = true;
    assert.deepEqual(projectGlobalGraph(f.expected, f.lock, [rootPath]), f.expected);
  });

  test(`${profile}: malformed and duplicate retained rows remain visible`, () => {
    const f = fixture(profile);
    for (const actual of [
      [...f.expected, f.extra, { ...f.extra }],
      [...f.expected, { ...f.extra, unexpected: true }],
      [...f.expected, { ...f.extra, directory: null }],
    ]) {
      assert.deepEqual(projectGlobalGraph(actual, f.lock, f.retained), actual);
    }
  });

  test(`${profile}: applicable optional nodes and missing required nodes still drift`, () => {
    const f = fixture(profile);
    const applicablePath = optionalPath.replace('darwin-arm64', 'linux-x64');
    assert.equal(f.lock.packages[applicablePath].optional, true);
    const actual = [...f.expected, f.row(applicablePath)];
    assert.deepEqual(projectGlobalGraph(actual, f.lock, f.retained), actual);
    assert.notDeepEqual(projectGlobalGraph(actual, f.lock, f.retained), f.expected);
    const missing = f.expected.slice(1);
    assert.notDeepEqual(projectGlobalGraph(missing, f.lock, f.retained), f.expected);
  });
}

// Actual provisioner boundary: caller-owned applicability/root validation must
// refuse before projection (the projection module is deliberately not staged).
for (const profile of ['modern', 'prior']) {
  for (const [scenario, retainedPath] of [
    ['applicable optional', optionalPath.replace('darwin-arm64', 'linux-x64')],
    ['selected root', 'node_modules/typebox'],
    ['required package', 'node_modules/@earendil-works/pi-ai'],
    ['unknown package', 'node_modules/foreign'],
  ]) {
    test(`${profile}: caller refuses retained ${scenario} before projection`, t => {
      const root = fs.mkdtempSync(path.join(os.tmpdir(), 'optional-projection-'));
      fs.chmodSync(root, 0o700);
      t.after(() => fs.rmSync(root, { recursive: true, force: true }));
      const source = `runtime/${profile === 'prior' ? 'prior' : 'project'}`;
      const directories = ['state', 'agent', 'home', 'tmp', 'config', 'runtime/node/bin',
        'runtime/node/lib/node_modules/npm/bin', `user-locks/${profile}`, source,
        `${source}/.gentle-shell-optional`, 'prefix/lib/node_modules/@earendil-works/pi-coding-agent'];
      for (const name of directories) fs.mkdirSync(path.join(root, name), { recursive: true, mode: 0o700 });
      fs.copyFileSync(process.execPath, path.join(root, 'runtime/node/bin/node'));
      fs.chmodSync(path.join(root, 'runtime/node/bin/node'), 0o700);
      fs.writeFileSync(path.join(root, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'), 'throw Error("unexpected npm execution");');
      const lockBytes = fs.readFileSync(new URL(`../scripts/user-locks/${profile}/package-lock.json`, import.meta.url));
      fs.writeFileSync(path.join(root, `user-locks/${profile}/package-lock.json`), lockBytes);
      fs.writeFileSync(path.join(root, source, 'package-lock.json'), lockBytes);
      fs.writeFileSync(path.join(root, 'prefix/lib/node_modules/@earendil-works/pi-coding-agent/package.json'),
        JSON.stringify({ version: profile === 'prior' ? '0.99.2' : '1.0.0' }));
      if (profile === 'prior') fs.writeFileSync(path.join(root, 'state/prior-global-graph.json'), '[]');
      const emptyInventorySHA = crypto.createHash('sha256').update(JSON.stringify([['', 'dir', 0o700]])).digest('hex');
      fs.writeFileSync(path.join(root, source, '.gentle-shell-optional.json'),
        JSON.stringify({ paths: [retainedPath], sha256: emptyInventorySHA }));
      const helper = new URL('../scripts/provision-gentle-shell-private-global.mjs', import.meta.url);
      const prefix = path.join(root, 'prefix');
      const result = spawnSync(process.execPath, [helper.pathname, root, prefix, path.join(root, 'agent'), prefix, root, 'separate', 'verify'], {
        env: { HOME: path.join(root, 'home'), TMPDIR: path.join(root, 'tmp'), PATH: '/usr/bin:/bin' },
        timeout: 10000, encoding: 'utf8', maxBuffer: 8192,
      });
      assert.notEqual(result.status, 0);
      assert.match(result.stderr, /retained acquisition is not locked nonapplicable optional/);
      assert.doesNotMatch(result.stderr, /ERR_MODULE_NOT_FOUND|unexpected npm execution/);
      assert.equal(fs.existsSync(path.join(root, 'user-global-graph.mjs')), false);
    });
  }
}
