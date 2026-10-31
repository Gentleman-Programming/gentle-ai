import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import test from 'node:test';
import vm from 'node:vm';
import { fileURLToPath, pathToFileURL } from 'node:url';

// Host contracts of the owned provisioner's Main SDK runtime actions. The full
// action pipeline needs an authenticated stock acquisition; these checks pin
// the platform facts its exact readbacks depend on.
const provision = process.argv[2] ?? fileURLToPath(new URL('./provision-gentle-shell-private-global.mjs', import.meta.url));
const source = fs.readFileSync(provision, 'utf8');

function declared(name) {
  const match = source.match(new RegExp(`const ${name} = ([^;]+);`));
  assert.ok(match, `provisioner no longer declares ${name}`);
  return match[1];
}

test('Main runtime placement and selection share one platform qualification', () => {
  const expression = declared('mainPlatform');
  const qualified = (platform, arch) => Function('process', `return ${expression};`)({ platform, arch });
  assert.equal(qualified('linux', 'x64'), true);
  assert.equal(qualified('darwin', 'arm64'), true);
  for (const [platform, arch] of [['linux', 'arm64'], ['darwin', 'x64'], ['win32', 'x64']]) {
    assert.equal(qualified(platform, arch), false, `${platform}/${arch} admitted`);
  }
  assert.match(source, /if \(!mainPlatform \|\| \(mode !== 'separate' && mode !== 'agent'\)/);
  assert.match(source, /if \(runtime && \(!mainPlatform \|\| prefix !== path\.join\(root, 'prefix'\)\)\)/);
  assert.doesNotMatch(source, /Main SDK (runtime|selection) requires Linux amd64 (private|Separate)/);
});

// The dependency binding readback is exact: its expected link mode must be the
// mode this host records for a symlink created under the provisioner's pinned
// 022 umask (Linux always 0777, macOS the umask complement 0755).
test('Main dependency link mode matches this host under the pinned umask', t => {
  assert.match(source, /process\.umask\(0o022\);/);
  const expression = declared('linkMode');
  const expected = Function('process', `return ${expression};`)(process);
  const root = fs.mkdtempSync(path.join(fs.realpathSync(os.tmpdir()), 'provision-link-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const previous = process.umask(0o022);
  try {
    fs.symlinkSync('../prefix/lib/node_modules', path.join(root, 'node_modules'));
  } finally {
    process.umask(previous);
  }
  assert.equal(fs.lstatSync(path.join(root, 'node_modules')).mode & 0o7777, expected);
  assert.match(source, /\(link\.mode & 0o7777\) !== linkMode/);
});

// Engram (S1): the companion gate and the core publication run as the
// provisioner's own helpers in a VM, with a stand-in gentle-pi and injected
// adapters; nothing here touches the network.
function extract(name) {
  const begin = new RegExp(`^(?:async )?function ${name}\\(`, 'm');
  const match = begin.exec(source);
  assert.ok(match, `${name} helper present`);
  const end = source.indexOf('\n}', match.index);
  return source.slice(match.index, end + 2);
}
function sandbox() {
  const context = vm.createContext({ fs, path, crypto, Buffer, console, pathToFileURL, uid: process.getuid(), rootNames: JSON.parse(declared('rootNames').replaceAll("'", '"')),
    companionName: JSON.parse(declared('companionName').replaceAll("'", '"')), reviewedSeed: Function(`return ${declared('reviewedSeed')}`)(), reject: message => { throw Error(`global provision refused: ${message}`); } });
  for (const name of ['physical', 'read', 'absent', 'sync', 'writeExclusive', 'lockRoots', 'rootDeclaration', 'companionPin', 'companionDeclaration', 'publishEngramCore', 'engramMemory']) {
    vm.runInContext(extract(name), context, { filename: `${name}.vm.js`, importModuleDynamically: vm.constants.USE_MAIN_CONTEXT_DEFAULT_LOADER });
  }
  return (code, ...args) => vm.runInContext(code, context)(...args);
}
const pinLine = 'export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@0.4.2";';
function scratch(t) {
  const root = fs.mkdtempSync(path.join(fs.realpathSync(os.tmpdir()), 'provision-engram-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  return root;
}
// A stand-in for the provisioned gentle-pi exporting the same names and
// signatures as gentle-pi 4.1.0-4.5.1; the download goes through the adapters.
function gentlePi(root, exports = true) {
  const gentle = path.join(root, 'gentle-pi');
  fs.mkdirSync(path.join(gentle, 'scripts'), { recursive: true });
  if (exports) {
    fs.writeFileSync(path.join(gentle, 'scripts/installer-downloads.mjs'), `
export function artifactFor(name, platform, arch) { return { name, platform, arch, version: '3.5.2', url: 'https://example.invalid/engram_' + platform + '_' + arch + '.tar.gz' }; }
export async function verifiedDownload(name, adapters = {}, platform, arch) {
  const descriptor = (adapters.artifact ?? artifactFor)(name, platform, arch);
  const bytes = await adapters.download(descriptor);
  if (!bytes.toString().startsWith('pinned:')) throw new Error('Download integrity mismatch');
  return bytes;
}`);
    fs.writeFileSync(path.join(gentle, 'scripts/bundled-install.mjs'), `
export function engramExecutable(bytes, descriptor) { return Buffer.from('core<' + bytes + '>' + descriptor.url); }`);
  }
  return gentle;
}

test('the companion pin is read only from the exact bundled Shell declaration', () => {
  const run = sandbox();
  assert.equal(run('text => companionPin(text)', `// header\n${pinLine}\nexport const OTHER = 1;`), '0.4.2');
  for (const text of ['export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@latest";', 'export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@^0.4.2";',
    'export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram";', 'export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@0.4.2-beta.1";', '', undefined]) {
    assert.equal(run('text => companionPin(text)', text), null, `${text} is not an exact pin`);
  }
});

test('the companion is an optional sixth root of the closed root set', () => {
  const run = sandbox();
  const roots = JSON.parse(declared('rootNames').replaceAll("'", '"'));
  const deps = names => Object.fromEntries(names.map(name => [name, '1.0.0']));
  assert.deepEqual([...run('d => lockRoots(d)', deps(roots))], [...roots].sort());
  assert.deepEqual([...run('d => lockRoots(d)', deps([...roots, 'gentle-engram']))], [...roots, 'gentle-engram'].sort());
  assert.equal(run('d => lockRoots(d)', deps([...roots, 'foreign'])), null);
  assert.equal(run('d => lockRoots(d)', deps([...roots.slice(1), 'gentle-engram'])), null);
  const declaration = run('(target, companion) => rootDeclaration(target, companion)', '4.5.1', '0.4.2');
  assert.equal(declaration.dependencies['gentle-engram'], '0.4.2', 'the companion is declared at the exact pin, never latest');
  assert.equal(declaration.dependencies['gentle-pi'], '4.5.1');
  assert.equal(Object.hasOwn(run('target => rootDeclaration(target)', '4.5.1').dependencies, 'gentle-engram'), false, 'no pin, no companion root');
});

test('the companion is declared only when pin, lock and installed version agree', t => {
  const run = sandbox();
  const errors = [];
  t.mock.method(console, 'error', message => errors.push(message));
  const gate = (...args) => run('(pin, locked, installed) => companionDeclaration(pin, locked, installed)', ...args);
  assert.equal(gate('0.4.2', '0.4.2', '0.4.2'), 'npm:gentle-engram@0.4.2', 'declared exactly as bundled Shell declares it');
  assert.deepEqual(errors, []);
  // A lock/pin mismatch never declares and never refuses: the install goes on.
  assert.equal(gate('0.5.0', '0.4.2', '0.4.2'), null);
  assert.equal(gate('0.4.2', '0.4.2', '0.4.3'), null);
  assert.equal(gate('0.4.2', undefined, undefined), null);
  assert.equal(errors.length, 3);
  assert.match(errors[0], /^Engram companion not declared: gentle-pi pins 0\.5\.0, lock 0\.4\.2, installed 0\.4\.2; Shell stays usable without memory$/);
  // Prior gentle-pi 4.0.0 pins none and its lock has no companion: silent, no companion.
  assert.equal(gate(null, undefined, undefined), null);
  assert.equal(errors.length, 3);
});

// The frozen locks keep their base five roots: the companion joins only the
// generated Stable Separate route (B1), so their SDK digest never moves.
test('neither frozen lock carries the companion', () => {
  for (const profile of ['prior', 'modern']) {
    const lock = JSON.parse(fs.readFileSync(fileURLToPath(new URL(`./user-locks/${profile}/package-lock.json`, import.meta.url))));
    assert.equal(Object.hasOwn(lock.packages[''].dependencies, 'gentle-engram'), false, profile);
    assert.equal(lock.packages['node_modules/gentle-engram'], undefined, profile);
  }
});

// Memory is the companion and the core together: a frozen Separate install
// (Main preview, base lock, gentle-pi 4.3.0 pinning 0.4.2) declares neither,
// publishes no core and succeeds with one note.
test('a frozen Separate install without a locked companion publishes no core', async t => {
  const root = scratch(t), run = sandbox();
  const errors = [], requested = [];
  t.mock.method(console, 'error', message => errors.push(message));
  const modern = JSON.parse(fs.readFileSync(fileURLToPath(new URL('./user-locks/modern/package-lock.json', import.meta.url))));
  const locked = modern.packages['node_modules/gentle-engram']?.version;
  const bin = path.join(root, 'agent/bin');
  fs.mkdirSync(path.dirname(bin), { mode: 0o700 });
  const adapters = { download: async descriptor => { requested.push(descriptor); return Buffer.from('pinned:archive'); } };
  assert.equal(await run('(...args) => engramMemory(...args)', '0.4.2', locked, undefined, gentlePi(root), bin, 'engram', 'linux', 'x64', adapters), null);
  assert.equal(fs.existsSync(bin), false, 'no core without a declared companion');
  assert.deepEqual(requested, [], 'nothing is downloaded for no client');
  assert.deepEqual(errors, ['Engram companion not declared: gentle-pi pins 0.4.2, lock none, installed none; Shell stays usable without memory']);
});

test('a generated Separate install with the agreed companion declares it and publishes the core', async t => {
  const root = scratch(t), run = sandbox();
  const bin = path.join(root, 'agent/bin');
  fs.mkdirSync(path.dirname(bin), { mode: 0o700 });
  const adapters = { download: async () => Buffer.from('pinned:archive') };
  assert.equal(await run('(...args) => engramMemory(...args)', '0.4.2', '0.4.2', '0.4.2', gentlePi(root), bin, 'engram', 'linux', 'x64', adapters), 'npm:gentle-engram@0.4.2');
  assert.equal(fs.lstatSync(path.join(bin, 'engram')).mode & 0o7777, 0o700);
  assert.match(source, /const declaration = await engramMemory\(companion, pins\[companionName\]\?\.\[0\], installed, supplier, path\.join\(agent, 'bin'\), 'engram', process\.platform, process\.arch\);/);
});

test('the verified Engram core is published at <agent>/bin/engram with mode 0700', async t => {
  const root = scratch(t), run = sandbox();
  const gentle = gentlePi(root), bin = path.join(root, 'agent/bin');
  fs.mkdirSync(path.dirname(bin), { mode: 0o700 });
  const requested = [];
  const adapters = { download: async descriptor => { requested.push(descriptor); return Buffer.from('pinned:archive'); } };
  assert.equal(await run('(...args) => publishEngramCore(...args)', gentle, bin, 'engram', 'linux', 'x64', adapters), true);
  const core = path.join(bin, 'engram');
  assert.equal(fs.readFileSync(core, 'utf8'), 'core<pinned:archive>https://example.invalid/engram_linux_x64.tar.gz');
  assert.equal(fs.lstatSync(core).mode & 0o7777, 0o700);
  assert.equal(fs.lstatSync(bin).mode & 0o7777, 0o700);
  assert.deepEqual(fs.readdirSync(bin), ['engram'], 'atomic publication leaves no stage file');
  assert.deepEqual(requested.map(({ name, platform, arch }) => [name, platform, arch]), [['engram', 'linux', 'x64']]);
});

test('a gentle-pi without the Engram exports skips the core without failing', async t => {
  const root = scratch(t), run = sandbox();
  const errors = [];
  t.mock.method(console, 'error', message => errors.push(message));
  const bin = path.join(root, 'agent/bin');
  fs.mkdirSync(path.dirname(bin), { mode: 0o700 });
  assert.equal(await run('(...args) => publishEngramCore(...args)', gentlePi(root, false), bin, 'engram', 'linux', 'x64', {}), false);
  assert.equal(fs.existsSync(bin), false);
  assert.deepEqual(errors, ['Engram core skipped: the installed gentle-pi pins no Engram core; Shell stays usable without memory']);
});

test('an unverified Engram download publishes nothing and keeps the install', async t => {
  const root = scratch(t), run = sandbox();
  const errors = [];
  t.mock.method(console, 'error', message => errors.push(message));
  const bin = path.join(root, 'agent/bin');
  fs.mkdirSync(bin, { recursive: true, mode: 0o700 });
  const adapters = { download: async () => Buffer.from('tampered') };
  assert.equal(await run('(...args) => publishEngramCore(...args)', gentlePi(root), bin, 'engram', 'linux', 'x64', adapters), false);
  assert.deepEqual(fs.readdirSync(bin), []);
  assert.deepEqual(errors, ['Engram core skipped: Download integrity mismatch; Shell stays usable without memory']);
  // An occupied contract path is never replaced.
  fs.writeFileSync(path.join(bin, 'engram'), 'foreign', { mode: 0o700 });
  assert.equal(await run('(...args) => publishEngramCore(...args)', gentlePi(root), bin, 'engram', 'linux', 'x64', { download: async () => Buffer.from('pinned:archive') }), false);
  assert.equal(fs.readFileSync(path.join(bin, 'engram'), 'utf8'), 'foreign');
  assert.deepEqual(fs.readdirSync(bin), ['engram']);
});
