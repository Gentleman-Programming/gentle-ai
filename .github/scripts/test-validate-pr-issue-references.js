'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {
  extractLinkedIssueNumbers,
} = require('./validate-pr-issue-references.js');

test('extracts a single Closes #N reference', () => {
  assert.deepEqual(extractLinkedIssueNumbers('Closes #1770'), [1770]);
});

test('strips a single-line HTML comment containing Closes #N', () => {
  const body = '<!-- Closes #42 -->\nCloses #1770';

  assert.deepEqual(extractLinkedIssueNumbers(body), [1770]);
});

test('strips a multi-line HTML comment', () => {
  const body = [
    '<!--',
    'Closes #1',
    'Fixes #2',
    '-->',
    'Resolves #1770',
  ].join('\n');

  assert.deepEqual(extractLinkedIssueNumbers(body), [1770]);
});

test('preserves multiple visible references', () => {
  const body = 'Closes #1770\nResolves #123';

  assert.deepEqual(extractLinkedIssueNumbers(body), [1770, 123]);
});

test('returns empty array when only HTML comments present', () => {
  const body = '<!-- Closes #1 --><!-- Fixes #2\nResolves #3 -->';

  assert.deepEqual(extractLinkedIssueNumbers(body), []);
});

test('treats closing references in code fences as visible', () => {
  const body = '```markdown\nCloses #42\n```';

  assert.deepEqual(extractLinkedIssueNumbers(body), [42]);
});
