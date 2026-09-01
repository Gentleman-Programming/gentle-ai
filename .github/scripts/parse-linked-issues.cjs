'use strict';

// Shared parser for closing issue references in PR bodies.
// Single seam used by every pr-check.yml gate that reads linked issues:
// both check-issue-reference and check-issue-approved consume this parse.

const CLOSING_REFERENCE_PATTERN = /(?:closes|fixes|resolves)\s+#(\d+)/gi;

// An HTML comment runs from `<!--` to the next `-->`, or to the end of the
// body when unclosed. The unclosed case matches how GitHub renders Markdown
// (the HTML spec closes an open comment at end of input), so everything after
// an unclosed `<!--` is invisible to reviewers and must not count.
const HTML_COMMENT_PATTERN = /<!--[\s\S]*?(?:-->|$)/g;

// Removes HTML comments so only reviewer-visible text remains.
function stripHtmlComments(body) {
  return (body || '').replace(HTML_COMMENT_PATTERN, '');
}

// Returns the linked issue numbers referenced by closing keywords in the
// visible (non-comment) PR body, in order of first appearance.
function parseLinkedIssues(body) {
  const visible = stripHtmlComments(body);
  const matches = [...visible.matchAll(CLOSING_REFERENCE_PATTERN)];
  return matches.map((m) => parseInt(m[1], 10));
}

module.exports = { parseLinkedIssues, stripHtmlComments };
