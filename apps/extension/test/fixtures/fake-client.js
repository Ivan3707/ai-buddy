// Stand-in for packages/client in background.test.js: every call goes to globalThis.__fakeClient (set by the test).
const call = name => (...args) => globalThis.__fakeClient[name](...args);
export const getMe = call('getMe');
export const linkSession = call('linkSession');
export const login = call('login');
export const logoutSession = call('logoutSession');
export const refreshSession = call('refreshSession');
