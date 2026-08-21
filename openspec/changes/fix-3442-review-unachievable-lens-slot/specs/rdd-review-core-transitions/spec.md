# Delta for rdd-review-core-transitions

## ADDED Requirements

### Requirement: Strict Finalize and Receipt Blocking on Unachieved Slots

`finalize` MUST fail closed if any selected lens is not admitted as completed (`ArtifactAdmissionCompleted`). An unachievable attempt record MUST NOT be admitted as a completed reviewer result and MUST NOT satisfy finalize preconditions.

#### Scenario: Finalize refused on unachieved lens slot
- GIVEN a review where one or more selected lenses has unachievable attempt records without completed admission
- WHEN `ReviewCore.finalize` is executed
- THEN finalization refuses with a typed error and no receipt is issued

#### Scenario: Finalize succeeds when all lenses are completed
- GIVEN all selected lenses have valid completed admissions
- WHEN `ReviewCore.finalize` is executed
- THEN finalization succeeds and issues the immutable receipt

### Requirement: Truthful Stop Transition on Exhausted Attempts

The next transition routing MUST map an unachieved lens slot with exhausted retry attempts to `Kind: reviewNextTransitionStop` with `ReasonCode: "unachievable_reviewer_attempt"`. Honest provider refusals and unachievable attempts MUST NOT emit `captured_artifacts_unverifiable`.

#### Scenario: Exhausted attempts transition to truthful stop reason
- GIVEN a reviewing lineage where a lens has reached the maximum attempt threshold without completed admission
- WHEN the next transition is evaluated
- THEN it emits a stop transition with `ReasonCode: "unachievable_reviewer_attempt"`

#### Scenario: Tampering or corruption triggers captured_artifacts_unverifiable
- GIVEN a captured reviewer artifact whose storage bytes fail integrity validation
- WHEN the next transition is evaluated
- THEN it emits a stop transition with `ReasonCode: "captured_artifacts_unverifiable"`
