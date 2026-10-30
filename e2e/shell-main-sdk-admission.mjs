import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { assertMainBindings, assertMainSDKPreviewBindings, mainBindingFiles } from '../scripts/user-main-bindings.mjs';

// Main is read as DATA only. Stock acquisition authentication belongs to the
// caller; this fixed plan never admits Main's changed installer as a supplier.
const [mainRoot, stockRoot] = process.argv.slice(2);
const read = root => Object.fromEntries(mainBindingFiles.map(name => [name, fs.readFileSync(path.join(root, name))]));
const main = read(mainRoot), stock = read(stockRoot);
// Reads can update atime; it is not a supplier mutation. Keep every other field.
const stableStat = file => {
  const { atimeMs: _atimeMs, atimeNs: _atimeNs, ...identity } = fs.statSync(file, { bigint: true });
  return identity;
};
const before = [mainRoot, stockRoot].flatMap(root => mainBindingFiles.map(name => {
  const file = path.join(root, name);
  return [file, fs.readFileSync(file), stableStat(file)];
}));
assert.throws(() => assertMainBindings(main, stock),
  error => error.message === 'Main stock binding differs: scripts/gentle-ai-installer.mjs');
const plan = assertMainSDKPreviewBindings(main, stock);
assert.deepEqual(plan, ['package.json', 'scripts/gentle-ai-installer.mjs', 'runtime/gentle-ai-binary.mjs']);
assert.ok(Object.isFrozen(plan));
assert.notDeepEqual(main[plan[1]], stock[plan[1]], 'fixture must exercise the actual supplier mismatch');

{
  const changed = { ...main, 'runtime/gentle-ai-binary.mjs': Buffer.concat([main['runtime/gentle-ai-binary.mjs'], Buffer.from('\nchanged\n')]) };
  assert.deepEqual(assertMainSDKPreviewBindings(changed, stock),
    ['package.json', 'scripts/gentle-ai-installer.mjs', 'runtime/gentle-ai-binary.mjs']);
}
for (const field of ['dependencies', 'exports', 'pi', 'engines', 'scripts', 'unknown']) {
  const metadata = JSON.parse(main['package.json']);
  metadata[field] = { changed: 'metadata may advance' };
  assert.deepEqual(assertMainSDKPreviewBindings({ ...main, 'package.json': Buffer.from(JSON.stringify(metadata)) }, stock),
    ['package.json', 'scripts/gentle-ai-installer.mjs', 'runtime/gentle-ai-binary.mjs']);
}
for (const value of [undefined, null, Buffer.alloc(0), Buffer.alloc(33554433)]) {
  assert.throws(() => assertMainSDKPreviewBindings({ ...main, 'scripts/gentle-ai-installer.mjs': value }, stock));
  assert.throws(() => assertMainSDKPreviewBindings(main, { ...stock, 'scripts/gentle-ai-installer.mjs': value }));
}
for (const [file, bytes, stat] of before) {
  assert.deepEqual(fs.readFileSync(file), bytes);
  assert.deepEqual(stableStat(file), stat);
}
console.log('MAIN_SDK_ADMISSION_PASS exact_metadata_resolver=true preserve_all_three_stock_bindings=true full_main_refused=true activated=false');
