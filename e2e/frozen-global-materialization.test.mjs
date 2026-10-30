import assert from 'node:assert/strict';
import fs from 'node:fs';
import crypto from 'node:crypto';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const scripts = fileURLToPath(new URL('../scripts/', import.meta.url));
const copy = { recursive: true, force: false, errorOnExist: true, verbatimSymlinks: true };
const npmExecutable = process.env.PATH.split(path.delimiter).map(dir => path.join(dir, 'npm')).find(file => fs.existsSync(file));
const rebuildProof = { skip: npmExecutable ? false : 'stock npm unavailable for offline rebuild proof' };
const reviewed = JSON.parse(fs.readFileSync(path.join(scripts, 'gentle-pi-suppliers.json'), 'utf8')).releases;
function newestReviewedListVersion(table) {
  const order = version => version.split('.').map(Number);
  return Object.keys(table).sort((left, right) => {
    const a = order(left), b = order(right);
    return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
  }).at(-1);
}
const targetVersion = newestReviewedListVersion(reviewed);
const rootNames = ['gentle-pi', '@earendil-works/pi-coding-agent', '@earendil-works/pi-tui', '@heyhuynhgiabuu/pi-pretty', 'typebox'];
// The stand-in listed gentle-pi pins its companion like gentle-pi 4.1.0+.
const companionPin = '0.4.2';
function fixture(t, mode = 'separate', scenario = '') {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'frozen-global-'));
  fs.chmodSync(root, 0o700);
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  // A stand-in for the listed gentle-pi tarball stock npm packs before the
  // lock is generated; the staged list binds its SRI like the real entry.
  fs.mkdirSync(path.join(root, 'packed/package/runtime'), { recursive: true, mode: 0o700 });
  fs.writeFileSync(path.join(root, 'packed/package/runtime/gentle-shell-bundled.mjs'), `export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@${companionPin}";\n`);
  const packed = path.join(root, 'packed/gentle-pi.tgz');
  assert.equal(spawnSync('/usr/bin/tar', ['-czf', packed, '-C', path.join(root, 'packed'), 'package']).status, 0);
  const listedIntegrity = `sha512-${crypto.createHash('sha512').update(fs.readFileSync(packed)).digest('base64')}`;
  for (const name of ['state', 'agent', 'home', 'tmp', 'config', 'runtime/node/bin', 'runtime/node/lib/node_modules', 'user-locks/modern', 'template/node_modules']) {
    fs.mkdirSync(path.join(root, name), { recursive: true, mode: 0o700 });
  }
  fs.copyFileSync(process.execPath, path.join(root, 'runtime/node/bin/node'));
  fs.chmodSync(path.join(root, 'runtime/node/bin/node'), 0o700);
  const stock = fs.realpathSync(npmExecutable);
  assert.ok(stock.endsWith('/bin/npm-cli.js'), 'identify the stock npm package');
  const npmRoot = path.join(root, 'runtime/node/lib/node_modules/npm');
  fs.cpSync(path.dirname(path.dirname(stock)), npmRoot, copy);
  fs.renameSync(path.join(npmRoot, 'bin/npm-cli.js'), path.join(npmRoot, 'bin/stock-cli.cjs'));
  fs.copyFileSync(path.join(scripts, 'normalize-private-optional-platform-closure.mjs'), path.join(root, 'normalize-private-optional-platform-closure.mjs'));
  // The Go worker stages the selected reviewed list and the lock-SRI
  // completion helper into every install stage; the fixture mirrors that.
  const list = JSON.parse(fs.readFileSync(path.join(scripts, 'gentle-pi-suppliers.json')));
  list.releases[targetVersion].integrity = listedIntegrity;
  fs.writeFileSync(path.join(root, 'gentle-pi-suppliers.json'), JSON.stringify(list), { mode: 0o600 });
  fs.copyFileSync(path.join(scripts, 'complete-generated-lock-sri.mjs'), path.join(root, 'complete-generated-lock-sri.mjs'));
  for (const name of ['user.npmrc', 'global.npmrc']) fs.writeFileSync(path.join(root, 'config', name), '');
  const originalSettings = '{"theme":"kept"}\n';
  fs.writeFileSync(path.join(root, 'agent/settings.json'), originalSettings);
  const production = JSON.parse(fs.readFileSync(path.join(scripts, 'user-locks/modern/package-lock.json')));
  const dependencies = production.packages[''].dependencies;
  const lock = { name: 'fixture-source', version: '1.0.0', lockfileVersion: 3, packages: {
    '': { name: 'fixture-source', version: '1.0.0', dependencies },
  } };
  const child = 'node_modules/@earendil-works/pi-coding-agent/node_modules/fixture-child';
  for (const [relative, name, version] of [
    ...Object.entries(dependencies).map(([name, version]) => [`node_modules/${name}`, name, version]),
    ['node_modules/gentle-engram', 'gentle-engram', companionPin],
    [child, 'fixture-child', '1.0.0'],
  ]) {
    // The generated lock pins gentle-pi at the newest listed release.
    const effectiveVersion = name === 'gentle-pi' ? targetVersion : version;
    lock.packages[relative] = { version: effectiveVersion, resolved: `https://registry.npmjs.org/${name}/-/${name.split('/').at(-1)}-${effectiveVersion}.tgz`, integrity: name === 'gentle-pi' ? listedIntegrity : `sha512-${Buffer.alloc(64).toString('base64')}` };
    const directory = path.join(root, 'template', relative);
    fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
    const command = name === 'gentle-pi' ? 'gentle-shell' : name === '@earendil-works/pi-coding-agent' ? 'pi' : name === 'fixture-child' ? 'fixture-child' : name === 'gentle-engram' ? 'pi-engram' : undefined;
    const metadata = { name, version: effectiveVersion, ...(command ? { bin: { [command]: 'cli.js' } } : {}),
      scripts: { install: 'node -e "require(\'node:fs\').writeFileSync(process.env.HOME+\'/hook-ran\',\'unexpected\')"' },
      ...(name === '@earendil-works/pi-coding-agent' ? { dependencies: { 'fixture-child': '1.0.0' } } : {}),
    };
    fs.writeFileSync(path.join(directory, 'package.json'), JSON.stringify(metadata));
    fs.writeFileSync(path.join(directory, 'cli.js'), '#!/usr/bin/env node\nconsole.log("fixture works");\n', { mode: 0o755 });
    if (scenario === 'supplier-mismatch' && name === 'gentle-pi') {
      // Supplier files whose bytes do not match the reviewed list entry.
      fs.mkdirSync(path.join(directory, 'scripts'), { recursive: true, mode: 0o700 });
      fs.mkdirSync(path.join(directory, 'runtime'), { recursive: true, mode: 0o700 });
      fs.writeFileSync(path.join(directory, 'scripts/gentle-ai-installer.mjs'), '// tampered supplier\n');
      fs.writeFileSync(path.join(directory, 'runtime/gentle-ai-binary.mjs'), '// tampered supplier\n');
    }
  }
  const nestedBins = path.join(root, 'template/node_modules/@earendil-works/pi-coding-agent/node_modules/.bin');
  fs.mkdirSync(nestedBins, { mode: 0o700 });
  fs.symlinkSync('../fixture-child/cli.js', path.join(nestedBins, 'fixture-child'));
  const staleHidden = '{"lockfileVersion":3,"packages":{"node_modules/nonapplicable-optional":{"version":"9.0.0"}}}';
  fs.writeFileSync(path.join(root, 'template/node_modules/.package-lock.json'), staleHidden);
  fs.writeFileSync(path.join(root, 'user-locks/modern/package-lock.json'), JSON.stringify(lock));
  const prefix = path.join(root, 'prefix');
  if (mode === 'shared') {
    fs.mkdirSync(path.join(prefix, 'lib'), { recursive: true, mode: 0o700 });
    fs.cpSync(path.join(root, 'template/node_modules'), path.join(prefix, 'lib/node_modules'), copy);
    // The selected Shared prefix predates the companion: stock roots only.
    fs.rmSync(path.join(prefix, 'lib/node_modules/gentle-engram'), { recursive: true });
    fs.mkdirSync(path.join(prefix, 'lib/node_modules/gentle-pi/.gentle-ai'), { mode: 0o700 });
  }
  // CI uses a tiny controlled graph. Rebuild delegates to REAL stock npm offline;
  // exit 73 then stops before unrelated fixed production supplier/hash readback.
  const trace = path.join(root, 'trace.jsonl');
  fs.writeFileSync(path.join(npmRoot, 'bin/npm-cli.js'), `
const fs = require('node:fs'), path = require('node:path'), cp = require('node:child_process');
const root = ${JSON.stringify(root)}, args = process.argv.slice(2);
fs.appendFileSync(${JSON.stringify(trace)}, JSON.stringify(args)+'\\n');
if (args[0] === 'pack') {
  const destination = args.find(arg => arg.startsWith('--pack-destination=')).slice('--pack-destination='.length);
  fs.copyFileSync(${JSON.stringify(packed)}, path.join(destination, 'gentle-pi-' + args[1].split('@')[1] + '.tgz'));
  process.exit(0);
}
const declared = () => JSON.parse(fs.readFileSync(path.join(process.cwd(), 'package.json'))).dependencies;
if (args[0] === 'ci') {
  fs.cpSync(path.join(root,'template/node_modules'), path.join(process.cwd(),'node_modules'), ${JSON.stringify(copy)});
  if (!declared()['gentle-engram']) fs.rmSync(path.join(process.cwd(),'node_modules/gentle-engram'), { recursive: true });
  if (${JSON.stringify(scenario)} === 'escaping-source') fs.symlinkSync(path.join(root,'agent/settings.json'), path.join(process.cwd(),'node_modules/gentle-pi/escape'));
  process.exit(0);
}
if (args[0] === 'install') {
  const lock = JSON.parse(fs.readFileSync(${JSON.stringify(path.join(root, 'user-locks/modern/package-lock.json'))}));
  lock.packages[''].dependencies = declared();
  if (!lock.packages[''].dependencies['gentle-engram']) delete lock.packages['node_modules/gentle-engram'];
  const gentle = lock.packages['node_modules/gentle-pi'];
  gentle.version = ${JSON.stringify(targetVersion)};
  gentle.resolved = 'https://registry.npmjs.org/gentle-pi/-/gentle-pi-' + gentle.version + '.tgz';
  gentle.integrity = ${JSON.stringify(listedIntegrity)};
  fs.writeFileSync(${JSON.stringify(path.join(root, 'runtime/project/package-lock.json'))}, JSON.stringify(lock));
  process.exit(0);
}
if (${JSON.stringify(scenario)} === 'rebuild-failure') process.exit(74);
const child = cp.spawnSync(process.execPath, [path.join(__dirname,'stock-cli.cjs'), ...args], {env:process.env, encoding:'utf8'});
fs.writeFileSync(path.join(root,'stock-status.json'), JSON.stringify({status:child.status, signal:child.signal, error:child.error?.message, output:child.stdout+child.stderr}));
if (${JSON.stringify(scenario)} === 'supplier-mismatch') process.exit(child.status ?? 1);
process.exit(73);
`);
  const result = spawnSync(process.execPath, [path.join(scripts, 'provision-gentle-shell-private-global.mjs'),
    root, prefix, path.join(root, 'agent'), prefix, root, mode, 'install'], {
    env: { HOME: path.join(root, 'home'), PATH: '/usr/bin:/bin' }, timeout: 30000, encoding: 'utf8', maxBuffer: 16384,
  });
  const calls = fs.existsSync(trace) ? fs.readFileSync(trace, 'utf8').trim().split('\n').map(JSON.parse) : [];
  return { root, prefix, result, calls, originalSettings, staleHidden, modules: path.join(prefix, 'lib/node_modules') };
}

