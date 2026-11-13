import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  clip, collectPageContext, contextLabel, describeActiveTab, isAllowedPageUrl, LIMITS,
  readPageInTab, sanitizePageContext, sanitizeUrl,
} from '../lib/page-context.js';

function fakeChrome({tab, tabsError, scriptResult, scriptError, noScripting = false} = {}) {
  const calls = {query: [], script: []};
  return {
    calls,
    tabs: {
      async query(filter) {
        calls.query.push(filter);
        if (tabsError) throw tabsError;
        return tab ? [tab] : [];
      },
    },
    scripting: noScripting ? undefined : {
      async executeScript(options) {
        calls.script.push(options);
        if (scriptError) throw scriptError;
        return scriptResult === undefined ? [] : [{result: scriptResult}];
      },
    },
  };
}

test('only http(s) pages are allowed', () => {
  for (const url of ['https://wiki.example/page', 'http://127.0.0.1:5173/']) assert.equal(isAllowedPageUrl(url), true, url);
  for (const url of [
    'chrome://settings', 'chrome-extension://abc/sidepanel.html', 'file:///C:/secret.txt', 'edge://flags',
    'about:blank', 'view-source:https://a.example', 'data:text/html,hi', 'javascript:alert(1)', 'not a url', undefined,
  ]) assert.equal(isAllowedPageUrl(url), false, String(url));
});

test('sanitizeUrl drops credentials, fragments and secret query params', () => {
  assert.equal(
    sanitizeUrl('https://user:pass@wiki.example/p?id=7&token=abc&Access_Token=x&API_KEY=k&q=grafana#section'),
    'https://wiki.example/p?id=7&q=grafana',
  );
  assert.equal(sanitizeUrl('file:///etc/passwd'), null);
  const longQuery = `https://a.example/p?q=${'x'.repeat(3000)}`;
  assert.equal(sanitizeUrl(longQuery), 'https://a.example/p');
  const longPath = `https://a.example/${'y'.repeat(3000)}`;
  assert.equal(sanitizeUrl(longPath).length, LIMITS.url);
});

test('clip trims, collapses whitespace and cuts by code points', () => {
  assert.equal(clip('  a \n\t b  ', 10), 'a b');
  assert.equal(clip('   ', 10), undefined);
  assert.equal(clip(42, 10), undefined);
  assert.equal(clip('😀😀😀', 2), '😀😀');
});

test('sanitizePageContext enforces contract limits and omits empty fields', () => {
  assert.equal(sanitizePageContext(null), null);
  assert.equal(sanitizePageContext({url: 'chrome://newtab', title: 'x'}), null);
  assert.deepEqual(sanitizePageContext({url: 'https://a.example/x', title: '', selection: '  '}), {url: 'https://a.example/x'});
  const context = sanitizePageContext({
    url: 'https://a.example/x', title: 'T'.repeat(500), selection: 'S'.repeat(5000),
  });
  assert.equal(context.title.length, LIMITS.title);
  assert.equal(context.selection.length, LIMITS.selection);
});

test('collectPageContext reads url/title/selection through scripting', async () => {
  const chromeApi = fakeChrome({
    tab: {id: 5, url: 'https://wiki.example/a', title: 'Вики'},
    scriptResult: {url: 'https://wiki.example/a#x', title: 'Вики', selection: 'выделенный текст'},
  });
  const result = await collectPageContext(chromeApi);
  assert.deepEqual(result, {state: 'ready', context: {url: 'https://wiki.example/a', title: 'Вики', selection: 'выделенный текст'}});
  assert.deepEqual(chromeApi.calls.query[0], {active: true, currentWindow: true});
  assert.equal(chromeApi.calls.script[0].target.tabId, 5);
  assert.equal(chromeApi.calls.script[0].func, readPageInTab);
});

test('collectPageContext never injects into restricted pages', async () => {
  const chromeApi = fakeChrome({tab: {id: 1, url: 'chrome://extensions', title: 'Extensions'}});
  assert.deepEqual(await collectPageContext(chromeApi), {context: null, state: 'restricted'});
  assert.equal(chromeApi.calls.script.length, 0);
});

test('collectPageContext falls back to tab url/title when injection fails', async () => {
  const chromeApi = fakeChrome({
    tab: {id: 2, url: 'https://chromewebstore.google.com/detail/x', title: 'Store'},
    scriptError: new Error('Cannot access contents of the page'),
  });
  assert.deepEqual(await collectPageContext(chromeApi), {
    state: 'ready', context: {url: 'https://chromewebstore.google.com/detail/x', title: 'Store'},
  });
  const noScripting = fakeChrome({tab: {url: 'https://a.example/', title: ''}, noScripting: true});
  assert.deepEqual(await collectPageContext(noScripting), {state: 'ready', context: {url: 'https://a.example/'}});
});

test('collectPageContext without activeTab grant sends nothing', async () => {
  const chromeApi = fakeChrome({tab: {id: 3}, scriptError: new Error('Cannot access')});
  assert.deepEqual(await collectPageContext(chromeApi), {context: null, state: 'no-access'});
  assert.deepEqual(await collectPageContext(fakeChrome()), {context: null, state: 'no-tab'});
  assert.deepEqual(await collectPageContext(fakeChrome({tabsError: new Error('x')})), {context: null, state: 'no-tab'});
  // A page that reports a restricted url from inside (e.g. after navigation) is rejected too.
  const navigated = fakeChrome({tab: {id: 4}, scriptResult: {url: 'file:///tmp/x', title: 'x'}});
  assert.deepEqual(await collectPageContext(navigated), {context: null, state: 'restricted'});
  const emptyResult = fakeChrome({tab: {id: 4, url: 'https://a.example/'}, scriptResult: null});
  assert.deepEqual(await collectPageContext(emptyResult), {state: 'ready', context: {url: 'https://a.example/'}});
});

test('describeActiveTab previews without injecting', async () => {
  assert.deepEqual(await describeActiveTab(fakeChrome({tab: {url: 'https://wiki.example/p', title: ' Вики '}})), {state: 'ready', title: 'Вики'});
  assert.deepEqual(await describeActiveTab(fakeChrome({tab: {url: 'https://wiki.example/p', title: ''}})), {state: 'ready', title: 'wiki.example'});
  assert.deepEqual(await describeActiveTab(fakeChrome({tab: {url: 'chrome://newtab'}})), {state: 'restricted'});
  assert.deepEqual(await describeActiveTab(fakeChrome({tab: {id: 1}})), {state: 'no-access'});
  assert.deepEqual(await describeActiveTab(fakeChrome()), {state: 'no-tab'});
  assert.deepEqual(await describeActiveTab(fakeChrome({tabsError: new Error('x')})), {state: 'no-tab'});
});

test('readPageInTab reads the page globals', () => {
  globalThis.location = {href: 'https://a.example/p'};
  globalThis.document = {title: 'Док'};
  globalThis.getSelection = () => ({toString: () => 'выделение'});
  try {
    assert.deepEqual(readPageInTab(), {url: 'https://a.example/p', title: 'Док', selection: 'выделение'});
    delete globalThis.getSelection;
    assert.equal(readPageInTab().selection, '');
  } finally {
    delete globalThis.location;
    delete globalThis.document;
  }
});

test('contextLabel', () => {
  assert.equal(contextLabel(null), '');
  assert.equal(contextLabel({url: 'https://a.example/x', title: 'T'}), 'T');
  assert.equal(contextLabel({url: 'https://a.example/x'}), 'a.example');
});
