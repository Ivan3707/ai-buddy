// The Бади window: the same widget as on the pages, filling a small browser window (no character, no backdrop).
// Opened by the toolbar button on pages where content scripts cannot run: chrome://, the new tab page, the Web Store.
(() => {
  const bridge = globalThis.AIBuddyBridge;
  const buddy = globalThis.AIBuddyWidget.mount({
    call: bridge.call,
    stream: bridge.stream,
    storage: bridge.storage,
    features: {login: true, logout: true, pageContext: false, hide: false, requests: true, embedded: true},
    portalUrl: (kind, id) => `http://127.0.0.1:5173/portal/requests/${kind}/${encodeURIComponent(id)}`,
  });
  buddy.show();
  void buddy.open();
  bridge.onInvalidated(() => buddy.remove());
})();
