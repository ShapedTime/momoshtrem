// Screenshot helper. NODE_PATH=$(npm root -g) node tools/shot.cjs <url-or-file> <out.png> [width=1200] [height=800] [scale=1] [fullPage=0]
const { chromium } = require('playwright');
const { pathToFileURL } = require('node:url');
const { resolve } = require('node:path');
(async () => {
  const [,, src, out, w = 1200, h = 800, scale = 1, full = '0'] = process.argv;
  const url = /^(https?|file):/.test(src) ? src : pathToFileURL(resolve(src)).href;
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: +w, height: +h }, deviceScaleFactor: +scale });
  page.on('console', (m) => { if (['error', 'warning'].includes(m.type())) console.log('[console]', m.type(), m.text()); });
  page.on('pageerror', (e) => console.log('[pageerror]', e.message));
  await page.goto(url, { waitUntil: 'networkidle' });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(300);
  await page.screenshot({ path: out, fullPage: full === '1' });
  await browser.close();
  console.log('saved', out);
})();
