/* Library. Route /library (tab: library).
   States (?state=): default (grid) · list · empty (first time) · filtered-empty · loading
   Params persisted in the URL: filter, sort, view, q.
   Data mirrors GET /library (PLAN.md §6): episodes_total / episodes_ready / episodes_watched / needs_review. */
(function () {
  const { ui, app, icon, data: D } = MS;
  const esc = ui.esc;
  const cls = (...a) => a.filter(Boolean).join(' ');
  const T = (id) => D.T[id];
  const isDesk = () => window.matchMedia('(min-width: 1024px)').matches;

  // Library rows. `addedD` / `watchedD` are days ago (for sorting); null = never watched.
  function rows() {
    const fr = app.state.frieren;
    const L = [
      { id: 'interstellar', addedD: 1, added: 'Oct 10', watchedD: null, size: 63.1, dl: 42, health: 'Few peers' },
      { id: 'frieren', addedD: 0, added: 'Today', watchedD: 1, watchedAt: 'Yesterday', size: fr === 'needs_torrent' ? 37.8 : 51.0,
        ready: fr === 'ready' ? 38 : 28, total: 38, watched: 28, review: fr === 'ready' ? 0 : 1, dl: fr === 'mapping' ? 12 : fr === 'ready' ? 64 : null },
      { id: 'mushishi', addedD: 2, added: 'Oct 9', watchedD: null, size: 14.6, ready: 12, total: 26, watched: 0 },
      { id: 'perfectdays', addedD: 3, added: 'Oct 8', watchedD: null, size: 0, noTorrent: true },
      { id: 'columbus', addedD: 7, added: 'Oct 4', watchedD: 0.5, watchedAt: 'Last night', size: 9.2, pct: 46 },
      { id: 'shogun', addedD: 9, added: 'Oct 2', watchedD: 0, watchedAt: 'Now', size: 42.6, ready: 6, total: 10, watched: 4, dl: 61, streaming: true },
      { id: 'thebear', addedD: 11, added: 'Sep 30', watchedD: 5, watchedAt: '5 days ago', size: 64.2, ready: 38, total: 38, watched: 30 },
      { id: 'dune2', addedD: 19, added: 'Sep 22', watchedD: null, size: 18.4, pct: 0 },
      { id: 'doctorwho', addedD: 58, added: 'Aug 14', watchedD: 21, watchedAt: '3 weeks ago', size: 96.4, ready: 40, total: 52, watched: 26, review: 3 },
      { id: 'arrival', addedD: 91, added: 'Jul 12', watchedD: 42, watchedAt: 'Aug 30', size: 24.8, pct: 100 },
      { id: 'totoro', addedD: 113, added: 'Jun 20', watchedD: null, size: 6.9, pct: 0 },
      { id: 'bluey', addedD: 130, added: 'Jun 3', watchedD: 1.2, watchedAt: 'Yesterday', size: 61.0, ready: 153, total: 153, watched: 120 },
      { id: 'severance', addedD: 268, added: 'Jan 17', watchedD: 2, watchedAt: '2 days ago', size: 58.3, ready: 14, total: 19, watched: 13 },
      { id: 'silo', addedD: 520, added: 'May 2024', watchedD: 39, watchedAt: 'Sep 2', size: 48.1, ready: 20, total: 20, watched: 20 },
      { id: 'onepiece', addedD: 900, added: 'Mar 2024', watchedD: 3, watchedAt: '3 days ago', size: 1454, ready: 1088, total: 1124, watched: 1071 },
    ];
    if (app.state.goodplace === 'added') L.unshift({ id: 'goodplace', addedD: -1, added: 'Just now', watchedD: null, size: 0, ready: 0, total: 53, watched: 0, noTorrent: true });
    return L.filter((r) => !removed.has(r.id)).map((r) => {
      const t = T(r.id);
      const tv = t.type === 'tv';
      const complete = tv ? r.ready === r.total : !r.noTorrent && r.dl == null;
      const watching = tv ? r.watched > 0 && r.watched < r.total : r.pct > 0 && r.pct < 100;
      return { ...r, t, tv, complete, watching, finished: tv ? r.watched === r.total : r.pct === 100 };
    });
  }

  const removed = new Set();
  const FILTERS = [
    ['all', 'All', () => true],
    ['movies', 'Movies', (r) => !r.tv],
    ['shows', 'Shows', (r) => r.tv],
    ['watching', 'Watching', (r) => r.watching],
    ['ready', 'Ready', (r) => r.complete],
    ['incomplete', 'Incomplete', (r) => !r.complete],
    ['review', 'Needs review', (r) => r.review > 0],
  ];
  const SORTS = [['added', 'Recently added'], ['title', 'Title'], ['watched', 'Last watched'], ['size', 'Size']];
  const sorters = {
    added: (a, b) => a.addedD - b.addedD,
    title: (a, b) => a.t.title.localeCompare(b.t.title),
    watched: (a, b) => (a.watchedD ?? 1e9) - (b.watchedD ?? 1e9),
    size: (a, b) => b.size - a.size,
  };
  const fmtSize = (gb) => (!gb ? '—' : gb >= 1000 ? `${(gb / 1000).toFixed(2)} TB` : `${gb.toFixed(1)} GB`);
  const num = (n) => n.toLocaleString('en-US');

  let S = null; // view state: { filter, sort, view, q, state, forceNoReview }

  function visible() {
    let list = rows();
    if (S.forceNoReview) list = list.map((r) => ({ ...r, review: 0 }));
    return list;
  }
  function apply(list) {
    const f = FILTERS.find((x) => x[0] === S.filter) || FILTERS[0];
    const q = S.q.trim().toLowerCase();
    return list.filter(f[2]).filter((r) => !q || r.t.title.toLowerCase().includes(q) || (r.t.short || '').toLowerCase().includes(q)).sort(sorters[S.sort] || sorters.added);
  }

  // ---- Status pieces (always visible, never hover-only) -----------------------
  function posterBadge(r) {
    // Only states that want attention get a badge; "ready" lives in the status line below.
    if (r.review) return ui.pill('review', 'Review', { onArt: true });
    if (r.streaming) return ui.pill('streaming', 'Playing', { onArt: true });
    if (r.dl != null) return ui.pill('downloading', `${r.dl}%`, { onArt: true, pct: r.dl });
    if (r.noTorrent) return ui.pill('needs_torrent', 'No torrent', { onArt: true });
    return '';
  }
  function readyLine(r) {
    if (r.tv) {
      if (!r.ready && r.noTorrent) return `<span class="ls ls--missing">${icon('search', { size: 13 })}Needs a torrent</span>`;
      return `<span class="${cls('ls', r.complete ? 'ls--ready' : 'ls--partial')}">${icon(r.complete ? 'checkCheck' : 'layers', { size: 13 })}<span><b>${num(r.ready)}</b>/${num(r.total)} ready</span></span>`;
    }
    if (r.noTorrent) return `<span class="ls ls--missing">${icon('search', { size: 13 })}Needs a torrent</span>`;
    if (r.dl != null) return `<span class="ls ls--dl">${icon('arrowDown', { size: 13 })}Downloading ${r.dl}%</span>`;
    return `<span class="ls ls--ready">${icon('play', { size: 12, fill: true })}Ready</span>`;
  }
  function watchLine(r) {
    if (r.tv) return r.finished ? 'Watched all' : r.watched ? `${num(r.watched)} of ${num(r.total)} watched` : 'Not started';
    return r.pct === 100 ? 'Watched' : r.pct ? `${r.pct}% watched` : r.t.runtime || '';
  }
  const watchPct = (r) => (r.tv ? Math.round((r.watched / r.total) * 100) : r.pct || 0);

  function card(r) {
    const t = r.t, href = `#/title/${t.type}/${t.id}`;
    const wp = watchPct(r);
    const badge = posterBadge(r);
    return `<article class="${cls('libcard', r.review && 'has-review')}" data-id="${t.id}">
      <a href="${href}" class="libcard-media" aria-label="${esc(t.title)}">
        <div class="media media--poster">${MS.art.poster(t.art)}
          ${badge ? `<span class="badge-tl">${badge}</span>` : ''}
          ${wp > 0 ? `<div class="libcard-watch" role="progressbar" aria-label="Watched ${wp}%" aria-valuenow="${wp}" aria-valuemin="0" aria-valuemax="100"><span style="width:${wp}%"></span></div>` : ''}
        </div>
      </a>
      <button class="libcard-more" data-act="lib-more" data-id="${t.id}" aria-label="${esc(t.title)}: more actions" aria-haspopup="menu">${icon('more', { size: 18 })}</button>
      <div class="libcard-meta">
        <a href="${href}" class="title">${esc(t.title)}</a>
        <p class="status">${readyLine(r)}</p>
        <p class="sub">${esc(watchLine(r))}</p>
      </div>
    </article>`;
  }

  function listRow(r) {
    const t = r.t, href = `#/title/${t.type}/${t.id}`;
    const wp = watchPct(r);
    const pills = [r.review ? ui.pill('review', `${r.review} to review`) : '', r.streaming ? ui.pill('streaming', 'Streaming') : '', r.tv && r.dl != null && !r.streaming ? ui.pill('downloading', `${r.dl}%`, { pct: r.dl }) : ''].filter(Boolean).join('');
    return `<li class="lrow" data-id="${t.id}">
      <a href="${href}" class="lrow-thumb" aria-hidden="true" tabindex="-1">${ui.poster(t.id)}</a>
      <div class="lrow-title">
        <a href="${href}" class="name">${esc(t.title)}</a>
        <span class="sub">${r.tv ? 'Show' : 'Movie'} · ${t.year}${r.size ? `<span class="xs-inline"> · ${fmtSize(r.size)}</span>` : ''}</span>
      </div>
      <div class="lrow-status">${readyLine(r)}${pills ? `<span class="lrow-pills">${pills}</span>` : ''}</div>
      <div class="lrow-watch"><span class="t-small c-2">${esc(watchLine(r))}</span>${wp > 0 ? ui.progress(wp, 'watched') : ''}</div>
      <span class="lrow-size t-num">${fmtSize(r.size)}</span>
      <span class="lrow-last">${r.watchedAt ? esc(r.watchedAt) : '<span class="c-3">Never</span>'}</span>
      <button class="btn btn-ghost btn-icon lrow-more" data-act="lib-more" data-id="${t.id}" aria-label="${esc(t.title)}: more actions" aria-haspopup="menu">${icon('moreV', { size: 18 })}</button>
    </li>`;
  }

  function resultsHTML() {
    const all = visible();
    const list = apply(all);
    if (!list.length) {
      if (S.q) {
        return `<div class="empty lib-empty">${ui.cat('sit', 96)}<h3>No titles match “${esc(S.q)}”</h3><p>Nothing in your library by that name. It might be out there, though.</p>
          <div class="actions"><button class="btn btn-secondary" data-act="lib-clear">Clear search</button><a class="btn btn-ghost" href="#/search?q=${encodeURIComponent(S.q)}">${icon('search', { size: 17 })}Search everywhere</a></div></div>`;
      }
      if (S.filter === 'review') {
        return `<div class="empty lib-empty lib-calm"><div class="lib-calm-art">${ui.cat('sit', 96)}<span class="lib-calm-check">${icon('check', { size: 16, stroke: 2.6 })}</span></div><h3>Nothing needs review</h3><p>Every file in your library has its episode. Noir checks new torrents as they arrive.</p>
          <div class="actions"><button class="btn btn-secondary" data-act="lib-filter" data-v="all">Show all titles</button></div></div>`;
      }
      const label = (FILTERS.find((f) => f[0] === S.filter) || FILTERS[0])[1];
      return `<div class="empty lib-empty">${ui.cat('sit', 96)}<h3>Nothing under ${esc(label)}</h3><p>Try another filter.</p><div class="actions"><button class="btn btn-secondary" data-act="lib-filter" data-v="all">Show all titles</button></div></div>`;
    }
    if (S.view === 'list') {
      return `<div class="lrows-head desk-only" aria-hidden="true"><span></span><span>Title</span><span>Status</span><span>Watched</span><span>Size</span><span>Last watched</span><span></span></div>
        <ul class="lrows">${list.map(listRow).join('')}</ul>`;
    }
    return `<div class="poster-grid lib-grid">${list.map(card).join('')}</div>`;
  }

  function chipsHTML() {
    const all = visible();
    return FILTERS.map(([v, label, fn]) => {
      const n = all.filter(fn).length;
      return ui.fchip(v === 'review' && n ? `${icon('alertCircle', { size: 14 })}${label}` : label, S.filter === v, n, `data-act="lib-filter" data-v="${v}"${v === 'review' && n ? ' data-review="1"' : ''}`);
    }).join('');
  }

  function totalsLine() {
    const all = visible();
    const size = all.reduce((s, r) => s + r.size, 0);
    const readyN = all.filter((r) => r.complete).length;
    return `${all.length} titles · ${fmtSize(size)} · ${readyN} ready in Infuse`;
  }

  function countLine() { const n = apply(visible()).length; return `${n} title${n === 1 ? '' : 's'}`; }

  function toolbarHTML() {
    return `<div class="lib-tools">
      <div class="input-wrap lib-search">${icon('search', { size: 18 })}<input class="input" type="search" data-lib-q value="${esc(S.q)}" placeholder="Search your library" aria-label="Search your library" autocomplete="off" enterkeyhint="search">
        ${S.q ? `<span class="input-end"><button class="btn btn-ghost btn-sm btn-icon" data-act="lib-clear" aria-label="Clear search">${icon('x', { size: 16 })}</button></span>` : ''}</div>
      <div class="fchips fchips--scroll lib-chips" role="group" aria-label="Filter" data-lib-chips>${chipsHTML()}</div>
      <p class="lib-count t-small c-3" data-lib-count>${countLine()}</p>
      <div class="lib-viewbar">
        <label class="lib-sort"><span class="sr-only">Sort by</span>${icon('arrowUpDown', { size: 16 })}<select class="input" data-lib-sort>${SORTS.map(([v, l]) => `<option value="${v}" ${S.sort === v ? 'selected' : ''}>${l}</option>`).join('')}</select></label>
        ${ui.seg([['grid', `${icon('grid', { size: 16 })}<span class="sr-only">Grid</span>`], ['list', `${icon('rows', { size: 16 })}<span class="sr-only">List</span>`]], S.view, 'data-lib-view aria-label="Layout"')}
      </div>
    </div>`;
  }

  function headHTML() {
    return `${ui.largeTitle('Library')}
      <p class="page phone-only t-small c-3 lib-totals" data-lib-totals>${totalsLine()}</p>
      <header class="lib-dhead page desk-only"><div class="stack gap-1"><h1 class="t-display">Library</h1><p class="c-3 t-small" data-lib-totals>${totalsLine()}</p></div></header>`;
  }

  function pageHTML(state) {
    const switcher = ui.protoStates('#/library', [['default', 'Grid'], ['list', 'List'], ['empty', 'Empty (first time)'], ['filtered-empty', 'Nothing needs review'], ['loading', 'Loading']], state);
    if (state === 'empty') {
      return `${switcher}${headHTML().replace(totalsLine(), 'Nothing here yet')}
        <section class="page lib-first">${ui.empty({ cat: 'sleep', title: 'Your library is empty', body: 'Add a movie or show and Noir gets it ready for Infuse. Everything you add lives here, with what\'s ready and what you\'ve watched.',
          actions: `<a class="btn btn-primary" href="#/discover">${icon('compass', { size: 18 })}Discover</a><a class="btn btn-secondary" href="#/search">${icon('search', { size: 18 })}Search titles</a>` })}</section>`;
    }
    if (state === 'loading') {
      return `${switcher}${headHTML().replace(totalsLine(), 'Loading…')}
        <div class="page lib-body" aria-busy="true">
          <div class="lib-tools lib-tools--skel"><div class="skel" style="height:44px;border-radius:var(--r-sm)"></div><div class="row gap-2">${Array.from({ length: 5 }, (_, i) => `<div class="skel" style="height:36px;width:${[56, 76, 70, 88, 72][i]}px;border-radius:var(--r-pill);flex-shrink:0"></div>`).join('')}</div></div>
          <div class="poster-grid lib-grid">${ui.skelPosters(12)}</div>
        </div>`;
    }
    return `${switcher}${headHTML()}
      <div class="page lib-body">
        ${toolbarHTML()}
        <div class="lib-results" data-lib-results aria-live="polite">${resultsHTML()}</div>
      </div>`;
  }

  // ---- Wiring ---------------------------------------------------------------------
  function syncURL() {
    const q = [];
    if (S.state && S.state !== 'default' && S.state !== 'list') q.push(`state=${S.state}`);
    if (S.filter !== 'all') q.push(`filter=${S.filter}`);
    if (S.sort !== 'added') q.push(`sort=${S.sort}`);
    if (S.view !== 'grid') q.push(`view=${S.view}`);
    if (S.q) q.push(`q=${encodeURIComponent(S.q)}`);
    try { history.replaceState(null, '', `#/library${q.length ? `?${q.join('&')}` : ''}`); } catch (e) { /* sandboxed */ }
  }
  function paint(opts = {}) {
    const main = document.getElementById('main'); if (!main) return;
    const set = (sel, html) => main.querySelectorAll(sel).forEach((el) => { el.innerHTML = html; });
    set('[data-lib-results]', resultsHTML());
    set('[data-lib-chips]', chipsHTML());
    set('[data-lib-totals]', totalsLine());
    set('[data-lib-count]', countLine());
    main.querySelectorAll('[data-lib-view] button').forEach((b) => b.setAttribute('aria-selected', String(b.dataset.v === S.view)));
    if (opts.search) {
      const wrap = main.querySelector('.lib-search');
      const end = wrap.querySelector('.input-end');
      if (S.q && !end) wrap.insertAdjacentHTML('beforeend', `<span class="input-end"><button class="btn btn-ghost btn-sm btn-icon" data-act="lib-clear" aria-label="Clear search">${icon('x', { size: 16 })}</button></span>`);
      if (!S.q && end) end.remove();
    }
    revealChip();
    syncURL();
  }
  // Keep the active filter chip in view on the phone's scrollable chip row
  function revealChip() {
    const row = document.querySelector('[data-lib-chips]'); if (!row || row.scrollWidth <= row.clientWidth) return;
    const on = row.querySelector('[aria-pressed="true"]'); if (!on) return;
    const l = on.offsetLeft - row.offsetLeft, r = l + on.offsetWidth;
    if (l < row.scrollLeft || r > row.scrollLeft + row.clientWidth) row.scrollLeft = Math.max(0, r - row.clientWidth + 24);
  }

  app.route('/library', (p) => {
    const state = p.state || (p.view === 'list' ? 'list' : 'default');
    S = { state, filter: p.filter || (state === 'filtered-empty' ? 'review' : 'all'), sort: p.sort || 'added', view: p.view || (state === 'list' ? 'list' : 'grid'), q: p.q || '', forceNoReview: state === 'filtered-empty' };
    return {
      title: 'Library', tab: 'library',
      html: `<div class="lib-page">${pageHTML(state)}</div>`,
      mount(main) {
        requestAnimationFrame(revealChip);
        main.addEventListener('input', (e) => { if (e.target.matches('[data-lib-q]')) { S.q = e.target.value; paint({ search: true }); } });
        main.addEventListener('change', (e) => { if (e.target.matches('[data-lib-sort]')) { S.sort = e.target.value; paint(); } });
        main.addEventListener('click', (e) => {
          const b = e.target.closest('[data-lib-view] button'); if (!b) return;
          S.view = b.dataset.v; S.state = S.view === 'list' ? 'list' : 'default'; paint();
        });
      },
    };
  });

  // ---- Overflow menu: bottom sheet on phone, popover on desktop -------------------------
  function menuItems(id) {
    const r = visible().find((x) => x.id === id); if (!r) return '';
    const t = r.t;
    const path = `https://momoshtrem.duckdns.org:36911/${r.tv ? 'Shows' : 'Movies'}/${encodeURIComponent(t.short || t.title)}/`;
    const items = [
      `<a role="menuitem" href="#/title/${t.type}/${t.id}">${icon('info', { size: 18 })}Open details</a>`,
      r.review ? `<a role="menuitem" href="#/mapping${t.id === 'doctorwho' ? '?state=doctorwho' : ''}">${icon('listChecks', { size: 18 })}Review mapping<span class="hint">${r.review} file${r.review === 1 ? '' : 's'}</span></a>` : '',
      r.tv && !r.complete ? `<button role="menuitem" data-act="find" data-show="${t.id}" data-se="">${icon('search', { size: 18 })}Find missing episodes<span class="hint">${num(r.total - r.ready)}</span></button>` : '',
      !r.tv && r.noTorrent ? `<button role="menuitem" data-act="find" data-show="${t.id}" data-se="">${icon('search', { size: 18 })}Find a torrent</button>` : '',
      r.complete || r.ready ? `<button role="menuitem" data-act="copy" data-copy="${esc(path)}">${icon('copy', { size: 18 })}Copy WebDAV path</button>` : '',
      r.dl != null ? `<a role="menuitem" href="#/activity">${icon('activity', { size: 18 })}See download</a>` : '',
      '<div class="sep"></div>',
      `<button role="menuitem" class="lib-remove" data-act="lib-remove" data-id="${t.id}">${icon('trash', { size: 16 })}Remove from library…</button>`,
    ];
    return items.join('');
  }

  let pop = null;
  function closePop() {
    if (!pop) return;
    pop.el.remove(); document.removeEventListener('click', pop.outside, true); document.removeEventListener('keydown', pop.key, true); window.removeEventListener('scroll', closePop);
    const f = pop.from; pop = null; if (f && f.isConnected) f.focus({ preventScroll: true });
  }
  function openPop(btn, id) {
    closePop();
    const r = btn.getBoundingClientRect();
    const el = document.createElement('div');
    el.className = 'menu lib-pop'; el.setAttribute('role', 'menu');
    el.innerHTML = menuItems(id);
    document.body.appendChild(el);
    const h = el.offsetHeight, w = 260;
    const top = r.bottom + 6 + h > window.innerHeight - 12 ? r.top - h - 6 : r.bottom + 6;
    el.style.cssText = `position:fixed;z-index:var(--z-palette);width:${w}px;top:${Math.max(12, top)}px;left:${Math.max(12, Math.min(r.right - w, window.innerWidth - w - 12))}px`;
    const outside = (e) => { if (!el.contains(e.target)) closePop(); else if (e.target.closest('[role="menuitem"]')) setTimeout(closePop, 0); };
    const key = (e) => { if (e.key === 'Escape') { e.stopPropagation(); closePop(); } };
    pop = { el, outside, key, from: btn };
    setTimeout(() => { document.addEventListener('click', outside, true); document.addEventListener('keydown', key, true); window.addEventListener('scroll', closePop, { passive: true }); }, 0);
    const first = el.querySelector('[role="menuitem"]'); if (first) first.focus({ preventScroll: true });
  }

  app.sheet('lib-item', (p) => {
    const r = visible().find((x) => x.id === p.id) || visible()[0];
    const t = r.t;
    return {
      cls: 'sheet--narrow',
      html: `<div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head"><div class="sheet-title"><div class="row gap-3" style="min-width:0"><div style="width:40px;flex-shrink:0">${ui.poster(t.id)}</div><div class="stack" style="min-width:0"><h2 class="t-title-3 clamp-1" id="lib-item-title">${esc(t.title)}</h2><span class="t-small c-3">${r.tv ? `${num(r.ready)}/${num(r.total)} ready` : r.noTorrent ? 'Needs a torrent' : 'Movie'} · ${fmtSize(r.size)}</span></div></div><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
        <div class="sheet-body"><div class="menu lib-sheet-menu">${menuItems(t.id)}</div></div>`,
    };
  });

  Object.assign(app.actions, {
    'lib-filter': (b) => { S.filter = b.dataset.v; if (S.state === 'filtered-empty' && S.filter !== 'review') { S.forceNoReview = false; S.state = 'default'; } paint(); },
    'lib-clear': () => { S.q = ''; const i = document.querySelector('[data-lib-q]'); if (i) { i.value = ''; i.focus(); } paint({ search: true }); },
    'lib-more': (b) => { if (isDesk()) openPop(b, b.dataset.id); else app.openSheet('lib-item', { id: b.dataset.id }); },
    'lib-remove': (b) => {
      const id = b.dataset.id; const r = visible().find((x) => x.id === id); if (!r) return;
      closePop(); if (b.closest('.sheet')) app.close();
      S.pending = id;
      const what = r.tv ? `the show and its ${num(r.total)} episode mappings` : 'the movie and its mapping';
      app.openDialog('remove', {
        title: `Remove ${esc(r.t.title)} from your library?`,
        body: `This removes ${what}. Infuse stops showing it. Your Trakt history isn't touched.`,
        confirm: 'Remove', danger: true, act: 'lib-remove-confirm',
        extra: r.size ? `<label class="row gap-3" style="min-height:44px"><input type="checkbox" checked style="width:20px;height:20px;accent-color:var(--accent)"> <span class="t-small c-2">Also delete downloaded data (${fmtSize(r.size)})</span></label>` : '',
      });
    },
    'lib-remove-confirm': () => {
      const id = S.pending; app.close(); if (!id) return;
      removed.add(id); paint();
      app.toast(null, { kind: 'info', icon: 'trash', title: `Removed ${esc(T(id).title)}`, body: 'Undo within 10 seconds', action: 'Undo', act: 'lib-undo', ms: 10000 });
      S.undo = id;
    },
    'lib-undo': (b) => { if (S.undo) { removed.delete(S.undo); S.undo = null; paint(); } const t = b.closest('.toast'); if (t) t.remove(); },
  });
})();
