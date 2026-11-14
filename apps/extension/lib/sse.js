// Minimal Server-Sent Events parser for fetch() streams.
// EventSource cannot send POST bodies or the Authorization header, so the widget
// reads the response body itself. Chunks may be split anywhere: in the middle of a
// line, of a "\r\n" pair or of a multi-byte UTF-8 character.

export function createSseParser(onEvent) {
  const decoder = new TextDecoder('utf-8');
  let buffer = '';
  let started = false;
  let eventType = '';
  let dataLines = [];

  function dispatch() {
    if (dataLines.length > 0) {
      onEvent({type: eventType || 'message', data: dataLines.join('\n')});
    }
    eventType = '';
    dataLines = [];
  }

  function processLine(line) {
    if (line === '') {
      dispatch();
      return;
    }
    if (line.startsWith(':')) return; // comment / keep-alive
    const colon = line.indexOf(':');
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? '' : line.slice(colon + 1);
    if (value.startsWith(' ')) value = value.slice(1);
    if (field === 'event') eventType = value;
    else if (field === 'data') dataLines.push(value);
    // "id" and "retry" are irrelevant for a single POST response.
  }

  function feed(text) {
    if (!started && text.length > 0) {
      started = true;
      if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
    }
    buffer += text;
    let start = 0;
    for (let i = 0; i < buffer.length; i += 1) {
      const char = buffer[i];
      if (char !== '\n' && char !== '\r') continue;
      if (char === '\r' && i + 1 === buffer.length) break; // maybe "\r\n" split across chunks
      processLine(buffer.slice(start, i));
      if (char === '\r' && buffer[i + 1] === '\n') i += 1;
      start = i + 1;
    }
    buffer = buffer.slice(start);
  }

  return {
    push(chunk) {
      feed(typeof chunk === 'string' ? chunk : decoder.decode(chunk, {stream: true}));
    },
    end() {
      feed(decoder.decode());
      if (buffer.endsWith('\r')) processLine(buffer.slice(0, -1));
      // Per the SSE spec an event without the terminating blank line is discarded.
      buffer = '';
      eventType = '';
      dataLines = [];
    },
  };
}

export class StreamIdleTimeoutError extends Error {
  constructor() {
    super('stream idle timeout');
    this.name = 'StreamIdleTimeoutError';
  }
}

// Reads a ReadableStream<Uint8Array> and calls onEvent({type, data}) for every event.
// idleTimeoutMs aborts a stream that stops sending bytes (the backend hangs).
export async function readSseStream(stream, onEvent, {idleTimeoutMs = 0} = {}) {
  const parser = createSseParser(onEvent);
  const reader = stream.getReader();
  try {
    for (;;) {
      const {done, value} = await readWithTimeout(reader, idleTimeoutMs);
      if (done) break;
      parser.push(value);
    }
    parser.end();
  } finally {
    reader.releaseLock();
  }
}

function readWithTimeout(reader, timeoutMs) {
  if (!timeoutMs) return reader.read();
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => {
      reject(new StreamIdleTimeoutError()); // settle first: cancel() resolves the pending read
      reader.cancel().catch(() => {});
    }, timeoutMs);
  });
  return Promise.race([reader.read(), timeout]).finally(() => clearTimeout(timer));
}
