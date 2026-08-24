'use strict';

const fs = require('node:fs');

const REVIEW_BUDGET_LIMIT = 400;
const POLICY_KEYS = ['version', 'enforcement', 'limit', 'activation_snapshot', 'grandfathered_prs'];

function validateIds(ids, name) {
  if (!Array.isArray(ids)) throw new Error(`${name} must be an array`);
  const seen = new Set();
  for (const id of ids) {
    if (!Number.isSafeInteger(id) || id <= 0) throw new Error(`${name} must contain positive integers`);
    if (seen.has(id)) throw new Error(`${name} contains duplicate PR number ${id}`);
    seen.add(id);
  }
  return seen;
}

function validatePolicy(policy) {
  if (!policy || typeof policy !== 'object' || Array.isArray(policy)) throw new Error('Policy must be an object');
  const keys = Object.keys(policy);
  const missing = POLICY_KEYS.filter((key) => !Object.hasOwn(policy, key));
  const unknown = keys.filter((key) => !POLICY_KEYS.includes(key));
  if (missing.length) throw new Error(`Policy has missing keys: ${missing.join(', ')}`);
  if (unknown.length) throw new Error(`Policy has unexpected keys: ${unknown.join(', ')}`);
  if (policy.version !== 1 || policy.limit !== REVIEW_BUDGET_LIMIT) throw new Error('Policy version and limit are fixed');
  if (!['dormant', 'enforcing'].includes(policy.enforcement)) throw new Error('Policy enforcement is invalid');

  const grandfathered = validateIds(policy.grandfathered_prs, 'grandfathered_prs');
  if (policy.enforcement === 'dormant') {
    if (policy.activation_snapshot !== null || grandfathered.size) {
      throw new Error('Dormant policy must have no activation snapshot or grandfathered PRs');
    }
    return policy;
  }

  const snapshot = validateIds(policy.activation_snapshot, 'activation_snapshot');
  for (const id of grandfathered) {
    if (!snapshot.has(id)) throw new Error('Policy rejects post-snapshot additions');
  }
  return policy;
}

function detectDuplicateKeys(raw) {
  let depth = 0, inString = false, escape = false, keyStart = -1;
  const seen = new Set();
  for (let i = 0; i < raw.length; i++) {
    const char = raw[i];
    if (escape) { escape = false; continue; }
    if (char === '\\') { escape = true; continue; }
    if (char === '"') {
      if (!inString) {
        inString = true;
        keyStart = i + 1;
      } else {
        inString = false;
        if (depth === 1) {
          let j = i + 1;
          while (j < raw.length && /\s/.test(raw[j])) j++;
          if (raw[j] === ':') {
            const key = JSON.parse(raw.slice(keyStart - 1, i + 1));
            if (seen.has(key)) throw new Error(`Policy contains duplicate key: ${key}`);
            seen.add(key);
          }
        }
      }
      continue;
    }
    if (!inString) {
      if (char === '{' || char === '[') depth++;
      else if (char === '}' || char === ']') depth--;
    }
  }
}

function parsePolicy(raw) {
  let policy;
  try {
    policy = JSON.parse(raw);
  } catch (error) {
    throw new Error(`Policy contains malformed JSON: ${error.message}`);
  }
  detectDuplicateKeys(raw);
  return validatePolicy(policy);
}

function loadPolicy(filePath) {
  try {
    return parsePolicy(fs.readFileSync(filePath, 'utf8'));
  } catch (error) {
    throw new Error(`Policy is unreadable or invalid: ${error.message}`);
  }
}

function pullRequestNumber(pr, key) {
  if (!Number.isSafeInteger(pr[key]) || pr[key] < 0) {
    throw new Error(`${key} must be a non-negative integer from the live PR API`);
  }
  return pr[key];
}

