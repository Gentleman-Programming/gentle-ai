// Portable VM fixtures for the provisioner's settings region only: they prove the
// typed-scalar allowlist logic, not an installed Windows runtime or Pi UI.
// Evidence: pinned @earendil-works/pi-coding-agent 1.0.0 SettingsManager writes
// defaultProvider/defaultModel (model selection), defaultThinkingLevel and theme
// (a name lookup; getTheme ignores "/" but Windows "\" would join a custom path).
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const provision = process.argv[2] ?? fileURLToPath(new URL('./provision-gentle-shell-windows.mjs', import.meta.url));
const source = fs.readFileSync(provision, 'utf8');
const begin = "const settingsPath = path.join(agent, 'settings.json');";
const end = "if (!read(lockPath).equals(lockBytes)) reject('source lock changed');";
const start = source.indexOf(begin), finish = source.indexOf(end);
assert.ok(start >= 0 && finish > start, 'settings-region boundaries must exist');
const body = source.slice(start, finish);
const win = path.win32, root = 'R:\\Owned Settings Fixture', agent = win.join(root, 'agent');
const owned = () => ({
  packages: [win.join(root, 'prefix/node_modules/gentle-pi')],
  npmCommand: [win.join(root, 'runtime/node/node.exe'), win.join(root, 'runtime/node/node_modules/npm/bin/npm-cli.js'), '--prefix', win.join(root, 'prefix')],
});
const refusal = 'Windows provision refused: owned package/settings bindings changed';
const cases = [
  ['baseline', {}, 'accept'],
  ['model-selection', { defaultProvider: 'anthropic', defaultModel: 'claude-sonnet-4-5' }, 'accept'],
  ['thinking-level', { defaultThinkingLevel: 'high' }, 'accept'],
  ['builtin-theme', { theme: 'dark' }, 'accept'],
  ['package-theme', { theme: 'Gentleman-Cute' }, 'accept'],
  ['all-ui-scalars', { lastChangelogVersion: '1.0.0', extensions: ['-builtin:codemode'], defaultProvider: 'openai', defaultModel: 'gpt-5', defaultThinkingLevel: 'off', theme: 'light' }, 'accept'],
  ['theme-windows-path', { theme: 'R:\\Foreign\\theme' }, 'reject'],
  ['theme-traversal', { theme: '..\\..\\foreign' }, 'reject'],
  ['theme-posix-path', { theme: 'foreign/theme' }, 'reject'],
  ['theme-dots', { theme: '..' }, 'reject'],
  ['theme-type', { theme: ['dark'] }, 'reject'],
  ['model-empty', { defaultModel: '' }, 'reject'],
  ['model-type', { defaultModel: { id: 'x' } }, 'reject'],
  ['model-control', { defaultModel: 'model\u001b[2J' }, 'reject'],
  ['model-bound', { defaultModel: 'm'.repeat(257) }, 'reject'],
  ['provider-type', { defaultProvider: 1 }, 'reject'],
  ['thinking-unknown', { defaultThinkingLevel: 'ultra' }, 'reject'],
  ['thinking-type', { defaultThinkingLevel: ['high'] }, 'reject'],
  ['changed-packages', { packages: ['R:\\Foreign Package'] }, 'reject'],
  ['changed-npm-command', { npmCommand: ['R:\\Foreign Executable'] }, 'reject'],
  ['extension-injection', { extensions: ['-builtin:codemode', 'R:\\Foreign'] }, 'reject'],
];
// Executable, resource, redirection, trust and telemetry keys stay refused.
for (const key of ['skills', 'prompts', 'themes', 'shellPath', 'shellCommandPrefix', 'externalEditor', 'sessionDir', 'httpProxy', 'defaultProjectTrust', 'enableAnalytics', 'customProviders', '__proto__', 'constructor']) {
  cases.push([`injected-${key}`, { [key]: 'R:\\Foreign Resource' }, 'reject']);
}
const failures = [];
for (const [name, extra, want] of cases) {
  // Computed keys (including __proto__) are own data properties, so JSON keeps them.
  const bytes = JSON.stringify({ ...owned(), ...extra });
  assert.ok(Object.keys(extra).every(key => bytes.includes(JSON.stringify(key))), `${name} fixture key serialized`);
  let observed = 'accept';
  try {
    vm.runInNewContext(body, {
      root, finalRoot: root, agent, path: win, action: 'verify',
      read(file) {
        assert.equal(file, win.join(agent, 'settings.json'), 'only virtual settings reads permitted');
        return Buffer.from(bytes);
      },
      exclusive() { assert.fail('verify attempted settings write'); },
      reject(message) { throw Error(`Windows provision refused: ${message}`); },
    }, { timeout: 1000 });
  } catch (error) {
    observed = error.message === refusal ? 'reject' : `infrastructure: ${error.message}`;
  }
  if (observed !== want) failures.push(`${name}: expected ${want}, observed ${observed}`);
}
console.log(`windows-mutable-settings: ${cases.length - failures.length}/${cases.length} passed`);
for (const failure of failures) console.log(`FAIL ${failure}`);
process.exitCode = failures.length === 0 ? 0 : 1;