for (const mode of ['separate', 'shared']) {
  test(`${mode} materializes the frozen tree and stock offline bins without hooks`, rebuildProof, t => {
    const f = fixture(t, mode);
    const calls = f.calls.filter(args => args[0] !== 'pack');
    assert.deepEqual(calls.map(args => args[0]), ['install', 'ci', 'rebuild'], f.result.stderr);
    assert.ok(calls[0].includes('--package-lock-only') && calls[0].includes('--min-release-age=0'));
    assert.ok(calls[2].includes('--global') && calls[2].includes('--offline') && calls[2].includes('--ignore-scripts'));
    // Separate acquires the companion as an authenticated root; Shared leaves
    // the selected prefix without a new package or command (B1).
    if (mode === 'separate') {
      assert.equal(JSON.parse(fs.readFileSync(path.join(f.modules, 'gentle-engram/package.json'))).version, companionPin);
      assert.equal(fs.realpathSync(path.join(f.prefix, 'bin/pi-engram')), path.join(f.modules, 'gentle-engram/cli.js'));
    } else {
      assert.deepEqual(f.calls.map(args => args[0]), ['install', 'ci', 'rebuild'], 'Shared never packs gentle-pi for a companion pin');
      assert.equal(fs.existsSync(path.join(f.modules, 'gentle-engram')), false);
      assert.equal(fs.existsSync(path.join(f.prefix, 'bin/pi-engram')), false);
      assert.equal(Object.hasOwn(JSON.parse(fs.readFileSync(path.join(f.root, 'runtime/project/package.json'))).dependencies, 'gentle-engram'), false);
    }
    const stock = JSON.parse(fs.readFileSync(path.join(f.root, 'stock-status.json')));
    assert.equal(stock.status, 0, stock.output);
    assert.match(f.result.stderr, /stock npm rebuild failed; status=73/);
    const source = path.join(f.root, 'runtime/project/node_modules');
    for (const [command, relative] of [['gentle-shell', 'gentle-pi'], ['pi', '@earendil-works/pi-coding-agent']]) {
      assert.equal(fs.realpathSync(path.join(f.prefix, 'bin', command)), path.join(f.modules, relative, 'cli.js'));
      assert.deepEqual(fs.readFileSync(path.join(f.modules, relative, 'cli.js')), fs.readFileSync(path.join(source, relative, 'cli.js')));
    }
    const child = '@earendil-works/pi-coding-agent/node_modules/fixture-child/cli.js';
    assert.ok(fs.existsSync(path.join(f.modules, child)));
    assert.equal(fs.realpathSync(path.join(f.modules, '@earendil-works/pi-coding-agent/node_modules/.bin/fixture-child')), path.join(f.modules, child));
    assert.equal(fs.readFileSync(path.join(source, '.package-lock.json'), 'utf8'), f.staleHidden);
    assert.equal(fs.existsSync(path.join(f.modules, '.package-lock.json')), false);
    assert.equal(fs.existsSync(path.join(f.root, 'home/hook-ran')), false);
    assert.equal(fs.readFileSync(path.join(f.root, 'agent/settings.json'), 'utf8'), f.originalSettings);
    const retained = fs.readdirSync(path.join(f.prefix, 'lib')).filter(name => name.startsWith('.gentle-shell-previous-'));
    assert.equal(retained.length, mode === 'shared' ? 1 : 0);
    if (mode === 'shared') {
      for (const directory of [f.modules, path.join(f.prefix, 'lib', retained[0]), path.join(f.root, 'state/prefix.preimage/lib/node_modules')]) {
        assert.ok(fs.statSync(path.join(directory, 'gentle-pi/.gentle-ai')).isDirectory());
        assert.equal(fs.statSync(path.join(directory, 'gentle-pi/.gentle-ai')).mode & 0o777, 0o700);
      }
    }
  });
}

