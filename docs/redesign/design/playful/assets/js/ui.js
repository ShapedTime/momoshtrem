/* Component builders. Each returns an HTML string; behaviour is wired by app.js through data-act / data-go. */
(function () {
  const esc = (s) => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  const icon = (name, cls = 'ico') => `<svg class="${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${(window.ICONS || {})[name] || ''}</svg>`;
  const momo = (pose, cls = '') => (window.MOMO[pose] || '').replace('class="momo ', `class="momo ${cls} `);
  const attrs = (o = {}) => Object.entries(o).filter(([, v]) => v !== undefined && v !== false && v !== null).map(([k, v]) => (v === true ? k : `${k}="${esc(v)}"`)).join(' ');

  // Release names and file paths: insert break opportunities after separators so lines break
  // at dots, dashes, brackets and slashes, never in the middle of a token.
  const breakable = (s) => esc(s).replace(/([._\-\/\]\)])(?=\S)/g, '$1<wbr>').replace(/(\S)(?=[\[\(])/g, '$1<wbr>');
  const machine = (raw, emph) => {
    if (emph && raw.startsWith(emph)) return `<b>${breakable(emph)}</b>${breakable(raw.slice(emph.length))}`;
    return breakable(raw);
  };

  // ------------------------------------------------------------------ buttons & controls
  const btn = (label, o = {}) => {
    const v = o.variant || 'secondary';
    const cls = ['btn', `btn-${v}`, o.size ? `btn-${o.size}` : '', o.block ? 'btn-block' : '', o.cls || ''].filter(Boolean).join(' ');
    const inner = `${o.icon ? icon(o.icon) : ''}${label ? `<span>${label}</span>` : ''}${o.iconEnd ? icon(o.iconEnd) : ''}`;
    const a = attrs({ class: cls, 'data-act': o.act, 'data-arg': o.arg, 'data-flow': o.flow, 'aria-label': o.aria, disabled: o.disabled, 'aria-disabled': o.ariaDisabled, type: o.type || (o.href ? undefined : 'button'), id: o.id, form: o.form });
    return o.href ? `<a href="${o.href}" ${a}>${inner}</a>` : `<button ${a}>${inner}</button>`;
  };
  const iconBtn = (name, label, o = {}) => {
    const a = attrs({ class: `icon-btn ${o.cls || ''}`, 'data-act': o.act, 'data-arg': o.arg, 'aria-label': label, title: o.title === false ? undefined : label, 'data-flow': o.flow, 'aria-pressed': o.pressed, 'aria-expanded': o.expanded, type: o.href ? undefined : 'button' });
    return o.href ? `<a href="${o.href}" ${a}>${icon(name)}${o.dot ? '<span class="badge-dot"></span>' : ''}</a>` : `<button ${a}>${icon(name)}${o.dot ? '<span class="badge-dot"></span>' : ''}</button>`;
  };
  const chip = (label, o = {}) => `<button type="button" ${attrs({ class: `chip ${o.cls || ''}`, 'aria-pressed': o.on ? 'true' : 'false', 'data-act': o.act, 'data-arg': o.arg, disabled: o.disabled, 'data-flow': o.flow })}>${o.icon ? icon(o.icon) : ''}${o.on && o.check !== false ? icon('check', 'ico ico-sm') : ''}<span>${label}</span>${o.count !== undefined ? `<span class="count">${o.count}</span>` : ''}${o.caret ? icon('chevron-down', 'ico ico-sm') : ''}${o.x ? `<span class="chip-x" aria-hidden="true">${icon('x', 'ico ico-xs')}</span>` : ''}</button>`;
  const seg = (opts, sel, o = {}) => `<div class="seg ${o.block ? 'block' : ''}" role="group" aria-label="${esc(o.label || '')}">${opts.map((x) => {
    const [val, lab, count] = Array.isArray(x) ? x : [x, x];
    return `<button type="button" aria-pressed="${val === sel}" data-act="${o.act || ''}" data-arg="${esc(val)}">${lab}${count !== undefined ? `<span class="count">${count}</span>` : ''}</button>`;
  }).join('')}</div>`;
  const tabs = (opts, sel, o = {}) => `<div class="tabs ${o.scroll ? 'scroll' : ''}" role="tablist" aria-label="${esc(o.label || '')}">${opts.map((x) => {
    const [val, lab, count] = Array.isArray(x) ? x : [x, x];
    return `<button type="button" role="tab" class="tab-btn" aria-selected="${val === sel}" data-act="${o.act || ''}" data-arg="${esc(val)}"><span class="ears" aria-hidden="true"></span>${lab}${count !== undefined ? ` <span class="count">${count}</span>` : ''}</button>`;
  }).join('')}</div>`;
  const tag = (t, kind) => `<span class="tag ${kind || ''}">${kind === 'warn' || kind === 'err' ? icon('triangle-alert', 'ico') : ''}${esc(t)}</span>`;
  const kbd = (k) => `<kbd>${k}</kbd>`;

  // ------------------------------------------------------------------ status
  const PILL = {
    ready: ['pill-ready', 'tv-minimal-play', 'Ready to watch'],
    ready_short: ['pill-ready', 'check', 'Ready'],
    downloading: ['pill-downloading', null, 'Downloading'],
    missing: ['pill-missing', 'circle-dashed', 'Missing'],
    needs_torrent: ['pill-missing', 'magnet', 'Needs a torrent'],
    not_in_library: ['pill-outline', 'plus', 'Not in library'],
    airs: ['pill-upcoming', 'calendar', 'Airs'],
    watched: ['pill-watched', 'check-check', 'Watched'],
    review: ['pill-review', 'scan-search', 'Needs review'],
    warning: ['pill-warning', 'triangle-alert', 'Warning'],
    error: ['pill-error', 'circle-alert', 'Error'],
    idle: ['pill-neutral', 'moon', 'Idle'],
    paused: ['pill-neutral', 'pause', 'Paused'],
    streaming: ['pill-ready', 'cast', 'Streaming'],
    added: ['pill-neutral', 'bookmark-check', 'In library'],
  };
  const pill = (kind, text, o = {}) => {
    const [cls, ic, def] = PILL[kind] || PILL.ready;
    const lead = kind === 'downloading' ? `<span class="mini-ring" style="--p:${o.pct ?? 40}"></span>` : ic ? icon(ic) : '';
    const label = text || (kind === 'downloading' && o.pct !== undefined ? `Downloading ${o.pct}%` : def);
    return `<span class="pill ${cls} ${o.sm ? 'sm' : ''}">${lead}<span class="ellipsis">${label}</span></span>`;
  };
  const sticker = (text, o = {}) => `<span class="sticker ${o.tone || ''}">${o.icon ? icon(o.icon) : ''}${text}</span>`;
  const progress = (pct, o = {}) => `<div class="progress ${o.cls || ''}" role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100" aria-label="${esc(o.label || 'Progress')}"><i style="width:${pct}%"></i></div>`;
  const ring = (pct, size = 44) => `<span class="ring" style="--p:${pct};--size:${size}px" role="img" aria-label="${pct}%"></span>`;
  const pawSteps = (steps) => `<ol class="paws">${steps.map((s) => `<li class="paw-step ${s.state}"><span class="paw-ico">${icon(s.state === 'done' ? 'check' : s.state === 'err' ? 'triangle-alert' : 'paw-print')}</span><div class="paw-text">${s.label}${s.detail ? `<small>${s.detail}</small>` : ''}</div></li>`).join('')}</ol>`;
  const pawTrack = (n, on) => `<span class="paw-track" role="img" aria-label="${on} of ${n}">${Array.from({ length: n }, (_, i) => icon('paw-print', `ico ${i < on ? 'on' : ''}`)).join('')}</span>`;

  // ------------------------------------------------------------------ artwork
  const T = (id) => DATA.T[id];
  const poster = (id, o = {}) => `<div class="art art-poster ${o.cls || ''}">${ART.poster(id, o.label)}</div>`;
  const wide = (svg, o = {}) => `<div class="art art-wide ${o.cls || ''}">${svg}</div>`;
  const meta = (t) => [t.type === 'movie' ? 'Movie' : 'Show', t.year, t.type === 'movie' && t.runtime ? `${Math.floor(t.runtime / 60)}h ${t.runtime % 60}m` : null].filter(Boolean).join(' · ');

  // poster card: title and meta always visible; status and reason beneath; actions never hover-only
  const posterCard = (id, o = {}) => {
    const t = T(id);
    if (!t) return '';
    const href = `#/${t.type}/${id}`;
    const foot = o.status ? `<div class="pc-foot">${o.status}</div>` : '';
    const more = o.menu ? iconBtn('ellipsis', `Options for ${t.title}`, { cls: 'sm pc-more', act: o.menu, arg: id }) : o.more !== false && (o.reason || o.feedback) ? iconBtn('ellipsis', `More for ${t.title}`, { cls: 'sm pc-more', act: 'feedback', arg: id }) : '';
    return `<article class="poster-card ${o.cls || ''}" ${o.flow ? `data-flow="${o.flow}"` : ''}>
      <a class="card-link" href="${href}" aria-label="${esc(t.title)}"></a>
      ${poster(id, { label: '' })}
      <div class="pc-body">
        <div class="row gap-1" style="align-items:flex-start"><h3 class="pc-title grow">${esc(t.title)}</h3>${more}</div>
        <p class="pc-meta">${esc(o.meta || meta(t))}</p>
        ${o.reason && t.reason ? `<p class="pc-reason">${icon('sparkle')}${esc(t.reason)}</p>` : ''}
        ${o.progress !== undefined ? progress(o.progress, { cls: 'thin watched', label: 'Watched' }) : ''}
        ${foot}
      </div>
    </article>`;
  };

  const AV = {
    ready: { pill: () => pill('ready'), cta: (it) => btn('How to play', { variant: 'tonal', size: 'sm', icon: 'tv', act: 'howto', arg: it.show }) },
    downloading: { pill: (it) => pill('downloading', null, { pct: it.pct }), cta: () => '' },
    needs_torrent: { pill: () => pill('needs_torrent'), cta: (it) => btn('Find torrent', { variant: 'primary', size: 'sm', icon: 'search', act: 'tsearch', arg: `${it.show}:${it.s}:${it.n}`, flow: it.show === 'frieren' ? 'find-torrent' : undefined }) },
    not_in_library: { pill: () => pill('not_in_library'), cta: (it) => btn('Add', { variant: 'secondary', size: 'sm', icon: 'plus', act: 'add-lib', arg: it.show }) },
    airs: { pill: (it) => pill('airs', it.airs), cta: () => '' },
    review: { pill: () => pill('review'), cta: (it) => btn('Review', { variant: 'secondary', size: 'sm', href: `#/review/${it.show}` }) },
  };
  const upCard = (it) => {
    const t = T(it.show);
    const st = AV[it.av];
    const stillN = it.n || (it.abs ? it.abs % 7 : 1);
    return `<article class="up-card" ${it.av === 'needs_torrent' && it.show === 'frieren' ? 'data-flow="upnext-frieren"' : ''}>
      <a href="#/show/${it.show}" class="art art-wide" aria-label="${esc(t.title)} ${esc(it.label)}">${ART.still(it.show, stillN, `${t.title} ${it.label}`)}
        ${it.progress ? `<div class="progress thin watched"><i style="width:${it.progress}%"></i></div>` : ''}
        ${it.av === 'downloading' ? `<div class="progress thin"><i style="width:${it.pct}%"></i></div>` : ''}</a>
      <div class="up-body">
        <div><h3 class="up-show clamp-1">${esc(t.short || t.title)}</h3><p class="up-ep clamp-1">${esc(it.label)} · ${esc(it.title)}</p></div>
        <div class="up-row">${st.pill(it)}${st.cta(it)}</div>
      </div>
    </article>`;
  };

  // ------------------------------------------------------------------ episodes
  const EP_STATE = {
    watched: () => pill('watched', 'Watched', { sm: true }),
    ready: () => pill('ready_short', 'Ready', { sm: true }),
    downloading: (e) => pill('downloading', `${e.progress ?? 40}%`, { sm: true, pct: e.progress ?? 40 }),
    missing: () => pill('missing', 'Missing', { sm: true }),
    upcoming: (e) => pill('airs', e.airs || 'Airs Oct 24', { sm: true }),
    review: () => pill('review', 'Needs review', { sm: true }),
  };
  const epCode = (e) => (e.s === 0 ? `SP${String(e.n).padStart(2, '0')}` : `E${String(e.n).padStart(2, '0')}`);
  const epRow = (e, show, o = {}) => {
    const act = e.state === 'missing' ? iconBtn('search', `Find a torrent for ${epCode(e)}`, { act: 'tsearch', arg: `${show}:${e.s}:${e.n}`, cls: 'sm' })
      : e.state === 'review' ? iconBtn('scan-search', `Review mapping for ${epCode(e)}`, { href: `#/review/${show}`, cls: 'sm' })
        : iconBtn('ellipsis-vertical', `More for ${epCode(e)}`, { act: 'ep-menu', arg: `${show}:${e.s}:${e.n}`, cls: 'sm' });
    return `<li class="ep-row ${e.state === 'watched' ? 'is-watched' : ''} ${o.next ? 'is-next' : ''} ${o.comfy ? 'comfy' : ''}" ${o.next ? 'id="next-up" aria-current="true"' : ''}>
      ${o.comfy ? wide(ART.still(show, e.n, ''), {}) : `<span class="ep-num ${e.abs > 99 || e.n > 99 ? 'wide' : ''}" aria-label="Episode ${e.abs || e.n}">${o.abs ? e.abs : epCode(e)}</span>`}
      <div class="ep-main">
        ${o.next ? '<span class="next-flag">Up next</span>' : ''}
        <span class="ep-title">${esc(e.title)}</span>
        <span class="ep-meta">${o.abs ? `<span>S${String(e.s).padStart(2, '0')}E${String(e.n).padStart(2, '0')}</span>` : ''}${e.date ? `<span>${esc(e.date)}</span>` : ''}${e.runtime ? `<span>${e.runtime} min</span>` : ''}</span>
      </div>
      <div class="ep-side">${EP_STATE[e.state] ? EP_STATE[e.state](e) : ''}${act}</div>
    </li>`;
  };

  // ------------------------------------------------------------------ release (torrent result) card
  const releaseCard = (r, o = {}) => {
    const warn = (r.warn || []).map((w) => `<p class="rel-warn">${icon('triangle-alert', 'ico ico-sm')}<span>${esc(w)}</span></p>`).join('');
    return `<article class="rel ${r.best ? 'is-best' : ''} ${r.warn ? 'has-warn' : ''}" ${r.best && o.flow ? `data-flow="${o.flow}"` : ''}>
      ${r.best ? `<div class="rel-best">${sticker('Best match', { icon: 'sparkles' })}<p class="rel-why">${r.why.map(esc).join(' · ')}</p></div>` : ''}
      <div class="rel-main">
        <h3 class="rel-name machine">${machine(r.raw, r.emph)}</h3>
        ${r.note ? `<p class="rel-note">${icon('info', 'ico ico-xs')}${esc(r.note)}</p>` : ''}
        <div class="tags">${r.tags.map(([t, k]) => tag(t, k)).join('')}</div>
      </div>
      <dl class="rel-meta">
        <div><dt>Size</dt><dd>${esc(r.size)}</dd></div>
        <div><dt>Seeders</dt><dd class="${r.se === 0 ? 'zero' : r.se > 100 ? 'good' : ''}">${icon('arrow-up', 'ico ico-xs')}${r.se.toLocaleString('en')}<span class="le">${icon('arrow-down', 'ico ico-xs')}${r.le}</span></dd></div>
        <div><dt>Age</dt><dd>${esc(r.age)}</dd></div>
        <div class="idx"><dt>Indexer</dt><dd class="ellipsis">${esc(r.idx)}</dd></div>
      </dl>
      ${warn}
      <div class="rel-actions">
        ${btn('Inspect', { variant: 'ghost', size: 'sm', icon: 'scan-search', act: 'inspect', arg: o.key, flow: r.best && o.flow ? 'inspect' : undefined })}
        ${iconBtn('magnet', 'Copy magnet link', { cls: 'sm', act: 'toast', arg: 'magnet' })}
        <span class="spacer"></span>
        ${btn('Add', { variant: r.best ? 'primary' : 'secondary', size: 'sm', icon: 'plus', act: 'add-release', arg: o.key, aria: `Add ${r.raw} to library and assign` })}
      </div>
    </article>`;
  };

  // ------------------------------------------------------------------ activity card
  const torrentCard = (t) => {
    const title = T(t.title);
    const st = { streaming: pill('streaming', 'Streaming now'), downloading: pill('downloading', `${t.pct}%`, { pct: t.pct }), idle: pill('idle', 'Idle'), paused: pill('paused', 'Paused'), review: pill('review', 'Needs review') }[t.state];
    const live = t.state === 'streaming' || t.state === 'downloading';
    return `<article class="tor ${t.state}">
      <a class="tor-art" href="#/${title.type}/${t.title}" aria-label="${esc(title.title)}">${poster(t.title, { label: '' })}</a>
      <div class="tor-main">
        <div class="tor-top"><div class="grow"><a class="tor-title" href="#/${title.type}/${t.title}">${esc(title.title)}</a><p class="tor-what">${esc(t.what)}</p></div>${st}</div>
        <p class="tor-name machine">${machine(t.name)}</p>
        ${t.reader ? `<p class="tor-reader">${icon('cast', 'ico ico-sm')}${esc(t.reader)}</p>` : ''}
        ${t.pct < 100 || live ? progress(t.pct, { cls: t.state === 'streaming' ? 'ready' : 'striped', label: 'Downloaded' }) : ''}
        <div class="tor-stats">
          ${live ? `<span>${icon('arrow-down', 'ico ico-xs')}<b>${t.down}</b></span><span>${icon('arrow-up', 'ico ico-xs')}${t.up}</span><span>${icon('users', 'ico ico-xs')}${t.peers}</span>${t.eta ? `<span>${icon('clock', 'ico ico-xs')}${t.eta} left</span>` : ''}` : `<span>${t.size}</span><span>${t.state === 'review' ? 'Complete · mapping waiting for you' : t.state === 'paused' ? 'Complete · paused' : 'Complete · loads when Infuse opens a file'}</span>`}
        </div>
      </div>
      <div class="tor-actions">
        ${t.state === 'review' ? btn('Review', { variant: 'tonal', size: 'sm', href: `#/review/${t.title}` }) : ''}
        ${t.state === 'paused' ? iconBtn('play', 'Resume', { act: 'toast', arg: 'resumed' }) : t.state !== 'idle' && t.state !== 'review' ? iconBtn('pause', 'Pause', { act: 'toast', arg: 'paused' }) : ''}
        ${iconBtn('chevron-right', `Details for ${title.title}`, { href: `#/activity/${t.hash}` })}
      </div>
    </article>`;
  };

  // ------------------------------------------------------------------ containers
  const shelf = (title, sub, items, o = {}) => `<section class="section shelf-sec" aria-label="${esc(title)}">
    <div class="shelf-head"><div><h2 class="shelf-title">${title}</h2>${sub ? `<p class="shelf-sub">${sub}</p>` : ''}</div>
      ${o.more ? `<a class="link-btn" href="${o.more}">See all</a>` : ''}
      <div class="shelf-nav">${iconBtn('chevron-left', `Scroll ${esc(title)} left`, { cls: 'sm filled', act: 'shelf', arg: '-1' })}${iconBtn('chevron-right', `Scroll ${esc(title)} right`, { cls: 'sm filled', act: 'shelf', arg: '1' })}</div>
    </div>
    <div class="shelf-wrap"><div class="shelf ${o.wide ? 'shelf-wide' : ''}" tabindex="0">${items}</div></div>
  </section>`;
  const emptyState = (pose, title, text, actions = '', o = {}) => `<div class="state ${o.compact ? 'compact' : ''}" role="${o.role || 'status'}">${momo(pose)}<${o.compact ? 'h3' : 'h2'}>${title}</${o.compact ? 'h3' : 'h2'}><p>${text}</p>${actions ? `<div class="row gap-2">${actions}</div>` : ''}</div>`;
  const banner = (kind, ic, title, text, action = '') => `<div class="banner ${kind}" role="${kind === 'err' ? 'alert' : 'status'}">${icon(ic)}<div class="grow"><b>${title}</b>${text ? `<span>${text}</span>` : ''}</div>${action}</div>`;

  const sheet = (o) => `<div class="scrim" data-act="close-sheet" ${o.nested ? 'style="z-index:calc(var(--z-sheet) + 1)"' : ''}></div>
    <section class="sheet ${o.cls || ''} ${o.nested ? 'nested' : ''}" role="dialog" aria-modal="true" aria-labelledby="sheet-title-${o.id || 'x'}">
      <div class="sheet-grab" aria-hidden="true"></div>
      <header class="sheet-head">${o.back ? iconBtn('arrow-left', 'Back', { act: o.back }) : ''}<div class="grow"><h2 id="sheet-title-${o.id || 'x'}">${o.title}</h2>${o.sub ? `<p class="sheet-sub">${o.sub}</p>` : ''}</div>${o.headExtra || ''}${iconBtn('x', 'Close', { act: o.nested ? 'close-nested' : 'close-sheet' })}</header>
      <div class="sheet-body">${o.body}</div>
      ${o.foot ? `<footer class="sheet-foot">${o.foot}</footer>` : ''}
    </section>`;
  const dialog = (title, text, actions, o = {}) => `<div class="scrim" style="z-index:calc(var(--z-dialog) - 1)" data-act="close-dialog"></div>
    <section class="dialog" role="alertdialog" aria-modal="true" aria-labelledby="dlg-t"><h2 id="dlg-t">${title}</h2><p>${text}</p>${o.extra || ''}<div class="dialog-actions">${actions}</div></section>`;

  window.UI = { esc, icon, momo, attrs, machine, breakable, btn, iconBtn, chip, seg, tabs, tag, kbd, pill, sticker, progress, ring, pawSteps, pawTrack,
    poster, wide, meta, posterCard, upCard, AV, epRow, epCode, releaseCard, torrentCard, shelf, emptyState, banner, sheet, dialog };
})();
