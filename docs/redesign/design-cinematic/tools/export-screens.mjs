// Export every screen/state in js/manifest.js to screens/<screen>/<state>-<width>.png
// Usage: node tools/export-screens.mjs [screenId…]
//   Needs Playwright (npx playwright, or set PLAYWRIGHT_MODULE to its index.mjs path).
// Pages render in "embed" mode (no prototype state switcher). Routes that open a
// sheet, palette or dialog are captured at viewport size; the rest as full pages
// with the fixed tab bar / action dock moved to the end of the page.
import { pathToFileURL, fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { mkdirSync, readFileSync } from 'node:fs';
import vm from 'node:vm';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const mod = process.env.PLAYWRIGHT_MODULE || 'playwright';
const { chromium } = await import(mod);

const ctx = { window: {} };
vm.runInNewContext(readFileSync(join(root, 'js/manifest.js'), 'utf8'), ctx);
const { screens } = ctx.window.MS.manifest;
const only = process.argv.slice(2);

const SIZES = [[390, 844], [768, 1024], [1440, 900]];
const EXPORT_CSS = `
  #app { position: relative; }
  .tabbar { position: absolute !important; }
  .action-dock { position: absolute !important; }
  .rail-arrows { display: flex; }
`;

const browser = await chromium.launch();
let n = 0, bad = [];
for (const s of screens) {
  if (only.length && !only.includes(s.id)) continue;
  mkdirSync(join(root, 'screens', s.id), { recursive: true });
  for (const st of s.states) {
    for (const [w, h] of SIZES) {
      if (w === 768 && !s.tablet) continue;
      const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 1, reducedMotion: 'reduce' });
      const errors = [];
      page.on('pageerror', (e) => errors.push(e.message));
      const url = `${pathToFileURL(join(root, 'prototype.html')).href}?embed=1#${st.route}`;
      await page.goto(url);
      const overlay = /[?&](sheet|palette|dialog|menu)=/.test(st.route);
      if (!overlay) await page.addStyleTag({ content: EXPORT_CSS });
      await page.waitForTimeout(1400);
      const sw = await page.evaluate(() => document.documentElement.scrollWidth);
      if (sw > w) bad.push(`${s.id}/${st.key}@${w}: scrollWidth ${sw}`);
      if (errors.length) bad.push(`${s.id}/${st.key}@${w}: ${errors.join('; ')}`);
      await page.screenshot({ path: join(root, 'screens', s.id, `${st.key}-${w}.png`), fullPage: !overlay });
      await page.close();
      n++;
    }
  }
}
await browser.close();
console.log(`exported ${n} screens`);
if (bad.length) { console.log('problems:\n' + bad.join('\n')); process.exitCode = 1; }
