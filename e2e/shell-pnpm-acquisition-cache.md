# Publish runtime payload, not npm acquisition cache

Native PNPM publication excludes exactly the producer's `home/.npm` and
`pnpm/store/v11/files` acquisition-cache directories.
That private acquisition cache is not an installed dependency: both final
launchers select `data/cache` through `NPM_CONFIG_CACHE`. Runtime packages in
`home/.gentle-shell`, `.local`, all runtime PNPM files, configuration and other HOME
entries remain independently copied and verified.

The original producer is never pruned. Its complete before/after inventory
still includes the cache; all source preservation checks remain required.
Staging refuses a surviving symlink into the excluded subtree rather than
publishing a dangling runtime reference. A non-directory at the exact cache
path is also refused. Similarly named and nested HOME paths are not excluded.

Qualified checks cover cache absence, exact non-cache HOME identities, source
and receipt preservation, final manifest/seal, canonical preferences and both
launchers. This is limited to native PNPM staging; legacy npm, Shared and resume
paths keep their previous behavior. No resource or execution limit changes.

The `shell_pnpm_publication` fixture regressions prepare a destination through
existing staging/manifest primitives with a real `/qualified-supervisor`, then
run both ordinary launchers in a separate boundary with producer/build mounts
hidden. Refusal cases use that disposable destination as their source and
restore it; they never mutate the retained original producer. All fixture
regressions skip in short mode and require explicit qualified mount setup.

PNPM11's executable global virtual store is `pnpm/store/v11/links`, not the
content-addressed `files` cache. Keep `links`, `projects`, `index.db` and all
other store paths. The retained fixture's `files` cache duplicates about 301 MB
through shared source inodes; runtime publication still uses independent copies.
Source before/after inventories include both caches. Surviving links into either
omitted subtree are refused; both final launchers and canonical consumers must
work with producer/build mounts hidden before qualifying the change.
