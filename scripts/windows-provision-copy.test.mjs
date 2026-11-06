// Portable VM fixtures for the Windows provisioner's single-materialization copy
// region: they prove the verified staging/promotion helper logic, not an installed
// Windows runtime. Adapted from the windows-mutable-settings VM region pattern.
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import vm from 'node:vm';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const provision = process.argv[2] ?? fileURLToPath(new URL('./provision-gentle-shell-windows.mjs', import.meta.url));
const source = fs.readFileSync(provision, 'utf8');
const refusal = message => { throw Error(`Windows provision refused: ${message}`); };

// Static contract: the second registry materialization is gone, the phase markers
// the Go progress mapping expects exist, and the promotion never deletes evidence.
assert.ok(!source.includes(`'install', '--global'`), 'second npm global install must be replaced by a verified copy');
for (const phase of ['provision-lock', 'provision-ci', 'provision-copy', 'provision-rebuild', 'provision-compare', 'provision-native']) {
  assert.ok(source.includes(`Windows install phase: ${phase}`), `phase marker ${phase} present`);
}
assert.ok(source.includes(`'rebuild', '--global', '--prefix', prefix, '--offline', '--ignore-scripts', '--bin-links=true', '--engine-strict', '--no-audit', '--no-fund'`), 'stock rebuild flags');
assert.ok(source.includes('fs.renameSync(acquired, modules)'), 'source tree promoted by rename');
assert.ok(source.includes("relative.slice('node_modules/'.length)"), 'promoted authority strips the node_modules prefix');
assert.ok(source.includes('const promotedAuthority = materialized || !fs.existsSync(path.join(source, \'node_modules\'))'), 'verify uses the promoted tree when source is gone');
assert.ok(source.includes('`' + '.previous-${crypto.randomUUID()}' + '`'), 'old modules tree preserved as .previous-<uuid> sibling');
const materializedDecl = source.indexOf('let materialized = false;');
assert.ok(materializedDecl >= 0, 'materialized state declared');
assert.ok(materializedDecl < source.indexOf('function materializeGlobal('), 'materialized declared before materializeGlobal');
assert.ok(materializedDecl < source.indexOf('materializeGlobal();'), 'materialized declared before the install call');

function extract(name) {
  const begin = `function ${name}(`;
  const start = source.indexOf(begin);
  assert.ok(start >= 0, `${name} helper present`);
  assert.equal(source.indexOf(begin, start + 1), -1, `${name} helper unique`);
  const end = source.indexOf('\n}', start);
  assert.ok(end > start, `${name} helper terminated`);
  return source.slice(start, end + 2);
}

const sandbox = { fs, path, crypto, console, setTimeout };
sandbox.reject = message => refusal(message);
sandbox.digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
vm.createContext(sandbox);
for (const name of ['read', 'absent', 'treeDigest', 'materializeGlobal']) {
  vm.runInContext(extract(name), sandbox, { filename: `${name}.vm.js` });
}
const run = (code, argument) => vm.runInContext(code, sandbox, { filename: 'call.vm.js' })(argument);

const failures = [];
const scenario = (name, body) => {
  try { body(); } catch (error) { failures.push(`${name}: ${error.message}`); }
};
function fixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'provision-copy-'));
  const acquired = path.join(root, 'source/node_modules');
  for (const [relative, bytes] of [
    ['gentle-pi/package.json', '{"name":"gentle-pi","version":"4.0.0"}'],
    ['gentle-pi/bin/gentle-shell.mjs', '#!/usr/bin/env node\n'],
    ['@scope/lib/package.json', '{"name":"@scope/lib","version":"1.0.0"}'],
    ['@scope/lib/index.js', 'export {}'],
    ['.package-lock.json', '{"lockfileVersion":3}'],
  ]) {
    fs.mkdirSync(path.join(acquired, path.dirname(relative)), { recursive: true });
    fs.writeFileSync(path.join(acquired, relative), bytes);
  }
  return { root, acquired };
}

