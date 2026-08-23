// Cross-lane battery pi host-relay driver.
//
// Feeds one provider-issued review.capture-result collect input to the
// INSTALLED gentle-pi review-host-relay implementation: the lib/ sources are
// copied byte-identical next to this file, relocated out of node_modules only
// because Node refuses type-stripping under node_modules. The relay itself is
// the real production code path: materialize prelude -> brand-new locked-down
// print-mode `pi` subprocess in an empty scratch directory (REVIEW_HOST_RELAY_PI_ARGV)
// -> submission of the raw final bytes through the provider-owned form.
//
// The wire submission (snake_case) is decoded into the relay's typed form
// exactly as gentle-pi's status decoder does; both the current `value` object
// and the decoder-era `values` array wire forms are accepted.
//
// Input: one case config JSON path as argv[2]:
//   {
//     "capture_argument_tokens": ["--lineage=...", ...],
//     "submission": { "operation_token", "argument_tokens", "value"|"values" },
//     "gentle_ai_executable": "/abs/path/gentle-ai",
//     "pi_executable": "pi"
//   }
// Output: one JSON result line on stdout:
//   { prompt_bytes, result_bytes, submission }
import { readFileSync } from "node:fs"
import { runReviewHostRelaySlot } from "./lib/review-host-relay.ts"

interface WireSubmissionValue {
  slot: string
  domain: string
  substitution_location: number
}

interface CaseConfig {
  capture_argument_tokens: string[]
  submission: {
    operation_token: string
    argument_tokens: string[]
    value?: WireSubmissionValue
    values?: WireSubmissionValue[]
  }
  gentle_ai_executable: string
  pi_executable: string
}

const config = JSON.parse(readFileSync(process.argv[2], "utf8")) as CaseConfig
const wire = config.submission
const rows = wire.values ?? (wire.value ? [wire.value] : [])
const submission = {
  operationToken: wire.operation_token,
  argumentTokens: wire.argument_tokens,
  values: rows.map((row) => ({
    slot: row.slot,
    domain: row.domain,
    substitutionLocation: row.substitution_location,
  })),
}
const result = await runReviewHostRelaySlot({
  captureArgumentTokens: config.capture_argument_tokens,
  submission,
  gentleAiExecutable: config.gentle_ai_executable,
  piExecutable: config.pi_executable,
  environment: { ...process.env, GENTLE_PI_REVIEW_RELAY_CONTRACT: "gentle-pi.review-relay/v1" },
})
console.log(JSON.stringify({
  prompt_bytes: result.promptByteLength,
  result_bytes: result.resultByteLength,
  submission: result.submission,
}))
