import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { normalizeTask, acceptTask, startTaskReconciliation } from './taskProgressState.mjs';
const flush = () => new Promise((resolve) => setImmediate(resolve));

test('persisted and SSE payloads share result conversion, malformed JSON is safe', () => {
  assert.deepEqual(normalizeTask({ id: '1', type: 'node_add', progress: 2, result: '{"added":2}' }).result, { added: 2 });
  assert.equal(normalizeTask({ taskId: '1', result: '{' }).result, null);
});
test('fast terminal wins over hydrate and duplicate SSE; replay history stays bounded', () => {
  const ids = new Set();
  assert.equal(acceptTask(ids, { taskId: '1', status: 'completed' }), true);
  assert.equal(acceptTask(ids, { taskId: '1', status: 'running' }), false);
  assert.equal(acceptTask(ids, { taskId: '1', status: 'completed' }), false);
  for (let i = 2; i < 1100; i++) acceptTask(ids, { taskId: String(i), status: 'error' });
  assert.equal(ids.size, 1000);
});
test('hydrate, missed terminal, sequential polling and cleanup', async () => {
  let resolveRunning;
  const received = [];
  const scheduled = [];
  const cancelled = [];
  const stop = startTaskReconciliation({
    loadRunning: () =>
      new Promise((resolve) => {
        resolveRunning = resolve;
      }),
    loadTask: async (id) => ({ data: { id, status: 'completed' } }),
    activeIds: () => ['existing'],
    receive: (task) => received.push(task),
    schedule: (fn) => {
      scheduled.push(fn);
      return 7;
    },
    unschedule: (timer) => cancelled.push(timer)
  });
  assert.equal(scheduled.length, 0);
  resolveRunning({ data: [{ id: 'hydrated', status: 'running' }] });
  await flush();
  assert.deepEqual(
    received.map((task) => task.id),
    ['hydrated', 'existing']
  );
  assert.equal(scheduled.length, 1);
  stop();
  assert.deepEqual(cancelled, [7]);
});
test('disconnect retries; logout/unmount ignores late requests and creates no timers', async () => {
  let resolve;
  let count = 0;
  const stop = startTaskReconciliation({
    loadRunning: () =>
      new Promise((r) => {
        resolve = r;
      }),
    loadTask: async () => ({}),
    activeIds: () => [],
    receive: () => {
      count++;
    },
    schedule: () => {
      count++;
    },
    unschedule: () => {}
  });
  stop();
  resolve({ data: [{ id: 'late' }] });
  await flush();
  assert.equal(count, 0);
  const scheduled = [];
  const retryStop = startTaskReconciliation({
    loadRunning: async () => {
      throw Error('offline');
    },
    loadTask: async () => ({}),
    activeIds: () => [],
    receive: () => {},
    schedule: (fn) => scheduled.push(fn),
    unschedule: () => {}
  });
  await flush();
  assert.equal(scheduled.length, 1);
  retryStop();
});
test('provider gates hydration and SSE on authenticated context (not token presence)', () => {
  const source = readFileSync(new URL('./TaskProgressContext.jsx', import.meta.url), 'utf8');
  assert.match(source, /const \{ isAuthenticated \} = useAuth\(\)/);
  assert.match(source, /if \(!isAuthenticated\)/);
  assert.match(source, /if \(isAuthenticated\) handleProgressEvent/);
  assert.doesNotMatch(source, /localStorage|setInterval/);
});
