import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// Host contracts of the owned provisioner's Main SDK runtime actions. The full
// action pipeline needs an authenticated stock acquisition; these checks pin
// the platform facts its exact readbacks depend on.
const source = fs.readFileSync(fileURLToPath(new URL('./provision-gentle-shell-private-global.mjs', import.meta.url)), 'utf8');

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

// The launch fast path is a bounded optimization of the byte-level graph
// audit, never a replacement: it engages only for the launch-time
// verify-main-sdk-selected action, its stamps bind ctime (which userspace
// cannot forge), and any malformed or drifting record falls back to the full
// audit, which alone may rewrite the receipt.
test('Main graph receipt fast path stays bounded to the launch verify action', () => {
  assert.match(source, /if \(verifyingSelectedSDK\) graphFast = graphFastPath\(\);/);
  assert.match(source, /schema: 'gentle-shell-main-graph-verify-receipt\/v1'/);
  assert.match(source, /installation: digest\(read\(installationPath\)\)/);
  assert.match(source, /lock: digest\(lockBytes\)/);
  assert.match(source, /if \(!graphStampsEqual\(receipt\.entries, graphStamps\(modules\)\)\) return null;/);
  // The recorded stamp must bind the inode identity and both stat times, so a
  // rewritten file can never match a record of the bytes it replaced.
  assert.match(source, /const stamp = `\$\{info\.dev\}:\$\{info\.ino\}:\$\{info\.uid\}:\$\{info\.mode\}:\$\{info\.size\}:\$\{info\.mtimeMs\}:\$\{info\.ctimeMs\}`;/);
  // The full audit stays the only receipt writer, after every check passed.
  assert.match(source, /if \(verifyingSelectedSDK\) \{\s*\n\s*graphReceiptCandidate = \{/);
  assert.match(source, /writeExclusive\(stage, `\$\{JSON\.stringify\(graphReceiptCandidate\)\}\\n`\);/);
  // The observed records the fast path reuses come from the receipt, and the
  // full walk stays the fallback branch.
  assert.match(source, /observed\.push\(\.\.\.graphFast\);/);
  assert.match(source, /packages\(modules\);/);
});
