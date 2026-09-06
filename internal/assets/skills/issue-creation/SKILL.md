---
name: issue-creation
description: "Trigger: issue creation, bug reports, feature requests, or issue approval. Create and triage GitHub issues from repository evidence."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.4"
---

# Issue Creation

## Activation Contract

Use this skill for drafting, creating, commenting on, triaging, or approving GitHub issues. Repository policy and selected YAML Issue Form are authoritative.

## Hard Rules

- Reuse verified current-session facts; discover only missing or stale facts.
- Before any needed target policy read or write, resolve the exact target as `[HOST/]OWNER/REPO`. Never assume the current repository.
- YAML Issue Forms are the single format authority. Never use Markdown, a blank body, an alternate publisher, or a browser route as a fallback.
- Complete one open-and-closed duplicate search before a write. Reuse that result while it remains current.
- Never invent facts, selections, affirmations, labels, approval, or policy; ask for the smallest missing fact.
- Create-time labels are limited to labels declared by the selected form, discovered to exist, and permitted for the actor.
- Before ANY post-publication workflow mutation, read `references/delegated-workflow-actions.md` completely and follow it. It is normative, not optional background.
- Require current direct human instruction: exact `HOST`, `REPO=OWNER/REPO`, issue/PR number/action.
- Protected policy labels are `status:approved`, `size:exception`, and any repository-defined gate-override/authorization label. For `status:approved`, require target-host evidence binding the direct instructing principal to approval authority as a repository maintainer or repository-authorized approver.
- Adding or removing a protected label requires current direct target-host-verified maintainer/authorized-approver instruction for exact add/remove plus authenticated actor target-host `viewerPermission` `MAINTAIN` or `ADMIN`; `TRIAGE` never suffices. `size:exception` also needs a documented rationale; unknown gate labels stop. Verify target-host capability; for ordinary generic actions only, `TRIAGE` permits only GitHub-granted existing-label and issue/PR close/reopen actions—not push/merge/label creation/deletion/administration.
- Reject inferred/model-authored authority; atomic `status:approved`, exactly one attempt/readback; fail closed.
- Keep all body-bearing data in private temporary files outside repositories. Do not print the contents of any protected file.
- Make one create or comment attempt with no blind retry. Classify it exactly `confirmed | no_write | unknown`; `unknown` stops every later mutation and retry.

## Decision Gates

| Path | Use when | Action |
| --- | --- | --- |
| Fast path | The current session has the exact target and form, reviewed answers and title, current labels and policy, and a completed classifiable duplicate search | Reuse them. |
| Minimal discovery | Any required fact is missing, stale, ambiguous, or belongs to another target | Resolve target, fetch only missing facts, and stop if any are unknown. |
| Conforming equivalent | A candidate read from the target host covers behavior and satisfies selected-form controls and required answers | Comment there instead of creating a duplicate. |
| Nonconforming concrete issue | A relevant candidate, read from the target host, covers the behavior but its body lacks required form information | Request that its author repair it in place; never auto-rewrite or approve it. |
| Question or triage | Policy routes questions to enabled Discussions, contact links, or review gates | Follow that route; otherwise request the smallest missing decision. |
| Post-publication workflow mutation | A current direct human instruction names one exact issue or PR action and exact target | Follow the delegated workflow action path; otherwise stop without writing. |

## Execution Steps

1. Choose the fast path or minimal discovery. When discovery is needed, derive and verify `HOST`, `REPO=OWNER/REPO`, and `TARGET=$HOST/$REPO` from an explicit target or one unambiguous authenticated remote before target reads. Authenticate to `HOST`; discover only missing repository policy, default-branch Issue Forms/config, issue availability, Discussions routing, and labels. Enumerate existing labels with one read-only call:

   ```bash
   gh api --hostname "$HOST" --paginate "repos/$REPO/labels?per_page=100" --jq '.[].name'
   ```

   Failed or ambiguous discovery stops publication.
2. Select the one YAML form whose declared purpose matches. Before duplicate classification, establish its controls and required-answer structure in declared order; omit `markdown` guidance and do not collect or materialize new-issue answers yet:

   | Control | Candidate comparison structure |
   | --- | --- |
   | `input` / `textarea` | Visible label and `validations.required` status |
   | `dropdown` | Visible label, `attributes.multiple` selection mode, declared options, required selection, and declared order |
   | `checkboxes` | Visible label, each option, and individually required status |

   Treat `dropdown.attributes.multiple: true` as multi-select; otherwise treat it as single-select. Preserve labels, emojis, option text, and values. Stop on malformed, unsupported, missing, or ambiguous required structure.
