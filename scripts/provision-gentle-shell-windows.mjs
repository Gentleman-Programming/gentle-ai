// Windows Separate only. Stock npm, explicit stock native installer, no updater.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const [root, action] = process.argv.slice(2);
const reject = message => { throw Error(`Windows provision refused: ${message}`); };
if (process.platform !== 'win32' || process.arch !== 'x64' || process.argv.length !== 4 || !['install', 'overlay', 'verify'].includes(action) || !path.isAbsolute(root)) reject('platform/arguments');
const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const node = path.join(root, 'runtime/node/node.exe');
const npm = path.join(root, 'runtime/node/node_modules/npm/bin/npm-cli.js');
const prefix = path.join(root, 'prefix');
const modules = path.join(prefix, 'node_modules');
const source = path.join(root, 'source');
const agent = path.join(root, 'agent');
const selection = JSON.parse(read(path.join(root, 'selection.json')));
const selectionKeys = ['Channel', 'Confirmation', 'Destination', 'Mode', 'SharedAgent', 'SharedPrefix'];
if (selection === null || typeof selection !== 'object' || Array.isArray(selection) ||
    Object.keys(selection).sort().join(',') !== selectionKeys.join(',') || selectionKeys.some(key => typeof selection[key] !== 'string') ||
    selection.Mode !== 'separate' || selection.SharedPrefix !== '' || selection.SharedAgent !== '' || !['stable', 'main'].includes(selection.Channel) ||
    !path.isAbsolute(selection.Destination) || (action === 'verify' && selection.Destination !== root)) reject('publication channel/selection');
