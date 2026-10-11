// Exports static PNGs of every screen and state into exports/.
// NODE_PATH=$(npm root -g) node tools/export-screens.cjs  (then python3 tools/optimize-png.py exports)
const { chromium } = require('playwright');
const { pathToFileURL } = require('node:url');
const { resolve, join } = require('node:path');
const { mkdirSync, writeFileSync } = require('node:fs');

// [name, hash, overlay?]  overlay = viewport-only capture (sheets, dialogs, palette)
const S = [
  ['01-signin', '#/signin'], ['01-signin-wrong-password', '#/signin?state=error'], ['01-signin-rate-limited', '#/signin?state=limited'],
  ['02-for-you', '#/for-you'], ['02-for-you-trakt-not-connected', '#/for-you?state=trakt-off'], ['02-for-you-refreshing', '#/for-you?state=refreshing'], ['02-for-you-first-time', '#/for-you?state=empty'],
  ['03-ask-start', '#/ask?new=1'], ['03-ask-working', '#/ask?thread=lighter&state=working'], ['03-ask-results', '#/ask?thread=lighter'],
  ['04-discover', '#/discover'], ['04-discover-filters', '#/discover?sheet=dfilters', 1], ['04-discover-nothing-matches', '#/discover?genres=Romance&anime=1&state=empty'],
  ['05-search', '#/search'], ['05-search-results', '#/search?q=fri'], ['05-search-no-results', '#/search?q=freiren'],
  ['06-movie-ready', '#/movie/dune-part-two'], ['06-movie-add', '#/movie/interstellar'], ['06-movie-find-torrent', '#/movie/past-lives'], ['06-movie-downloading', '#/movie/perfect-days'], ['06-movie-subtitles', '#/movie/dune-part-two?sheet=subs', 1], ['06-movie-how-to-play', '#/movie/dune-part-two?sheet=howto&hid=dune-part-two', 1],
  ['07-show-severance', '#/show/severance'], ['07-show-frieren-long-season', '#/show/frieren'], ['07-show-one-piece-arcs', '#/show/one-piece'], ['07-show-doctor-who-specials', '#/show/doctor-who?s=0'], ['07-show-settings', '#/show/frieren?sheet=showsettings&sid=frieren', 1],
  ['08-torrents', '#/show/severance?sheet=torrents&ctx=severance-s02e05', 1], ['08-torrents-searching', '#/show/severance?sheet=torrents&ctx=severance-s02e05&tstate=searching', 1], ['08-torrents-partial-failure', '#/show/severance?sheet=torrents&ctx=severance-s02e05&tstate=partial', 1],
  ['08-torrents-no-results', '#/show/frieren?sheet=torrents&ctx=frieren-s01e29&tstate=none', 1], ['08-torrents-filters-hide-all', '#/show/severance?sheet=torrents&ctx=severance-s02e05&tstate=filtered', 1], ['08-torrents-filters', '#/show/severance?sheet=torrents&ctx=severance-s02e05&nested=tsfilters', 1],
  ['08-torrents-inspect', '#/show/frieren?sheet=torrents&ctx=frieren-s01e29&nested=inspect&rel=frieren-s01e29%7C0', 1], ['08-torrents-frieren', '#/show/frieren?sheet=torrents&ctx=frieren-s01e29', 1], ['08-torrents-movie', '#/movie/dune-part-two?sheet=torrents&ctx=dune-part-two', 1],
  ['09-review-needs-review', '#/review/frieren'], ['09-review-working', '#/review/frieren?state=progress'], ['09-review-resolved', '#/review/frieren?state=resolved'], ['09-review-applied', '#/review/frieren?state=applied'],
  ['09-review-episode-picker', '#/review/frieren?sheet=picker&pf=%5BSubsPlease%5D%20Sousou%20no%20Frieren%20S2%20-%2010.5%20Recap%20(1080p).mkv', 1], ['09-review-recheck-ai', '#/review/frieren?sheet=recheck', 1], ['09-review-one-piece-absolute', '#/review/one-piece'], ['09-review-doctor-who-specials', '#/review/doctor-who'],
  ['10-library', '#/library'], ['10-library-list', '#/library?view=list'], ['10-library-needs-review', '#/library?f=review'], ['10-library-empty', '#/library?state=empty'], ['10-library-remove-confirm', '#/library?confirm=past-lives', 1],
  ['11-activity', '#/activity'], ['11-activity-detail', '#/activity/b1c9e4?adv=1'], ['11-activity-backend-offline', '#/activity?state=offline'], ['11-activity-all-quiet', '#/activity?state=empty'],
  ['12-settings', '#/settings?trakt=connected'], ['12-settings-trakt-not-connected', '#/settings?trakt=off', 1], ['12-settings-trakt-code', '#/settings?trakt=code', 1], ['12-settings-trakt-waiting', '#/settings?trakt=waiting', 1], ['12-settings-trakt-first-sync', '#/settings?trakt=syncing', 1],
  ['13-system-states', '#/states'],
];
const KEY = ['02-for-you', '06-movie-ready', '07-show-frieren-long-season', '08-torrents', '09-review-needs-review', '11-activity', '10-library'];
const jobs = [];
for (const [n, h, ov] of S) { jobs.push([n, h, ov, 390, 'light']); jobs.push([n, h, ov, 1440, 'light']); }
jobs.push(['05-search-palette', '#/for-you?palette=1', 1, 1440, 'light']);
for (const n of KEY) { const [, h, ov] = S.find((s) => s[0] === n); jobs.push([n, h, ov, 768, 'light']); jobs.push([n, h, ov, 390, 'dark']); jobs.push([n, h, ov, 1440, 'dark']); }

(async () => {
  const out = resolve(__dirname, '..', 'exports');
  mkdirSync(out, { recursive: true });
  const base = pathToFileURL(resolve(__dirname, '..', 'app.html')).href;
  const browser = await chromium.launch();
  const index = [];
  for (const [name, hash, overlay, w, theme] of jobs) {
    const h = w === 390 ? 844 : w === 768 ? 1024 : 900;
    const page = await browser.newPage({ viewport: { width: w, height: h } });
    await page.goto(`${base}?theme=${theme}${w === 390 ? '&safe=1' : ''}${hash}`, { waitUntil: 'load' });
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(overlay ? 900 : 450);
    const file = `${name}-${w}${theme === 'dark' ? '-dark' : ''}.png`;
    await page.screenshot({ path: join(out, file), fullPage: !overlay });
    index.push(file);
    await page.close();
  }
  await browser.close();
  writeFileSync(join(out, 'README.md'), `# Screen exports\n\nGenerated by \`tools/export-screens.cjs\` from the prototype (\`app.html\`). File names: \`<screen>-<state>-<width>[-dark].png\`. Phone frames include simulated iPhone safe areas. Page states are full-page captures; sheets, dialogs and the palette are viewport captures.\n\n${index.map((f) => `- [${f}](${encodeURI(f)})`).join('\n')}\n`);
  console.log('exported', index.length);
})();
