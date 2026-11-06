// Portable VM fixtures for the Main overlay composition region only: they prove
// that composition expectations are read from the frozen publisher snapshot's
// own metadata (#5902), not from values embedded in this installer. They do not
// prove an installed Windows runtime; the Guest e2e acceptance does that.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const provision = process.argv[2] ?? fileURLToPath(new URL('./provision-gentle-shell-windows.mjs', import.meta.url));
const source = fs.readFileSync(provision, 'utf8');
const begin = 'function satisfiesVersionRange(version, range) {';
const end = '// Fail fast: the frozen snapshot must run on the node runtime this installer';
const start = source.indexOf(begin), finish = source.indexOf(end);
assert.ok(start >= 0 && finish > start, 'overlay composition region boundaries must exist');
assert.equal(source.indexOf(begin, start + begin.length), -1, 'region start must be unique');
assert.equal(source.indexOf(end, finish + end.length), -1, 'region end must be unique');
const [satisfiesVersionRange, stagedPeerVersion, mainOverlayComposition] = vm.runInNewContext(
  `${source.slice(start, finish)}; [satisfiesVersionRange, stagedPeerVersion, mainOverlayComposition]`,
  { JSON, Object, Number, String, RegExp, Error },
);
assert.equal(typeof satisfiesVersionRange, 'function');
assert.equal(typeof stagedPeerVersion, 'function');
assert.equal(typeof mainOverlayComposition, 'function');
// The published installer refused every main-channel install whose snapshot
// declared @earendil-works/pi-coding-agent >=1.2.0. The embedded table must
// stay gone, and the branch must consult snapshot-derived composition only.
assert.ok(!source.includes("'>=0.99.1'"), 'embedded Main peer table must be removed');
assert.ok(source.includes('mainOverlayComposition(record, JSON.parse(acquired.bytes)'), 'overlay branch must use snapshot-derived composition');
assert.ok(source.includes('Main snapshot requires node'), 'engines preflight gate must be present');

const failures = [];
const check = (name, observed, want) => { if (!Object.is(observed, want)) failures.push(`${name}: expected ${want}, observed ${observed}`); };

// Range satisfaction: `*` and `>=x.y.z` only; everything else is undefined so
// the caller refuses explicitly instead of guessing.
const ranges = [
  ['1.2.0', '*', true],
  ['1.2.0', '>=0.99.1', true],
  ['0.99.0', '>=0.99.1', false],
  ['1.2.0', '>=1.2.0', true],
  ['1.2.1', '>=1.2.0', true],
  ['1.1.9', '>=1.2.0', false],
  ['2.0.0-rc.1', '>=1.0.0', true],
  ['1.2.0+build.7', '>=1.2.0', true],
  ['10.0.0', '>=9.10.11', true],
  ['1.2', '>=1.0.0', undefined],
  ['1.2.0', '^1.0.0', undefined],
  ['1.2.0', '~1.2', undefined],
  ['1.2.0', '>=1.x', undefined],
  ['1.2.0', '', undefined],
  ['v1.2.0', '>=1.0.0', undefined],
  ['latest', '>=1.0.0', undefined],
];
for (const [version, range, want] of ranges) check(`range ${version} ${range}`, satisfiesVersionRange(version, range), want);

// Staged-graph peer lookup: top-level hoisted packages only.
const modules = 'R:\\Stage\\prefix\\node_modules';
const tree = new Map([
  [path.win32.join(modules, 'typebox', 'package.json'), { version: '1.3.27' }],
  [path.win32.join(modules, '@earendil-works', 'pi-coding-agent', 'package.json'), { version: '1.2.0' }],
]);
const fakeFs = { existsSync: file => tree.has(file) };
const fakeRead = file => {
  assert.ok(tree.has(file), 'only staged package reads permitted');
  return Buffer.from(JSON.stringify(tree.get(file)));
};
check('staged unscoped', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, 'typebox'), '1.3.27');
check('staged scoped', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, '@earendil-works/pi-coding-agent'), '1.2.0');
check('staged absent', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, '@earendil-works/pi-ai'), null);
check('staged non-scope pair', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, 'a/b'), undefined);
check('staged too deep', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, 'a/b/c'), undefined);
check('staged bare scope', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, '@scope'), undefined);
check('staged empty segment', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, 'a//b'), undefined);
check('staged traversal', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, '..'), undefined);
check('staged dot', stagedPeerVersion(fakeFs, path.win32, fakeRead, modules, '.'), undefined);

