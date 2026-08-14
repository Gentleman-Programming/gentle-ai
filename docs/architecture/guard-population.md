# Guard population markers

`guard:population` comments are local reviewer context for selected production guards. They name the guard family, likely drift direction, and intended population beside the relevant `if`, `switch`, or `return`:

```go
// guard:population <family> <too-tight|too-loose|fail-closed>: <legitimate population and exclusion boundary>
```

The marker lint checks only that markers use this grammar, sit immediately above a supported guard node, and use unique family identifiers. It discovers production Go files recursively under `internal/` and excludes tests.

Markers are not a registry, completeness claim, or semantic proof. They do not establish that a population description is correct or that every boundary is marked. Named behavior tests at the affected runtime boundary are authoritative; reviewers must challenge those scenarios against real inputs and failure modes.
