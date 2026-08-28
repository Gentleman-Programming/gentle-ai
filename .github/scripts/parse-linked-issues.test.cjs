'use strict';

const { readFileSync } = require('node:fs');
const { resolve } = require('node:path');
const { test } = require('node:test');
const assert = require('node:assert/strict');

const { bindRenderedBody, parseLinkedIssues } = require('./parse-linked-issues.cjs');

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

test('bindRenderedBody returns the authoritative rendered body when unrelated activity updates the PR', () => {
  const eventPR = { number: 42, updated_at: '2026-03-10T12:00:00Z', body: 'Closes #42' };
  const currentPR = { number: 42, updated_at: '2026-03-10T12:00:01Z', body: 'Closes #42', body_text: 'Reviewer-visible text' };

  assert.equal(bindRenderedBody(eventPR, currentPR), currentPR.body_text);
});

test('bindRenderedBody rejects unmatched or unusable PR snapshots', () => {
  const eventPR = { number: 42, updated_at: '2026-03-10T12:00:00Z', body: null };
  const currentPR = { number: 42, updated_at: '2026-03-10T12:00:00Z', body: null, body_text: '' };
  const cases = [
    ['missing event PR', null, currentPR, /event pull request/i],
    ['missing fetched PR', eventPR, null, /fetched pull request/i],
    ['number mismatch', eventPR, { ...currentPR, number: 43 }, /numbers do not match/i],
    ['coerced event number', { ...eventPR, number: true }, { ...currentPR, number: 1 }, /numbers do not match/i],
    ['raw body mismatch', eventPR, { ...currentPR, body: 'Closes #42' }, /raw bodies do not match/i],
    ['matching invalid raw bodies', { ...eventPR, body: true }, { ...currentPR, body: true }, /raw bodies do not match/i],
    ['missing body_text', eventPR, { ...currentPR, body_text: undefined }, /body_text.*string/i],
    ['null body_text', eventPR, { ...currentPR, body_text: null }, /body_text.*string/i],
    ['non-string body_text', eventPR, { ...currentPR, body_text: 42 }, /body_text.*string/i],
  ];

  for (const [, event, current, error] of cases) {
    assert.throws(() => bindRenderedBody(event, current), error);
  }
});

test('the workflow binds and parses GitHub rendered text rather than the event body', () => {
  const workflow = readFileSync(resolve(__dirname, '../workflows/pr-check.yml'), 'utf8');

  assert.match(workflow, /pull-requests: read/);
  assert.match(workflow, /issues: read/);
  assert.match(workflow, /GET \/repos\/\{owner\}\/\{repo\}\/pulls\/\{pull_number\}/);
  assert.match(workflow, /accept:\s*'application\/vnd\.github\.full\+json'/);
  assert.match(workflow, /bindRenderedBody\(context\.payload\.pull_request, currentPR\)/);
  assert.match(workflow, /parseLinkedIssues\(bodyText\)/);
  assert.equal((workflow.match(/parseLinkedIssues\(/g) || []).length, 1);
  assert.doesNotMatch(workflow, /parseLinkedIssues\(context\.payload\.pull_request\.body\)/);
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
  const eventPR = { number: 42, body: null };
  const currentPR = { number: 42, body: null, body_text: '' };
  assert.equal(bindRenderedBody(eventPR, currentPR), '');

  for (const body of ['', null, undefined]) {
    assert.deepEqual(parseLinkedIssues(body), ok());
  }
});

test('malformed and cross-repository keyword references fail closed with raw and reason', () => {
  const cases = [
    ['Closes #abc', /malformed/i],
    ['Refs #', /malformed/i],
    ['Refs#43', /malformed/i],
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

test('slash-heavy text and URL/path fragments are not issue references', () => {
  const body = `https://x/refs#anchor https://x/path?next=Refs#43&other=1 Refs ${'segment/'.repeat(10_000)}tail`;

  assert.deepEqual(parseLinkedIssues(body), ok());
});

test('a valid reference next to a malformed one still fails closed, reporting both', () => {
  const result = parseLinkedIssues('Closes #1770\nRefs #oops');
  assert.deepEqual(result.references, [closing(1770)]);
  assert.equal(result.errors.length, 1);
  assert.match(result.errors[0].raw, /Refs #oops/);
  assert.match(result.errors[0].reason, /malformed/i);
});

test('a valid reference does not mask malformed suffixes or cross-repository targets', () => {
  const cases = [
    ['Refs #42/extra', /malformed/i],
    ['Refs github.com/owner/repo#42', /cross-repositor/i],
    ['Refs https://github.com/owner/repo#42', /cross-repositor/i],
  ];
  for (const [invalidReference, reason] of cases) {
    const result = parseLinkedIssues(`Closes #1770\n${invalidReference}`);
    assert.deepEqual(result.references, [closing(1770)]);
    assert.equal(result.errors.length, 1);
    assert.equal(result.errors[0].raw, invalidReference);
    assert.match(result.errors[0].reason, reason);
  }
});

test('punctuation after a valid reference is accepted, never treated as malformed', () => {
  for (const [body, kind] of [
    ['Closes #42.', 'closing'],
    ['Refs #42,', 'non-closing'],
    ['Closes #42)', 'closing'],
    ['Refs #42]', 'non-closing'],
    ['Closes #42`', 'closing'],
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
