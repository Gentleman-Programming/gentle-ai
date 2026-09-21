# CI policy

`scripts/merge-blocking-status-contexts.json` is the source-controlled list of
merge-blocking GitHub Actions status contexts. It includes the exact Darwin
entry `{"context":"Darwin Runtime","integration_id":15368}`.

`.github/workflows/policy-drift.yml` compares that list with active ruleset
`13932547`. It performs only authenticated `GET` requests and fails closed:

- `policy_drift` (exit 1) means the readable ruleset differs from the manifest.
- `policy_unverifiable` (exit 2) means authentication, API availability, rate
  limits, response schema, or active-ruleset discovery prevented verification.

The workflow runs from trusted base code on `pull_request_target`. It reads a
pull request's manifest through the Contents API as data; it never checks out
or executes fork code. Configure the repository secret `RULESET_READ_TOKEN`
with a GitHub App or equivalent credential limited to repository ruleset
administration read access. The checker never creates, updates, or deletes a
ruleset. Adding `Darwin Runtime` to the live ruleset remains a maintainer-owned
manual action.
