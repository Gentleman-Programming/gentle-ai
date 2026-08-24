'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const { parseLinkedIssues } = require('./parse-linked-issues.cjs');

const closing = (number) => ({ number, kind: 'closing' });
const nonClosing = (number) => ({ number, kind: 'non-closing' });
const ok = (...references) => ({ references, errors: [] });

test('references inside HTML comments are ignored; an unclosed comment hides the rest', () => {
  const cases = [
    ['## Summary\n\n<!--\nCloses #42\n-->\n\nSome visible text.', []],
    ['Closes #1770\n\n<!--\nExample: Closes #42\n-->', [closing(1770)]],
    ['Fixes #7\n<!-- Closes #42 --> trailing visible text', [closing(7)]],
    ['Closes #10\n<!-- Refs #42 -->', [closing(10)]],
    ['Closes #1770\n<!-- forgot to close this comment\nCloses #42', [closing(1770)]],
  ];
  for (const [body, references] of cases) {
    assert.deepEqual(parseLinkedIssues(body), ok(...references));
  }
});

test('Markdown reference definitions are ignored while visible references remain', () => {
  const body = [
    '[x]: https://example.invalid "Refs #42"',
    '[y]: https://example.invalid "Closes owner/repo#7"',
    '',
    'Refs #43',
    'Closes #44',
  ].join('\n');

  assert.deepEqual(parseLinkedIssues(body), ok(nonClosing(43), closing(44)));
});

test('closing and non-closing references are kind-tagged, in order of appearance', () => {
  const cases = [
    ['Closes #10\nFixes #11\nResolves #12', [closing(10), closing(11), closing(12)]],
    ['Refs #1770', [nonClosing(1770)]],
    ['refs #7', [nonClosing(7)]],
    ['Closes #10\nRefs #11\nFixes #12\nResolves #13', [closing(10), nonClosing(11), closing(12), closing(13)]],
  ];
  for (const [body, references] of cases) {
    assert.deepEqual(parseLinkedIssues(body), ok(...references));
  }
});

test('an empty or missing body yields no references and no errors', () => {
  for (const body of ['', null, undefined]) {
    assert.deepEqual(parseLinkedIssues(body), ok());
  }
});

test('malformed and cross-repository keyword references fail closed with raw and reason', () => {
  const cases = [
    ['Closes #abc', /malformed/i],
    ['Refs #', /malformed/i],
    ['Refs #12foo', /malformed/i],
    ['Refs #12_bar', /malformed/i],
    ['Fixes #7x', /malformed/i],
    ['Closes gentleman-programming/gentle-ai#42', /cross-repositor/i],
    ['Refs other/owner#7', /cross-repositor/i],
    ['Resolves upstream/repo#99', /cross-repositor/i],
    ['Closes owner/repo#abc', /cross-repositor/i],
    ['Refs owner/repo#abc', /cross-repositor/i],
    ['Refs owner/#7', /cross-repositor/i],
    ['Refs /repo#7', /cross-repositor/i],
    ['Resolves upstream/repo#', /cross-repositor/i],
  ];
  for (const [body, reason] of cases) {
    const result = parseLinkedIssues(body);
    assert.deepEqual(result.references, [], `expected no references for: ${body}`);
    assert.equal(result.errors.length, 1, `expected one error for: ${body}`);
    assert.equal(result.errors[0].raw, body);
    assert.match(result.errors[0].reason, reason);
  }
});

test('slash-heavy text without a hash is not a cross-repository reference', () => {
  const body = `Refs ${'segment/'.repeat(10_000)}tail`;

  assert.deepEqual(parseLinkedIssues(body), ok());
});

test('a valid reference next to a malformed one still fails closed, reporting both', () => {
  const result = parseLinkedIssues('Closes #1770\nRefs #oops');
  assert.deepEqual(result.references, [closing(1770)]);
  assert.equal(result.errors.length, 1);
  assert.match(result.errors[0].raw, /Refs #oops/);
  assert.match(result.errors[0].reason, /malformed/i);
});

test('punctuation after a valid reference is accepted, never treated as malformed', () => {
  for (const [body, kind] of [
    ['Closes #42.', 'closing'],
    ['Refs #42,', 'non-closing'],
    ['Closes #42', 'closing'],
  ]) {
    assert.deepEqual(parseLinkedIssues(body), ok({ number: 42, kind }));
  }
});

test('ordinary prose containing closing words is not a reference', () => {
  const body =
    'This PR closes the loop on the earlier discussion and resolves the confusion about whether the pipeline refs the right target.';
  assert.deepEqual(parseLinkedIssues(body), ok());
});

test('the same issue as both closing and non-closing is ambiguous and fails closed', () => {
  const result = parseLinkedIssues('Closes #42\nRefs #42');
  assert.equal(result.references.length, 2);
  assert.equal(result.errors.length, 1);
  assert.match(result.errors[0].raw, /#42/);
  assert.match(result.errors[0].reason, /ambiguous/i);
});
