import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createSseParser, readSseStream, StreamIdleTimeoutError } from '../lib/sse.js';

const encoder = new TextEncoder();

function parseChunks(chunks) {
  const events = [];
  const parser = createSseParser(event => events.push(event));
  for (const chunk of chunks) parser.push(chunk);
  parser.end();
  return events;
}

const sample = [
  'event: meta\ndata: {"conversation_id":"c1","message_id":"m1"}\n\n',
  'event: status\ndata: {"stage":"retrieving"}\n\n',
  'event: delta\ndata: {"text":"Отпуск — через «Заявку» 🙂"}\n\n',
  'event: done\ndata: {"finish_reason":"answered","mode":"mock"}\n\n',
].join('');

const expected = [
  {type: 'meta', data: '{"conversation_id":"c1","message_id":"m1"}'},
  {type: 'status', data: '{"stage":"retrieving"}'},
  {type: 'delta', data: '{"text":"Отпуск — через «Заявку» 🙂"}'},
  {type: 'done', data: '{"finish_reason":"answered","mode":"mock"}'},
];

test('parses a whole stream in one chunk', () => {
  assert.deepEqual(parseChunks([encoder.encode(sample)]), expected);
});

test('byte chunks split at every possible position (mid-line and mid-UTF-8)', () => {
  const bytes = encoder.encode(sample);
  for (let cut = 1; cut < bytes.length; cut += 1) {
    assert.deepEqual(parseChunks([bytes.slice(0, cut), bytes.slice(cut)]), expected, `cut at ${cut}`);
  }
});

test('one byte per chunk', () => {
  const bytes = encoder.encode(sample);
  assert.deepEqual(parseChunks([...bytes].map(byte => Uint8Array.of(byte))), expected);
});

test('CRLF and CR line endings, including CRLF split across chunks', () => {
  const crlf = sample.replaceAll('\n', '\r\n');
  for (let cut = 1; cut < crlf.length; cut += 1) {
    assert.deepEqual(parseChunks([crlf.slice(0, cut), crlf.slice(cut)]), expected, `cut at ${cut}`);
  }
  assert.deepEqual(parseChunks([sample.replaceAll('\n', '\r')]), expected);
});

test('multi-line data, comments, missing space, default type, unknown fields', () => {
  const events = parseChunks([
    '﻿: keep-alive\n',
    'id: 7\nretry: 1000\n',
    'data:first\ndata: second\n\n',
    'event: delta\ndata\n\n',
    'event: ignored-without-data\n\n',
  ]);
  assert.deepEqual(events, [
    {type: 'message', data: 'first\nsecond'},
    {type: 'delta', data: ''},
  ]);
});

test('an unterminated trailing event is discarded; a trailing CR line is processed', () => {
  assert.deepEqual(parseChunks(['event: done\ndata: {}']), []);
  assert.deepEqual(parseChunks(['data: x\n\r']), [{type: 'message', data: 'x'}]);
});

function streamOf(chunks) {
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
      controller.close();
    },
  });
}

test('readSseStream reads a ReadableStream', async () => {
  const events = [];
  await readSseStream(streamOf([sample.slice(0, 10), sample.slice(10)]), event => events.push(event), {idleTimeoutMs: 1000});
  assert.deepEqual(events, expected);
});

test('readSseStream rejects a stalled stream after the idle timeout', async () => {
  let cancelled = false;
  const stalled = new ReadableStream({
    start(controller) { controller.enqueue(encoder.encode('event: status\n')); },
    cancel() { cancelled = true; },
  });
  await assert.rejects(readSseStream(stalled, () => {}, {idleTimeoutMs: 20}), StreamIdleTimeoutError);
  assert.equal(cancelled, true);
});
