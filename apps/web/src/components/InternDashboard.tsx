import { useCallback, useEffect, useState } from 'react';
import { getChecklists, type ChecklistsResponse } from '@ai-buddy/client';
import { ChecklistView } from './ChecklistView';
import { Assistant } from './Assistant';
export function InternDashboard({ token }: { token: string }) {
  const [data, setData] = useState<ChecklistsResponse>(); const [loading, setLoading] = useState(true); const [error, setError] = useState('');
  const refresh = useCallback(async () => { setLoading(true); setError(''); try { setData(await getChecklists('', token)); } catch (reason) { setError(reason instanceof Error ? reason.message : 'Не удалось загрузить чек-листы'); } finally { setLoading(false); } }, [token]);
  useEffect(() => { void refresh(); }, [refresh]);
  return <main className="dashboard"><div className="page-title"><div><span className="eyebrow">Мой онбординг</span><h1>Путь в команду</h1><p>Двигайтесь по двум трекам в своём темпе. Прогресс сохраняется автоматически.</p></div><button className="ghost" onClick={() => void refresh()} disabled={loading}>↻ Обновить</button></div>{loading && !data ? <div className="center compact-center"><div className="loader"/><p>Загружаем задачи…</p></div> : error && !data ? <p className="notice error">{error}</p> : data && <div className="checklist-grid"><ChecklistView checklist={data.common} token={token} refresh={refresh}/><ChecklistView checklist={data.professional} token={token} refresh={refresh}/></div>}{error && data && <p className="notice error">{error}</p>}<Assistant token={token}/></main>;
}
