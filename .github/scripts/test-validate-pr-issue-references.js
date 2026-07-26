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

test('extracts a visible Fixes #N reference', () => {
  assert.deepEqual(extractLinkedIssueNumbers('Fixes #1770'), [1770]);
});

test('ignores embedded prose such as "discloses #42"', () => {
  assert.deepEqual(extractLinkedIssueNumbers('This PR discloses #42 in the body.'), []);
});

test('rejects malformed references with trailing letters', () => {
  // #42abc fails to match because the digit is followed by letters
  // (a word character), so the negative lookahead rejects the match.
  assert.deepEqual(extractLinkedIssueNumbers('Closes #42abc'), []);
});

test('accepts a single valid reference even when malformed neighbours exist', () => {
  // The malformed neighbour #42abc is rejected, the valid #1770 is kept.
  // This mirrors the CI status:approved gate, which only validates issue
  // numbers that survive the parser; a malformed neighbour must not pull a
  // fake issue number into the validation set.
  assert.deepEqual(extractLinkedIssueNumbers('Closes #42abc\nCloses #1770'), [1770]);
});
