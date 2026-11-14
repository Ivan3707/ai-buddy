import type { User } from '@ai-buddy/client';

/** «Бади»: что умеет помощник, где его найти и как поставить Chrome-расширение. */
export function BuddyInfoPage({ user }: { user: User }) {
  const staff = user.role !== 'intern';
  return (
    <div className="home info-page">
      <header className="greeting">
        <h1>Бади</h1>
        <p>AI-наставник живёт в правом нижнем углу каждой страницы</p>
      </header>

      <section className="staff-card info-card">
        <h2>Что он умеет</h2>
        <ul className="info-list">
          <li>Отвечает по базе знаний со ссылками на документы и честно говорит о противоречиях между ними.</li>
          <li>Оформляет заявки на отгул, отпуск и доступ после вашего подтверждения, передаёт вопрос наставнику, если ответа в базе нет.</li>
          <li>Сам подсказывает: когда задача идёт дольше плана, когда заявку согласовали и когда наставник ответил.</li>
          <li><b>⤢ / ⤡</b> — компактный и большой режим окна. В большом есть вкладки: вопросы наставнику, заявки, заметки и календарь.</li>
          <li>Источник под ответом открывает документ в окне Бади, а «Открыть документ полностью» ведёт в <a href="/wiki/index.html" target="_blank" rel="noreferrer">вики компании (демо)</a>.</li>
          {staff && <li>Для наставника: вопросы, которые передал бот, AI-черновики ответов и заявки на согласование — в разделе «Вопросы».</li>}
        </ul>
      </section>

      <section className="staff-card info-card is-pink">
        <h2>Chrome-расширение</h2>
        <p>С расширением Бади появляется на любом рабочем сайте: подсказывает по открытой странице и проверяет выделенный текст по базе знаний. Вход общий с сайтом.</p>
        <ol className="info-list">
          <li>Откройте <code>chrome://extensions</code> и включите «Режим разработчика».</li>
          <li>Нажмите «Загрузить распакованное расширение» и выберите папку <code>apps/extension/dist</code> (сборка описана в README).</li>
          <li>Закрепите иконку AI-buddy на панели браузера и обновите открытые вкладки.</li>
        </ol>
        <p className="info-links">
          Попробовать на демо-страницах:{' '}
          <a href="/demo/wiki.html" target="_blank" rel="noreferrer">вики команды</a>,{' '}
          <a href="/demo/grafana.html" target="_blank" rel="noreferrer">дашборд</a>,{' '}
          <a href="/demo/jira.html" target="_blank" rel="noreferrer">доска задач</a>.
        </p>
      </section>
    </div>
  );
}
