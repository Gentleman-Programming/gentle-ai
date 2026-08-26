'use strict';

const { chmodSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } = require('node:fs');
const { tmpdir } = require('node:os');
const { join } = require('node:path');
const { spawnSync } = require('node:child_process');
const { test } = require('node:test');
const assert = require('node:assert/strict');

const root = join(__dirname, '..', '..');
const checker = join(root, 'scripts', 'check-ruleset-drift.sh');

const manifest = JSON.parse(
  readFileSync(join(root, 'scripts', 'merge-blocking-status-contexts.json'), 'utf8'),
);

function fixture(rulesetEntries = manifest.required_status_checks) {
  const directory = mkdtempSync(join(tmpdir(), 'gentle-ai-policy-drift-'));
  const bin = join(directory, 'bin');
  mkdirSync(bin);
  const callsPath = join(directory, 'calls.log');
  const ruleset = {
      id: 13932547,
      enforcement: 'active',
      rules: [
        {
          type: 'required_status_checks',
          parameters: { required_status_checks: rulesetEntries },
        },
      ],
  };
  writeFileSync(
    join(bin, 'gh'),
    `#!/bin/sh
printf '%s\\n' "$*" >> "$GH_CALLS"
case "$*" in
  *'rulesets?includes_parents=false'*) printf '%s' "$GH_LIST" ;;
  *'rulesets/13932547'*) printf '%s' "$GH_RULESET" ;;
  *) exit 1 ;;
esac
`,
  );
  chmodSync(join(bin, 'gh'), 0o755);
  return {
    directory,
    callsPath,
    env: {
      ...process.env,
      PATH: `${bin}:${process.env.PATH}`,
      GH_LIST: JSON.stringify([{ id: 13932547, enforcement: 'active' }]),
      GH_RULESET: JSON.stringify(ruleset),
      GH_CALLS: callsPath,
      GH_TOKEN: 'test-secret-that-must-not-leak',
      GITHUB_REPOSITORY: 'Gentleman-Programming/gentle-ai',
      POLICY_MANIFEST: join(root, 'scripts', 'merge-blocking-status-contexts.json'),
    },
  };
}

function run(options = {}) {
  const testFixture = fixture(options.entries);
  if (options.list) {
    testFixture.env.GH_LIST = JSON.stringify(options.list);
  }
  if (options.ruleset) {
    testFixture.env.GH_RULESET = JSON.stringify(options.ruleset);
  }
  if (options.manifest) {
    const manifestPath = join(testFixture.directory, 'manifest.json');
    writeFileSync(manifestPath, JSON.stringify(options.manifest));
    testFixture.env.POLICY_MANIFEST = manifestPath;
  }
  if (options.missingToken) {
    delete testFixture.env.GH_TOKEN;
  }
  if (options.ghFailure) {
    writeFileSync(
      join(testFixture.directory, 'bin', 'gh'),
      '#!/bin/sh\nprintf \'gh failed test-secret-that-must-not-leak\\n\' >&2\nexit 1\n',
    );
    chmodSync(join(testFixture.directory, 'bin', 'gh'), 0o755);
  }
  const result = spawnSync(checker, [], {
    cwd: root,
    env: testFixture.env,
    encoding: 'utf8',
  });
  return {
    ...result,
    fixture: testFixture,
    output: `${result.stdout || ''}${result.stderr || ''}`,
  };
}

test('an exact active ruleset match passes using only GET API calls', () => {
  const result = run();

  assert.equal(result.status, 0, result.output);
  assert.match(result.stdout, /merge policy verified/);
  assert.doesNotMatch(result.output, /test-secret/);
  const calls = readFileSync(result.fixture.callsPath, 'utf8').trim().split('\n');
  assert.deepEqual(calls, [
    'api --method GET repos/Gentleman-Programming/gentle-ai/rulesets?includes_parents=false',
    'api --method GET repos/Gentleman-Programming/gentle-ai/rulesets/13932547',
  ]);
  assert.doesNotMatch(calls.join('\n'), /POST|PATCH|PUT|DELETE/);
});

test('a required context mismatch is typed policy_drift', () => {
  const result = run({
    entries: manifest.required_status_checks.filter(({ context }) => context !== 'Darwin Runtime'),
  });

  assert.equal(result.status, 1);
  assert.match(result.output, /policy_drift/);
  assert.doesNotMatch(result.output, /test-secret/);
});

test('an API failure is typed policy_unverifiable without exposing credentials', () => {
  const result = run({ ghFailure: true });

  assert.equal(result.status, 2);
  assert.match(result.output, /policy_unverifiable/);
  assert.doesNotMatch(result.output, /test-secret/);
});

test('missing repository-admin credentials are typed policy_unverifiable', () => {
  const result = run({ missingToken: true });

  assert.equal(result.status, 2);
  assert.match(result.output, /policy_unverifiable: GH_TOKEN is required/);
});

test('zero active rulesets is not treated as an empty policy', () => {
  const result = run({ list: [] });

  assert.equal(result.status, 2);
  assert.match(result.output, /policy_unverifiable: no active rulesets/);
});

test('an unknown ruleset schema is typed policy_unverifiable', () => {
  const result = run({
    ruleset: {
      id: 13932547,
      enforcement: 'active',
      rules: [{ type: 'required_status_checks', parameters: {} }],
    },
  });

  assert.equal(result.status, 2);
  assert.match(result.output, /policy_unverifiable: ruleset response has an unknown schema/);
});

test('an unknown manifest schema is typed policy_unverifiable', () => {
  const result = run({ manifest: { required_status_checks: [] } });

  assert.equal(result.status, 2);
  assert.match(result.output, /policy_unverifiable: policy manifest has an unknown schema/);
});
