/* momoshtrem prototype runtime: router, shell, overlays and component functions.
   Screens live in js/screens/*.js and register with MS.app.route(). */
(function () {
  const MS = window.MS;
  const { icon, art, brand } = MS;
  const D = MS.data;

  // ---- tiny helpers ------------------------------------------------------
  const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
  const cls = (...a) => a.filter(Boolean).join(' ');
  const T = (id) => D.T[id];
  const reduced = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ---- status vocabulary (one source for labels + icons) ------------------
  const STATUS = {
    ready: { kind: 'ready', label: 'Ready', icon: 'play' },
    ready_infuse: { kind: 'ready', label: 'Ready in Infuse', icon: 'play' },
    downloading: { kind: 'downloading', label: 'Downloading', icon: 'arrowDown' },
    needs_torrent: { kind: 'missing', label: 'Needs a torrent', icon: 'search' },
    missing: { kind: 'missing', label: 'Missing', icon: 'minus' },
    not_in_library: { kind: 'neutral', label: 'Not in library', icon: 'plus' },
    airs_on: { kind: 'upcoming', label: 'Airs', icon: 'calendar' },
    upcoming: { kind: 'upcoming', label: 'Upcoming', icon: 'calendar' },
    review: { kind: 'review', label: 'Needs review', icon: 'alertCircle' },
    needs_review: { kind: 'review', label: 'Needs review', icon: 'alertCircle' },
    watched: { kind: 'watched', label: 'Watched', icon: 'check' },
    partial: { kind: 'downloading', label: 'Partial', icon: 'layers' },
    error: { kind: 'error', label: 'Error', icon: 'alertTriangle' },
    warning: { kind: 'warning', label: 'Warning', icon: 'alertTriangle' },
    paused: { kind: 'neutral', label: 'Paused', icon: 'pause' },
    idle: { kind: 'neutral', label: 'Idle', icon: 'clock' },
    streaming: { kind: 'accent', label: 'Streaming', icon: 'tv' },
  };

  const ui = {};

  ui.icon = icon;
  ui.esc = esc;

  ui.pill = (key, label, opts = {}) => {
    const s = STATUS[key] || { kind: key, label: key, icon: 'info' };
    const ring = opts.pct != null ? ui.ring(opts.pct, 13, 'mini-ring') : icon(opts.icon || s.icon, { size: 13, stroke: 2.2 });
    return `<span class="${cls('pill', `pill--${s.kind}`, opts.lg && 'pill--lg', opts.onArt && 'pill--on-art', opts.cls)}">${ring}<span>${esc(label || s.label)}</span></span>`;
  };

  ui.ring = (pct, size = 28, extra = '') => {
    const r = 10, c = 2 * Math.PI * r;
    return `<svg class="${cls('ring', extra)}" width="${size}" height="${size}" viewBox="0 0 24 24" aria-hidden="true"><circle class="bg" cx="12" cy="12" r="${r}"/><circle class="fg" cx="12" cy="12" r="${r}" stroke-dasharray="${c.toFixed(1)}" stroke-dashoffset="${(c * (1 - pct / 100)).toFixed(1)}"/></svg>`;
  };

  ui.progress = (pct, kind = '', extra = '') => `<div class="${cls('progress', kind && `progress--${kind}`, extra)}" role="progressbar" aria-valuenow="${pct}" aria-valuemin="0" aria-valuemax="100"><span style="width:${pct}%"></span></div>`;

  ui.conf = (level) => {
    const label = { high: 'High', medium: 'Medium', low: 'Low', none: 'No match' }[level] || level;
    return `<span class="conf conf--${level}" title="Confidence: ${label}"><i><b></b><b></b><b></b></i>${label}</span>`;
  };

  ui.chip = (text, mod) => `<span class="${cls('chip', mod && `chip--${mod}`)}">${esc(text)}</span>`;

  // Release names / file paths: allow breaks only after separators.
  ui.releaseName = (raw, tag = 'p', extra = '') => {
    const html = esc(raw).replace(/([._\-\]\)\/])/g, (m) => (m === '.' || m === '_' ? `<span class="sep">${m}</span><wbr>` : `${m}<wbr>`));
    return `<${tag} class="${cls('release-name', extra)}" title="${esc(raw)}">${html}</${tag}>`;
  };
  ui.path = (p) => {
    const i = p.lastIndexOf('/');
    const dir = i >= 0 ? p.slice(0, i + 1) : '';
    const f = i >= 0 ? p.slice(i + 1) : p;
    const br = (s) => esc(s).replace(/([._\-\]\)\/ ])/g, '$1<wbr>');
    return `<span class="path">${dir ? `<span class="dir">${br(dir)}</span>` : ''}${br(f)}</span>`;
  };

  ui.poster = (id, extra = '', vt = '') => `<div class="${cls('media media--poster', extra)}"${vt ? ` style="view-transition-name:${vt}"` : ''}>${art.poster(T(id) ? T(id).art : id)}</div>`;
  ui.still = (id, n, extra = '') => `<div class="${cls('media media--wide', extra)}">${art.still(T(id) ? T(id).art : id, n)}</div>`;

  // Library status for a title → pill (shown without hover, always)
  ui.libPill = (t, onArt) => {
    const o = { onArt };
    switch (t.lib) {
      case 'ready': return ui.pill('ready', t.type === 'tv' ? 'Ready' : 'Ready', o);
      case 'downloading': return ui.pill('downloading', `${Math.round((t.progress || 0.42) * 100)}%`, { ...o, pct: Math.round((t.progress || 0.42) * 100) });
      case 'partial': return ui.pill('ready', `${t.ready}/${t.total}`, { ...o, icon: 'layers' });
      case 'needs_review': return ui.pill('review', 'Review', o);
      case 'needs_torrent': return ui.pill('needs_torrent', 'No torrent', o);
      default: return '';
    }
  };

  ui.posterCard = (id, opts = {}) => {
    const t = T(id); if (!t) return '';
    const href = opts.href || `#/title/${t.type}/${t.id}`;
    const sub = opts.sub ?? `${t.year}${t.type === 'tv' ? ' · Series' : t.runtime ? ` · ${t.runtime}` : ''}`;
    const badge = opts.badge ? `<span class="badge-tr">${opts.badge}</span>` : (opts.showLib !== false && t.lib && t.lib !== 'none' ? `<span class="badge-tl">${ui.libPill(t, true)}</span>` : '');
    const watchBar = opts.watched != null ? ui.progress(opts.watched, 'watched') : '';
    const fb = opts.feedback ? `<div class="feedback" role="group" aria-label="Feedback on ${esc(t.title)}">
        <button class="btn btn-ghost btn-sm btn-icon" data-act="fb" data-fb="like" aria-label="More like this">${icon('thumbsUp', { size: 16 })}</button>
        <button class="btn btn-ghost btn-sm btn-icon" data-act="fb" data-fb="dislike" aria-label="Less like this">${icon('thumbsDown', { size: 16 })}</button>
        <button class="btn btn-ghost btn-sm btn-icon" data-act="fbmenu" aria-label="More feedback: not interested, already seen">${icon('more', { size: 16 })}</button></div>` : '';
    return `<article class="pcard" data-id="${t.id}">
      <a href="${href}" class="pcard-link stack gap-2" aria-label="${esc(t.title)}, ${t.year}">
        <div class="media media--poster">${art.poster(t.art)}${badge}${watchBar}</div>
      </a>
      <div class="meta">
        <a href="${href}" class="title">${esc(t.title)}</a>
        ${opts.reason ? `<p class="reason">${esc(opts.reason)}</p>` : `<p class="sub">${esc(sub)}</p>`}
      </div>
      ${fb}
    </article>`;
  };

  // Up next / episode landscape card. Availability drives the action.
  ui.landCard = (u, opts = {}) => {
    const t = T(u.show);
    const ep = u.abs ? `${u.se} · #${u.abs}` : u.se;
    let pill = '', action = '';
    switch (u.availability) {
      case 'ready': pill = ui.pill('ready', 'Ready to watch'); action = `<button class="btn btn-ghost btn-sm" data-act="howto" aria-label="How to play ${esc(t.title)} in Infuse">${icon('tv', { size: 16 })}Play in Infuse</button>`; break;
      case 'downloading': pill = ui.pill('downloading', `Downloading ${u.pct}%`, { pct: u.pct }); action = `<a class="btn btn-ghost btn-sm" href="#/activity">Details</a>`; break;
      case 'needs_torrent': pill = ui.pill('needs_torrent'); action = `<button class="btn btn-secondary btn-sm" data-act="find" data-show="${t.id}" data-se="${u.se}">${icon('search', { size: 16 })}Find</button>`; break;
      case 'not_in_library': pill = ui.pill('not_in_library'); action = `<button class="btn btn-secondary btn-sm" data-act="add" data-id="${t.id}">${icon('plus', { size: 16 })}Add</button>`; break;
      case 'airs_on': pill = ui.pill('airs_on', u.airs); action = ''; break;
    }
    return `<article class="lcard" data-show="${t.id}">
      <a href="#/title/tv/${t.id}" class="media media--wide" aria-label="${esc(t.title)} ${ep}">${art.still(t.art, u.se.length + (u.abs || 3))}
        <div class="over"><span class="ep">${ep}</span>${opts.when ? `<span class="pill pill--glass">${esc(opts.when)}</span>` : ''}</div>
        ${u.availability === 'downloading' ? `<div class="progress" style="position:absolute;left:0;right:0;bottom:0;border-radius:0;z-index:3"><span style="width:${u.pct}%"></span></div>` : ''}
      </a>
      <div class="body">
        <span class="show">${esc(t.short)}</span>
        <a href="#/title/tv/${t.id}" class="name">${esc(u.name)}</a>
        <div class="foot">${pill}${action}</div>
      </div>
    </article>`;
  };

  ui.rail = ({ id, title, subtitle, kind = 'posters', items, seeAll, extraActions = '' }) => `
    <section class="section rail-wrap" aria-labelledby="${id}-h">
      <div class="section-head">
        <div class="titles"><h2 class="t-title-2" id="${id}-h">${esc(title)}</h2>${subtitle ? `<p class="t-small c-3">${esc(subtitle)}</p>` : ''}</div>
        <div class="actions">${extraActions}
          ${seeAll ? `<a class="btn btn-ghost btn-sm" href="${seeAll}">See all</a>` : ''}
          <div class="rail-arrows"><button class="btn btn-secondary btn-sm btn-icon" data-act="rail" data-dir="-1" data-rail="${id}" aria-label="Scroll ${esc(title)} left" disabled>${icon('chevronLeft', { size: 18 })}</button><button class="btn btn-secondary btn-sm btn-icon" data-act="rail" data-dir="1" data-rail="${id}" aria-label="Scroll ${esc(title)} right">${icon('chevronRight', { size: 18 })}</button></div>
        </div>
      </div>
      <div class="rail rail--${kind}" id="${id}" tabindex="-1">${items}</div>
    </section>`;

  ui.releaseCard = (r, opts = {}) => {
    const seedCls = r.seeders === 0 ? 'none' : r.seeders < 20 ? 'low' : '';
    const chips = r.attrs.map((a, i) => {
      if (/pack|Episodes|Complete/.test(a)) return ui.chip(a, 'pack');
      if (/2160p|DV|HDR|Atmos/.test(a)) return ui.chip(a, 'hi');
      return ui.chip(a);
    }).join('');
    const warn = r.warn ? `<p class="warnline">${icon('alertTriangle', { size: 15 })}${esc(r.warn)}</p>` : '';
    const note = r.note ? `<p class="t-small c-2 row gap-2 start">${icon('info', { size: 15, cls: 'c-3' })}<span>${esc(r.note)}</span></p>` : '';
    const best = r.best && !opts.noBest ? `<div class="best-head">${icon('sparkle', { size: 15 })}Best match<span class="c-3 w-medium">· score ${r.score}</span></div>` : '';
    const why = r.best && r.why && !opts.noBest ? `<p class="why">${r.why.map((w) => `<span>${esc(w)}</span>`).join('')}</p>` : '';
    const added = opts.added;
    return `<article class="${cls('release', r.best && !opts.noBest && 'is-best', r.seeders === 0 && 'is-dim', added && 'is-added')}" data-rid="${r.id}">
      <div class="r-main stack gap-3" style="min-width:0">
        ${best}
        ${ui.releaseName(r.raw)}
        <div class="chips">${chips}</div>
        <div class="facts">
          <span>${icon('hardDrive', { size: 14 })}${r.size}</span>
          <span class="seeds ${seedCls}">${icon('arrowUp', { size: 14 })}${r.seeders} seeders</span>
          <span>${icon('arrowDown', { size: 14 })}${r.leechers}</span>
          <span>${icon('clock', { size: 14 })}${r.age}</span>
          <span>${icon('server', { size: 14 })}${r.indexer}</span>
        </div>
        ${why}${note}${warn}
      </div>
      <div class="actions">
        ${added ? `<span class="btn btn-ready">${icon('check', { size: 18 })}Added</span>` :
          `<button class="btn ${r.best && !opts.noBest ? 'btn-primary' : 'btn-secondary'}" data-act="add-release" data-rid="${r.id}">${icon('plus', { size: 18 })}Add</button>`}
        <button class="btn btn-secondary" data-act="inspect" data-rid="${r.id}">${icon('scanSearch', { size: 18 })}Inspect</button>
        <span class="spacer"></span>
        <button class="btn btn-ghost btn-icon btn-magnet" data-act="copy-magnet" data-rid="${r.id}" aria-label="Copy magnet link">${icon('magnet', { size: 18 })}<span class="lbl" aria-hidden="true">Magnet</span></button>
      </div>
    </article>`;
  };

  ui.epRow = (ep, opts = {}) => {
    const st = ep.state;
    let pill = '';
    if (st === 'watched') pill = ui.pill('watched');
    else if (st === 'ready') pill = ui.pill('ready');
    else if (st === 'downloading') pill = ui.pill('downloading', `${ep.pct || 40}%`, { pct: ep.pct || 40 });
    else if (st === 'missing') pill = ui.pill('missing');
    else if (st === 'upcoming') pill = ui.pill('upcoming', ep.aired);
    else if (st === 'review') pill = ui.pill('review');
    const action = st === 'missing'
      ? `<button class="btn btn-ghost btn-sm btn-icon" data-act="find-ep" data-ep="${ep.n}" aria-label="Find a torrent for episode ${ep.n}">${icon('search', { size: 17 })}</button>`
      : st === 'review' ? `<a class="btn btn-ghost btn-sm btn-icon" href="#/mapping" aria-label="Review mapping for episode ${ep.n}">${icon('chevronRight', { size: 17 })}</a>`
      : `<button class="btn btn-ghost btn-sm btn-icon" data-act="ep-menu" data-ep="${ep.n}" aria-label="Episode ${ep.n} options: subtitles, replace file, remove">${icon('moreV', { size: 17 })}</button>`;
    const rich = opts.rich;
    return `<li class="${cls('eprow', rich && 'eprow--rich', opts.next && 'is-next', st === 'watched' && 'is-watched')}" id="ep-${ep.n}">
      ${rich ? `<div class="media media--wide">${art.still(opts.art, ep.n)}</div>` : `<span class="no">${ep.n}</span>`}
      <div class="info">
        ${opts.next ? `<span class="t-micro c-accent">Up next</span>` : ''}
        <span class="name">${rich ? `<span class="c-3 t-num">${ep.n}.</span> ` : ''}${esc(ep.name)}</span>
        <span class="sub">${opts.se || ''}${ep.runtime ? ` · ${ep.runtime} min` : ''}${ep.aired && st !== 'upcoming' ? ` · ${ep.aired}` : ''}</span>
      </div>
      <div class="end">${pill}${action}</div>
    </li>`;
  };

  ui.tcard = (tr) => {
    const t = T(tr.title);
    const st = tr.streaming ? 'streaming' : tr.state;
    const pill = st === 'streaming' ? ui.pill('streaming', 'Streaming now') : st === 'downloading' ? ui.pill('downloading', `${tr.pct}%`, { pct: tr.pct }) : st === 'paused' ? ui.pill('paused') : ui.pill('ready', 'Complete', { icon: 'check' });
    const health = tr.health === 'weak' ? `<span class="c-accent" style="color:var(--st-warning)">${icon('alertTriangle', { size: 14 })}Few peers</span>` : '';
    const active = st === 'downloading' || st === 'streaming';
    return `<article class="${cls('tcard', tr.streaming && 'is-streaming')}">
      <a href="#/title/${t.type}/${t.id}" aria-label="${esc(t.title)}">${ui.poster(t.id)}</a>
      <div class="stack gap-2" style="min-width:0">
        <div class="top"><div class="stack" style="min-width:0"><a href="#/title/${t.type}/${t.id}" class="w-semibold">${esc(tr.label)}</a>${ui.releaseName(tr.name, 'p', 'c-3 clamp-2')}</div>${pill}</div>
        ${tr.pct < 100 ? ui.progress(tr.pct, st === 'paused' ? '' : '', '') : ''}
        <div class="stats">
          ${active ? `<span>${icon('arrowDown', { size: 14 })}${tr.down}</span><span>${icon('arrowUp', { size: 14 })}${tr.up}</span><span>${icon('users', { size: 14 })}${tr.peers} peers</span>` : ''}
          ${tr.eta ? `<span>${icon('clock', { size: 14 })}${tr.eta} left</span>` : ''}
          <span>${icon('hardDrive', { size: 14 })}${tr.size}</span>
          ${tr.lastPlayed ? `<span>${icon('history', { size: 14 })}${tr.lastPlayed}</span>` : ''}
          ${health}
        </div>
      </div>
      ${tr.streaming ? `<div class="full banner banner--accent" style="padding:10px 12px">${icon('tv', { size: 18 })}<div class="grow"><strong>${esc(tr.streaming.file)}</strong><span>${esc(tr.streaming.who)} · ${tr.streaming.at} · ${tr.streaming.buffer}</span></div></div>` : ''}
      <div class="full row gap-2">
        ${st === 'paused' ? `<button class="btn btn-secondary btn-sm" data-act="resume">${icon('play', { size: 15 })}Resume</button>` : active ? `<button class="btn btn-secondary btn-sm" data-act="pause">${icon('pause', { size: 15 })}Pause</button>` : ''}
        <a class="btn btn-ghost btn-sm" href="#/activity/${encodeURIComponent(tr.hash)}">Details</a>
        <span class="grow"></span>
        <button class="btn btn-ghost btn-sm btn-icon" data-act="remove-torrent" aria-label="Remove torrent">${icon('trash', { size: 16 })}</button>
      </div>
    </article>`;
  };

  // Cat silhouettes for empty states (sitting, sleeping, peeking)
  ui.cat = (variant = 'sit', size = 120) => {
    const G = brand.G;
    const body = 'M41 66C33 76 25 96 25 116c0 6 4 10 10 10h50c6 0 10-4 10-10 0-20-8-40-16-50z';
    const tail = 'M90 122c14 0 22-6 22-16 0-8-6-12-11-10';
    const head = `<g transform="translate(28 18) scale(1)"><path d="${G.head}" fill="currentColor"/></g>`;
    const eyesOpen = `<g transform="translate(28 18)"><path d="${G.eyeL}" fill="var(--accent)"/><path d="${G.eyeR}" fill="var(--accent)"/><ellipse cx="${G.pupL[0]}" cy="${G.pupL[1]}" rx="1.2" ry="3" fill="var(--ink-1)"/><ellipse cx="${G.pupR[0]}" cy="${G.pupR[1]}" rx="1.2" ry="3" fill="var(--ink-1)"/></g>`;
    const eyesClosed = `<g transform="translate(28 18)" fill="none" stroke="var(--text-3)" stroke-width="1.8" stroke-linecap="round"><path d="M16 38q6 4 12 0"/><path d="M36 38q6 4 12 0"/></g>`;
    if (variant === 'peek') {
      return `<svg class="art" width="${size}" height="${size * 0.6}" viewBox="0 0 120 72" aria-hidden="true"><g transform="translate(28 2)"><path d="${G.head}" fill="currentColor"/><path d="${G.eyeL}" fill="var(--accent)"/><path d="${G.eyeR}" fill="var(--accent)"/></g><rect x="4" y="52" width="112" height="3" rx="1.5" fill="var(--line-3)"/></svg>`;
    }
    const eyes = variant === 'sleep' ? eyesClosed : variant === 'blank' ? '' : eyesOpen;
    const tailEl = `<path d="${tail}" fill="none" stroke="currentColor" stroke-width="7" stroke-linecap="round"/>`;
    return `<svg class="art" width="${size}" height="${size * 1.12}" viewBox="0 0 120 134" aria-hidden="true">${tailEl}<path d="${body}" fill="currentColor"/>${head}${eyes}</svg>`;
  };

  ui.empty = ({ cat = 'sit', title, body, actions = '' }) => `<div class="empty">${ui.cat(cat, 112)}<h3>${esc(title)}</h3>${body ? `<p>${body}</p>` : ''}${actions ? `<div class="actions">${actions}</div>` : ''}</div>`;
  ui.loader = (text = 'Loading…', sub = '') => `<div class="loader" role="status" aria-live="polite"><div class="cat-dark">${brand.eyes(76)}</div><div class="stack gap-1"><p class="w-medium c-1">${esc(text)}</p>${sub ? `<p class="t-small c-3">${esc(sub)}</p>` : ''}</div></div>`;

  ui.skelPosters = (n = 6) => Array.from({ length: n }, () => `<div class="stack gap-2"><div class="skel skel-poster"></div><div class="skel skel-line" style="width:80%"></div><div class="skel skel-line" style="width:45%"></div></div>`).join('');
  ui.skelWide = (n = 3) => Array.from({ length: n }, () => `<div class="stack gap-3"><div class="skel skel-wide"></div><div class="skel skel-line" style="width:40%"></div><div class="skel skel-line" style="width:75%"></div></div>`).join('');

  ui.seg = (items, selected, attrs = '') => `<div class="seg" role="tablist" ${attrs}>${items.map(([v, l]) => `<button role="tab" aria-selected="${v === selected}" data-v="${v}">${l}</button>`).join('')}</div>`;
  ui.tabs = (items, selected, attrs = '') => `<div class="tabs" role="tablist" ${attrs}>${items.map(([v, l, n]) => `<button role="tab" aria-selected="${v === selected}" data-v="${v}">${l}${n != null ? ` <span class="n">${n}</span>` : ''}</button>`).join('')}</div>`;
  ui.fchip = (label, on, n, attrs = '') => `<button class="fchip" aria-pressed="${!!on}" ${attrs}>${label}${n != null ? ` <span class="n">${n}</span>` : ''}</button>`;
  ui.switch = (id, on, label) => `<label class="switch" for="${id}"><input type="checkbox" id="${id}" ${on ? 'checked' : ''} aria-label="${esc(label)}"><span class="track"></span><span class="thumb"></span></label>`;

  // Prototype-only state switcher (hidden in gallery embeds)
  ui.protoStates = (base, states, current) => {
    const cur = states.find((x) => x[0] === (current ?? states[0][0])) || states[0];
    return `<details class="proto-states"><summary aria-label="Prototype states">${MS.icon('layers', { size: 14 })}<span>State: ${cur[1]}</span></summary><div class="ps-list" role="navigation" aria-label="Prototype states">${states.map(([v, l]) => `<a href="${base}${base.includes('?') ? '&' : '?'}state=${v}" aria-current="${v === cur[0]}">${l}</a>`).join('')}</div></details>`;
  };

  // Phone large-title header (hidden ≥1024 where the top bar carries the nav)
  ui.largeTitle = (title, actions = '') => `<header class="large-title page phone-only"><h1>${esc(title)}</h1><div class="actions">${actions}<button class="avatar-btn" data-act="menu" aria-label="Account menu: settings, log out">${brand.markMono(20, 'var(--text-2)')}</button></div></header>`;

  ui.phoneBar = (extra = '') => `<div class="phone-bar"><a href="#/home" class="brand" aria-label="momoshtrem">${brand.mark(26)}<span>momoshtrem</span></a><div class="row gap-2">${extra}<button class="avatar-btn" data-act="menu" aria-label="Account menu: settings, log out">${brand.markMono(20, 'var(--text-2)')}</button></div></div>`;

  // ======================================================================
  // App: router, shell, overlays, toasts
  // ======================================================================
  const routes = [];
  const sheets = {};
  const app = {
    state: {
      trakt: 'connected',          // 'off' | 'code' | 'waiting' | 'syncing' | 'connected'
      frieren: 'needs_torrent',    // 'needs_torrent' | 'mapping' | 'ready'
      goodplace: 'none',           // 'none' | 'added'
      tsQuery: null,               // survives sheet close/reopen
      tsScope: null,
      tsFilters: {},
    },
    params: {},
    path: '/home',
  };
  const DEFAULTS = JSON.parse(JSON.stringify(app.state));
  // Gallery frames (?embed=1) always start from the defaults and never persist.
  const isEmbed = /[?&]embed=1/.test(location.search);
  try { const s = !isEmbed && sessionStorage.getItem('ms-proto'); if (s) Object.assign(app.state, JSON.parse(s)); } catch (e) { /* storage unavailable */ }
  app.save = () => { if (isEmbed) return; try { sessionStorage.setItem('ms-proto', JSON.stringify(app.state)); } catch (e) { /* ignore */ } };
  app.reset = () => { try { sessionStorage.removeItem('ms-proto'); } catch (e) { /* ignore */ } Object.keys(app.state).forEach((k) => delete app.state[k]); Object.assign(app.state, JSON.parse(JSON.stringify(DEFAULTS))); };

  app.route = (pattern, render) => {
    const keys = [];
    const re = new RegExp('^' + pattern.replace(/:(\w+)/g, (_, k) => { keys.push(k); return '([^/]+)'; }) + '$');
    routes.push({ re, keys, render });
  };
  app.sheet = (name, render) => { sheets[name] = render; };

  function parse(hash) {
    const h = (hash || '').replace(/^#/, '') || '/home';
    const [path, qs] = h.split('?');
    const params = {};
    if (qs) qs.split('&').forEach((kv) => { const [k, v] = kv.split('='); params[decodeURIComponent(k)] = decodeURIComponent(v || ''); });
    return { path: path || '/home', params };
  }

  const embedded = () => document.documentElement.dataset.embed === '1';
  app.go = (hash, opts = {}) => {
    const run = () => render(hash);
    try { if (location.hash !== hash) history.pushState(null, '', hash); } catch (e) { /* sandbox */ }
    if (!opts.noTransition && document.startViewTransition && !reduced()) document.startViewTransition(run);
    else run();
  };

  let current = null;
  function render(hash) {
    const { path, params } = parse(hash);
    app.path = path; app.params = params;
    if (params.reset === '1') { app.reset(); }
    // Allow gallery to set state through query: ?st.frieren=ready
    Object.keys(params).filter((k) => k.startsWith('st.')).forEach((k) => { app.state[k.slice(3)] = params[k]; });
    let match = null;
    for (const r of routes) { const m = path.match(r.re); if (m) { match = { r, args: Object.fromEntries(r.keys.map((k, i) => [k, decodeURIComponent(m[i + 1])])) }; break; } }
    if (!match) match = { r: routes[0], args: {} };
    const view = match.r.render({ ...match.args, ...params, params });
    current = view;
    document.title = `${view.title ? view.title + ' · ' : ''}momoshtrem`;
    shell(view);
    window.scrollTo(0, 0);
    closeOverlays(true);
    if (params.sheet) app.openSheet(params.sheet, params);
    if (params.palette) app.openPalette(params.q || '');
    if (params.dialog) app.openDialog(params.dialog);
    if (params.toast) app.toast(params.toast);
    if (params.menu) app.openMenu();
  }

  const NAV = [
    ['home', '#/home', 'For You', 'sparkles'],
    ['discover', '#/discover', 'Discover', 'compass'],
    ['search', '#/search', 'Search', 'search'],
    ['library', '#/library', 'Library', 'library'],
    ['activity', '#/activity', 'Activity', 'activity'],
  ];

  function shell(view) {
    const root = document.getElementById('app');
    if (view.bare) { root.innerHTML = `<main id="main" class="main main--bare">${view.html}</main>${overlayRoots()}`; mountView(view); return; }
    const top = `<header class="topbar ${view.overArt ? 'is-over-art' : 'is-solid'}" id="topbar">
        <a href="#/home" class="brand" aria-label="momoshtrem, For You">${brand.mark(30)}<span>momoshtrem</span></a>
        <nav aria-label="Primary">${NAV.filter((n) => n[0] !== 'search').map(([k, href, label, ic]) => `<a href="${href}" ${view.tab === k ? 'aria-current="page"' : ''}>${label}${k === 'activity' ? '<span class="badge">3</span>' : ''}</a>`).join('')}</nav>
        <button class="search-trigger" data-act="palette" aria-label="Search titles, your library and actions">${icon('search', { size: 16 })}<span>Search or jump to…</span><span class="kbd">⌘K</span></button>
        <a class="ask-trigger" href="#/ask">${brand.avatar(28)}Ask momoshtrem</a>
        <button class="avatar-btn" data-act="menu" aria-label="Account menu: settings, log out" aria-haspopup="menu">${brand.markMono(20, 'var(--text-2)')}</button>
      </header>`;
    const tabs = `<nav class="tabbar" aria-label="Primary">${NAV.map(([k, href, label, ic]) => `<a href="${href}" ${view.tab === k ? 'aria-current="page"' : ''}>${icon(ic, { size: 24, stroke: view.tab === k ? 2 : 1.6 })}<span>${label}</span>${k === 'activity' ? '<span class="dot">3</span>' : ''}</a>`).join('')}</nav>`;
    root.innerHTML = `<a href="#main" class="skip">Skip to content</a>${top}<main id="main" class="main ${view.mainCls || ''}">${view.html}</main>${view.noTabbar ? '' : tabs}${overlayRoots()}`;
    mountView(view);
  }
  const overlayRoots = () => '<div id="overlays"></div><div class="toast-region" id="toasts" aria-live="polite"></div>';

  function mountView(view) {
    const main = document.getElementById('main');
    if (view.mount) view.mount(main, app);
    // topbar solidifies after the hero
    const tb = document.getElementById('topbar');
    if (tb && view.overArt) {
      const onScroll = () => { const solid = window.scrollY > 120; tb.classList.toggle('is-solid', solid); tb.classList.toggle('is-over-art', !solid); };
      window.removeEventListener('scroll', app._onScroll || (() => {}));
      app._onScroll = onScroll; window.addEventListener('scroll', onScroll, { passive: true });
    }
    initRails(document);
  }

  function initRails(scope) {
    scope.querySelectorAll('.rail').forEach((rail) => {
      const id = rail.id; if (!id) return;
      const upd = () => {
        const prev = scope.querySelector(`[data-rail="${id}"][data-dir="-1"]`), next = scope.querySelector(`[data-rail="${id}"][data-dir="1"]`);
        if (prev) prev.disabled = rail.scrollLeft < 8;
        if (next) next.disabled = rail.scrollLeft + rail.clientWidth > rail.scrollWidth - 8;
      };
      rail.addEventListener('scroll', upd, { passive: true }); upd();
    });
  }
  app.initRails = initRails;

  // ---- overlays ----------------------------------------------------------
  const stack = [];
  let lastFocus = null;
  function closeOverlays(silent) { stack.splice(0).forEach((o) => o.el.remove()); const ov = document.getElementById('overlays'); if (ov) ov.innerHTML = ''; document.body.style.overflow = ''; }
  app.close = () => {
    const top = stack.pop(); if (!top) return;
    top.el.remove(); if (top.onClose) top.onClose();
    if (!stack.length) { document.body.style.overflow = ''; if (lastFocus) lastFocus.focus({ preventScroll: true }); }
  };
  app.closeAll = () => { while (stack.length) app.close(); };
  function push(html, opts = {}) {
    const ov = document.getElementById('overlays');
    const el = document.createElement('div');
    el.className = 'overlay-layer';
    el.innerHTML = `${opts.scrim === false ? '' : '<div class="scrim" data-act="close"></div>'}${html}`;
    if (!stack.length) lastFocus = document.activeElement;
    ov.appendChild(el); stack.push({ el, onClose: opts.onClose });
    document.body.style.overflow = 'hidden';
    const f = el.querySelector('[autofocus], .sheet input, .palette input, .dialog button');
    if (f && !opts.noFocus) setTimeout(() => f.focus({ preventScroll: true }), 30);
    initRails(el);
    return el;
  }
  app.openSheet = (name, props = {}) => {
    const s = sheets[name]; if (!s) return;
    const v = s(props, app);
    const el = push(`<div class="${cls('sheet', v.cls)}" role="dialog" aria-modal="true" aria-labelledby="${name}-title">${v.html}</div>`, { onClose: v.onClose });
    if (v.mount) v.mount(el.querySelector('.sheet'), app);
    return el;
  };
  app.replaceSheet = (name, props = {}) => { app.close(); return app.openSheet(name, props); };
  app.openDialog = (key, d) => {
    const dd = d || DIALOGS[key] || DIALOGS.remove;
    push(`<div class="dialog" role="alertdialog" aria-modal="true" aria-labelledby="dlg-t" aria-describedby="dlg-b">
      <h2 class="t-title-3" id="dlg-t">${dd.title}</h2><p class="c-2" id="dlg-b">${dd.body}</p>
      ${dd.extra || ''}
      <div class="actions"><button class="btn btn-secondary" data-act="close">${dd.cancel || 'Cancel'}</button><button class="btn ${dd.danger ? 'btn-danger' : 'btn-primary'}" data-act="${dd.act || 'close'}">${dd.confirm}</button></div></div>`);
  };
  const DIALOGS = {
    remove: { title: 'Remove Severance from your library?', body: 'This removes the show and its 19 episode mappings. Downloaded data for its 2 torrents is deleted too. Your Trakt history isn\'t touched.', confirm: 'Remove show', danger: true, act: 'removed',
      extra: '<label class="row gap-3" style="min-height:44px"><input type="checkbox" checked style="width:20px;height:20px;accent-color:var(--accent)"> <span class="t-small c-2">Also delete downloaded data (58.3 GB)</span></label>' },
    'remove-torrent': { title: 'Remove this torrent?', body: 'Infuse will no longer be able to play Severance · Season 2. The show stays in your library and shows "Needs a torrent".', confirm: 'Remove torrent', danger: true, act: 'removed' },
    disconnect: { title: 'Disconnect Trakt?', body: 'Your imported history, ratings and watchlist will be deleted from momoshtrem. Nothing changes on Trakt. Personal picks fall back to trending.', confirm: 'Disconnect', danger: true, act: 'trakt-off' },
  };

  app.openPalette = (q = '') => {
    const el = push(MS.palette ? MS.palette(q) : '', {});
    if (MS.paletteMount) MS.paletteMount(el, app);
  };
  app.openMenu = () => {
    const isDesk = window.matchMedia('(min-width: 1024px)').matches;
    const menu = `<div class="menu" role="menu" style="position:fixed;z-index:var(--z-palette);${isDesk ? 'top:64px;right:var(--gutter)' : 'left:16px;right:16px;bottom:calc(var(--tabbar-h) + 12px + env(safe-area-inset-bottom,0px))'}">
      <div class="row gap-3" style="padding:10px 12px">${brand.avatar(36)}<div class="stack"><span class="w-semibold">teymur</span><span class="t-small c-3">Trakt · synced 4 min ago</span></div></div>
      <div class="sep"></div>
      <a role="menuitem" href="#/settings">${icon('settings', { size: 18 })}Settings</a>
      <a role="menuitem" href="#/settings?section=playback">${icon('tv', { size: 18 })}Infuse setup</a>
      <button role="menuitem" data-act="palette">${icon('command', { size: 18 })}Command palette<span class="hint">⌘K</span></button>
      <div class="sep"></div>
      <a role="menuitem" href="#/signin">${icon('logout', { size: 18 })}Log out</a></div>`;
    push(menu, { scrim: true });
  };

  app.toast = (key, t) => {
    const tt = t || TOASTS[key] || { kind: 'info', title: key };
    const region = document.getElementById('toasts'); if (!region) return;
    const el = document.createElement('div');
    el.className = `toast toast--${tt.kind}`; el.setAttribute('role', 'status');
    el.innerHTML = `${icon(tt.icon || (tt.kind === 'ready' ? 'checkCheck' : tt.kind === 'error' ? 'alertTriangle' : 'info'), { size: 20 })}<div class="grow"><strong>${tt.title}</strong>${tt.body ? `<span class="c-2">${tt.body}</span>` : ''}</div>${tt.action ? `<a class="btn btn-ghost btn-sm" href="${tt.href || '#'}" ${tt.act ? `data-act="${tt.act}"` : ''}>${tt.action}</a>` : ''}<button class="btn btn-ghost btn-sm btn-icon" aria-label="Dismiss" data-act="dismiss-toast">${icon('x', { size: 16 })}</button>`;
    region.appendChild(el);
    if (!tt.sticky) setTimeout(() => el.remove(), tt.ms || 5200);
  };
  const TOASTS = {
    added: { kind: 'ready', title: 'Added to library', body: 'The Good Place · 4 seasons, 53 episodes', action: 'Find a torrent', act: 'find-goodplace' },
    copied: { kind: 'ready', title: 'Copied', body: 'Paste it into Infuse › Add files › WebDAV', icon: 'copy' },
    magnet: { kind: 'ready', title: 'Magnet link copied', icon: 'magnet' },
    ready: { kind: 'ready', title: 'Frieren S01E29–E38 are ready', body: 'They\'ll appear in Infuse after its next library refresh.', action: 'How to play', href: '#/title/tv/frieren?howto=1', sticky: true },
    removed: { kind: 'info', title: 'Removed', body: 'Undo within 10 seconds', action: 'Undo', icon: 'trash' },
    feedback: { kind: 'info', title: 'Got it, fewer like this', action: 'Undo', icon: 'thumbsDown' },
    error: { kind: 'error', title: 'Couldn\'t reach momoshtrem', body: 'Retrying in 10 s. Your changes are kept.', action: 'Retry now', sticky: true },
    'trakt-off': { kind: 'info', title: 'Trakt disconnected', icon: 'link' },
  };

  // ---- global events -----------------------------------------------------
  document.addEventListener('click', (e) => {
    const a = e.target.closest('a[href^="#/"]');
    if (a && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      const href = a.getAttribute('href');
      // Shared-element: tag the clicked poster so the detail hero morphs from it
      const media = a.closest('.pcard')?.querySelector('.media');
      if (media && !reduced()) media.style.viewTransitionName = 'hero-poster';
      app.go(href);
      return;
    }
    const b = e.target.closest('[data-act]');
    if (!b) return;
    const act = b.dataset.act;
    const handler = app.actions[act];
    if (handler) { e.preventDefault(); handler(b, e); }
  });
  document.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); app.openPalette(); }
    if (e.key === 'Escape' && stack.length) { e.preventDefault(); app.close(); }
    if (e.key === '/' && !stack.length && !/INPUT|TEXTAREA/.test(document.activeElement.tagName)) { e.preventDefault(); app.openPalette(); }
  });
  window.addEventListener('popstate', () => render(location.hash));
  window.addEventListener('hashchange', () => render(location.hash));

  app.actions = {
    close: () => app.close(),
    'dismiss-toast': (b) => b.closest('.toast').remove(),
    palette: () => { app.closeAll(); app.openPalette(); },
    menu: () => app.openMenu(),
    rail: (b) => { const r = document.getElementById(b.dataset.rail); if (r) r.scrollBy({ left: Number(b.dataset.dir) * r.clientWidth * 0.85, behavior: reduced() ? 'auto' : 'smooth' }); },
    howto: () => app.openSheet('howto'),
    copy: (b) => { const v = b.dataset.copy; try { navigator.clipboard.writeText(v).catch(() => {}); } catch (e) { /* ignore */ } app.toast('copied'); },
    'copy-magnet': () => app.toast('magnet'),
    fb: (b) => { b.closest('.pcard')?.classList.add('is-dismissed'); app.toast('feedback'); },
    fbmenu: (b) => app.openSheet('feedback', { id: b.closest('.pcard')?.dataset.id }),
    removed: () => { app.close(); app.toast('removed'); },
    'remove-torrent': () => app.openDialog('remove-torrent'),
    'trakt-off': () => { app.close(); app.state.trakt = 'off'; app.save(); app.toast('trakt-off'); render(location.hash || '#/settings'); },
    find: (b) => app.openSheet('torrent', { show: b.dataset.show, se: b.dataset.se }),
    'find-goodplace': () => app.openSheet('torrent', { show: 'goodplace', scope: 'season', season: '1' }),
  };

  // Generic "how to play" sheet (Infuse over WebDAV)
  app.sheet('howto', () => ({
    cls: 'sheet--narrow',
    html: `<div class="sheet-grabber" aria-hidden="true"></div>
      <div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="howto-title">Play it in Infuse</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
      <div class="sheet-body stack gap-5">
        <p class="c-2">momoshtrem doesn't play video. It gets things ready, and Infuse on your Apple TV streams them over WebDAV.</p>
        <ol class="stack gap-4 howto-steps">
          <li><span class="n">1</span><div><strong>Open Infuse on Apple TV</strong><p class="t-small c-2">Settings › Shares › Add share › WebDAV</p></div></li>
          <li><span class="n">2</span><div class="stack gap-2" style="min-width:0"><strong>Use this address</strong>
            <div class="copy-field"><code class="path">https://momoshtrem.duckdns.org:36911/</code><button class="btn btn-secondary btn-sm" data-act="copy" data-copy="https://momoshtrem.duckdns.org:36911/">${icon('copy', { size: 15 })}Copy</button></div>
            <p class="t-small c-3">Username and password are the same as here.</p></div></li>
          <li><span class="n">3</span><div><strong>Find it under Library</strong><p class="t-small c-2">Shows › Frieren › Season 01. New episodes appear after Infuse's next library refresh (pull down to refresh).</p></div></li>
        </ol>
      </div>`,
  }));

  MS.ui = ui;
  MS.app = app;
  MS.STATUS = STATUS;
  MS.start = () => render(location.hash || '#/home');
})();
