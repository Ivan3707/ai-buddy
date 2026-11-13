import { test, expect, chromium, type Page } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import path from 'node:path';

function requiredCredential(name: 'E2E_BUDDY_EMAIL' | 'E2E_BUDDY_PASSWORD') {
  const value = process.env[name];
  if (!value) throw new Error(`${name} must be set for authenticated E2E tests`);
  return value;
}

async function login(page: Page, email: string, password: string) {
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Пароль').fill(password);
  await page.getByRole('button', { name: 'Войти' }).click();
}

test('buddy creates an intern, intern opens checklists and uses AI assistant', async ({ page }) => {
  const buddyEmail = requiredCredential('E2E_BUDDY_EMAIL');
  const buddyPassword = requiredCredential('E2E_BUDDY_PASSWORD');
  const unique = randomUUID();
  const internEmail = `e2e-intern-${unique}@example.com`;
  const internPassword = `E2e-${unique}-password!`;

  await page.goto('/');
  await login(page, buddyEmail, buddyPassword);

  const createIntern = page.getByRole('region', { name: 'Создать стажёра' });
  await expect(createIntern).toBeVisible();
  await createIntern.getByLabel('Email').fill(internEmail);
  await createIntern.getByLabel('Временный пароль').fill(internPassword);
  await createIntern.getByRole('button', { name: 'Создать стажёра' }).click();
  await expect(createIntern.getByText(internEmail, { exact: false })).toBeVisible();

  await page.getByRole('button', { name: 'Выйти' }).click();
  await expect(page.getByRole('button', { name: 'Войти' })).toBeVisible();
  await login(page, internEmail, internPassword);

  const commonChecklist = page.getByRole('region', { name: 'Общий чек-лист' });
  const professionalChecklist = page.getByRole('region', { name: 'Профессиональный чек-лист' });
  await expect(commonChecklist).toBeVisible();
  await expect(professionalChecklist).toBeVisible();
  await expect(commonChecklist.getByRole('article').first()).toBeVisible();
  await expect(professionalChecklist.getByRole('article').first()).toBeVisible();
  await expect(commonChecklist.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0');
  await expect(professionalChecklist.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '0');

  await page.getByRole('button', { name: 'Спросить AI-помощника' }).click();
  const assistant = page.getByRole('region', { name: 'AI-помощник' });
  await assistant.getByLabel('Ваш вопрос').fill('Как оформить отпуск?');
  await assistant.getByRole('button', { name: 'Отправить' }).click();
  await expect(assistant.getByRole('link', { name: 'Заявление на отпуск' })).toHaveAttribute('href', 'https://hr.example.com/leave');
});

test('real MV3 side panel logs in and calls authenticated chat', async () => {
  const buddyEmail = requiredCredential('E2E_BUDDY_EMAIL');
  const buddyPassword = requiredCredential('E2E_BUDDY_PASSWORD');
  const extension = path.resolve('apps/extension/dist');
  const context = await chromium.launchPersistentContext('', {
    channel: 'chromium',
    headless: true,
    args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  });
  try {
    const worker = context.serviceWorkers()[0] || await context.waitForEvent('serviceworker');
    const id = worker.url().split('/')[2];
    let page = await context.newPage();
    await page.goto(`chrome-extension://${id}/sidepanel.html`);

    await page.getByLabel('Email').fill(buddyEmail);
    await page.getByLabel('Пароль').fill(buddyPassword);
    await page.getByRole('button', { name: 'Войти' }).click();
    await expect(page.getByRole('button', { name: 'Выйти' })).toBeVisible();

    await page.close();
    page = await context.newPage();
    await page.goto(`chrome-extension://${id}/sidepanel.html`);
    await expect(page.getByRole('button', { name: 'Выйти' })).toBeVisible();
    await expect.poll(() => page.evaluate(async () => (await chrome.storage.session.get('accessToken')).accessToken)).not.toBe('');

    await page.getByLabel('Ваш вопрос').fill('Как оформить отпуск?');
    await page.getByRole('button', { name: 'Отправить' }).click();
    await expect(page.getByRole('link', { name: 'Заявление на отпуск (демо)' })).toHaveAttribute('href', 'https://hr.example.com/leave');

    await page.getByRole('button', { name: 'Выйти' }).click();
    await expect(page.getByRole('button', { name: 'Войти' })).toBeVisible();
    await expect.poll(() => page.evaluate(async () => (await chrome.storage.session.get('accessToken')).accessToken)).toBeUndefined();
  } finally {
    await context.close();
  }
});
