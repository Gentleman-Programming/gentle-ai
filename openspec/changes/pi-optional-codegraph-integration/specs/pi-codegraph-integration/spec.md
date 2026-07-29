# Pi CodeGraph Integration Specification

## Purpose

Define optional CodeGraph support for Pi.

## Requirements

### Requirement: Optional selection boundary

The system MUST provision Pi CodeGraph integration only when selected. Otherwise, Pi MUST NOT install CodeGraph CLI, MCP, or extension assets; modify Pi configuration; add guidance; or create an index. gentle-pi MUST remain CodeGraph-agnostic.

#### Scenario: Pi unselected

- GIVEN Pi is selected and CodeGraph is not selected
- WHEN installation completes
- THEN no owned artifact or new `.codegraph/` is created
- AND gentle-pi contains no CodeGraph content

#### Scenario: Selection records ownership

- GIVEN Pi and CodeGraph are selected
- WHEN provisioning completes successfully
- THEN ownership records identify target children

### Requirement: Selected Pi provisioning and direct verification

When selected, the system MUST install required CodeGraph CLI, MCP, and Pi extension assets to the observed Pi contract. It MUST classify children as compatible, guidance-only, unavailable, or misconfigured. Compatible children MUST have effective tools and all children MUST have lazy-init guidance. It MUST directly verify and report each classification. A parent `APPEND_SYSTEM.md` marker alone MUST fail verification.

#### Scenario: Compatible child

- GIVEN a discovered child supports the observed contract
- WHEN selected provisioning runs
- THEN the child has verified tools and lazy-init guidance
- AND verification identifies the child as compatible

#### Scenario: Marker-only evidence

- GIVEN a parent marker exists but a child lacks tools or guidance
- WHEN verification runs
- THEN verification fails for that child
- AND the marker is not accepted as capability proof

#### Scenario: Unsupported child

- GIVEN a discovered child cannot support the observed tool contract
- WHEN provisioning runs
- THEN it receives guidance without unsupported tool injection
- AND its reported classification is guidance-only or unavailable

### Requirement: Root-scoped lazy initialization

Lazy-init guidance MUST require a child handling structural work to resolve the project root, reject unsafe roots, check `<project-root>/.codegraph/` before broad exploration, and run `gentle-ai codegraph init --cwd <project-root>` once when absent and available. It MUST use CodeGraph after success and explain a failed-init or failed-use fallback.

#### Scenario: Index absent

- GIVEN a compatible child receives structural work in a valid root without `.codegraph/`
- WHEN it follows its guidance
- THEN it runs `gentle-ai codegraph init --cwd <project-root>` there before broad exploration
- AND it uses CodeGraph after initialization succeeds

#### Scenario: Init fails

- GIVEN CodeGraph initialization fails
- WHEN the child handles the structural request
- THEN it may fall back only after the failed attempt
- AND it reports the fallback reason

### Requirement: Reconciliation, idempotence, and recovery

The system MUST reconcile after gentle-pi sync, restoring missing or overwritten owned artifacts and re-verifying every child. Repeated valid provision or reconciliation MUST produce no duplicates or changes. On failure, it MUST restore affected owned configuration and report failure without verification success.

#### Scenario: Sync drift

- GIVEN sync removes an owned child artifact
- WHEN post-sync reconciliation runs
- THEN it is restored and directly re-verified

#### Scenario: Repeat provisioning

- GIVEN selected integration is already valid
- WHEN provisioning runs again
- THEN no duplicate tools, guidance, or ownership records are created

#### Scenario: Update fails

- GIVEN a later child update fails after an owned change
- WHEN recovery completes
- THEN affected owned artifacts equal their prior state
- AND the operation reports failure

### Requirement: Ownership-safe removal

The system MUST remove only Gentle-AI-owned Pi CodeGraph artifacts during uninstall or deselection. It MUST preserve user-managed and gentle-pi artifacts, restore bounded backups where applicable, and remain idempotent.

#### Scenario: User-config uninstall

- GIVEN a child contains both Gentle-AI-owned and user-managed configuration
- WHEN the integration is uninstalled
- THEN only owned artifacts are removed or restored
- AND user-managed and gentle-pi artifacts remain unchanged

#### Scenario: Repeat uninstall

- GIVEN all owned integration artifacts were removed
- WHEN uninstall runs again
- THEN it succeeds without removing unrelated artifacts
