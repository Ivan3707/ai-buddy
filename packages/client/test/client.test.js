import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chat } from '../index.js';

test('client uses backend and exposes failures', async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = async (url, init) => {
      assert.equal(url, '/api/chat');
      assert.deepEqual(JSON.parse(init.body), { message: 'отпуск' });
      return new Response(JSON.stringify({answer:'demo', sources:[], mode:'mock'}));
    };
    assert.equal((await chat('', 'отпуск')).mode, 'mock');
    globalThis.fetch = async () => new Response('', {status:502});
    await assert.rejects(chat('', 'test'), /502/);
  } finally { globalThis.fetch = original; }
});
