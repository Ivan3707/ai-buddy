import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate } from 'k6/metrics';

export const baseURL = __ENV.BASE_URL || 'http://backend:8080';
export const functionalSuccess = new Rate('functional_success');
export const workloadRequests = new Counter('workload_requests');
export const options = {
  vus: 1,
  duration: '10s',
  summaryTrendStats: ['avg', 'min', 'med', 'p(95)', 'p(99)', 'max'],
  thresholds: {
    'http_req_failed{phase:workload}': ['rate<0.01'],
    'http_req_duration{phase:workload}': ['p(95)<2000'],
    functional_success: ['rate>0.99'],
  },
};

export function json(response) {
  try { return response.json(); } catch (_) { return null; }
}

export function assertResponse(response, status, bodyCheck, label) {
  const body = json(response);
  const success = check(response, {
    [`${label}: status ${status}`]: (r) => r.status === status,
    [`${label}: JSON contract`]: () => bodyCheck(body),
  });
  functionalSuccess.add(success);
  return body;
}

export function get(path, token) {
  workloadRequests.add(1);
  return http.get(`${baseURL}${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
    tags: { phase: 'workload' },
  });
}

export function post(path, token, body) {
  workloadRequests.add(1);
  return http.post(`${baseURL}${path}`, JSON.stringify(body), {
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    tags: { phase: 'workload' },
  });
}

let identity;
export function internIdentity() {
  if (identity) return identity;
  if (__VU < 1 || __VU > 100) throw new Error('Prepared users support 1..100 VU');
  const password = __ENV.LOAD_USER_PASSWORD;
  if (!password) throw new Error('LOAD_USER_PASSWORD is required');
  const email = `load.intern.${String(__VU).padStart(3, '0')}@example.invalid`;
  const response = http.post(`${baseURL}/api/auth/login`, JSON.stringify({ email, password }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { phase: 'auth' },
  });
  const body = json(response);
  if (response.status !== 200 || !body || !body.access_token || body.token_type !== 'Bearer' || body.user?.email !== email || !body.user?.id) {
    throw new Error(`Login failed for VU ${__VU}: HTTP ${response.status}`);
  }
  identity = { token: body.access_token, id: body.user.id, email };
  return identity;
}

export function pause() {
  sleep(Number(__ENV.K6_PAUSE_SECONDS || 0.2));
}
