import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile, open, readdir, unlink } from 'node:fs/promises';
import { join } from 'node:path';
import { completeLock, completeFile } from './complete-generated-lock-sri.mjs';

const sri = `sha512-${Buffer.alloc(64, 7).toString('base64')}`;
const name = '@sample/pkg';
const url = 'https://registry.npmjs.org/@sample/pkg/-/pkg-1.2.3.tgz';
const key = 'node_modules/@sample/pkg';
const record = { version: '1.2.3', resolved: url, license: 'MIT' };
const lock = () => ({ name: 'private', version: '1.0.0', lockfileVersion: 3,
  packages: { '': { dependencies: { '@sample/pkg': '1.2.3' } }, [key]: { ...record } } });
const metadata = () => ({ name, version: '1.2.3', dist: { tarball: url, integrity: sri } });
let requests = 0;
const response = (data = metadata(), status = 200) => ({ status,
  body: new ReadableStream({ start(c) { c.enqueue(Buffer.from(JSON.stringify(data))); c.close(); } }) });
const fetcher = async (address, options) => {
  requests++;
  assert.equal(address, 'https://registry.npmjs.org/%40sample%2Fpkg/1.2.3');
  assert.equal(options.redirect, 'error');
  return response();
};
let passed = 0;
async function check(fn) { await fn(); passed++; }
async function rejects(input, options = {}) {
  const before = JSON.stringify(input);
  await assert.rejects(completeLock(input, { fetchImpl: fetcher, ...options }));
  assert.equal(JSON.stringify(input), before);
}
try {
  await check(async () => {
    const input = lock(), before = JSON.stringify(input);
    const result = await completeLock(input, { fetchImpl: fetcher });
    assert.equal(result.packages[key].integrity, sri);
    assert.equal(JSON.stringify(input), before);
    delete result.packages[key].integrity;
    assert.deepEqual(result, input);
  });
  await check(async () => {
    const input = lock(); input.packages[key].integrity = sri;
    requests = 0;
    assert.deepEqual(await completeLock(input, { fetchImpl: fetcher }), input);
    assert.equal(requests, 0);
  });
  await check(async () => {
    const input = lock(); input.packages[key].name = name;
    input.packages['node_modules/alias'] = input.packages[key]; delete input.packages[key];
    assert.equal((await completeLock(input, { fetchImpl: fetcher })).packages['node_modules/alias'].integrity, sri);
  });
  for (const mutate of [
    m => { m.name = 'other'; }, m => { m.version = '1.2.4'; },
    m => { m.dist.tarball += '?redirect'; }, m => { delete m.dist.integrity; },
    m => { m.dist.integrity = 'sha1-abc'; }, m => { m.dist.integrity = sri + ' '; },
  ]) await check(async () => {
    const m = metadata(); mutate(m);
    await rejects(lock(), { fetchImpl: async () => response(m) });
  });
  for (const mutate of [
    p => { p.resolved = 'http://registry.npmjs.org/pkg'; },
    p => { p.resolved = url + '?x'; }, p => { p.resolved = url.replace('registry.npmjs.org', 'evil.example'); },
    p => { p.version = '^1.2.3'; }, p => { p.version = '01.2.3'; },
    p => { p.name = '../pkg'; }, p => { p.link = true; },
    p => { p.integrity = ''; }, p => { p.integrity = 'sha256-abc'; },
    p => { p.integrity = `${sri} ${sri}`; },
  ]) await check(async () => { const input = lock(); mutate(input.packages[key]); await rejects(input); });
  for (const path of ['node_modules/../pkg', '/node_modules/pkg', 'node_modules/pkg/',
    'node_modules/pkg/other', 'node_modules/@sample', 'node_modules/pkg//node_modules/pkg']) {
    await check(async () => {
      const input = lock(); input.packages[path] = input.packages[key]; delete input.packages[key];
      await rejects(input);
    });
  }
  for (const status of [301, 302, 404, 500]) await check(async () => {
    await rejects(lock(), { fetchImpl: async () => response(metadata(), status) });
  });
  await check(async () => { await rejects(lock(), { maxBytes: 10 }); });
  await check(async () => {
    await rejects(lock(), { maxBytes: 10, fetchImpl: async () => ({ status: 200,
      body: new ReadableStream({ start(c) {
        c.enqueue(Buffer.alloc(6)); c.enqueue(Buffer.alloc(6)); c.close();
      } }) }) });
  });
  await check(async () => {
    const input = lock(); input.packages[`node_modules/outer/${key}`] = input.packages[key];
    delete input.packages[key];
    assert.equal((await completeLock(input, { fetchImpl: fetcher })).packages[`node_modules/outer/${key}`].integrity, sri);
  });
  await check(async () => { await rejects(lock(), { maxRequests: 0 }); });
  await check(async () => { await rejects(lock(), { maxRecords: 1 }); });
  await check(async () => {
    await rejects(lock(), { requestMs: 5, fetchImpl: () => new Promise(() => {}) });
  });
  await check(async () => {
    await rejects(lock(), { requestMs: 5, fetchImpl: async () => ({ status: 200,
      body: new ReadableStream({ start() {} }) }) });
  });
  await check(async () => {
    let ticks = 0;
    await rejects(lock(), { totalMs: 10, now: () => ticks++ * 11 });
  });
  await check(async () => {
    const input = lock(); input.packages['node_modules/second'] = {
      name, ...record, version: '1.2.4', resolved: url.replaceAll('1.2.3', '1.2.4') };
    let calls = 0;
    const dir = await mkdtemp('/acquire/sri-test-');
    const file = join(dir, 'package-lock.json');
    const original = JSON.stringify(input); await writeFile(file, original);
    await assert.rejects(completeFile(file, { fetchImpl: async () => {
      calls++; return calls === 1 ? response() : response({}, 404);
    } }));
    assert.equal(calls, 2); assert.equal(await readFile(file, 'utf8'), original);
  });
  await check(async () => {
    const dir = await mkdtemp('/acquire/sri-test-'), file = join(dir, 'package-lock.json');
    await writeFile(file, JSON.stringify(lock()));
    await completeFile(file, { fetchImpl: fetcher });
    assert.equal(JSON.parse(await readFile(file, 'utf8')).packages[key].integrity, sri);
    const completed = await readFile(file, 'utf8');
    requests = 0; await completeFile(file, { fetchImpl: fetcher });
    assert.equal(requests, 0); assert.equal(await readFile(file, 'utf8'), completed);
  });
  for (const fault of ['write', 'close', 'rename']) await check(async () => {
    const dir = await mkdtemp('/acquire/sri-fault-');
    const file = join(dir, 'package-lock.json'), original = JSON.stringify(lock());
    await writeFile(file, original);
    const failure = new Error(`injected ${fault}`);
    let closed = false;
    const io = {
      open: async (...args) => {
        const handle = await open(...args);
        return {
          writeFile: async data => {
            if (fault === 'write') {
              await handle.writeFile(data.slice(0, 17));
              throw failure;
            }
            await handle.writeFile(data);
          },
          close: async () => {
            if (!closed) { await handle.close(); closed = true; }
            if (fault === 'close') throw failure;
          },
        };
      },
      ...(fault === 'rename' ? { rename: async () => { throw failure; } } : {}),
    };
    await assert.rejects(completeFile(file, { fetchImpl: fetcher }, io), error =>
      error === failure || error.cause === failure);
    assert.equal(closed, true);
    assert.equal(await readFile(file, 'utf8'), original);
    assert.deepEqual(await readdir(dir), ['package-lock.json']);
  });
  await check(async () => {
    const dir = await mkdtemp('/acquire/sri-collision-'), file = join(dir, 'package-lock.json');
    const original = JSON.stringify(lock()), staging = `${file}.sri-${process.pid}`;
    await writeFile(file, original); await writeFile(staging, 'preexisting sentinel');
    await assert.rejects(completeFile(file, { fetchImpl: fetcher }), { code: 'EEXIST' });
    assert.equal(await readFile(file, 'utf8'), original);
    assert.equal(await readFile(staging, 'utf8'), 'preexisting sentinel');
  });
  await check(async () => {
    const dir = await mkdtemp('/acquire/sri-cleanup-'), file = join(dir, 'package-lock.json');
    const original = JSON.stringify(lock()); await writeFile(file, original);
    const initial = new Error('rename failed'), cleanup = new Error('unlink failed');
    await assert.rejects(completeFile(file, { fetchImpl: fetcher }, {
      rename: async () => { throw initial; }, unlink: async () => { throw cleanup; },
    }), error => error.cause === initial && error.errors.includes(cleanup));
    assert.equal(await readFile(file, 'utf8'), original);
    // This injected cleanup failure intentionally retains the owned stage; release it.
    await unlink(`${file}.sri-${process.pid}`);
  });
  await check(async () => {
    const dir = await mkdtemp('/acquire/sri-size-'), file = join(dir, 'package-lock.json');
    await writeFile(file, Buffer.alloc(8388609, 32));
    requests = 0; await assert.rejects(completeFile(file, { fetchImpl: fetcher }));
    assert.equal(requests, 0);
  });
  await check(async () => {
    const input = lock();
    // The last sextet has nonzero padding bits: regex-looking but noncanonical.
    input.packages[key].integrity = `sha512-${'A'.repeat(85)}B==`;
    requests = 0; await rejects(input); assert.equal(requests, 0);
  });
  await check(async () => {
    const input = lock(); input.packages['node_modules/later'] = { ...record, link: true };
    requests = 0; await rejects(input); assert.equal(requests, 0);
  });
  console.log(`CompleteGeneratedLockSRI controls passed: ${passed}`);
} catch {
  console.error(`CompleteGeneratedLockSRI controls failed after ${passed} passes`);
  process.exitCode = 1;
}
