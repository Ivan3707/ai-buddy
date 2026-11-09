import { test, expect, chromium } from '@playwright/test';
import path from 'node:path';

test('web → backend → AI and role onboarding', async ({page, request}) => {
  await page.goto('/');
  await expect(page.getByRole('heading', {name:'Общий онбординг',exact:true})).toBeVisible();
  await expect(page.getByRole('heading', {name:'Онбординг ML-инженера'})).toBeVisible();
  await page.getByRole('checkbox').first().check();
  await expect(page.getByRole('checkbox').first()).toBeChecked();
  await page.getByRole('combobox').selectOption('backend');
  await expect(page.getByRole('heading', {name:'Онбординг backend-разработчика'})).toBeVisible();
  await expect(page.getByRole('heading', {name:'Онбординг ML-инженера'})).toHaveCount(0);
  await page.getByRole('button', {name:'Отправить'}).click();
  await expect(page.getByRole('link', {name:'Заявление на отпуск (демо-ссылка)'})).toHaveAttribute('href','https://hr.example.com/leave');
  await page.getByLabel('Ваш вопрос').fill('xyz');
  await page.getByRole('button', {name:'Отправить'}).click();
  await expect(page.getByText('В демо-базе нет ответа.', {exact:false})).toBeVisible();
  const invalid = await request.post('/api/chat', {data:{message:''}});
  expect(invalid.status()).toBe(400);
});

test('real MV3 popup → backend → AI', async () => {
  const extension = path.resolve('apps/extension/dist');
  const context = await chromium.launchPersistentContext('', {
    channel:'chromium', headless:true,
    args:[`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  });
  try {
    const worker = context.serviceWorkers()[0] || await context.waitForEvent('serviceworker');
    const id = worker.url().split('/')[2];
    const page = await context.newPage();
    await page.goto(`chrome-extension://${id}/popup.html`);
    await page.getByRole('button', {name:'Отправить'}).click();
    await expect(page.getByRole('link', {name:'Заявление на отпуск (демо)'})).toHaveAttribute('href','https://hr.example.com/leave');
  } finally { await context.close(); }
});
