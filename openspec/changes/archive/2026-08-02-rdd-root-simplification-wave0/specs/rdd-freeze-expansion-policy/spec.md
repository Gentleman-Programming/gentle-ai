# RDD Freeze-Expansion Policy Specification

## Purpose

Defines the documentation-only policy governing old-facade recovery/transport work during migration: scope, security-defect acceptance criteria, evidence, and escalation.

## Requirements

### Requirement: Policy Scope, Criteria, Evidence, and Escalation

The freeze-expansion policy MUST state its scope, the criteria for a "proven security defect", the evidence required to invoke the exemption, and the escalation path, as documentation only — with no CI enforcement introduced in this change.

#### Scenario: All four elements present

- GIVEN the freeze-expansion policy document
- WHEN it is inspected
- THEN it states scope, security-defect criteria, required evidence, and escalation path
- AND it introduces no CI enforcement mechanism

### Requirement: Security-Defect Reproduction Criterion

"Proven security defect" MUST require a reproduction on current `main`, per the RDD defect workflow; a credible report alone is insufficient. (Assumption, pending maintainer confirmation — Proposal question round #3.)

#### Scenario: Report without reproduction

- GIVEN a credible security report with no current-`main` reproduction
- WHEN the policy's exemption criteria are applied
- THEN the exemption does not apply until reproduction is provided

### Requirement: Maintainer-Internal, Non-CI-Binding Scope

The policy MUST state it is maintainer-internal guidance and is not binding on external contributors, and MUST NOT be enforced by CI in this change. (Assumption, pending maintainer confirmation — Proposal question round #1.)

#### Scenario: Scope statement present

- GIVEN the policy document
- WHEN its scope section is inspected
- THEN it states maintainer-internal guidance and absence of CI enforcement