3. Create an owner-only temporary directory and `DISCOVERY_FILE`, `BODY_FILE`, `READBACK_FILE`, `PRE_READ_FILE`, plus `POST_READ_FILE` in it (`0700`/`0600`, or strict Windows ACL equivalents), and install cleanup before writing body-bearing data. Clean up all five files on every stop, signal, failure, `confirmed`, `no_write`, and `unknown` path.
4. Complete one duplicate search covering open and closed issues, unless a matching current-session result is still valid:

   ```bash
   gh issue list --repo "$TARGET" --state all --search "$QUERY" --limit 1000
   ```

   Treat saturated or otherwise incomplete results as unknown discovery; narrow read-only search or stop without writing. Read only each relevant candidate that can affect the duplicate decision from the exact target host. Capture its number, URL, title, and body in `DISCOVERY_FILE`; do not print that body-bearing data:

   ```bash
   gh issue view "$CANDIDATE_NUMBER" --repo "$TARGET" --json number,url,title,body >"$DISCOVERY_FILE"
   ```

   Require the returned candidate number and URL in `DISCOVERY_FILE` to identify `$CANDIDATE_NUMBER` in `$REPO` on `$HOST` before classification; a mismatch is `unknown`. Compare each candidate's body controls and required answers with the selected YAML form before classifying it conforming or needing repair. Unavailable body, target mismatch, incomplete data, or ambiguous classification is `unknown` and stops without writing. Reuse valid current-session candidate results; keep the read set proportional.
5. Follow the evidence-based duplicate decision. For a conforming equivalent, prepare the comment; for a nonconforming concrete issue, prepare only the in-place repair request. Only when classification selects the new-issue path, process reviewed answers and materialize the body:

   | Control | Materialized body |
   | --- | --- |
   | `input` / `textarea` | `### <visible label>` followed by the reviewed answer |
   | `dropdown` | `### <visible label>` followed by its exact selected option text; for multi-select, every exact selected option text in declared options order |
   | `checkboxes` | `### <visible label>` followed by each option as `- [x] <exact text>` or `- [ ] <exact text>` |

   Require every dropdown selection to exactly match a declared option. A required dropdown must have at least one valid selection. For multi-select, preserve every valid reviewed selection in declared options order; for single-select, preserve its one selected option. Enforce every `validations.required` field and individually required checkbox option. Require explicit user affirmation for first-person checkbox text. For `textarea.attributes.render`, fence the answer with the declared language and a fence long enough for its content. Render an unanswered optional control as `_No response_`; stop on malformed, unsupported, missing, or ambiguous required input. Review the exact target, title, selected form, materialized body or comment, and permitted discovered form labels.
6. Apply this label expansion contract to the single canonical create command:

   | Permitted labels | Command suffix |
   | --- | --- |
   | Zero | Append no `--label` tokens. |
   | Each permitted label | Append exactly one separate `--label "$PERMITTED_LABEL"` pair. |

7. Immediately before mutation, perform one practical privacy scan of the title and body for actual local paths, usernames, hostnames, credentials or secrets, private project names, and private network addresses. Replace findings with `<project-name>`, `<user>`, `<hostname>`, or `<token>` as applicable while preserving intentionally public identifiers and useful reproduction structure. Make only the applicable GitHub CLI attempt, appending `--label` tokens to the create command exactly as the step 6 label expansion contract directs:

   ```bash
   gh issue create --repo "$TARGET" --title "$TITLE" --body-file "$BODY_FILE"
   gh issue comment "$NUMBER" --repo "$TARGET" --body-file "$BODY_FILE"
   ```

8. Capture the returned target-host issue or comment identity and read it back from that host into `READBACK_FILE`. Redirect stdout from both body-bearing read-back commands:

   ```bash
   gh issue view "$NUMBER" --repo "$TARGET" --json number,url,title,body,state,labels >"$READBACK_FILE"
   gh api --hostname "$HOST" "repos/$REPO/issues/comments/$COMMENT_ID" >"$READBACK_FILE"
   ```

   For comments, require the returned comment's `issue_url` data in `READBACK_FILE` to identify issue `$NUMBER` in `$REPO` on `$HOST`; absent or mismatched parent identity is `unknown`. Validate and compare only from `READBACK_FILE`: compare issue titles exactly and bodies after only CRLF-to-LF and trailing-final-newline normalization.
9. Report exactly one result:
   - `confirmed`: a stable identity was returned and target-host read-back matches; report only labels present in read-back.
   - `no_write`: an authoritative rejection proves no issue or comment could have been created.
   - `unknown`: timeout, lost response, network/5xx ambiguity, missing identity, unavailable read-back, or mismatch leaves the write uncertain. Clean up and stop all mutations and retries.

## Output Contract

Return the exact target, selected YAML form when creating, duplicate decision when applicable, direct instruction and capability verification for a workflow action, mutation kind, stable identity and read-back labels/state when confirmed, and exactly one of `confirmed | no_write | unknown`. When stopping before mutation, name the missing fact and state that no write occurred.
