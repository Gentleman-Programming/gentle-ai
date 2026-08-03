# RDD Backlog Disposition Specification

## Purpose

Defines the classification vocabulary and baseline for open RDD-related review/SDD issues and PRs, without proposing closures.

## Requirements

### Requirement: Fixed Classification Vocabulary

The backlog baseline MUST classify each item using exactly one of: `superseded-by-design`, `absorbed-into-wave-N`, `still-valid-fix-now`, `orthogonal`, or the fallback `unclassified — needs triage`.

#### Scenario: Item receives a valid label

- GIVEN any classified backlog item
- WHEN its label is inspected
- THEN it is one of the five defined labels, with `wave-N` in `absorbed-into-wave-N` set to a concrete wave number

### Requirement: Coverage-Map-Bound Baseline Completeness

The baseline MUST classify every issue and PR named in the design's coverage map and the audit's issue map; any item not classifiable from those two documents MUST be marked `unclassified — needs triage`.

#### Scenario: Item present in coverage map or audit

- GIVEN an issue or PR named in the design's coverage map or the audit's issue map
- WHEN the baseline is built
- THEN the item receives a classification or the `unclassified` fallback — never an omission

#### Scenario: Item outside both sources

- GIVEN an issue or PR not named in either source
- WHEN the baseline is built
- THEN it is out of scope for this baseline and is not assigned a fabricated classification

### Requirement: Classification-Only, No Closure Proposals

The baseline MUST NOT propose issue or PR closures or relabeling; it produces classification only. (Assumption, pending maintainer confirmation — Proposal question round #2.)

#### Scenario: Baseline output inspected

- GIVEN the completed backlog baseline
- WHEN it is inspected
- THEN it contains no closure or relabel recommendation for any item
