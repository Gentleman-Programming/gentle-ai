# Delta for Bounded Review Transaction and Findings Ledger

## ADDED Requirements

### Requirement: Proportional post-candidate review

RDD MUST begin after candidate freeze. Trivial candidates MUST use structural readback with zero AI reviewers; standard risk exactly one consolidated review. Focused 4R MUST require genuine high-risk evidence, never file count or LOC alone.

#### Scenario: Trivial candidate

- GIVEN a docs-only/trivial frozen candidate
- WHEN applicability is evaluated
- THEN structural readback runs
- AND zero AI reviewers run

#### Scenario: Standard-risk candidate

- GIVEN a standard-risk frozen candidate
- WHEN review is selected
- THEN exactly one consolidated review runs
- AND no additional lens opens

#### Scenario: High-risk candidate

- GIVEN evidence of high risk
- WHEN review is selected
- THEN focused 4R runs
- AND size alone cannot produce that selection

### Requirement: Proportional verification and authorization

No useful verification MUST record N/A; cheap deterministic checks MUST run automatically. Expensive checks MUST require one consent and reuse the frozen plan. Impossible checks MUST report an evidence gap without loops. Destructive or permission-sensitive checks MUST require immediate authorization.

#### Scenario: N/A or cheap verification

- GIVEN no useful signal or a cheap deterministic check
- WHEN verification is planned
- THEN the former records N/A
- AND the latter runs automatically

#### Scenario: Long verification

- GIVEN a frozen expensive verification plan
- WHEN the user consents once
- THEN that exact plan launches
- AND resume cannot regenerate it or ask again

#### Scenario: Impossible verification

- GIVEN no credible verifier exists
- WHEN verification is evaluated
- THEN an explicit evidence gap is recorded
- AND no retry loop starts

#### Scenario: Sensitive verification

- GIVEN a destructive or permission-sensitive check
- WHEN it is ready to launch
- THEN immediate authorization is required
- AND no effect starts beforehand

### Requirement: Scoped correction and receipt reuse

Ordinary proportional review with a confirmed blocker MUST permit at most one scoped correction. An unchanged approved candidate MUST reuse one receipt across repository-policy-permitted delivery gates. Explicit Judgment Day retains its existing round budget.

#### Scenario: Correction budget is exhausted

- GIVEN a confirmed candidate-caused blocker
- WHEN one scoped correction completes
- THEN its correction budget is exhausted
- AND a second correction is rejected

#### Scenario: Delivery reuses approval

- GIVEN an approved unchanged candidate
- WHEN direct main/push, issue-less/linked PR, or emergency delivery runs
- THEN every gate validates the same receipt
- AND no reviewer reopens

### Requirement: Local-only authority surface

Native review commands MUST remain local and machine-readable. They MUST NOT expose productive-runtime, remote WorkRun, URL, token, CA, or global runtime-agent commands.

#### Scenario: Local replay remains available

- GIVEN persisted local review authority
- WHEN exact replay or receipt validation is requested
- THEN native local entry points serve it
- AND no remote runtime configuration is required

### Requirement: User-controlled safe disablement

Global user mode and an uncommitted clone-local Git-common-dir override MUST combine with `off` winning. A repository MAY disable RDD but MUST NOT force it on; another clone MUST NOT inherit it. `gentle-ai review mode enable|disable|status` and diagnostics MUST report source/effective mode. Disabled mode MUST reject starts, freeze active authority read-only, preserve tests/hooks/CI, and report `disabled/unmanaged` delivery under repository policy. Re-enable MUST permit a fresh frozen review of the current candidate whatever its authorship time, so work produced during the disabled window is recoverable without being discarded; re-enable MUST NOT carry forward, inherit, or reinstate any approval or receipt for work that was never reviewed, and MUST NOT restore retired paths. An approval MUST be content-bound: a receipt approves only the exact frozen candidate bytes and policy it was issued for, so a receipt issued before or during the disabled window can never govern different bytes.

#### Scenario: Effective mode is asymmetric and observable

- GIVEN global mode is off or one clone records an off override
- WHEN mode status and diagnostics run
- THEN effective mode is off and no repository can force on
- AND another clone does not inherit the override

#### Scenario: Disable and re-enable preserve authority

- GIVEN candidates created before and after re-enable
- WHEN review or delivery is attempted
- THEN active authority stays read-only and disabled work stays unmanaged
- AND unreviewed work never reports an approval while tests, hooks, and CI remain active

#### Scenario: Re-enable recovers work authored while disabled

- GIVEN RDD was disabled, the user kept working, and RDD is re-enabled
- WHEN a review starts on the current candidate
- THEN the candidate is frozen and reviewed now regardless of when it was authored
- AND it reaches a fresh receipt content-bound to exactly those frozen bytes

#### Scenario: Re-enable never inherits an approval across the disabled window

- GIVEN a receipt issued before the disabled window and bytes changed while disabled
- WHEN delivery or verification is attempted after re-enable
- THEN the earlier receipt does not govern the changed bytes because it binds other content
- AND a candidate that has not completed a review yields no receipt at all
