// On-page character "Бади" (content script, classic script — no imports).
// A pure view: every backend call goes through the background worker, which also holds the token.
// Everything is rendered inside a closed shadow root, so page styles cannot break the character and
// our styles cannot leak into the page. Page text is untrusted: it is only ever set via textContent
// or escaped before the tiny markdown pass.
(() => {
  if (globalThis.__aiBuddyCharacter) return; // injected twice (registration + executeScript)
  globalThis.__aiBuddyCharacter = true;
  // The AI-buddy web app has its own Бади widget; only its static demo pages (/demo/*.html) get this one.
  if (/^(127\.0\.0\.1|localhost):5173$/.test(location.host) && !location.pathname.startsWith('/demo/')) return;

  const SIZE = 72;
  const POS_KEY = 'buddyPos';
  const HIDDEN_KEY = 'aiBuddyHidden';
  const today = new Date().toISOString().slice(0, 10);
  const hiddenOnTab = sessionStorage.getItem(HIDDEN_KEY) === '1'; // "×" on the character: until the tab closes

  // ---------- DOM ----------
  const host = document.createElement('ai-buddy-character');
  host.style.cssText = 'all:initial;position:fixed;inset:0 auto auto 0;width:0;height:0;z-index:2147483647;';
  const root = host.attachShadow({mode: 'closed'});
  root.innerHTML = `<style>${CSS()}</style>
    <div class="char" part="char">
      <button class="body" type="button" aria-label="AI-buddy: открыть чат (можно перетаскивать)">${MASCOT()}</button>
      <span class="badge" aria-hidden="true">!</span>
      <button class="hide" type="button" title="Скрыть на этой вкладке" aria-label="Скрыть помощника">×</button>
    </div>
    <div class="bubble" role="status" hidden><span class="bubble-text"></span><button class="bubble-go" type="button"></button></div>
    <section class="chat" role="dialog" aria-label="Чат с AI-buddy" hidden>
      <header><b>Бади</b><span class="tag">демо</span>
        <button class="ghost new" type="button" title="Новый диалог">Новый</button>
        <button class="ghost wide" type="button" title="Открыть в боковой панели">⤢</button>
        <button class="ghost close" type="button" aria-label="Свернуть">×</button></header>
      <div class="log" role="log" aria-live="polite"></div>
      <form class="login" hidden>
        <p>Войдите, чтобы Бади знал ваш онбординг.</p>
        <input name="email" type="email" placeholder="Email" autocomplete="email" required>
        <input name="password" type="password" placeholder="Пароль" autocomplete="current-password" required>
        <button type="submit" class="primary">Войти</button><p class="login-error" role="alert"></p>
      </form>
      <div class="composer-wrap">
        <div class="chip" hidden><span class="chip-text"></span><button type="button" class="chip-x" aria-label="Не учитывать выделение">×</button></div>
        <label class="page"><input type="checkbox" class="use-page" checked> Учитывать эту страницу</label>
        <form class="composer"><textarea rows="1" maxlength="2000" placeholder="Спроси Бади…"></textarea>
          <button type="submit" class="primary send" aria-label="Отправить">➤</button></form>
      </div>
    </section>`;
  const mount = () => { if (!host.isConnected) (document.body || document.documentElement).append(host); };
  if (!hiddenOnTab) mount();

  const $ = selector => root.querySelector(selector);
  const char = $('.char'), body = $('.body'), bubble = $('.bubble'), chat = $('.chat'), log = $('.log');
  const input = $('.composer textarea'), sendButton = $('.send'), loginForm = $('.login');
  const chip = $('.chip'), usePage = $('.use-page');

  let pos = {x: innerWidth - SIZE - 24, y: innerHeight - SIZE - 24};
  let busy = false, signedIn = null, user = null, selection = '', requestSeq = 0, current = null;

  // ---------- messaging ----------
  const call = message => chrome.runtime.sendMessage(message).catch(() => ({error: 'extension reloaded'}));
  let port = null;
  const handlers = new Map();
  function connect() {
    port = chrome.runtime.connect({name: 'buddy'});
    port.onMessage.addListener(message => handlers.get(message.requestId)?.(message));
    port.onDisconnect.addListener(() => { port = null; });
  }
  function stream(payload, onMessage) {
    if (!port) connect();
    const requestId = ++requestSeq;
    handlers.set(requestId, onMessage);
    port.postMessage({...payload, requestId});
    return requestId;
  }

  // ---------- position & drag ----------
  function clamp(p) {
    const max = (value, limit) => Math.min(Math.max(value, 8), Math.max(8, limit - SIZE - 8));
    return {x: Math.round(max(p.x, innerWidth)), y: Math.round(max(p.y, innerHeight))};
  }
  function place() {
    pos = clamp(pos);
    char.style.transform = `translate(${pos.x}px, ${pos.y}px)`;
    placeFloating();
  }
  function placeFloating() {
    const right = pos.x + SIZE / 2 > innerWidth / 2; // character on the right half -> open to the left
    const below = pos.y + SIZE / 2 < innerHeight / 2;
    const w = Math.min(380, innerWidth - 16), h = Math.min(560, innerHeight - 16);
    const cx = right ? pos.x + SIZE - w : pos.x;
    const cy = below ? pos.y + SIZE + 8 : pos.y - h - 8;
    chat.style.width = `${w}px`; chat.style.height = `${h}px`;
    chat.style.transform = `translate(${Math.min(Math.max(cx, 8), innerWidth - w - 8)}px, ${Math.min(Math.max(cy, 8), innerHeight - h - 8)}px)`;
    const bw = Math.min(280, innerWidth - 16);
    bubble.style.maxWidth = `${bw}px`;
    const bx = right ? pos.x - bw - 6 : pos.x + SIZE + 6;
    bubble.style.transform = `translate(${Math.min(Math.max(bx, 8), innerWidth - bw - 8)}px, ${Math.max(pos.y - 4, 8)}px)`;
    bubble.classList.toggle('from-right', right);
  }
  chrome.storage.local.get(POS_KEY).then(stored => { if (stored[POS_KEY]) pos = stored[POS_KEY]; place(); }, place);
  addEventListener('resize', place);

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
    if (moved) { void chrome.storage.local.set({[POS_KEY]: pos}); return; }
    toggleChat();
  });
  body.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); toggleChat(); } });

  // ---------- mood ----------
  let moodTimer = 0;
  function setMood(mood, ms = 0) {
    char.dataset.mood = mood;
    clearTimeout(moodTimer);
    if (ms) moodTimer = setTimeout(() => { char.dataset.mood = 'idle'; }, ms);
  }
  setMood('idle');

  // ---------- bubble ----------
  let bubbleTimer = 0;
  function say(text, {button = '', onClick = null, ms = 14000, mood = 'waving'} = {}) {
    if (!chat.hidden) return;
    $('.bubble-text').textContent = text;
    const go = $('.bubble-go');
    go.textContent = button;
    go.hidden = !button;
    go.onclick = event => { event.stopPropagation(); hideBubble(); onClick?.(); };
    bubble.onclick = () => { hideBubble(); onClick ? onClick() : openChat(); };
    bubble.hidden = false;
    placeFloating();
    setMood(mood, 2400);
    clearTimeout(bubbleTimer);
    if (ms) bubbleTimer = setTimeout(hideBubble, ms);
  }
  function hideBubble() { bubble.hidden = true; }

  // ---------- chat ----------
  function toggleChat() { chat.hidden ? openChat() : closeChat(); }
  async function openChat() {
    hideBubble();
    chat.hidden = false;
    char.classList.add('open');
    placeFloating();
    await refreshSession();
    if (signedIn) {
      input.focus();
      const key = `buddyBrief:${user?.id || ''}`;
      const stored = await chrome.storage.local.get(key);
      if (stored[key] !== today && !log.childElementCount) {
        await chrome.storage.local.set({[key]: today});
        runBrief();
      }
    }
  }
  function closeChat() { chat.hidden = true; char.classList.remove('open'); }

  async function refreshSession() {
    const state = await call({op: 'session'});
    signedIn = Boolean(state?.authenticated);
    user = state?.user || null;
    loginForm.hidden = signedIn;
    $('.composer-wrap').hidden = !signedIn;
    if (!signedIn && state?.unavailable) $('.login-error').textContent = 'Backend недоступен. Проверьте, что стенд запущен.';
  }

  loginForm.addEventListener('submit', async event => {
    event.preventDefault();
    const data = new FormData(loginForm);
    const result = await call({op: 'login', email: String(data.get('email')), password: String(data.get('password'))});
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
    for (const line of text.split('\n')) {
      const item = /^\s*(?:[-*•]|\d+\.)\s+(.*)$/.exec(line);
      if (item) { if (!list) { html += '<ul>'; list = true; } html += `<li>${inline(item[1])}</li>`; continue; }
      if (list) { html += '</ul>'; list = false; }
      if (line.trim()) html += `<p>${inline(line)}</p>`;
    }
    return html + (list ? '</ul>' : '');
  }
  const el = (tag, cls, text) => { const n = document.createElement(tag); if (cls) n.className = cls; if (text != null) n.textContent = text; return n; };
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
        for (const s of view.sources) {
          const li = el('li'); const a = el('a', '', s.title);
          a.href = s.url; a.target = '_blank'; a.rel = 'noreferrer';
          li.append(a); if (s.snippet) li.append(el('span', 'snip', `«${s.snippet}»`));
          ul.append(li);
        }
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
      const outcome = await call({op, actionId: c.id});
      result.textContent = outcome?.text || 'Не удалось связаться с расширением.';
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
        const r = await call({op: 'feedback', messageId, rating});
        note.textContent = r?.ok ? 'Спасибо!' : 'Не сохранилось';
      };
    }
    row.append(note);
    return row;
  }

  let queued = null; // question asked while the brief is still streaming: sent right after it
  function setBusy(value) {
    busy = value;
    sendButton.disabled = value;
    if (!value && queued) { const next = queued; queued = null; ask(next); }
  }

  function finish(message, render) {
    if (message.op === 'view') { render(message.view); if (message.view.finished) setBusy(false); return; }
    setBusy(false);
    if (message.op === 'signed-out') { void refreshSession(); }
  }

  function runBrief() {
    setBusy(true);
    const render = assistantBubble('brief');
    current = stream({op: 'brief'}, message => {
      if (message.op === 'unsupported') { setBusy(false); log.lastElementChild?.remove(); return; }
      finish(message, render);
    });
  }

  function pageContext() {
    if (!usePage.checked) return null;
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
    const page = pageContext();
    const userNode = el('div', 'msg user', text);
    if (page) userNode.append(el('span', 'ctx', page.selection ? '📄 страница + выделенный текст' : '📄 с учётом страницы'));
    log.append(userNode);
    setSelection('');
    setBusy(true);
    const render = assistantBubble();
    current = stream({op: 'chat', message: text, pageContext: page}, message => finish(message, render));
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
    if (port && current) port.postMessage({op: 'abort'});
    setBusy(false);
    await call({op: 'new-dialog'});
    log.replaceChildren();
    input.focus();
  };
  $('.wide').onclick = () => { void call({op: 'open-panel'}); };
  $('.hide').onclick = event => {
    event.stopPropagation();
    sessionStorage.setItem(HIDDEN_KEY, '1');
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
  document.addEventListener('mouseup', event => {
    if (event.composedPath().includes(host)) return;
    setTimeout(() => {
      const text = String(getSelection() || '').trim().replace(/\s+/g, ' ');
      if (text.length < 12) return;
      setSelection(text.slice(0, 2000));
      if (chat.hidden) {
        say('Спросить меня про выделенный текст?', {button: 'Это актуально?', mood: 'curious', ms: 8000,
          onClick: async () => { await openChat(); ask('Это ещё актуально?'); }});
      }
    }, 0);
  });

  // ---------- proactive ----------
  chrome.runtime.onMessage.addListener(message => {
    if (message?.op === 'remove') host.remove();
    if (message?.op === 'toggle') { // toolbar icon: show Бади (even if hidden with ×) and open/close the chat
      sessionStorage.removeItem(HIDDEN_KEY);
      if (!host.isConnected) { mount(); place(); void openChat(); } else toggleChat();
    }
  });

  async function greet() {
    await refreshSession();
    const shownKey = 'buddyGreeting';
    const stored = await chrome.storage.local.get(shownKey);
    if (stored[shownKey] !== today) {
      await chrome.storage.local.set({[shownKey]: today});
      const name = user?.name ? `, ${user.name}` : '';
      say(signedIn ? `Привет${name}! У меня есть сводка на сегодня.` : 'Привет! Я Бади, помогу с онбордингом. Войди, и начнём.',
        {button: signedIn ? 'Показать' : 'Войти', onClick: openChat});
      return;
    }
    const hint = (await call({op: 'hint', title: document.title, url: location.href}))?.hint;
    const hintKey = `aiBuddyHint:${location.pathname}`;
    if (hint && signedIn && !sessionStorage.getItem(hintKey)) {
      sessionStorage.setItem(hintKey, '1');
      say(hint.text, {button: hint.question ? 'Да, подскажи' : '', mood: 'curious', onClick: async () => {
        await openChat();
        if (hint.question) ask(hint.question);
      }});
    }
  }
  setTimeout(() => { void greet(); }, 1200);

  async function pollRequests() {
    if (document.hidden) return;
    const {changes = []} = await call({op: 'requests'}) || {};
    const last = changes[changes.length - 1];
    if (!last) return;
    const good = last.status === 'approved' || last.status === 'done';
    say(`${good ? '🎉 ' : ''}${last.text}`, {mood: good ? 'happy' : 'sad', onClick: openChat});
  }
  void pollRequests();
  setInterval(() => { void pollRequests(); }, 20000);

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
    .bubble{position:fixed;left:0;top:0;background:#fff;color:#1d2330;border-radius:14px;padding:10px 12px;font-size:14px;line-height:1.4;box-shadow:0 6px 24px rgba(0,0,0,.18);border:1px solid #e1e4ea;cursor:pointer;animation:appear .25s}
    .bubble-go{all:unset;display:block;margin-top:6px;color:#1f6f50;font-weight:600;font-size:13px;cursor:pointer}
    @keyframes appear{from{opacity:0;transform:translateY(6px)}}
    .chat{position:fixed;left:0;top:0;display:flex;flex-direction:column;background:#fff;color:#1d2330;border-radius:16px;box-shadow:0 12px 40px rgba(0,0,0,.25);border:1px solid #e1e4ea;overflow:hidden;font-size:14px;line-height:1.45}
    .chat header{display:flex;align-items:center;gap:6px;padding:10px 12px;background:#1f6f50;color:#fff}
    .chat header b{font-size:15px}.tag{font-size:10px;text-transform:uppercase;background:#fff4cc;color:#5a4a00;border-radius:8px;padding:1px 6px;margin-right:auto}
    button{font:inherit;cursor:pointer;border-radius:8px;border:1px solid #c5cad3;background:#fff;color:#1d2330;padding:4px 10px}
    button:disabled{opacity:.5;cursor:default}
    .primary{background:#1f6f50;border-color:#1f6f50;color:#fff}
    .ghost{background:transparent;border-color:transparent;color:#fff;padding:2px 8px}.ghost:hover{background:rgba(255,255,255,.15)}
    .log{flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:8px;background:#f7f8fa}
    .msg{max-width:90%;padding:8px 11px;border-radius:12px;overflow-wrap:anywhere}
    .user{align-self:flex-end;background:#1f6f50;color:#fff;white-space:pre-wrap}
    .ctx{display:block;font-size:11px;opacity:.85;margin-top:2px}
    .bot{align-self:flex-start;background:#fff;border:1px solid #e6e9ee}.brief{border-left:3px solid #1f6f50}
    .bot p{margin:0 0 5px}.bot p:last-child{margin:0}.bot ul{margin:3px 0 5px;padding-left:18px}.bot sup{color:#1f6f50;font-size:10px}
    .bot code{background:#f2f4f7;padding:0 3px;border-radius:4px}
    .status{margin:0;color:#68708a;font-size:12px}.status::before{content:"";display:inline-block;width:7px;height:7px;margin-right:6px;border-radius:50%;background:#1f6f50;animation:pulse 1s infinite}
    .sources{list-style:none;margin:6px 0 0;padding:6px 0 0;border-top:1px solid #eceef2;font-size:12px}.sources a{color:#1f6f50}
    .snip{display:block;color:#68708a;font-style:italic;font-size:11px}
    .card{margin-top:8px;border:1px solid #d9dee6;border-radius:10px;padding:8px 10px;background:#fbfcfd}
    .card dl{display:grid;grid-template-columns:max-content 1fr;gap:1px 8px;margin:6px 0;font-size:12px}.card dt{color:#68708a}.card dd{margin:0}
    .summary{font-size:13px}.row{display:flex;gap:6px}
    .result{margin:6px 0 0;font-size:12px}.result[data-tone=success]{color:#1f6f50}.result[data-tone=error]{color:#a12a2a}.result[data-tone=muted]{color:#68708a}
    .fb{display:flex;gap:2px;align-items:center;margin-top:4px;font-size:11px;color:#68708a}.fb button{border:none;background:transparent;padding:0 4px}.fb .on{background:#eef1f6}
    .err{margin-top:6px;padding:5px 8px;border-radius:8px;background:#fbeaea;color:#a12a2a;font-size:12px}
    .login{padding:16px;display:grid;gap:8px}.login p{margin:0}.login input{font:inherit;padding:7px 10px;border:1px solid #c5cad3;border-radius:8px}
    .login-error{color:#a12a2a;font-size:12px}
    .composer-wrap{border-top:1px solid #e6e9ee;padding:6px 10px 10px;background:#fff}
    .page{display:flex;gap:6px;align-items:center;font-size:11px;color:#68708a;margin-bottom:4px}
    .chip{display:flex;align-items:center;gap:4px;background:#eef6f2;border-radius:8px;padding:3px 6px;font-size:11px;margin-bottom:4px}
    .chip-text{flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.chip-x{border:none;background:transparent;padding:0 4px}
    .composer{display:flex;gap:6px;align-items:flex-end}
    .composer textarea{flex:1;resize:none;font:inherit;padding:7px 10px;border:1px solid #c5cad3;border-radius:10px;min-height:38px;max-height:120px;color:#1d2330;background:#fff}
    .send{padding:7px 12px}
    @media (prefers-color-scheme:dark){
      .chat,.composer-wrap,.bubble{background:#1c2027;color:#e6e8ee;border-color:#2c323c}
      .log{background:#14171c}.bot{background:#222830;border-color:#2c323c}.card{background:#1c2027;border-color:#2c323c}
      .composer textarea,.login input{background:#14171c;color:#e6e8ee;border-color:#2c323c}
      button{background:#222830;color:#e6e8ee;border-color:#2c323c}.primary{background:#2f9b70;border-color:#2f9b70;color:#fff}
      .sources a,.bubble-go,.bot sup{color:#5fd0a0}.chip{background:#1f3a2f}.bot code{background:#14171c}
    }`;
  }
})();