test('escaping source refuses before deployment or rebuild', rebuildProof, t => {
  const f = fixture(t, 'separate', 'escaping-source');
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install', 'ci'], f.result.stderr);
  assert.match(f.result.stderr, /inventory escaping link/);
  assert.equal(fs.existsSync(f.modules), false);
  assert.equal(fs.readFileSync(path.join(f.root, 'agent/settings.json'), 'utf8'), f.originalSettings);
});

test('rebuild failure retains old, new and confirmed recovery evidence', rebuildProof, t => {
  const f = fixture(t, 'shared', 'rebuild-failure');
  assert.deepEqual(f.calls.map(args => args[0]), ['install', 'ci', 'rebuild'], f.result.stderr);
  assert.match(f.result.stderr, /stock npm rebuild failed; status=74/);
  const retained = fs.readdirSync(path.join(f.prefix, 'lib')).filter(name => name.startsWith('.gentle-shell-previous-'));
  assert.equal(retained.length, 1);
  for (const directory of [f.modules, path.join(f.prefix, 'lib', retained[0]), path.join(f.root, 'state/prefix.preimage/lib/node_modules')]) {
    assert.ok(fs.existsSync(path.join(directory, 'gentle-pi/package.json')));
  }
  assert.equal(fs.readFileSync(path.join(f.root, 'agent/settings.json'), 'utf8'), f.originalSettings);
  const selection = fs.readFileSync(path.join(f.root, 'state/selection.json'));
  const authority = { selectionSHA: crypto.createHash('sha256').update(selection).digest('hex') };
  for (const [directory, key] of [[f.root, 'rootID'], [f.prefix, 'prefixID'], [path.join(f.root, 'agent'), 'agentID']]) {
    const stat = fs.lstatSync(directory, { bigint: true });
    authority[key] = `${stat.dev}:${stat.ino}`;
  }
  const restored = spawnSync(process.execPath, [path.join(scripts, 'provision-gentle-shell-private-global.mjs'),
    f.root, f.prefix, path.join(f.root, 'agent'), f.prefix, f.root, 'shared', 'restore', JSON.stringify(authority)], {
    env: { HOME: path.join(f.root, 'home'), PATH: '/usr/bin:/bin' }, timeout: 10000, encoding: 'utf8',
  });
  assert.equal(restored.status, 0, restored.stderr);
  assert.match(restored.stdout, /Shared preimages restored/);
  assert.deepEqual(fs.readdirSync(path.join(f.prefix, 'lib')), ['node_modules']);
  assert.equal(fs.readFileSync(path.join(f.modules, '.package-lock.json'), 'utf8'), f.staleHidden);
  assert.equal(fs.statSync(path.join(f.modules, 'gentle-pi/.gentle-ai')).mode & 0o777, 0o700);
  assert.ok(fs.readdirSync(path.join(f.root, 'state')).some(name => name.startsWith('prefix.preimage.quarantine-')));
});

test('supplier bytes differing from the reviewed list entry refuse before the supplier runs', rebuildProof, t => {
  const f = fixture(t, 'separate', 'supplier-mismatch');
  assert.deepEqual(f.calls.map(args => args[0]), ['pack', 'install', 'ci', 'rebuild'], f.result.stderr);
  assert.match(f.result.stderr, /stock supplier pin/);
  assert.equal(fs.readFileSync(path.join(f.root, 'agent/settings.json'), 'utf8'), f.originalSettings);
  assert.equal(fs.existsSync(path.join(f.root, 'home/hook-ran')), false);
});
