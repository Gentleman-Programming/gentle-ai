'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const { parseLinkedIssues } = require('./parse-linked-issues.cjs');

test('a closing reference that only appears inside an HTML comment is not a linked issue', () => {
  const body = [
    '## Summary',
    '',
    '<!--',
    'Link the approved issue this PR resolves, e.g.:',
    'Closes #42',
    '-->',
    '',
    'Some visible description without any reference.',
  ].join('\n');

  assert.deepEqual(parseLinkedIssues(body), []);
});

test('a real reference outside a comment plus the template example inside one finds exactly the real one', () => {
  const body = [
    'Closes #1770',
    '',
    '<!--',
    'Example: Closes #42',
    '-->',
  ].join('\n');

  assert.deepEqual(parseLinkedIssues(body), [1770]);
});

test('a single-line HTML comment is also ignored', () => {
  const body = 'Fixes #7\n<!-- Closes #42 --> trailing visible text';

  assert.deepEqual(parseLinkedIssues(body), [7]);
});

test('multiple visible closing references are all preserved, in order', () => {
  const body = 'Closes #10\nFixes #11\nResolves #12';

  assert.deepEqual(parseLinkedIssues(body), [10, 11, 12]);
});

// Documented behavior: an unclosed `<!--` hides everything through the end of
// the body. This matches how GitHub renders Markdown (the HTML spec closes an
// open comment at end of input), so a reference after an unclosed `<!--` is
// invisible to reviewers and must not count as a linked issue.
test('an unclosed HTML comment hides the rest of the body, matching rendered output', () => {
  const body = 'Closes #1770\n<!-- forgot to close this comment\nCloses #42';

  assert.deepEqual(parseLinkedIssues(body), [1770]);
});

test('an empty or missing body yields no linked issues', () => {
  assert.deepEqual(parseLinkedIssues(''), []);
  assert.deepEqual(parseLinkedIssues(null), []);
  assert.deepEqual(parseLinkedIssues(undefined), []);
});
