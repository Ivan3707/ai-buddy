// Content script: mounts the shared Бади widget (buddy/widget.js, injected right before this file) on the page.
// A pure view: every backend call goes through the background worker, which also holds the token.
// On the AI-buddy web app it replaces the site's own widget and accepts the site's nudges (see bridge below).
(() => {
  if (globalThis.__aiBuddyCharacter) return; // injected twice (registration + executeScript)
  globalThis.__aiBuddyCharacter = true;
  if (!globalThis.AIBuddyWidget) return;

  const HIDDEN_KEY = 'aiBuddyHidden';
  const hiddenOnTab = sessionStorage.getItem(HIDDEN_KEY) === '1'; // "×" on the character: until the tab closes

  const call = message => chrome.runtime.sendMessage(message).catch(() => ({error: 'extension reloaded'}));
  let port = null;
  const handlers = new Map();
  let seq = 0;
  function stream(payload, onMessage) {
    if (!port) {
      port = chrome.runtime.connect({name: 'buddy'});
      port.onMessage.addListener(message => handlers.get(message.requestId)?.(message));
      port.onDisconnect.addListener(() => { port = null; });
    }
    const requestId = ++seq;
    handlers.set(requestId, onMessage);
    port.postMessage({...payload, requestId});
    return () => port?.postMessage({op: 'abort'});
  }
  const storage = {
    get: key => chrome.storage.local.get(key).then(stored => stored[key]),
    set: (key, value) => chrome.storage.local.set({[key]: value}),
  };

  const buddy = globalThis.AIBuddyWidget.mount({
    call, stream, storage,
    features: {login: true, pageContext: true, hide: true, requests: true},
    onHide: () => sessionStorage.setItem(HIDDEN_KEY, '1'),
  });
  if (!hiddenOnTab) buddy.show();

  chrome.runtime.onMessage.addListener(message => {
    if (message?.op === 'remove') buddy.remove();
    if (message?.op === 'toggle') { // toolbar icon: show Бади (even if hidden with ×) and open/close the chat
      sessionStorage.removeItem(HIDDEN_KEY);
      if (!buddy.isShown()) { buddy.show(); void buddy.open(); } else buddy.toggle();
    }
  });

  // ---------- bridge with the AI-buddy web app ----------
  // The site hides its own Бади when it sees the extension and sends its nudges here instead
  // ("задача идёт дольше плана", "наставник ответил"). Only the site's own window on its own origin is heard,
  // only plain text is accepted, and the text is rendered via textContent.
  const WEB_APP = /^(127\.0\.0\.1|localhost):5173$/;
  if (!WEB_APP.test(location.host)) return;
  document.documentElement.dataset.aiBuddyExtension = '1';
  dispatchEvent(new CustomEvent('ai-buddy:extension'));
  const str = (value, max) => (typeof value === 'string' ? value.slice(0, max) : '');
  addEventListener('message', event => {
    if (event.source !== window || event.origin !== location.origin) return;
    const m = event.data;
    if (!m || m.source !== 'ai-buddy-web') return;
    if (!buddy.isShown()) buddy.show();
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
