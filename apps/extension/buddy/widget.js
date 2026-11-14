// "Бади" widget: the on-page character with its chat window. One code for both places it lives:
// - the Chrome extension (buddy/content.js mounts it on every site, the background worker talks to the backend);
// - the AI-buddy web app (apps/web mounts it with a gateway on the site session).
// Classic script, no imports: it is injected as a content script and also imported by the web app; it only
// defines globalThis.AIBuddyWidget = {mount(env)}.
//
// env = {
//   call(message): Promise<object>        ops: session, login, confirm, cancel, feedback, new-dialog, requests, hint, doc
//   stream(payload, onMessage): () => void streams view models for {op: 'chat'|'brief'}; returns abort()
//   storage: {get(key): Promise<any>, set(key, value): Promise<void>}   per-browser settings (position, sizes, mode)
//   features: {login, pageContext, hide, requests}   what this host supports (all true in the extension)
//   onHide?(): void                        the "×" on the character was clicked
// }
// Everything renders in a closed shadow root: page styles cannot break the character and ours cannot leak.
// Page and answer text is untrusted: set via textContent or escaped before the tiny markdown pass.
(() => {
  if (globalThis.AIBuddyWidget) return;

  const SIZE = 72;
  const MIN = {w: 300, h: 340};
  const DEFAULT_SIZE = {
    compact: () => ({w: 380, h: 560}),
    full: () => ({w: Math.min(980, innerWidth - 32), h: Math.min(740, innerHeight - 48)}),
  };
  const KEYS = {pos: 'buddyPos', mode: 'buddyChatMode', sizes: 'buddyChatSizes', greeting: 'buddyGreeting'};

  function mount(env) {
    const opt = {login: true, pageContext: true, hide: true, requests: true, ...(env.features || {})};
    const store = env.storage;
    const today = new Date().toISOString().slice(0, 10);
    const cleanup = [];

    // ---------- DOM ----------
    const host = document.createElement('ai-buddy-character');
    host.style.cssText = 'all:initial;position:fixed;inset:0 auto auto 0;width:0;height:0;z-index:2147483647;';
    const root = host.attachShadow({mode: 'closed'});
    root.innerHTML = `<style>${CSS()}</style>
      <div class="char">
        <button class="body" type="button" aria-label="Бади: открыть чат (можно перетаскивать)">${MASCOT()}</button>
        <span class="badge" aria-hidden="true">!</span>
        <button class="hide" type="button" title="Скрыть на этой вкладке" aria-label="Скрыть помощника">×</button>
      </div>
      <div class="bubble" role="status" hidden>
        <button class="bubble-x" type="button" aria-label="Скрыть">×</button>
        <span class="bubble-text"></span><span class="bubble-detail"></span><div class="bubble-actions"></div>
      </div>
      <div class="scrim" hidden></div>
      <section class="chat" role="dialog" aria-label="Чат с Бади" hidden>
        <header><b>Бади</b>
          <button class="ghost new" type="button" title="Новый диалог">Новый</button>
          <button class="ghost mode" type="button"></button>
          <button class="ghost close" type="button" aria-label="Свернуть">×</button></header>
        <nav class="tabs" role="tablist" hidden>
          <button type="button" role="tab" data-tab="chat">Чат</button>
          <button type="button" role="tab" data-tab="notes">Заметки</button>
          <button type="button" role="tab" data-tab="reqs">Заявки</button>
        </nav>
        <div class="log" role="log" aria-live="polite"></div>
        <div class="reader" hidden>
          <div class="reader-bar"><button class="back" type="button">← К чату</button>
            <a class="open-full primary-link" target="_blank" rel="noreferrer">Открыть документ полностью ↗</a></div>
          <div class="reader-body"></div>
        </div>
        <section class="panel notes" hidden>
          <aside class="side"><button type="button" class="primary new-note">+ Новая заметка</button><div class="items"></div></aside>
          <div class="editor">
            <input class="n-title" maxlength="160" placeholder="Заголовок">
            <textarea class="n-text" maxlength="8000" placeholder="Текст заметки…"></textarea>
            <div class="bar"><span class="n-status muted"></span><button type="button" class="primary n-save">Сохранить</button></div>
          </div>
        </section>
        <section class="panel reqs" hidden>
          <div class="col">
            <div class="reqs-head"><b>Мои заявки</b><button type="button" class="primary r-new">+ Новая заявка</button></div>
            <form class="r-form" hidden>
              <label>Что оформить
                <select name="type"><option value="leave_request">Отгул или отпуск</option><option value="access_request">Доступ к системе</option></select></label>
              <div class="f-leave">
                <label>Вид <select name="kind"><option value="day_off">Отгул</option><option value="vacation">Отпуск</option></select></label>
                <div class="two"><label>С <input type="date" name="date_from"></label><label>По <input type="date" name="date_to"></label></div>
                <label>Комментарий <input name="comment" maxlength="300" placeholder="необязательно"></label>
              </div>
              <div class="f-access" hidden>
                <label>Система <select name="system"><option>jira</option><option>confluence</option><option>gitlab</option><option>grafana</option><option>redash</option><option>kubernetes</option></select></label>
                <label>Ресурс <input name="resource" maxlength="200" placeholder="проект, дашборд, репозиторий…"></label>
                <label>Уровень <select name="access_level"><option value="read">read — чтение</option><option value="write">write — изменение</option></select></label>
                <label>Зачем нужен доступ <textarea name="justification" rows="2" maxlength="500"></textarea></label>
              </div>
              <div class="bar"><span class="r-status muted"></span><button type="button" class="r-cancel">Отмена</button><button type="submit" class="primary">Отправить заявку</button></div>
            </form>
            <div class="r-list"></div>
          </div>
        </section>
        <form class="login" hidden>
          <h3>Привет! Я Бади</h3>
          <p>Войдите, чтобы я знал ваш онбординг.</p>
          <input name="email" type="email" placeholder="Email" autocomplete="email" required>
          <input name="password" type="password" placeholder="Пароль" autocomplete="current-password" required>
          <button type="submit" class="primary">Войти</button><p class="login-error" role="alert"></p>
        </form>
        <div class="expired" hidden><h3>Сессия истекла</h3><p>Обновите страницу и войдите снова.</p></div>
        <div class="composer-wrap">
          <div class="chip" hidden><span class="chip-text"></span><button type="button" class="chip-x" aria-label="Не учитывать выделение">×</button></div>
          <label class="page"><input type="checkbox" class="use-page" checked> Учитывать эту страницу</label>
          <form class="composer"><textarea rows="1" maxlength="2000" placeholder="Спроси Бади…"></textarea>
            <button type="submit" class="primary send" aria-label="Отправить">➤</button></form>
        </div>
        <div class="grip" aria-hidden="true" title="Потяните, чтобы изменить размер"></div>
      </section>`;

    const $ = selector => root.querySelector(selector);
    const char = $('.char'), body = $('.body'), bubble = $('.bubble'), chat = $('.chat'), log = $('.log');
    const input = $('.composer textarea'), sendButton = $('.send'), loginForm = $('.login');
    const chip = $('.chip'), usePage = $('.use-page'), reader = $('.reader'), readerBody = $('.reader-body');
    const grip = $('.grip'), modeButton = $('.mode'), openFull = $('.open-full');
    const scrim = $('.scrim'), tabsBar = $('.tabs'), notesPanel = $('.notes'), reqsPanel = $('.reqs');
    $('.hide').hidden = !opt.hide;
    $('.page').hidden = !opt.pageContext;
    if (!opt.pageContext) usePage.checked = false;

    let pos = {x: innerWidth - SIZE - 24, y: innerHeight - SIZE - 24};
    let mode = 'compact';
    let sizes = {};
    let anchor = {right: true, below: false};
    let busy = false, signedIn = null, user = null, selection = '', abortCurrent = null;
    let tab = 'chat', readerOpen = false; // tabs exist only in the full mode

    const mountHost = () => { if (!host.isConnected) (document.body || document.documentElement).append(host); };

    // ---------- position, mode & size ----------
    function clamp(p) {
      const lim = (value, limit) => Math.min(Math.max(value, 8), Math.max(8, limit - SIZE - 8));
      return {x: Math.round(lim(p.x, innerWidth)), y: Math.round(lim(p.y, innerHeight))};
    }
    function chatSize() {
      const s = sizes[mode] || DEFAULT_SIZE[mode]();
      return {w: Math.round(Math.min(Math.max(s.w, MIN.w), innerWidth - 16)), h: Math.round(Math.min(Math.max(s.h, MIN.h), innerHeight - 16))};
    }
    function place() {
      pos = clamp(pos);
      char.style.transform = `translate(${pos.x}px, ${pos.y}px)`;
      placeFloating();
    }
    function placeFloating() {
      const right = pos.x + SIZE / 2 > innerWidth / 2; // character on the right half -> open to the left
      const below = pos.y + SIZE / 2 < innerHeight / 2;
      anchor = {right, below};
      const {w, h} = chatSize();
      chat.style.width = `${w}px`; chat.style.height = `${h}px`;
      if (mode === 'full') { // fixed in the middle of the screen, grows from the centre
        chat.style.transform = `translate(${Math.round((innerWidth - w) / 2)}px, ${Math.round((innerHeight - h) / 2)}px)`;
        grip.dataset.corner = 'br';
      } else {
        const cx = right ? pos.x + SIZE - w : pos.x;
        const cy = below ? pos.y + SIZE + 8 : pos.y - h - 8;
        chat.style.transform = `translate(${Math.min(Math.max(cx, 8), innerWidth - w - 8)}px, ${Math.min(Math.max(cy, 8), innerHeight - h - 8)}px)`;
        // the resize grip sits in the corner away from the character
        grip.dataset.corner = `${below ? 'b' : 't'}${right ? 'l' : 'r'}`;
      }
      const bw = Math.min(300, innerWidth - 16);
      bubble.style.maxWidth = `${bw}px`;
      const bx = right ? pos.x - bw - 6 : pos.x + SIZE + 6;
      bubble.style.transform = `translate(${Math.min(Math.max(bx, 8), innerWidth - bw - 8)}px, ${Math.max(Math.min(pos.y - 4, innerHeight - 160), 8)}px)`;
      bubble.classList.toggle('from-right', right);
    }
    function setMode(next, persist = true) {
      mode = next === 'full' ? 'full' : 'compact';
      chat.classList.toggle('full', mode === 'full');
      modeButton.textContent = mode === 'full' ? '⤡' : '⤢';
      modeButton.title = mode === 'full' ? 'Компактный режим' : 'Развернуть';
      modeButton.setAttribute('aria-label', modeButton.title);
      if (mode !== 'full') tab = 'chat';
      placeFloating();
      layout();
      if (persist) void store.set(KEYS.mode, mode);
    }
    modeButton.onclick = () => setMode(mode === 'full' ? 'compact' : 'full');

    Promise.all([store.get(KEYS.pos), store.get(KEYS.mode), store.get(KEYS.sizes)]).then(([p, m, s]) => {
      if (p && Number.isFinite(p.x) && Number.isFinite(p.y)) pos = p;
      if (s && typeof s === 'object') sizes = s;
      setMode(m, false);
      place();
    }, () => { setMode('compact', false); place(); });
    const onResize = () => place();
    addEventListener('resize', onResize);
    cleanup.push(() => removeEventListener('resize', onResize));

    let resizing = null;
    grip.addEventListener('pointerdown', event => {
      if (event.button !== 0) return;
      event.preventDefault();
      resizing = {sx: event.clientX, sy: event.clientY, ...chatSize()};
      grip.setPointerCapture(event.pointerId);
    });
    grip.addEventListener('pointermove', event => {
      if (!resizing) return;
      const dx = event.clientX - resizing.sx, dy = event.clientY - resizing.sy;
      sizes = {...sizes, [mode]: mode === 'full'
        ? {w: resizing.w + 2 * dx, h: resizing.h + 2 * dy}
        : {w: resizing.w + (anchor.right ? -dx : dx), h: resizing.h + (anchor.below ? dy : -dy)}};
      sizes[mode] = chatSize();
      placeFloating();
    });
    grip.addEventListener('pointerup', () => {
      if (!resizing) return;
      resizing = null;
      void store.set(KEYS.sizes, sizes); // remembered per mode, in this browser
    });

    let drag = null;
    body.addEventListener('pointerdown', event => {
      if (event.button !== 0) return;
      drag = {sx: event.clientX, sy: event.clientY, x: pos.x, y: pos.y, moved: false};
      body.setPointerCapture(event.pointerId);
    });
    body.addEventListener('pointermove', event => {
      if (!drag) return;
      const dx = event.clientX - drag.sx, dy = event.clientY - drag.sy;
      if (!drag.moved && Math.hypot(dx, dy) < 5) return;
      drag.moved = true;
      char.classList.add('dragging');
      pos = {x: drag.x + dx, y: drag.y + dy};
      place();
    });
    body.addEventListener('pointerup', () => {
      if (!drag) return;
      const moved = drag.moved;
      drag = null;
      char.classList.remove('dragging');
      if (moved) { void store.set(KEYS.pos, pos); return; }
      toggleChat();
    });
    body.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); toggleChat(); } });

    // ---------- mood ----------
    let moodTimer = 0;
    function setMood(next, ms = 0) {
      char.dataset.mood = next;
      clearTimeout(moodTimer);
      if (ms) moodTimer = setTimeout(() => { char.dataset.mood = 'idle'; }, ms);
    }
    setMood('idle');

    // ---------- bubble: one at a time, queued messages wait until the chat is closed ----------
    const queue = [];
    let bubbleTimer = 0, bubbleDone = null;
    function say(text, o = {}) {
      return new Promise(resolve => {
        const item = {text, ...o, resolve};
        if (o.queue) { queue.push(item); showNext(); return; }
        if (!chat.hidden || !bubble.hidden) { resolve(); return; } // a passing remark: skip if busy
        show(item);
      });
    }
    function showNext() {
      if (!chat.hidden || !bubble.hidden || !queue.length || !host.isConnected) return;
      show(queue.shift());
    }
    function show({text, detail = '', button = '', onClick = null, buttons = [], ms = 14000, mood = 'waving', resolve}) {
      const actions = button ? [{label: button, onClick}, ...buttons] : buttons;
      $('.bubble-text').textContent = text;
      $('.bubble-detail').textContent = detail;
      $('.bubble-detail').hidden = !detail;
      const row = $('.bubble-actions');
      row.replaceChildren(...actions.map(a => {
        const b = el('button', a.ghost ? 'bubble-go ghosty' : 'bubble-go', a.label);
        b.type = 'button';
        b.onclick = event => { event.stopPropagation(); hideBubble(); a.onClick?.(); };
        return b;
      }));
      bubble.onclick = () => { hideBubble(); onClick ? onClick() : openChat(); };
      bubbleDone = resolve;
      bubble.hidden = false;
      placeFloating();
      setMood(mood, 2400);
      clearTimeout(bubbleTimer);
      if (ms) bubbleTimer = setTimeout(hideBubble, ms);
    }
    function hideBubble() {
      if (bubble.hidden) return;
      bubble.hidden = true;
      const done = bubbleDone; bubbleDone = null; done?.();
      setTimeout(showNext, 400);
    }
    $('.bubble-x').onclick = event => { event.stopPropagation(); hideBubble(); };

    // ---------- chat ----------
    function toggleChat() { chat.hidden ? openChat() : closeChat(); }
    async function openChat() {
      hideBubble();
      chat.hidden = false;
      char.classList.add('open');
      placeFloating();
      layout();
      await refreshSession();
      if (signedIn) {
        if (!readerOpen && tab === 'chat') input.focus();
        const key = `buddyBrief:${user?.id || ''}`;
        if (await store.get(key) !== today && !log.childElementCount) {
          await store.set(key, today);
          runBrief();
        }
      }
    }
    function closeChat() { chat.hidden = true; char.classList.remove('open'); layout(); setTimeout(showNext, 400); }
    scrim.onclick = closeChat;

    // One place decides what the window shows: login (not signed in), or - signed in - the chat / reader / notes / requests.
    function layout() {
      const full = mode === 'full';
      if (!full) tab = 'chat';
      const out = !signedIn;
      scrim.hidden = !(full && !chat.hidden);
      tabsBar.hidden = out || !full;
      for (const b of tabsBar.querySelectorAll('button')) {
        b.classList.toggle('on', b.dataset.tab === tab);
        b.setAttribute('aria-selected', String(b.dataset.tab === tab));
      }
      loginForm.hidden = signedIn !== false || !opt.login;
      $('.expired').hidden = signedIn !== false || opt.login;
      log.hidden = out || tab !== 'chat' || readerOpen;
      reader.hidden = out || tab !== 'chat' || !readerOpen;
      $('.composer-wrap').hidden = out || tab !== 'chat' || readerOpen;
      notesPanel.hidden = out || tab !== 'notes';
      reqsPanel.hidden = out || tab !== 'reqs';
    }
    function showTab(next) {
      tab = next;
      readerOpen = false;
      layout();
      if (next === 'notes') void loadNotes();
      if (next === 'reqs') void loadRequests();
      if (next === 'chat') { scroll(); input.focus(); }
    }
    for (const b of tabsBar.querySelectorAll('button')) b.onclick = () => showTab(b.dataset.tab);

    async function refreshSession() {
      const state = await env.call({op: 'session'});
      signedIn = Boolean(state?.authenticated);
      user = state?.user || null;
      layout();
      if (!signedIn && state?.unavailable) $('.login-error').textContent = 'Backend недоступен. Проверьте, что стенд запущен.';
    }

    loginForm.addEventListener('submit', async event => {
      event.preventDefault();
      const data = new FormData(loginForm);
      const result = await env.call({op: 'login', email: String(data.get('email')), password: String(data.get('password'))});
      loginForm.password.value = '';
      if (!result?.ok) { $('.login-error').textContent = result?.error || 'Не удалось войти.'; return; }
      $('.login-error').textContent = '';
      await openChat();
    });

    const esc = s => String(s).replace(/[&<>"]/g, c => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;'}[c]));
    function md(text) {
      const inline = s => esc(s).replace(/\*\*(.+?)\*\*/g, '<b>$1</b>').replace(/`([^`]+)`/g, '<code>$1</code>')
        .replace(/\[(\d+(?:,\s*\d+)*)\]/g, '<sup>[$1]</sup>');
      let html = '', list = false;
      for (const line of String(text).split('\n')) {
        const item = /^\s*(?:[-*•]|\d+\.)\s+(.*)$/.exec(line);
        if (item) { if (!list) { html += '<ul>'; list = true; } html += `<li>${inline(item[1])}</li>`; continue; }
        if (list) { html += '</ul>'; list = false; }
        const heading = /^#{2,4}\s+(.*)$/.exec(line);
        if (heading) html += `<h4>${inline(heading[1])}</h4>`;
        else if (line.trim()) html += `<p>${inline(line)}</p>`;
      }
      return html + (list ? '</ul>' : '');
    }
    function el(tag, cls, text) { const n = document.createElement(tag); if (cls) n.className = cls; if (text != null) n.textContent = text; return n; }
    const scroll = () => { log.scrollTop = log.scrollHeight; };

    function assistantBubble(extra = '') {
      const node = el('div', `msg bot ${extra}`);
      const status = node.appendChild(el('p', 'status'));
      const text = node.appendChild(el('div', 'text'));
      const tail = node.appendChild(el('div'));
      log.append(node); scroll();
      let cardId = null, feedbackShown = false;
      return view => {
        status.textContent = view.status; status.hidden = !view.status;
        text.innerHTML = md(view.text || (view.finished && !view.error && !view.card ? 'Ответ пуст.' : ''));
        if (view.finished) tail.replaceChildren();
        if (view.finished && view.sources.length) {
          const ul = el('ul', 'sources');
          view.sources.forEach((s, i) => {
            const li = el('li');
            const a = el('a', '', `[${i + 1}] ${s.title}`);
            a.href = s.url; a.target = '_blank'; a.rel = 'noreferrer';
            a.title = 'Открыть документ в окне Бади';
            a.onclick = event => {
              if (!s.id || event.ctrlKey || event.metaKey || event.shiftKey) return; // modifier: the full wiki page
              event.preventDefault();
              void openDoc(s);
            };
            li.append(a); if (s.snippet) li.append(el('span', 'snip', `«${s.snippet}»`));
            ul.append(li);
          });
          tail.append(ul);
        }
        if (view.card && view.card.id !== cardId) { cardId = view.card.id; node.append(card(view.card)); }
        if (view.error) {
          const box = el('div', 'err'); box.append(el('span', '', view.error)); tail.append(box);
        }
        if (view.messageId && !feedbackShown) { feedbackShown = true; node.append(feedback(view.messageId)); }
        setMood(view.mood === 'idle' && view.finished && !view.error ? 'happy' : view.mood, view.finished ? 1600 : 0);
        scroll();
      };
    }

    function card(c) {
      const box = el('div', 'card');
      box.append(el('b', '', c.title));
      if (c.summary) box.append(el('div', 'summary', c.summary));
      const dl = el('dl');
      for (const row of c.rows) dl.append(el('dt', '', row.label), el('dd', '', row.value));
      box.append(dl);
      const result = el('p', 'result');
      if (!c.pending) { result.textContent = c.result; box.append(result); return box; }
      const row = el('div', 'row');
      const ok = row.appendChild(el('button', 'primary', 'Подтвердить'));
      const no = row.appendChild(el('button', '', 'Отменить'));
      ok.type = no.type = 'button';
      box.append(row, result);
      setMood('waiting');
      const run = async op => {
        ok.disabled = no.disabled = true;
        result.textContent = op === 'confirm' ? 'Отправляю заявку…' : 'Отменяю…';
        result.dataset.tone = 'muted';
        const outcome = await env.call({op, actionId: c.id});
        result.textContent = outcome?.text || 'Не удалось связаться с сервером.';
        result.dataset.tone = outcome?.tone || 'error';
        if (outcome?.retryable) { ok.disabled = no.disabled = false; return; }
        row.remove();
        setMood(outcome?.tone === 'success' ? 'happy' : outcome?.tone === 'error' ? 'sad' : 'idle', 2500);
      };
      ok.onclick = () => run('confirm');
      no.onclick = () => run('cancel');
      return box;
    }

    function feedback(messageId) {
      const row = el('div', 'fb');
      const note = el('span');
      for (const [rating, icon] of [['up', '👍'], ['down', '👎']]) {
        const b = row.appendChild(el('button', '', icon));
        b.type = 'button';
        b.setAttribute('aria-label', rating === 'up' ? 'Полезный ответ' : 'Бесполезный ответ');
        b.onclick = async () => {
          for (const x of row.querySelectorAll('button')) x.disabled = true;
          b.classList.add('on');
          const r = await env.call({op: 'feedback', messageId, rating});
          note.textContent = r?.ok ? 'Спасибо!' : 'Не сохранилось';
        };
      }
      row.append(note);
      return row;
    }

    // ---------- document reader (sources open here, in the full mode) ----------
    const plain = s => String(s || '').replace(/\*\*|`/g, '').replace(/\s+/g, ' ').trim();
    function fragmentUrl(url, snippet) {
      if (!/^https?:\/\//.test(url || '')) return '';
      const words = plain(snippet).split(' ').slice(0, 8).join(' ');
      return words ? `${url}#:~:text=${encodeURIComponent(words)}` : url; // Chrome scrolls to and highlights it
    }
    async function openDoc(source) {
      setMode('full');
      tab = 'chat'; readerOpen = true; layout();
      readerBody.replaceChildren(el('p', 'muted', 'Открываю документ…'));
      const fallback = fragmentUrl(source.url, source.snippet);
      openFull.href = fallback || '#'; openFull.hidden = !fallback;
      const result = await env.call({op: 'doc', id: source.id});
      if (!result?.doc) { readerBody.replaceChildren(el('p', 'err', result?.error || 'Не удалось загрузить документ.')); return; }
      const doc = result.doc;
      const full = fragmentUrl(doc.url, source.snippet) || fallback;
      openFull.href = full || '#'; openFull.hidden = !full;
      const head = el('div', 'doc-head');
      head.append(el('h3', '', doc.title), el('p', 'muted', `База знаний · обновлено ${doc.updated || '—'} · демо`));
      const article = el('article', 'doc');
      for (const section of doc.sections || []) {
        if (section.heading) article.append(el('h4', '', section.heading));
        const part = el('div');
        part.innerHTML = md(section.text);
        article.append(...part.childNodes);
      }
      readerBody.replaceChildren(head, article);
      readerBody.scrollTop = 0;
      const needle = plain(source.snippet).slice(0, 60).toLowerCase();
      if (needle) {
        const hit = [...article.querySelectorAll('p, li')].find(n => plain(n.textContent).toLowerCase().includes(needle));
        if (hit) { hit.classList.add('hit'); hit.scrollIntoView({block: 'center'}); }
      }
    }
    function closeDoc() {
      readerOpen = false; layout();
      scroll();
    }
    $('.back').onclick = closeDoc;

    // ---------- notes tab ----------
    const when = iso => { try { return new Date(iso).toLocaleDateString('ru-RU', {day: '2-digit', month: '2-digit'}); } catch { return ''; } };
    let notes = [], noteId = '';
    const nTitle = $('.n-title'), nText = $('.n-text'), nStatus = $('.n-status'), nSave = $('.n-save');
    function renderNotes() {
      const box = $('.notes .items');
      box.replaceChildren(...notes.map(n => {
        const b = el('button', n.id === noteId ? 'item on' : 'item');
        b.type = 'button';
        b.append(el('b', '', n.title || 'Без названия'), el('small', 'muted', when(n.updated_at)));
        b.onclick = () => { noteId = n.id; nTitle.value = n.title; nText.value = n.content; nStatus.textContent = ''; renderNotes(); };
        return b;
      }));
      if (!notes.length) box.append(el('p', 'muted pad', 'Заметок пока нет.'));
    }
    async function loadNotes() {
      const result = await env.call({op: 'notes'});
      if (result?.notes) { notes = result.notes; renderNotes(); nStatus.textContent = ''; } else nStatus.textContent = result?.error || '';
    }
    $('.new-note').onclick = () => { noteId = ''; nTitle.value = ''; nText.value = ''; nStatus.textContent = ''; renderNotes(); nTitle.focus(); };
    nSave.onclick = async () => {
      nSave.disabled = true; nStatus.textContent = 'Сохраняю…';
      const result = await env.call({op: 'note-save', id: noteId || undefined, title: nTitle.value, content: nText.value});
      nSave.disabled = false;
      if (!result?.note) { nStatus.textContent = result?.error || 'Не удалось сохранить.'; return; }
      noteId = result.note.id;
      nStatus.textContent = 'Сохранено';
      await loadNotes();
      nStatus.textContent = 'Сохранено';
    };

    // ---------- requests tab ----------
    const STATUS = {pending_approval: 'на согласовании', approved: 'согласована', rejected: 'отклонена'};
    const WORDS = {day_off: 'Отгул', vacation: 'Отпуск', read: 'чтение', write: 'изменение', admin: 'администрирование'};
    const dayRu = iso => (iso || '').split('-').reverse().join('.');
    function describeRequest(r) {
      const p = r.params || {};
      return r.type === 'leave_request'
        ? `${WORDS[p.kind] || 'Отсутствие'}: ${dayRu(p.date_from)}${p.date_to && p.date_to !== p.date_from ? ` — ${dayRu(p.date_to)}` : ''}`
        : `Доступ: ${p.system || ''} · ${p.resource || ''} · ${WORDS[p.access_level] || p.access_level || ''}`;
    }
    function renderRequests(list) {
      const box = $('.r-list');
      if (!list.length) { box.replaceChildren(el('p', 'muted pad', 'Заявок пока нет. Нажмите «Новая заявка» или попросите Бади в чате.')); return; }
      box.replaceChildren(...list.map(r => {
        const row = el('div', 'req');
        const head = el('div', 'req-head');
        head.append(el('b', '', r.number), el('span', `pill ${r.status}`, STATUS[r.status] || r.status));
        row.append(head, el('div', '', describeRequest(r)));
        if (r.params?.comment || r.params?.justification) row.append(el('small', 'muted', r.params.comment || r.params.justification));
        if (r.approver) row.append(el('small', 'muted', `Согласует: ${r.approver}`));
        return row;
      }));
    }
    async function loadRequests() {
      const result = await env.call({op: 'requests-list'});
      if (result?.requests) renderRequests(result.requests); else $('.r-list').replaceChildren(el('p', 'err', result?.error || 'Не удалось загрузить заявки.'));
    }
    const rForm = $('.r-form'), rStatus = $('.r-status');
    const toggleFields = () => {
      const access = rForm.type.value === 'access_request';
      $('.f-leave').hidden = access; $('.f-access').hidden = !access;
    };
    rForm.type.onchange = toggleFields;
    $('.r-new').onclick = () => {
      const iso = new Date(Date.now() + 86400000).toISOString().slice(0, 10);
      rForm.reset(); rForm.date_from.min = rForm.date_to.min = iso; rForm.date_from.value = rForm.date_to.value = iso;
      toggleFields(); rStatus.textContent = ''; rForm.hidden = false;
    };
    $('.r-cancel').onclick = () => { rForm.hidden = true; };
    rForm.addEventListener('submit', async event => {
      event.preventDefault();
      const f = new FormData(rForm);
      const type = String(f.get('type'));
      const params = type === 'leave_request'
        ? {kind: f.get('kind'), date_from: f.get('date_from'), date_to: f.get('date_to') || f.get('date_from'), comment: f.get('comment')}
        : {system: f.get('system'), resource: f.get('resource'), access_level: f.get('access_level'), justification: f.get('justification')};
      if (type === 'access_request' && (!String(params.resource).trim() || !String(params.justification).trim())) { rStatus.textContent = 'Укажите ресурс и зачем нужен доступ.'; return; }
      if (type === 'leave_request' && !params.date_from) { rStatus.textContent = 'Укажите даты.'; return; }
      const submit = rForm.querySelector('[type=submit]');
      submit.disabled = true; rStatus.textContent = 'Отправляю…';
      const result = await env.call({op: 'request-create', type, params});
      submit.disabled = false;
      if (!result?.request) { rStatus.textContent = result?.error || 'Не удалось создать заявку.'; return; }
      rForm.hidden = true;
      await loadRequests();
      const note = el('p', 'notice', `Заявка ${result.request.number} создана, статус: ${STATUS[result.request.status] || result.request.status}.`);
      $('.r-list').prepend(note);
      setTimeout(() => note.remove(), 8000);
    });

    // ---------- asking ----------
    let queued = null; // question asked while the brief is still streaming: sent right after it
    function setBusy(value) {
      busy = value;
      sendButton.disabled = value;
      if (!value && queued) { const next = queued; queued = null; ask(next); }
    }
    function finish(message, render) {
      if (message.op === 'view') { render(message.view); if (message.view.finished) setBusy(false); return; }
      setBusy(false);
      if (message.op === 'signed-out') void refreshSession();
    }
    function runBrief() {
      setBusy(true);
      const render = assistantBubble('brief');
      abortCurrent = env.stream({op: 'brief'}, message => {
        if (message.op === 'unsupported') { setBusy(false); log.lastElementChild?.remove(); return; }
        finish(message, render);
      });
    }
    function pageContext() {
      if (!opt.pageContext || !usePage.checked) return null;
      const url = /^https?:/.test(location.protocol) ? `${location.origin}${location.pathname}` : '';
      if (!url) return null;
      return {
        url: url.slice(0, 2000),
        ...(document.title ? {title: document.title.slice(0, 300)} : {}),
        ...(selection ? {selection: selection.slice(0, 2000)} : {}),
      };
    }
    function ask(text) {
      if (!text || !signedIn) return;
      if (busy) { queued = text; return; }
      if (readerOpen) closeDoc();
      if (tab !== 'chat') showTab('chat');
      const page = pageContext();
      const userNode = el('div', 'msg user', text);
      if (page) userNode.append(el('span', 'ctx', page.selection ? '📄 страница + выделенный текст' : '📄 с учётом страницы'));
      log.append(userNode);
      setSelection('');
      setBusy(true);
      const render = assistantBubble();
      abortCurrent = env.stream({op: 'chat', message: text, pageContext: page}, message => finish(message, render));
    }

    $('.composer').addEventListener('submit', event => {
      event.preventDefault();
      const text = input.value.trim();
      if (!text || (busy && queued)) return;
      input.value = '';
      ask(text);
    });
    input.addEventListener('keydown', event => {
      if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); $('.composer').requestSubmit(); }
    });
    $('.close').onclick = closeChat;
    $('.new').onclick = async () => {
      abortCurrent?.();
      setBusy(false);
      await env.call({op: 'new-dialog'});
      log.replaceChildren();
      closeDoc();
      input.focus();
    };
    $('.hide').onclick = event => {
      event.stopPropagation();
      env.onHide?.();
      host.remove();
    };
    root.addEventListener('keydown', event => { if (event.key === 'Escape') closeChat(); });

    // ---------- selection on the page ----------
    function setSelection(text) {
      selection = text;
      chip.hidden = !text;
      $('.chip-text').textContent = text ? `Выделено: «${text.length > 70 ? `${text.slice(0, 70)}…` : text}»` : '';
    }
    $('.chip-x').onclick = () => setSelection('');
    if (opt.pageContext) {
      const onMouseUp = event => {
        if (event.composedPath().includes(host)) return;
        setTimeout(() => {
          const text = String(getSelection() || '').trim().replace(/\s+/g, ' ');
          if (text.length < 12) return;
          setSelection(text.slice(0, 2000));
          if (chat.hidden) {
            void say('Спросить меня про выделенный текст?', {button: 'Это актуально?', mood: 'curious', ms: 8000,
              onClick: async () => { await openChat(); ask('Это ещё актуально?'); }});
          }
        }, 0);
      };
      document.addEventListener('mouseup', onMouseUp);
      cleanup.push(() => document.removeEventListener('mouseup', onMouseUp));
    }

    // ---------- proactive ----------
    async function greet() {
      await refreshSession();
      if (await store.get(KEYS.greeting) !== today) {
        await store.set(KEYS.greeting, today);
        const name = user?.name ? `, ${user.name}` : '';
        void say(signedIn ? `Привет${name}! У меня есть сводка на сегодня.` : 'Привет! Я Бади, помогу с онбордингом. Войди, и начнём.',
          {button: signedIn ? 'Показать' : 'Войти', onClick: openChat});
        return;
      }
      if (!opt.pageContext) return;
      const hint = (await env.call({op: 'hint', title: document.title, url: location.href}))?.hint;
      const hintKey = `aiBuddyHint:${location.pathname}`;
      if (hint && signedIn && !sessionStorage.getItem(hintKey)) {
        sessionStorage.setItem(hintKey, '1');
        void say(hint.text, {button: hint.question ? 'Да, подскажи' : '', mood: 'curious', onClick: async () => {
          await openChat();
          if (hint.question) ask(hint.question);
        }});
      }
    }
    const greetTimer = setTimeout(() => { if (host.isConnected) void greet(); }, 1200);
    cleanup.push(() => clearTimeout(greetTimer));

    if (opt.requests) {
      const pollRequests = async () => {
        if (document.hidden || !host.isConnected) return;
        const {changes = []} = await env.call({op: 'requests'}) || {};
        const last = changes[changes.length - 1];
        if (!last) return;
        const good = last.status === 'approved' || last.status === 'done';
        void say(`${good ? '🎉 ' : ''}${last.text}`, {mood: good ? 'happy' : 'sad', onClick: openChat, queue: true});
      };
      void pollRequests();
      const timer = setInterval(() => { void pollRequests(); }, 20000);
      cleanup.push(() => clearInterval(timer));
    }

    return {
      host,
      show() { mountHost(); place(); },
      remove() { abortCurrent?.(); for (const fn of cleanup) fn(); host.remove(); },
      isShown: () => host.isConnected,
      isOpen: () => !chat.hidden,
      open: openChat,
      close: closeChat,
      toggle: toggleChat,
      async ask(text) { await openChat(); ask(text); },
      say,
    };
  }

  function MASCOT() {
    return `<svg viewBox="0 0 72 72" width="72" height="72" aria-hidden="true">
      <defs><linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#3fbf8a"/><stop offset="1" stop-color="#1f6f50"/></linearGradient></defs>
      <g class="whole">
        <line x1="36" y1="12" x2="36" y2="5" stroke="#1f6f50" stroke-width="3" stroke-linecap="round"/>
        <circle class="antenna" cx="36" cy="5" r="4"/>
        <ellipse cx="36" cy="66" rx="18" ry="3" fill="rgba(0,0,0,.15)" class="shadow"/>
        <rect x="9" y="12" width="54" height="50" rx="22" fill="url(#g)"/>
        <rect x="16" y="22" width="40" height="28" rx="13" fill="#eafff5"/>
        <g class="eyes"><ellipse class="eye" cx="28" cy="35" rx="4" ry="5" fill="#123"/><ellipse class="eye" cx="44" cy="35" rx="4" ry="5" fill="#123"/></g>
        <g class="happy-eyes"><path d="M24 36 q4 -6 8 0" /><path d="M40 36 q4 -6 8 0" /></g>
        <path class="mouth" d="M31 43 q5 4 10 0" fill="none" stroke="#123" stroke-width="2.4" stroke-linecap="round"/>
        <circle cx="21" cy="42" r="2.6" fill="#ffb3b3" opacity=".7"/><circle cx="51" cy="42" r="2.6" fill="#ffb3b3" opacity=".7"/>
        <path class="arm left" d="M10 44 q-7 2 -6 -8" fill="none" stroke="#1f6f50" stroke-width="4" stroke-linecap="round"/>
        <path class="arm right" d="M62 44 q7 2 6 -8" fill="none" stroke="#1f6f50" stroke-width="4" stroke-linecap="round"/>
      </g></svg>`;
  }

  function CSS() {
    return `
    :host{all:initial}
    *{box-sizing:border-box;font-family:system-ui,-apple-system,"Segoe UI",sans-serif}
    [hidden]{display:none!important}
    .char{position:fixed;left:0;top:0;width:${SIZE}px;height:${SIZE}px;touch-action:none}
    .body{all:unset;display:block;width:${SIZE}px;height:${SIZE}px;cursor:grab;filter:drop-shadow(0 4px 10px rgba(0,0,0,.25));transition:transform .15s}
    .body:hover{transform:scale(1.06)}.body:focus-visible{outline:3px solid #3fbf8a;outline-offset:2px;border-radius:50%}
    .dragging .body{cursor:grabbing;transform:scale(1.1) rotate(-6deg)}
    .hide{all:unset;position:absolute;top:-6px;right:-6px;width:20px;height:20px;border-radius:50%;background:#fff;color:#556;font-size:14px;line-height:20px;text-align:center;cursor:pointer;box-shadow:0 1px 4px rgba(0,0,0,.3);opacity:0;transition:opacity .2s}
    .char:hover .hide,.hide:focus-visible{opacity:1}
    .badge{position:absolute;top:0;left:-2px;width:20px;height:20px;border-radius:50%;background:#f5a623;color:#fff;font:700 13px/20px system-ui;text-align:center;display:none;box-shadow:0 1px 4px rgba(0,0,0,.3)}
    .char[data-mood=waiting] .badge{display:block;animation:pop 1s infinite}
    .whole{transform-origin:36px 60px;animation:bob 3.2s ease-in-out infinite}
    .eye{transform-box:fill-box;transform-origin:center;animation:blink 4.5s infinite}
    .antenna{fill:#7fe0b4;transition:fill .3s}
    .happy-eyes{display:none;fill:none;stroke:#123;stroke-width:2.6;stroke-linecap:round}
    .arm{transform-box:fill-box}.arm.right{transform-origin:0% 100%}.arm.left{transform-origin:100% 100%}
    .mouth{transform-box:fill-box;transform-origin:center}
    .char[data-mood=thinking] .antenna{fill:#f5a623;animation:pulse .8s infinite}
    .char[data-mood=thinking] .eyes{transform:translate(2px,-3px)}
    .char[data-mood=talking] .mouth{animation:talk .28s infinite alternate}
    .char[data-mood=happy] .eyes{display:none}.char[data-mood=happy] .happy-eyes{display:inline}
    .char[data-mood=happy] .whole{animation:hop .5s 2}
    .char[data-mood=waving] .arm.right,.char[data-mood=curious] .arm.right{animation:wave .5s 3}
    .char[data-mood=curious] .whole{transform:rotate(-8deg)}
    .char[data-mood=sad] .mouth{transform:scaleY(-1)}.char[data-mood=sad] .antenna{fill:#e06666}
    .open .body{filter:drop-shadow(0 0 0 transparent) drop-shadow(0 4px 12px rgba(31,111,80,.6))}
    @keyframes bob{50%{transform:translateY(-3px)}}
    @keyframes blink{0%,94%,100%{transform:scaleY(1)}97%{transform:scaleY(.1)}}
    @keyframes pulse{50%{opacity:.35}}
    @keyframes talk{to{transform:scaleY(2.2)}}
    @keyframes hop{50%{transform:translateY(-10px)}}
    @keyframes wave{50%{transform:rotate(-25deg)}}
    @keyframes pop{50%{transform:scale(1.15)}}
    @media (prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}
    .bubble{position:fixed;left:0;top:0;background:#fff;color:#1d2330;border-radius:14px;padding:10px 26px 10px 12px;font-size:14px;line-height:1.4;box-shadow:0 6px 24px rgba(0,0,0,.18);border:1px solid #e1e4ea;cursor:pointer;animation:appear .25s}
    .bubble-detail{display:block;margin-top:6px;padding:6px 8px;border-radius:8px;background:#eef6f2;font-size:13px}
    .bubble-actions{display:flex;flex-wrap:wrap;gap:10px;margin-top:6px}
    .bubble-go{all:unset;color:#1f6f50;font-weight:600;font-size:13px;cursor:pointer}.bubble-go.ghosty{color:#68708a;font-weight:500}
    .bubble-x{all:unset;position:absolute;top:4px;right:8px;color:#9aa2b5;font-size:16px;cursor:pointer}
    @keyframes appear{from{opacity:0;transform:translateY(6px)}}
    .chat{position:fixed;left:0;top:0;display:flex;flex-direction:column;background:#fff;color:#1d2330;border-radius:16px;box-shadow:0 12px 40px rgba(0,0,0,.25);border:1px solid #e1e4ea;overflow:hidden;font-size:14px;line-height:1.45}
    .chat.full{font-size:15px}
    .chat header{display:flex;align-items:center;gap:6px;padding:10px 12px;background:#1f6f50;color:#fff}
    .chat header b{font-size:15px;margin-right:auto}
    button{font:inherit;cursor:pointer;border-radius:8px;border:1px solid #c5cad3;background:#fff;color:#1d2330;padding:4px 10px}
    button:disabled{opacity:.5;cursor:default}
    .primary{background:#1f6f50;border-color:#1f6f50;color:#fff}
    .ghost{background:transparent;border-color:transparent;color:#fff;padding:2px 8px}.ghost:hover{background:rgba(255,255,255,.15)}
    .mode{font-size:16px;line-height:1}
    .log{flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:8px;background:#f7f8fa}
    .full .log{padding:16px 20px}
    .msg{max-width:90%;padding:8px 11px;border-radius:12px;overflow-wrap:anywhere}
    .full .msg{max-width:78%}
    .user{align-self:flex-end;background:#1f6f50;color:#fff;white-space:pre-wrap}
    .ctx{display:block;font-size:11px;opacity:.85;margin-top:2px}
    .bot{align-self:flex-start;background:#fff;border:1px solid #e6e9ee}.brief{border-left:3px solid #1f6f50}
    .bot p{margin:0 0 5px}.bot p:last-child{margin:0}.bot ul{margin:3px 0 5px;padding-left:18px}.bot sup{color:#1f6f50;font-size:10px}
    .bot code,.doc code{background:#f2f4f7;padding:0 3px;border-radius:4px}
    .status{margin:0;color:#68708a;font-size:12px}.status::before{content:"";display:inline-block;width:7px;height:7px;margin-right:6px;border-radius:50%;background:#1f6f50;animation:pulse 1s infinite}
    .sources{list-style:none;margin:6px 0 0;padding:6px 0 0;border-top:1px solid #eceef2;font-size:12px}.sources li{margin-top:3px}
    .sources a{color:#1f6f50;text-decoration:underline;text-underline-offset:2px;cursor:pointer}
    .snip{display:block;color:#68708a;font-style:italic;font-size:11px}
    .card{margin-top:8px;border:1px solid #d9dee6;border-radius:10px;padding:8px 10px;background:#fbfcfd}
    .card dl{display:grid;grid-template-columns:max-content 1fr;gap:1px 8px;margin:6px 0;font-size:12px}.card dt{color:#68708a}.card dd{margin:0}
    .summary{font-size:13px}.row{display:flex;gap:6px}
    .result{margin:6px 0 0;font-size:12px}.result[data-tone=success]{color:#1f6f50}.result[data-tone=error]{color:#a12a2a}.result[data-tone=muted]{color:#68708a}
    .fb{display:flex;gap:2px;align-items:center;margin-top:4px;font-size:11px;color:#68708a}.fb button{border:none;background:transparent;padding:0 4px}.fb .on{background:#eef1f6}
    .err{margin-top:6px;padding:5px 8px;border-radius:8px;background:#fbeaea;color:#a12a2a;font-size:12px}
    .muted{color:#68708a}
    .reader{flex:1;display:flex;flex-direction:column;min-height:0;background:#f7f8fa}
    .reader-bar{display:flex;align-items:center;gap:8px;justify-content:space-between;padding:8px 12px;border-bottom:1px solid #e6e9ee;background:#fff}
    .primary-link{background:#1f6f50;color:#fff;border-radius:8px;padding:5px 12px;text-decoration:none;font-size:13px;font-weight:600}
    .reader-body{flex:1;overflow-y:auto;padding:16px 22px 24px}
    .doc-head h3{margin:0 0 2px;font-size:19px}.doc-head p{margin:0 0 12px;font-size:12px}
    .doc h4{margin:16px 0 4px;font-size:15px}.doc p{margin:0 0 8px}.doc ul{margin:0 0 8px;padding-left:20px}
    .hit{background:#fff3b0;border-radius:6px;box-shadow:0 0 0 4px #fff3b0}
    .login{flex:1;display:flex;flex-direction:column;justify-content:center;gap:10px;width:100%;max-width:340px;margin:0 auto;padding:24px}
    .login h3,.expired h3{margin:0;font-size:21px;text-align:center}.login p,.expired p{margin:0;text-align:center;color:#68708a}
    .login input{font:inherit;padding:9px 12px;border:1px solid #c5cad3;border-radius:10px}.login .primary{padding:9px}
    .login-error{color:#a12a2a;font-size:12px}.login-error:empty{display:none}
    .expired{flex:1;display:flex;flex-direction:column;justify-content:center;gap:8px;padding:24px}
    .scrim{position:fixed;inset:0;background:rgba(15,20,25,.45)}
    .tabs{display:flex;gap:2px;padding:6px 12px 0;background:#fff;border-bottom:1px solid #e6e9ee}
    .tabs button{border:none;border-bottom:3px solid transparent;border-radius:0;background:transparent;padding:8px 16px;color:#68708a}
    .tabs button.on{border-bottom-color:#1f6f50;color:#1f6f50;font-weight:600}
    .panel{flex:1;min-height:0;display:flex;background:#f7f8fa}
    .notes .side{width:240px;flex:none;display:flex;flex-direction:column;gap:8px;padding:12px;border-right:1px solid #e6e9ee;background:#fff;overflow-y:auto}
    .item{display:flex;flex-direction:column;align-items:flex-start;gap:0;width:100%;text-align:left;margin-bottom:4px}
    .item.on{border-color:#1f6f50;background:#eef6f2}.item small{font-size:11px}
    .editor{flex:1;min-width:0;display:flex;flex-direction:column;gap:8px;padding:12px}
    .editor input,.editor textarea,.r-form input,.r-form select,.r-form textarea{font:inherit;padding:8px 10px;border:1px solid #c5cad3;border-radius:10px;color:#1d2330;background:#fff;width:100%}
    .editor textarea{flex:1;resize:none}
    .bar{display:flex;align-items:center;justify-content:flex-end;gap:8px}.bar .muted{margin-right:auto;font-size:12px}
    .reqs .col{flex:1;min-width:0;overflow-y:auto;padding:14px 18px;display:flex;flex-direction:column;gap:10px}
    .reqs-head{display:flex;align-items:center;justify-content:space-between}
    .r-form{display:grid;gap:8px;padding:12px;border:1px solid #d9dee6;border-radius:12px;background:#fff}
    .r-form label{display:grid;gap:3px;font-size:12px;color:#68708a}.r-form .two{display:grid;grid-template-columns:1fr 1fr;gap:8px}
    .f-leave,.f-access{display:grid;gap:8px}
    .req{display:flex;flex-direction:column;gap:2px;padding:10px 12px;border:1px solid #e6e9ee;border-radius:12px;background:#fff}
    .req-head{display:flex;align-items:center;gap:8px}
    .pill{font-size:11px;border-radius:999px;padding:1px 8px;background:#eef1f6;color:#556}
    .pill.approved{background:#dff3e8;color:#1f6f50}.pill.rejected{background:#fbeaea;color:#a12a2a}.pill.pending_approval{background:#fff4cc;color:#5a4a00}
    .notice{margin:0;padding:6px 10px;border-radius:8px;background:#dff3e8;color:#1f6f50;font-size:13px}
    .pad{padding:8px 4px;margin:0}
    .composer-wrap{border-top:1px solid #e6e9ee;padding:6px 10px 10px;background:#fff}
    .page{display:flex;gap:6px;align-items:center;font-size:11px;color:#68708a;margin-bottom:4px}
    .chip{display:flex;align-items:center;gap:4px;background:#eef6f2;border-radius:8px;padding:3px 6px;font-size:11px;margin-bottom:4px}
    .chip-text{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.chip-x{border:none;background:transparent;padding:0 4px}
    .composer{display:flex;gap:6px;align-items:flex-end}
    .composer textarea{flex:1;resize:none;font:inherit;padding:7px 10px;border:1px solid #c5cad3;border-radius:10px;min-height:38px;max-height:120px;color:#1d2330;background:#fff}
    .send{padding:7px 12px}
    .grip{position:absolute;width:18px;height:18px;z-index:2;opacity:.55;touch-action:none}
    .grip:hover{opacity:1}
    .grip::before{content:"";position:absolute;inset:4px;border-color:#9aa2b5;border-style:solid;border-width:0}
    .grip[data-corner=tl]{top:0;left:0;cursor:nwse-resize}.grip[data-corner=tl]::before{border-top-width:2px;border-left-width:2px;border-top-left-radius:6px;border-color:#fff}
    .grip[data-corner=tr]{top:0;right:0;cursor:nesw-resize}.grip[data-corner=tr]::before{border-top-width:2px;border-right-width:2px;border-top-right-radius:6px;border-color:#fff}
    .grip[data-corner=bl]{bottom:0;left:0;cursor:nesw-resize}.grip[data-corner=bl]::before{border-bottom-width:2px;border-left-width:2px;border-bottom-left-radius:6px}
    .grip[data-corner=br]{bottom:0;right:0;cursor:nwse-resize}.grip[data-corner=br]::before{border-bottom-width:2px;border-right-width:2px;border-bottom-right-radius:6px}
    @media (prefers-color-scheme:dark){
      .chat,.composer-wrap,.bubble,.reader-bar,.tabs,.notes .side,.r-form,.req{background:#1c2027;color:#e6e8ee;border-color:#2c323c}
      .panel{background:#14171c}.item.on{background:#1f3a2f}.editor input,.editor textarea,.r-form input,.r-form select,.r-form textarea{background:#14171c;color:#e6e8ee;border-color:#2c323c}
      .pill{background:#2c323c;color:#c5cad3}.notice{background:#1f3a2f;color:#5fd0a0}.tabs button.on{color:#5fd0a0;border-bottom-color:#5fd0a0}
      .log,.reader{background:#14171c}.bot{background:#222830;border-color:#2c323c}.card{background:#1c2027;border-color:#2c323c}
      .composer textarea,.login input{background:#14171c;color:#e6e8ee;border-color:#2c323c}
      button{background:#222830;color:#e6e8ee;border-color:#2c323c}.primary,.primary-link{background:#2f9b70;border-color:#2f9b70;color:#fff}
      .sources a,.bubble-go,.bot sup{color:#5fd0a0}.chip,.bubble-detail{background:#1f3a2f}.bot code,.doc code{background:#14171c}
      .hit{background:#4a4200;box-shadow:0 0 0 4px #4a4200;color:#fff}.muted,.snip{color:#9aa2b5}
    }`;
  }

  globalThis.AIBuddyWidget = {mount};
})();
