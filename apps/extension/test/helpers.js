// Test helpers: fake fetch that records calls and returns scripted responses.
const encoder = new TextEncoder();

export function sseResponse(frames, {status = 200, contentType = 'text/event-stream; charset=utf-8'} = {}) {
  const text = frames.map(([type, data]) => `event: ${type}\ndata: ${JSON.stringify(data)}\n\n`).join('');
  const bytes = encoder.encode(text);
  return new Response(new ReadableStream({
    start(controller) {
      // Split into odd-sized chunks to exercise the parser like a real network.
      for (let i = 0; i < bytes.length; i += 7) controller.enqueue(bytes.slice(i, i + 7));
      controller.close();
    },
  }), {status, headers: {'Content-Type': contentType}});
}

export function jsonResponse(body, status = 200) {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: {'Content-Type': 'application/json'},
  });
}

export function textResponse(text, status) {
  return new Response(text, {status, headers: {'Content-Type': 'text/plain; charset=utf-8'}});
}

export function fakeFetch(...responses) {
  const calls = [];
  const impl = async (url, init) => {
    calls.push({url, init, body: init?.body ? JSON.parse(init.body) : undefined});
    const next = responses.shift();
    if (!next) throw new Error(`unexpected fetch ${url}`);
    if (next instanceof Error) throw next;
    return typeof next === 'function' ? next(url, init) : next;
  };
  impl.calls = calls;
  return impl;
}
