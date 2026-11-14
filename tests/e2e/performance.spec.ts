
import { test, expect } from '@playwright/test';
import { performance } from 'node:perf_hooks';
import {
  mkdirSync,
  readFileSync,
  writeFileSync,
} from 'node:fs';
import path from 'node:path';

const WARMUP = 3;
const RUNS = 30;
const THRESHOLD_MS = 1000;

type Credential = {
  email: string;
  password: string;
};

function credentials(): Credential {
  const file = process.env.E2E_CREDENTIALS_FILE;

  if (!file) {
    throw new Error('E2E_CREDENTIALS_FILE is required');
  }

  const accounts = JSON.parse(
    readFileSync(file, 'utf8')
  ) as Credential[];

  const account = accounts.find(
    item => item.email === 'backend.intern@demo.example'
  );

  if (!account) {
    throw new Error('Test intern credentials not found');
  }

  return account;
}

function percentile(values: number[], p: number): number {
  const sorted = [...values].sort((a, b) => a - b);

  const index = Math.max(
    0,
    Math.ceil(sorted.length * p / 100) - 1
  );

  return sorted[index];
}
test.use({
  channel: 'chrome',
});
test('Web navigation performance baseline', async ({ page }) => {
  test.setTimeout(600_000);

  const account = credentials();

  await page.goto('/');

  await page.getByTestId('email').fill(account.email);
  await page.getByTestId('password').fill(account.password);
  await page.getByTestId('login').click();

  await expect(
    page.locator('.dashboard-panels')
  ).toBeVisible();

  const scenarios = [
    {
      name: 'checklists',
      nav: 'nav-checklists',
      heading: 'Мои чек-листы',
      ready: '.task-row',
    },
    {
      name: 'calendar',
      nav: 'nav-calendar',
      heading: 'Календарь',
      ready: '.calendar-grid',
    },
    {
      name: 'profile',
      nav: 'nav-profile',
      heading: 'Профиль',
      ready: '.profile-grid',
    },
  ];

  const rows: string[] = [
    'scenario,run,duration_ms',
  ];

  for (const scenario of scenarios) {
    const samples: number[] = [];

    for (let i = -WARMUP; i < RUNS; i++) {
      // Подготавливаем одинаковое исходное состояние.
      await page.getByTestId('nav-home').click();

      await expect(
        page.locator('.dashboard-panels')
      ).toBeVisible();

      const start = performance.now();

      await page.getByTestId(scenario.nav).click();

      await expect(
        page.getByRole('heading', {
          name: scenario.heading,
          level: 1,
        })
      ).toBeVisible();

      await expect(
        page.locator(scenario.ready).first()
      ).toBeVisible();

      const duration = performance.now() - start;

      await expect(
        page.locator('.notice.error')
      ).toHaveCount(0);

      // Отбрасываем прогревочные измерения.
      if (i < 0) {
        continue;
      }

      samples.push(duration);

      rows.push(
        `${scenario.name},${i + 1},${duration.toFixed(2)}`
      );
    }

    const summary = {
      scenario: scenario.name,
      samples: samples.length,
      p50_ms: Math.round(percentile(samples, 50)),
      p95_ms: Math.round(percentile(samples, 95)),
      p99_ms: Math.round(percentile(samples, 99)),
      max_ms: Math.round(Math.max(...samples)),
      over_1000ms: samples.filter(
        value => value >= THRESHOLD_MS
      ).length,
    };

    console.log(JSON.stringify(summary));
  }

  const output = path.resolve(
    'load-results/web-performance.csv'
  );

  mkdirSync(path.dirname(output), { recursive: true });

  writeFileSync(
    output,
    rows.join('\n') + '\n',
    'utf8'
  );

  console.log(`Saved measurements: ${output}`);
});
