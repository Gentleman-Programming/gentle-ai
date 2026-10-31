// Owned, bounded stock-npm composition. No custom Pi updater or lifecycle hooks.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const [root, prefix, agent, finalPrefix, finalRoot, mode, requestedAction = 'install', recoveryAuthority] = process.argv.slice(2);
const preparing = requestedAction === 'prepare-prior';
const action = preparing ? 'install' : requestedAction;
const selectingSDK = action === 'select-main-sdk-package';
const verifyingSelectedSDK = action === 'verify-main-sdk-selected';
const mainRuntimeActions = ['materialize-main-sdk-runtime', 'verify-main-sdk-runtime', 'select-main-sdk-package', 'verify-main-sdk-selected'];
const mainActions = ['verify-main-source', 'verify-main-sdk-source', 'materialize-main-sdk-source', 'verify-main-sdk-composition', ...mainRuntimeActions];
// Main runtime placement and selection are qualified where the stock native
// member is pinned for this POSIX flow: Linux amd64 and macOS arm64.
const mainPlatform = (process.platform === 'linux' && process.arch === 'x64') || (process.platform === 'darwin' && process.arch === 'arm64');
const uid = process.getuid();
// npm materializes the acquired tree under the inherited login umask, which is
// group-writable (0002) on common distros, and the audit below rejects every
// group-writable object. Pin a deterministic 022 so acquisition is independent
// of the invoking shell; the script's own explicit 0700/0600 modes are unaffected.
process.umask(0o022);
const reject = message => { throw Error(`global provision refused: ${message}`); };
const digest = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const packageName = /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/;
const rootNames = ['gentle-pi', '@earendil-works/pi-coding-agent', '@earendil-works/pi-tui', '@heyhuynhgiabuu/pi-pretty', 'typebox'];
// The Engram companion is an optional sixth root of the generated Stable
// Separate lock only, present when the listed gentle-pi pins it (a gentle-pi
// before 4.1.0 pins none). The frozen locks keep their five roots.
const companionName = 'gentle-engram';
function lockRoots(dependencies) {
  const names = Object.keys(dependencies ?? {}).sort();
  const base = [...rootNames].sort();
  return JSON.stringify(names) === JSON.stringify(base) || JSON.stringify(names) === JSON.stringify([...base, companionName].sort()) ? names : null;
}
// Stable Separate private-global route (S19): gentle-pi is declared at the
// exact newest release of the staged reviewed supplier list, the companion at
// that release's exact pin, and the other four roots at `latest`; the lock is
// generated at install time like Windows.
const reviewedSeed = { name: 'gentle-shell-owned-seed', version: '1.0.0' };
function rootDeclaration(target, companion = null) {
  const dependencies = Object.fromEntries(rootNames.map(name => [name, name === 'gentle-pi' ? target : 'latest']));
  if (companion) dependencies[companionName] = companion;
  return { name: reviewedSeed.name, version: reviewedSeed.version, private: true, dependencies };
}
// gentle-pi pins its companion as bundled Shell declares it
// (runtime/gentle-shell-bundled.mjs ENGRAM_COMPANION_SOURCE); never `latest`.
function companionPin(text) {
  const match = /^export const ENGRAM_COMPANION_SOURCE = "npm:gentle-engram@((?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*))";$/m.exec(text ?? '');
  return match ? match[1] : null;
}
// The companion is declared exactly as bundled Shell declares it, and only
// when the gentle-pi pin, the lock root and the installed package agree. Any
// disagreement keeps the install and leaves Shell usable without memory.
function companionDeclaration(pin, locked, installed) {
  if (pin && locked === pin && installed === pin) return `npm:${companionName}@${pin}`;
  if (pin || locked || installed) console.error(`Engram companion not declared: gentle-pi pins ${pin ?? 'none'}, lock ${locked ?? 'none'}, installed ${installed ?? 'none'}; Shell stays usable without memory`);
  return null;
}
// The verified Engram core at the contract path <agent>/bin/engram. Its pin,
// download, digest and archive reading belong to the provisioned gentle-pi;
// one without them, or any failure, skips memory instead of the install.
async function publishEngramCore(gentle, bin, name, platform, arch, adapters = {}) {
  let downloads, bundled;
  try {
    downloads = await import(pathToFileURL(path.join(gentle, 'scripts/installer-downloads.mjs')).href);
    bundled = await import(pathToFileURL(path.join(gentle, 'scripts/bundled-install.mjs')).href);
  } catch { /* An older gentle-pi ships neither module. */ }
  if (typeof downloads?.artifactFor !== 'function' || typeof downloads.verifiedDownload !== 'function' || typeof bundled?.engramExecutable !== 'function') {
    console.error('Engram core skipped: the installed gentle-pi pins no Engram core; Shell stays usable without memory');
    return false;
  }
  try {
    const descriptor = (adapters.artifact ?? downloads.artifactFor)('engram', platform, arch);
    const executable = bundled.engramExecutable(await downloads.verifiedDownload('engram', adapters, platform, arch), descriptor);
    if (!fs.existsSync(bin)) fs.mkdirSync(bin, { mode: 0o700 });
    const target = path.join(bin, name), stage = path.join(bin, `.${name}-${crypto.randomUUID()}`);
    absent(target);
    writeExclusive(stage, executable, 0o700);
    fs.renameSync(stage, target);
    sync(bin);
    return true;
  } catch (error) {
    console.error(`Engram core skipped: ${error.message}; Shell stays usable without memory`);
    return false;
  }
}
// Memory is the companion and the core together: the core is published only
// for a declared companion, so no Engram runs for no client.
async function engramMemory(pin, locked, installed, gentle, bin, name, platform, arch, adapters = {}) {
  const declaration = companionDeclaration(pin, locked, installed);
  if (declaration) await publishEngramCore(gentle, bin, name, platform, arch, adapters);
  return declaration;
}
// The staged reviewed supplier list (gentle-pi-suppliers.json, schema
// gentle-shell-reviewed-suppliers/v1) is written by the Go worker from the
// list authority its detection resolved. This re-validation never widens it:
// each entry binds the exact version to its npm tarball SRI and to the SHA-256
// of the two native supplier files inside the package. Returns null when the
// stage carries no list: a legacy installation, or a Main preview install or
// verify, which keep the frozen-lock route and its Go-side supplier
// authentication exactly as before this feature.
function readReviewedList() {
  const listPath = path.join(root, 'gentle-pi-suppliers.json');
  let raw;
  try { raw = read(listPath); }
  catch (error) { if (error.code === 'ENOENT') return null; throw error; }
  // The staged list read is bounded at 256 KiB, matching the Go loader's bound.
  if (raw.length > 262144) reject('reviewed supplier list');
  const list = JSON.parse(raw);
  const semver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
  const sri = value => typeof value === 'string' && /^sha512-[A-Za-z0-9+/]{86}==$/.test(value) &&
    Buffer.from(value.slice(7), 'base64').toString('base64') === value.slice(7);
  if (list === null || typeof list !== 'object' || Array.isArray(list) || list.schema !== 'gentle-shell-reviewed-suppliers/v1' ||
      list.releases === null || typeof list.releases !== 'object' || Array.isArray(list.releases)) reject('reviewed supplier list');
  if (Object.keys(list).sort().join(',') !== 'releases,schema') reject('reviewed supplier list');
  for (const [version, entry] of Object.entries(list.releases)) {
    if (!semver.test(version) || entry === null || typeof entry !== 'object' || Array.isArray(entry) ||
        Object.keys(entry).sort().join(',') !== 'binary,installer,integrity' || !sri(entry.integrity) ||
        !/^[0-9a-f]{64}$/.test(entry.installer) || !/^[0-9a-f]{64}$/.test(entry.binary)) reject('reviewed supplier list');
  }
  if (Object.keys(list.releases).length === 0) reject('reviewed supplier list');
  return list.releases;
}
// Newest reviewed version in semantic order (4.10.0 > 4.9.0): the exact
// gentle-pi version the Stable install declares.
function newestReviewedListVersion(table) {
  const order = version => version.split('.').map(Number);
  return Object.keys(table).sort((left, right) => {
    const a = order(left), b = order(right);
    return a[0] - b[0] || a[1] - b[1] || a[2] - b[2];
  }).at(-1);
}
function physical(absolute, directory = false) {
  if (!path.isAbsolute(absolute) || path.resolve(absolute) !== absolute || fs.realpathSync(absolute) !== absolute) reject('noncanonical path');
  const stat = fs.lstatSync(absolute);
  if (stat.uid !== uid || (stat.mode & 0o022) !== 0 || (directory ? !stat.isDirectory() : !stat.isFile())) reject('foreign or nonphysical object');
  return stat;
}
function read(absolute) {
  const before = physical(absolute);
  if (before.size > 33554432) reject('file byte bound');
  const bytes = fs.readFileSync(absolute);
  const after = physical(absolute);
  if (before.ino !== after.ino || before.dev !== after.dev || before.size !== after.size || before.mtimeMs !== after.mtimeMs || before.ctimeMs !== after.ctimeMs) reject('source preimage changed');
  return bytes;
}
function absent(absolute) {
  try { fs.lstatSync(absolute); reject('occupied path'); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
}
function sync(absolute) {
  const fd = fs.openSync(absolute, 'r');
  try { fs.fsyncSync(fd); } finally { fs.closeSync(fd); }
}
function syncTree(directory) {
  for (const name of fs.readdirSync(directory)) {
    const absolute = path.join(directory, name), info = fs.lstatSync(absolute);
    if (info.isDirectory()) syncTree(absolute);
    else if (info.isFile()) sync(absolute);
  }
  sync(directory);
}
function writeExclusive(absolute, bytes, permissions = 0o600) {
  fs.writeFileSync(absolute, bytes, { flag: 'wx', mode: permissions });
  if (!read(absolute).equals(Buffer.from(bytes))) reject('exclusive write readback');
  sync(absolute);
  sync(path.dirname(absolute));
}
for (const value of [root, prefix, agent, finalPrefix, finalRoot]) {
  if (typeof value !== 'string' || !/^\/[A-Za-z0-9_./-]+$/.test(value) || path.resolve(value) !== value) reject('unsafe selection');
}
if (process.argv.length !== (requestedAction === 'restore' ? 10 : 9) || !['separate', 'shared', 'agent'].includes(mode) || !['install', 'verify', 'restore', 'prepare-prior', ...mainActions].includes(requestedAction) || uid === 0) reject('arguments or UID');
physical(root, true);
const state = path.join(root, 'state');
physical(state, true);
const node = path.join(root, 'runtime/node/bin/node');
const npm = path.join(root, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js');
physical(node);
physical(npm);
const cache = path.join(root, 'runtime/cache');
let source = path.join(root, 'runtime/project');
const modules = path.join(prefix, 'lib/node_modules');
const settingsPath = path.join(agent, 'settings.json');
const upgradeState = path.join(state, 'upgrade');
const recoveryState = fs.existsSync(path.join(upgradeState, 'selection.json')) ? upgradeState : state;
const selectionPath = path.join(recoveryState, 'selection.json');
const priorGraph = path.join(state, 'prior-global-graph.json');
const env = {
  HOME: path.join(root, 'home'), TMPDIR: path.join(root, 'tmp'),
  XDG_CONFIG_HOME: path.join(root, 'config'), XDG_STATE_HOME: state,
  PATH: `${path.dirname(node)}:/usr/bin:/bin`, NODE_USE_SYSTEM_CA: '1',
  NPM_CONFIG_USERCONFIG: path.join(root, 'config/user.npmrc'),
  NPM_CONFIG_GLOBALCONFIG: path.join(root, 'config/global.npmrc'),
  NPM_CONFIG_IGNORE_SCRIPTS: 'true', npm_config_ignore_scripts: 'true',
  NPM_CONFIG_PREFIX: prefix, npm_config_prefix: prefix, NPM_CONFIG_CACHE: cache,
};
const NPM_NETWORK_TIMEOUT = 1200000; // Network-bound npm runs must not depend on connection speed.
function stockNpm(args, cwd = source, timeout = NPM_NETWORK_TIMEOUT) {
  const started = Date.now();
  let child = spawnSync(node, [npm, ...args], {
    cwd, env, shell: false, timeout, killSignal: 'SIGKILL',
    maxBuffer: 8388608, encoding: 'utf8',
  });
  if (child.error) {
    // Transport-class failure (spawn error or the spawn timeout): one bounded
    // retry with a fresh window separates a slow connection from a dead one.
    child = spawnSync(node, [npm, ...args], {
      cwd, env, shell: false, timeout, killSignal: 'SIGKILL',
      maxBuffer: 8388608, encoding: 'utf8',
    });
  }
  if (child.error || child.signal || child.status !== 0) {
    const output = (child.stdout ?? '') + (child.stderr ?? '');
    const detail = Buffer.byteLength(output) < 1024 && !output.includes('\0') && !output.includes('\ufffd')
      ? output : `entire npm output withheld; bytes=${Buffer.byteLength(output)}; sha256=${digest(Buffer.from(output))}`;
    reject(`stock npm ${args[0]} failed; status=${child.status}; signal=${child.signal}; error=${child.error?.code ?? ''}; elapsedMs=${Date.now() - started}; ${detail}`);
  }
  return child.stdout;
}
function inventory(directory) {
  const records = [];
  let bytes = 0;
  function visit(absolute) {
    if (records.length >= 250000) reject('inventory entry bound');
    const info = fs.lstatSync(absolute);
    if (info.uid !== uid) reject('inventory foreign owner');
    const relative = path.relative(directory, absolute);
    if (info.isSymbolicLink()) {
      const target = fs.realpathSync(absolute);
      if (!target.startsWith(directory + '/')) reject('inventory escaping link');
      records.push([relative, 'link', fs.readlinkSync(absolute)]);
    } else if (info.isDirectory()) {
      physical(absolute, true);
      records.push([relative, 'dir', info.mode & 0o777]);
      for (const child of fs.readdirSync(absolute).sort()) visit(path.join(absolute, child));
    } else {
      if (!info.isFile()) reject('inventory special file');
      const raw = read(absolute);
      bytes += raw.length;
      if (bytes > 1073741824) reject('inventory aggregate bound');
      records.push([relative, 'file', info.mode & 0o777, digest(raw)]);
    }
  }
  visit(directory);
  return digest(JSON.stringify(records));
}
// Stock fs.cp creates directories with default modes, not the source modes.
// Readback includes modes, so preserve them for acquisition and recovery copies.
function copyTree(from, to) {
  fs.cpSync(from, to, { recursive: true, force: false, errorOnExist: true, dereference: false, verbatimSymlinks: true });
  function modes(original, copied) {
    const info = fs.lstatSync(original);
    if (!info.isDirectory()) return;
    for (const name of fs.readdirSync(original)) modes(path.join(original, name), path.join(copied, name));
    fs.chmodSync(copied, info.mode & 0o777);
  }
  modes(from, to);
}
function snapshot(upgrade = false) {
  const store = upgrade ? upgradeState : state;
  if (upgrade) {
    absent(store);
    fs.mkdirSync(store, { mode: 0o700 });
  }
  const before = { mode: upgrade ? 'shared' : mode, prefix, agent, finalPrefix, finalRoot, originalMode: mode };
  if (before.mode === 'shared') {
    physical(prefix, true);
    physical(agent, true);
    before.prefixSHA = inventory(prefix);
    before.agentSHA = inventory(agent);
    for (const [from, name] of [[prefix, 'prefix.preimage'], [agent, 'agent.preimage']]) {
      const to = path.join(store, name);
      absent(to);
      copyTree(from, to);
      if (inventory(to) !== inventory(from)) reject('snapshot readback differs');
      syncTree(to);
    }
    if (inventory(prefix) !== before.prefixSHA || inventory(agent) !== before.agentSHA) reject('selected files changed during snapshot');
  }
  writeExclusive(path.join(store, 'selection.json'), `${JSON.stringify(before)}\n`);
}
function restore() {
  const authority = JSON.parse(recoveryAuthority);
  const selection = JSON.parse(read(selectionPath));
  if (mode !== 'shared' || selection.mode !== 'shared' || selection.prefix !== prefix || selection.agent !== agent || selection.finalRoot !== finalRoot || selection.finalPrefix !== finalPrefix || finalPrefix !== prefix) reject('recovery selection differs');
  const targets = [[prefix, 'prefix.preimage', selection.prefixSHA], [agent, 'agent.preimage', selection.agentSHA]];
  function validate() {
    if (!authority || !/^[a-f0-9]{64}$/.test(authority.selectionSHA) || digest(read(selectionPath)) !== authority.selectionSHA) reject('confirmed recovery selection changed');
    for (const [target, key] of [[root, 'rootID'], [prefix, 'prefixID'], [agent, 'agentID']]) {
      physical(target, true);
      const stat = fs.lstatSync(target, { bigint: true });
      if (`${stat.dev}:${stat.ino}` !== authority[key]) reject('confirmed recovery root changed');
    }
    // Check both saved trees before touching either live target.
    for (const [, name, expected] of targets) {
      if (!/^[a-f0-9]{64}$/.test(expected) || inventory(path.join(recoveryState, name)) !== expected) reject('recovery preimage differs');
    }
  }
  validate();
  for (const [target, name, expected] of targets) {
    const saved = path.join(recoveryState, name);
    let unchanged = false;
    try { unchanged = inventory(target) === expected; } catch { /* Damaged live contents are untrusted evidence, not restore authority. */ }
    if (unchanged) continue;
    validate();
    const quarantine = path.join(recoveryState, `${name}.quarantine-${crypto.randomUUID()}`);
    absent(quarantine);
    fs.mkdirSync(quarantine, { mode: 0o700 });
    // Same-filesystem renames preserve uncertain new evidence, not destructive rm.
    for (const entry of fs.readdirSync(target)) fs.renameSync(path.join(target, entry), path.join(quarantine, entry));
    copyTree(saved, target);
    if (inventory(target) !== expected) reject('restored tree differs');
  }
  console.log('Shared preimages restored; uncertain new evidence retained');
}
if (action === 'restore') {
  restore();
} else {
  if (preparing) {
    const check = spawnSync(node, [process.argv[1], root, prefix, agent, finalPrefix, finalRoot, mode, 'verify'], { cwd: root, env, timeout: 60000, maxBuffer: 8388608 });
    if (check.error || check.signal || check.status !== 0) reject('fixture prior requires verified existing installation');
  }
  const prior = preparing || (fs.existsSync(priorGraph) && JSON.parse(read(path.join(modules, '@earendil-works/pi-coding-agent/package.json'))).version === '0.99.2');
  if (mainActions.includes(action) && ((mode !== 'separate' && mode !== 'agent') || prior)) reject('Main preflight requires modern Separate or agent stock');
  if (prior) source = path.join(root, 'runtime/prior');
  const graphPath = prior ? priorGraph : path.join(state, 'global-graph.json');
  const lockPath = path.join(source, 'package-lock.json');
  const reviewed = prior ? null : readReviewedList();
  // Only the Stable channel takes the generated reviewed-list route: the Go
  // worker stages the selected list into every Stable install stage and fails
  // the install when it cannot, so an install stage without one is a Main
  // preview request, which keeps the frozen modern lock's stock base exactly
  // as before the route existed (S23/M4). A verify or Main action without a
  // staged list is likewise a legacy installation or a Main tree, checked
  // against its frozen lock as before, with supplier authentication enforced
  // at the Go launch boundary against the embedded list entry (S23/M1).
  const frozenLock = prior || reviewed === null;
  const versionPattern = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/;
  // Every per-package canonical-URL/SRI gate, shared by the frozen and the
  // generated lock route. The refusal slugs name the route that produced
  // them: the generated reviewed-list route never reports "frozen" (S23/L2).
  function assertCanonicalPackages(lock, route) {
    const identities = new Map();
    for (const [relative, entry] of Object.entries(lock.packages)) {
      if (!relative) continue;
      if (!/^node_modules\/(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*(?:\/node_modules\/(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*)*$/.test(relative)) reject(`${route} lock path`);
      const name = relative.split('node_modules/').at(-1);
      const version = typeof entry?.version === 'string' && versionPattern.exec(entry.version);
      if (!entry || entry.link || (entry.name !== undefined && entry.name !== name) || !packageName.test(name) ||
          !version || version[4]?.split('.').some(part => /^0[0-9]+$/.test(part))) reject(`${route} package identity`);
      const canonical = `https://registry.npmjs.org/${name}/-/${name.split('/').at(-1)}-${entry.version}.tgz`;
      if (entry.resolved !== canonical || typeof entry.integrity !== 'string' || !/^sha512-[A-Za-z0-9+/]{86}==$/.test(entry.integrity) ||
          Buffer.from(entry.integrity.slice(7), 'base64').toString('base64') !== entry.integrity.slice(7)) reject(`${route} source integrity`);
      const key = `${name}@${entry.version}`;
      if (identities.has(key) && identities.get(key) !== entry.integrity) reject(`ambiguous ${route} identity`);
      identities.set(key, entry.integrity);
    }
  }
  let lockBytes, lock, seed, pins, companion;
  if (frozenLock) {
    lockBytes = read(path.join(root, 'user-locks', prior ? 'prior' : 'modern', 'package-lock.json'));
    lock = JSON.parse(lockBytes);
    seed = lock.packages?.[''];
    if (lock.lockfileVersion !== 3 || !lock.packages || Array.isArray(lock.packages) || !seed?.dependencies ||
        lock.name !== seed.name || lock.version !== seed.version || typeof seed.name !== 'string' || !packageName.test(seed.name) ||
        JSON.stringify(Object.keys(seed.dependencies).sort()) !== JSON.stringify([...rootNames].sort())) reject('frozen source lock');
    if (typeof seed.version !== 'string' || !versionPattern.test(seed.version)) reject('frozen seed version');
    assertCanonicalPackages(lock, 'frozen');
    pins = Object.fromEntries(rootNames.map(name => {
      const entry = lock.packages[`node_modules/${name}`];
      if (!entry || seed.dependencies[name] !== entry.version) reject('frozen root declaration');
      return [name, [entry.version, entry.integrity]];
    }));
    if (prior && pins['@earendil-works/pi-coding-agent'][0] !== '0.99.2') reject('frozen coding profile');
    if (prior && lock.packages['node_modules/@earendil-works/pi-coding-agent/node_modules/@earendil-works/pi-tui']?.version !== '0.99.2') reject('prior nested TUI placement');
    if (action === 'install') {
      physical(agent, true);
      snapshot(preparing);
      for (const directory of (preparing ? [source] : [cache, source])) {
        absent(directory);
        fs.mkdirSync(directory, { mode: 0o700 });
      }
      writeExclusive(path.join(source, 'package.json'), `${JSON.stringify({ name: seed.name, version: seed.version, private: true, dependencies: seed.dependencies })}\n`);
      writeExclusive(path.join(source, 'package-lock.json'), lockBytes);
    }
  } else {
    // Generated-lock route (S19): declare gentle-pi at the exact newest listed
    // release and the other roots at `latest`, resolve the lock at install
    // time, and complete its SRI records like the Windows provisioner. An
    // unknown `latest` never occurs for gentle-pi because it is pinned to a
    // listed version; a listed version with a mismatch is refused below.
    const target = newestReviewedListVersion(reviewed);
    if (action === 'install') {
      physical(agent, true);
      snapshot(preparing);
      for (const directory of [cache, source]) {
        absent(directory);
        fs.mkdirSync(directory, { mode: 0o700 });
      }
      // Separate only (B1): Shared and agent installs keep the five stock
      // roots. The companion pin is read once, before the lock exists, from
      // the listed tarball stock npm fetches into the private cache (npm ci
      // reuses it), only after its bytes match the reviewed SRI.
      if (mode === 'separate') {
        const packed = path.join(root, 'tmp', `gentle-pi-${crypto.randomUUID()}`);
        fs.mkdirSync(packed, { mode: 0o700 });
        stockNpm(['pack', `gentle-pi@${target}`, `--pack-destination=${packed}`, '--ignore-scripts', '--registry=https://registry.npmjs.org/']);
        const tarball = read(path.join(packed, `gentle-pi-${target}.tgz`));
        if (`sha512-${crypto.createHash('sha512').update(tarball).digest('base64')}` !== reviewed[target].integrity) reject('stock supplier pin');
        const member = spawnSync('/usr/bin/tar', ['-xOzf', '-', 'package/runtime/gentle-shell-bundled.mjs'], { input: tarball, env, shell: false, timeout: 60000, maxBuffer: 8388608, encoding: 'utf8' });
        companion = member.status === 0 ? companionPin(member.stdout) : null;
        fs.unlinkSync(path.join(packed, `gentle-pi-${target}.tgz`));
        fs.rmdirSync(packed);
      }
      writeExclusive(path.join(source, 'package.json'), `${JSON.stringify(rootDeclaration(target, companion))}\n`);
      stockNpm(['install', '--package-lock-only', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/']);
      const { completeFile } = await import(pathToFileURL(path.join(root, 'complete-generated-lock-sri.mjs')).href);
      await completeFile(lockPath);
    }
    lockBytes = read(lockPath);
    lock = JSON.parse(lockBytes);
    seed = lock.packages?.[''];
    if (lock.lockfileVersion !== 3 || !lock.packages || Array.isArray(lock.packages) || !seed?.dependencies ||
        lock.name !== seed.name || lock.version !== seed.version || typeof seed.name !== 'string' || !packageName.test(seed.name) ||
        !lockRoots(seed.dependencies)) reject('generated source lock');
    if (typeof seed.version !== 'string' || !versionPattern.test(seed.version)) reject('generated seed version');
    assertCanonicalPackages(lock, 'generated');
    pins = Object.fromEntries(lockRoots(seed.dependencies).map(name => {
      const entry = lock.packages[`node_modules/${name}`];
      if (!entry) reject('generated root declaration');
      return [name, [entry.version, entry.integrity]];
    }));
    // The locked gentle-pi must be the declared listed release with the
    // entry's exact npm tarball SRI, on install and on every later verify.
    const listed = reviewed[pins['gentle-pi'][0]];
    if (!listed || listed.integrity !== pins['gentle-pi'][1] ||
        (action === 'install' && pins['gentle-pi'][0] !== target)) reject('stock supplier pin');
  }
  if (frozenLock && !read(lockPath).equals(lockBytes)) reject('selected frozen lock differs');
  if (action === 'install') {
    // The generated route resolves nothing new at ci time, so it mirrors the
    // Windows flags; the frozen prior route keeps its conservative profile.
    stockNpm(frozenLock
      ? ['ci', '--install-strategy=shallow', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=3', '--registry=https://registry.npmjs.org/']
      : ['ci', '--ignore-scripts', '--engine-strict', '--no-audit', '--no-fund', '--min-release-age=0', '--registry=https://registry.npmjs.org/']);
    if (!read(lockPath).equals(lockBytes)) reject(frozenLock ? 'npm ci changed the frozen lock' : 'npm ci changed the generated lock');
  }
  const retainedRoot = path.join(source, '.gentle-shell-optional');
  const retainedManifest = path.join(source, '.gentle-shell-optional.json');
  function matches(rule, value) {
    if (rule === undefined) return true;
    if (!Array.isArray(rule) || rule.some(item => typeof item !== 'string' || !/^!?[a-z0-9_]+$/.test(item))) reject('platform rule');
    return !rule.includes(`!${value}`) && !rule.includes('!any') && (!rule.some(item => !item.startsWith('!')) || rule.includes(value) || rule.includes('any'));
  }
  // Darwin arm64 has no libc family: like npm, any libc rule makes a record inapplicable there.
  const platform = process.platform === 'darwin' ? { os: 'darwin', cpu: 'arm64', libc: null } : { os: 'linux', cpu: 'x64', libc: 'glibc' };
  const applicable = record => [matches(record.os, platform.os), matches(record.cpu, platform.cpu), matches(record.libc, platform.libc)].every(Boolean);
  if (action === 'install') {
    const actual = [];
    function acquiredPackages(directory) {
      physical(directory, true);
      for (const name of fs.readdirSync(directory).sort()) {
        if (name === '.bin' || name === '.package-lock.json') continue;
        const absolute = path.join(directory, name);
        physical(absolute, true);
        if (name.startsWith('@')) acquiredPackages(absolute);
        else {
          actual.push(path.relative(source, absolute));
          if (actual.length > 4096) reject('acquired closure bound');
          const nested = path.join(absolute, 'node_modules');
          if (fs.existsSync(nested)) acquiredPackages(nested);
        }
      }
    }
    acquiredPackages(path.join(source, 'node_modules'));
    const expected = Object.entries(lock.packages).filter(([key, record]) => key && (!record.optional || applicable(record))).map(([key]) => key);
    const { normalize } = await import(pathToFileURL(path.join(root, 'normalize-private-optional-platform-closure.mjs')).href);
    absent(retainedRoot);
    fs.mkdirSync(retainedRoot, { mode: 0o700 });
    // Preserve acquired sources in this unpublished stage; never normalize the global prefix.
    const removed = normalize({ root: source, lock, expectedPaths: expected, actualPaths: actual, platform }, { remove: absolute => {
      const saved = path.join(retainedRoot, path.relative(source, absolute));
      absent(saved);
      fs.mkdirSync(path.dirname(saved), { recursive: true, mode: 0o700 });
      fs.renameSync(absolute, saved);
      sync(path.dirname(absolute));
      sync(path.dirname(saved));
    } });
    syncTree(retainedRoot);
    writeExclusive(retainedManifest, `${JSON.stringify({ paths: removed, sha256: inventory(retainedRoot) })}\n`);
  }
  const retained = JSON.parse(read(retainedManifest));
  if (!Array.isArray(retained.paths) || retained.paths.length > 4096 || new Set(retained.paths).size !== retained.paths.length || inventory(retainedRoot) !== retained.sha256) reject('retained acquisition inventory differs');
  for (const key of retained.paths) {
    const record = lock.packages[key];
    if (typeof key !== 'string' || !/^node_modules\/(?:@?[a-z0-9][a-z0-9._-]*\/)*[a-z0-9][a-z0-9._-]*$/.test(key) || !record || record.optional !== true || applicable(record) || Object.keys(pins).some(name => key === `node_modules/${name}`)) reject('retained acquisition is not locked nonapplicable optional');
  }
  const authority = new Map();
  for (const [relative, entry] of Object.entries(lock.packages)) {
    if (!relative) continue;
    if (!/^node_modules\/(?:@?[a-z0-9][a-z0-9._-]*\/)*[a-z0-9][a-z0-9._-]*$/.test(relative)) reject('lock path');
    const name = entry.name ?? relative.split('node_modules/').at(-1);
    const canonical = `https://registry.npmjs.org/${name}/-/${name.split('/').at(-1)}-${entry.version}.tgz`;
    if (!packageName.test(name) || entry.resolved !== canonical || !/^sha512-[A-Za-z0-9+/]{86}==$/.test(entry.integrity)) reject('source integrity');
    let metadata = path.join(source, relative, 'package.json');
    if (!fs.existsSync(metadata)) {
      if (entry.optional !== true) reject('required acquired metadata absent');
      if (applicable(entry) || !retained.paths.some(key => relative === key || relative.startsWith(`${key}/`))) continue;
      metadata = path.join(retainedRoot, relative, 'package.json');
      if (!fs.existsSync(metadata)) reject('retained acquired metadata absent');
    }
    const bytes = read(metadata);
    const identity = JSON.parse(bytes);
    if (identity.name !== name || identity.version !== entry.version) reject('source identity');
    const key = `${name}@${entry.version}`;
    const prior = authority.get(key);
    if (prior && (prior.integrity !== entry.integrity || !prior.bytes.equals(bytes))) reject('ambiguous acquired identity');
    authority.set(key, { bytes, integrity: entry.integrity, source: path.dirname(metadata) });
  }
  let priorSettings;
  try { priorSettings = read(settingsPath); }
  catch (error) { if (error.code !== 'ENOENT') throw error; }
  const settings = priorSettings ? JSON.parse(priorSettings) : {};
  if (!settings || typeof settings !== 'object' || Array.isArray(settings) || (settings.packages !== undefined && !Array.isArray(settings.packages))) reject('malformed settings');
  const packageRoot = path.join(finalPrefix, 'lib/node_modules/gentle-pi');
  const mainPackageRoot = path.join(finalRoot, 'runtime/main-sdk');
  let selectedAgentIdentity;
  if (selectingSDK || verifyingSelectedSDK) {
    if (!mainPlatform || (mode !== 'separate' && mode !== 'agent') || prefix !== path.join(root, 'prefix') || (mode === 'separate' && agent !== path.join(root, 'agent')) || (mode === 'agent' && !path.isAbsolute(agent)) || finalPrefix !== path.join(finalRoot, 'prefix')) reject('Main SDK selection requires Linux amd64 or macOS arm64 Separate or agent reuse');
    selectedAgentIdentity = physical(agent, true);
    if (mode === 'agent' ? (selectedAgentIdentity.mode & 0o022) !== 0 : (selectedAgentIdentity.mode & 0o7777) !== 0o700) reject('Main SDK agent custody');
    const info = physical(settingsPath);
    if (info.nlink !== 1 || (mode === 'agent' ? (info.mode & 0o022) !== 0 : (info.mode & 0o7777) !== 0o600)) reject('Main SDK settings custody');
    const stockDeclarations = settings.packages?.filter(entry => (typeof entry === 'string' ? entry : entry?.source) === packageRoot) ?? [];
    const mainDeclarations = settings.packages?.filter(entry => (typeof entry === 'string' ? entry : entry?.source) === mainPackageRoot) ?? [];
    if (selectingSDK) {
      absent(path.join(root, 'installation.json')); // Unpublished stage only.
      if (stockDeclarations.length !== 1 || stockDeclarations[0] !== packageRoot || mainDeclarations.length !== 0) reject('Main SDK selection requires one plain stock declaration');
    } else if (stockDeclarations.length !== 0 || mainDeclarations.length !== 1 || mainDeclarations[0] !== mainPackageRoot) reject('Main SDK selected declaration differs');
  }
  if (action === 'install' && !preparing) {
    if (settings.npmCommand !== undefined) reject('foreign npm override');
    for (const entry of settings.packages ?? []) {
      const value = typeof entry === 'string' ? entry : entry?.source;
      if (typeof value !== 'string' || value.startsWith('npm:gentle-pi') || value === packageRoot) reject('foreign Gentle declaration');
    }
    if (mode === 'separate' || mode === 'agent') {
      absent(prefix);
      fs.mkdirSync(prefix, { mode: 0o700 });
    }
  }
  const observed = [];
  let comparedFiles = 0;
  function sourceBytes(directory, acquired, gentle = false) {
    const include = name => name !== 'node_modules' && !(gentle && name === '.gentle-ai');
    const entries = fs.readdirSync(directory).filter(include).sort();
    const expected = fs.readdirSync(acquired).filter(include).sort();
    if (JSON.stringify(entries) !== JSON.stringify(expected)) reject('global source file set differs');
    for (const name of entries) {
      const actual = path.join(directory, name), original = path.join(acquired, name);
      if (fs.lstatSync(actual).isDirectory()) {
        physical(actual, true);
        physical(original, true);
        sourceBytes(actual, original);
      } else if (++comparedFiles > 250000 || !read(actual).equals(read(original))) reject('global source bytes differ');
    }
  }
  function packages(directory) {
    physical(directory, true);
    for (const name of fs.readdirSync(directory).sort()) {
      if (name === '.bin' || name === '.package-lock.json') continue;
      const entry = path.join(directory, name);
      physical(entry, true);
      if (name.startsWith('@')) {
        for (const leaf of fs.readdirSync(entry).sort()) visit(path.join(entry, leaf));
      } else visit(entry);
    }
  }
  function visit(directory) {
    if (observed.length >= 4096) reject('global graph bound');
    physical(directory, true);
    const bytes = read(path.join(directory, 'package.json'));
    const metadata = JSON.parse(bytes);
    const acquired = authority.get(`${metadata.name}@${metadata.version}`);
    if (!acquired || !bytes.equals(acquired.bytes)) reject(`global package lacks authenticated metadata: ${JSON.stringify({ name: metadata.name, version: metadata.version, placement: path.relative(prefix, directory), actualSHA256: digest(bytes), acquiredSHA256: acquired ? digest(acquired.bytes) : null })}`);
    sourceBytes(directory, acquired.source, metadata.name === 'gentle-pi');
    observed.push({ directory: path.relative(prefix, directory), name: metadata.name, version: metadata.version, integrity: acquired.integrity });
    const nested = path.join(directory, 'node_modules');
    if (fs.existsSync(nested)) packages(nested);
  }
  function nativeState(native = path.join(modules, 'gentle-pi/.gentle-ai'), required = false) {
    if (!fs.existsSync(native)) {
      if (required) reject('required stock native absent');
      return null;
    }
    physical(native, true);
    const entries = fs.readdirSync(native).sort();
    if (entries.length === 0) {
      if (required) reject('required stock native absent');
      return null;
    }
    // The stock installer publishes exactly one version bundle named after its
    // own pinned Gentle AI release; the version is read, never frozen here.
    if (entries.length !== 1 || !versionPattern.test(entries[0].slice(1)) || !entries[0].startsWith('v')) reject('foreign native recovery state');
    const version = entries[0].slice(1);
    const bundle = path.join(native, entries[0]);
    physical(bundle, true);
    if (JSON.stringify(fs.readdirSync(bundle).sort()) !== '["gentle-ai","integrity.json"]') reject('native file set');
    const binary = read(path.join(bundle, 'gentle-ai'));
    const manifestBytes = read(path.join(bundle, 'integrity.json'));
    const manifest = JSON.parse(manifestBytes);
    if (manifest === null || typeof manifest !== 'object' || Array.isArray(manifest) || manifest.version !== version ||
        typeof manifest.binarySha256 !== 'string' || !/^[0-9a-f]{64}$/.test(manifest.binarySha256) ||
        manifestBytes.toString() !== `${JSON.stringify(manifest)}\n`) reject('native manifest version');
    if (manifest.method === undefined) {
      // Stock signed-release shape: the pinned archive and its extracted binary.
      const release = process.platform === 'darwin' ? 'darwin_arm64' : 'linux_amd64';
      if (JSON.stringify(Object.keys(manifest).sort()) !== '["asset","assetSha256","binarySha256","version"]' ||
          manifest.asset !== `gentle-ai_${version}_${release}.tar.gz` ||
          typeof manifest.assetSha256 !== 'string' || !/^[0-9a-f]{64}$/.test(manifest.assetSha256)) reject('native manifest shape');
    } else {
      // Stock Windows Go SumDB source-build shape; never published on POSIX.
      if (manifest.method !== 'go-sumdb-source-build' ||
          JSON.stringify(Object.keys(manifest).sort()) !==
            '["architecture","binarySha256","buildMode","cgoEnabled","compiler","goVersion","goarch","goos","method","module","moduleChecksum","package","tag","version"]') reject('native manifest shape');
    }
    if (digest(binary) !== manifest.binarySha256) reject('native independent readback');
    return version;
  }
  function materializeGlobal() {
    const parent = path.dirname(modules);
    if (!fs.existsSync(parent)) fs.mkdirSync(parent, { mode: 0o700 });
    physical(parent, true);
    // Siblings keep promotions on the prefix filesystem, even for shared installs.
    const staged = path.join(parent, `.gentle-shell-staged-${crypto.randomUUID()}`);
    const previous = path.join(parent, `.gentle-shell-previous-${crypto.randomUUID()}`);
    absent(staged);
    absent(previous);
    const acquired = path.join(source, 'node_modules');
    const acquiredSHA = inventory(acquired);
    copyTree(acquired, staged);
    if (inventory(acquired) !== acquiredSHA || inventory(staged) !== acquiredSHA) reject('materialized source differs');
    // CI's hidden lock includes optional placements normalized out of this tree.
    // Keep the original as acquisition evidence; do not publish its stale projection.
    const hidden = path.join(staged, '.package-lock.json');
    if (fs.existsSync(hidden)) fs.unlinkSync(hidden);
    const native = path.join(modules, 'gentle-pi/.gentle-ai');
    if (fs.existsSync(native)) {
      const copied = path.join(staged, 'gentle-pi/.gentle-ai');
      absent(copied);
      const nativeSHA = inventory(native);
      copyTree(native, copied);
      if (inventory(native) !== nativeSHA || inventory(copied) !== nativeSHA) reject('native copy differs');
    }
    syncTree(staged);
    sync(parent);
    // Never delete the old tree or uncertain new evidence on a failed promotion.
    // Existing confirmed whole-prefix recovery owns restoration, not this helper.
    if (fs.existsSync(modules)) {
      fs.renameSync(modules, previous);
      sync(parent);
    }
    fs.renameSync(staged, modules);
    sync(parent);
    stockNpm(['rebuild', '--global', '--prefix', prefix, '--offline', '--ignore-scripts', '--bin-links=true', '--engine-strict', '--no-audit', '--no-fund'], source, 180000);
  }
  if (action === 'install') {
    nativeState();
    if (mode === 'shared' && !preparing) packages(modules);
    observed.length = 0;
    materializeGlobal();
  }
  packages(modules);
  const semver = createRequire(npm)('semver'); // Authenticated stock Node/npm closure.
  function resolve(directory, name) {
    for (let parent = directory; parent.startsWith(modules); parent = path.dirname(parent)) {
      const candidate = path.join(parent, 'node_modules', name);
      if (observed.some(record => path.join(prefix, record.directory) === candidate)) return candidate;
    }
    const candidate = path.join(modules, name);
    if (observed.some(record => path.join(prefix, record.directory) === candidate)) return candidate;
  }
  for (const item of observed) {
    const directory = path.join(prefix, item.directory), metadata = JSON.parse(read(path.join(directory, 'package.json')));
    for (const [declarations, peer] of [[metadata.dependencies ?? {}, false], [metadata.peerDependencies ?? {}, true]]) {
      for (const [name, range] of Object.entries(declarations)) {
        if (!packageName.test(name) || typeof range !== 'string') reject('dependency declaration');
        const target = resolve(directory, name);
        const optional = peer ? metadata.peerDependenciesMeta?.[name]?.optional : metadata.optionalDependencies?.[name];
        if (!target && optional) continue;
        if (!target || !semver.satisfies(JSON.parse(read(path.join(target, 'package.json'))).version, range)) reject('global required/peer range mismatch');
      }
    }
    for (const bins of [path.join(directory, 'node_modules/.bin'), ...(directory === path.join(modules, 'gentle-pi') ? [path.join(modules, '.bin')] : [])]) {
      if (!fs.existsSync(bins)) continue;
      physical(bins, true);
      for (const name of fs.readdirSync(bins)) {
        const link = path.join(bins, name), target = fs.realpathSync(link);
        if (!fs.lstatSync(link).isSymbolicLink() || !target.startsWith(modules + '/')) reject('foreign nested bin');
        read(target);
      }
    }
  }
  for (const [name, [version]] of Object.entries(pins)) {
    const metadata = JSON.parse(read(path.join(modules, name, 'package.json')));
    if (metadata.name !== name || metadata.version !== version) reject('global root mutation');
    const bins = typeof metadata.bin === 'string' ? { [name.split('/').at(-1)]: metadata.bin } : metadata.bin ?? {};
    for (const [command, relative] of Object.entries(bins)) {
      if (!/^[A-Za-z0-9._-]+$/.test(command) || typeof relative !== 'string') reject('root bin declaration');
      const link = path.join(prefix, 'bin', command), expected = path.resolve(modules, name, relative);
      if (!expected.startsWith(path.join(modules, name) + '/') || !fs.lstatSync(link).isSymbolicLink() || fs.realpathSync(link) !== expected) reject('root bin binding');
      read(expected);
    }
  }
  const hidden = path.join(modules, '.package-lock.json');
  if (fs.existsSync(hidden)) {
    const installed = JSON.parse(read(hidden));
    if (installed.lockfileVersion !== 3 || !installed.packages) reject('global hidden lock');
    for (const [relative, record] of Object.entries(installed.packages)) {
      if (!observed.some(item => item.directory === path.posix.join('lib', relative) && item.version === record.version && item.integrity === record.integrity)) reject('global hidden lock differs');
    }
  }
  const supplier = path.join(modules, 'gentle-pi');
  // Reviewed supplier sources (S19): with the staged reviewed list — every
  // Stable install this feature provisions stages one — the installed
  // gentle-pi's supplier files must match its locked version's list entry
  // before the supplier runs. A stage without a staged list (a Main preview
  // install, or a legacy tree) keeps the frozen-lock route, whose supplier
  // authentication the Go launch boundary enforces against the embedded list
  // entry (S23/M1).
  if (reviewed) {
    const entry = reviewed[pins['gentle-pi'][0]];
    if (!entry) reject('stock supplier pin');
    for (const [relative, expected] of [
      ['scripts/gentle-ai-installer.mjs', entry.installer],
      ['runtime/gentle-ai-binary.mjs', entry.binary],
    ]) {
      if (digest(read(path.join(supplier, relative))) !== expected) reject('stock supplier pin');
    }
  }
  nativeState();
  if (action === 'install' && !preparing) {
    let fresh;
    try { fresh = read(settingsPath); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    if (Boolean(fresh) !== Boolean(priorSettings) || (fresh && !fresh.equals(priorSettings))) reject('settings changed');
    const declarations = [packageRoot];
    if (mode === 'separate') {
      // Engram (S1) for Separate only: the generated route read the pin above.
      // The frozen locks carry no companion, so the frozen route reads the
      // authenticated stock pin only to report why memory stays off.
      if (frozenLock) {
        try { companion = companionPin(read(path.join(supplier, 'runtime/gentle-shell-bundled.mjs')).toString()); }
        catch (error) { if (error.code !== 'ENOENT') throw error; }
      }
      const installed = observed.find(record => record.directory === path.join('lib/node_modules', companionName))?.version;
      const declaration = await engramMemory(companion, pins[companionName]?.[0], installed, supplier, path.join(agent, 'bin'), 'engram', process.platform, process.arch);
      if (declaration && !(settings.packages ?? []).includes(declaration)) declarations.push(declaration);
    }
    settings.packages = [...(settings.packages ?? []), ...declarations];
    settings.npmCommand = [path.join(finalRoot, 'runtime/node/bin/node'), path.join(finalRoot, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'), '--prefix', finalPrefix];
    const stage = path.join(agent, `.gentle-shell-settings-${process.pid}`);
    writeExclusive(stage, `${JSON.stringify(settings, null, 2)}\n`);
    fs.renameSync(stage, settingsPath);
    sync(agent); // Complete settings publication before persisting its graph witness.
    writeExclusive(path.join(state, 'global-graph.json'), `${JSON.stringify(observed)}\n`);
  } else {
    const selectedPackageRoot = verifyingSelectedSDK ? mainPackageRoot : packageRoot;
    const declaration = settings.packages?.some(entry => (typeof entry === 'string' ? entry : entry?.source) === selectedPackageRoot);
    const command = [path.join(finalRoot, 'runtime/node/bin/node'), path.join(finalRoot, 'runtime/node/lib/node_modules/npm/bin/npm-cli.js'), '--prefix', finalPrefix];
    if (!declaration || JSON.stringify(settings.npmCommand) !== JSON.stringify(command)) reject('selected settings bindings changed');
    if (preparing) {
      if (!read(settingsPath).equals(priorSettings)) reject('fixture changed selected settings');
      writeExclusive(graphPath, `${JSON.stringify(observed)}\n`);
    }
    else {
      // Stock updates may restore foreign optionals retained during acquisition.
      // Authenticate every physical package above before projecting comparison;
      // never normalize the live prefix or rewrite the original graph evidence.
      const { projectGlobalGraph } = await import(pathToFileURL(path.join(root, 'user-global-graph.mjs')).href);
      const comparable = projectGlobalGraph(observed, lock, retained.paths);
      if (!read(graphPath).equals(Buffer.from(`${JSON.stringify(comparable)}\n`))) reject('global graph changed; inspect update before relaunch');
    }
  }
  if (!read(lockPath).equals(lockBytes)) reject('acquired lock changed');
  if (mainActions.includes(action)) {
    // All original stock checks above remain mandatory, including exact stock
    // supplier pins. Composition is unpublished DATA, not installation/activation.
    const { assertMainBindings, assertMainSDKPreviewBindings, mainBindingFiles } = await import(pathToFileURL(path.join(root, 'user-main-bindings.mjs')).href);
    const acquired = authority.get(`gentle-pi@${pins['gentle-pi'][0]}`);
    if (!acquired) reject('Main preflight lacks authenticated stock');
    const bindings = directory => Object.fromEntries(mainBindingFiles.map(name => [name, read(path.join(directory, name))]));
    const main = bindings(path.join(root, 'runtime/main-source')), stock = bindings(acquired.source);
    if (action !== 'verify-main-source') {
      const preserved = assertMainSDKPreviewBindings(main, stock);
      if (action !== 'verify-main-sdk-source') {
        // The Go caller binds main-source to the retained archive on both sides.
        // No Main module is imported; only bounded source bytes are copied.
        const mainRoot = path.join(root, 'runtime/main-source');
        const composed = path.join(root, 'runtime/main-sdk');
        const writing = action === 'materialize-main-sdk-source';
        const runtime = mainRuntimeActions.includes(action);
        const writingRuntime = action === 'materialize-main-sdk-runtime';
        if (runtime && (!mainPlatform || prefix !== path.join(root, 'prefix'))) reject('Main SDK runtime requires Linux amd64 or macOS arm64 private Separate');
        physical(path.dirname(composed), true);
        if (writing) absent(composed);
        const expected = new Map();
        let bytes = 0;
        function sourceMembers(directory, relative = '') {
          const before = physical(directory, true);
          if ((before.mode & 0o7777) !== 0o700) reject('Main source directory custody');
          if (expected.size >= 32768) reject('Main composition member bound');
          expected.set(relative, null);
          for (const name of fs.readdirSync(directory).sort()) {
            const original = path.join(directory, name), key = path.posix.join(relative, name);
            if (fs.lstatSync(original).isDirectory()) sourceMembers(original, key);
            else {
              const info = physical(original);
              if (info.nlink !== 1 || (info.mode & 0o7777) !== 0o600) reject('Main source file custody');
              const originalBytes = read(original);
              const content = preserved.includes(key) ? stock[key] : originalBytes;
              bytes += originalBytes.length + (preserved.includes(key) ? content.length : 0);
              if (expected.size >= 32768 || bytes > 1073741824) reject('Main composition byte/member bound');
              expected.set(key, content);
            }
          }
          const after = physical(directory, true);
          if (before.dev !== after.dev || before.ino !== after.ino || before.mtimeMs !== after.mtimeMs || before.ctimeMs !== after.ctimeMs) reject('Main source directory changed');
        }
        sourceMembers(mainRoot);
        for (const name of preserved) if (!expected.has(name) || expected.get(name) === null) reject('Main binding member absent');
        if (writing) {
          for (const [name, content] of expected) {
            const target = path.join(composed, name);
            if (content === null) {
              fs.mkdirSync(target, { mode: 0o700 });
              sync(target);
              sync(path.dirname(target));
            } else writeExclusive(target, content);
          }
        }
        const remaining = new Map(expected);
        let nativeBinary = null;
        function compare(directory, relative = '') {
          const before = physical(directory, true);
          if ((before.mode & 0o7777) !== 0o700 || !remaining.has(relative) || remaining.get(relative) !== null) reject('Main composition directory differs');
          remaining.delete(relative);
          for (const name of fs.readdirSync(directory).sort()) {
            const target = path.join(directory, name), key = path.posix.join(relative, name);
            if (!remaining.has(key)) reject('Main composition extra member');
            if (fs.lstatSync(target).isDirectory()) compare(target, key);
            else {
              const info = physical(target), content = remaining.get(key);
              const permissions = runtime && key === nativeBinary ? 0o700 : 0o600;
              if (info.nlink !== 1 || (info.mode & 0o7777) !== permissions || content === null || !read(target).equals(content)) reject('Main composition file differs');
              remaining.delete(key);
            }
          }
          const after = physical(directory, true);
          if (before.dev !== after.dev || before.ino !== after.ino || before.mtimeMs !== after.mtimeMs || before.ctimeMs !== after.ctimeMs) reject('Main composition directory changed');
        }
        if (runtime) {
          // Source composition is already materialized. Admit only the pinned
          // stock native subtree and this one rename-safe dependency link.
          if (expected.has('.gentle-ai')) reject('Main source collides with stock native');
          const nativeSource = path.join(supplier, '.gentle-ai');
          const nativeTarget = path.join(composed, '.gentle-ai');
          const dependency = path.join(root, 'runtime/node_modules');
          const linkText = '../prefix/lib/node_modules';
          // Linux symlinks are always 0777; macOS records the creating umask,
          // pinned to 022 above. Neither grants access through the link mode.
          const linkMode = process.platform === 'darwin' ? 0o755 : 0o777;
          const dependencyTarget = modules; // Authenticated stock is hoisted here.
          physical(dependencyTarget, true);
          if (writingRuntime) {
            absent(nativeTarget);
            absent(dependency);
            compare(composed);
            if (remaining.size !== 0) reject('Main composition missing member');
          }
          const nativeVersion = nativeState(nativeSource, true); // Existing independent pins, mandatory.
          nativeBinary = `.gentle-ai/v${nativeVersion}/gentle-ai`;
          const nativeMembers = new Map([
            ['.gentle-ai', null], [`.gentle-ai/v${nativeVersion}`, null],
            [nativeBinary, read(path.join(nativeSource, `v${nativeVersion}/gentle-ai`))],
            [`.gentle-ai/v${nativeVersion}/integrity.json`, read(path.join(nativeSource, `v${nativeVersion}/integrity.json`))],
          ]);
          const nativeBytes = [...nativeMembers.values()].reduce((sum, content) => sum + (content?.length ?? 0), 0);
          if (expected.size + nativeMembers.size > 32768 || bytes + nativeBytes > 1073741824) reject('Main runtime byte/member bound');
          for (const [name, content] of nativeMembers) {
            if (expected.has(name)) reject('Main native member collision');
            expected.set(name, content);
            if (content !== null && physical(path.join(supplier, name)).nlink !== 1) reject('stock native link custody');
          }
          if (writingRuntime) {
            for (const [name, content] of nativeMembers) {
              const target = path.join(composed, name);
              if (content === null) {
                fs.mkdirSync(target, { mode: 0o700 });
                sync(target);
                sync(path.dirname(target));
              } else writeExclusive(target, content, name === nativeBinary ? 0o700 : 0o600);
            }
            fs.symlinkSync(linkText, dependency);
            sync(path.dirname(dependency));
          }
          const link = fs.lstatSync(dependency);
          if (!link.isSymbolicLink() || link.uid !== uid || link.nlink !== 1 || (link.mode & 0o7777) !== linkMode || fs.readlinkSync(dependency) !== linkText || fs.realpathSync(dependency) !== dependencyTarget) reject('Main SDK dependency binding differs');
          nativeState(nativeSource, true);
          for (const [name, content] of nativeMembers) {
            if (content !== null && !read(path.join(supplier, name)).equals(content)) reject('stock native changed during placement');
          }
          remaining.clear();
          for (const [name, content] of expected) remaining.set(name, content);
        }
        compare(composed);
        if (remaining.size !== 0) reject('Main composition missing member');
        console.log(`Main SDK composition exact readback; materialized=${writing || writingRuntime}; activated=false`);
        if (selectingSDK) {
          // Only after every stock/source/native/dependency check. Preserve all
          // other settings and declarations; never mutate the stock prefix.
          const freshAgent = physical(agent, true);
          if (freshAgent.dev !== selectedAgentIdentity.dev || freshAgent.ino !== selectedAgentIdentity.ino || (freshAgent.mode & 0o7777) !== 0o700) reject('Main SDK agent changed before selection');
          if (!read(settingsPath).equals(priorSettings)) reject('Main SDK settings changed before selection');
          settings.packages = settings.packages.map(entry => entry === packageRoot ? mainPackageRoot : entry);
          const selectedBytes = Buffer.from(`${JSON.stringify(settings, null, 2)}\n`);
          const stage = path.join(agent, `.gentle-shell-settings-${process.pid}`);
          writeExclusive(stage, selectedBytes);
          fs.renameSync(stage, settingsPath);
          sync(agent);
          const afterAgent = physical(agent, true);
          if (afterAgent.dev !== selectedAgentIdentity.dev || afterAgent.ino !== selectedAgentIdentity.ino || (afterAgent.mode & 0o7777) !== 0o700) reject('Main SDK selected agent changed');
          const info = physical(settingsPath);
          if (info.nlink !== 1 || (info.mode & 0o7777) !== 0o600 || !read(settingsPath).equals(selectedBytes)) reject('Main SDK selected settings readback');
          console.log('SDK Main + stock provider private package selected; personal_activated=false');
        } else if (verifyingSelectedSDK) {
          const afterAgent = physical(agent, true), info = physical(settingsPath);
          if (afterAgent.dev !== selectedAgentIdentity.dev || afterAgent.ino !== selectedAgentIdentity.ino || (afterAgent.mode & 0o7777) !== 0o700 || info.nlink !== 1 || (info.mode & 0o7777) !== 0o600 || !read(settingsPath).equals(priorSettings)) reject('Main SDK selected settings changed during verification');
        }
      }
      console.log(`Main SDK composition bindings verified; preserve authenticated stock=${preserved.join(',')}; activated=false`);
    } else {
      assertMainBindings(main, stock);
      console.log('Main declared stock bindings verified; source API/runtime unqualified; activated=false');
    }
  }
  console.log(`Global readback verified; authenticated packages=${observed.length}; functional Ready=false`);
}
