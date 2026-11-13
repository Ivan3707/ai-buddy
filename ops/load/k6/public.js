import { assertResponse, get, options as defaults, pause } from './common.js';

export const options = defaults;

export default function () {
  assertResponse(get('/api/onboarding?role=backend'), 200,
    (v) => Array.isArray(v) && v.length > 0 && v.every((s) => s.id && s.title && Array.isArray(s.tasks)), 'onboarding');
  pause();
}
