package cli

// review_offer_door.go is internal/cli's one door into reviewtransaction's
// offer/ReviewCore symbols for the SDD apply/verify/archive/status/continue
// surface (design.md decision 4). It is a distinct door from
// internal/sddstatus's review_door.go, scoped to the verify-success-exit
// path only: the ~30 other internal/cli files that import reviewtransaction
// (review_*.go, explicit `review` subcommands such as `review start`,
// `review status`, `review finalize`) are user-invoked commands, not
// automatic SDD apply/verify/archive paths, and stay out of this door's
// scope entirely.
//
// offerEntryHook is not yet called by any production code in this slice.
// Wiring the actual verify-success-exit call site (design.md task 4.4)
// requires a concrete internal/cli command/interface for "the SDD verify
// success exit" that this slice's investigation found genuinely
// underspecified: the only existing verify-related command,
// RunSDDVerifyValidate (sdd_verify_validate.go), is deliberately
// repo/context-free by its own design ("validates a complete report
// without touching an artifact store") and carries no --cwd/--change/
// --lineage the offer point would need; internal/assets/*/commands/
// sdd-verify.md's own routing prose ("Continue only when the native
// bounded transaction is ready_final_verification or final_verifying")
// still encodes pre-verify review gating design.md does not list for
// removal. This is recorded in apply-progress as a blocking design
// question rather than resolved by inventing an unreviewed CLI contract.
// The door is created now so the absence guard
// (review_offer_absence_guard_test.go) has its one named exemption ready
// for that future call site, following the same "ship the shape ahead of
// its caller" precedent internal/sddstatus/review_door.go documents.
var offerEntryHook = func() {}
