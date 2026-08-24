'use strict';

// Single parse seam for every pr-check.yml gate reading linked issues.
// Returns kind-tagged references plus errors that make invalid input fail closed.

const CLOSING_KEYWORDS = 'closes|fixes|resolves';
const NON_CLOSING_KEYWORDS = 'refs';
const KEYWORDS = `${CLOSING_KEYWORDS}|${NON_CLOSING_KEYWORDS}`;

// Valid references end at whitespace or common Markdown punctuation. Every
// other suffix (for example, #12/extra) is malformed rather than #12.
const VALID_REFERENCE_END = String.raw`$|[\s.,;:!?)}\]'"\`]`;
const REFERENCE_PATTERN = new RegExp(
  `\\b(${KEYWORDS})\\s+#(\\d+)(?=${VALID_REFERENCE_END})`,
  'gi'
);

// owner/repo#N fails closed; non-overlapping token classes bound each match
// to a linear scan, and the approval gate resolves only base-repo issues.
const CROSS_REPO_PATTERN = new RegExp(
  `\\b(${KEYWORDS})\\s+[^\\s#\\/]*\\/[^\\s#]*#\\S*`,
  'gi'
);

// Catch keyword + invalid `#` tokens and numeric suffixes that are not valid
// reference delimiters.
const MALFORMED_PATTERN = new RegExp(
  `\\b(${KEYWORDS})\\s+#(?:(?!\\d)\\S*|\\d+(?=[^\\d])(?!${VALID_REFERENCE_END})\\S*)`,
  'gi'
);

// An HTML comment runs to `-->` or EOF, matching GitHub's rendering; an
// unclosed `<!--` hides the remaining references from reviewers.
const HTML_COMMENT_PATTERN = /<!--[\s\S]*?(?:-->|$)/g;

// Markdown reference definitions are not reviewer-visible text.
const MARKDOWN_REFERENCE_DEFINITION_PATTERN =
  /^[ \t]{0,3}\[[^\]\r\n]+\]:[^\r\n]*(?:\r?\n[ \t]+(?:"[^\r\n]*"|'[^\r\n]*'|\([^\r\n]*\)))?/gm;

// Removes non-visible Markdown constructs so only reviewer-visible text remains.
function stripHtmlComments(body) {
  return (body || '')
    .replace(HTML_COMMENT_PATTERN, '')
    .replace(MARKDOWN_REFERENCE_DEFINITION_PATTERN, '');
}

function kindFor(keyword) {
  return NON_CLOSING_KEYWORDS.split('|').includes(keyword.toLowerCase())
    ? 'non-closing'
    : 'closing';
}

function parseLinkedIssues(body) {
  const visible = stripHtmlComments(body);
  const references = [];
  const errors = [];

  for (const match of visible.matchAll(REFERENCE_PATTERN)) {
    references.push({ number: parseInt(match[2], 10), kind: kindFor(match[1]) });
  }

  for (const match of visible.matchAll(CROSS_REPO_PATTERN)) {
    errors.push({
      raw: match[0],
      reason:
        'cross-repository keyword reference; the approval gate only resolves issues in the base repository',
    });
  }

  for (const match of visible.matchAll(MALFORMED_PATTERN)) {
    errors.push({
      raw: match[0],
      reason: `malformed issue reference: expected "#<number>" after "${match[1]}"`,
    });
  }

  const kindsByNumber = new Map();
  for (const reference of references) {
    const kinds = kindsByNumber.get(reference.number) || new Set();
    kinds.add(reference.kind);
    kindsByNumber.set(reference.number, kinds);
  }
  for (const [number, kinds] of kindsByNumber) {
    if (kinds.size > 1) {
      errors.push({
        raw: `#${number}`,
        reason: `ambiguous: issue #${number} is referenced as both closing and non-closing`,
      });
    }
  }

  return { references, errors };
}

module.exports = { parseLinkedIssues, stripHtmlComments };
