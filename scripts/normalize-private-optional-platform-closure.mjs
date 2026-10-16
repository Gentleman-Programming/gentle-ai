import fs from 'node:fs';
import path from 'node:path';

const packagePath = /^(?:node_modules\/(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*)(?:\/node_modules\/(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*)*$/;
const rootPins = new Set(['gentle-pi', '@earendil-works/pi-coding-agent', '@earendil-works/pi-tui', '@heyhuynhgiabuu/pi-pretty', 'typebox'].map(name => `node_modules/${name}`));
function reject(message) { throw Error(message); }
function matches(rule, value) {
  if (rule === undefined) return true;
  if (!Array.isArray(rule) || rule.some(x => typeof x !== 'string' || !/^!?[a-z0-9_]+$/.test(x))) reject('invalid platform rule');
  return !rule.includes(`!${value}`) && !rule.includes('!any') &&
    (!rule.some(x => !x.startsWith('!')) || rule.includes(value) || rule.includes('any'));
}
function sameStat(a, b) {
  return ['dev', 'ino', 'mode', 'size', 'mtimeNs', 'ctimeNs'].every(key => a[key] === b[key]);
}

// Caller owns a new exclusive private staging project under its rollback trap.
// lock and expectedPaths are authenticated and validated before offline npm ci.
// actualPaths is the COMPLETE physical inventory from the installer's walk:
// every package, scope and node_modules directory was lstat-checked, never followed.
// This function does not execute package code or broaden the expected closure.
// Same-UID hostile concurrent mutation hardening remains outside this contract.
export function normalize({ root, lock, expectedPaths, actualPaths, platform }, ops = {}) {
  if (!path.isAbsolute(root) || path.resolve(root) !== root) reject('noncanonical root path');
  function physicalDirectory(absolute) {
    if (!fs.lstatSync(absolute).isDirectory()) reject(`nonphysical directory: ${absolute}`);
  }
  function parents(absolute) {
    const parts = absolute.split(path.sep).filter(Boolean);
    let current = path.parse(absolute).root;
    physicalDirectory(current);
    for (const part of parts) {
      current = path.join(current, part);
      physicalDirectory(current);
    }
  }
  parents(root);
  const expected = new Set(expectedPaths);
  const actual = new Set(actualPaths);
  if (expected.size !== expectedPaths.length || actual.size !== actualPaths.length) reject('duplicate inventory path');
  for (const key of [...expected, ...actual]) {
    if (typeof key !== 'string' || !packagePath.test(key)) reject(`unsafe package path: ${key}`);
  }
  for (const key of expected) {
    if (!actual.has(key)) reject(`missing expected package: ${key}`);
  }
  const extras = actualPaths.filter(key => !expected.has(key));
  const plan = [];
  for (const key of extras) {
    const record = lock.packages[key];
    if (!record || rootPins.has(key)) reject(`unknown or root pin extra: ${key}`);
    if (record.optional !== true) reject(`nonoptional extra: ${key}`);
    const applicable = [matches(record.os, platform.os), matches(record.cpu, platform.cpu), matches(record.libc, platform.libc)].every(Boolean);
    if (applicable) reject(`applicable extra: ${key}`);
    if ([...expected].some(item => item.startsWith(`${key}/`))) reject(`expected subtree overlap: ${key}`);
    const absolute = path.join(root, key);
    parents(absolute);
    const metadata = path.join(absolute, 'package.json');
    const stat = fs.lstatSync(metadata, { bigint: true });
    if (!stat.isFile()) reject(`nonregular metadata: ${key}`);
    const bytes = fs.readFileSync(metadata);
    const identity = JSON.parse(bytes);
    if (identity.name !== (record.name ?? key.split('node_modules/').at(-1)) || identity.version !== record.version) reject(`identity differs: ${key}`);
    plan.push({ key, absolute, metadata, stat, bytes, directory: fs.lstatSync(absolute, { bigint: true }) });
  }
  // Every extra, including children of removable parents, is validated first.
  // Fresh preimages for the COMPLETE plan are checked before the first effect.
  ops.beforeApply?.();
  for (const item of plan) {
    parents(item.absolute);
    const stat = fs.lstatSync(item.metadata, { bigint: true });
    if (!sameStat(item.directory, fs.lstatSync(item.absolute, { bigint: true })) ||
        !stat.isFile() || !sameStat(item.stat, stat) || !fs.readFileSync(item.metadata).equals(item.bytes)) reject(`preimage differs: ${item.key}`);
  }
  const topLevel = plan.filter(item => !extras.some(parent => item.key.startsWith(`${parent}/`)));
  for (const item of topLevel) {
    // Node recursive rm unlinks inner symlinks; it does not traverse their targets.
    (ops.remove ?? (absolute => fs.rmSync(absolute, { recursive: true, force: false })))(item.absolute);
  }
  return topLevel.map(item => item.key);
}
