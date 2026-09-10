# mise Toolchain Support Specification

## Purpose

Defines a repo-root `mise.toml` that mirrors the authoritative Go and Node
versions, a CI drift guard that keeps those pins honest, and mise as one
additive, documented install option alongside the existing alternatives.

## Requirements

### Requirement: Repo-Root mise Toolchain Pins

A repo-root `mise.toml` MUST pin `go` to the exact version in `go.mod`'s
`go` directive and `node` to the exact `node-version` used by `ci.yml`.

#### Scenario: mise install provisions the toolchain

- GIVEN a fresh clone of the repository with mise installed
- WHEN a contributor runs `mise install` at the repo root
- THEN mise provisions the pinned Go and Node versions with no manual steps

### Requirement: CI Drift Guard Enforces Pin Equality

A drift-check script MUST run as a step in the `unit-tests` CI job. It MUST
fail that job when `mise.toml`'s `go` value differs from `go.mod`'s `go`
directive, or when `mise.toml`'s `node` value differs from `ci.yml`'s
`node-version`. The guard MUST assert pin equality only; it MUST NOT enforce
a separate Node version-floor constraint.

#### Scenario: Go pin drifts from go.mod

- GIVEN `go.mod`'s `go` directive changes without a matching `mise.toml` update
- WHEN the `unit-tests` job runs
- THEN the drift-check step fails with an actionable message

#### Scenario: Node pin drifts from ci.yml

- GIVEN `ci.yml`'s `node-version` changes without a matching `mise.toml` update
- WHEN the `unit-tests` job runs
- THEN the drift-check step fails with an actionable message

#### Scenario: Pins match

- GIVEN `mise.toml`'s `go` and `node` values equal `go.mod`'s `go` directive
  and `ci.yml`'s `node-version` respectively
- WHEN the `unit-tests` job runs
- THEN the drift-check step passes

#### Scenario: Guard does not enforce a version floor

- GIVEN `mise.toml`'s `node` value equals `ci.yml`'s `node-version`
- WHEN the drift-check step evaluates that pair
- THEN it asserts equality only and applies no separate minimum-version check

### Requirement: mise Documented as an Additive Install Option

README.md's existing "Alternative install and scope options" `<details>`
block, `docs/quickstart.md`, and the `docs/platforms.md` matrix MUST each
list mise as one option equal to Homebrew, `go install`, and Scoop — not as
a preferred or recommended path — using the command
`mise use -g gentle-ai@latest` (the registry short name).

#### Scenario: README lists mise alongside existing options

- GIVEN the README's alternative install `<details>` block
- WHEN a reader inspects it
- THEN mise appears alongside Homebrew, `go install`, and Scoop as one equal
  option with the documented command, with no preferred-path language

#### Scenario: Quickstart and platforms docs include mise

- GIVEN `docs/quickstart.md` and the `docs/platforms.md` matrix
- WHEN a reader inspects them
- THEN each includes an entry for mise using the documented command

### Requirement: CONTRIBUTING Node Prerequisite via mise

CONTRIBUTING.md's Prerequisites section MUST name Node as a required
prerequisite and MUST point at mise as one available option for provisioning
it, not as a mandatory install method.

#### Scenario: Node prerequisite is named

- GIVEN CONTRIBUTING.md's Prerequisites section
- WHEN a reader inspects it
- THEN Node is listed as a prerequisite and mise is named as an available
  option to provision it

#### Scenario: mise is not mandated

- GIVEN CONTRIBUTING.md's Prerequisites section
- WHEN a reader inspects the mise mention
- THEN the wording does not require mise as the only install path
