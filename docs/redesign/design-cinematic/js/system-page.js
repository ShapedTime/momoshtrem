/* Renders system.html from the same tokens and component functions the prototype uses. */
(function () {
  const { ui, brand, icon, art, data: D } = MS;
  const esc = ui.esc;
  document.getElementById('lockup').innerHTML = brand.lockup(40);

  const sec = (id, title, intro, body) => `<section class="ds-sec" id="${id}"><header><h2 class="ds-h">${title}</h2>${intro ? `<p>${intro}</p>` : ''}</header>${body}</section>`;
  const sub = (t, d = '') => `<div class="stack gap-1"><h3 class="ds-sub">${t}</h3>${d ? `<p class="t-small c-2" style="max-width:70ch">${d}</p>` : ''}</div>`;
  const card = (label, inner, extra = '') => `<div class="ds-card" ${extra}><span class="ds-label">${label}</span>${inner}</div>`;
  const L = 'assets/logo/';

  // ---------------------------------------------------------------- brand
  const logo = sec('logo', 'Logo', 'Three directions, then one refined. The mark has to read at 16 px in a browser tab and still feel premium at 1024 px on an Apple TV-adjacent home screen.', `
    ${sub('Explorations')}
    <div class="ds-grid" style="--min:260px">
      ${[['a-night-watch', 'A · Night watch', 'A cat head with almond eyes. Ears and eyes are the whole idea. It owns the "eyes in the dark" motif that becomes the loader. <strong class="c-accent">Chosen.</strong>'],
        ['b-m-ears', 'B · m-ears', 'A lowercase m whose arches become ears. Clever, but it reads as a gate or crown, and the monoline thins out to nothing at 16 px.'],
        ['c-peek', 'C · Peek', 'Ears and eyes rising over the top edge of a screen. A charming story, but the TV outline makes it busy and it collapses into a rectangle as a favicon.']]
        .map(([f, t, d]) => card(t, `<div class="logo-tile"><img src="${L}exploration-${f}.svg" alt="${t}"></div><p class="t-small c-2">${d}</p>`)).join('')}
    </div>
    ${sub('Final set', 'Geometry on a 64-unit grid: ears at 30° from the cheek line, almond eyes tilted up at the outer corner, vertical slit pupils. The pupils drop out below 24 px.')}
    <div class="ds-grid" style="--min:150px">
      ${card('Mark · colour', `<div class="logo-tile"><img src="${L}mark.svg" alt="Mark"></div>`)}
      ${card('Mark · mono (light)', `<div class="logo-tile"><img src="${L}mark-mono.svg" alt="Mono mark"></div>`)}
      ${card('Mark · mono (dark)', `<div class="logo-tile light"><img src="${L}mark-mono-black.svg" alt="Mono mark on light"></div>`)}
      ${card('App icon · 1024 / 180', `<div class="logo-tile" style="padding:0;overflow:hidden"><img src="${L}app-icon-rounded.svg" alt="App icon" style="border-radius:22%"></div><div class="row gap-3" style="align-items:flex-end"><img src="${L}app-icon-180.svg" width="60" alt="" style="border-radius:13px"><img src="${L}app-icon-180.svg" width="40" alt="" style="border-radius:9px"><img src="${L}app-icon-180.svg" width="29" alt="" style="border-radius:7px"></div>`)}
      ${card('Maskable PWA · 512', `<div class="logo-tile" style="padding:0;overflow:hidden;position:relative"><img src="${L}maskable-512.svg" alt="Maskable icon" style="border-radius:50%"><span style="position:absolute;inset:10%;border:1px dashed rgb(255 255 255 / 0.35);border-radius:50%"></span></div><p class="t-small c-3">Dashed ring = 80% safe zone</p>`)}
      ${card('Favicon · 32 / 16', `<div class="logo-tile"><div class="row gap-6" style="align-items:center"><img src="${L}favicon-32.svg" width="32" height="32" alt=""><img src="${L}favicon-16.svg" width="16" height="16" alt=""><img src="${L}favicon-16.svg" width="64" height="64" alt="" style="image-rendering:pixelated"></div></div><p class="t-small c-3">Amber head, eyes cut out, pupils dropped. Shown at 32, 16 and 16 enlarged.</p>`)}
    </div>
    <div class="ds-grid" style="--min:320px">
      ${card('Lockup', `<div class="ds-stage" style="align-items:flex-start"><img src="${L}lockup.svg" height="48" alt="momoshtrem lockup" style="height:48px;width:auto"></div>`)}
      ${card('Wordmark · always lowercase', `<div class="ds-stage" style="align-items:flex-start"><img src="${L}wordmark.svg" alt="momoshtrem wordmark" style="height:36px;width:auto"></div><p class="t-small c-3">Geist SemiBold, tracking −3.5%, outlined.</p>`)}
      ${card('Lockup · mono on light', `<div class="ds-stage" style="background:#f4f2ee;align-items:flex-start"><img src="${L}lockup-mono-black.svg" alt="" style="height:40px;width:auto"></div>`)}
    </div>
    <p class="t-small c-3">Files: <code class="t-mono">assets/logo/*.svg</code> (mark, mark-mono, mark-currentcolor, wordmark, lockup, lockup-mono, app-icon-1024, app-icon-180, maskable-512, favicon-16, favicon-32). Clear space: one ear-height on all sides. Minimum size: mark 16 px, lockup 96 px wide.</p>`);

  const mascot = sec('mascot', 'Noir, the cat', 'Proposed name for the mascot: <strong>Noir</strong>. A black cat, film-noir dark, and in the dark all you see of it is two amber eyes. It stays understated: no full-body poses, no speech bubbles, no jokes in serious errors.', `
    <div class="ds-grid" style="--min:220px">
      ${card('Loader · eyes in the dark', `<div class="ds-stage" style="align-items:center;background:var(--ink-0)">${brand.eyes(96)}</div><p class="t-small c-2">Blinks every 3.2 s with a soft glow. Reduced motion: static eyes.</p>`)}
      ${card('Ask momoshtrem avatar', `<div class="ds-stage" style="align-items:center"><div class="row gap-4">${brand.avatar(56)}${brand.avatar(40)}${brand.avatar(28)}${brand.avatar(20)}</div></div><p class="t-small c-2">Noir's face in a warm-lit circle. Used wherever the AI speaks.</p>`)}
      ${card('Empty states · silhouettes', `<div class="ds-stage" style="align-items:center;color:var(--surface-4)"><div class="row gap-4" style="align-items:flex-end">${ui.cat('sit', 72)}${ui.cat('sleep', 72)}${ui.cat('blank', 72)}</div></div><p class="t-small c-2">Sitting (waiting), sleeping (empty library), eyes-off (filters hide everything).</p>`)}
      ${card('Peek', `<div class="ds-stage" style="align-items:center;color:var(--surface-4)">${ui.cat('peek', 120)}</div><p class="t-small c-2">Not found, no results, Trakt not connected.</p>`)}
    </div>
    <div class="ds-grid" style="--min:280px">
      ${card('Voice · do', `<ul class="stack gap-2 t-small c-2"><li>“Evening. Three steps and this page fills up.”</li><li>“Your filters hide all 27 results.”</li><li>“Picked up where you left off.”</li><li>“momoshtrem isn't responding. Infuse can still play everything that's ready.”</li></ul>`)}
      ${card('Voice · don\'t', `<ul class="stack gap-2 t-small c-2"><li>“Oopsie! Noir knocked something off the table 🐾”</li><li>“Purr-fect! Your torrent is ready!”</li><li>Cat puns in any error that costs the user something.</li></ul>`)}
    </div>`);

  // ---------------------------------------------------------------- colour
  const sw = (name, v, note = '') => `<div class="swatch"><i style="background:var(${v})"></i><div><b>${name}</b><span class="ds-label">${v}</span>${note ? `<span class="contrast" data-c="${v}"></span>` : ''}</div></div>`;
  const color = sec('color', 'Colour', 'Near-black with a cool blue-violet bias, never #000. Surfaces step up in small increments; borders are translucent so they work over artwork. Glass is reserved for chrome over imagery.', `
    ${sub('Neutrals')}
    <div class="ds-grid" style="--min:140px">${[['Ink 0 · letterbox', '--ink-0'], ['Ink 1 · canvas', '--ink-1'], ['Surface 1', '--surface-1'], ['Surface 2', '--surface-2'], ['Surface 3', '--surface-3'], ['Surface 4', '--surface-4']].map(([n, v]) => sw(n, v)).join('')}</div>
    ${sub('Text', 'Contrast measured against the canvas (--ink-1).')}
    <div class="ds-grid" style="--min:140px">${[['Text 1', '--text-1'], ['Text 2', '--text-2'], ['Text 3', '--text-3'], ['Text 4 · decorative', '--text-4'], ['Accent text', '--accent-text']].map(([n, v]) => sw(n, v, true)).join('')}</div>
    ${sub('Glass, lines and scrims')}
    <div class="ds-grid" style="--min:260px">
      ${card('Glass over artwork', `<div class="hero-mini"><div class="hero-bg">${art.backdrop('bluey')}</div><div class="glass" style="padding:14px 16px;border-radius:var(--r-lg);display:flex;gap:12px;align-items:center;width:100%">${ui.pill('ready', 'Ready in Infuse')}<span class="t-small">Bluey · S03E49</span></div></div><span class="ds-label">--glass-bg + blur(--blur-lg) saturate(170%)</span>`)}
      ${card('Scrim · bottom + left', `<div class="hero-mini"><div class="hero-bg">${art.backdrop('pluribus')}</div><div class="sc"></div><div class="stack"><span class="t-title-2">Pluribus</span><span class="t-small c-2">The brightest artwork in the set stays legible.</span></div></div><span class="ds-label">--scrim-bottom · --scrim-left (≥1024)</span>`)}
      ${card('Lines', `<div class="stack gap-3"><div style="height:1px;background:var(--line-1)"></div><span class="ds-label">--line-1 · dividers</span><div style="height:1px;background:var(--line-2)"></div><span class="ds-label">--line-2 · borders</span><div style="height:1px;background:var(--line-3)"></div><span class="ds-label">--line-3 · hover, strong</span></div>`)}
    </div>`);

  const accent = sec('accent', 'Accent decision', 'The brief asked for both candidates before committing. Same component set in each:', `
    <div class="acc-compare">
      ${['amber', 'lav'].map((k) => `<div class="acc-demo ${k === 'lav' ? 'lav' : ''}">
        <div class="row between"><span class="w-semibold">${k === 'amber' ? 'Cat-eye amber · #F2B33D' : 'Luminous lavender · #B7A6FF'}</span>${k === 'amber' ? ui.pill('accent', 'Adopted', { icon: 'check' }) : ui.pill('neutral', 'Explored')}</div>
        <svg class="eyes" width="88" height="27" viewBox="13 31 38 12" aria-hidden="true"><path d="${brand.G.eyeL}"/><path d="${brand.G.eyeR}"/></svg>
        <div class="row gap-2 wrap"><span class="btn b">${icon('search', { size: 18 })}Find a torrent</span><span class="fchip f" style="border:1px solid">2160p <span class="n">4</span></span><span class="fchip">1080p <span class="n">31</span></span></div>
        <div class="row gap-2 wrap">${ui.pill('ready', 'Ready')}${ui.pill('downloading', '42%', { pct: 42 })}${ui.pill('review', 'Needs review')}</div>
      </div>`).join('')}
    </div>
    <div class="ds-card"><p class="c-2"><strong class="c-1">Why amber.</strong> It is literally the cat's eye, so the loader, the logo and the primary action share one colour story. Warm against the cool near-black, it reads as "light in a dark room" rather than "tech". Lavender sits too close to the needs-review violet and to TMDB/Apple TV purples, so primary actions and review states would blur. Amber's risk is clashing with warning orange, so warning is pushed redder (#FF9D5C) and always carries an icon.</p></div>`);

  const statuses = [['ready', 'Ready', '--st-ready'], ['downloading', 'Downloading', '--st-downloading'], ['missing', 'Missing / needs a torrent', '--st-missing'], ['review', 'Needs review', '--st-review'], ['watched', 'Watched', '--st-watched'], ['upcoming', 'Airs / upcoming', '--st-upcoming'], ['warning', 'Warning', '--st-warning'], ['error', 'Error', '--st-error']];
  const status = sec('status', 'Status colours', 'Tuned for dark backgrounds. Every status is colour + icon + word, never colour alone. Missing is the only outlined (dashed) pill: it is an absence, not a state of progress.', `
    <div class="ds-scroll"><table class="ds-table"><thead><tr><th>Status</th><th>Pill</th><th>Token</th><th>Contrast on canvas</th><th>Used for</th></tr></thead><tbody>
      ${statuses.map(([k, l, v]) => `<tr><td>${l}</td><td>${ui.pill(k === 'missing' ? 'missing' : k, l.split(' /')[0])}</td><td class="ds-label">${v}</td><td class="contrast" data-c="${v}"></td><td class="t-small c-2">${{ ready: 'Playable in Infuse now', downloading: 'Torrent active; may already stream', missing: 'In library, no file', review: 'Mapping needs a human', watched: 'From Trakt history', upcoming: 'Not aired yet', warning: 'Slow, oversized, partial results', error: 'Something failed' }[k]}</td></tr>`).join('')}
    </tbody></table></div>`);

  // ---------------------------------------------------------------- type
  const typeRows = [['Display XL · 64/0.98 · −4%', 't-display-xl', 'Past Lives'], ['Display · 48/1.02 · −3.5%', 't-display', 'Frieren: Beyond Journey\'s End'], ['Title 1 · 34/1.08 · −3%', 't-title-1', 'Library'], ['Title 2 · 24/1.18 · −2.2%', 't-title-2', 'Because you loved Severance'], ['Title 3 · 19/1.26 · −1.5%', 't-title-3', 'Find a torrent'], ['Body L · 17/1.55', 't-body-l', 'You gave Perfect Days a 9 and finished Aftersun twice.'], ['Body · 15/1.55', 't-body', 'A season pack usually covers them all in one go.'], ['Small · 13/1.45', 't-small', '412 seeders · 13.2 GB · 9 months ago'], ['Micro · 11 · +8% · caps', 't-micro', 'Up next'], ['Mono · 13/1.5 · Geist Mono', 'release-name', 'Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX']];
  const type = sec('type', 'Typography', 'Geist for everything you read, Geist Mono for release names, file paths and hashes. Display sizes tighten; body text breathes. Load with <code class="t-mono">next/font/google</code> (Geist, Geist_Mono).', `
    <div>${typeRows.map(([l, c, s]) => `<div class="type-row"><span class="ds-label">${l}</span>${c === 'release-name' ? ui.releaseName(s) : `<span class="${c}">${esc(s)}</span>`}</div>`).join('')}</div>
    ${sub('Long text rules')}
    <div class="ds-grid" style="--min:280px">
      ${card('Release names', `${ui.releaseName(D.severanceRelease)}<p class="t-small c-2">Mono 13. Break only after <code class="t-mono">. - _ ] ) /</code>; dots dimmed. Full name in lists on desktop; on phone the card grows (no clamp), because the name is the decision. Never truncate the middle.</p>`)}
      ${card('File paths', `${ui.path('Frieren (2023)/Season 01/[SubsPlease] Sousou no Frieren - 05 (1080p) [8E3D5C21].mkv')}<p class="t-small c-2">Folder part dimmed, file name in full. Same break rule plus spaces.</p>`)}
      ${card('Titles', `<div class="pcard" style="max-width:140px">${ui.poster('onlymurders')}<div class="meta"><span class="title">Only Murders in the Building</span><span class="sub">2021 · Series</span></div></div><p class="t-small c-2">Cards: 2 lines, then ellipsis, always visible (never hover-only). Heroes: balanced wrap, never clamped.</p>`)}
    </div>`);

  // ---------------------------------------------------------------- space, radii
  const sp = [['0.5', 2], ['1', 4], ['1.5', 6], ['2', 8], ['3', 12], ['4', 16], ['5', 20], ['6', 24], ['8', 32], ['10', 40], ['12', 48], ['16', 64], ['20', 80], ['24', 96]];
  const space = sec('space', 'Spacing, radii and layout', '4 px base. Page gutter 16 / 24 / 40 / 56 px at 320 / 640 / 1024 / 1440. Columns 4 / 8 / 12. Touch targets ≥ 44 px, even when the visual control is 36 px.', `
    <div class="ds-grid" style="--min:280px">
      ${card('Spacing scale', `<div class="stack gap-2">${sp.map(([k, v]) => `<div class="row gap-3"><span class="ds-label" style="width:64px">--sp-${k}</span><span class="bar" style="width:${v}px"></span><span class="t-small c-3 t-num">${v}</span></div>`).join('')}</div>`)}
      ${card('Radii', `<div class="ds-row">${[['xs', 6], ['sm', 8], ['md', 12], ['lg', 16], ['xl', 22], ['2xl', 28], ['pill', 999]].map(([k, v]) => `<div class="stack gap-1" style="align-items:center"><div style="width:56px;height:56px;background:var(--surface-3);border:1px solid var(--line-2);border-radius:${Math.min(v, 28)}px"></div><span class="ds-label">${k} · ${v === 999 ? '∞' : v}</span></div>`).join('')}</div><p class="t-small c-2">md: buttons, posters, cards. lg: panels, release cards. xl: dialogs, hero poster. 2xl: sheet tops.</p>`)}
      ${card('Breakpoints', `<table class="ds-table"><tbody>${[['< 640', 'Phone', 'Bottom tab bar, large titles, bottom sheets, 3-col posters'], ['640–1023', 'Tablet', 'Tab bar, centred sheets (max 640), 4–5 col posters'], ['1024–1439', 'Laptop', 'Top bar + ⌘K, right-hand panels, 7 col posters'], ['≥ 1440', 'Desktop', 'Gutter 56, 8 col posters, max content 1600']].map((r) => `<tr><td class="t-num">${r[0]}</td><td>${r[1]}</td><td class="c-2">${r[2]}</td></tr>`).join('')}</tbody></table>`)}
    </div>`);

  const elev = sec('elev', 'Elevation and blur', 'Shadows are deep and soft, as if lit from the screen. Lifted posters gain a hairline highlight.', `
    <div class="ds-grid" style="--min:160px">${[['e-1', 'Rows, posters at rest'], ['e-2', 'Menus, raised cards'], ['e-3', 'Popovers, toasts'], ['e-4', 'Sheets, dialogs'], ['e-lift', 'Poster hover / focus']].map(([k, d]) => `<div class="ds-card"><div style="height:80px;border-radius:var(--r-md);background:var(--surface-2);box-shadow:var(--${k})"></div><span class="ds-label">--${k}</span><span class="t-small c-2">${d}</span></div>`).join('')}</div>
    <div class="ds-grid" style="--min:160px">${[['sm', 8, 'Chips over art'], ['md', 16, 'Glass buttons, pills'], ['lg', 28, 'Top bar, tab bar, toasts'], ['xl', 60, 'Palette, ambient hero glow']].map(([k, v, d]) => `<div class="ds-card"><div class="hero-mini" style="height:80px;padding:0"><div class="hero-bg">${art.backdrop('dune2')}</div><div style="position:absolute;inset:16px;border-radius:12px;backdrop-filter:blur(${v}px);-webkit-backdrop-filter:blur(${v}px);background:rgb(17 20 27 / 0.3)"></div></div><span class="ds-label">--blur-${k} · ${v}px</span><span class="t-small c-2">${d}</span></div>`).join('')}</div>`);

  const motion = sec('motion', 'Motion', 'Slow and smooth. Hover a row to play its curve. Under <code class="t-mono">prefers-reduced-motion</code> every movement becomes an instant change or a short fade, loaders stop blinking, and shimmer is replaced by a static block.', `
    <div class="ds-scroll"><table class="ds-table"><thead><tr><th>Token</th><th>Value</th><th>Use</th><th style="min-width:260px">Curve</th></tr></thead><tbody>
      ${[['--d-fast + --ease-out', '140ms', 'Hover, chip toggle', 'var(--d-fast) var(--ease-out)'], ['--d-base + --ease-out', '220ms', 'Tabs, popovers, poster lift', 'var(--d-base) var(--ease-out)'], ['--d-slow + --ease-sheet', '360ms', 'Sheets, dialogs', 'var(--d-slow) var(--ease-sheet)'], ['--d-slower + --ease-in-out', '560ms', 'Page crossfade, poster → detail shared element', 'var(--d-slower) var(--ease-in-out)'], ['--d-cinematic + --ease-out', '900ms', 'Hero backdrop crossfade, parallax settle', 'var(--d-cinematic) var(--ease-out)'], ['--d-ambient', '3200ms', 'Loader blink cycle', '3200ms var(--ease-in-out)']].map(([k, v, u, c]) => `<tr class="ds-motion" tabindex="0"><td class="ds-label">${k}</td><td class="t-num">${v}</td><td class="c-2">${u}</td><td><div class="ease-demo"><i style="transition:transform ${c}"></i></div></td></tr>`).join('')}
    </tbody></table></div>
    <div class="ds-grid" style="--min:260px">
      ${card('Shared element', '<p class="t-small c-2">Poster in a grid morphs into the detail hero poster (View Transitions API, <code class="t-mono">view-transition-name: hero-poster</code>), while the page crossfades. Reduced motion: plain crossfade.</p>')}
      ${card('Hero parallax', '<p class="t-small c-2">Backdrop scales 1.06 and moves at 0.18× scroll speed, clamped to 600 px. Disabled for reduced motion and on low-power devices.</p>')}
      ${card('Ambient colour', '<p class="t-small c-2">The hero glow is tinted by the backdrop\'s dominant colour (<code class="t-mono">--ambient</code>), blurred 60 px at 45% opacity, and crossfades over 900 ms when the pick changes.</p>')}
    </div>`);

  const icons = sec('icons', 'Iconography', 'Lucide outline icons, 1.75 stroke at 20/24 px, 2.2 inside 13 px pills. Filled only for play and star. Names map to <code class="t-mono">lucide-react</code>.', `<div class="icon-grid">${MS.iconNames.map((n) => `<div>${icon(n, { size: 22 })}<span>${n}</span></div>`).join('')}</div>`);

  // ---------------------------------------------------------------- components
  const btnStates = (cls, label, ic) => ['', 'is-hover', 'is-focus', 'is-active', 'disabled', 'is-loading'].map((s) => `<div class="stack gap-2" style="align-items:flex-start"><button class="btn ${cls} ${s !== 'disabled' ? s : ''}" ${s === 'disabled' ? 'disabled' : ''} tabindex="-1">${ic ? icon(ic, { size: 18 }) : ''}${label}</button><span class="ds-label">${s.replace('is-', '') || 'default'}</span></div>`).join('');
  const buttons = sec('buttons', 'Buttons', 'One primary per view, and it always tells the truth about the state (see the detail page state machine). Destructive actions are quiet: text-red ghost buttons in menus, confirmed in a dialog, never larger than the benign actions next to them.', `
    <div class="ds-stage">
      <div class="ds-row">${btnStates('btn-primary', 'Find a torrent', 'search')}</div>
      <div class="ds-row">${btnStates('btn-secondary', 'Inspect', 'scanSearch')}</div>
      <div class="ds-row">${btnStates('btn-ghost', 'See all', '')}</div>
      <div class="ds-row">${btnStates('btn-danger-quiet', 'Remove', 'trash')}</div>
    </div>
    <div class="ds-grid" style="--min:260px">
      ${card('Sizes', `<div class="ds-row"><button class="btn btn-primary btn-lg">Large · 52</button><button class="btn btn-primary">Default · 44</button><button class="btn btn-secondary btn-sm">Small · 36 (44 hit)</button></div>`)}
      ${card('Icon buttons', `<div class="ds-row"><button class="btn btn-secondary btn-icon" aria-label="Copy">${icon('copy')}</button><button class="btn btn-ghost btn-icon" aria-label="More">${icon('more')}</button><button class="btn btn-secondary btn-sm btn-icon" aria-label="Next">${icon('chevronRight', { size: 18 })}</button></div>`)}
      ${card('Glass, over artwork', `<div class="hero-mini" style="height:120px"><div class="hero-bg">${art.backdrop('frieren')}</div><div class="sc"></div><div class="row gap-2"><button class="btn btn-primary">${icon('plus', { size: 18 })}Add to library</button><button class="btn btn-glass">Details</button></div></div>`)}
      ${card('Status action (ready)', `<div class="ds-row"><span class="btn btn-ready">${icon('check', { size: 18 })}Added</span><button class="btn btn-danger">Remove show</button></div><p class="t-small c-3">Solid danger appears only inside a confirmation dialog.</p>`)}
    </div>`);

  const inputs = sec('inputs', 'Inputs', '44 px tall, 16 px text (prevents iOS zoom). Labels sit above; errors below with an icon. Auth fields use real autocomplete attributes so password managers save and fill.', `
    <div class="ds-grid" style="--min:260px">
      ${card('Default', `<div class="field"><label class="label" for="i1">Username</label><input class="input" id="i1" autocomplete="username" value="teymur"></div>`)}
      ${card('Focus', `<div class="field"><label class="label" for="i2">Password</label><input class="input is-focus" id="i2" type="password" value="hunter22"></div>`)}
      ${card('Error', `<div class="field"><label class="label" for="i3">Password</label><input class="input" id="i3" type="password" value="hunter2" aria-invalid="true"><span class="error-text">${icon('alertCircle', { size: 15 })}Wrong username or password</span></div>`)}
      ${card('Disabled', `<div class="field"><label class="label" for="i4">WebDAV port</label><input class="input" id="i4" value="36911" disabled><span class="help">Set in config.yaml</span></div>`)}
      ${card('Search', `<div class="input-wrap">${icon('search', { size: 18 })}<input class="input input-lg" value="Severance S02E05" aria-label="Search releases"><span class="input-end"><button class="btn btn-ghost btn-sm btn-icon" aria-label="Clear">${icon('x', { size: 16 })}</button></span></div>`)}
      ${card('Select · switch · range', `<select class="input" aria-label="Model"><option>Opus 5.5 · deeper</option></select><div class="row between"><span>Include specials</span>${ui.switch('sw1', true, 'Include specials')}</div><div class="range"><div class="range-track"><div class="range-fill" style="left:10%;right:40%"></div><div class="range-thumb" style="left:10%"></div><div class="range-thumb" style="left:60%"></div></div><div class="range-values"><span>1990</span><span>2025</span></div></div>`)}
    </div>`);

  const chips = sec('chips', 'Chips and filter chips', 'Attribute chips are small static labels parsed from release names. Filter chips are toggles with facet counts; at zero they dim but stay visible so you know the option exists.', `
    <div class="ds-grid" style="--min:280px">
      ${card('Attribute chips', `<div class="chips">${ui.chip('2160p', 'hi')}${ui.chip('WEB-DL')}${ui.chip('x265')}${ui.chip('DV', 'hi')}${ui.chip('Atmos', 'hi')}${ui.chip('FLUX')}${ui.chip('Season 2 pack', 'pack')}${ui.chip('CAM', 'err')}${ui.chip('Oversized', 'warn')}${ui.chip('Preferred group', 'accent')}</div>`)}
      ${card('Filter chips · off / on / zero / removable', `<div class="fchips">${ui.fchip('1080p', false, 31)}${ui.fchip('2160p', true, 4)}<button class="fchip is-zero" disabled>AV1 <span class="n">0</span></button><button class="fchip is-on">Dolby Vision<span class="x">${icon('x', { size: 12, stroke: 2.5 })}</span></button><button class="fchip fchip--menu">${icon('sliders', { size: 16 })}Filters <span class="n">3</span></button><button class="fchip fchip--menu is-focus">${icon('arrowUpDown', { size: 16 })}Best</button></div>`)}
      ${card('Mood chips', `<div class="fchips">${['Cosy', 'Mind-bending', 'Slow cinema', 'Laugh-out-loud', 'Short & sharp'].map((m, i) => ui.fchip(m, i === 2)).join('')}</div>`)}
    </div>`);

  const seg = sec('seg', 'Segmented controls and tabs', 'Segmented controls switch a mode of the same content (scope, view). Tabs switch between sibling content (seasons, result types) and show counts.', `
    <div class="ds-grid" style="--min:300px">
      ${card('Segmented · scope', ui.seg([['e', 'Episode'], ['s', 'Season pack'], ['a', 'Any']], 'e'))}
      ${card('Segmented · block (phone)', `<div class="seg seg--block">${['Grid', 'List'].map((x, i) => `<button aria-selected="${i === 0}">${icon(i ? 'list' : 'grid', { size: 16 })}&nbsp;${x}</button>`).join('')}</div>`)}
      ${card('Tabs · seasons incl. Specials', ui.tabs([['1', 'Season 1', 9], ['2', 'Season 2', 10], ['0', 'Specials', 6]], '2'))}
      ${card('Tabs · results', ui.tabs([['m', 'Movies', 3], ['s', 'Shows', 12]], 's'))}
    </div>`);

  const pills = sec('pills', 'Status pills', 'Every availability state the app talks about, with its exact wording. On artwork, pills switch to a dark glass base so they stay legible on bright posters.', `
    <div class="ds-stage"><div class="ds-row">
      ${ui.pill('ready', 'Ready to watch')}${ui.pill('ready_infuse')}${ui.pill('downloading', 'Downloading 42%', { pct: 42 })}${ui.pill('needs_torrent')}${ui.pill('missing')}${ui.pill('not_in_library')}${ui.pill('airs_on', 'Airs Thu')}${ui.pill('upcoming', 'Airs Oct 24')}${ui.pill('review')}${ui.pill('watched')}${ui.pill('streaming', 'Streaming now')}${ui.pill('paused')}${ui.pill('warning', 'Few seeders')}${ui.pill('error', 'Failed')}
    </div><div class="ds-row">${ui.pill('ready', '14/19', { icon: 'layers', lg: true })}${ui.pill('review', 'Needs review', { lg: true })}${ui.conf('high')}${ui.conf('medium')}${ui.conf('low')}${ui.conf('none')}</div></div>
    <div class="ds-grid" style="--min:120px">${['bluey', 'pluribus', 'studio', 'shogun'].map((k, i) => `<div class="pcard"><div class="media media--poster">${art.poster(k)}<span class="badge-tl">${[ui.pill('ready', 'Ready', { onArt: true }), ui.pill('downloading', '42%', { pct: 42, onArt: true }), ui.pill('review', 'Review', { onArt: true }), ui.pill('ready', '6/10', { icon: 'layers', onArt: true })][i]}</span></div></div>`).join('')}</div>`);

  const sample = D.releases[0];
  const cards = sec('cards', 'Cards', 'Titles and status are always visible, because touch has no hover. Hover and focus lift the artwork 4 px and add a hairline highlight.', `
    ${sub('Poster card', 'Default, with reason + feedback (AI shelves), with library status, with watched progress, focused.')}
    <div class="poster-grid" style="--poster-min:150px">
      ${ui.posterCard('severance')}${ui.posterCard('silo', { reason: 'Same sealed-world mystery, unspools one clue an episode', feedback: true })}${ui.posterCard('interstellar')}${ui.posterCard('thebear', { watched: 78 })}
      <div class="pcard"><a class="pcard-link" tabindex="-1"><div class="media media--poster" style="transform:translateY(-4px);box-shadow:var(--e-lift);outline:2px solid var(--accent);outline-offset:3px">${art.poster('perfectdays')}</div></a><div class="meta"><span class="title">Perfect Days</span><span class="sub">focus state</span></div></div>
    </div>
    ${sub('Landscape card', 'Up next and new episodes. The action matches availability.')}
    <div class="ds-grid" style="--min:260px">${D.upNext.slice(0, 4).map((u) => ui.landCard(u)).join('')}${ui.landCard(D.upNext[4])}${ui.landCard(D.upNext[5])}</div>
    ${sub('Episode rows', 'Compact (default for long seasons) and with stills. The next-up row gets an amber rule.')}
    <div class="ds-grid" style="--min:340px">
      <ol class="ep-list ds-stage" style="padding:8px">${D.severanceS2.slice(3, 10).map((e) => ui.epRow({ ...e, runtime: 52 }, { next: e.n === 5, se: `S02E${String(e.n).padStart(2, '0')}` })).join('')}${ui.epRow({ n: 6, name: 'The Waters of Mars', state: 'review', runtime: 62 }, { se: 'S00E06' })}</ol>
      <ol class="ep-list ds-stage" style="padding:8px">${D.severanceS2.slice(4, 7).map((e) => ui.epRow({ ...e, runtime: 52 }, { rich: true, art: 'severance', next: e.n === 5, se: `S02E${String(e.n).padStart(2, '0')}` })).join('')}</ol>
    </div>
    ${sub('Release card (torrent result)', 'Best match, normal, warning, no seeders, added.')}
    <div class="ts-sheet stack gap-3" style="max-width:760px">${ui.releaseCard(sample)}${ui.releaseCard(D.releases[1], { noBest: true })}${ui.releaseCard(D.releases[3], { noBest: true })}${ui.releaseCard(D.releases[5], { noBest: true })}${ui.releaseCard(D.releases[2], { noBest: true, added: true })}</div>
    ${sub('Torrent activity card')}
    <div class="ds-grid" style="--min:340px">${ui.tcard(D.torrents[1])}${ui.tcard(D.torrents[2])}${ui.tcard(D.torrents[3])}${ui.tcard(D.torrents[4])}</div>`);

  const overlays = sec('overlays', 'Sheets and dialogs', 'Phone: bottom sheets with a grabber, sticky head and foot, rounded 28 px tops, max height = screen minus 12 px. Search sheets are full height from the start, so they never jump as results load. Nested sheets stack on top. Tablet: the same sheet centred, max 640. Desktop: a right-hand panel (760 px; 480 narrow), full height. Dialogs are only for confirmations.', `
    <div class="ds-grid" style="--min:320px">
      ${card('Bottom sheet (phone)', `<div class="sheet static-sheet" style="height:420px"><div class="sheet-grabber"></div><div class="sheet-head"><div class="sheet-title"><h3 class="t-title-3">Sort by</h3><button class="btn btn-ghost btn-icon" aria-label="Close">${icon('x')}</button></div></div><div class="sheet-body stack gap-2">${['Best match', 'Seeders', 'Size', 'Newest'].map((x, i) => `<label class="row gap-3" style="min-height:48px;padding:0 12px;border-radius:12px;border:1px solid var(--line-2);${i === 0 ? 'background:var(--accent-soft);border-color:var(--accent-line)' : ''}"><input type="radio" name="dsort" ${i === 0 ? 'checked' : ''} style="accent-color:var(--accent);width:18px;height:18px"> ${x}</label>`).join('')}</div><div class="sheet-foot"><button class="btn btn-primary grow">Show 45 results</button></div></div>`)}
      ${card('Confirmation dialog', `<div class="dialog static-dialog" style="width:100%"><h3 class="t-title-3">Remove Severance from your library?</h3><p class="c-2">This removes the show and its 19 episode mappings. Your Trakt history isn't touched.</p><label class="row gap-3" style="min-height:44px"><input type="checkbox" checked style="width:20px;height:20px;accent-color:var(--accent)"><span class="t-small c-2">Also delete downloaded data (58.3 GB)</span></label><div class="actions"><button class="btn btn-secondary">Cancel</button><button class="btn btn-danger">Remove show</button></div></div>`)}
      ${card('Menu', `<div class="menu"><button>${icon('tv', { size: 18 })}Play in Infuse</button><button>${icon('captions', { size: 18 })}Subtitles<span class="hint">EN, RU</span></button><button>${icon('arrowRightLeft', { size: 18 })}Change file mapping</button><div class="sep"></div><button class="danger">${icon('trash', { size: 18 })}Remove file from episode</button></div>`)}
    </div>
    <div class="row gap-2 wrap"><button class="btn btn-secondary" data-act="demo-sheet">Open a live sheet</button><button class="btn btn-secondary" data-act="demo-dialog">Open a live dialog</button></div>`);

  const toasts = sec('toasts', 'Toasts', 'Bottom-centre above the tab bar on phone, bottom-right on desktop. Success and info dismiss after 5 s; errors and "ready" moments stay until dismissed. One action at most.', `
    <div class="stack gap-3" style="max-width:460px">
      <div class="toast toast--ready" style="animation:none">${icon('checkCheck', { size: 20 })}<div class="grow"><strong>Frieren S01E29–E38 are ready</strong><span class="c-2">They'll appear in Infuse after its next refresh.</span></div><button class="btn btn-ghost btn-sm">How to play</button></div>
      <div class="toast toast--info" style="animation:none">${icon('thumbsDown', { size: 20 })}<div class="grow"><strong>Got it, fewer like this</strong></div><button class="btn btn-ghost btn-sm">Undo</button></div>
      <div class="toast toast--error" style="animation:none">${icon('alertTriangle', { size: 20 })}<div class="grow"><strong>Couldn't reach momoshtrem</strong><span class="c-2">Retrying in 10 s. Your changes are kept.</span></div><button class="btn btn-ghost btn-sm">Retry now</button></div>
    </div>
    <div class="row gap-2"><button class="btn btn-secondary" data-act="demo-toast">Show a live toast</button></div>`);

  const feedback = sec('feedback', 'Loading, empty and error', 'Skeletons mirror the final layout so nothing jumps. Noir appears in loading, empty and success moments; errors stay plain and tell you what still works.', `
    <div class="ds-grid" style="--min:280px">
      ${card('Skeleton · rail', `<div class="rail rail--posters" style="margin:0;padding:0;--rail-col:31%">${ui.skelPosters(3)}</div>`)}
      ${card('Skeleton · up next', `<div class="stack gap-3">${ui.skelWide(1)}</div>`)}
      ${card('Loader', ui.loader('Asking AI about the tricky ones', '3 files left · about 10 s'))}
    </div>
    <div class="ds-grid" style="--min:280px">
      ${card('Empty · library', ui.empty({ cat: 'sleep', title: 'Your library is empty', body: 'Add a show or movie and it appears here, with what\'s ready to watch.', actions: '<button class="btn btn-primary">Discover</button><button class="btn btn-secondary">Search</button>' }))}
      ${card('Empty · no results', ui.empty({ cat: 'peek', title: 'No releases for "Frieren S01E29"', body: 'Release names are often shorter than titles.', actions: '<button class="btn btn-secondary">Try "Frieren 29"</button>' }))}
      ${card('Empty · Trakt not connected', ui.empty({ cat: 'sit', title: 'Make this page yours', body: 'Connect Trakt for Up next and personal picks. Read-only.', actions: '<button class="btn btn-primary">Connect Trakt</button>' }))}
    </div>
    <div class="stack gap-3">
      <div class="banner banner--error">${icon('wifiOff', { size: 20 })}<div class="grow"><strong>momoshtrem isn't responding</strong><span>Showing what was loaded 2 minutes ago. Infuse can still play everything that's ready.</span></div><div class="actions"><button class="btn btn-secondary btn-sm">Retry</button></div></div>
      <div class="banner banner--warning">${icon('alertTriangle', { size: 20 })}<div class="grow"><strong>2 indexers timed out</strong><span>Results from Nyaa only. The Pirate Bay and TorrentLeech didn't answer within 15 s.</span></div><div class="actions"><button class="btn btn-secondary btn-sm">Retry</button></div></div>
      <div class="banner banner--review">${icon('alertCircle', { size: 20 })}<div class="grow"><strong>2 files need a quick review</strong><span>A recap (28.5) and a v2 re-release didn't map cleanly.</span></div><div class="actions"><button class="btn btn-secondary btn-sm">Review mapping</button></div></div>
    </div>`);

  const progress = sec('progress', 'Progress', 'Bars for downloads, rings inside pills, steps for multi-stage jobs (mapping, Ask). Indeterminate only when we truly don\'t know.', `
    <div class="ds-grid" style="--min:260px">
      ${card('Bars', `<div class="stack gap-4">${ui.progress(42)}${ui.progress(78, 'watched')}${ui.progress(100, 'ready')}${ui.progress(30, 'accent', 'progress--lg')}<div class="progress progress--indeterminate"><span></span></div></div>`)}
      ${card('Rings', `<div class="row gap-4">${ui.ring(12, 32)}${ui.ring(42, 32)}${ui.ring(86, 32)}${ui.pill('downloading', '64%', { pct: 64 })}</div>`)}
      ${card('Steps', `<div class="steps"><div class="step is-done"><span class="ic">${icon('check', { size: 14, stroke: 2.5 })}</span>Fetching metadata<span class="detail">16 files</span></div><div class="step is-done"><span class="ic">${icon('check', { size: 14, stroke: 2.5 })}</span>Matching files<span class="detail">12 of 16</span></div><div class="step is-active"><span class="ic"></span>Asking AI about the tricky ones</div></div>`)}
    </div>`);

  const palette = sec('palette', 'Command palette', '⌘K or / on desktop; the Search tab on phone opens the same results full-screen. Groups: your library first, then titles, then actions. Arrow keys move, Enter opens, ⌘Enter runs the secondary action.', `
    <div class="palette static-palette" role="dialog" aria-label="Command palette example">
      <div class="p-input">${icon('search', { size: 20 })}<input value="sev" aria-label="Search"><span class="kbd">esc</span></div>
      <div class="p-body">
        <div class="p-group">Your library</div>
        <button class="p-item" aria-selected="true"><div class="thumb">${ui.poster('severance')}</div><span class="grow"><span><mark>Sev</mark>erance</span><span>Series · 14 of 19 ready · Up next S02E05</span></span><span class="enter">${icon('corner', { size: 16 })}</span></button>
        <div class="p-group">Titles</div>
        <button class="p-item"><div class="thumb">${ui.poster('arrival')}</div><span class="grow"><span>The <mark>Sev</mark>enth Seal</span><span>Movie · 1957 · not in library</span></span><span class="enter">${icon('corner', { size: 16 })}</span></button>
        <div class="p-group">Actions</div>
        <button class="p-item"><span class="ic">${icon('search', { size: 16 })}</span><span class="grow"><span>Find a torrent for <mark>Sev</mark>erance S02E08</span><span>Missing episode</span></span><span class="enter">${icon('corner', { size: 16 })}</span></button>
        <button class="p-item"><span class="ic">${icon('activity', { size: 16 })}</span><span class="grow"><span>Go to Activity</span><span>3 downloading</span></span></button>
      </div>
      <div class="p-foot"><span><span class="kbd">↑</span><span class="kbd">↓</span>move</span><span><span class="kbd">↵</span>open</span><span><span class="kbd">⌘</span><span class="kbd">↵</span>find torrent</span></div>
    </div>`);

  const nav = sec('nav', 'Navigation', 'Five primary destinations. Settings and Log out live in the cat menu. Desktop: a glass top bar that is transparent over heroes and solid after scrolling, plus ⌘K and the Ask entry. Phone and tablet: a bottom tab bar with large titles, safe-area aware.', `
    ${card('Top bar (≥1024)', `<header class="topbar topbar--solid static-top"><a class="brand">${brand.mark(30)}<span>momoshtrem</span></a><nav><a aria-current="page">For You</a><a>Discover</a><a>Library</a><a>Activity<span class="badge">3</span></a></nav><button class="search-trigger">${icon('search', { size: 16 })}<span>Search or jump to…</span><span class="kbd">⌘K</span></button><a class="ask-trigger">${brand.avatar(28)}Ask momoshtrem</a><button class="avatar-btn" aria-label="Account">${brand.markMono(20, 'var(--text-2)')}</button></header>`, 'style="overflow-x:auto"')}
    <div class="ds-grid" style="--min:320px">
      ${card('Tab bar (<1024)', `<nav class="tabbar static-bar" style="border-radius:var(--r-lg)">${[['For You', 'sparkles', 1], ['Discover', 'compass'], ['Search', 'search'], ['Library', 'library'], ['Activity', 'activity']].map(([l, ic, on], i) => `<a ${on ? 'aria-current="page"' : ''}>${icon(ic, { size: 24, stroke: on ? 2 : 1.6 })}<span>${l}</span>${i === 4 ? '<span class="dot">3</span>' : ''}</a>`).join('')}</nav>`)}
      ${card('Large title (phone)', `<div class="large-title" style="padding:0"><h1>Library</h1><div class="actions"><button class="btn btn-ghost btn-icon" aria-label="Search library">${icon('search')}</button><button class="avatar-btn" aria-label="Account">${brand.markMono(20, 'var(--text-2)')}</button></div></div>`)}
    </div>`);

  document.getElementById('ds').innerHTML = [logo, mascot, color, accent, status, type, space, elev, motion, icons, buttons, inputs, chips, seg, pills, cards, overlays, toasts, feedback, progress, palette, nav].join('');
  MS.app.initRails(document);

  // Contrast ratios, computed from the live tokens
  const lum = (rgb) => { const a = rgb.match(/\d+(\.\d+)?/g).slice(0, 3).map(Number).map((v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4; }); return 0.2126 * a[0] + 0.7152 * a[1] + 0.0722 * a[2]; };
  const probe = document.createElement('span'); document.body.appendChild(probe);
  const colorOf = (v) => { probe.style.color = `var(${v})`; return getComputedStyle(probe).color; };
  const bg = lum(colorOf('--ink-1'));
  document.querySelectorAll('[data-c]').forEach((el) => { const L1 = lum(colorOf(el.dataset.c)); const r = (Math.max(L1, bg) + 0.05) / (Math.min(L1, bg) + 0.05); el.textContent = `${r.toFixed(1)}:1 ${r >= 4.5 ? 'AA' : r >= 3 ? 'AA large' : '—'}`; });
  probe.remove();

  Object.assign(MS.app.actions, {
    'demo-sheet': () => MS.app.openSheet('howto'),
    'demo-dialog': () => MS.app.openDialog('remove'),
    'demo-toast': () => MS.app.toast('ready'),
  });
})();
