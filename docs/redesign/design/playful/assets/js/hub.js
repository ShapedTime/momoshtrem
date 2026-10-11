/* Design hub: renders specimens with the same UI builders as the prototype, the screen viewer and the prototype frames. */
(function () {
  const U = UI;
  const { icon, momo, btn, iconBtn, chip, esc } = U;
  const $ = (s) => document.querySelector(s);
  const root = document.documentElement;
  // the S global some builders read
  window.S = window.S || { feedback: {}, added: {}, frierenReady: false, trakt: 'connected', job: {}, ts: { filters: {} } };

  // ------------------------------------------------------------------ theme toggle
  const setTheme = (t) => { root.dataset.theme = t; document.querySelectorAll('[data-hub-theme]').forEach((b) => b.setAttribute('aria-pressed', b.dataset.hubTheme === t)); try { localStorage.setItem('momo-hub-theme', t); } catch (e) { /* storage blocked */ } };
  let saved = null; try { saved = localStorage.getItem('momo-hub-theme'); } catch (e) { /* storage blocked */ }
  // An explicit theme already on <html> (set by the host viewer) wins; then the saved choice; then the OS.
  setTheme(root.dataset.theme || saved || (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'));
  document.addEventListener('click', (e) => { const b = e.target.closest('[data-hub-theme]'); if (b) setTheme(b.dataset.hubTheme); });

  const pair = (title, fn, o = {}) => `<div class="theme-pair">${['light', 'dark'].map((t) => `<div class="panel" data-theme="${t}"><div class="panel-h"><span>${title}</span><span>${t}</span></div>${fn(t)}</div>`).join('')}</div>`;

  // ------------------------------------------------------------------ mascot sheet
  const POSES = [['sleeping', 'Sleeping', 'Empty library, quiet Activity. Momo naps on the TV.'], ['peeking', 'Curious, peeking', 'Loading and waiting: Trakt approval, metadata fetch, sign-in.'], ['knocking', 'Knocking a cup off', 'Errors that are not critical: a page that broke, a failed fetch.'], ['proud', 'Proud', 'Everything mapped, ready in Infuse, fresh picks. Tail flicks.'], ['detective', 'Detective', 'The AI is researching: Ask, re-check with AI, no-results suggestions.'], ['yawn', 'Stretch & yawn', '“Nothing new tonight”, first run, caught up.'], ['avatar', 'Avatar', 'The face of Ask momoshtrem and the cat menu.']];
  $('#momo-sheet').innerHTML = POSES.map(([k, t, d]) => `<div class="momo-card">${momo(k)}<b>${t}</b><span>${d}</span></div>`).join('');

  // ------------------------------------------------------------------ colour
  const P = {
    cream: ['#FFFDF9', '#FFF8F0', '#F8ECDF', '#F0DDCC', '#E2C8B5', '#C9A994'], terracotta: ['#F79B76', '#F2895F', '#E2704D', '#D45A3A', '#B8482C', '#A93F25', '#8A321D'],
    peach: ['#FFE9E0', '#FFD3C3', '#FFB9A2', '#FFA48D', '#FF8B6E'], matcha: ['#F2F6D8', '#E3EAA0', '#CFD866', '#B7C83A', '#9DB021', '#7A900F', '#5F7A08', '#4F6B06'],
    butter: ['#FFF6D6', '#FFEDA8', '#FFE58A', '#F9D25C'], espresso: ['#3F2E27', '#33251F', '#2A1E19', '#211814', '#1A120F', '#140E0B'], sky: ['#DDEEFA', '#7EC1F2', '#3B92D1', '#1D669C'], grape: ['#ECE5FC', '#BBA6FF', '#8B6CE8', '#6343BE'], amber: ['#FFE9C4', '#F6B44A', '#E39A2A', '#9A5300'], raspberry: ['#FCE3E6', '#FF8197', '#D9435F', '#B02A47'],
  };
  const lum = (h) => { const c = h.replace('#', '').match(/../g).map((x) => parseInt(x, 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4)); return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]; };
  const ramp = (name, cols) => `<div><p class="spec-label">${name}</p><div class="ramp">${cols.map((c) => `<i style="background:${c};color:${lum(c) > 0.4 ? '#2A1C16' : '#FFF8F0'}">${c.slice(1)}</i>`).join('')}</div></div>`;
  const SEM = ['bg', 'surface', 'surface-2', 'raised', 'line', 'ink', 'ink-2', 'ink-3', 'primary', 'primary-soft', 'accent', 'butter'];
  const STATUS = [['ready', 'ready', 'Ready', 'The TV is on: playable in Infuse'], ['downloading', 'downloading', 'Downloading 42%', 'Bytes moving; also info'], ['needs_torrent', 'missing', 'Missing', 'Dashed: an empty slot'], ['review', 'review', 'Needs review', 'A human decision is waiting'], ['watched', 'watched', 'Watched', 'Done; recedes'], ['airs', 'upcoming', 'Airs Thu', 'Not out yet'], ['warning', 'warning', 'Warning', 'Suspicious, slow, partial'], ['error', 'error', 'Error', 'Failed; always with words']];
  $('#color-specimens').innerHTML = `
    <div class="h-grid c2">
      <div class="h-card"><h4>Brand</h4><p>From your icon: cream page, terracotta for the one primary action, matcha (the TV) for ready and success, butter for stickers, peach for Momo’s world.</p>
        ${ramp('Cream', P.cream)}${ramp('Terracotta · primary', P.terracotta)}${ramp('Matcha · ready, accent', P.matcha)}${ramp('Peach · Momo', P.peach)}${ramp('Butter · stickers', P.butter)}${ramp('Espresso · dark theme', P.espresso)}</div>
      <div class="h-card"><h4>Status families</h4><p>Each status has text, tint and solid. Text on its tint passes AA in both themes. No Netflix red: errors are raspberry, and always carry an icon and words.</p>
        ${ramp('Sky · downloading', P.sky)}${ramp('Grape · needs review', P.grape)}${ramp('Amber · warning', P.amber)}${ramp('Raspberry · error', P.raspberry)}</div>
    </div>
    ${pair('Semantic tokens', () => `<div class="sem">${SEM.map((s) => `<div><i style="background:var(--color-${s})"></i>${s}</div>`).join('')}</div>
      <p class="spec-label">Status</p><div class="spec-row">${STATUS.map(([k, , label]) => U.pill(k, label, { pct: 42 })).join('')}</div>
      <p class="spec-label">Text</p><p style="font:600 15px/1.4 var(--font-sans)">Ink · <span style="color:var(--color-ink-2)">Ink 2</span> · <span style="color:var(--color-ink-3)">Ink 3</span> · <span style="color:var(--color-primary-ink)">Link</span> · <span style="color:var(--color-ready)">Ready</span> · <span style="color:var(--color-review)">Review</span> · <span style="color:var(--color-error)">Error</span></p>`)}
    <div class="table-wrap"><table class="ho-table"><thead><tr><th>Status</th><th>Token</th><th>Meaning</th></tr></thead><tbody>${STATUS.map(([k, tok, label, m]) => `<tr><td>${U.pill(k, label, { sm: true, pct: 42 })}</td><td><code>--color-${tok}</code>, <code>-bg</code>, <code>-solid</code></td><td>${m}</td></tr>`).join('')}</tbody></table></div>
    <p class="h-cap">Every text/background pair is checked by the token build: <a href="tokens/CONTRAST.md">tokens/CONTRAST.md</a> (all pass AA).</p>`;

  // ------------------------------------------------------------------ type
  const TYPE = [['display', 'Display', 'Fredoka 600 · 40→56 px fluid', 'Dune: Part Two'], ['title-1', 'Title 1', 'Fredoka 600 · 30→40 px · large titles', 'Good evening'], ['title-2', 'Title 2', 'Fredoka 600 · 24 px', 'Check the episode mapping'], ['title-3', 'Title 3', 'Fredoka 600 · 20 px · shelves, cards', 'Because you loved Severance'], ['headline', 'Headline', 'Atkinson 700 · 17 px', 'S02E06 · Attila'], ['body', 'Body', 'Atkinson 400 · 16 px / 24', 'Mark leads a team at Lumon whose memories have been surgically split.'], ['callout', 'Callout', 'Atkinson 400 · 15 px', 'Open Infuse → momoshtrem → Movies on your Apple TV.'], ['label', 'Label', 'Atkinson 600 · 14 px', 'Find missing (2)'], ['caption', 'Caption', 'Atkinson 500 · 13 px', '1,284 plays · 312 ratings · 46 on watchlist'], ['micro', 'Micro', 'Atkinson 700 · 12 px', 'SIZE · SEEDERS · AGE'], ['code', 'Code', 'Atkinson Mono 400 · 13 px', '[SubsPlease] Sousou no Frieren - 05 (1080p) [8E3D5C21].mkv']];
  $('#type-specimens').innerHTML = `<div class="h-card">${TYPE.map(([k, n, spec, sample]) => `<div class="type-row"><div><b style="font:700 14px/1.2 var(--font-sans)">${n}</b><br><code>--text-${k}</code><br><code>${spec}</code></div><div class="t-${k} ${k === 'code' ? 'machine' : ''}" style="min-width:0">${k === 'code' ? U.breakable(sample) : sample}</div></div>`).join('')}</div>
    <div class="h-card" style="margin-top:14px"><h4>Long text rules</h4>
      <p><b>Release names and file paths</b> wrap only at separators: a zero-width break is inserted after <code>. _ - / ] )</code> and before <code>[ (</code>, with <code>overflow-wrap:anywhere</code> as the last resort for a single huge token (a hash), so lines never break inside a token. Rows show up to <b>3 lines</b> on phone and <b>2 on desktop</b> (enough for almost every scene name), then clamp with an ellipsis; the full name is always one tap away in Inspect. The parsed title part (“Severance.S02E05”) is bold ink, the rest is ink 2.</p>
      <div class="rel" style="margin-top:12px;max-width:420px"><h3 class="rel-name machine">${U.machine('Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', 'Severance.S02E05')}</h3></div>
      <p><b>Titles</b> wrap to 2 lines under posters, then ellipsis. <b>Episode titles</b> in rows are 1 line with ellipsis (the number and state never truncate). <b>Paths</b> (WebDAV) are mono and wrap at slashes.</p></div>`;

  // ------------------------------------------------------------------ space / radii / elevation
  const SP = [['1', 4], ['2', 8], ['3', 12], ['4', 16], ['5', 20], ['6', 24], ['8', 32], ['10', 40], ['12', 48], ['16', 64], ['20', 80]];
  $('#space-specimens').innerHTML = `<div class="h-grid c3">
    <div class="h-card"><h4>Spacing · 4 px base</h4>${SP.map(([k, v]) => `<div class="space-row"><span style="width:58px">${k} · ${v}</span><i style="width:${v * 2}px"></i></div>`).join('')}<p class="h-cap">Tailwind: <code>--spacing: 0.25rem</code>, so <code>p-4</code> is 16 px.</p></div>
    <div class="h-card"><h4>Radii · generous</h4><div class="radius-demo">${[['xs', 6], ['sm', 10], ['poster', 12], ['md', 14], ['lg', 20], ['xl', 28], ['2xl', 36], ['pill', 999]].map(([k, v]) => `<div style="border-radius:${Math.min(v, 36)}px">${k}<br>${v === 999 ? '∞' : v}</div>`).join('')}</div><p class="h-cap">Pills for buttons and chips, lg for cards, xl for sheets, poster 12.</p></div>
    <div class="h-card"><h4>Elevation · warm</h4><div class="elev-demo">${['1', '2', '3', '4'].map((k) => `<div style="box-shadow:var(--shadow-${k})">shadow-${k}</div>`).join('')}<div style="box-shadow:var(--shadow-sticker);background:var(--color-butter);color:var(--color-on-sticker)">sticker</div><div style="box-shadow:var(--shadow-lip);background:var(--color-primary);color:var(--color-on-primary)">lip</div></div><p class="h-cap">Shadows are tinted brown, never grey. The sticker and lip shadows echo the offset shadow in your drawing. Blur: glass bars 18 px, scrims 4 px.</p></div>
  </div>
  <div class="h-card" style="margin-top:14px"><h4>Z-index</h4><p><code>base 0 · raised 1 · sticky 10 · nav 20 · dropdown 30 · sheet 40 · dialog 50 · toast 60 · palette 70 · tooltip 80</code></p></div>`;

  // ------------------------------------------------------------------ motion
  const MO = [['quick · out', 'var(--duration-quick)', 'var(--ease-out)', 'Hovers, chips, toggles'], ['base · spring', 'var(--duration-base)', 'var(--ease-spring)', 'Pop-ins, ears, switches'], ['slow · out', 'var(--duration-slow)', 'var(--ease-out)', 'Content rising in'], ['sheet', 'var(--duration-sheet)', 'var(--ease-sheet)', 'Bottom sheets, side panels'], ['celebrate', 'var(--duration-celebrate)', 'var(--ease-spring)', 'Success, tail flick']];
  $('#motion-specimens').innerHTML = `<div class="motion-demo">${MO.map(([n, d, e, use]) => `<button type="button" data-motion-demo><b>${n}</b><span class="track"><span class="ball" style="transition:left ${d} ${e}"></span></span><code>${d.replace('var(', '').replace(')', '')} · ${e.replace('var(', '').replace(')', '')}</code><span class="t-caption ink-2">${use}</span></button>`).join('')}</div>
    <div class="h-grid c3" style="margin-top:14px">
      <div class="h-card"><h4>Durations</h4><p><code>instant 90 · quick 150 · base 220 · slow 320 · sheet 420 · celebrate 900 ms</code></p></div>
      <div class="h-card"><h4>Easings</h4><p><code>out (.22,1,.36,1) · in-out (.65,0,.35,1) · spring (.34,1.56,.64,1) · sheet (.32,.72,0,1)</code></p></div>
      <div class="h-card"><h4>Reduced motion</h4><p>Same timing, no travel: <code>spring → out</code>, sheets and toasts fade, skeleton shimmer and Momo’s loops stop, celebrations are 0 ms. Opt in or out per device in Settings → App.</p></div>
    </div>`;
  document.addEventListener('click', (e) => { const b = e.target.closest('[data-motion-demo]'); if (b) b.classList.toggle('run'); });

  // ------------------------------------------------------------------ icons
  const ICO = ['heart', 'compass', 'search', 'library-big', 'activity', 'sparkles', 'tv', 'tv-minimal-play', 'magnet', 'scan-search', 'plus', 'check', 'copy', 'captions', 'calendar', 'circle-dashed', 'check-check', 'triangle-alert', 'circle-alert', 'sliders-horizontal', 'arrow-up-down', 'layout-grid', 'list', 'pause', 'play', 'trash-2', 'cast', 'paw-print', 'thumbs-up', 'thumbs-down', 'eye-off', 'settings', 'log-out', 'refresh-cw', 'external-link', 'command'];
  $('#icon-grid').innerHTML = ICO.map((n) => `<div>${icon(n)}${n}</div>`).join('');

  // ------------------------------------------------------------------ components
  const T = DATA.T;
  const rel = DATA.releases['severance-s02e05'].items;
  const fig = (html, label) => `<figure>${html}<span class="state-tag">${label}</span></figure>`;
  const C = [
    ['Buttons', () => `<p class="spec-label">Variants</p><div class="spec-row">${btn('Find a torrent', { variant: 'primary', icon: 'search' })}${btn('Copy path', { variant: 'secondary', icon: 'copy' })}${btn('Find torrent', { variant: 'tonal', icon: 'sparkles' })}${btn('Change torrent', { variant: 'ghost' })}${btn('Remove…', { variant: 'danger', icon: 'trash-2' })}${btn('Remove', { variant: 'danger-solid' })}</div>
      <p class="spec-label">States · primary</p><div class="spec-row">${fig(btn('Add', { variant: 'primary', icon: 'plus' }), 'default')}${fig(btn('Add', { variant: 'primary', icon: 'plus', cls: 'is-hover' }), 'hover')}${fig(btn('Add', { variant: 'primary', icon: 'plus', cls: 'is-pressed' }), 'pressed')}${fig(btn('Add', { variant: 'primary', icon: 'plus', cls: 'is-focus' }), 'focus')}${fig(btn('Add', { variant: 'primary', icon: 'plus', disabled: true }), 'disabled')}${fig(btn('Add', { variant: 'primary', cls: 'is-loading' }), 'loading')}</div>
      <p class="spec-label">States · secondary</p><div class="spec-row">${fig(btn('Inspect', { icon: 'scan-search' }), 'default')}${fig(btn('Inspect', { icon: 'scan-search', cls: 'is-hover' }), 'hover')}${fig(btn('Inspect', { icon: 'scan-search', cls: 'is-pressed' }), 'pressed')}${fig(btn('Inspect', { icon: 'scan-search', cls: 'is-focus' }), 'focus')}${fig(btn('Inspect', { icon: 'scan-search', disabled: true }), 'disabled')}</div>
      <p class="spec-label">Sizes &amp; icon buttons</p><div class="spec-row">${btn('Small', { variant: 'primary', size: 'sm' })}${btn('Medium', { variant: 'primary' })}${btn('Large', { variant: 'primary', size: 'lg' })}${iconBtn('ellipsis', 'More')}${iconBtn('pause', 'Pause', { cls: 'filled' })}${iconBtn('thumbs-up', 'Liked', { cls: 'on' })}${iconBtn('chevron-right', 'Next', { cls: 'sm filled' })}</div>`],
    ['Inputs', () => `<div class="stack gap-4" style="max-width:420px">
      <div class="field"><label class="field-label">Username</label><input class="input" placeholder="Default"></div>
      <div class="field"><label class="field-label">Focused</label><input class="input is-focus" value="teymur"></div>
      <div class="field"><label class="field-label">Error</label><input class="input is-error" value="wrong" aria-invalid="true"><p class="field-error">${icon('circle-alert', 'ico ico-sm')}Wrong username or password.</p></div>
      <div class="field"><label class="field-label">Disabled</label><input class="input" value="Locked for 4:59" disabled><p class="field-hint">Hint text sits under the field.</p></div>
      <div class="input-wrap search-field">${icon('search')}<input class="input" value="Severance S02E05" aria-label="Search"><span class="input-end">${iconBtn('x', 'Clear', { cls: 'sm' })}</span></div>
      <select class="input" aria-label="Sort"><option>Recently added</option></select>
      <div class="row gap-6 wrap"><label class="check"><input type="checkbox" checked> 2160p <span class="count">38</span></label><label class="check"><input type="checkbox"> 720p <span class="count">29</span></label><label class="row gap-2"><input type="checkbox" class="switch" checked aria-label="On"> On</label><label class="row gap-2"><input type="checkbox" class="switch" aria-label="Off"> Off</label></div></div>`],
    ['Chips, filter chips, tags', () => `<p class="spec-label">Filter chips</p><div class="spec-row">${fig(chip('Drama'), 'default')}${fig(chip('Drama', { on: true }), 'selected')}${fig(chip('2160p', { count: 38 }), 'with count')}${fig(chip('2160p', { on: true, check: false, x: true }), 'removable')}${fig(chip('Filters · 2', { icon: 'sliders-horizontal', cls: 'outline' }), 'opens sheet')}${fig(chip('Sort: Best', { caret: true, cls: 'outline' }), 'menu')}${fig(chip('Add', { icon: 'plus', cls: 'dashed' }), 'add')}${fig(chip('BluRay', { count: 0, disabled: true }), 'disabled · 0')}</div>
      <p class="spec-label">Release attributes</p><div class="tags">${[['2160p', 'hi'], ['WEB-DL'], ['x265'], ['DV', 'hi'], ['HDR'], ['Atmos', 'hi'], ['FLUX', 'group'], ['Season 2 pack', 'pack'], ['CAM', 'err'], ['No seeders', 'warn']].map(([t, k]) => U.tag(t, k)).join('')}</div>
      <p class="spec-label">Stickers &amp; badges</p><div class="spec-row">${U.sticker('Best match', { icon: 'sparkles' })}${U.sticker('New', { tone: 'peach' })}${U.sticker('All mapped', { tone: 'matcha', icon: 'check' })}<span class="count-badge">3</span><span class="dot ready"></span><span class="dot downloading pulse"></span><span class="dot review"></span><span class="dot warning"></span><span class="dot error"></span></div>`],
    ['Segmented control & tabs', () => `<div class="stack gap-4">${U.seg([['episode', 'Episode'], ['season', 'Season pack'], ['any', 'Any']], 'episode', { label: 'Scope' })}${U.seg([['grid', 'Grid'], ['list', 'List']], 'list', { label: 'View' })}${U.tabs([['all', 'All', 14], ['movies', 'Movies', 6], ['shows', 'Shows', 8], ['review', 'Needs review', 1]], 'all', { label: 'Filter' })}${U.tabs([['0', 'Specials', 12], ['1', 'Season 1'], ['2', 'Season 2']], '2', { label: 'Seasons' })}
      <p class="t-caption ink-3">The active tab wears cat ears (springs up 220 ms; static with reduced motion).</p></div>`],
    ['Status pills', () => `<div class="spec-row">${['ready', 'downloading', 'needs_torrent', 'not_in_library', 'airs', 'watched', 'review', 'missing', 'streaming', 'idle', 'paused', 'added', 'warning', 'error'].map((k) => U.pill(k, k === 'airs' ? 'Airs Thu' : null, { pct: 42 })).join('')}</div>
      <p class="spec-label">Small (rows, cards)</p><div class="spec-row">${['watched', 'ready_short', 'downloading', 'missing', 'airs', 'review'].map((k) => U.pill(k, k === 'airs' ? 'Airs Oct 24' : k === 'downloading' ? '64%' : null, { sm: true, pct: 64 })).join('')}</div>`],
    ['Poster cards', () => `<div class="spec-row" style="align-items:flex-start">${fig(U.posterCard('severance', {}), 'default')}${fig(U.posterCard('delicious-in-dungeon', { reason: true, feedback: true }), 'AI pick + reason')}${fig(U.posterCard('frieren', { progress: 74, status: '<span class="lib-count"><span><b>28</b>/38 ready</span></span>', menu: 'noop' }), 'library')}${fig(U.posterCard('perfect-days', { status: U.pill('downloading', '63%', { sm: true, pct: 63 }) }), 'downloading')}</div>`],
    ['Landscape cards · Up next', () => `<div class="spec-wide">${DATA.upNext.slice(0, 6).map((it) => U.upCard(it)).join('')}</div>`],
    ['Episode rows', () => { const t = T.severance; return `<ol class="ep-list">${[t.seasons[1].eps[4], t.seasons[1].eps[5], t.seasons[1].eps[6], t.seasons[1].eps[8], t.seasons[1].eps[9], { s: 3, n: 1, title: 'Episode 1', state: 'upcoming', airs: 'Airs Oct 24', runtime: 52 }, { s: 0, n: 5, title: 'The Runaway Bride', state: 'review', runtime: 64 }].map((e, i) => U.epRow(e, 'severance', { next: i === 1 })).join('')}</ol><p class="spec-label">Comfortable (with stills)</p><ol class="ep-list">${U.epRow(t.seasons[1].eps[6], 'severance', { comfy: true })}</ol>`; }],
    ['Release cards · torrent results', () => `<div class="rel-list">${[rel[0], rel[2], rel[6]].map((r, i) => U.releaseCard(r, { key: `severance-s02e05|${i}` })).join('')}</div>`],
    ['Torrent activity cards', () => `<div class="tor-list">${DATA.torrents.slice(0, 2).concat(DATA.torrents[4]).map(U.torrentCard).join('')}</div>`],
    ['Banners, empty & error states', () => `<div class="stack gap-3">${U.banner('review', 'scan-search', '3 files need a quick look', 'Specials matched with low confidence.', btn('Review', { size: 'sm' }))}${U.banner('warn', 'triangle-alert', '2 indexers timed out', 'Showing results from the other 3.', btn('Retry', { size: 'sm' }))}${U.banner('err', 'server', 'Backend offline', 'Streaming new files is paused.', btn('Retry', { size: 'sm' }))}${U.banner('ok', 'circle-check', 'Everything is matched', '10 files are ready to apply.')}${U.banner('info', 'sparkles', 'Momo is picking tonight’s shelves', 'About a minute.')}</div>
      <div class="spec-wide" style="margin-top:14px"><div class="card">${U.emptyState('sleeping', 'Your library is empty', 'Momo is napping on the TV.', btn('Discover', { variant: 'primary', size: 'sm' }), { compact: true })}</div><div class="card">${U.emptyState('knocking', 'Something broke', 'Reloading usually fixes it.', btn('Reload', { size: 'sm' }), { compact: true })}</div></div>`],
    ['Progress & skeletons', () => `<div class="stack gap-4">${U.progress(42, { cls: 'striped' })}${U.progress(100, { cls: 'ready' })}${U.progress(68, { cls: 'thin watched' })}${U.progress(30, { cls: 'indeterminate' })}<div class="row gap-4 wrap">${U.ring(42)}${U.ring(80, 56)}${U.pawTrack(5, 3)}</div>${U.pawSteps([{ label: 'Fetching metadata', detail: '14 files', state: 'done' }, { label: 'Matching files', detail: '9 by rules', state: 'done' }, { label: 'Asking AI about the tricky ones', detail: 'Working…', state: 'now' }, { label: 'Ready to review', state: 'todo' }])}
      <div class="row gap-3"><div class="skel skel-poster" style="width:96px"></div><div class="stack gap-2 grow"><div class="skel skel-line" style="width:70%"></div><div class="skel skel-line" style="width:40%"></div><div class="skel skel-line"></div></div></div></div>`],
    ['Sheet, dialog, menu, toast', () => `<div class="spec-wide">${U.sheet({ id: 'demo', title: 'Pick an episode', sub: 'Bottom sheet on phone, dialog on desktop', body: `<div class="stack gap-1"><button class="pick-row on"><span class="ep-num">E33</span><span class="grow">Episode 33</span>${icon('check')}</button><button class="pick-row"><span class="ep-num">E34</span><span class="grow">Episode 34</span></button></div>`, foot: `${btn('Ignore', { variant: 'ghost' })}<span class="spacer"></span>${btn('Done', { variant: 'primary' })}` }).replace(/<div class="scrim"[^>]*><\/div>/, '')}
      <div class="stack gap-3"><div class="dialog static"><h2>Remove Past Lives?</h2><p>It disappears from your library and from Infuse. Watch history on Trakt stays.</p><div class="dialog-actions">${btn('Cancel')}${btn('Remove', { variant: 'danger-solid' })}</div></div>
      <div class="menu"><button class="menu-item">${icon('tv')}How to play</button><button class="menu-item">${icon('copy')}Copy WebDAV path</button><div class="menu-sep"></div><button class="menu-item danger">${icon('trash-2')}Remove from library…</button></div>
      <div class="toast">${momo('proud')}<div class="toast-text">Added. Momo’s on it.<small>Ready in Infuse soon.</small></div><button class="btn btn-ghost btn-sm">View</button></div>
      <div class="toast err"><span class="toast-ico">${icon('circle-alert')}</span><div class="toast-text">Couldn’t reach Trakt<small>Retrying in 30 s.</small></div></div></div></div>`],
    ['Command palette', () => `<section class="palette"><div class="palette-input">${icon('search')}<input value="fri" aria-label="Search">${U.kbd('esc')}</div><div class="palette-body"><p class="palette-group">Titles</p><a class="palette-item" aria-selected="true">${U.poster('frieren')}<span class="pi-text"><b>Frieren: Beyond Journey’s End</b><span>Show · 2023</span></span>${U.pill('added', 'In library', { sm: true })}</a><p class="palette-group">Actions</p><a class="palette-item"><span class="pi-ico">${icon('sparkles')}</span><span class="pi-text"><b>Ask momoshtrem “fri…”</b></span></a></div><div class="palette-foot"><span>${U.kbd('↑')}${U.kbd('↓')} move</span><span>${U.kbd('↵')} open</span></div></section>`],
    ['Navigation', () => `<div class="stack gap-4"><header class="topbar"><div class="topbar-inner"><a class="brand"><img class="brand-mark" src="brand/logo/mark.svg" alt=""><span class="brand-word">momoshtrem</span></a><nav class="nav">${[['For You', 'heart', 1], ['Discover', 'compass'], ['Library', 'library-big']].map(([l, i, on]) => `<a class="nav-item" ${on ? 'aria-current="page"' : ''}><span class="ears"></span>${icon(i, 'ico ico-sm')}${l}</a>`).join('')}</nav></div></header>
      <nav class="tabbar">${[['For You', 'heart', 1], ['Discover', 'compass'], ['Search', 'search'], ['Library', 'library-big'], ['Activity', 'activity']].map(([l, i, on]) => `<a class="tab" ${on ? 'aria-current="page"' : ''}><span class="tab-ico"><span class="ears"></span>${icon(i)}</span><span class="tab-label">${l}</span></a>`).join('')}</nav>
      <p class="t-caption ink-3">Desktop (≥1024): top bar with ⌘K search, Ask, and the cat menu (Settings, theme, Log out). Phone and tablet: 5-tab bar with safe-area padding; large titles; the cat menu sits top-right.</p></div>`],
  ];
  $('#components-specimens').innerHTML = C.map(([title, fn]) => `<div class="spec">${pair(title, fn)}</div>`).join('');

  // ------------------------------------------------------------------ screens viewer
  const SCR = [
    { id: 'signin', n: 'Sign in', hash: '#/signin', states: [['Default', ''], ['Password focus', '?state=password'], ['Submitting', '?state=submitting'], ['Wrong password', '?state=error'], ['Rate-limited', '?state=limited']], solves: ['Real username + password fields with autocomplete, so password managers save and fill them', 'Errors are calm and plain; Momo looks away while you type the password'] },
    { id: 'for-you', n: 'For You', key: true, hash: '#/for-you', states: [['Personal', ''], ['Trakt not connected', '?state=trakt-off'], ['Refreshing picks', '?state=refreshing'], ['First time', '?state=empty']], solves: ['Up next shows availability for every show, with the one action that makes sense', 'Titles and reasons are always visible under posters; shelves have arrows for mouse users', 'Feedback (like, dislike, not interested, seen) from the ⋯ on each pick'] },
    { id: 'ask', n: 'Ask momoshtrem', hash: '#/ask', states: [['Start', '?new=1'], ['Working', '?thread=lighter&state=working'], ['Results', '?thread=lighter']], solves: ['Streams visible steps while the agent works', 'Every card has a reason, Add to library and feedback; refine chips are follow-ups', 'Full-screen chat on phone; threads rail on desktop'] },
    { id: 'discover', n: 'Discover', hash: '#/discover', states: [['Browse', ''], ['Filters (sheet on phone)', '?sheet=dfilters'], ['Filtered', '?genres=Drama,Sci-Fi'], ['Nothing matches', '?genres=Romance&anime=1&state=empty']], solves: ['Several genres at once, year, rating, runtime, language with an Anime toggle, status', 'Filter state lives in the URL', 'Infinite scroll with skeletons that keep their box'] },
    { id: 'search', n: 'Search', hash: '#/search', states: [['Empty', ''], ['Results', '?q=fri'], ['No results', '?q=freiren'], ['⌘K palette', '?palette=1']], solves: ['One search: a dedicated tab on phone, ⌘K palette on desktop', 'Movies / Shows / In library tabs; original titles match too'] },
    { id: 'movie', n: 'Movie detail', key: true, hash: '#/movie/dune-part-two', states: [['Ready in Infuse', ''], ['Add to library', '#/movie/interstellar'], ['Find a torrent', '#/movie/past-lives'], ['Downloading', '#/movie/perfect-days'], ['Subtitles', '?sheet=subs'], ['How to play', '?sheet=howto&hid=dune-part-two']], solves: ['One primary action that moves Add → Find a torrent → Downloading → Ready in Infuse', 'Ready means a WebDAV path to copy and a how-to-play hint; no fake disabled button', 'Torrent card and subtitles sit above the fold on desktop'] },
    { id: 'show', n: 'Show detail', key: true, hash: '#/show/severance', states: [['Severance', ''], ['Frieren (38 eps)', '#/show/frieren'], ['One Piece (1,160)', '#/show/one-piece'], ['Doctor Who · specials', '#/show/doctor-who?s=0'], ['Show settings', '#/show/frieren?sheet=showsettings&sid=frieren']], solves: ['Compact 64 px rows, watched episodes collapse, ranges and jump-to-episode for long seasons', 'Arcs for 1,000+ episode anime; Specials as a season', 'Needs-review banner; ordering (Aired, Absolute, group) and specials in show settings'] },
    { id: 'torrents', n: 'Torrent search', key: true, hash: '#/show/severance?sheet=torrents&ctx=severance-s02e05', states: [['Results', ''], ['Searching', '&tstate=searching'], ['Partial failure', '&tstate=partial'], ['No results', '#/show/frieren?sheet=torrents&ctx=frieren-s01e29&tstate=none'], ['Filters hide all', '&tstate=filtered'], ['Filters (nested sheet)', '&nested=tsfilters'], ['Inspect', '#/show/frieren?sheet=torrents&ctx=frieren-s01e29&nested=inspect&rel=frieren-s01e29%7C0'], ['Movie', '#/movie/dune-part-two?sheet=torrents&ctx=dune-part-two']], solves: ['Full-height sheet with a fixed height: it never jumps while results load', 'Release names wrap at dots, attributes are chips, Add is a fixed-size button', 'Query and filters survive close and reopen; filters that hide everything say which ones', 'No horizontal scrolling, even at 320 px'] },
    { id: 'review', n: 'Mapping review', key: true, hash: '#/review/frieren', states: [['Needs review', ''], ['Working', '?state=progress'], ['Resolved', '?state=resolved'], ['Applied', '?state=applied'], ['Episode picker', '?sheet=picker&pf=%5BSubsPlease%5D%20Sousou%20no%20Frieren%20S2%20-%2010.5%20Recap.mkv'], ['Re-check with AI', '?sheet=recheck'], ['One Piece · absolute', '#/review/one-piece'], ['Doctor Who · specials', '#/review/doctor-who']], solves: ['Groups: Matched · Needs review · Specials · Extras & ignored · Unmatched', 'Every row: file, size, target episode, confidence and a short reason', 'Two panes on desktop (files ↔ episodes); stacked list and picker sheet on phone'] },
    { id: 'library', n: 'Library', hash: '#/library', states: [['Grid', ''], ['List', '?view=list'], ['Needs review', '?f=review'], ['Empty', '?state=empty'], ['Remove · confirm', '?confirm=past-lives']], solves: ['Status without hover: ready/total, watched progress, downloading', 'Remove sits in a menu and asks first'] },
    { id: 'activity', n: 'Activity', hash: '#/activity', states: [['Now', ''], ['Torrent detail', '#/activity/b1c9e4?adv=1'], ['Backend offline', '?state=offline'], ['All quiet', '?state=empty']], solves: ['Every torrent links to its title; speeds, peers, progress and what’s streaming', 'Nerd stats (piece map, readers) tucked into Advanced'] },
    { id: 'settings', n: 'Settings', hash: '#/settings', states: [['Connected', '?trakt=connected'], ['Trakt · not connected', '?trakt=off'], ['Trakt · code', '?trakt=code'], ['Trakt · waiting', '?trakt=waiting'], ['Trakt · first sync', '?trakt=syncing'], ['Playback', '?section=playback'], ['System status', '?section=system']], solves: ['Device-code Trakt connect with clear waiting and syncing states', 'AI schedule, model, budget and spend; quality profile; WebDAV + Infuse guide; system status'] },
    { id: 'states', n: 'System states', hash: '#/states', states: [['All states', '']], solves: ['Skeletons, empty and error states, toasts, dialog and progress in one place'] },
  ];
  const sizes = { phone: [390, 844], tablet: [768, 1024], desktop: [1440, 900] };
  const vs = { i: 0, s: 0, theme: 'both' };
  function hashFor(scr, st) {
    const v = scr.states[st][1];
    if (v.startsWith('#')) return v;
    if (v.startsWith('&')) return scr.hash + v;
    if (v.startsWith('?')) return scr.hash.split('?')[0] + v;
    return scr.hash;
  }
  const frame = (kind, theme, hash, scale) => {
    const [w, h] = sizes[kind];
    const src = `app.html?theme=${theme}${kind === 'phone' ? '&safe=1' : ''}${hash}`;
    return `<div class="frame"><div class="frame-cap"><b>${w}</b> · ${theme}<a href="${src}" target="_blank" rel="noopener">Open ↗</a></div>
      <div class="device ${kind}" style="width:${Math.round(w * scale) + (kind === 'desktop' ? 2 : 16)}px;height:${Math.round(h * scale) + (kind === 'desktop' ? 2 : 16)}px"><iframe title="${esc(SCR[vs.i].n)} at ${w} px, ${theme}" src="${src}" width="${w}" height="${h}" style="transform:scale(${scale})" loading="lazy"></iframe></div></div>`;
  };
  function renderViewer() {
    const scr = SCR[vs.i];
    const hash = hashFor(scr, vs.s);
    const themes = vs.theme === 'both' ? ['light', 'dark'] : [vs.theme];
    const deskW = Math.min(($('#viewer').clientWidth || 1000) - (innerWidth >= 1280 ? 274 : 0) - 4, 1100);
    const ds = Math.min(0.8, deskW / 1440);
    $('#viewer').innerHTML = `<div class="v-list" role="group" aria-label="Screens">${SCR.map((s, i) => `<button type="button" aria-pressed="${i === vs.i}" data-v="${i}"><span>${String(i + 1).padStart(2, '0')}</span>${s.n}</button>`).join('')}</div>
      <div class="v-main">
        <div class="v-head"><div><p class="h-kicker">Screen ${vs.i + 1} of ${SCR.length}${scr.key ? ' · key screen' : ''}</p><h3>${scr.n}</h3></div>${U.seg([['both', 'Both themes'], ['light', 'Light'], ['dark', 'Dark']], vs.theme, { label: 'Theme', act: 'noop' }).replace(/data-act="noop"/g, 'data-vt')}</div>
        <ul class="v-solves">${scr.solves.map((x) => `<li>${icon('circle-check', 'ico ico-sm')}<span>${x}</span></li>`).join('')}</ul>
        <div class="v-controls chips">${scr.states.map(([l], i) => `<button type="button" class="chip ${i === vs.s ? 'on' : ''}" aria-pressed="${i === vs.s}" data-vs="${i}">${l}</button>`).join('')}</div>
        <div class="frames">${themes.map((t) => frame('phone', t, hash, 0.72)).join('')}${scr.key ? frame('tablet', themes[0], hash, 0.5) : ''}</div>
        <div class="frames frames-desk" style="margin-top:24px">${themes.map((t) => frame('desktop', t, hash, ds)).join('')}</div>
      </div>`;
  }
  document.addEventListener('click', (e) => {
    const a = e.target.closest('[data-v]'); if (a) { vs.i = +a.dataset.v; vs.s = 0; renderViewer(); return; }
    const b = e.target.closest('[data-vs]'); if (b) { vs.s = +b.dataset.vs; renderViewer(); return; }
    const c = e.target.closest('[data-vt]'); if (c) { vs.theme = c.dataset.arg; renderViewer(); }
  });
  renderViewer();
  let rz; addEventListener('resize', () => { clearTimeout(rz); rz = setTimeout(renderViewer, 250); });

  // ------------------------------------------------------------------ prototypes
  const FL = [
    { n: '1', title: 'Phone · Get it ready', device: 'phone', start: '#/for-you', steps: ['For You: Frieren’s Up next says <b>Needs a torrent</b>', 'Torrent search opens pre-filled; the <b>best match</b> covers E29–E38', '<b>Inspect</b> previews files → episodes (dry run)', '<b>Add &amp; assign</b> starts the job', 'Mapping review: metadata → matching → AI', 'Resolve the E33 duplicate, <b>Apply 10 mappings</b>', '<b>Ready</b>: back on For You it says Ready to watch'] },
    { n: '2', title: 'Desktop · Ask momoshtrem', device: 'desktop', start: '#/ask?new=1', steps: ['Ask <b>“something like Severance but lighter”</b>', 'Watch the steps stream: history, web, TMDB', 'Pick <b>The Good Place</b>: <b>Add to library</b>', '<b>Find a torrent</b>: the sheet opens on the season pack'] },
    { n: '3', title: 'Settings · Connect Trakt', device: 'phone', start: '#/settings', steps: ['<b>Connect Trakt</b> (read-only)', 'A device code and trakt.tv/activate', 'Waiting for approval', 'First sync: history, ratings, watchlist, progress', '<b>See your picks</b>: For You fills in'] },
  ];
  const pv = { i: 0 };
  function renderProtos() {
    const f = FL[pv.i];
    const w = $('#protos').clientWidth || 1000;
    const scale = f.device === 'phone' ? Math.min(0.9, (w - 18) / 390) : Math.min(0.62, (w - (innerWidth >= 1024 ? 360 : 0)) / 1440);
    const [W, H] = sizes[f.device];
    const src = `app.html?flow=${f.n}&theme=${root.dataset.theme}${f.device === 'phone' ? '&safe=1' : ''}${f.start}`;
    $('#protos').innerHTML = `<div class="v-controls">${FL.map((x, i) => `<button type="button" class="chip ${i === pv.i ? 'on' : ''}" aria-pressed="${i === pv.i}" data-pv="${i}">Flow ${x.n} · ${x.title}</button>`).join('')}</div>
      <div class="proto">
        <div class="frame"><div class="frame-cap"><b>${W}</b> · ${f.device}<a href="${src}" target="_blank" rel="noopener">Open full size ↗</a></div>
          <div class="device ${f.device}" style="width:${Math.round(W * scale) + (f.device === 'desktop' ? 2 : 16)}px;height:${Math.round(H * scale) + (f.device === 'desktop' ? 2 : 16)}px"><iframe id="proto-frame" title="Prototype flow ${f.n}" src="${src}" width="${W}" height="${H}" style="transform:scale(${scale})"></iframe></div></div>
        <div class="h-card"><h4>Flow ${f.n} · ${f.title}</h4><ol class="proto-steps" style="margin-top:10px">${f.steps.map((s, i) => `<li><span class="n">${i + 1}</span><span>${s}</span></li>`).join('')}</ol>
          <div class="row gap-2" style="margin-top:14px">${btn('Restart', { icon: 'rotate-cw', size: 'sm', cls: 'proto-restart' })}</div></div>
      </div>`;
  }
  document.addEventListener('click', (e) => {
    const p = e.target.closest('[data-pv]'); if (p) { pv.i = +p.dataset.pv; renderProtos(); return; }
    if (e.target.closest('.proto-restart')) { const f = $('#proto-frame'); f.src = f.src.split('#')[0] + FL[pv.i].start; f.contentWindow.location.reload(); }
    if (e.target.closest('[data-hub-theme]')) renderProtos();
  });
  renderProtos();

  // ------------------------------------------------------------------ handoff (summary; full text in HANDOFF.md)
  const tr = (rows) => rows.map((r) => `<tr>${r.map((c) => `<td>${c}</td>`).join('')}</tr>`).join('');
  $('#handoff-body').innerHTML = `
    <h3 class="h-h3">Layout grid</h3>
    <div class="grid-demo">
      <div class="gd"><b>Phone · &lt;640</b><div class="gd-cols" style="grid-template-columns:repeat(4,1fr);gap:12px">${'<i></i>'.repeat(4)}</div><p>4 columns · 16 px gutter at 390 (fluid <code>--gutter</code>) · 12 px gaps · posters 3-up · bottom tab bar 64 px + safe area · sheets from the bottom.</p></div>
      <div class="gd"><b>Tablet · 640–1023</b><div class="gd-cols" style="grid-template-columns:repeat(8,1fr);gap:16px">${'<i></i>'.repeat(8)}</div><p>8 columns · 24 px gutter · posters 4–5-up · still the tab bar · detail pages single column with a larger hero.</p></div>
      <div class="gd"><b>Desktop · ≥1024 (1440 design)</b><div class="gd-cols" style="grid-template-columns:repeat(12,1fr);gap:20px">${'<i></i>'.repeat(12)}</div><p>12 columns · max 1360 · 40 px gutter · top bar 68 px · detail = content + 384 px sticky side column · sheets become dialogs (760 / 1120 wide).</p></div>
    </div>
    <h3 class="h-h3">Component inventory</h3>
    <div class="table-wrap"><table class="ho-table"><thead><tr><th>Component</th><th>Variants / states</th><th>Notes</th></tr></thead><tbody>${tr([
      ['Button', 'primary · secondary · tonal · ghost · danger · danger-solid; sm/md/lg; default, hover, pressed, focus, disabled, loading', 'One primary per view. sm keeps a 44 px hit area.'],
      ['IconButton', 'plain · filled · on · glass (over art)', 'Always aria-label + tooltip.'],
      ['Input / Search / Select', 'default, focus, error, disabled; leading icon; trailing clear', '16 px text (no iOS zoom).'],
      ['Checkbox · Switch · Range', 'on/off, disabled, with facet count', ''],
      ['Chip / FilterChip', 'default, selected (ink), count, removable, outline, dashed, disabled', 'Facet counts in chips.'],
      ['Tag (release attribute)', 'plain, hi (2160p, DV, Atmos), pack, group, warn, err', ''],
      ['StatusPill', 'ready, downloading %, needs torrent, missing, not in library, airs, watched, review, streaming, idle, paused, warning, error; sm', 'Icon + words always.'],
      ['Sticker · CountBadge · Dot', 'butter / peach / matcha', 'Never on artwork.'],
      ['SegmentedControl · Tabs', 'block or inline; tabs with cat ears; scroll on phone', ''],
      ['PosterCard', 'default, reason, status, progress, menu', 'Title + meta always visible.'],
      ['UpNextCard (landscape)', 'ready, downloading, needs torrent, not in library, airs, review', 'CTA follows availability.'],
      ['EpisodeRow', 'watched, next, ready, downloading, missing, upcoming, review; compact / comfy', '64 px compact.'],
      ['ReleaseCard', 'best, default, warning; Inspect · magnet · Add', 'Name wraps at separators.'],
      ['TorrentCard (activity)', 'streaming, downloading, review, idle, paused', 'Links to title.'],
      ['MapRow · EpisodePicker', 'matched, conflict, kept, ignored, unmatched; confidence H/M/L', ''],
      ['Sheet', 'bottom (phone) / dialog or side (desktop); nested; full-height', 'Stack-safe, fixed height when results load.'],
      ['Dialog · Menu · Toast · Tooltip', 'confirm / destructive; menu items incl. danger; ok / error toasts with action', ''],
      ['Skeleton · EmptyState · ErrorState · Banner', 'poster, wide, line; with Momo pose; review/warn/err/info/ok', ''],
      ['Progress · Ring · PawSteps · PawTrack', 'determinate, striped, indeterminate', ''],
      ['CommandPalette · TopBar · TabBar · CatMenu', '', '⌘K anywhere on desktop.'],
    ])}</tbody></table></div>
    <h3 class="h-h3">Interaction details</h3>
    <div class="h-grid c2">
      <div class="h-card"><h4>Sheets</h4><p>Phone: slide up 420 ms (sheet easing), grabber + close button, drag down past 30 % or flick to dismiss, scrim tap closes, body scrolls with overscroll contained. Torrent search and Mapping review open full height and keep that height while loading. Nested sheets (filters, inspect) stack above with their own back. Desktop: centred dialog (760 px, 1120 px for torrent search) or side panel. Esc closes the top layer. Focus is trapped and returned. Closing torrent search keeps the query, scope and filters for the session.</p></div>
      <div class="h-card"><h4>Sticky things</h4><p>Top bar (glass, blur 18) and tab bar always. In sheets: header and the search/scope/chips block stick; footers stick. Mapping review: the Apply bar sticks to the bottom; the episode pane sticks on desktop. Detail pages: the side column (availability, torrent, subtitles) sticks on desktop. Large phone titles scroll away.</p></div>
      <div class="h-card"><h4>Loading choreography</h4><p>Skeletons render in the final layout boxes (no shift). Torrent search shows per-indexer chips as each answers, keeps 4 skeleton cards, then swaps in results. Ask streams status steps as paw prints, then cards rise with a 50 ms stagger. Mapping jobs show the three paw steps, then the groups. Polls pause when the tab is hidden.</p></div>
      <div class="h-card"><h4>Success &amp; feedback</h4><p>Toasts bottom-centre above the tab bar (phone) or bottom-right (desktop), 4 s, with Undo where it makes sense. Big moments (all mapped, fresh picks) get proud Momo and a tail flick. Destructive actions always confirm and say what stays.</p></div>
    </div>
    <h3 class="h-h3">Data this design needs that PLAN.md §6 doesn’t have yet</h3>
    <div class="table-wrap"><table class="ho-table"><thead><tr><th>Screen</th><th>Field / endpoint</th><th>Why</th></tr></thead><tbody>${tr([
      ['For You', '<code>up-next[].still_path</code>, <code>episode_title</code>', 'Landscape Up next cards.'],
      ['For You', '<code>recommendations/home.run.model, cost_usd, finished_at</code>', '“Picks by Opus 5.5 at 04:02 · $0.61”.'],
      ['For You', '<code>/me/new-episodes</code> (or <code>up-next[].new_count</code>)', '“New episodes for your shows”.'],
      ['Detail', '<code>titles.webdav_path</code> per movie/episode', 'Copy WebDAV path; how-to-play.'],
      ['Detail', '<code>titles.trailer {site,key,duration}</code>', 'Trailer button with length.'],
      ['Show', '<code>shows/{id}/arcs</code> or episode-group names', 'One Piece arc chips.'],
      ['Torrent search', '<code>Release.display_emphasis</code> (title+SxxEyy span)', 'Bold parsed part of release names.'],
      ['Torrent search', '<code>warnings[]: {indexer, kind, ms}</code> + per-indexer progress (SSE or poll)', 'Indexer chips while searching; “2 indexers timed out”.'],
      ['Torrent search', '<code>Release.coverage {from,to}</code> in episode terms', '“Covers E29–E38”.'],
      ['Mapping', '<code>proposal.conflict_group</code>, <code>proposal.size_bytes</code>', 'Duplicate pairs; sizes in rows.'],
      ['Activity', '<code>torrent.readers[] {client, file, position, readahead, piece_window}</code>, piece bitmap', 'Streaming now and the Advanced piece map.'],
      ['Settings', '<code>/settings/ai/usage</code> split by feature', 'Spend bar.'],
      ['Settings', '<code>trakt.device {user_code, verification_url, expires_at}</code>', 'Code card and expiry.'],
      ['Sign in', '429 with <code>retry_after</code>', 'Rate-limit countdown.'],
    ])}</tbody></table></div>
    <h3 class="h-h3">Files</h3>
    <div class="dl-list"><a class="chip outline" href="tokens/tokens.css">tokens/tokens.css</a><a class="chip outline" href="tokens/theme.css">tokens/theme.css (Tailwind v4)</a><a class="chip outline" href="tokens/tokens.json">tokens/tokens.json</a><a class="chip outline" href="tokens/CONTRAST.md">tokens/CONTRAST.md</a><a class="chip outline" href="HANDOFF.md">HANDOFF.md</a><span class="chip outline">brand/ (in the repo)</span><span class="chip outline">exports/ (PNG, in the repo)</span></div>`;

  // nav highlight
  const links = [...document.querySelectorAll('.h-nav a')];
  const io = new IntersectionObserver((es) => es.forEach((e) => { if (e.isIntersecting) links.forEach((l) => l.classList.toggle('on', l.getAttribute('href') === `#${e.target.id}`)); }), { rootMargin: '-40% 0px -55% 0px' });
  document.querySelectorAll('.h-section, .h-h3[id]').forEach((s) => s.id && io.observe(s));
})();
