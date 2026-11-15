// Content script: mounts the shared Бади widget (buddy/widget.js, injected right before this file) on the page.
// A pure view: every backend call goes through the background worker, which also holds the token.
// On the AI-buddy web app it replaces the site's own widget and accepts the site's nudges (see bridge below).
(() => {
  if (globalThis.__aiBuddyCharacter) return; // injected twice (registration + executeScript)
  globalThis.__aiBuddyCharacter = true;
  if (!globalThis.AIBuddyWidget || !globalThis.AIBuddyBridge) return;

  const SITE_ORIGIN = 'http://127.0.0.1:5173';
  const VISIBLE_KEY = 'buddyVisible'; // shared by all windows: false = hidden everywhere, anything else = shown everywhere
  const POS_KEY = 'buddyPos';
  // The AI-buddy web app itself: there Бади cannot be hidden (no "×", the "hidden" flag is ignored); on other sites he can.
  const WEB_APP = /^(127\.0\.0\.1|localhost):5173$/;
  const onSite = WEB_APP.test(location.host);

  const {call, stream, storage} = globalThis.AIBuddyBridge; // guarded: orphaned scripts stay quiet after an update

  const buddy = globalThis.AIBuddyWidget.mount({
    call, stream, storage,
    features: {login: true, logout: true, pageContext: true, hide: !onSite, requests: true},
    portalUrl: (kind, id) => `${SITE_ORIGIN}/portal/requests/${kind}/${encodeURIComponent(id)}`, // pseudo corporate site of the web app
    onHide: () => storage.set(VISIBLE_KEY, false), // the "×": hidden in every window
  });
  storage.get(VISIBLE_KEY).then(visible => { if (onSite || visible !== false) buddy.show(); });
  // after an extension update this script is orphaned: remove the character and stop every timer
  globalThis.AIBuddyBridge.onInvalidated(() => buddy.remove());
  // one state for all windows: shown / hidden and the position follow what any window changes
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area !== 'local') return;
    if (changes[VISIBLE_KEY]) { if (changes[VISIBLE_KEY].newValue === false && !onSite) buddy.hide(); else buddy.show(); }
    if (changes[POS_KEY]) void buddy.syncPosition();
  });

  chrome.runtime.onMessage.addListener(message => {
    if (message?.op === 'sso-changed' && onSite) { // the extension signed in/out: the site syncs
      window.postMessage({source: 'ai-buddy-extension', op: 'changed'}, location.origin);
      return;
    }
    if (message?.op === 'toggle') { // toolbar icon: Бади is shown now; if he already was, it opens/closes the chat here
      if (!buddy.isShown()) buddy.show();
      if (message.wasShown === false) void buddy.open(); else buddy.toggle();
    }
  });

  // ---------- bridge with the AI-buddy web app ----------
  // The site hides its own Бади when it sees the extension and sends its nudges here instead
  // ("задача идёт дольше плана", "наставник ответил"). Only the site's own window on its own origin is heard,
  // only plain text is accepted, and the text is rendered via textContent.
  if (!onSite) return;
  document.documentElement.dataset.aiBuddyExtension = '1';
  dispatchEvent(new CustomEvent('ai-buddy:extension'));
  const str = (value, max) => (typeof value === 'string' ? value.slice(0, max) : '');
  addEventListener('message', event => {
    if (event.source !== window || event.origin !== location.origin) return;
    const m = event.data;
    if (!m || m.source !== 'ai-buddy-web') return;
    if (m.op === 'sso') { // single sign-on: request/reply between the site and the background worker
      const op = {handshake: 'sso-handshake', adopt: 'sso-adopt', logout: 'sso-logout'}[m.request];
      if (!op || typeof m.id !== 'string') return;
      void call({op, state: m.state, pair: m.pair, loginAt: m.loginAt}).then(reply => {
        window.postMessage({source: 'ai-buddy-extension', op: 'sso-reply', id: m.id, ...reply}, location.origin);
        void buddy.refresh();
      });
      return;
    }
    if (!buddy.isShown()) return; // hidden by the user: the site does not bring him back
    if (m.op === 'ask' && str(m.question, 500)) { void buddy.ask(str(m.question, 500)); return; }
    if (m.op !== 'say' || !str(m.text, 300)) return;
    const question = str(m.question, 500);
    const id = str(m.id, 100);
    const done = () => window.postMessage({source: 'ai-buddy-extension', op: 'done', id}, location.origin);
    void buddy.say(str(m.text, 300), {
      detail: str(m.detail, 600), queue: true, ms: 0, mood: m.mood === 'happy' ? 'happy' : 'waving',
      onClick: () => { done(); if (question) void buddy.ask(question); else void buddy.open(); },
      buttons: [
        ...(question ? [{label: 'Обсудить с Бади', onClick: () => { done(); void buddy.ask(question); }}] : []),
        {label: 'Понятно', ghost: true, onClick: done},
      ],
    });
  });
})();
