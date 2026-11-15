import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  addTaskComment, fetchAttachment, listTaskComments,
  type ChecklistItem, type ChecklistStage, type TaskAttachment, type TaskComment,
} from '@ai-buddy/client';
import { formatDateTime, staffRoleLabel, weekLabel } from '../../lib/format';
import { COLUMN_TITLES, columnOf, nextStepLabel } from '../../lib/taskColumns';
import { PaperclipIcon } from '../icons';
import { History } from './TaskReview';
import { InternHistory } from '../staff/InternHistory';

export const MAX_FILES = 5;
export const MAX_FILE_BYTES = 5 * 1024 * 1024;

type Props = {
  item: ChecklistItem;
  stage?: Pick<ChecklistStage, 'week_start' | 'week_end' | 'title'>;
  checklistName?: string;
  token: string;
  /** Для наставника и руководителя: чью переписку по задаче открываем. Стажёр открывает свою. */
  internId?: string;
  /** Есть только у стажёра: перенос на следующий этап прямо из окна. */
  onAdvance?: (result: { text: string; url: string }) => Promise<void>;
  busy?: boolean;
  onClose: () => void;
};

const sizeText = (bytes: number) => (bytes >= 1024 * 1024 ? `${(bytes / 1024 / 1024).toFixed(1)} МБ` : `${Math.max(1, Math.round(bytes / 1024))} КБ`);
const errorText = (reason: unknown, fallback: string) => (reason instanceof Error ? reason.message : fallback);

function Attachment({ file, token }: { file: TaskAttachment; token: string }) {
  const [error, setError] = useState('');
  const download = async () => {
    setError('');
    try {
      const blob = await fetchAttachment('', token, file.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = file.filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 10_000);
    } catch (reason) { setError(errorText(reason, 'Не удалось скачать файл')); }
  };
  return (
    <span className="tm-file">
      <button type="button" onClick={() => void download()}><PaperclipIcon width={14} height={14} /> <span>{file.filename}</span></button>
      <small>{sizeText(file.size)}</small>
      {error && <small className="tm-error" role="alert">{error}</small>}
    </span>
  );
}

/**
 * Окно задачи: полное описание, перенос на следующий этап и переписка стажёра с наставником (с файлами).
 * Открывается с канбана, с главной и из раздела «Команда» у наставника.
 */
