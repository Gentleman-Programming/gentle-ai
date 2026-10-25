import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const [mainRoot, stockRoot, warmRoot, scenario] = process.argv.slice(2);
assert.equal(process.argv.length, 6);
const load = (root, name) => import(pathToFileURL(path.join(root, name)).href);
const main = await load(mainRoot, 'scripts/gentle-ai-installer.mjs');
const stock = await load(stockRoot, 'scripts/gentle-ai-installer.mjs');
const mainResolver = await load(mainRoot, 'runtime/gentle-ai-binary.mjs');
const stockResolver = await load(stockRoot, 'runtime/gentle-ai-binary.mjs');

// Only the fixed ABI imported by the published native resolver, not a generic
// provider surface or a claim about changed download/build/publication behavior.
for (const name of [
  'INSTALLER_VERSION', 'GENTLE_AI_INSTALL_METHOD',
  'GENTLE_AI_WINDOWS_SOURCE_PACKAGE_PATH', 'GENTLE_AI_WINDOWS_SOURCE_MODULE',
  'GENTLE_AI_WINDOWS_SOURCE_TAG', 'GENTLE_AI_WINDOWS_SOURCE_MODULE_CHECKSUM',
]) assert.deepEqual(main[name], stock[name], name);
assert.equal(main.INSTALLER_VERSION, '4.2.0');
assert.deepEqual(main.GENTLE_AI_RELEASE_ASSETS, stock.GENTLE_AI_RELEASE_ASSETS);
for (const [platform, arch] of [['linux', 'x64'], ['darwin', 'arm64']]) {
  assert.deepEqual(main.resolveGentleAiReleaseAsset(platform, arch),
    stock.resolveGentleAiReleaseAsset(platform, arch));
}
assert.equal(main.isWindowsGoSumdbSourceTarget('win32', 'x64'), stock.isWindowsGoSumdbSourceTarget('win32', 'x64'));
for (const version of ['go1.25.9', 'go1.25.10']) {
  assert.equal(main.isGentleAiWindowsGoVersionSupported(version),
    stock.isGentleAiWindowsGoVersionSupported(version));
}

function snapshot(file) {
  const info = fs.lstatSync(file, { bigint: true });
  const identity = [info.dev, info.ino, info.mode, info.size, info.mtimeNs, info.ctimeNs];
  if (info.isSymbolicLink()) return [identity, fs.readlinkSync(file)];
  if (info.isDirectory()) return [identity, fs.readdirSync(file).sort().map(name => [name, snapshot(path.join(file, name))])];
  return [identity, fs.readFileSync(file)];
}
const warmBefore = snapshot(warmRoot);
const env = { env: { GENTLE_PI_CONFIG_HOME: path.join(warmRoot, 'config') }, home: path.join(warmRoot, 'home') };
for (const resolver of [stockResolver, mainResolver]) {
  assert.equal(resolver.resolveGentleAiDevBinaryOverride(env, 'linux'), undefined);
  assert.equal(resolver.gentleAiDevBinaryOverrideConfigured(env), false);
  const resolve = () => resolver.resolveGentleAiBinary(warmRoot, 'linux', fs.readFileSync, env);
  if (scenario === 'valid') {
    assert.equal(resolve(), path.join(warmRoot, '.gentle-ai/v4.2.0/gentle-ai'));
  } else {
    assert.throws(resolve, error => error instanceof resolver.PackageLocalGentleAiBinaryMissingError &&
      error.code === 'package-local-binary-missing');
  }
}
assert.deepEqual(snapshot(warmRoot), warmBefore, 'native resolution changed warm bundle/config');
console.log(JSON.stringify({
  scenario, contract: 'fixed native resolver ABI plus actual Linux warm bundle',
  mainSupplierImported: true, mainResolverImported: true, acceptedWarmBundle: scenario === 'valid',
  installerCalled: false, nativeBinaryExecuted: false, sdkLoaded: false,
  runtimeAdmissionChanged: false, activated: false, ready: false,
}));
