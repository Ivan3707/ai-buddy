import { useCallback, useEffect, useState } from 'react';
import { getChecklists, type ChecklistsResponse } from '@ai-buddy/client';

export type ChecklistsState = {
  data: ChecklistsResponse | undefined;
  loading: boolean;
  error: string;
  refresh: () => Promise<void>;
};

/** Загружает чек-листы стажёра и умеет перезагружать их после действий. */
export function useChecklists(token: string): ChecklistsState {
  const [data, setData] = useState<ChecklistsResponse>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setData(await getChecklists('', token));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : 'Не удалось загрузить чек-листы');
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => { void refresh(); }, [refresh]);

  return { data, loading, error, refresh };
}