export function TaskModal({ item, stage, checklistName, token, internId, onAdvance, busy, onClose }: Props) {
  const [comments, setComments] = useState<TaskComment[] | null>(null);
  const [loadError, setLoadError] = useState('');
  const [text, setText] = useState('');
  const [files, setFiles] = useState<File[]>([]);
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState('');
  const [resultText, setResultText] = useState('');
  const [resultUrl, setResultUrl] = useState('');
  const [advanceError, setAdvanceError] = useState('');
  const picker = useRef<HTMLInputElement>(null);
  const thread = useRef<HTMLDivElement>(null);
  const column = columnOf(item);
  /** Комментарии и файлы нужны только по задаче на проверке: в работе, завершённая и недоступная — только чтение. */
  const canDiscuss = column === 'review';
  const canAdvance = Boolean(onAdvance) && item.status === 'active';

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);

  const load = useCallback(async () => {
    try {
      setComments(await listTaskComments('', token, item.id, internId));
      setLoadError('');
    } catch (reason) { setLoadError(errorText(reason, 'Не удалось загрузить комментарии')); }
  }, [token, item.id, internId]);
  useEffect(() => { setComments(null); void load(); }, [load]);
  useEffect(() => { if (thread.current) thread.current.scrollTop = thread.current.scrollHeight; }, [comments?.length]);

  const pick = (list: FileList | null) => {
    const next = [...files, ...Array.from(list ?? [])];
    if (picker.current) picker.current.value = '';
    if (next.length > MAX_FILES) { setSendError(`Не больше ${MAX_FILES} файлов в одном сообщении.`); return; }
    const big = next.find(file => file.size > MAX_FILE_BYTES);
    if (big) { setSendError(`Файл «${big.name}» больше ${MAX_FILE_BYTES / 1024 / 1024} МБ.`); return; }
    setSendError('');
    setFiles(next);
  };

  const send = async (event: FormEvent) => {
    event.preventDefault();
    if (sending || (!text.trim() && files.length === 0)) return;
    setSending(true);
    setSendError('');
    try {
      const created = await addTaskComment('', token, item.id, { body: text.trim(), files, internId });
      setComments(list => [...(list ?? []), created]);
      setText('');
      setFiles([]);
    } catch (reason) { setSendError(errorText(reason, 'Не удалось отправить сообщение')); }
    finally { setSending(false); }
  };

  const advance = async () => {
    if (!onAdvance) return;
    setAdvanceError('');
    try {
      await onAdvance({ text: resultText.trim(), url: resultUrl.trim() });
      onClose();
    } catch (reason) { setAdvanceError(errorText(reason, 'Не удалось перенести задачу')); }
  };

  const facts: [string, string][] = [
    ['Статус', COLUMN_TITLES[column]],
    ...(checklistName ? [['Чек-лист', checklistName] as [string, string]] : []),
    ...(stage ? [['Срок', weekLabel(stage)] as [string, string]] : []),
    ...(item.due_at ? [['Дедлайн', formatDateTime(item.due_at)] as [string, string]] : []),
    ...(item.estimated_minutes ? [['Оценка', `≈ ${item.estimated_minutes} мин`] as [string, string]] : []),
    ['Проверка', item.requires_review ? 'наставник' : 'не требуется'],
  ];

  return (
    <div className="dialog-backdrop tm-backdrop" onClick={onClose}>
      <section className="tm" role="dialog" aria-modal="true" aria-label={`Задача: ${item.title}`} onClick={event => event.stopPropagation()}>
        <header className="tm-head">
          <div>
            {stage && <small>{stage.title}</small>}
            <h2>{item.title}</h2>
          </div>
          <button type="button" className="dialog-close" aria-label="Закрыть" onClick={onClose}>×</button>
        </header>

        <dl className="tm-facts">
          {facts.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}
        </dl>

        <div className="tm-body">
          <div className="tm-info">
            <h3>Описание</h3>
            <p className="tm-text">{item.description || 'Описание не задано.'}</p>
            {item.expected_result && (<><h3>Ожидаемый результат</h3><p className="tm-text">{item.expected_result}</p></>)}
            {item.returned_for_rework && item.status === 'active' && (
              <div className="rework-note"><span className="rework-badge">На доработку</span>
                <p className="tm-text">{item.rework_comment || 'Наставник вернул работу без комментария.'}</p></div>
            )}

            {canAdvance && (
              <div className="tm-advance">
                <h3>Следующий этап</h3>
                {item.requires_review && (
                  <>
                    <label>Комментарий к результату
                      <textarea rows={2} value={resultText} onChange={event => setResultText(event.target.value)} placeholder="Кратко опишите, что сделано" />
                    </label>
                    <label>Ссылка на результат
                      <input type="url" value={resultUrl} onChange={event => setResultUrl(event.target.value)} placeholder="https://…" />
                    </label>
                  </>
                )}
                {advanceError && <p className="tm-error" role="alert">{advanceError}</p>}
                <button type="button" className="tm-primary" disabled={busy || (item.requires_review && !resultText.trim() && !resultUrl.trim())} onClick={() => void advance()}>
                  {busy ? 'Сохраняем…' : `${nextStepLabel(item)} →`}
                </button>
                <small>{item.requires_review ? 'Нужен комментарий или ссылка на результат. Задачу примет наставник: пока он не проверит, она будет в колонке «На проверке».' : 'Проверка не нужна: задача сразу окажется в «Завершено».'}</small>
              </div>
            )}
            {internId && item.requires_review && item.status !== 'unavailable' && <InternHistory internId={internId} itemId={item.id} token={token} />}
            {onAdvance && item.requires_review && item.status !== 'unavailable' && <History itemID={item.id} token={token} />}
          </div>

          <div className="tm-chat">
            <h3>Комментарии</h3>
            <div className="tm-thread" ref={thread} aria-live="polite">
              {loadError && <p className="tm-error" role="alert">{loadError} <button type="button" className="link-button" onClick={() => void load()}>Повторить</button></p>}
              {!comments && !loadError && <p className="muted">Загружаем…</p>}
              {comments && comments.length === 0 && <p className="muted">{canDiscuss ? 'Пока нет сообщений. Здесь стажёр и наставник обсуждают задачу и делятся файлами.' : 'Пока нет сообщений. Комментарии и файлы появятся, когда задача уйдёт на проверку.'}</p>}
              {comments?.map(comment => (
                <article key={comment.id} className={`tm-msg is-${comment.author_role}`}>
                  <header><b>{comment.author_name}</b><span>{staffRoleLabel(comment.author_role)}</span><time>{formatDateTime(comment.created_at)}</time></header>
                  {comment.body && <p>{comment.body}</p>}
                  {comment.attachments.length > 0 && <div className="tm-files">{comment.attachments.map(file => <Attachment key={file.id} file={file} token={token} />)}</div>}
                </article>
              ))}
            </div>
            {canDiscuss && <form className="tm-compose" onSubmit={event => void send(event)}>
              <textarea rows={2} aria-label="Новый комментарий" maxLength={2000} value={text} onChange={event => setText(event.target.value)} placeholder="Напишите комментарий…" />
              {files.length > 0 && (
                <ul className="tm-picked">
                  {files.map((file, index) => (
                    <li key={`${file.name}-${index}`}><PaperclipIcon width={14} height={14} /> <span>{file.name}</span> <small>{sizeText(file.size)}</small>
                      <button type="button" aria-label={`Убрать ${file.name}`} onClick={() => setFiles(list => list.filter((_, i) => i !== index))}>×</button>
                    </li>
                  ))}
                </ul>
              )}
              {sendError && <p className="tm-error" role="alert">{sendError}</p>}
              <div className="tm-compose-bar">
                <input ref={picker} type="file" multiple hidden aria-label="Выбрать файлы" onChange={event => pick(event.target.files)} />
                <button type="button" className="tm-attach" onClick={() => picker.current?.click()}><PaperclipIcon width={16} height={16} /> Прикрепить файл</button>
                <button className="tm-primary" disabled={sending || (!text.trim() && files.length === 0)}>{sending ? 'Отправляем…' : 'Отправить'}</button>
              </div>
            </form>}
          </div>
        </div>
      </section>
    </div>
  );
}
