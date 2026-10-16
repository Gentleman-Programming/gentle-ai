// Guest-only fixtures for the actual provisioner's settings region, not SDK/source-graph qualification.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';

assert.equal(process.platform, 'win32', 'fixture execution requires the qualified Windows Guest');
assert.equal(process.argv.length, 4, 'supply installed provision.mjs and a fresh private report path');
const source = fs.readFileSync(process.argv[2], 'utf8');
const begin = "const settingsPath = path.join(agent, 'settings.json');";
const end = "if (!read(lockPath).equals(lockBytes)) reject('source lock changed');";
const start = source.indexOf(begin), finish = source.indexOf(end);
assert.ok(start >= 0 && finish > start, 'settings-region boundaries must exist');
assert.equal(source.indexOf(begin, start + begin.length), -1, 'settings-region start must be unique');
assert.equal(source.indexOf(end, finish + end.length), -1, 'settings-region end must be unique');
const body = source.slice(start, finish);
const root = 'R:\\Owned Settings Fixture', agent = path.join(root, 'agent');
const expected = {
  packages: [path.join(root, 'prefix/node_modules/gentle-pi')],
  npmCommand: [path.join(root, 'runtime/node/node.exe'), path.join(root, 'runtime/node/node_modules/npm/bin/npm-cli.js'), '--prefix', path.join(root, 'prefix')],
};
const refusal = 'Windows provision refused: owned package/settings bindings changed';
const copy = () => JSON.parse(JSON.stringify(expected));
const cases = [
  { name: 'baseline', value: copy(), want: 'accept' },
  { name: 'sdk-changelog-version', value: { ...copy(), lastChangelogVersion: '1.0.0' }, want: 'accept' },
  { name: 'owned-codemode-exclusion', value: { ...copy(), extensions: ['-builtin:codemode'] }, want: 'accept' },
  { name: 'foreign-builtin-exclusion', value: { ...copy(), extensions: ['-builtin:foreign'] }, want: 'reject' },
  { name: 'mixed-extension-injection', value: { ...copy(), extensions: ['-builtin:codemode', 'R:\\Foreign'] }, want: 'reject' },
  { name: 'theme-path-injection', value: { ...copy(), theme: 'R:\\Foreign\\theme' }, want: 'reject' },
  { name: 'changed-packages', value: { ...copy(), packages: ['R:\\Foreign Package'] }, want: 'reject' },
  { name: 'changed-npm-command', value: { ...copy(), npmCommand: ['R:\\Foreign Executable'] }, want: 'reject' },
  { name: 'missing-packages', value: { npmCommand: expected.npmCommand }, want: 'reject' },
  { name: 'missing-npm-command', value: { packages: expected.packages }, want: 'reject' },
  { name: 'null-settings', value: null, want: 'reject' },
  { name: 'array-settings', value: [], want: 'reject' },
  { name: 'install-readback', value: null, action: 'install', want: 'accept' },
];
for (const key of ['extensions', 'skills', 'customProviders', 'constructor', '__proto__', 'toString']) {
  cases.push({ name: `injected-${key}`, value: { ...copy(), [key]: ['R:\\Foreign Resource'] }, want: 'reject' });
}
for (const [name, value] of [['null', null], ['number', 1], ['array', []], ['object', {}]]) {
  cases.push({ name: `changelog-type-${name}`, value: { ...copy(), lastChangelogVersion: value }, want: 'reject' });
}
const outcomes = [];
for (const fixture of cases) {
  let actual = fixture.value, observed = 'accept', infrastructure = false;
  try {
    vm.runInNewContext(body, {
      root, finalRoot: root, agent, path, action: fixture.action ?? 'verify',
      read(file) {
        if (file === path.join(root, 'selection.json')) return Buffer.from(JSON.stringify({ Destination: root }));
        assert.equal(file, path.join(agent, 'settings.json'), 'only virtual settings and selection reads permitted');
        return Buffer.from(JSON.stringify(actual));
      },
      exclusive(file, bytes) {
        assert.equal(fixture.action, 'install', 'verify must not write settings');
        assert.equal(file, path.join(agent, 'settings.json'));
        actual = JSON.parse(bytes);
      },
      reject(message) { throw Error(`Windows provision refused: ${message}`); },
    }, { timeout: 1000 });
  } catch (error) {
    observed = error.message === refusal ? 'reject' : 'infrastructure';
    infrastructure = observed === 'infrastructure';
  }
  let assertionCode = null;
  try { assert.equal(observed, fixture.want, fixture.name); } catch (error) { assertionCode = error.code; }
  outcomes.push({ name: fixture.name, expected: fixture.want, observed, infrastructure, assertionCode });
}
const configStart = source.indexOf("const configPath = path.join(root, 'config/gentle-shell.json');");
assert.ok(configStart >= 0 && configStart < start, 'owned Shell config region must exist');
const configBody = source.slice(configStart, start), homeKey = path.join(root, 'agent');
const goodConfig = { home: 'isolated', provisioned: { [homeKey]: { gentleAi: '4.0.0', gentlePi: '4.0.0', at: '2026-01-01T00:00:00.000Z' } } };
for (const [name, value, want] of [
  ['owned-shell-config', goodConfig, 'accept'],
  ['shell-link-refused', { ...goodConfig, home: 'link' }, 'reject'],
  ['shell-pin-refused', { ...goodConfig, provisioned: { [homeKey]: { ...goodConfig.provisioned[homeKey], gentleAi: 'beta' } } }, 'reject'],
  ['shell-foreign-home-refused', { ...goodConfig, provisioned: { foreign: goodConfig.provisioned[homeKey] } }, 'reject'],
  ['shell-extra-key-refused', { ...goodConfig, injected: true }, 'reject'],
]) {
  let observed = 'accept';
  try {
    vm.runInNewContext(configBody, {
      root, finalRoot: root, path, action: 'verify', gentleMetadata: { version: '4.0.0' },
      read(file) { assert.equal(file, path.join(root, 'config/gentle-shell.json')); return Buffer.from(JSON.stringify(value)); },
      exclusive() { assert.fail('verify attempted config write'); },
      reject(message) { throw Error(`Windows provision refused: ${message}`); },
    }, { timeout: 1000 });
  } catch (error) {
    observed = error.message === 'Windows provision refused: owned Shell provisioning binding changed' ? 'reject' : 'infrastructure';
  }
  let assertionCode = null;
  try { assert.equal(observed, want, name); } catch (error) { assertionCode = error.code; }
  outcomes.push({ name, expected: want, observed, infrastructure: observed === 'infrastructure', assertionCode });
}
const supplierStart = source.indexOf('// Stock supplier pins authorize installation; Main source is bound by the frozen ZIP.');
assert.ok(supplierStart >= 0 && supplierStart < configStart, 'native authority region must exist');
const supplierBody = `${source.match(/^const mainOverlay = .*$/m)[0]}\n${source.slice(supplierStart, configStart)}`;
for (const [name, action, channel, want] of [
  ['main-stock-install', 'install', 'main', 'reject'],
  ['stable-stock-verify', 'verify', 'stable', 'reject'],
  ['stock-runtime', 'verify', 'stable', 'reject'],
  ['main-overlay', 'overlay', 'main', 'accept'],
  ['main-verify', 'verify', 'main', 'accept'],
  ['main-native-version', 'verify', 'main', 'reject'],
  ['main-native-method', 'verify', 'main', 'reject'],
  ['main-native-checksum', 'verify', 'main', 'reject'],
  ['main-native-digest', 'verify', 'main', 'reject'],
]) {
  let supplierReads = 0, imports = 0, observed = 'accept';
  const nativeManifest = { version: name === 'main-native-version' ? '4.1.0' : '4.0.0', method: name === 'main-native-method' ? 'signed-release-asset' : 'go-sumdb-source-build', moduleChecksum: name === 'main-native-checksum' ? 'changed' : 'h1:pZ/XZ2Pk3U9lgXigOTY62zlxxFOHnc9CjQhLgaV/Hfc=', binarySha256: 'owned-native' };
  try {
    await vm.runInNewContext(`(async () => {${supplierBody}})()`, {
      gentle: path.join(root, 'prefix/node_modules/gentle-pi'), selection: { Channel: channel }, action, path,
      read(file) {
        if (file.endsWith('.mjs')) supplierReads++;
        return Buffer.from(file.endsWith('integrity.json') ? JSON.stringify(nativeManifest) : file);
      },
      digest(data) {
        const file = data.toString();
        if (file.endsWith('gentle-ai.exe')) return name === 'main-native-digest' ? 'changed' : 'owned-native';
        return name === 'stock-runtime' && file.endsWith('gentle-ai-installer.mjs') ? 'bc2da0585026fa538f0c6ae0cf50463767c175b71d0dfdb582a88cbe894c84ca' : 'changed';
      },
      pathToFileURL() { imports++; assert.fail('supplier API invocation attempted'); },
      reject(message) { throw Error(`Windows provision refused: ${message}`); },
    }, { timeout: 1000 });
  } catch (error) {
    observed = ['Windows provision refused: stock native supplier source pin', 'Windows provision refused: stock native source manifest'].includes(error.message) ? 'reject' : 'infrastructure';
  }
  let assertionCode = null;
  try {
    assert.equal(observed, want, name);
    assert.equal(imports, 0, 'supplier API must not be reached');
    assert.equal(supplierReads, channel === 'main' && action !== 'install' ? 0 : name === 'stock-runtime' ? 2 : 1);
  } catch (error) { assertionCode = error.code; }
  outcomes.push({ name, expected: want, observed, infrastructure: observed === 'infrastructure', assertionCode });
}
const passed = outcomes.filter(result => result.assertionCode === null).length;
// v2: successful outcomes omit redundant flags; failures retain full details.
const wireOutcomes = outcomes.map(({ name, expected, observed, infrastructure, assertionCode }) =>
  assertionCode === null && !infrastructure ? { name, expected, observed } : { name, expected, observed, infrastructure, assertionCode });
const report = JSON.stringify({ schema: 'windows-owned-settings-fixtures/v2', total: outcomes.length, passed, failed: outcomes.length - passed, outcomes: wireOutcomes });
if (Buffer.byteLength(report, 'utf8') > 4096) throw Object.assign(new Error('whole fixture report exceeds transport bound'), { code: 'WINDOWS_FIXTURE_REPORT_BOUND' });
fs.writeFileSync(process.argv[3], report, { flag: 'wx', encoding: 'utf8' });
process.exitCode = passed === outcomes.length ? 0 : 1;
