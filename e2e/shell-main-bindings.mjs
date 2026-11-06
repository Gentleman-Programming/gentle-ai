import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { assertMainBindings, mainBindingFiles } from '../scripts/user-main-bindings.mjs';

const [mainRoot, stockRoot, expectation] = process.argv.slice(2);
assert.equal(process.argv.length, 5);
const read = root => Object.fromEntries(mainBindingFiles.map(name => [name, fs.readFileSync(path.join(root, name))]));
const before = [mainRoot, stockRoot].flatMap(root => mainBindingFiles.map(name => {
  const file = path.join(root, name);
  return [file, fs.readFileSync(file), fs.statSync(file, { bigint: true })];
}));
if (expectation === 'accept') assert.doesNotThrow(() => assertMainBindings(read(mainRoot), read(stockRoot)));
else {
  assert.ok(mainBindingFiles.includes(expectation), 'fixed rejected binding');
  assert.throws(() => assertMainBindings(read(mainRoot), read(stockRoot)),
    error => error.message === `Main stock binding differs: ${expectation}`);
}
for (const [file, bytes, stat] of before) {
  assert.deepEqual(fs.readFileSync(file), bytes);
  assert.deepEqual(fs.statSync(file, { bigint: true }), stat);
}
console.log('Main declared stock bindings preflight settled; no source executed; Ready=false');
