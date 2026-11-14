import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chat, getBuddies, listCorporateTools, getFeedback, submitFeedback, updateEvent, ApiError } from '../index.js';

test('client uses backend and exposes failures', async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = async (url, init) => {
      assert.equal(url, '/api/chat');
      assert.deepEqual(JSON.parse(init.body), { message: 'отпуск' });
      assert.equal(init.headers.Authorization, 'Bearer test-token');
      return new Response(JSON.stringify({answer:'demo', sources:[], mode:'mock'}));
    };
    assert.equal((await chat('', 'отпуск', 'test-token')).mode, 'mock');
    globalThis.fetch = async () => new Response('', {status:502});
    await assert.rejects(chat('', 'test'), /502/);
  } finally { globalThis.fetch = original; }
});

test('Web endpoints preserve backend errors and request methods', async () => {
  const original = globalThis.fetch;
  const seen = [];
  try {
    globalThis.fetch = async (url, init) => {
      seen.push([url, init.method]);
      if (url === '/api/checklists/checklist/feedback' && init.method === 'POST') return new Response(JSON.stringify({error:'feedback already submitted'}), {status:409});
      if (url === '/api/staff/buddies') return new Response(JSON.stringify([{id:'buddy'}]));
      if (url === '/api/corporate/tools') return new Response(JSON.stringify([{tool:'repository',allowed:false}]));
      return new Response(JSON.stringify({id:'event'}));
    };
    assert.equal((await getBuddies('', 'token'))[0].id, 'buddy');
    assert.equal((await listCorporateTools('', 'token'))[0].allowed, false);
    assert.equal((await getFeedback('', 'token', 'checklist')).id, 'event');
    await assert.rejects(submitFeedback('', 'token', 'checklist', {rating:5,comment:''}), error => error instanceof ApiError && error.status === 409);
    await updateEvent('', 'token', 'event', {title:'Meeting',description:'',starts_at:'2026-11-13T10:00:00Z',ends_at:'2026-11-13T11:00:00Z'});
    assert.deepEqual(seen, [['/api/staff/buddies','GET'],['/api/corporate/tools','GET'],['/api/checklists/checklist/feedback','GET'],['/api/checklists/checklist/feedback','POST'],['/api/calendar/events/event','PUT']]);
  } finally { globalThis.fetch = original; }
});

test('401 and 403 remain distinct and 401 signals session refresh', async () => {
  const originalFetch = globalThis.fetch;
  const originalDispatch = globalThis.dispatchEvent;
  let signals = 0;
  try {
    globalThis.dispatchEvent = event => { if (event.type === 'ai-buddy:unauthorized') signals++; return true; };
    globalThis.fetch = async () => new Response(JSON.stringify({error:'unauthorized'}), {status:401});
    await assert.rejects(getBuddies('', 'token'), error => error instanceof ApiError && error.status === 401);
    assert.equal(signals, 1);
    globalThis.fetch = async () => new Response(JSON.stringify({error:'forbidden'}), {status:403});
    await assert.rejects(getBuddies('', 'token'), error => error instanceof ApiError && error.status === 403);
    assert.equal(signals, 1);
  } finally { globalThis.fetch = originalFetch; globalThis.dispatchEvent = originalDispatch; }
});
