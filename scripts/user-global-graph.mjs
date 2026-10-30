// The caller authenticates ALL observed package bytes and validates retainedPaths
// against the immutable lock, nonapplicability and retained-source inventory.
// This pure projection changes only comparison, never files or that authority.
export function projectGlobalGraph(observed, lock, retainedPaths) {
  const retained = new Set(retainedPaths);
  const roots = new Set(Object.keys(lock.packages[''].dependencies).map(name => `node_modules/${name}`));
  const placements = new Map();
  for (const row of observed) {
    placements.set(row?.directory, (placements.get(row?.directory) ?? 0) + 1);
  }
  return observed.filter(row => {
    if (!row || typeof row.directory !== 'string' || !row.directory.startsWith('lib/node_modules/') ||
        Object.keys(row).sort().join(',') !== 'directory,integrity,name,version') return true;
    const key = row.directory.slice('lib/'.length);
    const record = lock.packages[key];
    // Only a single exact retained placement may disappear. Relocations,
    // duplicates, roots and changed identities remain visible to the seal.
    return !(retained.has(key) && !roots.has(key) && placements.get(row.directory) === 1 &&
      record?.optional === true && row.name === (record.name ?? key.split('node_modules/').at(-1)) &&
      row.version === record.version && row.integrity === record.integrity);
  });
}
