// Собирает PDF презентации из docs/presentation/slides/index.html через Chromium (Playwright).
//   node docs/presentation/build.cjs            -> AI-buddy-draft.pdf (черновик из HTML; итоговая AI-buddy.pdf собрана командой вручную)
//   node docs/presentation/build.cjs --png out  -> ещё и PNG каждого слайда в папку out (для проверки вёрстки)
// Нужен установленный @playwright/test (есть в devDependencies корня) и браузер: npx playwright install chromium.
const path = require('path');
const fs = require('fs');
const { chromium } = require('@playwright/test');

const here = __dirname;
const page = 'file:///' + path.join(here, 'slides', 'index.html').replace(/\\/g, '/');
const pngDir = process.argv.includes('--png') ? process.argv[process.argv.indexOf('--png') + 1] : null;

(async () => {
  const browser = await chromium.launch();
  const tab = await browser.newPage({ viewport: { width: 1920, height: 1080 } });
  for (const [file, review] of [['AI-buddy-draft.pdf', false]]) {
    await tab.goto(page, { waitUntil: 'networkidle' });
    await tab.evaluate(on => document.body.classList.toggle('review', on), review);
    await tab.evaluate(() => document.fonts.ready);
    await tab.pdf({ path: path.join(here, file), width: '1920px', height: '1080px', printBackground: true, preferCSSPageSize: true });
    const count = await tab.evaluate(() => [...document.querySelectorAll('.slide')].filter(s => getComputedStyle(s).display !== 'none').length);
    console.log(`${file}: ${count} slides`);
    if (pngDir && !review) {
      fs.mkdirSync(pngDir, { recursive: true });
      const slides = await tab.$$('.slide');
      let n = 0;
      for (const s of slides) {
        if (!(await s.isVisible())) continue;
        n += 1;
        await s.screenshot({ path: path.join(pngDir, `slide-${String(n).padStart(2, '0')}.png`) });
      }
    }
  }
  await browser.close();
})().catch(e => { console.error(e); process.exit(1); });
