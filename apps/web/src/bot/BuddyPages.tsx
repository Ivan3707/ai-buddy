import type { User } from '@ai-buddy/client';
import { MentorDesk } from './MentorDesk';
import './bot.css';

/** /chat: what Бади is, where to find him and how to install the Chrome extension. */
export function BuddyInfoPage({ user }: { user: User }) {
  const staff = user.role !== 'intern';
  return <main className="bot-page">
    <header className="bot-page-head">
      <div><span className="eyebrow">AI-наставник · демо</span><h1>Бади</h1>
        <p className="muted">Бади живёт в правом нижнем углу каждой страницы. Нажмите на него, чтобы открыть чат.
          Он отвечает по базе знаний со ссылками на документы, честно говорит о противоречиях, оформляет заявки после
          вашего подтверждения и передаёт вопрос наставнику, если ответа в базе нет. Данные синтетические.</p></div>
    </header>

    <section className="panel">
      <h2>Окно чата</h2>
      <ul className="bot-info-list">
        <li><b>⤢ / ⤡</b> — компактный режим и полноэкранный. Размер окна в каждом режиме можно поменять, потянув за угол: браузер его запомнит.</li>
        <li><b>Источники</b> под ответом открывают документ прямо в окне Бади, с подсветкой фрагмента. Кнопка «Открыть документ полностью» ведёт в <a href="/wiki/index.html" target="_blank" rel="noreferrer">вики компании (демо)</a>.</li>
        <li><b>Бади можно перетащить</b> в любое место экрана, позиция сохранится.</li>
        <li>Бади сам подсказывает: если задача идёт дольше плана, если заявку согласовали и если наставник ответил на вопрос.</li>
      </ul>
    </section>

    <section className="panel">
      <span className="eyebrow">Chrome-расширение</span>
      <h2>Бади на любом рабочем сайте</h2>
      <p className="muted">С расширением Бади появляется и в вики, Grafana, Jira, GitLab: подсказывает по открытой странице и проверяет выделенный текст по базе знаний.
        На этом сайте его заменяет Бади из расширения — персонаж один и тот же.</p>
      <ol className="bot-info-list">
        <li>Откройте <code>chrome://extensions</code> и включите «Режим разработчика» (справа вверху).</li>
        <li>Нажмите «Загрузить распакованное расширение» и выберите папку <code>apps/extension/dist</code> из репозитория (сборка: <code>npm run build --workspace @ai-buddy/extension</code>, см. README).</li>
        <li>Закрепите иконку AI-buddy на панели: кнопка «пазл» → булавка.</li>
        <li>Обновите открытые вкладки. Бади появится в правом нижнем углу; клик по нему или по иконке открывает чат. Войдите той же учёткой.</li>
      </ol>
      <p className="muted">Попробовать на демо-страницах:{' '}
        <a href="/demo/wiki.html" target="_blank" rel="noreferrer">вики команды</a> (выделите жёлтую фразу),{' '}
        <a href="/demo/grafana.html" target="_blank" rel="noreferrer">дашборд</a>,{' '}
        <a href="/demo/jira.html" target="_blank" rel="noreferrer">доска задач</a>.</p>
    </section>

    {staff && <section className="panel">
      <h2>Для наставника</h2>
      <p className="muted">Вопросы, которые бот передал вам, AI-черновики ответов, заявки на согласование и метрики эффекта — в разделе «Кабинет наставника».</p>
    </section>}
  </main>;
}

/** /mentor: buddy / teamlead side of the bot. */
export function MentorPage({ token }: { token: string }) {
  return <main className="bot-page">
    <header className="bot-page-head">
      <div><span className="eyebrow">Бади · AI</span><h1>Кабинет наставника</h1>
        <p className="muted">Что бот передал вам: вопросы стажёров с AI-черновиком ответа, заявки на согласование и эффект — сколько вопросов он закрыл сам.</p></div>
    </header>
    <MentorDesk token={token}/>
  </main>;
}
