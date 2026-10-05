# AI-Assisted Contribution Policy

AI-assisted contributions are permitted. The human contributor must understand, review, validate, and take full responsibility for everything they submit.

## Human Responsibility

The human contributor remains fully responsible for:

- The security, correctness, and ongoing maintenance of the complete submission.
- Reviewing and validating every change, claim, and test result.
- Ensuring appropriate licensing and confidence in the provenance of submitted material.
- Explaining and defending the design, implementation, and tradeoffs during review.

AI assistance does not transfer authorship, accountability, or legal responsibility away from the contributor.

## Disclosure

Disclose material AI assistance used to produce or substantively review any part of a contribution, including:

- Code, tests, or documentation.
- Designs, prompts, skills, schemas, or workflows.
- Substantive review, investigation, or analysis.

For material assistance, the pull request declaration must state:

1. The tool or model, if known.
2. The material scope of the assistance.
3. The verification the contributor performed.

Raw prompts and private conversation logs are not required by default.

Trivial formatting, spelling corrections, minor autocomplete, search or navigation, and trivial, non-substantive mechanical transformations do not require disclosure.

## Review and Attribution

Maintainers may request an explanation, prompt summary, provenance information, supporting evidence, or additional tests. They may reject work that the contributor cannot explain, verify, or defend.

AI tools must not receive human attribution, including `Co-Authored-By`, `Reviewed-by`, `Tested-by`, `Signed-off-by`, approval, or equivalent credit. An optional `Assisted-by` trailer may be accepted, but the pull request declaration is sufficient.

## Advisory Review and RDD

Assistance used to produce a contribution is distinct from AI-assisted advisory review and receipt-driven development (RDD). Although materially substantive AI-assisted advisory review remains subject to the disclosure rules above, it remains distinct from contribution authorship. Automated review or RDD receipts do not replace human authorship, provenance, consent, testing, or legal responsibility.

## Submission Quality

Review is based on observable submission quality, not on whether text or code appears to be AI-generated. Before proposing a fix, contributors should identify the underlying cause and the responsible invariant, then explain and defend why the change is proportionate. Prefer the smallest change that restores that invariant without adding duplicate authority, unnecessary abstractions, or unrelated complexity. This does not require broad or architectural work when a focused fix is sufficient.

Unacceptable behavior includes:

- Submitting output that the contributor has not reviewed.
- Making claims that cannot be verified or reporting results that did not occur.
- Inventing APIs, paths, behavior, evidence, or test results.
- Masking a symptom, shifting the failure elsewhere, or leaving the responsible invariant broken.
- Adding duplicate authority, unnecessary abstractions, or unrelated complexity that creates likely regressions.
- Including broad or unrelated changes outside the approved scope.
- Copying output without confidence in its provenance or license compatibility.
- Being unable to explain the change, its design, or its consequences.
- Delegating the work of understanding, validating, or repairing the submission back to maintainers.

## Enforcement

For now, maintainers enforce this policy through reviewer judgment and documented review decisions only. The project does not use automated AI detection or an automated disclosure gate.
