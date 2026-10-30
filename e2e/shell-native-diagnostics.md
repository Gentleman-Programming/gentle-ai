# Private native installation diagnostics

The snapshot-bound `run-with-location` client preserves the original installer result and commands. If installation fails, it retains the existing adapter stderr tail in `native-setup.stderr`, beside the private producer home.

- The receipt contains at most 4096 bytes, is created exclusively with mode0600, and never overwrites an existing path.
- Raw diagnostics are not printed or included in the public JSON result. Treat the retained file as potentially sensitive.
- Planning probes, non-failed results, and legacy `run` calls do not create receipts.
- The authenticated original source and pins are unchanged. This diagnostic does not prove installation or authorize retrying a retained producer.

Run `TestUserNativeFailureReceipt` in the existing bounded fixture with `GENTLE_PRIVATE_DIAGNOSTIC_NODE` pointing to its authenticated read-only Node binary. A missing fixture or `-short` skips this external-process check, not a passing installation result.