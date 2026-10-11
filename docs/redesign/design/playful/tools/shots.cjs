// Batch screenshots: node tools/shots.cjs out-dir "w|theme|hash|name" ...   (fullPage)
const { chromium } = require('playwright');
const { pathToFileURL } = require('node:url');
const { resolve, join } = require('node:path');
(async () => {
  const [,, outDir, ...jobs] = process.argv;
  const browser = await chromium.launch();
  const base = pathToFileURL(resolve(__dirname, '..', 'app.html')).href;
  for (const job of jobs) {
    const [w, theme, hash, name, extra = '', h = '900', full = '1'] = job.split('|');
    const page = await browser.newPage({ viewport: { width: +w, height: +h }, deviceScaleFactor: 1 });
    const errs = [];
    page.on('pageerror', (e) => errs.push(e.message));
    page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });
    await page.goto(`${base}?theme=${theme}${extra}${hash}`, { waitUntil: 'load' });
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(500);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    await page.screenshot({ path: join(outDir, `${name}.png`), fullPage: full === '1' });
    console.log(name, overflow > 0 ? `OVERFLOW ${overflow}px` : 'ok', errs.length ? `ERR ${errs.join(' | ')}` : '');
    await page.close();
  }
  await browser.close();
})();