const finalRoot = selection.Destination; // Install/overlay stages differ from the final root.
const mainOverlay = selection.Channel === 'main' && action !== 'install';
if (selection.Channel === 'main') {
  const snapshot = JSON.parse(read(path.join(root, 'main.json'))), commit = snapshot?.Prefix?.slice('gentle-shell-'.length);
  if (Object.keys(snapshot).sort().join(',') !== 'Archive,Bound,Prefix,SHA,Size,URL' || !/^[0-9a-f]{40}$/.test(commit) ||
      snapshot.Prefix !== `gentle-shell-${commit}` || snapshot.URL !== `https://codeload.github.com/Gentleman-Programming/gentle-shell/zip/${commit}` ||
      !/^[0-9a-f]{64}$/.test(snapshot.SHA) || snapshot.Archive !== 'main.zip' || snapshot.Bound !== 33554432 || snapshot.Size !== 0) reject('Main descriptor authority');
}
const pins = {
  'gentle-pi': ['4.0.0', 'sha512-ZG/diWBSKPfjU4MjiHVUXxHWQvDSRS2dvpPCma4ZINuV+8UGdZAPHca6vcK27FLAAG7crlJpwdWOcwVNW4rh/Q=='],
  '@earendil-works/pi-coding-agent': ['1.0.0', 'sha512-/FtbxoSQU/mEv1QnichJjRjqteqaIaMWxmhB4G367+MwZfX7/DI5B9YAg5lqbN7nztFskBEtUSZ+FlmMBECtMw=='],
  '@earendil-works/pi-tui': ['1.0.0', 'sha512-JsT7kXnpZA2YOtQu6RyriyxEO0eJIzPyfiH09bH+OLN5+s18HYkwaUD/tBkjhnSfMu6/50CQPRYJagzSP6HdPw=='],
  '@heyhuynhgiabuu/pi-pretty': ['0.6.27', 'sha512-4Jj+n6ZBFdn979fWAA3nMcJ45Q5qtcLeq1Pe6+Oo2LDIpDhqv7heoTKkBpq9G74/pNYc0EaXR28pssQ5Wbc5bg=='],
  'typebox': ['1.3.27', 'sha512-zu+jc1pcy4UiNThxikUr36f0Rybk9PEeCg/NE6adeWr/SKsdNO4EzZHYRDlv2YCVAfj3Odq3dESSo/jNyoBXzA=='],
};
// NTFS owner/DACL/reparse checks are enforced by the Go worker before entry and
// on the complete produced tree afterward. This is not a substitute for them.
function read(file) {
  const before = fs.lstatSync(file);
  if (!before.isFile() || before.isSymbolicLink() || before.size > 33554432) reject('physical file/bound');
  const value = fs.readFileSync(file);
  const after = fs.lstatSync(file);
  if (before.ino !== after.ino || before.size !== after.size || before.mtimeMs !== after.mtimeMs || before.ctimeMs !== after.ctimeMs) reject('file preimage differs');
  return value;
}
function exclusive(file, value) {
  fs.writeFileSync(file, value, { flag: 'wx' });
  if (!read(file).equals(Buffer.from(value))) reject('exclusive readback');
}
function npmCommand(args) {
  const child = spawnSync(node, [npm, ...args], { cwd: source, env: process.env, shell: false, timeout: 180000, maxBuffer: 8388608 });
  if (child.error || child.status !== 0 || child.signal) {
    const bytes = Buffer.concat([child.stdout ?? Buffer.alloc(0), child.stderr ?? Buffer.alloc(0)]);
    const text = bytes.toString('utf8');
    const detail = bytes.length <= 8192 && Buffer.from(text, 'utf8').equals(bytes) && !text.includes('\0')
      ? JSON.stringify(text).replace(/[\u007f-\u009f]/g, char => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`)
      : 'whole npm diagnostic withheld (size/encoding guard)';
    reject(`npm ${args[0]} failed; status=${child.status}; error=${child.error?.code ?? 'none'}; bytes=${bytes.length}; sha256=${digest(bytes)}; diagnostic=${detail}`);
  }
}
const lockPath = path.join(source, 'package-lock.json');
if (action === 'install') {
  fs.mkdirSync(source);
  exclusive(path.join(source, 'package.json'), JSON.stringify({ name: 'gentle-shell-windows-private', private: true, version: '1.0.0', dependencies: Object.fromEntries(Object.entries(pins).map(([name, [version]]) => [name, version])) }));
  npmCommand(['install', '--package-lock-only', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/']);
  const { completeFile } = await import(pathToFileURL(path.join(root, 'complete-generated-lock-sri.mjs')).href);
  await completeFile(lockPath);
}
const lockBytes = read(lockPath), lock = JSON.parse(lockBytes);
if (lock.lockfileVersion !== 3 || !lock.packages || Object.keys(lock.packages).length > 4096 || Object.keys(lock.packages[''].dependencies).length !== 5) reject('source lock');
for (const [name, [version, integrity]] of Object.entries(pins)) {
  const record = lock.packages[`node_modules/${name}`];
  if (record?.version !== version || record.integrity !== integrity || lock.packages[''].dependencies[name] !== version) reject('root SRI/version');
}
for (const [relative, record] of Object.entries(lock.packages)) {
  if (!relative) continue;
  if (!/^node_modules\/(?:@?[a-z0-9][a-z0-9._-]*\/)*[a-z0-9][a-z0-9._-]*$/.test(relative)) reject('lock placement');
  const name = record.name ?? relative.split('node_modules/').at(-1);
  if (record.resolved !== `https://registry.npmjs.org/${name}/-/${name.split('/').at(-1)}-${record.version}.tgz` || !/^sha512-[A-Za-z0-9+/]{86}==$/.test(record.integrity)) reject('locked supplier');
}
if (action === 'install') {
  npmCommand(['ci', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/']);
  npmCommand(['install', '--global', '--prefix', prefix, '--offline', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', ...Object.entries(pins).map(([name, [version]]) => `${name}@${version}`)]);
}
const authority = new Map();
for (const [relative, record] of Object.entries(lock.packages)) {
  if (!relative) continue;
  const metadata = path.join(source, relative, 'package.json');
  if (!fs.existsSync(metadata)) {
    if (record.optional !== true) reject('required acquired source absent');
    continue;
  }
  const bytes = read(metadata), identity = JSON.parse(bytes);
  const name = record.name ?? relative.split('node_modules/').at(-1);
  if (identity.name !== name || identity.version !== record.version) reject('acquired identity');
  const key = `${name}@${record.version}`, previous = authority.get(key);
  if (previous && (!previous.bytes.equals(bytes) || previous.integrity !== record.integrity)) reject('ambiguous acquired source');
  authority.set(key, { directory: path.dirname(metadata), bytes, integrity: record.integrity });
}
let count = 0, compared = 0, bytes = 0;
function compare(actual, original, gentle = false) {
  const names = directory => fs.readdirSync(directory).filter(name => name !== 'node_modules' && !(gentle && name === '.gentle-ai')).sort();
  const a = names(actual), b = names(original);
  if (JSON.stringify(a) !== JSON.stringify(b)) reject('global source set differs');
  for (const name of a) {
    if (++compared > 250000) reject('source file count');
    const left = path.join(actual, name), right = path.join(original, name);
    const info = fs.lstatSync(left);
    if (info.isSymbolicLink()) reject('source alias');
    if (info.isDirectory()) compare(left, right);
    else {
      const value = read(left);
      bytes += value.length;
      if (bytes > 1073741824 || !value.equals(read(right))) reject('source bytes/bound');
    }
  }
}
function packages(directory) {
  for (const name of fs.readdirSync(directory).sort()) {
    if (name === '.bin' || name === '.package-lock.json') continue;
    const absolute = path.join(directory, name);
    const info = fs.lstatSync(absolute);
    if (!info.isDirectory() || info.isSymbolicLink()) reject('global package alias/type');
    if (name.startsWith('@')) { packages(absolute); continue; }
    if (++count > 4096) reject('global graph bound');
    const metadata = read(path.join(absolute, 'package.json')), record = JSON.parse(metadata);
    const acquired = authority.get(`${record.name}@${record.version}`);
    if (mainOverlay && absolute === path.join(modules, 'gentle-pi')) {
      // Go has compared every Main source member/set against the retained ZIP.
      // Its authority is the frozen publisher snapshot, not this npm SRI record.
      const peers = { '@earendil-works/pi-coding-agent': '>=0.99.1', '@earendil-works/pi-ai': '*', '@earendil-works/pi-tui': '*', typebox: '*' };
      if (!acquired || record.name !== 'gentle-pi' || record.version !== '4.0.0' || record.engines?.node !== '>=22.19.0' ||
          JSON.stringify(record.dependencies) !== JSON.stringify(JSON.parse(acquired.bytes).dependencies) ||
          Object.keys(record.peerDependencies ?? {}).length !== 4 || Object.entries(peers).some(([name, range]) => record.peerDependencies?.[name] !== range)) reject('unsupported Main dependency/native composition');
    } else {
      if (!acquired || !metadata.equals(acquired.bytes)) reject('global unauthenticated metadata');
      compare(absolute, acquired.directory, record.name === 'gentle-pi');
    }
    if (fs.existsSync(path.join(absolute, 'node_modules'))) packages(path.join(absolute, 'node_modules'));
  }
}
packages(modules);
const gentle = path.join(modules, 'gentle-pi');
const piMetadata = JSON.parse(read(path.join(modules, '@earendil-works/pi-coding-agent/package.json')));
const gentleMetadata = JSON.parse(read(path.join(gentle, 'package.json')));
if (piMetadata.bin?.pi !== 'dist/bundle/cli.js' || gentleMetadata.bin?.['gentle-shell'] !== 'bin/gentle-shell.mjs') reject('stock role bin metadata');
read(path.join(modules, '@earendil-works/pi-coding-agent/dist/bundle/cli.js'));
read(path.join(gentle, 'bin/gentle-shell.mjs'));
// Stock supplier pins authorize installation; Main source is bound by the frozen ZIP.
if (action === 'install' || !mainOverlay) for (const [name, expected] of [
  ['scripts/gentle-ai-installer.mjs', 'bc2da0585026fa538f0c6ae0cf50463767c175b71d0dfdb582a88cbe894c84ca'],
  ['runtime/gentle-ai-binary.mjs', 'cbdf5deac8b7a85ab1253dbd049953aeb192a7d1f7987f9206916ab449c10a92'],
]) if (digest(read(path.join(gentle, name))) !== expected) reject('stock native supplier source pin');
if (action === 'install') {
  const { installGentleAi } = await import(pathToFileURL(path.join(gentle, 'scripts/gentle-ai-installer.mjs')).href);
  await installGentleAi({ packageRoot: gentle }); // Explicit, authenticated stock API, no postinstall/fullscreen hook.
}
const native = path.join(gentle, '.gentle-ai/v4.0.0');
const manifest = JSON.parse(read(path.join(native, 'integrity.json')));
if (manifest.version !== '4.0.0' || manifest.method !== 'go-sumdb-source-build' || manifest.moduleChecksum !== 'h1:pZ/XZ2Pk3U9lgXigOTY62zlxxFOHnc9CjQhLgaV/Hfc=' || manifest.binarySha256 !== digest(read(path.join(native, 'gentle-ai.exe')))) reject('stock native source manifest');
const configPath = path.join(root, 'config/gentle-shell.json');
const homeKey = path.join(finalRoot, 'agent');
if (action === 'install') exclusive(configPath, `${JSON.stringify({ home: 'isolated', provisioned: { [homeKey]: { gentleAi: '4.0.0', gentlePi: gentleMetadata.version, at: new Date().toISOString() } } })}\n`);
const config = JSON.parse(read(configPath));
const provisioned = config?.provisioned, entry = provisioned?.[homeKey];
if (config?.home !== 'isolated' || Object.keys(config).sort().join(',') !== 'home,provisioned' ||
    provisioned === null || typeof provisioned !== 'object' || Array.isArray(provisioned) || Object.keys(provisioned).length !== 1 ||
    entry === null || typeof entry !== 'object' || Array.isArray(entry) || Object.keys(entry).sort().join(',') !== 'at,gentleAi,gentlePi' ||
    entry.gentleAi !== '4.0.0' || entry.gentlePi !== gentleMetadata.version || typeof entry.at !== 'string' ||
    !Number.isFinite(Date.parse(entry.at))) reject('owned Shell provisioning binding changed');
const settingsPath = path.join(agent, 'settings.json');
const settings = { packages: [path.join(finalRoot, 'prefix/node_modules/gentle-pi')], npmCommand: [path.join(finalRoot, 'runtime/node/node.exe'), path.join(finalRoot, 'runtime/node/node_modules/npm/bin/npm-cli.js'), '--prefix', path.join(finalRoot, 'prefix')] };
if (action === 'install') exclusive(settingsPath, `${JSON.stringify(settings, null, 2)}\n`);
const observedSettings = JSON.parse(read(settingsPath));
// Pinned Pi 1.0.0 SettingsManager writes these typed scalars from changelog, model,
// thinking-level and theme selection. A theme is a name lookup, never a path.
// Packages, commands, resources, trust, redirection and telemetry stay refused.
const uiScalar = value => typeof value === 'string' && value.length > 0 && value.length <= 256 && !/[\u0000-\u001f\u007f]/.test(value);
const mutableSettings = {
  lastChangelogVersion: value => typeof value === 'string',
  defaultProvider: uiScalar,
  defaultModel: uiScalar,
  defaultThinkingLevel: value => ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'].includes(value),
  theme: value => uiScalar(value) && /^[A-Za-z0-9][A-Za-z0-9 ._-]*$/.test(value) && !value.includes('..'),
};
// The authenticated launcher excludes exactly this builtin; no resource paths.
const settingsKeys = ['packages', 'npmCommand', 'extensions', ...Object.keys(mutableSettings)];
if (observedSettings === null || typeof observedSettings !== 'object' || Array.isArray(observedSettings) ||
    Object.keys(observedSettings).some(key => !settingsKeys.includes(key)) ||
    Object.entries(mutableSettings).some(([key, valid]) => Object.hasOwn(observedSettings, key) && !valid(observedSettings[key])) ||
    (Object.hasOwn(observedSettings, 'extensions') && JSON.stringify(observedSettings.extensions) !== '["-builtin:codemode"]') ||
    JSON.stringify({ packages: observedSettings.packages, npmCommand: observedSettings.npmCommand }) !== JSON.stringify(settings)) {
  reject('owned package/settings bindings changed');
}
if (!read(lockPath).equals(lockBytes)) reject('source lock changed');
console.log(`Windows stock composition verified; packages=${count}; registration and full Ready remain unqualified`);
