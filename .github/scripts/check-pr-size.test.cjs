'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const {
  REVIEW_BUDGET_LIMIT,
  evaluatePrSize,
  loadPolicy,
  parsePolicy,
  validatePolicyTransition,
} = require('./check-pr-size.cjs');

const policyPath = path.join(__dirname, '..', 'grandfather-size-exceptions.json');
const workflowPath = path.join(__dirname, '..', 'workflows', 'pr-size-policy.yml');

function yamlMapping(workflow, key, indent = '') {
  const match = workflow.match(new RegExp(`^${indent}${key}:[ \\t]*\\r?\\n(?<mapping>(?:^${indent}  [^\\r\\n]*(?:\\r?\\n|$))*)`, 'm'));
  assert.ok(match, `workflow must declare a ${indent ? 'nested' : 'top-level'} ${key} mapping`);
  return match.groups.mapping;
}
function dormantPolicy(overrides = {}) {
  return { version: 1, enforcement: 'dormant', limit: REVIEW_BUDGET_LIMIT, activation_snapshot: null, grandfathered_prs: [], ...overrides };
}

function pullRequest(overrides = {}) {
  return { number: 3586, additions: 200, deletions: 200, labels: [], ...overrides };
}
test('400 is within the dormant review budget', () => {
  const result = evaluatePrSize(pullRequest(), dormantPolicy());
  assert.equal(result.total, 400);
  assert.equal(result.outcome, 'pass');
  assert.equal(result.enforced, false);
});
test('401 is reported but not enforced while the policy is dormant', () => {
  const result = evaluatePrSize(pullRequest({ additions: 401, deletions: 0 }), dormantPolicy());
  assert.equal(result.total, 401);
  assert.equal(result.outcome, 'warning');
  assert.equal(result.enforced, false);
  assert.match(result.message, /dormant/i);
});
test('missing, null, non-integer, and negative API counts fail closed', () => {
  for (const additions of [undefined, null, '400', 400.5, -1]) {
    assert.throws(() => evaluatePrSize(pullRequest({ additions }), dormantPolicy()), /additions must be a non-negative integer/);
  }
  for (const deletions of [undefined, null, '0', 0.5, -1]) {
    assert.throws(() => evaluatePrSize(pullRequest({ deletions }), dormantPolicy()), /deletions must be a non-negative integer/);
  }
});

test('policy accepts valid JSON with the expected schema', () => {
  const policy = parsePolicy(fs.readFileSync(policyPath, 'utf8'));
  assert.deepEqual(policy, dormantPolicy());
});

test('policy rejects a literal duplicate top-level key', () => {
  const raw = '{"version":1,"enforcement":"dormant","enforcement":"enforcing","limit":400,"activation_snapshot":null,"grandfathered_prs":[]}';
  assert.throws(() => parsePolicy(raw), /Policy contains duplicate key: enforcement/);
});

test('policy rejects a duplicate top-level key with a semantically equivalent JSON escape', () => {
  const raw = '{"version":1,"enforcement":"dormant","enforc\\u0065ment":"enforcing","limit":400,"activation_snapshot":null,"grandfathered_prs":[]}';
  assert.throws(() => parsePolicy(raw), /Policy contains duplicate key: enforcement/);
});

test('policy rejects invalid JSON escapes', () => {
  const raw = '{"version":1,"enforcement":"dormant","limit":400,"activation_snapshot":null,"grandfathered_prs":[],"invalid\\x20key":true}';
  assert.throws(() => parsePolicy(raw), /Policy contains malformed JSON/);
});

test('policy rejects JSON with an incorrect schema', () => {
  assert.throws(() => parsePolicy(JSON.stringify(dormantPolicy({ unknown: true }))), /unexpected keys/);
  assert.throws(() => parsePolicy('{}'), /missing keys/);
  assert.throws(() => loadPolicy(''), /unreadable or invalid/);
});

test('policy rejects duplicate and invalid grandfather IDs', () => {
  assert.throws(() => parsePolicy(JSON.stringify(dormantPolicy({ grandfathered_prs: [7, 7] }))), /duplicate PR number/);
  assert.throws(() => parsePolicy(JSON.stringify(dormantPolicy({ grandfathered_prs: [0] }))), /positive integers/);
});