// Composition: the frozen snapshot's own metadata is the expectation.
const frozen = {
  name: 'gentle-pi', version: '4.5.1', engines: { node: '>=22.19.0' },
  dependencies: { '@heyhuynhgiabuu/pi-pretty': '0.6.27' },
  peerDependencies: { '@earendil-works/pi-coding-agent': '>=1.2.0', '@earendil-works/pi-ai': '*', '@earendil-works/pi-tui': '*', typebox: '*' },
};
const stagedRecord = (extra = {}, peers = {}) => ({
  name: 'gentle-pi', version: '4.5.1', engines: { node: '>=22.19.0' },
  dependencies: { '@heyhuynhgiabuu/pi-pretty': '0.6.27' },
  peerDependencies: { ...frozen.peerDependencies, ...peers }, ...extra,
});
const stagedPeers = { '@earendil-works/pi-coding-agent': '1.2.0', '@earendil-works/pi-ai': '1.9.0', '@earendil-works/pi-tui': '1.0.0', typebox: '1.3.27' };
const compose = (record, snapshot = frozen, lockVersion = '4.5.1', peers = stagedPeers, metadata = snapshot) => {
  const outcome = composeMessage(record, metadata, lockVersion, peers);
  return outcome === null ? 'accept' : outcome.startsWith('infrastructure: ') ? outcome : 'reject';
};
const composeMessage = (record, snapshot, lockVersion = '4.5.1', peers = stagedPeers) => {
  try {
    mainOverlayComposition(record, snapshot, lockVersion, name => Object.hasOwn(peers, name) ? peers[name] : null,
      message => { throw Error(`Windows provision refused: ${message}`); });
    return null;
  } catch (error) {
    return error.message.startsWith('Windows provision refused: ') ? error.message.slice('Windows provision refused: '.length) : `infrastructure: ${error.message}`;
  }
};
check('compose baseline', compose(stagedRecord()), 'accept');
// The exact #5902 regression: snapshot raised pi-coding-agent to >=1.2.0 while
// the embedded table still expected >=0.99.1; a satisfied snapshot installs.
check('compose regression 5902', compose(stagedRecord()), 'accept');
assert.equal(composeMessage(stagedRecord(), frozen), null);
check('compose peer unsatisfied', compose(stagedRecord(), frozen, '4.5.1', { ...stagedPeers, '@earendil-works/pi-coding-agent': '1.0.0' }), 'reject');
assert.match(composeMessage(stagedRecord(), frozen, '4.5.1', { ...stagedPeers, '@earendil-works/pi-coding-agent': '1.0.0' }),
  /@earendil-works\/pi-coding-agent@>=1\.2\.0 is unsatisfied by the staged @earendil-works\/pi-coding-agent@1\.0\.0/);
check('compose peer absent', compose(stagedRecord(), frozen, '4.5.1', { typebox: '1.3.27' }), 'reject');
assert.match(composeMessage(stagedRecord(), frozen, '4.5.1', { typebox: '1.3.27' }), /@earendil-works\/pi-coding-agent@>=1\.2\.0 is absent from the staged graph/);
check('compose optional peer absent', compose(stagedRecord(), frozen, '4.5.1', stagedPeers,
  { ...frozen, peerDependencies: { ...frozen.peerDependencies, 'extra-peer': '*' }, peerDependenciesMeta: { 'extra-peer': { optional: true } } }), 'accept');
check('compose unsupported range', compose(stagedRecord(), frozen, '4.5.1', stagedPeers,
  { ...frozen, peerDependencies: { ...frozen.peerDependencies, 'extra-peer': '^1.0.0' } }), 'reject');
check('compose metadata drift', compose(stagedRecord({ version: '4.5.0' }), frozen), 'reject');
check('compose engines drift', compose(stagedRecord({ engines: { node: '>=20.0.0' } }), frozen), 'reject');
check('compose dependencies drift', compose(stagedRecord({ dependencies: {} }), frozen), 'reject');
check('compose name drift', compose(stagedRecord({ name: 'foreign-pi' }), frozen), 'reject');
check('compose lock drift', compose(stagedRecord(), frozen, '4.4.0'), 'reject');
assert.match(composeMessage(stagedRecord(), frozen, '4.4.0'), /is not the locked reviewed gentle-pi 4\.4\.0/);
check('compose missing record', compose(null, frozen), 'reject');

const total = ranges.length + 9 + 12;
console.log(`windows-main-overlay-composition: ${total - failures.length}/${total} passed`);
for (const failure of failures) console.log(`FAIL ${failure}`);
process.exitCode = failures.length === 0 ? 0 : 1;
