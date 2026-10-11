// Renders PNG exports of the logo set with Chromium. NODE_PATH=$(npm root -g) node tools/render-brand.cjs
const { chromium } = require('playwright');
const { readFileSync, writeFileSync } = require('node:fs');
const { join } = require('node:path');
const root = join(__dirname, '..');
const jobs = [
  ['brand/logo/app-icon.svg', 'brand/png/app-icon-1024.png', 1024],
  ['brand/logo/app-icon.svg', 'brand/png/apple-touch-icon-180.png', 180],
  ['brand/logo/maskable.svg', 'brand/png/maskable-512.png', 512],
  ['brand/logo/maskable.svg', 'brand/png/maskable-192.png', 192],
  ['brand/logo/mark.svg', 'brand/png/icon-512.png', 512],
  ['brand/logo/mark.svg', 'brand/png/icon-192.png', 192],
  ['brand/logo/favicon.svg', 'brand/png/favicon-32.png', 32],
  ['brand/logo/favicon-16.svg', 'brand/png/favicon-16.png', 16],
  ['brand/logo/favicon.svg', 'brand/png/favicon-48.png', 48],
];
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage();
  for (const [src, out, size] of jobs) {
    const svg = readFileSync(join(root, src), 'utf8');
    await page.setViewportSize({ width: size, height: size });
    await page.setContent(`<html><body style="margin:0;background:transparent">
      <img style="display:block;width:${size}px;height:${size}px" src="data:image/svg+xml;base64,${Buffer.from(svg).toString('base64')}"></body></html>`);
    await page.waitForTimeout(50);
    await page.screenshot({ path: join(root, out), omitBackground: true });
    console.log(out);
  }
  await browser.close();
})();
