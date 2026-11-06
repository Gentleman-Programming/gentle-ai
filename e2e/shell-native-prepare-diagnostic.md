# Private preparation failure diagnostics

A failed native prerequisite `prepare` process retains its existing bounded
combined output in `native-prepare.stderr` at the private producer root. The
receipt is at most 4096 bytes, newly created with mode 0600, synchronized and closed. It
never overwrites an earlier receipt or follows an existing symlink.

Raw diagnostics are not added to errors, structured results or terminal output.
The original nonzero/cancellation/deadline/uncertain error remains discoverable;
a receipt-write failure is joined rather than replacing that process error.
Empty output and other operations retain their previous behavior. The source
snapshot, original installer, process controls and all limits are unchanged.
Inspect a retained receipt privately and disclose only scrubbed relevant errors.
