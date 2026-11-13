// Page context for StreamChatRequest.page_context ({url, title?, selection?}).
// Least privilege: only "activeTab" + "scripting". Chrome grants activeTab to the tab
// that was active when the user clicked the toolbar icon; without that grant tab.url
// is undefined and executeScript fails, so nothing is sent. The backend treats the
// context as untrusted data, never as instructions.

export const LIMITS = Object.freeze({url: 2000, title: 300, selection: 2000});

// Query parameters that commonly carry secrets; they are removed before sending.
const SENSITIVE_PARAM = /^(?:access_?token|id_?token|refresh_?token|token|code|state|key|api_?key|secret|client_secret|password|passwd|pwd|auth|authorization|session|session_?id|sid|jwt|signature|sig|x-amz-signature|x-amz-credential)$/i;

export function isAllowedPageUrl(raw) {
  try {
    const url = new URL(raw);
    // chrome://, edge://, about:, file://, chrome-extension://, data:, view-source: ... are never sent.
    return url.protocol === 'https:' || url.protocol === 'http:';
  } catch {
    return false;
  }
}

// Truncates by Unicode code points so a surrogate pair is never cut in half.
export function clip(value, max) {
  if (typeof value !== 'string') return undefined;
  const trimmed = value.replace(/\s+/g, ' ').trim();
  if (!trimmed) return undefined;
  const chars = Array.from(trimmed);
  return chars.length <= max ? trimmed : chars.slice(0, max).join('');
}

export function sanitizeUrl(raw) {
  if (!isAllowedPageUrl(raw)) return null;
  const url = new URL(raw);
  url.username = '';
  url.password = '';
  url.hash = '';
  for (const name of [...url.searchParams.keys()]) {
    if (SENSITIVE_PARAM.test(name)) url.searchParams.delete(name);
  }
  if (url.href.length > LIMITS.url) url.search = '';
  if (url.href.length > LIMITS.url) return url.href.slice(0, LIMITS.url);
  return url.href;
}

export function sanitizePageContext(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const url = sanitizeUrl(raw.url);
  if (!url) return null;
  const title = clip(raw.title, LIMITS.title);
  const selection = clip(raw.selection, LIMITS.selection);
  return {url, ...(title ? {title} : {}), ...(selection ? {selection} : {})};
}

// Injected into the page by chrome.scripting.executeScript; must be self-contained.
export function readPageInTab() {
  return {
    url: location.href,
    title: document.title,
    selection: String(globalThis.getSelection?.() ?? ''),
  };
}

async function activeTab(chromeApi) {
  const [tab] = await chromeApi.tabs.query({active: true, currentWindow: true});
  return tab;
}

// Preview for the UI without injecting anything: {state, title?}.
// state: 'ready' | 'restricted' | 'no-access' | 'no-tab'
export async function describeActiveTab(chromeApi) {
  let tab;
  try { tab = await activeTab(chromeApi); } catch { return {state: 'no-tab'}; }
  if (!tab) return {state: 'no-tab'};
  if (!tab.url) return {state: 'no-access'};
  if (!isAllowedPageUrl(tab.url)) return {state: 'restricted'};
  const title = clip(tab.title, LIMITS.title) || new URL(tab.url).host;
  return {state: 'ready', title};
}

// Collects {url, title, selection} of the active tab. Resolves {context, state}.
export async function collectPageContext(chromeApi) {
  let tab;
  try { tab = await activeTab(chromeApi); } catch { return {context: null, state: 'no-tab'}; }
  if (!tab) return {context: null, state: 'no-tab'};
  if (tab.url && !isAllowedPageUrl(tab.url)) return {context: null, state: 'restricted'};

  let fromPage = null;
  if (typeof tab.id === 'number' && chromeApi.scripting?.executeScript) {
    try {
      const [injection] = await chromeApi.scripting.executeScript({target: {tabId: tab.id}, func: readPageInTab});
      fromPage = injection?.result ?? null;
    } catch {
      // No activeTab grant, or a page where scripts are blocked (e.g. the Web Store).
    }
  }
  const candidate = fromPage?.url ? fromPage : {url: tab.url, title: tab.title};
  if (!candidate.url) return {context: null, state: 'no-access'};
  const context = sanitizePageContext(candidate);
  return context ? {context, state: 'ready'} : {context: null, state: 'restricted'};
}

export function contextLabel(context) {
  if (!context) return '';
  return context.title || new URL(context.url).host;
}
