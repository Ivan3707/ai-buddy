import { assertResponse, get, internIdentity, options as defaults, pause } from './common.js';

export const options = defaults;

export default function () {
  const user = internIdentity();
  assertResponse(get('/api/auth/me', user.token), 200,
    (v) => v && v.id === user.id && v.email === user.email && v.role === 'intern', 'me');
  assertResponse(get('/api/checklists', user.token), 200,
    (v) => v && v.common?.id && v.professional?.id && Array.isArray(v.common.stages) && Array.isArray(v.professional.stages), 'checklists');
  assertResponse(get('/api/progress', user.token), 200,
    (v) => v && v.user_id === user.id && Number.isInteger(v.total_items) && v.total_items >= 2, 'progress');
  assertResponse(get('/api/hints', user.token), 200,
    (v) => Array.isArray(v), 'hints');
  pause();
}