test('policy transitions require explicit activation facts and reject malformed open-PR records', () => {
  const active = { version: 1, enforcement: 'enforcing', limit: REVIEW_BUDGET_LIMIT, activation_snapshot: [7, 8], grandfathered_prs: [7, 8] };
  assert.throws(() => validatePolicyTransition(dormantPolicy(), active), /Activation pull requests must be explicitly provided/);
  assert.throws(() => validatePolicyTransition(dormantPolicy(), active, {}), /Activation pull requests must be explicitly provided/);
  assert.throws(() => validatePolicyTransition(dormantPolicy(), active, { activationPullRequests: [{ state: 'open', number: 'invalid', labels: ['size:exception'] }] }), /Invalid activation PR record number/);
  assert.throws(() => validatePolicyTransition(dormantPolicy(), active, { activationPullRequests: [{ state: 'open', number: 7, labels: 'not-an-array' }] }), /Invalid open PR record labels/);
  for (const state of [undefined, 'invalid']) {
    assert.throws(() => validatePolicyTransition(dormantPolicy(), active, { activationPullRequests: [{ state, number: 7, labels: [] }] }), /Invalid activation PR record state/);
  }
  assert.throws(() => validatePolicyTransition(dormantPolicy(), active, { activationPullRequests: [{ state: 'closed', number: 7 }, { state: 'open', number: 7, labels: ['size:exception'] }] }), /duplicate PR number/);
});

test('policy transitions allow only closed or merged grandfather removals after activation', () => {
  const active = { version: 1, enforcement: 'enforcing', limit: REVIEW_BUDGET_LIMIT, activation_snapshot: [7, 8], grandfathered_prs: [7, 8] };
  assert.deepEqual(validatePolicyTransition(dormantPolicy(), active, {
    activationPullRequests: [
      { number: 7, state: 'open', labels: ['size:exception'] },
      { number: 8, state: 'open', labels: ['size:exception'] },
    ],
  }), { valid: true });
  assert.throws(() => validatePolicyTransition(dormantPolicy(), { ...active, activation_snapshot: [7], grandfathered_prs: [7] }, {
    activationPullRequests: [{ number: 7, state: 'open', labels: ['size:exception'] }, { number: 8, state: 'open', labels: ['size:exception'] }],
  }), /exactly match all qualified open PRs/);
  assert.throws(() => validatePolicyTransition(dormantPolicy(), active, { activationPullRequests: [{ number: 7, state: 'closed', labels: ['size:exception'] }] }), /exactly match all qualified open PRs/);
  assert.equal(evaluatePrSize(pullRequest({ number: 7, additions: 401, deletions: 0 }), active).enforced, false);
  assert.deepEqual(validatePolicyTransition(active, { ...active, grandfathered_prs: [7] }, { closedOrMergedPullRequests: [{ number: 8, state: 'closed' }] }), { valid: true });
  assert.throws(() => validatePolicyTransition(active, { ...active, grandfathered_prs: [7] }, { closedOrMergedPullRequests: [] }), /not proven closed or merged/);
  assert.throws(() => validatePolicyTransition(active, { ...active, grandfathered_prs: [7, 8, 9] }), /post-snapshot additions/);
});

test('workflow is trusted, read-only, and never evaluates merge queue or candidate bytes', () => {
  const workflow = fs.readFileSync(workflowPath, 'utf8');
  const lifecycle = yamlMapping(yamlMapping(workflow, 'on'), 'pull_request_target', '  ');
  const permissions = yamlMapping(workflow, 'permissions');
  assert.match(lifecycle, /^    types: \[opened, reopened, synchronize, edited, labeled, unlabeled\]\r?$/m);
  assert.match(permissions, /^[ \t]+contents:[ \t]+read[ \t]*$/m);
  assert.match(permissions, /^[ \t]+pull-requests:[ \t]+read[ \t]*$/m);
  assert.match(permissions, /^[ \t]+issues:[ \t]+read[ \t]*$/m);
  assert.match(workflow, /ref: \$\{\{ github\.event\.repository\.default_branch \}\}/);
  assert.match(workflow, /persist-credentials: false/);
  assert.match(workflow, /sparse-checkout: \|/);
  assert.match(workflow, /github\.rest\.pulls\.get/);
  assert.match(workflow, /Unable to read live PR facts/);
  assert.doesNotMatch(workflow, /merge_group/);
  assert.doesNotMatch(workflow, /refs\/pull/);
  assert.doesNotMatch(workflow, /github\.event\.pull_request\.head/);
});

test('workflow lifecycle and permissions ignore similarly named job commands', () => {
  const workflow = [
    'on:',
    '  pull_request_target:',
    '    types: [opened]',
    'permissions:',
    '  contents: write',
    'jobs:',
    '  policy:',
    '    steps:',
    '      - run: |',
    '          pull_request_target:',
    '            types: [opened, reopened, synchronize, edited, labeled, unlabeled]',
    '          contents: read',
  ].join('\n');
  const lifecycle = yamlMapping(yamlMapping(workflow, 'on'), 'pull_request_target', '  '), permissions = yamlMapping(workflow, 'permissions');

  assert.doesNotMatch(lifecycle, /^    types: \[opened, reopened, synchronize, edited, labeled, unlabeled\]\r?$/m);
  assert.doesNotMatch(permissions, /^[ \t]+contents:[ \t]+read[ \t]*$/m);
});
