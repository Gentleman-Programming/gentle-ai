# Bootstrap package launchers

The private Node bootstrap uses regular `npm` and `npx` shell launchers that execute the sibling Node binary and unchanged canonical npm package entrypoints. Simply dereferencing the publisher links would relocate JavaScript files and break their relative imports.

The archive pins, Node bytes, canonical npm package, all-regular layout and publication behavior remain unchanged. Both launchers are probed before the inventory is generated; helper output and refusal messages remain unchanged for existing cases.

`TestBootstrapPackageLaunchers` uses the authenticated archive supplied through `GENTLE_SHELL_NODE_ARCHIVE`. It compares Node and canonical entry/library bytes with the archive, then executes both launchers through PATH after publication. Missing archive, non-Linux execution or short mode skips that integration test; this is not native macOS qualification or complete Gentle Shell acceptance.