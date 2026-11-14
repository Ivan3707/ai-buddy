import { assertResponse, get, options as defaults, pause } from './common.js';

export const options = defaults;

export default function () {
  assertResponse(get('/health'), 200,
    (v) => v && v.status === 'ok' && v.mode === 'mock', 'health');
  pause();
}
