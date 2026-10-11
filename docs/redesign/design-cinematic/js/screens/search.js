/* Search (phone tab + desktop results page) and the ⌘K command palette.
   States: idle (recent + trending) · results (?q=dune) · none (no results, suggestions)
   MS.palette(q) → markup, MS.paletteMount(layer, app) → wiring (core.js openPalette calls these). */
(function () {
  const { ui, app, art, brand, icon, data: D } = MS;
  const esc = ui.esc;
  const T = (id) => D.T[id];

  // ---- extra TMDB-only titles so a search like "dune" looks real -------------
  // (local fixtures; added to MS.data.T with lib:'none' so cards and detail links work)
  const EXTRA = {
    dune2021: { id: 'dune2021', type: 'movie', title: 'Dune', year: 2021, rating: 7.8, genres: ['Sci-Fi', 'Adventure'], runtime: '2h 35m', director: 'Denis Villeneuve', cast: ['Timothée Chalamet', 'Rebecca Ferguson', 'Oscar Isaac', 'Zendaya'], overview: 'Paul Atreides, a brilliant and gifted young man born into a great destiny beyond his understanding, must travel to the most dangerous planet in the universe to ensure the future of his family and his people.', lib: 'none' },
    dune1984: { id: 'dune1984', type: 'movie', title: 'Dune', year: 1984, rating: 6.3, genres: ['Sci-Fi', 'Adventure'], runtime: '2h 17m', director: 'David Lynch', cast: ['Kyle MacLachlan', 'Francesca Annis', 'Sting'], overview: 'In the year 10,191 the most precious substance in the universe is the spice Melange, and the desert planet Arrakis is the only place it is found.', lib: 'none' },
    jodorowsky: { id: 'jodorowsky', type: 'movie', title: 'Jodorowsky’s Dune', year: 2013, rating: 7.9, genres: ['Documentary'], runtime: '1h 30m', director: 'Frank Pavich', cast: ['Alejandro Jodorowsky', 'Michel Seydoux', 'H.R. Giger'], overview: 'The story of cult film-maker Alejandro Jodorowsky’s ambitious but ultimately doomed attempt to adapt Dune in the 1970s.', lib: 'none' },
    duneprophecy: { id: 'duneprophecy', type: 'tv', title: 'Dune: Prophecy', year: 2024, years: '2024–', rating: 7.6, genres: ['Sci-Fi', 'Drama'], runtime: '~60 min', network: 'HBO', status: 'Returning', cast: ['Emily Watson', 'Olivia Williams', 'Travis Fimmel'], overview: 'Ten thousand years before the ascension of Paul Atreides, two Harkonnen sisters combat forces that threaten the future of humankind and establish the fabled sect that will become the Bene Gesserit.', seasons: [{ n: 1, eps: 6 }], lib: 'none' },
    frankherbert: { id: 'frankherbert', type: 'tv', title: 'Frank Herbert’s Dune', year: 2000, years: '2000', rating: 6.9, genres: ['Sci-Fi', 'Drama'], runtime: '~90 min', network: 'Sci Fi', status: 'Ended', cast: ['Alec Newman', 'William Hurt', 'Saskia Reeves'], overview: 'A three-part miniseries adaptation of Frank Herbert’s novel.', seasons: [{ n: 1, eps: 3 }], lib: 'none' },
  };
  const ART = {
    dune2021: { bg: ['#c9a27a', '#7a5a3e', '#1d140e'], fg: '#f6ead8', acc: '#e8d2b0', motif: 'dunes', type: 'wide', title: 'DUNE', ambient: '#8a6a48' },
    dune1984: { bg: ['#3a5aa8', '#1d2c5e', '#0a0f22'], fg: '#f2d27a', acc: '#f2d27a', motif: 'sun', type: 'serif', title: 'DUNE', sub: '1984', ambient: '#34508f' },
    jodorowsky: { bg: ['#f04a6a', '#7a1f6a', '#1a0a2a'], fg: '#ffe9c8', acc: '#ffd34a', motif: 'vortex', type: 'cond', title: 'JODOROWSKY’S DUNE', ambient: '#b02a62' },
    duneprophecy: { bg: ['#2a2a2e', '#141416', '#050506'], fg: '#e9dcc4', acc: '#c9a25a', motif: 'ring', type: 'wide', title: 'DUNE', sub: 'PROPHECY', ambient: '#7a6440' },
    frankherbert: { bg: ['#e9c46a', '#b5651d', '#3a1a0a'], fg: '#2a160a', acc: '#fff1c8', motif: 'dunes', type: 'serif', title: 'Frank Herbert’s Dune', ambient: '#c07a30' },
  };
  Object.entries(EXTRA).forEach(([k, t]) => { if (!D.T[k]) D.T[k] = { ...t, art: k, short: t.title }; });
  Object.entries(ART).forEach(([k, a]) => { if (!art.palette[k]) art.palette[k] = a; });

  const ALIASES = { frieren: ['sousou no frieren', 'frieren beyond journeys end'], onepiece: ['wan pisu'], shogun: ['shogun'], dandadan: ['dandadan'], totoro: ['tonari no totoro'], pluribus: ['plur1bus'] };

  // ---- matching ------------------------------------------------------------
  // Fold per character so indices stay aligned with the original (for <mark>).
  const foldC = (c) => { const f = c.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase(); return f.length === 1 ? f : c.toLowerCase().slice(0, 1) || ' '; };
  const fold = (s) => [...String(s)].map(foldC).join('');
  const norm = (s) => fold(s).replace(/[’'’:.,!?\-]/g, ' ').replace(/\s+/g, ' ').trim();

  function mark(text, q) {
    const qq = fold(q).trim();
    if (!qq) return esc(text);
    const chars = [...String(text)];
    const hay = chars.map(foldC).join('');
    const i = hay.indexOf(qq);
    if (i < 0) return esc(text);
    return `${esc(chars.slice(0, i).join(''))}<mark>${esc(chars.slice(i, i + qq.length).join(''))}</mark>${esc(chars.slice(i + qq.length).join(''))}`;
  }

  function search(q) {
    const nq = norm(q);
    if (!nq) return [];
    const words = nq.split(' ');
    const out = [];
    Object.values(D.T).forEach((t) => {
      const title = norm(t.title);
      let score = 0, via = '';
      if (title === nq) score = 100;
      else if (title.startsWith(nq)) score = 80;
      else if (words.every((w) => title.includes(w))) score = 60;
      else if ((ALIASES[t.id] || []).some((a) => a.includes(nq))) { score = 55; via = 'Original title match'; }
      else {
        const people = [t.director, t.creator, ...(t.cast || [])].filter(Boolean);
        const hit = people.find((p) => words.every((w) => norm(p).split(' ').some((x) => x.startsWith(w))));
        if (hit) { score = 40; via = `${hit === t.director || hit === t.creator ? (t.type === 'movie' ? 'Directed by' : 'Created by') : 'With'} ${hit}`; }
      }
      if (score) out.push({ t, score: score + t.rating / 10 + (t.lib !== 'none' ? 2 : 0), via });
    });
    return out.sort((a, b) => b.score - a.score);
  }

  // Did you mean (edit distance on titles)
  function lev(a, b) {
    const m = a.length, n = b.length, d = Array.from({ length: m + 1 }, (_, i) => [i]);
    for (let j = 1; j <= n; j++) d[0][j] = j;
    for (let i = 1; i <= m; i++) for (let j = 1; j <= n; j++) d[i][j] = Math.min(d[i - 1][j] + 1, d[i][j - 1] + 1, d[i - 1][j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1));
    return d[m][n];
  }
  function suggest(q) {
    const nq = norm(q);
    let best = null;
    Object.values(D.T).forEach((t) => {
      const cand = [norm(t.title), ...norm(t.title).split(' ').filter((w) => w.length > 3)];
      cand.forEach((c) => { const d = lev(nq, c); if (d <= Math.max(2, Math.floor(nq.length / 4)) && (!best || d < best.d)) best = { t, d }; });
    });
    return best && best.t;
  }

  // ---- status line for library items ---------------------------------------
  function libInfo(t) {
    if (t.id === 'frieren' && app.state.frieren === 'ready') return { pill: ui.pill('ready', 'Ready'), act: `<button class="btn btn-secondary btn-sm" data-act="howto">${icon('tv', { size: 15 })}Play</button>`, sub: '38 of 38 ready' };
    switch (t.lib) {
      case 'ready': return { pill: ui.pill('ready', t.type === 'movie' ? 'Ready in Infuse' : `${t.ready}/${t.total} ready`), act: `<button class="btn btn-secondary btn-sm" data-act="howto">${icon('tv', { size: 15 })}Play</button>`, sub: t.type === 'movie' ? `${t.quality || '2160p'} · ${t.size || ''}` : `${t.watched} of ${t.total} watched` };
      case 'partial': return { pill: ui.pill('ready', `${t.ready}/${t.total} ready`, { icon: 'layers' }), act: `<button class="btn btn-secondary btn-sm" data-act="find" data-show="${t.id}">${icon('search', { size: 15 })}Find missing</button>`, sub: `${t.total - t.ready} episodes missing` };
      case 'downloading': return { pill: ui.libPill(t), act: '<a class="btn btn-ghost btn-sm" href="#/activity">Activity</a>', sub: 'Downloading · 1 h 52 min left' };
      case 'needs_review': return { pill: ui.pill('review'), act: '<a class="btn btn-secondary btn-sm" href="#/mapping">Review</a>', sub: `${t.ready} of ${t.total} ready · 2 files to check` };
      case 'needs_torrent': return { pill: ui.pill('needs_torrent'), act: `<button class="btn btn-secondary btn-sm" data-act="find" data-show="${t.id}">${icon('search', { size: 15 })}Find</button>`, sub: 'In library, no torrent yet' };
      default: return { pill: '', act: '', sub: '' };
    }
  }

  function libRow(t, q) {
    const li = libInfo(t);
    const href = `#/title/${t.type}/${t.id}`;
    return `<article class="srch-row">
      <a href="${href}" aria-hidden="true" tabindex="-1">${ui.poster(t.id)}</a>
      <div class="info"><a href="${href}" class="w-semibold">${mark(t.title, q)}</a><span class="sub">${t.year} · ${t.type === 'tv' ? 'Show' : 'Movie'}${li.sub ? ` · ${esc(li.sub)}` : ''}</span>${li.pill}</div>
      <div class="end">${li.act}</div>
    </article>`;
  }

  // ---- page ----------------------------------------------------------------
  const RECENT = ['frieren', 'severance s02', 'past lives', 'wim wenders', 'one piece egghead'];
  const DEFAULT_Q = { results: 'dune', none: 'seveerance' };

  function idleBody() {
    return `<div class="srch-idle">
      <section class="section" aria-labelledby="srch-recent-h">
        <div class="section-head"><div class="titles"><h2 class="t-title-3" id="srch-recent-h">Recent searches</h2></div><div class="actions"><button class="btn btn-ghost btn-sm" data-act="srch-clear-recent">Clear</button></div></div>
        <ul class="srch-recent" id="srch-recent">${RECENT.map((r) => `<li><a href="#/search?q=${encodeURIComponent(r)}">${icon('history', { size: 18 })}<span>${esc(r)}</span></a><button class="btn btn-ghost btn-sm btn-icon" data-act="srch-forget" aria-label="Remove ${esc(r)} from recent searches">${icon('x', { size: 16 })}</button></li>`).join('')}</ul>
      </section>
      <section class="section" aria-labelledby="srch-mood-h">
        <div class="section-head"><div class="titles"><h2 class="t-title-3" id="srch-mood-h">Not sure what you want?</h2><p class="t-small c-3">Browse by mood, or ask.</p></div></div>
        <div class="fchips">${(MS.discover ? MS.discover.moods : []).map(([v, l]) => `<a class="fchip" href="#/discover?mood=${v}">${esc(l)}</a>`).join('')}<a class="fchip" href="#/ask">${brand.avatar(20)}Ask momoshtrem</a></div>
      </section>
      <section class="section full" aria-labelledby="srch-trend-h">
        <div class="section-head"><div class="titles"><h2 class="t-title-3" id="srch-trend-h">Trending this week</h2><p class="t-small c-3">From TMDB</p></div></div>
        <div class="poster-grid">${D.trending.map((id) => ui.posterCard(id)).join('')}</div>
      </section>
    </div>`;
  }

  function resultsBody(q, tab) {
    const hits = search(q);
    const lib = hits.filter((h) => h.t.lib !== 'none');
    const shows = hits.filter((h) => h.t.type === 'tv');
    const movies = hits.filter((h) => h.t.type === 'movie');
    const sel = ['tv', 'movie'].includes(tab) ? tab : 'all';
    const inTab = (h) => sel === 'all' || h.t.type === sel;
    const libIn = lib.filter(inTab);
    const rest = hits.filter((h) => h.t.lib === 'none' && inTab(h));
    const tabs = ui.tabs([['all', 'All', hits.length], ['tv', 'Shows', shows.length], ['movie', 'Movies', movies.length]], sel, 'aria-label="Result type" data-srch-tabs');
    const card = (h) => ui.posterCard(h.t.id, { sub: h.via || undefined }).replace(`>${esc(h.t.title)}</a>`, `>${mark(h.t.title, q)}</a>`);
    return `<div class="srch-tabs">${tabs}</div>
      ${libIn.length ? `<section class="section" aria-labelledby="srch-lib-h"><div class="srch-group-h"><h2 class="t-title-3" id="srch-lib-h">In your library</h2><span class="n">${libIn.length}</span></div><div class="srch-lib">${libIn.map((h) => libRow(h.t, q)).join('')}</div></section>` : ''}
      ${rest.length ? `<section class="section" aria-labelledby="srch-tmdb-h"><div class="srch-group-h"><h2 class="t-title-3" id="srch-tmdb-h">${sel === 'tv' ? 'Shows' : sel === 'movie' ? 'Movies' : 'Shows and movies'}</h2><span class="n">${rest.length} on TMDB</span></div><div class="poster-grid srch-hit">${rest.map(card).join('')}</div></section>` : ''}
      ${!libIn.length && !rest.length ? `<p class="c-2">No ${sel === 'tv' ? 'shows' : 'movies'} for “${esc(q)}”. <button class="btn btn-ghost btn-sm" data-srch-tab="all">See all ${hits.length} results</button></p>` : ''}
      <a class="ask-entry srch-ask" href="#/ask?q=${encodeURIComponent(q)}">${brand.avatar(36)}<span class="grow"><strong>Not quite it?</strong><span class="t-small">Ask momoshtrem about “${esc(q)}”</span></span><span class="btn btn-secondary btn-icon btn-sm" aria-hidden="true">${icon('arrowRight', { size: 18 })}</span></a>`;
  }

  function noneBody(q) {
    const s = suggest(q);
    return `<div class="srch-none">
      ${ui.empty({ cat: 'sit', title: `Nothing for “${q}”`, body: s ? `Did you mean <a class="c-accent w-semibold" href="#/search?q=${encodeURIComponent(s.title.toLowerCase())}">${esc(s.title)}</a>?` : 'Check the spelling, or try fewer words.',
        actions: `${s ? `<a class="btn btn-primary" href="#/search?q=${encodeURIComponent(s.title.toLowerCase())}">${icon('search', { size: 18 })}Search “${esc(s.title)}”</a>` : ''}<a class="btn btn-secondary" href="#/ask?q=${encodeURIComponent(q)}">${brand.avatar(22)}Ask momoshtrem</a>` })}
      <ul class="srch-tips panel panel-pad" aria-label="Search tips">
        <li>${icon('info', { size: 16 })}<span>Original titles work too: “sousou no frieren” finds Frieren.</span></li>
        <li>${icon('user', { size: 16 })}<span>Search people: “wim wenders”, “vince gilligan”.</span></li>
        <li>${icon('compass', { size: 16 })}<span>Know the vibe, not the name? <a class="c-accent" href="#/discover">Browse Discover</a>.</span></li>
      </ul>
      <section class="section" aria-labelledby="srch-pop-h"><div class="section-head"><div class="titles"><h2 class="t-title-3" id="srch-pop-h">Popular right now</h2></div></div><div class="poster-grid">${D.trending.slice(0, 6).map((id) => ui.posterCard(id)).join('')}</div></section>
    </div>`;
  }

  const bodyFor = (q, tab) => (!q.trim() ? idleBody() : search(q).length ? resultsBody(q, tab) : noneBody(q));

  app.route('/search', (p) => {
    const state = p.state || (!p.q ? 'idle' : search(p.q).length ? 'results' : 'none');
    const q = p.q != null && p.q !== '' ? p.q : state === 'idle' ? '' : DEFAULT_Q[state] || '';
    const switcher = ui.protoStates('#/search', [['idle', 'Idle'], ['results', 'Results'], ['none', 'No results']], state);
    const html = `${switcher}
      <div class="srch page">
        <header class="srch-head">
          <div class="row between"><h1>Search</h1><button class="avatar-btn phone-only" data-act="menu" aria-label="Account menu: settings, log out">${brand.markMono(20, 'var(--text-2)')}</button></div>
          <form class="srch-form" role="search" action="#/search" id="srch-form">
            <div class="input-wrap">${icon('search', { size: 20 })}
              <input class="input" type="search" id="srch-q" name="q" value="${esc(q)}" placeholder="Shows, movies, people" aria-label="Search shows, movies and people" autocomplete="off" autocapitalize="none" autocorrect="off" spellcheck="false" enterkeyhint="search">
              <div class="input-end"><button type="button" class="btn btn-ghost btn-icon" data-act="srch-clear" aria-label="Clear search" ${q ? '' : 'hidden'}>${icon('x', { size: 18 })}</button></div>
            </div>
          </form>
          <p class="srch-tip t-small c-3 desk-only">Tip: press <span class="kbd">⌘</span><span class="kbd">K</span> anywhere to search and jump.</p>
        </header>
        <div class="srch-body" id="srch-body" aria-live="polite">${bodyFor(q, p.tab)}</div>
      </div>`;
    return {
      title: q ? `“${q}”` : 'Search', tab: 'search', html,
      mount(root) {
        const input = root.querySelector('#srch-q');
        const body = root.querySelector('#srch-body');
        const clearBtn = root.querySelector('[data-act="srch-clear"]');
        let tab = p.tab || 'all';
        const update = (push) => {
          const v = input.value;
          clearBtn.hidden = !v;
          body.innerHTML = bodyFor(v, tab);
          const hash = v.trim() ? `#/search?q=${encodeURIComponent(v.trim())}${tab !== 'all' ? `&tab=${tab}` : ''}` : '#/search';
          try { history.replaceState(null, '', hash); } catch (e) { /* sandbox */ }
          document.title = `${v ? `“${v}”` : 'Search'} · momoshtrem`;
          app.initRails(body);
        };
        let t;
        input.addEventListener('input', () => { clearTimeout(t); t = setTimeout(update, 120); });
        root.querySelector('#srch-form').addEventListener('submit', (e) => { e.preventDefault(); update(); input.blur(); });
        body.addEventListener('click', (e) => {
          const b = e.target.closest('[data-srch-tabs] [data-v], [data-srch-tab]');
          if (!b) return;
          tab = b.dataset.v || b.dataset.srchTab; update();
        });
        app.actions['srch-clear'] = () => { input.value = ''; tab = 'all'; update(); input.focus(); };
        if (!q) setTimeout(() => input.focus({ preventScroll: true }), 50);
      },
    };
  });

  Object.assign(app.actions, {
    'srch-forget': (b) => { const li = b.closest('li'); if (li) li.remove(); },
    'srch-clear-recent': () => { const ul = document.getElementById('srch-recent'); if (ul) ul.outerHTML = '<p class="t-small c-3">No recent searches.</p>'; },
  });

  // ======================================================================
  // ⌘K command palette
  // ======================================================================
  const ACTIONS = () => [
    { k: 'find-sev', label: 'Find a torrent for Severance S02E06', sub: 'Attila · upgrade or replace the file', ic: 'search', run: () => app.openSheet('torrent', { show: 'severance', se: 'S02E06' }) },
    { k: 'find-frieren', label: 'Find a torrent for Frieren S01E29–E38', sub: '10 episodes missing', ic: 'search', run: () => app.openSheet('torrent', { show: 'frieren', se: 'S01E29' }) },
    { k: 'mapping', label: 'Review Frieren mapping', sub: '2 files need a look', ic: 'listChecks', href: '#/mapping' },
    { k: 'activity', label: 'Go to Activity', sub: '3 active · Shōgun streaming now', ic: 'activity', href: '#/activity' },
    { k: 'library', label: 'Go to Library', sub: '24 titles', ic: 'library', href: '#/library' },
    { k: 'discover', label: 'Go to Discover', sub: 'Browse with filters', ic: 'compass', href: '#/discover' },
    { k: 'ask', label: 'Ask momoshtrem', sub: 'Conversational picks from your history', ic: 'ask', href: '#/ask' },
    app.state.trakt === 'off'
      ? { k: 'trakt', label: 'Connect Trakt', sub: 'Personal picks and Up next', ic: 'link', href: '#/settings?section=trakt&state=code' }
      : { k: 'trakt', label: 'Sync Trakt now', sub: 'Last synced 4 min ago', ic: 'refresh', run: () => app.toast(null, { kind: 'ready', icon: 'refresh', title: 'Syncing with Trakt', body: 'Up next updates in a moment.' }) },
    { k: 'settings', label: 'Open settings', sub: 'Integrations, AI, quality, playback', ic: 'settings', href: '#/settings' },
    { k: 'infuse', label: 'Infuse setup', sub: 'Copy the WebDAV address', ic: 'tv', run: () => app.openSheet('howto') },
  ];

  const LIB_DEFAULT = ['severance', 'frieren', 'shogun', 'dune2'];
  const t2 = (t) => `${t.type === 'tv' ? 'Show' : 'Movie'} · ${t.year}`;
  const RECENT_PAL = ['frieren', 'past lives'];

  const statusText = (t) => {
    if (t.lib === 'none') return '';
    const s = { ready: t.type === 'movie' ? 'Ready in Infuse' : 'Ready', partial: `${t.ready}/${t.total} ready`, downloading: 'Downloading', needs_review: 'Needs review', needs_torrent: 'Needs a torrent' }[t.lib];
    return s || '';
  };
  const titleItem = (t, q, via) => {
    const st = statusText(t);
    return `<a class="p-item" role="option" href="#/title/${t.type}/${t.id}">
      <div class="media media--poster thumb">${art.poster(t.art)}</div>
      <span class="grow"><span class="label">${mark(t.title, q)}</span><span>${via ? mark(via, q) : `${t.type === 'tv' ? 'Show' : 'Movie'} · ${t.year}${st ? ` · ${esc(st)}` : ''}`}</span></span>
      ${t.lib !== 'none' ? ui.libPill(t) : `<span class="t-small c-3 sm-up">Add</span>`}
      <span class="enter" aria-hidden="true">${icon('corner', { size: 16 })}</span></a>`;
  };
  const actionItem = (a, q) => `<${a.href ? `a href="${a.href}"` : `button type="button" data-act="pal-run" data-k="${a.k}"`} class="p-item" role="option">
      <span class="ic ${a.ic === 'ask' ? 'is-accent' : ''}">${a.ic === 'ask' ? brand.avatar(24) : icon(a.ic, { size: 16 })}</span>
      <span class="grow"><span class="label">${mark(a.label, q)}</span><span>${esc(a.sub)}</span></span>
      <span class="enter" aria-hidden="true">${icon('corner', { size: 16 })}</span></${a.href ? 'a' : 'button'}>`;
  const group = (label, items) => (items ? `<div class="p-group" role="presentation">${label}</div>${items}` : '');

  function palBody(q) {
    const nq = norm(q);
    if (!nq) {
      return group('Recent', RECENT_PAL.map((r) => `<a class="p-item" role="option" href="#/search?q=${encodeURIComponent(r)}"><span class="ic">${icon('history', { size: 16 })}</span><span class="grow"><span class="label">${esc(r)}</span><span>Search</span></span><span class="enter" aria-hidden="true">${icon('corner', { size: 16 })}</span></a>`).join(''))
        + group('Your library', LIB_DEFAULT.map((id) => titleItem(T(id), '')).join(''))
        + group('Actions', ACTIONS().slice(0, 9).map((a) => actionItem(a, '')).join(''));
    }
    const hits = search(q);
    const lib = hits.filter((h) => h.t.lib !== 'none').slice(0, 4);
    const titles = hits.filter((h) => h.t.lib === 'none').slice(0, 5);
    const words = nq.split(' ');
    const acts = ACTIONS().filter((a) => words.every((w) => norm(`${a.label} ${a.sub}`).includes(w)));
    const always = [
      { k: 'all', label: `Search all results for “${q}”`, sub: hits.length ? `${hits.length} title${hits.length > 1 ? 's' : ''}` : 'Movies, shows and people', ic: 'search', href: `#/search?q=${encodeURIComponent(q)}` },
      { k: 'askq', label: `Ask momoshtrem: “${q}”`, sub: 'Picks with a reason for each', ic: 'ask', href: `#/ask?q=${encodeURIComponent(q)}` },
    ];
    return group('Your library', lib.map((h) => titleItem(h.t, q, h.via ? `${t2(h.t)} · ${h.via}` : '')).join(''))
      + group('Titles', titles.map((h) => titleItem(h.t, q, h.via ? `${t2(h.t)} · ${h.via}` : '')).join(''))
      + group('Actions', [...acts, ...always].map((a) => actionItem(a, acts.includes(a) ? q : '')).join(''));
  }

  MS.palette = (q = '') => `<div class="palette ms-pal" role="dialog" aria-modal="true" aria-label="Search and quick actions">
      <div class="p-input">${icon('search', { size: 20, cls: 'c-3' })}
        <input type="text" role="combobox" aria-expanded="true" aria-controls="pal-list" aria-autocomplete="list" aria-label="Search titles, your library, or run an action" placeholder="Search or jump to…" value="${esc(q)}" autocomplete="off" autocapitalize="none" autocorrect="off" spellcheck="false" enterkeyhint="go">
        <button type="button" class="btn btn-ghost btn-sm p-cancel" data-act="close">Cancel</button>
      </div>
      <div class="p-body" id="pal-list" role="listbox" aria-label="Results">${palBody(q)}</div>
      <div class="p-foot" aria-hidden="true"><span><span class="kbd">↑</span><span class="kbd">↓</span>Navigate</span><span><span class="kbd">↵</span>Open</span><span><span class="kbd">esc</span>Close</span><span class="grow"></span><span>${brand.markMono(14, 'var(--text-3)')}<span class="kbd">⌘K</span> anywhere</span></div>
    </div>`;

  MS.paletteMount = (layer) => {
    const input = layer.querySelector('.ms-pal input');
    const list = layer.querySelector('#pal-list');
    if (!input || !list) return;
    let sel = 0;
    const items = () => [...list.querySelectorAll('.p-item')];
    const paint = (scroll) => {
      const it = items();
      if (!it.length) { input.removeAttribute('aria-activedescendant'); return; }
      sel = Math.max(0, Math.min(sel, it.length - 1));
      it.forEach((el, i) => { el.id = `pal-opt-${i}`; el.setAttribute('aria-selected', String(i === sel)); el.tabIndex = -1; });
      input.setAttribute('aria-activedescendant', `pal-opt-${sel}`);
      if (scroll) it[sel].scrollIntoView({ block: 'nearest' });
    };
    const refresh = () => { list.innerHTML = palBody(input.value) || '<p class="p-empty">No matches.</p>'; sel = 0; paint(false); list.scrollTop = 0; };
    input.addEventListener('input', refresh);
    input.addEventListener('keydown', (e) => {
      const n = items().length;
      if (e.key === 'ArrowDown') { e.preventDefault(); sel = (sel + 1) % n; paint(true); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); sel = (sel - 1 + n) % n; paint(true); }
      else if (e.key === 'Home' && e.ctrlKey) { sel = 0; paint(true); }
      else if (e.key === 'Enter') { e.preventDefault(); const el = items()[sel]; if (el) el.click(); }
    });
    list.addEventListener('mousemove', (e) => { const el = e.target.closest('.p-item'); if (!el) return; const i = items().indexOf(el); if (i !== sel) { sel = i; paint(false); } });
    paint(false);
    const end = input.value.length; try { input.setSelectionRange(end, end); } catch (e) { /* ignore */ }
  };

  app.actions['pal-run'] = (b) => {
    const a = ACTIONS().find((x) => x.k === b.dataset.k);
    app.closeAll();
    if (a && a.run) a.run();
  };
})();
