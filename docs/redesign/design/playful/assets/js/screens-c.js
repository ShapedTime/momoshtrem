/* Screens: Mapping review · Library · Activity (+ detail) · Settings · System states · prototype flows */
(function () {
  const U = UI;
  const { icon, momo, btn, iconBtn, chip, esc } = U;
  const D = DATA;
  const T = (id) => D.T[id];
  const pad = (n) => String(n).padStart(2, '0');

  // ================================================================== 9 · Episode mapping review
  const CONF = { high: ['High', 3], medium: ['Medium', 2], low: ['Low', 1] };
  const conf = (c) => `<span class="conf conf-${c}" title="${CONF[c][0]} confidence"><i></i><i></i><i></i><span>${CONF[c][0]}</span></span>`;
  const epLabel = (tg) => (tg.s === 0 ? `S00E${pad(tg.n)}` : `S${pad(tg.s)}E${pad(tg.n)}`);

  SCREENS.review = (ctx) => {
    const id = ctx.arg || 'frieren';
    const job = D.jobs[id] || D.jobs.frieren;
    const t = T(job.show);
    const j = S.job[id] || (S.job[id] = { phase: 3, resolved: {}, applied: false });
    const forced = ctx.state;
    const phase = forced === 'progress' ? 2 : forced ? 3 : j.phase;
    const applied = forced === 'applied' || j.applied;
    const g = job.groups;
    const conflictDone = forced === 'resolved' || !!j.resolved.conflict;
    const reviewLeft = g.needs_review.length && !conflictDone ? (id === 'frieren' ? 1 : g.needs_review.length) : 0;
    const applyN = g.matched.length + g.specials.length + (conflictDone ? (id === 'frieren' ? 1 : g.needs_review.length) : 0);
    const tab = ctx.p.get('g') || 'all';

    const header = `<header class="rv-head">
        ${iconBtn('arrow-left', 'Back', { act: 'back' })}
        <div class="grow"><p class="eyebrow">Mapping review · ${esc(t.short || t.title)}</p><h1 class="t-title-2">Check the episode mapping</h1>
        <p class="machine t-caption ink-2 rv-torrent">${U.machine(job.torrent)} · ${job.size} · ${job.files} files</p></div>
      </header>`;

    if (phase < 3) {
      const steps = [['Fetching metadata', `${job.files} files · ${job.size}`], ['Matching files', `${g.matched.length} matched by rules`], ['Asking AI about the tricky ones', `${g.needs_review.length + g.unmatched.length} files · Claude Haiku 5.5`]];
      return { title: 'Mapping review', nav: null, focus: true, html: `<div class="page page-narrow">${header}
        <section class="rv-progress card card-pad" aria-live="polite">
          <div class="rv-progress-art">${momo(phase >= 2 ? 'detective' : 'peeking')}</div>
          <div class="grow">${U.pawSteps(steps.map(([l, d], i) => ({ label: l, detail: i < phase ? d : i === phase ? 'Working…' : '', state: i < phase ? 'done' : i === phase ? 'now' : 'todo' })))}</div>
        </section>
        <div class="rv-skel">${Array.from({ length: 5 }, () => '<div class="map-row skel-row"><div class="skel skel-line" style="width:88%"></div><div class="skel skel-line" style="width:40%"></div></div>').join('')}</div>
      </div>` };
    }

    if (applied) {
      const n = id === 'frieren' ? 10 : applyN;
      return { title: 'Mapping applied', nav: null, html: `<div class="page page-narrow">
        <div class="rv-done is-celebrating">
          <div class="rv-done-art">${momo('proud')}</div>
          <p class="eyebrow">All mapped</p>
          <h1 class="t-title-1">${n} episodes are ready in Infuse</h1>
          <p class="ink-2">${esc(t.short || t.title)} ${id === 'frieren' ? 'S01E29–E38' : ''} now plays from <b>TV Shows → ${esc(t.title.replace(/:/g, '-'))} (${t.year})</b>. The duplicate and the extras were skipped.</p>
          <div class="row gap-2 wrap" style="justify-content:center;margin-top:18px">${btn(`Open ${esc(t.short || t.title)}`, { variant: 'primary', icon: 'tv', href: `#/show/${t.id}`, flow: 'open-show' })}${btn('Back to For You', { variant: 'secondary', href: '#/for-you', flow: 'back-home' })}</div>
          <ul class="rv-done-list">${(id === 'frieren' ? Array.from({ length: 10 }, (_, i) => [`S01E${29 + i}`, `Episode ${29 + i}`]) : g.matched.map((m) => [epLabel(m.target), m.target.title])).slice(0, 10).map(([a, b]) => `<li>${icon('check', 'ico ico-sm')}<b>${a}</b><span>${b}</span></li>`).join('')}</ul>
        </div></div>`, after: () => { if (id === 'frieren') S.frierenReady = true; } };
    }

    const row = (f, kind) => {
      const isConflict = f.conflict;
      const chosen = j.resolved.conflict;
      const keep = isConflict && (chosen ? (chosen === 'v2') === f.file.includes('v2') : null);
      const ignored = (isConflict && chosen && !keep) || j.resolved['ign:' + f.file];
      return `<li class="map-row ${isConflict ? 'is-conflict' : ''} ${ignored ? 'is-ignored' : ''} ${keep ? 'is-kept' : ''}">
        <div class="map-file">${icon(kind === 'extra' ? 'file' : 'file-video', 'ico ico-sm')}<div class="grow"><p class="machine mono t-code">${U.breakable(f.file)}</p><p class="t-caption ink-3">${f.size}${f.kind ? ` · ${f.kind}` : ''}</p></div></div>
        <div class="map-target">
          <span class="map-arrow" aria-hidden="true">${icon('arrow-right', 'ico ico-sm')}</span>
          ${f.target && !ignored ? `<button class="ep-pick" data-act="pick-ep" data-arg="${esc(f.file)}" aria-label="Change episode, currently ${epLabel(f.target)}"><b>${epLabel(f.target)}</b><span class="ellipsis">${esc(f.target.title)}</span>${icon('chevron-down', 'ico ico-sm')}</button>${conf(f.conf)}`
            : ignored ? '<span class="ink-3 t-label">Ignored</span>' : kind === 'extra' ? '<span class="ink-3 t-label">Skipped · extra</span>' : `<button class="ep-pick empty" data-act="pick-ep" data-arg="${esc(f.file)}">${icon('plus', 'ico ico-sm')}<span>Pick an episode</span></button>`}
        </div>
        ${f.reason ? `<p class="map-reason">${f.src === 'ai' ? '<span class="ai-badge">AI</span>' : ''}${esc(f.reason.replace(/^AI: /, ''))}</p>` : ''}
        ${kind === 'review' && isConflict ? `<div class="map-actions">${keep ? U.pill('ready_short', 'Keeping this one', { sm: true }) : ignored ? btn('Use this instead', { variant: 'ghost', size: 'sm', act: 'keep', arg: f.file.includes('v2') ? 'v2' : 'v1' }) : btn('Keep this one', { variant: f.file.includes('v2') ? 'primary' : 'secondary', size: 'sm', icon: 'check', act: 'keep', arg: f.file.includes('v2') ? 'v2' : 'v1', flow: f.file.includes('v2') ? 'keep-v2' : undefined })}</div>`
          : kind === 'review' || kind === 'unmatched' ? `<div class="map-actions">${btn('Accept', { variant: 'secondary', size: 'sm', icon: 'check', act: 'toast', arg: 'Accepted' })}${btn('Ignore', { variant: 'ghost', size: 'sm', act: 'ign', arg: f.file })}</div>` : ''}
      </li>`;
    };
    const group = (key, title, list, kind, note) => (list.length && (tab === 'all' || tab === key) ? `<section class="map-group ${key}" aria-labelledby="mg-${key}">
        <h2 class="map-group-h" id="mg-${key}"><span class="dot ${{ matched: 'ready', needs_review: 'review', specials: 'ready', extras: 'idle', unmatched: 'warning' }[key]}"></span>${title}<span class="count">${list.length}</span>${note ? `<span class="t-caption ink-3 grow" style="font-weight:500">${note}</span>` : ''}</h2>
        <ol class="map-list">${list.map((f) => row(f, kind)).join('')}</ol></section>` : '');

    // right pane (desktop): episodes and what lands on each
    const pane = id === 'frieren' ? Array.from({ length: 14 }, (_, i) => 25 + i).map((n) => {
      const m = g.matched.find((x) => x.target.n === n);
      const c = n === 33;
      return `<li class="${n < 29 ? 'has' : m ? 'new' : c ? (conflictDone ? 'new' : 'clash') : ''}"><b>E${n}</b><span class="ellipsis">${n < 29 ? 'Already in library' : m ? m.file.replace(/^\[SubsPlease\] Sousou no Frieren /, '').replace(/ \(1080p\).*/, '') : c ? (conflictDone ? 'S2 - 05v2' : '2 files · pick one') : '—'}</span></li>`;
    }).join('') : (g.matched.concat(g.needs_review, g.specials)).map((m) => `<li class="new"><b>${epLabel(m.target)}</b><span class="ellipsis">${esc(m.target.title)}</span></li>`).join('');

    const counts = { matched: g.matched.length, needs_review: g.needs_review.length, specials: g.specials.length, extras: g.extras.length, unmatched: g.unmatched.length };
    return {
      title: 'Mapping review', nav: null, focus: true,
      html: `<div class="page rv-page">${header}
        ${reviewLeft ? U.banner('review', 'scan-search', `${reviewLeft} thing${reviewLeft > 1 ? 's' : ''} need${reviewLeft > 1 ? '' : 's'} you`, id === 'frieren' ? 'Two files claim S01E33. Everything else matched with high confidence.' : 'Specials matched by title, not number. Check them before applying.') : U.banner('ok', 'circle-check', 'Everything is matched', `${applyN} files are ready to apply.`)}
        <div class="rv-filter">${U.tabs([['all', 'All', job.files], ['matched', 'Matched', counts.matched], ['needs_review', 'Needs review', counts.needs_review], ['specials', 'Specials', counts.specials], ['extras', 'Extras & ignored', counts.extras], ['unmatched', 'Unmatched', counts.unmatched]].filter(([k, , c]) => k === 'all' || c), tab, { act: 'rv-tab', label: 'Groups', scroll: true })}</div>
        <div class="rv-cols">
          <div class="rv-files">
            ${group('needs_review', 'Needs review', g.needs_review, 'review', id === 'frieren' ? 'Pick one' : '')}
            ${group('unmatched', 'Unmatched', g.unmatched, 'unmatched')}
            ${group('matched', 'Matched', g.matched, 'matched')}
            ${group('specials', 'Specials', g.specials, 'matched')}
            ${group('extras', 'Extras & ignored', g.extras, 'extra')}
          </div>
          <aside class="rv-pane card" aria-label="Episodes"><h2 class="t-headline">${id === 'frieren' ? 'Season 1 · E25–E38' : 'Episodes'}</h2><p class="t-caption ink-3">What each episode will play</p><ol class="pane-list">${pane}</ol>
            <p class="pane-legend"><span><i class="lg has"></i>In library</span><span><i class="lg new"></i>From this torrent</span><span><i class="lg clash"></i>Conflict</span></p></aside>
        </div>
      </div>
      <div class="rv-foot"><div class="rv-foot-inner">
        ${btn('Re-check with AI', { variant: 'secondary', icon: 'sparkles', act: 'recheck' })}
        <span class="rv-foot-note t-caption ink-2">${reviewLeft ? 'The E33 duplicate stays unassigned until you pick.' : 'Nothing waits on you.'}</span>
        <span class="spacer"></span>
        ${btn(`Apply ${applyN} mappings`, { variant: 'primary', icon: 'check', act: 'apply', arg: id, flow: conflictDone ? 'apply' : undefined })}
      </div></div>`,
    };
  };
  Object.assign(ACT, {
    keep: (v) => { const id = parseRoute().parts[1] || 'frieren'; S.job[id].resolved.conflict = v; render(); toast({ text: v === 'v2' ? 'Keeping S2 - 05v2' : 'Keeping the original', sub: 'The other file is ignored.', icon: 'check' }); },
    ign: (f) => { const id = parseRoute().parts[1] || 'frieren'; S.job[id].resolved['ign:' + f] = true; render(); },
    'rv-tab': (v) => setParams({ g: v === 'all' ? null : v }, true),
    apply: (id) => { S.job[id].applied = true; if (id === 'frieren') S.frierenReady = true; render(); toast({ text: 'Applied. Ready in Infuse.', sub: 'Infuse sees the new files on its next refresh.', momo: 'proud' }); },
    'pick-ep': (file) => setParams({ sheet: 'picker', pf: file }),
    recheck: () => setParams({ sheet: 'recheck' }),
    'recheck-go': () => { const id = parseRoute().parts[1] || 'frieren'; S.job[id].phase = 2; setParams({ sheet: null }); window.runJob(id); },
  });
  SHEETS.picker = (ctx) => {
    const f = ctx.p.get('pf') || '';
    const t = T('frieren');
    const eps = t.seasons;
    return U.sheet({ id: 'pk', title: 'Pick an episode', sub: `<span class="machine mono">${U.breakable(f)}</span>`, cls: 'full picker', body: `
      <div class="sheet-sticky"><div class="input-wrap search-field">${icon('search')}<input class="input" placeholder="Number or title, e.g. 33 or “recap”" aria-label="Search episodes" id="pick-q"></div></div>
      ${eps.map((s) => `<section><h3 class="picker-h">${s.n === 0 ? 'Specials' : `Season ${s.n}`}<span class="ink-3">${s.eps.length}</span></h3><ol>${s.eps.filter((e) => s.n === 0 || e.n > 24).map((e) => `<li><button class="pick-row ${e.n === 33 && s.n === 1 ? 'on' : ''}" data-act="picked" data-arg="${s.n}:${e.n}"><span class="ep-num">${s.n === 0 ? `SP${pad(e.n)}` : `E${pad(e.n)}`}</span><span class="grow ellipsis">${esc(e.title)}</span>${e.state === 'watched' || (S.frierenReady && e.n > 28) ? U.pill('ready_short', 'Has file', { sm: true }) : ''}${e.n === 33 && s.n === 1 ? icon('check') : ''}</button></li>`).join('')}</ol>${s.n === 1 ? '<p class="t-caption ink-3 picker-more">Episodes 1–24 hidden: they already have files. <button class="link-btn" data-act="noop">Show</button></p>' : ''}</section>`).join('')}`,
      foot: `${btn('Ignore this file', { variant: 'ghost', act: 'close-sheet' })}<span class="spacer"></span>${btn('Done', { variant: 'primary', act: 'close-sheet' })}` });
  };
  ACT.picked = (arg) => { setParams({ sheet: null }); toast({ text: `Mapped to ${arg.startsWith('0') ? 'SP' : 'S01E'}${pad(arg.split(':')[1])}`, icon: 'check' }); };
  SHEETS.recheck = () => U.sheet({ id: 'rc', title: 'Re-check with AI', sub: 'Claude Haiku 5.5 looks at the unresolved files again. About $0.01.', body: `<div class="stack gap-4">
      <fieldset class="fgroup"><legend>Which files</legend>${U.seg([['unresolved', 'Unresolved (3)'], ['all', 'All 14']], 'unresolved', { block: true, label: 'Scope' })}</fieldset>
      <div class="field"><label class="field-label" for="hint">Hint for Momo <span class="ink-3">(optional)</span></label><textarea class="input" id="hint" rows="3" placeholder="e.g. these are absolute numbers"></textarea>
        <div class="chips">${['these are absolute numbers', 'season 2 = TMDB season 1', 'these are the OVAs', 'ignore recaps'].map((x) => chip(x, { act: 'hint', arg: x, cls: 'outline' })).join('')}</div></div></div>`,
    foot: `${btn('Cancel', { variant: 'ghost', act: 'close-sheet' })}<span class="spacer"></span>${btn('Re-check', { variant: 'primary', icon: 'sparkles', act: 'recheck-go' })}` });
  ACT.hint = (x) => { const h = document.getElementById('hint'); h.value = x; h.focus(); };

  // ================================================================== 10 · Library
  const libStatus = (id) => {
    const l = D.libState[id];
    if (!l) return '';
    if (l.kind === 'movie') {
      return { ready: U.pill('ready_short', 'Ready', { sm: true }), downloading: U.pill('downloading', `${l.pct}%`, { sm: true, pct: l.pct }), missing: U.pill('needs_torrent', 'Needs a torrent', { sm: true }) }[l.status] || '';
    }
    const ready = id === 'frieren' && S.frierenReady ? 38 : l.ready;
    const pills = l.status === 'review' ? U.pill('review', `${l.review} to review`, { sm: true }) : l.status === 'downloading' ? U.pill('downloading', `${l.dl} eps · ${l.pct}%`, { sm: true, pct: l.pct }) : '';
    return `<span class="lib-count ${ready === l.total ? 'full' : ''}">${icon(ready === l.total ? 'circle-check' : 'tv-minimal', 'ico ico-xs')}<span><b>${ready.toLocaleString('en')}</b>/${l.total.toLocaleString('en')} ready</span></span>${pills}`;
  };
  SCREENS.library = (ctx) => {
    const p = ctx.p;
    const f = p.get('f') || 'all';
    const view = p.get('view') || S.lib.view;
    const q = (p.get('lq') || '').toLowerCase();
    const empty = ctx.state === 'empty';
    const cf = p.get('confirm');
    if (cf && S._cf !== cf) { S._cf = cf; setTimeout(() => ACT['lib-remove'](cf), 0); }
    let ids = D.library.filter((id) => !S.lib.removed[id]);
    const L = D.libState;
    const fx = {
      all: () => true, movies: (id) => L[id].kind === 'movie', shows: (id) => L[id].kind === 'show',
      watching: (id) => L[id].status === 'watching' || (L[id].watched > 0 && L[id].watched < (L[id].total || 1)),
      ready: (id) => L[id].status === 'ready' || (L[id].kind === 'show' && L[id].ready === L[id].total),
      incomplete: (id) => L[id].kind === 'show' && L[id].ready < L[id].total, review: (id) => L[id].status === 'review',
    };
    const counts = Object.fromEntries(Object.entries(fx).map(([k, fn]) => [k, ids.filter(fn).length]));
    ids = ids.filter(fx[f]).filter((id) => !q || T(id).title.toLowerCase().includes(q));
    if (empty) ids = [];
    const progressOf = (id) => (L[id].kind === 'show' ? Math.round((L[id].watched / L[id].total) * 100) : L[id].watched ? 100 : undefined);
    const grid = `<div class="grid-posters lib-grid">${ids.map((id) => U.posterCard(id, { meta: `${T(id).type === 'movie' ? 'Movie' : 'Show'} · ${T(id).year}`, progress: progressOf(id), status: libStatus(id), menu: 'lib-menu' })).join('')}</div>`;
    const list = `<ul class="list lib-list">${ids.map((id) => {
      const t = T(id);
      const pr = progressOf(id);
      return `<li class="list-row lib-row"><a href="#/${t.type}/${id}" class="lib-row-link">${U.poster(id, { cls: 'thumb' })}<span class="grow"><b>${esc(t.title)}</b><span class="t-caption ink-3">${t.type === 'movie' ? 'Movie' : 'Show'} · ${t.year}</span></span></a>
        <span class="lib-row-status">${libStatus(id)}</span>${pr !== undefined ? `<span class="lib-row-prog">${U.progress(pr, { cls: 'thin watched', label: 'Watched' })}<span class="t-caption ink-3">${pr}% watched</span></span>` : '<span class="lib-row-prog"></span>'}
        ${iconBtn('ellipsis', `Options for ${t.title}`, { cls: 'sm', act: 'lib-menu', arg: id })}</li>`;
    }).join('')}</ul>`;
    return {
      title: 'Library', nav: 'library',
      html: `<div class="page">
        ${phead('Library', { sub: empty ? '' : `${D.library.length} titles · 9 shows, 5 movies · 2.1 TB on disk` })}
        ${empty ? U.emptyState('sleeping', 'Your library is empty', 'Momo is napping on the TV until you add something. Add a title, then find a torrent for it.', `${btn('Discover something', { variant: 'primary', icon: 'compass', href: '#/discover' })}${btn('Ask momoshtrem', { variant: 'secondary', icon: 'sparkles', href: '#/ask' })}`) : `
        <div class="lib-bar">
          <div class="input-wrap search-field lib-search">${icon('search')}<label class="sr-only" for="lq">Search your library</label><input class="input" id="lq" type="search" placeholder="Search your library" value="${esc(p.get('lq') || '')}"></div>
          <label class="sr-only" for="lsort">Sort</label><select class="input sort-select" id="lsort"><option>Recently added</option><option>Recently watched</option><option>Title A–Z</option><option>Release year</option></select>
          ${U.seg([['grid', icon('layout-grid', 'ico ico-sm') + '<span class="sr-only">Grid</span>'], ['list', icon('list', 'ico ico-sm') + '<span class="sr-only">List</span>']], view, { act: 'lib-view', label: 'View' })}
        </div>
        ${U.tabs([['all', 'All', counts.all], ['movies', 'Movies', counts.movies], ['shows', 'Shows', counts.shows], ['watching', 'Watching', counts.watching], ['ready', 'Ready', counts.ready], ['incomplete', 'Incomplete', counts.incomplete], ['review', 'Needs review', counts.review]], f, { act: 'lib-f', label: 'Filter library', scroll: true })}
        <div class="section lib-results">${ids.length ? (view === 'list' ? list : grid) : U.emptyState('detective', 'Nothing here', 'No titles match this filter.', btn('Show all', { variant: 'secondary', size: 'sm', act: 'lib-f', arg: 'all' }), { compact: true })}</div>`}
      </div>`,
      after: () => { const i = document.getElementById('lq'); if (i) i.addEventListener('input', () => { clearTimeout(i._t); i._t = setTimeout(() => setParams({ lq: i.value || null }, true), 200); }); },
    };
  };
  Object.assign(ACT, {
    'lib-view': (v) => { S.lib.view = v; setParams({ view: v }, true); },
    'lib-f': (v) => setParams({ f: v === 'all' ? null : v }, true),
    'lib-menu': (id) => setParams({ sheet: 'libmenu', lid: id }),
    'lib-remove': (id) => {
      setParams({ sheet: null, lid: null });
      const t = T(id);
      S.dialog = () => U.dialog(`Remove ${esc(t.title)}?`, `It disappears from your library and from Infuse. Watch history on Trakt stays. ${t.type === 'show' ? 'Torrents only used by this show are removed too.' : ''}`, `${btn('Cancel', { act: 'close-dialog' })}${btn('Remove', { variant: 'danger-solid', act: 'lib-remove-ok', arg: id })}`, { extra: '<label class="check" style="margin-top:10px"><input type="checkbox"> Also delete downloaded data</label>' });
      render();
    },
    'lib-remove-ok': (id) => { S.lib.removed[id] = true; S.dialog = null; render(); toast({ text: `${T(id).title} removed`, action: 'Undo', act: 'lib-undo' }); window._lastRm = id; },
    'lib-undo': () => { delete S.lib.removed[window._lastRm]; render(); },
  });
  SHEETS.libmenu = (ctx) => {
    const id = ctx.p.get('lid');
    const t = T(id);
    return U.sheet({ id: 'lm', title: esc(t.title), sub: `${t.type === 'movie' ? 'Movie' : 'Show'} · ${t.year}`, cls: 'menu-sheet', body: `<div class="stack gap-1">
      <a class="menu-item" href="#/${t.type}/${id}">${icon('arrow-up-right')}Open</a>
      <button class="menu-item" data-act="howto" data-arg="${id}">${icon('tv')}How to play</button>
      <button class="menu-item" data-act="tsearch" data-arg="${id}${t.type === 'show' ? ':pack' : ''}">${icon('search')}Find a torrent</button>
      <div class="menu-sep"></div>
      <button class="menu-item danger" data-act="lib-remove" data-arg="${id}">${icon('trash-2')}Remove from library…</button></div>` });
  };

  // ================================================================== 11 · Activity
  SCREENS.activity = (ctx) => {
    if (ctx.arg) return activityDetail(ctx);
    const st = ctx.state;
    if (st === 'offline') {
      return { title: 'Activity', nav: 'activity', html: `<div class="page">${phead('Activity')}
        <div class="offline card card-pad" role="alert">
          <div class="offline-art">${momo('knocking')}</div>
          <div class="grow"><h2 class="t-title-2">Can’t reach momoshtrem</h2>
            <p class="ink-2">The backend at <code class="mono">momoshtrem:4444</code> isn’t answering. Infuse can’t stream new files until it’s back; downloads resume on their own.</p>
            <ul class="offline-facts"><li>${icon('clock', 'ico ico-sm')}Last contact 2 min ago</li><li>${icon('server', 'ico ico-sm')}Check the <code class="mono">momoshtrem</code> container: <code class="mono">docker compose ps</code></li></ul>
            <div class="row gap-2 wrap" style="margin-top:16px">${btn('Try again', { variant: 'primary', icon: 'refresh-cw', act: 'toast', arg: 'Still offline. Retrying every 10 s.' })}${btn('System status', { variant: 'secondary', href: '#/settings?section=system' })}</div></div>
        </div></div>` };
    }
    const list = st === 'empty' ? [] : D.torrents.concat(S.frierenReady ? [{ hash: 'f21b8a', name: '[SubsPlease] Sousou no Frieren S2 (01-10) (1080p) [Batch]', title: 'frieren', what: 'S01E29–E38 · 10 episodes', state: 'downloading', pct: 12, down: '18.9 MB/s', up: '0.8 MB/s', peers: '210 peers · 188 seeds', eta: '11 min', size: '14.2 GB' }] : []);
    const sec = (title, items, note) => (items.length ? `<section class="section"><div class="shelf-head"><div><h2 class="shelf-title">${title}</h2>${note ? `<p class="shelf-sub">${note}</p>` : ''}</div></div><div class="tor-list">${items.map(U.torrentCard).join('')}</div></section>` : '');
    const bars = [4, 6, 5, 9, 12, 10, 14, 18, 16, 22, 26, 24, 30, 28, 34, 36, 33, 36];
    return {
      title: 'Activity', nav: 'activity',
      html: `<div class="page">
        ${phead('Activity', { sub: 'What momoshtrem is doing right now' })}
        ${list.length ? `<section class="act-sum card" aria-label="Totals">
          <div class="act-stat"><span class="t-micro ink-3">Download</span><b>${icon('arrow-down', 'ico ico-sm')}36.0 MB/s</b></div>
          <div class="act-stat"><span class="t-micro ink-3">Upload</span><b>${icon('arrow-up', 'ico ico-sm')}3.3 MB/s</b></div>
          <div class="act-stat"><span class="t-micro ink-3">Now</span><b>1 streaming · ${2 + (S.frierenReady ? 1 : 0)} downloading</b></div>
          <div class="act-spark" aria-hidden="true">${bars.map((b) => `<i style="height:${b * 2.4}%"></i>`).join('')}</div>
        </section>
        ${sec('Streaming now', list.filter((t) => t.state === 'streaming'), 'Pieces near the playhead download first')}
        ${sec('Downloading', list.filter((t) => t.state === 'downloading'))}
        ${sec('Waiting for you', list.filter((t) => t.state === 'review'))}
        ${sec('Idle', list.filter((t) => t.state === 'idle' || t.state === 'paused'), 'Complete. Loaded on demand when Infuse opens a file.')}`
        : U.emptyState('sleeping', 'All quiet', 'Nothing is downloading or streaming. Momo is napping on the TV.', btn('Find something', { variant: 'secondary', href: '#/for-you' }))}
      </div>`,
    };
  };
  function activityDetail(ctx) {
    const t = D.torrents.find((x) => x.hash === ctx.arg) || D.torrents[0];
    const title = T(t.title);
    const eps = title.seasons ? (title.seasons.find((s) => s.n === 2) || title.seasons[title.seasons.length - 1]).eps : [];
    let pieces = '';
    const R = ((s) => () => ((s = (s * 16807) % 2147483647) / 2147483647))(7);
    for (let i = 0; i < 360; i++) { const done = i < 300 || R() > 0.6; const prio = i >= 300 && i < 316; pieces += `<i class="${prio ? 'prio' : done ? 'have' : R() > 0.8 ? 'dl' : ''}"></i>`; }
    return {
      title: title.title, nav: 'activity',
      html: `<div class="page page-narrow">
        <div class="backbar">${iconBtn('arrow-left', 'Back to Activity', { href: '#/activity' })}<span class="t-label ink-2">Activity</span></div>
        <header class="ad-head">${U.poster(t.title, { cls: 'thumb-lg' })}<div class="grow"><p class="eyebrow">${esc(t.what)}</p><h1 class="t-title-2"><a href="#/${title.type}/${t.title}" class="plain">${esc(title.title)}</a></h1><p class="machine t-caption ink-2">${U.machine(t.name)}</p></div></header>
        <section class="card card-pad ad-stats">
          <div class="row gap-3"><div class="grow">${U.progress(t.pct, { cls: t.state === 'streaming' ? 'ready' : 'striped' })}</div><b>${t.pct}%</b></div>
          <dl class="stat-grid"><div><dt>Download</dt><dd>${t.down || '—'}</dd></div><div><dt>Upload</dt><dd>${t.up || '—'}</dd></div><div><dt>Peers</dt><dd>${t.peers || '—'}</dd></div><div><dt>Left</dt><dd>${t.eta || 'Done'}</dd></div><div><dt>Size</dt><dd>${t.size}</dd></div><div><dt>Ratio</dt><dd>0.31</dd></div></dl>
          <div class="row gap-2 wrap">${t.state === 'paused' ? btn('Resume', { variant: 'secondary', icon: 'play', act: 'toast', arg: 'resumed' }) : btn('Pause', { variant: 'secondary', icon: 'pause', act: 'toast', arg: 'paused' })}${btn('Copy magnet', { variant: 'ghost', icon: 'magnet', act: 'toast', arg: 'magnet' })}<span class="spacer"></span>${btn('Remove…', { variant: 'danger', icon: 'trash-2', act: 'confirm-remove-torrent' })}</div>
        </section>
        ${t.reader ? `<div class="section">${U.banner('ok', 'cast', 'Streaming now', esc(t.reader) + ' · 38% in')}</div>` : ''}
        <section class="section"><h2 class="block-title">Files <span class="ink-3">· ${eps.length || 1}</span></h2>
          <ul class="list files">${(eps.length ? eps : [{ n: 1, title: title.title }]).map((e, i) => `<li class="list-row"><span class="ep-num">${eps.length ? `E${pad(e.n)}` : 'Film'}</span><div class="grow"><p class="machine mono t-code">${U.breakable(eps.length ? `${t.name}/${title.title.replace(/[: ]+/g, '.')}.S02E${pad(e.n)}.${e.title.replace(/[^A-Za-z0-9]+/g, '.')}.2160p.mkv` : `${t.name}.mkv`)}</p><p class="t-caption ink-3">→ ${eps.length ? `S02E${pad(e.n)} · ${esc(e.title)}` : 'Movie'} · ${['4.3', '3.9', '4.1', '4.6', '4.1', '3.8', '4.4', '3.6', '4.2', '6.1'][i % 10]} GB</p></div>${i === 5 ? U.pill('streaming', 'Playing', { sm: true }) : i < 8 ? U.pill('ready_short', '100%', { sm: true }) : U.pill('downloading', `${i === 8 ? 64 : 12}%`, { sm: true, pct: i === 8 ? 64 : 12 })}</li>`).join('')}</ul></section>
        <details class="section advanced card" ${ctx.p.get('adv') ? 'open' : ''}>
          <summary>${icon('cpu', 'ico ico-sm')}<b>Advanced</b><span class="t-caption ink-3">Piece map, readers, peers</span>${icon('chevron-down', 'ico ico-sm chev')}</summary>
          <div class="adv-body">
            <h3 class="t-label">Piece map <span class="ink-3">· 2,764 pieces · 16 MiB</span></h3>
            <div class="piece-map" role="img" aria-label="Piece map: 84% have, 16 high-priority pieces downloading near the playhead">${pieces}</div>
            <p class="pane-legend"><span><i class="lg pm-have"></i>Have</span><span><i class="lg pm-dl"></i>Downloading</span><span><i class="lg pm-prio"></i>Playhead priority</span><span><i class="lg pm-none"></i>Missing</span></p>
            <h3 class="t-label" style="margin-top:18px">Active readers</h3>
            <table class="adv-table"><thead><tr><th>Reader</th><th>File</th><th>Position</th><th>Read-ahead</th></tr></thead><tbody>
              <tr><td>Infuse · Apple TV</td><td>S02E06</td><td>1.62 / 4.26 GB</td><td>64 MiB · pieces 1840–1904</td></tr>
              <tr><td>Infuse · scan</td><td>S02E09</td><td>0.01 / 4.18 GB</td><td>8 MiB</td></tr></tbody></table>
            <p class="t-caption ink-3 mono" style="margin-top:12px">Info hash b1c9e4d07a3f…91c2 · added Oct 9 · 41 peers (33 seeds) · DHT, PEX on</p>
          </div>
        </details>
      </div>`,
    };
  }

  // ================================================================== 12 · Settings
  const SECTIONS = [['integrations', 'Integrations', 'link'], ['ai', 'AI', 'sparkles'], ['quality', 'Quality profile', 'sliders-horizontal'], ['playback', 'Playback', 'tv'], ['system', 'System status', 'server'], ['app', 'App', 'settings']];
  const traktCard = () => {
    const st = S.trakt;
    let body;
    if (st === 'off') body = `<p class="ink-2">Read-only: history, ratings, watchlist and show progress. momoshtrem never writes to Trakt; Infuse already scrobbles.</p>${btn('Connect Trakt', { variant: 'primary', icon: 'link', act: 'trakt', arg: 'code', flow: 'trakt-connect' })}`;
    if (st === 'code') body = `<div class="device-code">
        <p class="t-label">1 · Go to <a href="https://trakt.tv/activate" target="_blank" rel="noreferrer">trakt.tv/activate</a> on any device</p>
        <p class="t-label">2 · Enter this code</p>
        <div class="code-row"><output class="code mono" aria-label="Device code 4F7K 29QX">4F7K-29QX</output>${iconBtn('copy', 'Copy code', { act: 'toast', arg: 'code' })}</div>
        <p class="t-caption ink-3">${icon('timer', 'ico ico-xs')} Code expires in 9:42. momoshtrem checks every 5 seconds.</p>
        <div class="row gap-2 wrap">${btn('Open trakt.tv/activate', { variant: 'primary', icon: 'external-link', act: 'trakt', arg: 'waiting', flow: 'trakt-open' })}${btn('Cancel', { variant: 'ghost', act: 'trakt', arg: 'off' })}</div></div>`;
    if (st === 'waiting') body = `<div class="trakt-wait">${momo('peeking')}<div><p class="t-headline">Waiting for Trakt…</p><p class="ink-2 t-callout">Approve momoshtrem on trakt.tv. This page updates on its own.</p>${U.pawTrack(5, 3)}</div></div>`;
    if (st === 'syncing') body = `<p class="t-headline">Connected as @${DATA.user.trakt}. First sync…</p>${U.pawSteps([{ label: 'History', detail: '1,284 plays', state: 'done' }, { label: 'Ratings', detail: '312 ratings', state: 'done' }, { label: 'Watchlist', detail: 'Fetching…', state: 'now' }, { label: 'Show progress', detail: '', state: 'todo' }])}`;
    if (st === 'connected') body = `<div class="trakt-user"><span class="trakt-av">${ART.face('couchcat')}</span><div class="grow"><b>@${DATA.user.trakt}</b><p class="t-caption ink-3">Last sync ${S.justSynced ? 'just now' : DATA.user.lastSync} · 1,284 plays · 312 ratings · 46 on watchlist</p></div></div>
      ${S.justSynced ? U.banner('ok', 'party-popper', 'All synced', 'Momo is building your first shelves now.', btn('See your picks', { variant: 'primary', size: 'sm', href: '#/for-you', flow: 'see-picks' })) : ''}
      <div class="row gap-2 wrap">${btn('Sync now', { variant: 'secondary', size: 'sm', icon: 'refresh-cw', act: 'toast', arg: 'Syncing with Trakt' })}${btn('Disconnect…', { variant: 'danger', size: 'sm', act: 'trakt-disconnect' })}</div>`;
    return `<section class="card card-pad set-card" id="integrations" aria-labelledby="trakt-h"><div class="row gap-3"><span class="brand-chip trakt">${icon('tv-minimal-play')}</span><div class="grow"><h3 class="t-headline" id="trakt-h">Trakt</h3><p class="t-caption ink-3">Watch history and progress</p></div>${st === 'connected' ? U.pill('ready_short', 'Connected', { sm: true }) : st === 'off' ? U.pill('neutral', 'Not connected', { sm: true }) : U.pill('downloading', 'Connecting', { sm: true, pct: 60 })}</div>${body}</section>`;
  };
  const row = (label, sub, control) => `<div class="set-row"><div class="grow"><b>${label}</b>${sub ? `<small>${sub}</small>` : ''}</div>${control}</div>`;
  const sw = (on, label) => `<input type="checkbox" class="switch" ${on ? 'checked' : ''} aria-label="${label}">`;
  SCREENS.settings = (ctx) => {
    const tp = ctx.p.get('trakt');
    if (tp === 'start' && S.trakt !== 'connected' && !S._autoStart) { S._autoStart = true; S.trakt = 'off'; }
    if (tp && tp !== 'start' && S._tp !== tp) { S._tp = tp; S.trakt = tp; if (tp === 'connected') S.justSynced = !!ctx.p.get('synced'); }
    const sec = ctx.p.get('section');
    return {
      title: 'Settings', nav: null,
      html: `<div class="page settings">
        ${phead('Settings')}
        <div class="set-layout">
          <nav class="set-nav" aria-label="Settings sections">${SECTIONS.map(([id, label, ic]) => `<a href="#/settings?section=${id}" class="set-link ${sec === id ? 'on' : ''}" data-act="set-jump" data-arg="${id}">${icon(ic, 'ico ico-sm')}${label}</a>`).join('')}</nav>
          <div class="set-main">
            <h2 class="set-h">Integrations</h2>
            ${traktCard()}
            <h2 class="set-h" id="ai">AI</h2>
            <section class="card card-pad set-card">
              ${row('Nightly picks', 'Five shelves every night, ready by breakfast.', sw(true, 'Nightly picks'))}
              ${row('Schedule', null, '<label class="sr-only" for="sched">Schedule</label><select class="input set-select" id="sched"><option>Every night at 04:00</option><option>Weekends only</option><option>Manual</option></select>')}
              <div class="set-row col"><div><b>Model</b><small>Used for shelves and Ask. Web search needs Opus or Sonnet.</small></div>${U.seg([['opus', 'Opus 5.5'], ['sonnet', 'Sonnet 5.5']], 'opus', { block: true, label: 'Model' })}<small class="t-caption ink-3">Opus about $0.60 a night · Sonnet about $0.30</small></div>
              <div class="set-row col"><div class="row gap-2"><b class="grow">Spend this month</b><span class="t-label">$7.84 <span class="ink-3">of $25</span></span></div>
                <div class="budget"><i class="b-recs" style="width:23.6%"></i><i class="b-ask" style="width:6.1%"></i><i class="b-cls" style="width:1.7%"></i></div>
                <p class="budget-legend"><span><i class="b-recs"></i>Shelves $5.90</span><span><i class="b-ask"></i>Ask $1.52</span><span><i class="b-cls"></i>Classifier $0.42</span></p></div>
              ${row('Monthly budget', 'Momo skips runs when it’s used up, and says so.', '<div class="input-wrap money"><span class="cur">$</span><label class="sr-only" for="bud">Monthly budget in dollars</label><input class="input" id="bud" inputmode="decimal" value="25"></div>')}
              ${row('AI file classifier', 'Claude Haiku 5.5 resolves tricky file names. ~$0.01 per torrent.', sw(true, 'AI file classifier'))}
              ${row('Auto-apply confident mappings', 'Only when every file is high confidence and nothing conflicts.', sw(true, 'Auto-apply confident mappings'))}
            </section>
            <h2 class="set-h" id="quality">Quality profile</h2>
            <section class="card card-pad set-card">
              <div class="set-row col"><b>Preferred resolution</b>${U.seg(['720p', '1080p', '2160p'], '2160p', { block: true, label: 'Preferred resolution' })}</div>
              <div class="set-row col"><div class="row"><b class="grow">Max size per hour</b><span class="t-label">12 GB/h</span></div><input class="range" type="range" min="1" max="40" value="12" aria-label="Max size per hour"><small class="ink-3 t-caption">A 2-hour film up to 24 GB · a 50-minute episode up to 10 GB</small></div>
              ${row('Prefer x265 (HEVC)', null, sw(true, 'Prefer x265'))}${row('Prefer HDR', null, sw(true, 'Prefer HDR'))}${row('Prefer Dolby Vision', 'Your Apple TV 4K supports it.', sw(true, 'Prefer Dolby Vision'))}
              <div class="set-row col"><b>Avoid these terms</b><div class="chips">${['CAM', 'TS', 'HDCAM', 'HC', 'KORSUB', '3D'].map((x) => chip(x, { x: true, cls: 'outline' })).join('')}${chip('Add', { icon: 'plus', cls: 'dashed' })}</div></div>
              <div class="set-row col"><b>Preferred groups</b><div class="chips">${['FLUX', 'NTb', 'SubsPlease', 'VARYG'].map((x) => chip(x, { x: true, cls: 'outline' })).join('')}${chip('Add', { icon: 'plus', cls: 'dashed' })}</div></div>
            </section>
            <h2 class="set-h" id="playback">Playback</h2>
            <section class="card card-pad set-card">
              <div class="set-row col"><b>WebDAV address</b><div class="copy-field"><code class="mono">${DATA.webdav}</code>${btn('Copy', { variant: 'secondary', size: 'sm', icon: 'copy', act: 'toast', arg: 'path' })}</div><small class="ink-3 t-caption">User <code class="mono">webdav</code> · password from <code class="mono">WEBDAV_PASSWORD</code> in your .env</small></div>
              <div class="infuse-guide"><h3 class="t-headline">Set up Infuse (once)</h3>
                <ol class="howto-steps">
                  <li><span class="check-num">1</span><div><b>Infuse → Settings → Shares → Add Share → WebDAV</b><span>On the Apple TV.</span></div></li>
                  <li><span class="check-num">2</span><div><b>Address <code class="mono">momoshtrem.duckdns.org</code>, port <code class="mono">36911</code>, HTTPS on</b><span>Then the user and password above.</span></div></li>
                  <li><span class="check-num">3</span><div><b>Add “Movies” and “TV Shows” as library folders</b><span>Infuse fetches artwork and keeps your place.</span></div></li>
                </ol></div>
            </section>
            <h2 class="set-h" id="system">System status</h2>
            <section class="card set-card status-list">
              ${[['Backend', 'v2.0.0 · up 6 days', 'ready'], ['Database', 'Postgres 16 · 41 ms', 'ready'], ['Jackett', '3 of 4 indexers OK · IPTorrents timing out', 'warning'], ['TMDB', 'OK · cache 92% hit', 'ready'], ['Trakt', S.trakt === 'connected' ? `@${DATA.user.trakt} · synced ${DATA.user.lastSync}` : 'Not connected', S.trakt === 'connected' ? 'ready' : 'idle'], ['AI', 'Anthropic API OK · $7.84 this month', 'ready'], ['WebDAV', 'Serving · 1 reader', 'ready']].map(([n, d, s]) => `<div class="list-row"><span class="dot ${s} ${s === 'warning' ? 'pulse' : ''}"></span><div class="grow"><b>${n}</b><p class="t-caption ink-3">${d}</p></div>${s === 'warning' ? btn('Details', { variant: 'ghost', size: 'sm' }) : ''}</div>`).join('')}
            </section>
            <h2 class="set-h" id="app">App</h2>
            <section class="card card-pad set-card">
              <div class="set-row col"><b>Theme</b>${U.seg([['system', 'System'], ['light', 'Light'], ['dark', 'Dark']], document.documentElement.dataset.theme, { block: true, act: 'set-theme', label: 'Theme' })}</div>
              <div class="set-row col"><b>Motion</b>${U.seg([['system', 'System'], ['full', 'Full'], ['reduce', 'Reduced']], document.documentElement.dataset.motion || 'system', { block: true, act: 'set-motion', label: 'Motion' })}</div>
              ${row('Log out', 'You’ll need your password manager to sign back in.', btn('Log out', { variant: 'secondary', size: 'sm', icon: 'log-out', href: '#/signin' }))}
            </section>
          </div>
        </div>
      </div>`,
      after: () => { if (sec) { const el = document.getElementById(sec); if (el) el.scrollIntoView({ block: 'start' }); } },
    };
  };
  Object.assign(ACT, {
    trakt: (st) => {
      S.trakt = st; render();
      if (st === 'waiting') setTimeout(() => { S.trakt = 'syncing'; render(); setTimeout(() => { S.trakt = 'connected'; S.justSynced = true; render(); }, 2600); }, 2000);
    },
    'trakt-disconnect': () => { S.dialog = () => U.dialog('Disconnect Trakt?', 'momoshtrem forgets your imported history, ratings and watchlist. Up next and personal picks stop until you reconnect.', `${btn('Cancel', { act: 'close-dialog' })}${btn('Disconnect', { variant: 'danger-solid', act: 'trakt-off' })}`); render(); },
    'trakt-off': () => { S.dialog = null; S.trakt = 'off'; render(); },
    'set-theme': (v) => { document.documentElement.dataset.theme = v === 'system' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : v; render(); },
    'set-motion': (v) => { if (v === 'system') delete document.documentElement.dataset.motion; else document.documentElement.dataset.motion = v; render(); },
    'set-jump': (id, el, ev) => { ev.preventDefault(); history.replaceState(null, '', `#/settings?section=${id}`); document.getElementById(id)?.scrollIntoView({ block: 'start', behavior: 'smooth' }); },
  });

  // ================================================================== 13 · System states & feedback
  SCREENS.states = () => ({
    title: 'System states', nav: null,
    html: `<div class="page states-page">
      ${phead('System states', { sub: 'Loading, empty, error, feedback and progress, as they appear in the app.' })}
      <h2 class="set-h">Skeleton loading</h2>
      <div class="card card-pad"><div class="skel-home">
        <div class="skel skel-line" style="width:40%;height:22px"></div>
        <div class="skel-up">${Array.from({ length: 3 }, () => '<div class="stack gap-2"><div class="skel skel-wide"></div><div class="skel skel-line" style="width:60%"></div><div class="skel skel-line" style="width:35%"></div></div>').join('')}</div>
        <div class="skel-posters">${Array.from({ length: 6 }, () => '<div class="stack gap-2"><div class="skel skel-poster"></div><div class="skel skel-line" style="width:80%"></div></div>').join('')}</div>
        <p class="t-caption ink-3">Skeletons match the real layout box for box, so nothing jumps when data lands. Shimmer stops with reduced motion.</p></div></div>
      <h2 class="set-h">Empty states</h2>
      <div class="state-grid">
        <div class="card">${U.emptyState('sleeping', 'Your library is empty', 'Momo is napping on the TV until you add something.', btn('Discover', { variant: 'primary', size: 'sm' }), { compact: true })}</div>
        <div class="card">${U.emptyState('detective', 'No matches for “freiren”', 'Did you mean <a href="#/search?q=Frieren">Frieren</a>?', btn('Ask momoshtrem', { variant: 'secondary', size: 'sm' }), { compact: true })}</div>
        <div class="card">${U.emptyState('peeking', 'Trakt isn’t connected', 'Connect it and For You becomes yours.', btn('Connect Trakt', { variant: 'primary', size: 'sm' }), { compact: true })}</div>
        <div class="card">${U.emptyState('yawn', 'Nothing new tonight', 'You’re caught up on everything. Momo found 3 films for a quiet evening.', btn('See them', { variant: 'secondary', size: 'sm' }), { compact: true })}</div>
      </div>
      <h2 class="set-h">Errors</h2>
      <div class="stack gap-3">
        ${U.banner('err', 'server', 'Backend offline', 'momoshtrem isn’t answering. Streaming new files is paused; downloads resume on their own.', btn('Retry', { variant: 'secondary', size: 'sm' }))}
        ${U.banner('warn', 'triangle-alert', '2 indexers timed out', 'IPTorrents and 1337x didn’t answer in 8 s. Showing results from the other 3.', btn('Retry', { variant: 'secondary', size: 'sm' }))}
        ${U.banner('err', 'circle-alert', 'Couldn’t add this torrent', 'The .torrent link returned 404, so there’s no magnet to add. Pick another release.', btn('Back to results', { variant: 'secondary', size: 'sm' }))}
        <div class="card">${U.emptyState('knocking', 'Something broke on this page', 'Not your fault. Reloading usually fixes it; your library is safe.', btn('Reload', { variant: 'primary', size: 'sm', icon: 'rotate-cw' }), { compact: true, role: 'alert' })}</div>
      </div>
      <h2 class="set-h">Toasts</h2>
      <div class="stack gap-2 toast-demo">
        <div class="toast static ok">${momo('proud')}<div class="toast-text">Added. Momo’s on it.<small>Dune: Part Two is downloading.</small></div><button class="btn btn-ghost btn-sm">View</button></div>
        <div class="toast static ok"><span class="toast-ico">${icon('check')}</span><div class="toast-text">Frieren removed</div><button class="btn btn-ghost btn-sm">Undo</button></div>
        <div class="toast static err"><span class="toast-ico">${icon('circle-alert')}</span><div class="toast-text">Couldn’t reach Trakt<small>We’ll retry in 30 s.</small></div><button class="btn btn-ghost btn-sm">Retry</button></div>
      </div>
      <h2 class="set-h">Confirmation dialog</h2>
      <div class="dialog static" role="alertdialog" aria-labelledby="dd"><h2 id="dd">Remove Past Lives?</h2><p>It disappears from your library and from Infuse. Watch history on Trakt stays.</p><label class="check" style="margin-top:10px"><input type="checkbox"> Also delete downloaded data</label><div class="dialog-actions">${btn('Cancel')}${btn('Remove', { variant: 'danger-solid' })}</div></div>
      <h2 class="set-h">Progress</h2>
      <div class="card card-pad stack gap-5">
        <div class="stack gap-2"><span class="t-label">Download</span>${U.progress(42, { cls: 'striped' })}<span class="t-label">Ready</span>${U.progress(100, { cls: 'ready' })}<span class="t-label">Watched</span>${U.progress(68, { cls: 'thin watched' })}<span class="t-label">Unknown length</span>${U.progress(30, { cls: 'indeterminate' })}</div>
        <div class="row gap-4 wrap">${U.ring(42)}${U.ring(63, 64)}${U.pill('downloading', null, { pct: 42 })}${U.pawTrack(5, 3)}</div>
        ${U.pawSteps([{ label: 'Fetching metadata', detail: '14 files · 14.2 GB', state: 'done' }, { label: 'Matching files', detail: '11 matched by rules', state: 'done' }, { label: 'Asking AI about the tricky ones', detail: 'Working…', state: 'now' }, { label: 'Ready to review', state: 'todo' }])}
      </div>
    </div>`,
  });

  // ================================================================== prototype flows
  const resetFrieren = () => { const t = T('frieren'); t.seasons[1].eps.forEach((e) => { if (e.n > 28) e.state = 'missing'; }); S.frierenReady = false; delete S.job.frieren; S.ts = { ctx: 'frieren-s01e29', query: 'Frieren S01E29', scope: 'episode', filters: {}, sort: 'best', shown: 7, arg: 'frieren:1:29' }; };
  const route = (c) => c.r.parts[0] || 'for-you';
  FLOWS['1'] = {
    name: 'Flow 1 · Get it ready (phone)', start: '#/for-you', reset: resetFrieren,
    steps: [
      { when: (c) => route(c) === 'for-you', target: '[data-flow="find-torrent"]', text: 'Frieren’s next episode <b>needs a torrent</b>. Tap <b>Find torrent</b>.' },
      { when: (c) => c.p.get('sheet') === 'torrents', target: '[data-flow="inspect"]', text: 'Momo’s best match covers E29 and the 9 missing after it. <b>Inspect</b> it first.' },
      { when: (c) => c.p.get('nested') === 'inspect', target: '[data-flow="add-inspect"]', text: 'The “S2” files line up with TMDB S01E29–E38. Tap <b>Add &amp; assign</b>.' },
      { when: (c) => route(c) === 'review' && (S.job.frieren?.phase ?? 3) < 3, text: 'Momo fetches metadata, matches files, then asks AI about the tricky ones.' },
      { when: (c) => route(c) === 'review' && S.job.frieren?.phase === 3 && !S.job.frieren?.resolved.conflict, target: '[data-flow="keep-v2"]', text: 'Two files claim E33. <b>Keep</b> the fixed v2 release.' },
      { when: (c) => route(c) === 'review' && S.job.frieren?.resolved.conflict && !S.job.frieren?.applied, target: '[data-flow="apply"]', text: 'Everything else is high confidence. <b>Apply 10 mappings</b>.' },
      { when: (c) => S.job.frieren?.applied, target: '[data-flow="back-home"]', text: '<b>Ready.</b> Back on For You, Frieren now says “Ready to watch”.' },
    ],
  };
  FLOWS['2'] = {
    name: 'Flow 2 · Ask (desktop)', start: '#/ask?new=1', reset: (s) => { s.ask = { phase: 'idle', step: 0 }; delete s.added['the-good-place']; },
    steps: [
      { when: (c) => route(c) === 'ask', target: '[data-flow="ask-example"]', text: 'Ask momoshtrem for <b>“something like Severance but lighter”</b>.' },
      { when: (c) => route(c) === 'ask' && S.ask.phase === 'working', text: 'Momo reads your history, searches the web and checks TMDB. Steps stream in as they happen.' },
      { when: (c) => route(c) === 'ask' && S.ask.phase === 'results', target: '[data-flow="rec-add"]', text: 'Pick <b>The Good Place</b> and add it to your library.' },
      { when: () => S.added['the-good-place'], target: '[data-flow="rec-find"]', text: 'It’s in. Now <b>Find a torrent</b>.' },
      { when: (c) => c.p.get('sheet') === 'torrents', target: '[data-flow="best-gen"]', text: 'A complete season pack is the best match. From here it’s Flow 1.' },
    ],
  };
  FLOWS['3'] = {
    name: 'Flow 3 · Connect Trakt', start: '#/settings', startTrakt: 'off', reset: (s) => { s.trakt = 'off'; s.justSynced = false; },
    steps: [
      { when: (c) => route(c) === 'settings' && S.trakt === 'off', target: '[data-flow="trakt-connect"]', text: 'Tap <b>Connect Trakt</b>. It’s read-only.' },
      { when: () => S.trakt === 'code', target: '[data-flow="trakt-open"]', text: 'Enter the code at trakt.tv/activate. Here, tap <b>Open trakt.tv/activate</b>.' },
      { when: () => S.trakt === 'waiting', text: 'momoshtrem polls Trakt until you approve…' },
      { when: () => S.trakt === 'syncing', text: 'First sync: history, ratings, watchlist, progress.' },
      { when: (c) => S.trakt === 'connected' && route(c) === 'settings', target: '[data-flow="see-picks"]', text: 'All synced. <b>See your picks</b>.' },
      { when: (c) => S.trakt === 'connected' && route(c) === 'for-you', text: 'For You fills in: Up next with availability, then personal shelves.' },
    ],
  };
})();
