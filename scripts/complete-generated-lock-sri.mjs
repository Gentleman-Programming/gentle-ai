import { readFile, open, rename, unlink, stat } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

const registry = 'https://registry.npmjs.org/';
const component = '[a-z0-9][a-z0-9._-]*';
const packageName = new RegExp(`^(?:@${component}/)?${component}$`);
const packagePath = new RegExp(`^node_modules/(?:@${component}/)?${component}(?:/node_modules/(?:@${component}/)?${component})*$`);
const versionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$/;
function requireValid(condition) {
  if (!condition) throw new Error('CompleteGeneratedLockSRI rejected data');
}
function validSRI(value) {
  return typeof value === 'string' && /^sha512-[A-Za-z0-9+/]{86}==$/.test(value) &&
    Buffer.from(value.slice(7), 'base64').toString('base64') === value.slice(7);
}
function identity(path, entry) {
  requireValid(packagePath.test(path) && entry && typeof entry === 'object' &&
    !Array.isArray(entry) && !Object.hasOwn(entry, 'link'));
  const suffix = path.split('node_modules/').at(-1);
  const name = Object.hasOwn(entry, 'name') ? entry.name : suffix;
  requireValid(typeof name === 'string' && name.length <= 214 && packageName.test(name));
  requireValid(name.split('/').every(part => part !== '.' && part !== '..'));
  const version = entry.version;
  requireValid(typeof version === 'string' && version.length <= 128 && versionPattern.test(version));
  const pre = versionPattern.exec(version)[4];
  requireValid(!pre || pre.split('.').every(part => !/^\d+$/.test(part) || part === '0' || !part.startsWith('0')));
  const base = name.split('/').at(-1);
  const tarball = `${registry}${name}/-/${base}-${version}.tgz`;
  requireValid(entry.resolved === tarball);
  if (Object.hasOwn(entry, 'integrity')) requireValid(validSRI(entry.integrity));
  return { name, version, tarball };
}

export async function completeLock(input, {
  fetchImpl = globalThis.fetch, now = Date.now, maxBytes = 65536,
  requestMs = 10000, totalMs = 120000, maxRequests = 256, maxRecords = 4096,
} = {}) {
  const started = now();
  const remaining = () => totalMs - (now() - started);
  requireValid(input && input.lockfileVersion === 3 && input.packages &&
    typeof input.packages === 'object' && !Array.isArray(input.packages) && input.packages['']);
  const records = Object.entries(input.packages);
  requireValid(records.length <= maxRecords);
  // Validate the COMPLETE closure before any metadata request, preserving all fields.
  const missing = [];
  for (const [path, entry] of records) {
    requireValid(remaining() > 0);
    if (path === '') continue;
    const locked = identity(path, entry);
    if (!Object.hasOwn(entry, 'integrity')) missing.push([path, locked]);
  }
  requireValid(missing.length <= maxRequests);
  const output = structuredClone(input);
  for (const [path, locked] of missing) {
    const budget = Math.min(requestMs, remaining());
    requireValid(budget > 0);
    const controller = new AbortController();
    let timer;
    const deadline = new Promise((_, reject) => {
      timer = setTimeout(() => { controller.abort(); reject(new Error('metadata deadline')); }, budget);
    });
    let reader;
    try {
      const operation = async () => {
        const response = await fetchImpl(`${registry}${encodeURIComponent(locked.name)}/${encodeURIComponent(locked.version)}`,
          { redirect: 'error', signal: controller.signal, headers: { accept: 'application/json' } });
        requireValid(response.status === 200 && response.body);
        reader = response.body.getReader();
        const chunks = [];
        let bytes = 0;
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          bytes += value.byteLength;
          requireValid(bytes <= maxBytes && remaining() > 0);
          chunks.push(Buffer.from(value));
        }
        const metadata = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks)));
        requireValid(metadata.name === locked.name && metadata.version === locked.version &&
          metadata.dist?.tarball === locked.tarball && validSRI(metadata.dist?.integrity));
        return metadata.dist.integrity;
      };
      output.packages[path].integrity = await Promise.race([operation(), deadline]);
      requireValid(remaining() > 0);
    } finally {
      clearTimeout(timer);
      controller.abort();
      if (reader) void reader.cancel().catch(() => {});
    }
  }
  requireValid(remaining() > 0);
  return output;
}

export async function completeFile(file, options, {
  open: openStage = open, rename: commitStage = rename, unlink: removeStage = unlink,
} = {}) {
  requireValid((await stat(file)).size <= 8388608);
  const raw = await readFile(file, 'utf8');
  requireValid(Buffer.byteLength(raw) <= 8388608);
  const input = JSON.parse(raw);
  const output = await completeLock(input, options);
  if (JSON.stringify(input) === JSON.stringify(output)) return;
  // The private acquisition directory has a single writer, not a hostile same UID.
  // Acquire ownership before entering cleanup: EEXIST must never remove a sentinel.
  const staging = `${file}.sri-${process.pid}`;
  const handle = await openStage(staging, 'wx', 0o600);
  let closed = false;
  try {
    await handle.writeFile(`${JSON.stringify(output, null, 2)}\n`);
    await handle.close();
    closed = true;
    await commitStage(staging, file);
  } catch (initial) {
    const failures = [initial];
    // Retry a failed close before attempting to release only our owned stage.
    if (!closed) {
      try { await handle.close(); } catch (error) { failures.push(error); }
    }
    try { await removeStage(staging); } catch (error) { failures.push(error); }
    if (failures.length > 1) {
      throw new AggregateError(failures, 'staging failed; cleanup also failed', { cause: initial });
    }
    throw initial;
  }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    requireValid(process.argv.length === 3);
    await completeFile(process.argv[2]);
    console.log('CompleteGeneratedLockSRI validated complete lock');
  } catch {
    console.error('CompleteGeneratedLockSRI failed closed');
    process.exitCode = 1;
  }
}
