export const tokenKey = 'accessToken';

export async function readToken(storage) {
  const stored = await storage.get(tokenKey);
  return typeof stored[tokenKey] === 'string' && stored[tokenKey] ? stored[tokenKey] : '';
}

export async function saveToken(storage, token) {
  await storage.set({[tokenKey]: token});
}

export async function clearToken(storage) {
  await storage.remove(tokenKey);
}

export function isUnauthorized(error) {
  return error != null && typeof error === 'object' && error.status === 401;
}

export async function restoreSession(storage, validateToken) {
  const token = await readToken(storage);
  if (!token) return {status: 'signed-out'};

  try {
    const user = await validateToken(token);
    return {status: 'authenticated', token, user};
  } catch (error) {
    if (isUnauthorized(error)) {
      await clearToken(storage);
      return {status: 'unauthorized'};
    }
    return {status: 'unavailable', token, error};
  }
}
