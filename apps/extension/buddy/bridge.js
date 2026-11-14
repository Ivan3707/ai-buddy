// Link between a page that shows Бади (a content script or the Бади window) and the background worker.
// Classic script, no imports; defines globalThis.AIBuddyBridge.
//
// After the extension is updated or reloaded, the scripts that were already running in open tabs are orphaned:
// every chrome.* call throws "Extension context invalidated". They must stop quietly, otherwise each update leaves
// an error on the extensions page that keeps growing. So every call is guarded, and onInvalidated() lets the owner clean up.
(() => {
  if (globalThis.AIBuddyBridge) return;

  const alive = () => { try { return Boolean(chrome.runtime?.id); } catch { return false; } };

  const call = async message => {
    if (!alive()) return {error: 'extension reloaded'};
    try { return await chrome.runtime.sendMessage(message); } catch { return {error: 'extension reloaded'}; }
  };

  const storage = {
    get: async key => {
      if (!alive()) return undefined;
      try { return (await chrome.storage.local.get(key))[key]; } catch { return undefined; }
    },
    set: async (key, value) => {
      if (!alive()) return;
      try { await chrome.storage.local.set({[key]: value}); } catch { /* the extension was reloaded */ }
    },
  };

  let port = null;
  const handlers = new Map();
  let seq = 0;
  const reloaded = {
    op: 'view',
    view: {status: '', text: '', sources: [], finished: true, error: 'Расширение обновилось. Обновите страницу.', card: null, messageId: null, mood: 'sad'},
  };

  /** Streams an answer ({op: 'chat'|'brief'}) as view models from the background worker; returns abort(). */
  function stream(payload, onMessage) {
    try {
      if (!alive()) throw new Error('reloaded');
      if (!port) {
        port = chrome.runtime.connect({name: 'buddy'});
        port.onMessage.addListener(message => handlers.get(message.requestId)?.(message));
        port.onDisconnect.addListener(() => { port = null; });
      }
      const requestId = ++seq;
      handlers.set(requestId, onMessage);
      port.postMessage({...payload, requestId});
    } catch {
      port = null;
      onMessage(reloaded);
      return () => {};
    }
    return () => { try { port?.postMessage({op: 'abort'}); } catch { /* the port is gone */ } };
  }

  /** Runs cleanup() once, as soon as this script is orphaned by an extension update. */
  function onInvalidated(cleanup) {
    const timer = setInterval(() => {
      if (alive()) return;
      clearInterval(timer);
      cleanup();
    }, 3000);
  }

  globalThis.AIBuddyBridge = {alive, call, storage, stream, onInvalidated};
})();