function evaluatePrSize(pr, policy) {
  validatePolicy(policy);
  if (!pr || typeof pr !== 'object') throw new Error('Live PR facts must be an object');
  if (!Number.isSafeInteger(pr.number) || pr.number <= 0) throw new Error('number must be a positive integer from the live PR API');
  const additions = pullRequestNumber(pr, 'additions');
  const deletions = pullRequestNumber(pr, 'deletions');
  const total = additions + deletions;
  if (!Number.isSafeInteger(total)) throw new Error('PR size total must be a safe integer');
  if (total <= policy.limit) return { total, outcome: 'pass', enforced: false, message: 'PR is within the 400-line review budget.' };
  if (policy.enforcement === 'dormant') {
    return { total, outcome: 'warning', enforced: false, message: 'PR exceeds the 400-line review budget; enforcement is dormant.' };
  }
  if (policy.grandfathered_prs.includes(pr.number)) {
    return { total, outcome: 'warning', enforced: false, message: 'PR exceeds the budget but is grandfathered by the activation snapshot.' };
  }
  return { total, outcome: 'failure', enforced: true, message: 'PR exceeds the 400-line review budget.' };
}

function sameIds(left, right) {
  return left.length === right.length && left.every((id, index) => id === right[index]);
}

function qualifiedAtActivation(pullRequests) {
  if (!Array.isArray(pullRequests)) throw new Error('activationPullRequests must be an array');
  const qualified = new Set();
  const seen = new Set();
  for (const pr of pullRequests) {
    if (!pr || typeof pr !== 'object') throw new Error('Activation pull request record must be an object');
    if (!Number.isSafeInteger(pr.number) || pr.number <= 0) {
      throw new Error(`Invalid activation PR record number: ${JSON.stringify(pr.number)}`);
    }
    if (seen.has(pr.number)) throw new Error(`Activation pull requests contain duplicate PR number ${pr.number}`);
    seen.add(pr.number);
    if (!['open', 'closed'].includes(pr.state)) throw new Error(`Invalid activation PR record state for PR #${pr.number}`);
    if (pr.state === 'open') {
      if (!Array.isArray(pr.labels)) {
        throw new Error(`Invalid open PR record labels in activation pull requests for PR #${pr.number}`);
      }
      if (pr.labels.some((label) => (typeof label === 'string' ? label : label?.name) === 'size:exception')) {
        qualified.add(pr.number);
      }
    }
  }
  return qualified;
}

function closedOrMergedIds(pullRequests) {
  if (!Array.isArray(pullRequests)) throw new Error('closedOrMergedPullRequests must be an array');
  const closed = new Set();
  for (const pr of pullRequests) {
    if (pr && Number.isSafeInteger(pr.number) && pr.number > 0 && (pr.state === 'closed' || pr.merged === true)) closed.add(pr.number);
  }
  return closed;
}

function validatePolicyTransition(previous, next, options = {}) {
  validatePolicy(previous);
  validatePolicy(next);
  const closedOrMergedPullRequests = options.closedOrMergedPullRequests || [];
  const closed = closedOrMergedIds(closedOrMergedPullRequests);
  if (previous.activation_snapshot === null) {
    if (next.activation_snapshot === null) {
      if (!sameIds(previous.grandfathered_prs, next.grandfathered_prs)) throw new Error('Dormant policy cannot add grandfathered PRs');
      return { valid: true };
    }
    if (!options || !Array.isArray(options.activationPullRequests)) {
      throw new Error('Activation pull requests must be explicitly provided');
    }
    const eligible = qualifiedAtActivation(options.activationPullRequests);
    if (next.enforcement !== 'enforcing' || !sameIds(next.activation_snapshot, next.grandfathered_prs)) {
      throw new Error('Activation snapshot must exactly establish the enforcing grandfather list');
    }
    if (eligible.size !== next.activation_snapshot.length || next.activation_snapshot.some((id) => !eligible.has(id))) {
      throw new Error('Activation snapshot must exactly match all qualified open PRs');
    }
    return { valid: true };
  }
  if (!sameIds(previous.activation_snapshot, next.activation_snapshot)) throw new Error('Activation snapshot is immutable');
  const before = new Set(previous.grandfathered_prs);
  const after = new Set(next.grandfathered_prs);
  for (const id of after) if (!before.has(id)) throw new Error('Policy rejects post-snapshot additions');
  for (const id of before) if (!after.has(id) && !closed.has(id)) throw new Error(`PR #${id} is not proven closed or merged`);
  return { valid: true };
}

module.exports = { REVIEW_BUDGET_LIMIT, evaluatePrSize, loadPolicy, parsePolicy, validatePolicyTransition };
