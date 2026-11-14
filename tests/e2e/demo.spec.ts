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
  await page.getByTestId('email').fill(email);
  await page.getByTestId('password').fill(password);
  await page.getByTestId('login').click();
}

test('role-oriented Web works for intern, buddy and teamlead', async ({ page }) => {
  const cases = [
    { email: 'backend.intern@demo.example', heading: 'Мои чек-листы', route: 'checklists' },
    { email: 'buddy@demo.example', heading: 'Команда', route: 'team' },
    { email: 'teamlead@demo.example', heading: 'Команда', route: 'team' },
  ];
  for (const item of cases) {
    await page.goto('/');
    await loginWeb(page, item.email, initialPassword(item.email));
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    await page.getByTestId(`nav-${item.route}`).click();
    await expect(page.getByRole('heading', { name: item.heading, level: 1 })).toBeVisible();
    await page.getByTestId('nav-profile').click();
    await expect(page.getByText('Сопоставление подтверждено')).toBeVisible();
    await page.getByTitle('Выйти').click();
    await expect(page.getByTestId('login')).toBeVisible();
  }
});

test('intern opens calendar, notes, requests and mock AI', async ({ page }) => {
  await page.goto('/');
  await loginWeb(page, 'ml.intern@demo.example', initialPassword('ml.intern@demo.example'));
  for (const [route, heading] of [['calendar', 'Календарь'], ['notes', 'Заметки'], ['requests', 'Корпоративные инструменты'], ['chat', 'AI-помощник']]) {
    await page.getByTestId(`nav-${route}`).click();
    await expect(page.getByRole('heading', { name: heading, level: 1 })).toBeVisible();
  }
  await page.getByPlaceholder('Напишите вопрос…').fill('Что важно сделать в первую неделю?');
  await page.getByRole('button', { name: 'Отправить' }).click();
  await expect(page.getByText('AI-Buddy', { exact: true })).toBeVisible();
});

test('Web UX exposes task details, calendar, tools and role actions without writes', async ({ page }) => {
  await page.goto('/');
  await loginWeb(page, 'backend.intern@demo.example', initialPassword('backend.intern@demo.example'));
  await page.getByTestId('nav-checklists').click();
  const firstTask = page.locator('.task-row').first();
  await expect(firstTask).toBeVisible();
  await firstTask.click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByText('Ожидаемый результат:', { exact: false })).toBeVisible();
  await page.getByRole('button', { name: 'Закрыть' }).click();
  await page.getByTestId('nav-calendar').click();
  await expect(page.locator('.calendar-grid')).toBeVisible();
  await page.getByTestId('nav-requests').click();
  await expect(page.locator('.catalog-list article').first()).toBeVisible();
  await expect(page.locator('.catalog-list button:disabled').first()).toBeVisible();
  await page.getByTitle('Выйти').click();
  await loginWeb(page, 'buddy@demo.example', initialPassword('buddy@demo.example'));
  await page.getByTestId('nav-team').click();
  await expect(page.getByRole('button', { name: '+ Стажёр' })).toHaveCount(0);
  await page.getByTitle('Выйти').click();
  await loginWeb(page, 'teamlead@demo.example', initialPassword('teamlead@demo.example'));
  await page.getByTestId('nav-team').click();
  await expect(page.getByRole('button', { name: '+ Стажёр' })).toHaveCount(0);
  await page.getByRole('button', { name: '+ Задача' }).click();
  await expect(page.getByRole('dialog').getByLabel('Позиция в профессиональном чек-листе')).toBeVisible();
  await page.getByRole('button', { name: 'Закрыть' }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator('.mobile-top')).toBeVisible();
  await page.locator('.mobile-top button').click();
  await expect(page.getByTestId('nav-calendar')).toBeVisible();
});

test('real MV3 popup logs in and calls authenticated chat', async ({}, testInfo) => {
  const buddyEmail = 'buddy@demo.example';
  const buddyPassword = initialPassword(buddyEmail);
  const extension = path.resolve('apps/extension/dist');
  const stages: string[] = [];
  const stage = async (name: string, action: () => Promise<void>) => {
    stages.push(name + ': started');
    console.info('[MV3] ' + name + ': started');
    await test.step(name, action);
    stages.push(name + ': completed');
    console.info('[MV3] ' + name + ': completed');
  };
  let context: Awaited<ReturnType<typeof chromium.launchPersistentContext>> | undefined;
  let page: Page | undefined;
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
    await stage('open popup', async () => {
      page = await context!.newPage();
      await page.goto('chrome-extension://' + id + '/popup.html');
      await expect(page.locator('#login')).toBeVisible();
    });
    await stage('login', async () => {
      await page!.locator('#email').fill(buddyEmail);
      await page!.locator('#password').fill(buddyPassword);
      await page!.locator('#login button').click();
      await expect(page!.locator('#logout')).toBeVisible();
    });
    await stage('reopen popup', async () => {
      await page!.close();
      page = await context!.newPage();
      await page.goto('chrome-extension://' + id + '/popup.html');
      await expect(page.locator('#logout')).toBeVisible();
      await expect.poll(() => page!.evaluate(async () => {
        const token = (await chrome.storage.session.get('accessToken')).accessToken;
        return typeof token === 'string' && token.length > 0;
      })).toBe(true);
    });
    await stage('request AI chat', async () => {
      await page!.locator('#message').fill('Как оформить отпуск?');
      await page!.locator('#chat button').click();
      await expect(page!.locator('#reply a')).toHaveAttribute('href', 'https://hr.example.com/leave');
    });
    await stage('logout', async () => {
      await page!.locator('#logout').click();
      await expect(page!.locator('#login')).toBeVisible();
      await expect.poll(() => page!.evaluate(async () =>
        (await chrome.storage.session.get('accessToken')).accessToken === undefined
      )).toBe(true);
    });
  } catch (error) {
    if (page && !page.isClosed()) {
      try {
        // Traces may contain fill values and Authorization headers. Save only a redacted screenshot.
        await page.evaluate(() => {
          const password = document.querySelector<HTMLInputElement>('#password');
          if (password) password.value = '';
        });
        await testInfo.attach('mv3-popup-failure', {
          body: await page.screenshot({ timeout: 5_000 }), contentType: 'image/png',
        });
      } catch {
        stages.push('screenshot: unavailable');
      }
    }
    throw error;
  } finally {
    await testInfo.attach('mv3-stages', { body: stages.join('\n'), contentType: 'text/plain' });
    if (context) await context.close();
  }
});
