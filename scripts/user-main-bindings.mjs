// Fixed declaration/supplier preflight, not source API or runtime qualification.
// The caller must authenticate stock acquisition and bind Main to its snapshot.
export const mainBindingFiles = Object.freeze([
  'package.json',
  'scripts/gentle-ai-installer.mjs',
  'runtime/gentle-ai-binary.mjs',
]);

// A composition keeps these three files from authenticated stock. Main may
// advance all three, but its bytes are never installed or interpreted: the
// returned plan always substitutes the authenticated stock copies.
export function assertMainSDKPreviewBindings(main, stock) {
  for (const name of mainBindingFiles) {
    if (!Buffer.isBuffer(main[name]) || !Buffer.isBuffer(stock[name]) ||
        main[name].length === 0 || stock[name].length === 0 ||
        main[name].length > 33554432 || stock[name].length > 33554432) {
      throw Error(`Main stock binding differs: ${name}`);
    }
  }
  return mainBindingFiles;
}

export function assertMainBindings(main, stock) {
  for (const name of mainBindingFiles) {
    if (!Buffer.isBuffer(main[name]) || !Buffer.isBuffer(stock[name]) ||
        main[name].length > 33554432 || stock[name].length > 33554432 ||
        !main[name].equals(stock[name])) {
      throw Error(`Main stock binding differs: ${name}`);
    }
  }
}