scenario('fresh-materialization', () => {
  const { root, acquired } = fixture();
  sandbox.source = path.join(root, 'source');
  sandbox.prefix = path.join(root, 'prefix');
  sandbox.modules = path.join(sandbox.prefix, 'node_modules');
  run('materializeGlobal');
  assert.deepEqual(fs.readdirSync(sandbox.prefix).sort(), ['node_modules'], 'promotion leaves no staged sibling');
  for (const [relative, bytes] of [
    ['gentle-pi/package.json', '{"name":"gentle-pi","version":"4.0.0"}'],
    ['@scope/lib/index.js', 'export {}'],
  ]) {
    assert.equal(fs.readFileSync(path.join(sandbox.modules, relative), 'utf8'), bytes, `promoted ${relative}`);
  }
  assert.ok(!fs.existsSync(path.join(sandbox.modules, '.package-lock.json')), 'hidden lock not published');
  assert.ok(!fs.existsSync(acquired), 'source tree was moved, not copied');
});

scenario('previous-tree-preserved', () => {
  const { root } = fixture();
  sandbox.source = path.join(root, 'source');
  sandbox.prefix = path.join(root, 'prefix');
  sandbox.modules = path.join(sandbox.prefix, 'node_modules');
  const old = path.join(sandbox.modules, 'gentle-pi/package.json');
  fs.mkdirSync(path.dirname(old), { recursive: true });
  fs.writeFileSync(old, '{"name":"gentle-pi","version":"3.0.0"}');
  run('materializeGlobal');
  assert.equal(fs.readFileSync(path.join(sandbox.modules, 'gentle-pi/package.json'), 'utf8'), '{"name":"gentle-pi","version":"4.0.0"}', 'fresh tree promoted');
  const siblings = fs.readdirSync(sandbox.prefix).filter(name => name.startsWith('.previous-'));
  assert.equal(siblings.length, 1, 'exactly one preserved sibling');
  assert.equal(fs.readFileSync(path.join(sandbox.prefix, siblings[0], 'gentle-pi/package.json'), 'utf8'), '{"name":"gentle-pi","version":"3.0.0"}', 'old tree evidence preserved');
  assert.equal(fs.readdirSync(sandbox.prefix).filter(name => name.startsWith('.gentle-shell-staged-')).length, 0, 'no staged leftover');
});

scenario('tree-digest-detects-drift', () => {
  const { root } = fixture();
  sandbox.source = path.join(root, 'source');
  sandbox.prefix = path.join(root, 'prefix');
  sandbox.modules = path.join(sandbox.prefix, 'node_modules');
  const acquired = path.join(sandbox.source, 'node_modules');
  const digest = tree => run('treeDigest', tree);
  const copy = path.join(root, 'copy');
  fs.cpSync(acquired, copy, { recursive: true });
  assert.equal(digest(acquired), digest(copy), 'identical trees share one digest');
  fs.appendFileSync(path.join(copy, 'gentle-pi/bin/gentle-shell.mjs'), '// drift');
  assert.notEqual(digest(acquired), digest(copy), 'byte drift changes the digest');
  fs.writeFileSync(path.join(copy, 'gentle-pi/bin/gentle-shell.mjs'), '#!/usr/bin/env node\n');
  fs.writeFileSync(path.join(copy, 'extra.js'), '');
  assert.notEqual(digest(acquired), digest(copy), 'extra member changes the digest');
});

scenario('absent-source-refused', () => {
	const { root } = fixture();
	sandbox.source = path.join(root, 'source');
	sandbox.prefix = path.join(root, 'prefix');
	sandbox.modules = path.join(sandbox.prefix, 'node_modules');
	fs.rmSync(path.join(sandbox.source, 'node_modules'), { recursive: true, force: true });
	assert.throws(() => run('materializeGlobal'), /Windows provision refused: acquired source absent/);
	assert.ok(!fs.existsSync(sandbox.modules), 'failed promotion publishes no modules tree');
});

scenario('special-file-fails-closed', function () {
  if (process.platform === 'win32') return; // mkfifo is POSIX-only.
  const { root } = fixture();
  sandbox.source = path.join(root, 'source');
  sandbox.prefix = path.join(root, 'prefix');
  sandbox.modules = path.join(sandbox.prefix, 'node_modules');
  const pipe = path.join(sandbox.source, 'node_modules/gentle-pi/pipe');
  if (spawnSync('mkfifo', [pipe]).status !== 0) return; // Skip when unavailable.
  assert.throws(() => run('treeDigest', path.join(sandbox.source, 'node_modules')), /Windows provision refused: inventory special file/);
});

console.log(`windows-provision-copy: ${5 - failures.length}/5 scenarios passed`);
for (const failure of failures) console.log(`FAIL ${failure}`);
process.exitCode = failures.length === 0 ? 0 : 1;
