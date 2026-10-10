import { test } from 'node:test';
import assert from 'node:assert/strict';

// progress.ts reads the global `localStorage`, so each test installs its own before a
// fresh import (the query string defeats the module cache and resets the in-memory map).
let n = 0;
const load = (storage) => {
  Object.defineProperty(globalThis, 'localStorage', { value: storage, configurable: true, writable: true });
  return import(`../../src/lib/progress.ts?${n++}`);
};
const memoryStorage = () => {
  const m = new Map();
  return { getItem: (k) => m.get(k) ?? null, setItem: (k, v) => void m.set(k, String(v)), raw: m };
};
const throwing = {
  getItem() {
    throw new Error('denied');
  },
  setItem() {
    throw new Error('denied');
  },
};

test('saveDone writes a JSON array under fsp:done:week-NN (two digits)', async () => {
  const s = memoryStorage();
  const p = await load(s);
  p.saveDone(4, ['a', 'b']);
  p.saveDone(12, ['x']);
  assert.equal(s.raw.get('fsp:done:week-04'), '["a","b"]');
  assert.equal(s.raw.get('fsp:done:week-12'), '["x"]');
  assert.deepEqual(p.loadDone(4), ['a', 'b']);
});

test('loadDone returns [] for a missing, corrupt or non-array value', async () => {
  const s = memoryStorage();
  const p = await load(s);
  assert.deepEqual(p.loadDone(1), []);
  s.raw.set('fsp:done:week-01', '{not json');
  assert.deepEqual(p.loadDone(1), []);
  s.raw.set('fsp:done:week-01', '{"a":1}');
  assert.deepEqual(p.loadDone(1), []);
  s.raw.set('fsp:done:week-01', '["a",3,null]');
  assert.deepEqual(p.loadDone(1), ['a']);
});

test('weekComplete needs total ticked items, and a total above zero', async () => {
  const p = await load(memoryStorage());
  p.saveDone(2, ['a', 'b']);
  assert.equal(p.weekComplete(2, 3), false);
  p.saveDone(2, ['a', 'b', 'c']);
  assert.equal(p.weekComplete(2, 3), true);
  assert.equal(p.weekComplete(5, 0), false);
});

test('when storage throws, state lives in memory and nothing throws', async () => {
  const p = await load(throwing);
  assert.deepEqual(p.loadDone(4), []);
  p.saveDone(4, ['a', 'b', 'c']);
  assert.deepEqual(p.loadDone(4), ['a', 'b', 'c']);
  assert.equal(p.weekComplete(4, 3), true);
});

test('when the localStorage getter itself throws, it still works in memory', async () => {
  const p = await import(`../../src/lib/progress.ts?${n++}`);
  Object.defineProperty(globalThis, 'localStorage', {
    get() {
      throw new Error('SecurityError');
    },
    configurable: true,
  });
  p.saveDone(7, ['z']);
  assert.deepEqual(p.loadDone(7), ['z']);
});
