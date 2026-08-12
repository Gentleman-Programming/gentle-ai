'use strict';

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');
const assert = require('node:assert/strict');

const workflowPath = path.join(__dirname, '..', 'workflows', 'label-gentle-report.yml');

function workflowScript() {
  const workflow = fs.readFileSync(workflowPath, 'utf8');
  const marker = '          script: |\n';
  const start = workflow.indexOf(marker);
  assert.notEqual(start, -1, 'workflow has an embedded github-script guard');
  return workflow.slice(start + marker.length)
    .split('\n')
    .filter((line) => line.startsWith('            '))
    .map((line) => line.slice(12))
    .join('\n');
}

const reportMarker = '<!-- gentle-ai-provider-report:v1 -->';

async function runGuard(body, labels = []) {
  const mutations = [];
  const context = {
    payload: { issue: { number: 2211, title: 'untrusted $(echo title)', body, labels } },
    repo: { owner: 'Gentleman-Programming', repo: 'gentle-ai' },
  };
  const github = { rest: { issues: { addLabels: async (request) => mutations.push(request) } } };
  await vm.runInNewContext(`(async () => {\n${workflowScript()}\n})()`, { context, github });
  return JSON.parse(JSON.stringify(mutations));
}

test('a body with exactly one marker receives exactly one label mutation', async () => {
  assert.deepEqual(await runGuard(`arbitrary content\n${reportMarker}\nmore arbitrary content`), [{
    owner: 'Gentleman-Programming', repo: 'gentle-ai', issue_number: 2211, labels: ['gentle-report'],
  }]);
});

test('a body without the marker is ignored', async () => {
  assert.deepEqual(await runGuard('ordinary manual issue body'), []);
});

test('a duplicate marker is ignored', async () => {
  assert.deepEqual(await runGuard(null), []);
  assert.deepEqual(await runGuard(`${reportMarker}\n${reportMarker}`), []);
});

test('an existing gentle-report label produces no duplicate mutation', async () => {
  assert.deepEqual(await runGuard(reportMarker, [{ name: 'gentle-report' }]), []);
});

test('arbitrary content around one marker needs no report schema', async () => {
  assert.deepEqual(await runGuard(`not a report\n${reportMarker}\n$() <>`), [{
    owner: 'Gentleman-Programming', repo: 'gentle-ai', issue_number: 2211, labels: ['gentle-report'],
  }]);
});

test('the workflow is opened-only, least-privilege, no-checkout, and never reads issue titles or shells', () => {
  const workflow = fs.readFileSync(workflowPath, 'utf8');
  const script = workflowScript();

  assert.match(workflow, /^on:\n  issues:\n    types: \[opened\]$/m);
  assert.match(workflow, /^permissions:\n  issues: write$/m);
  assert.doesNotMatch(workflow, /actions\/checkout|pull_request_target|^\s*run:/m);
  assert.match(workflow, /actions\/github-script@[0-9a-f]{40}/);
  assert.doesNotMatch(script, /context\.payload\.issue\.title|child_process|\bexec\b|\bspawn\b|process\.env/);
});
