import { test } from 'node:test';
import assert from 'node:assert/strict';
import { chat, getBuddies, listCorporateTools, getFeedback, submitFeedback, updateEvent, listTaskComments, addTaskComment, fetchAttachment, ApiError } from '../index.js';

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
    globalThis.dispatchEvent = event => { if (event.type === 'ai-buddy:unauthorized') { assert.equal(event.detail.token, 'token'); signals++; } return true; };
    globalThis.fetch = async () => new Response(JSON.stringify({error:'unauthorized'}), {status:401});
    await assert.rejects(getBuddies('', 'token'), error => error instanceof ApiError && error.status === 401);
    assert.equal(signals, 1);
    globalThis.fetch = async () => new Response(JSON.stringify({error:'forbidden'}), {status:403});
    await assert.rejects(getBuddies('', 'token'), error => error instanceof ApiError && error.status === 403);
    assert.equal(signals, 1);
  } finally { globalThis.fetch = originalFetch; globalThis.dispatchEvent = originalDispatch; }
});

test('task comments: multipart upload without a manual Content-Type, query for staff, blob download', async () => {
  const original = globalThis.fetch;
  const seen = [];
  try {
    globalThis.fetch = async (url, init) => {
      seen.push({ url, method: init.method, headers: init.headers, body: init.body });
      if (url.startsWith('/api/attachments/')) return new Response('file-bytes', { headers: { 'Content-Type': 'application/pdf' } });
      if (init.method === 'POST' && init.body.get('body') === 'big') return new Response(JSON.stringify({ error: 'Файл слишком большой' }), { status: 413 });
      return new Response(JSON.stringify(init.method === 'POST' ? { id: 'c1', attachments: [] } : []), { status: init.method === 'POST' ? 201 : 200 });
    };
    assert.deepEqual(await listTaskComments('', 'token', 'item'), []);
    await listTaskComments('', 'token', 'item', 'intern id');
    const file = new File(['abc'], 'a.txt', { type: 'text/plain' });
    assert.equal((await addTaskComment('', 'token', 'item', { body: 'hi', files: [file], internId: 'intern' })).id, 'c1');
    const post = seen[2];
    assert.ok(post.body instanceof FormData);
    assert.equal(post.headers['Content-Type'], undefined);
    assert.equal(post.headers.Authorization, 'Bearer token');
    assert.equal(post.body.get('body'), 'hi');
    assert.equal(post.body.get('intern_id'), 'intern');
    assert.equal(post.body.getAll('files')[0].name, 'a.txt');
    assert.equal(seen[1].url, '/api/checklist-items/item/comments?intern_id=intern%20id');
    await assert.rejects(addTaskComment('', 'token', 'item', { body: 'big' }), error => error instanceof ApiError && error.status === 413 && /слишком/.test(error.message));
    const blob = await fetchAttachment('', 'token', 'att');
    assert.equal(await blob.text(), 'file-bytes');
    assert.equal(blob.type, 'application/pdf');
  } finally { globalThis.fetch = original; }
});

test('teamlead checklist edits, staff history and demo completion use the documented routes', async () => {
  const original = globalThis.fetch;
  const seen = [];
  try {
    globalThis.fetch = async (url, init) => {
      seen.push([url, init.method, init.body ? JSON.parse(init.body) : undefined]);
      if (init.method === 'PATCH' || init.method === 'DELETE') return new Response(null, {status:204});
      if (url.endsWith('/complete-checklists')) return new Response(JSON.stringify({intern_id:'i', items_completed:3, submissions_approved:1}));
      return new Response(JSON.stringify([{id:'s', status:'rejected', reviewed_by:'b', review_comment:'redo'}]));
    };
    const { updateInternTask, removeInternTask, getInternSubmissionHistory, adminCompleteChecklists } = await import('../index.js');
    assert.equal(await updateInternTask('', 'token', 'i', 't', {title:'New'}), undefined);
    assert.equal(await removeInternTask('', 'token', 'i', 't'), undefined);
    assert.equal((await getInternSubmissionHistory('', 'token', 'i', 't'))[0].review_comment, 'redo');
    assert.equal((await adminCompleteChecklists('', 'token', 'i')).items_completed, 3);
    assert.deepEqual(seen, [
      ['/api/teamlead/interns/i/checklist-items/t', 'PATCH', {title:'New'}],
      ['/api/teamlead/interns/i/checklist-items/t', 'DELETE', undefined],
      ['/api/staff/interns/i/checklist-items/t/submissions', 'GET', undefined],
      ['/api/admin/accounts/i/complete-checklists', 'POST', undefined],
    ]);
  } finally { globalThis.fetch = original; }
});

test('startItem posts to the start route of the task', async () => {
  const original = globalThis.fetch;
  const seen = [];
  try {
    globalThis.fetch = async (url, init) => { seen.push([url, init.method, init.headers.Authorization]); return new Response(null, { status: 204 }); };
    const { startItem } = await import('../index.js');
    await startItem('', 'token', 'item/1');
    assert.deepEqual(seen, [['/api/checklist-items/item%2F1/start', 'POST', 'Bearer token']]);
  } finally { globalThis.fetch = original; }
});
