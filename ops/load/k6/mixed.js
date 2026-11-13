import { assertResponse, get, internIdentity, options as defaults, pause, post } from './common.js';

export const options = defaults;
let lastNoteID;
let lastEventID;

function ownRecords(records, vu) {
  return Array.isArray(records) && records.every((item) =>
    typeof item.id === 'string' && typeof item.title === 'string' &&
    (!item.title.startsWith('load-vu-') || item.title.startsWith(`load-vu-${vu}-`)));
}

export default function () {
  const user = internIdentity();
  const vu = __VU;
  const notes = assertResponse(get('/api/notes', user.token), 200,
    (v) => ownRecords(v, vu) && (!lastNoteID || v.some((n) => n.id === lastNoteID)), 'notes list');
  const events = assertResponse(get('/api/calendar/events', user.token), 200,
    (v) => ownRecords(v, vu) && (!lastEventID || v.some((e) => e.id === lastEventID)), 'events list');
  assertResponse(get('/api/notes', user.token), 200,
    (v) => ownRecords(v, vu), 'notes list repeat');
  assertResponse(get('/api/calendar/events', user.token), 200,
    (v) => ownRecords(v, vu), 'events list repeat');
  const run = __ENV.K6_RUN_ID || 'local';
  const title = `load-vu-${vu}-${run}-${__ITER}`;
  if (__ITER % 2 === 0) {
    const created = assertResponse(post('/api/notes', user.token, { title, content: 'Synthetic k6 note' }), 201,
      (v) => v && typeof v.id === 'string' && v.title === title && v.content === 'Synthetic k6 note' && v.updated_at, 'note create');
    if (created?.id) lastNoteID = created.id;
  } else {
    const start = new Date(Date.now() + 3600000);
    const end = new Date(start.getTime() + 3600000);
    const created = assertResponse(post('/api/calendar/events', user.token, {
      title, description: 'Synthetic k6 event', starts_at: start.toISOString(), ends_at: end.toISOString(),
    }), 201,
      (v) => v && typeof v.id === 'string' && v.title === title && v.description === 'Synthetic k6 event' && v.starts_at && v.ends_at, 'event create');
    if (created?.id) lastEventID = created.id;
  }
  pause();
}
