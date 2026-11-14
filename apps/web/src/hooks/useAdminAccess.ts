import { useEffect, useState } from 'react';
import { admin } from '../bot/adminApi';

/** Открыта ли админ-панель для этой роли: руководителю всегда, остальным только при ADMIN_OPEN_FOR_ALL на бэкенде (демо). */
export function useAdminAccess(token: string, role: string): boolean {
  const [allowed, setAllowed] = useState(role === 'teamlead');
  useEffect(() => {
    if (role === 'teamlead') { setAllowed(true); return; }
    let live = true;
    void admin.access(token).then(ok => { if (live) setAllowed(ok); });
    return () => { live = false; };
  }, [token, role]);
  return allowed;
}
