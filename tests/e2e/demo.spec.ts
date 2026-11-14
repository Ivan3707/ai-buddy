import { test, expect, chromium, type Page } from '@playwright/test';
import path from 'node:path';
import { readFileSync } from 'node:fs';

function initialPassword(email: string) {
  const file = process.env.E2E_CREDENTIALS_FILE;
  if (!file) throw new Error('E2E_CREDENTIALS_FILE must be set for authenticated E2E tests');
  const credentials = JSON.parse(readFileSync(file, 'utf8')) as Array<{ email: string; password: string }>;
  const account = credentials.find(item => item.email === email);
  if (!account) throw new Error(`No initial credential for ${email}`);
  return account.password;
}

async function loginWeb(page: Page, email: string, password: string) {
  await page.getByLabel('Логин, почта, номер телефона').fill(email);
  await page.getByLabel('Пароль').fill(password);
  await page.getByRole('button', { name: 'Войти' }).click();
}

async function logoutWeb(page: Page) {
  await page.getByRole('button', { name: 'Выйти' }).click();
  await expect(page.getByRole('heading', { name: 'Вход' })).toBeVisible();
}

test('one design for every role: intern, buddy and teamlead sign in and out', async ({ page }) => {
  const cases = [
    { email: 'backend.intern@demo.example', nav: 'Задачи' },
    { email: 'buddy@demo.example', nav: 'Команда' },
    { email: 'teamlead@demo.example', nav: 'Команда' },
  ];
  for (const item of cases) {
    await page.goto('/');
    await loginWeb(page, item.email, initialPassword(item.email));
    await expect(page.getByRole('heading', { name: /Привет/ })).toBeVisible();
    await page.getByRole('button', { name: item.nav, exact: true }).click();
    await expect(page.locator('ai-buddy-character')).toBeAttached(); // Бади живёт на каждой странице
    await logoutWeb(page);
  }
});

test('sign-in survives a reload and a new tab (persistent session)', async ({ page, context }) => {
  const email = 'ml.intern@demo.example';
  await page.goto('/');
  await loginWeb(page, email, initialPassword(email));
  await expect(page.getByRole('heading', { name: /Привет/ })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('heading', { name: /Привет/ })).toBeVisible();
  const other = await context.newPage();
  await other.goto('/');
  await expect(other.getByRole('heading', { name: /Привет/ })).toBeVisible();
  await logoutWeb(page);
});

test('intern: calendar, tasks, notes and profile panels', async ({ page }) => {
  const email = 'ml.intern@demo.example';
  await page.goto('/');
  await loginWeb(page, email, initialPassword(email));
  await page.getByRole('button', { name: 'Календарь', exact: true }).click();
  await expect(page.getByRole('heading', { level: 1, name: 'Календарь' })).toBeVisible();
  await page.getByRole('button', { name: 'Задачи', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Общий чек-лист' })).toBeVisible();
  await page.getByRole('button', { name: 'Профиль' }).click();
  await expect(page.getByRole('dialog', { name: 'Профиль' })).toBeVisible();
  await page.keyboard.press('Escape');
  await page.getByRole('button', { name: 'Заметки' }).click();
  await expect(page.getByRole('dialog', { name: 'Заметки' })).toBeVisible();
  await page.keyboard.press('Escape');
  await logoutWeb(page);
});

test('teamlead sees the team in the same design', async ({ page }) => {
  const email = 'teamlead@demo.example';
  await page.goto('/');
  await loginWeb(page, email, initialPassword(email));
  await page.getByRole('button', { name: 'Команда', exact: true }).click();
  await expect(page.getByRole('heading', { level: 1, name: 'Команда' })).toBeVisible();
  await expect(page.getByRole('button', { name: '+ Задача' })).toBeVisible();
  await logoutWeb(page);
});

test('real MV3 extension: the Бади window signs in through the background worker and keeps the session', async ({}, testInfo) => {
  const buddyEmail = 'buddy@demo.example';
  const buddyPassword = initialPassword(buddyEmail);
  const extension = path.resolve('apps/extension/dist');
  const stages: string[] = [];
  const stage = async (name: string, action: () => Promise<void>) => {
    stages.push(name + ': started');
    await test.step(name, action);
    stages.push(name + ': completed');
  };
  let context: Awaited<ReturnType<typeof chromium.launchPersistentContext>> | undefined;
  try {
    await stage('launch browser', async () => {
      context = await chromium.launchPersistentContext('', {
        channel: 'chromium', headless: process.env.E2E_EXTENSION_HEADED !== '1',
        args: ['--disable-extensions-except=' + extension, '--load-extension=' + extension],
      });
    });
    let id = '';
    await stage('discover service worker', async () => {
      const worker = context!.serviceWorkers()[0] || await context!.waitForEvent('serviceworker', { timeout: 15_000 });
      id = new URL(worker.url()).host;
      expect(id).toBeTruthy();
    });
    const windowUrl = () => 'chrome-extension://' + id + '/buddy-window.html';
    const send = (page: Page, message: object) => page.evaluate(m => chrome.runtime.sendMessage(m), message);
    await stage('sign in through the background worker', async () => {
      const page = await context!.newPage();
      await page.goto(windowUrl());
      const result = await send(page, { op: 'login', email: buddyEmail, password: buddyPassword });
      expect(result).toMatchObject({ ok: true });
      await page.close();
    });
    await stage('the session survives reopening the window', async () => {
      const page = await context!.newPage();
      await page.goto(windowUrl());
      await expect.poll(() => page.evaluate(async () => {
        const auth = (await chrome.storage.local.get('auth')).auth;
        return typeof auth?.access === 'string' && typeof auth?.refresh === 'string';
      })).toBe(true);
      expect(await send(page, { op: 'session' })).toMatchObject({ authenticated: true });
      await page.close();
    });
    await stage('sign out clears the session', async () => {
      const page = await context!.newPage();
      await page.goto(windowUrl());
      expect(await send(page, { op: 'logout' })).toMatchObject({ ok: true });
      await expect.poll(() => page.evaluate(async () => (await chrome.storage.local.get('auth')).auth === undefined)).toBe(true);
      await page.close();
    });
  } finally {
    await testInfo.attach('mv3-stages', { body: stages.join('\n'), contentType: 'text/plain' });
    if (context) await context.close();
  }
});
