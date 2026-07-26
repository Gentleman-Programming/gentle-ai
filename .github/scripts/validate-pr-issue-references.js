'use strict';

const fs = require('node:fs');

const HTML_COMMENT_PATTERN = /<!--[\s\S]*?-->/g;
const CLOSING_REFERENCE_PATTERN = /(^|[^A-Za-z0-9_])(?:closes|fixes|resolves)\s+#(\d+)\b/gi;

/**
 * Remove HTML comments from a Markdown string.
 *
 * @param {string} markdown
 * @returns {string}
 */
function stripHtmlComments(markdown) {
  return markdown.replace(HTML_COMMENT_PATTERN, '');
}

/**
 * Extract issue numbers from visible closing references in a PR body.
 *
 * @param {string} body
 * @returns {number[]}
 */
function extractLinkedIssueNumbers(body) {
  const visibleBody = stripHtmlComments(body);
  const numbers = [];
  for (const match of visibleBody.matchAll(CLOSING_REFERENCE_PATTERN)) {
    // group 1 is the leading boundary; group 2 is the issue number
    numbers.push(Number.parseInt(match[2], 10));
  }
  return numbers;
}

module.exports = {
  extractLinkedIssueNumbers,
  stripHtmlComments,
};

if (require.main === module) {
  const body = process.argv.length > 2
    ? process.argv.slice(2).join(' ')
    : fs.readFileSync(0, 'utf8');

  process.stdout.write(`${JSON.stringify(extractLinkedIssueNumbers(body))}\n`);
}
