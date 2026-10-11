/* Discover: browse with real filters. States: default · filtered · empty · loading
   Filter state lives in the URL (#/discover?type=tv&genres=comedy,mystery&rating=8),
   so a view can be bookmarked and survives reloads. Desktop: left filter column,
   applied live. Phone: chip row + "Filters" bottom sheet with a sticky
   "Show N results" footer (changes are staged until you tap it). */
(function () {
  const { ui, app, brand, icon, data: D } = MS;
  const esc = ui.esc;

  // ---- vocabulary ----------------------------------------------------------
  const GENRES = ['Action', 'Adventure', 'Animation', 'Comedy', 'Documentary', 'Drama', 'Family', 'Fantasy', 'History', 'Mystery', 'Romance', 'Sci-Fi', 'Thriller', 'War'];
  const slug = (g) => g.toLowerCase().replace(/[^a-z]+/g, '-');
  const GENRE_BY_SLUG = Object.fromEntries(GENRES.map((g) => [slug(g), g]));
  const LANGS = [['en', 'English'], ['ja', 'Japanese'], ['ko', 'Korean'], ['fr', 'French'], ['es', 'Spanish']];
  const COUNTRIES = [['', 'Any country'], ['US', 'United States'], ['GB', 'United Kingdom'], ['JP', 'Japan'], ['KR', 'South Korea'], ['AU', 'Australia'], ['FR', 'France']];
  const SORTS = [['popular', 'Most popular'], ['rating', 'Highest rated'], ['newest', 'Newest first'], ['title', 'Title A–Z']];
  const RUNTIMES = [['', 'Any'], ['short', 'Short'], ['standard', 'Standard'], ['long', 'Long']];
  const RATINGS = [['', 'Any'], ['6', '6+'], ['7', '7+'], ['8', '8+']];
  const STATUSES = [['', 'Any'], ['returning', 'Airing'], ['ended', 'Ended'], ['upcoming', 'Upcoming']];
  const YEAR_MIN = 1950, YEAR_MAX = 2025;
  const DECADES = [['2020', '2025', '2020s'], ['2010', '2019', '2010s'], ['2000', '2009', '2000s'], ['1950', '1999', 'Before 2000']];

  const MOODS = [
    ['cosy', 'Cosy', ['bluey', 'goodplace', 'onlymurders', 'totoro', 'shrinking', 'perfectdays', 'mushishi', 'pushingdaisies', 'frieren', 'columbus']],
    ['mind', 'Mind-bending', ['severance', 'pluribus', 'interstellar', 'arrival', 'paprika', 'silo', 'dandadan', 'doctorwho', 'dune2021']],
    ['slow', 'Slow cinema', ['perfectdays', 'columbus', 'aftersun', 'pastlives', 'mushishi', 'andor', 'frieren', 'arrival']],
    ['laugh', 'Laugh-out-loud', ['goodplace', 'mythicquest', 'studio', 'shrinking', 'onlymurders', 'dandadan', 'bluey', 'pushingdaisies']],
    ['short', 'Short & sharp', ['bluey', 'goodplace', 'mythicquest', 'paprika', 'totoro', 'studio', 'shrinking', 'dandadan', 'frieren']],
    ['epic', 'Epic nights', ['dune2', 'interstellar', 'shogun', 'onepiece', 'andor', 'dune2021', 'duneprophecy']],
  ];

  // Facts the fixtures don't carry (original language, country, minutes, status)
  const LANG = { frieren: 'ja', onepiece: 'ja', dandadan: 'ja', mushishi: 'ja', paprika: 'ja', totoro: 'ja', perfectdays: 'ja' };
  const COUNTRY = { frieren: 'JP', onepiece: 'JP', dandadan: 'JP', mushishi: 'JP', paprika: 'JP', totoro: 'JP', perfectdays: 'JP', bluey: 'AU', doctorwho: 'GB', slowhorses: 'GB', aftersun: 'GB', pastlives: 'US' };
  const MINUTES = { dandadan: 24, mushishi: 24, silo: 50, andor: 45, slowhorses: 45, studio: 30, shrinking: 33 };
  const STATUS_OF = { dandadan: 'returning', mushishi: 'ended', silo: 'returning', andor: 'ended', slowhorses: 'returning', studio: 'returning', shrinking: 'returning', onlymurders: 'returning', mythicquest: 'ended', pushingdaisies: 'ended' };
  const POPULAR = ['pluribus', 'studio', 'dune2', 'severance', 'andor', 'shrinking', 'dandadan', 'frieren', 'thebear', 'slowhorses', 'pastlives', 'shogun', 'onepiece', 'interstellar', 'silo', 'bluey', 'goodplace', 'onlymurders', 'dune2021', 'duneprophecy', 'arrival', 'perfectdays', 'aftersun', 'doctorwho', 'mythicquest', 'paprika', 'totoro', 'mushishi', 'pushingdaisies', 'columbus', 'dune1984', 'jodorowsky', 'frankherbert'];

  const mins = (t) => {
    if (MINUTES[t.id]) return MINUTES[t.id];
    const r = String(t.runtime || '');
    const h = /(\d+)h/.exec(r), m = /(\d+)\s*m/.exec(r);
    if (h || m) return (h ? Number(h[1]) * 60 : 0) + (m ? Number(m[1]) : 0);
    return t.type === 'tv' ? 30 : 110;
  };
  const statusOf = (t) => (t.type === 'movie' ? 'released' : STATUS_OF[t.id] || (t.status === 'Ended' ? 'ended' : 'returning'));
  const runtimeBand = (t) => {
    const m = mins(t);
    if (t.type === 'tv') return m <= 30 ? 'short' : m <= 50 ? 'standard' : 'long';
    return m < 100 ? 'short' : m <= 140 ? 'standard' : 'long';
  };

  // ---- filter state <-> URL -----------------------------------------------
  const KEYS = ['type', 'genres', 'from', 'to', 'rating', 'runtime', 'lang', 'country', 'anime', 'status', 'mood', 'sort'];
  const blank = () => ({ type: 'all', genres: [], from: '', to: '', rating: '', runtime: '', lang: '', country: '', anime: false, status: '', mood: '', sort: 'popular' });
  const fromParams = (p) => ({
    ...blank(),
    type: ['movie', 'tv'].includes(p.type) ? p.type : 'all',
    genres: (p.genres || '').split(',').filter((g) => GENRE_BY_SLUG[g]),
    from: p.from || '', to: p.to || '', rating: p.rating || '', runtime: p.runtime || '',
    lang: p.lang || '', country: p.country || '', anime: p.anime === '1', status: p.status || '', mood: p.mood || '',
    sort: SORTS.some(([v]) => v === p.sort) ? p.sort : 'popular',
  });
  const toQuery = (f) => {
    const q = [];
    if (f.type !== 'all') q.push(`type=${f.type}`);
    if (f.genres.length) q.push(`genres=${f.genres.join(',')}`);
    ['from', 'to', 'rating', 'runtime', 'lang', 'country', 'status', 'mood'].forEach((k) => { if (f[k]) q.push(`${k}=${encodeURIComponent(f[k])}`); });
    if (f.anime) q.push('anime=1');
    if (f.sort !== 'popular') q.push(`sort=${f.sort}`);
    return q.length ? `#/discover?${q.join('&')}` : '#/discover';
  };
  const hasParams = (p) => KEYS.some((k) => p[k]);

  const PRESETS = {
    filtered: { type: 'tv', genres: 'comedy', rating: '8', from: '2015' },
    empty: { type: 'tv', anime: '1', genres: 'comedy', from: '2020', rating: '8', status: 'ended' },
  };

  // ---- matching ------------------------------------------------------------
  const titles = () => Object.values(D.T);
  function match(f, skip) {
    const mood = MOODS.find((m) => m[0] === f.mood);
    let list = titles().filter((t) => {
      if (f.type !== 'all' && t.type !== f.type) return false;
      if (skip !== 'mood' && mood && !mood[2].includes(t.id)) return false;
      for (const g of f.genres) if (skip !== `genre:${g}` && !(t.genres || []).some((x) => slug(x) === g)) return false;
      if (skip !== 'year' && f.from && t.year < Number(f.from)) return false;
      if (skip !== 'year' && f.to && t.year > Number(f.to)) return false;
      if (skip !== 'rating' && f.rating && t.rating < Number(f.rating)) return false;
      if (skip !== 'runtime' && f.runtime && runtimeBand(t) !== f.runtime) return false;
      if (skip !== 'lang' && f.lang && (LANG[t.id] || 'en') !== f.lang) return false;
      if (skip !== 'country' && f.country && (COUNTRY[t.id] || 'US') !== f.country) return false;
      if (skip !== 'anime' && f.anime && !t.anime) return false;
      if (skip !== 'status' && f.status && t.type === 'tv' && statusOf(t) !== f.status) return false;
      if (skip !== 'status' && f.status && t.type === 'movie') return false;
      return true;
    });
    const pop = (t) => { const i = POPULAR.indexOf(t.id); return i < 0 ? 99 : i; };
    const by = { popular: (a, b) => pop(a) - pop(b), rating: (a, b) => b.rating - a.rating, newest: (a, b) => b.year - a.year, title: (a, b) => a.title.localeCompare(b.title) }[f.sort];
    return list.sort(by);
  }

  // Active filters as removable chips (type is the noun, not a chip)
  function chips(f) {
    const c = [];
    if (f.mood) c.push(['mood', `Mood: ${MOODS.find((m) => m[0] === f.mood)?.[1] || f.mood}`]);
    f.genres.forEach((g) => c.push([`genre:${g}`, GENRE_BY_SLUG[g]]));
    if (f.anime) c.push(['anime', 'Anime']);
    if (f.from || f.to) c.push(['year', `${f.from || YEAR_MIN}–${f.to || YEAR_MAX}`]);
    if (f.rating) c.push(['rating', `Rated ${f.rating}+`]);
    if (f.runtime) c.push(['runtime', `${RUNTIMES.find((r) => r[0] === f.runtime)[1]} runtime`]);
    if (f.lang) c.push(['lang', LANGS.find((l) => l[0] === f.lang)?.[1] || f.lang]);
    if (f.country) c.push(['country', COUNTRIES.find((l) => l[0] === f.country)?.[1] || f.country]);
    if (f.status) c.push(['status', STATUSES.find((s) => s[0] === f.status)[1]]);
    return c;
  }
  function without(f, key) {
    const g = { ...f, genres: [...f.genres] };
    if (key.startsWith('genre:')) g.genres = g.genres.filter((x) => x !== key.slice(6));
    else if (key === 'year') { g.from = ''; g.to = ''; } else if (key === 'anime') g.anime = false; else g[key] = '';
    return g;
  }
  // The catalog is TMDB; fixtures stand in for the first pages. Totals scale like a real catalog.
  const SCALE = [312, 96, 41, 17, 7, 3, 2, 1];
  const total = (f, n) => (n === 0 ? 0 : n * SCALE[Math.min(chips(f).length + (f.type !== 'all' ? 1 : 0), 7)]);
  const nf = (n) => n.toLocaleString('en-US');
  const noun = (f, n) => (f.type === 'tv' ? (n === 1 ? 'show' : 'shows') : f.type === 'movie' ? (n === 1 ? 'movie' : 'movies') : (n === 1 ? 'title' : 'titles'));

  // ---- markup --------------------------------------------------------------
  const sec = (label, body, extra = '') => `<fieldset class="disc-sec" ${extra}><legend class="t-micro c-3">${label}</legend>${body}</fieldset>`;
  const select = (key, opts, val, label, extra = '') => `<label class="sr-only" for="df-${key}-${extra}">${label}</label><select class="input" id="df-${key}-${extra}" data-f="${key}">${opts.map(([v, l]) => `<option value="${v}" ${String(v) === String(val) ? 'selected' : ''}>${esc(l)}</option>`).join('')}</select>`;
  const years = (first) => { const o = [['', first]]; for (let y = YEAR_MAX; y >= YEAR_MIN; y -= y > 2000 ? 1 : 10) o.push([String(y), String(y)]); return o; };

  function panel(f, where) {
    const isMovie = f.type === 'movie';
    return `<div class="disc-panel" data-where="${where}">
      ${sec('Type', ui.seg([['all', 'All'], ['movie', 'Movies'], ['tv', 'Shows']], f.type, 'data-f="type" aria-label="Type"').replace('class="seg"', 'class="seg seg--block"'))}
      ${where === 'sheet' ? sec('Sort by', `<div class="fchips">${SORTS.map(([v, l]) => ui.fchip(l, f.sort === v, null, `data-f="sort" data-v="${v}"`)).join('')}</div>`) : ''}
      ${sec('Genres <span class="disc-hint">match all selected</span>', `<div class="fchips">${GENRES.map((g) => ui.fchip(g, f.genres.includes(slug(g)), null, `data-f="genres" data-v="${slug(g)}"`)).join('')}</div>`)}
      ${sec('Release year', `<div class="disc-years">${select('from', years('From'), f.from, 'From year', where)}<span class="c-3" aria-hidden="true">–</span>${select('to', years('To'), f.to, 'To year', where)}</div>
        <div class="fchips">${DECADES.map(([a, b, l]) => ui.fchip(l, f.from === a && f.to === b, null, `data-f="decade" data-v="${a}-${b}"`)).join('')}</div>`)}
      ${sec('Rating', ui.seg(RATINGS.map(([v, l]) => [v, v ? `${icon('star', { size: 12, fill: true })} ${l}` : l]), f.rating, 'data-f="rating" aria-label="Minimum rating"').replace('class="seg"', 'class="seg seg--block"'))}
      ${sec('Runtime', `${ui.seg(RUNTIMES, f.runtime, 'data-f="runtime" aria-label="Runtime"').replace('class="seg"', 'class="seg seg--block"')}<p class="help">Short: under 30 min an episode, or under 100 min for a movie. Long: over 50 min, or over 2 h 20 m.</p>`)}
      ${sec('Language & country', `<div class="disc-anime"><div class="grow"><span class="w-semibold">Anime</span><span class="help">Japanese animation, shows and films</span></div>${ui.switch(`df-anime-${where}`, f.anime, 'Anime only').replace('<input ', '<input data-f="anime" ')}</div>
        <div class="fchips">${LANGS.map(([v, l]) => ui.fchip(l, f.lang === v, null, `data-f="lang" data-v="${v}"`)).join('')}</div>
        ${select('country', COUNTRIES, f.country, 'Country of origin', where)}`)}
      ${sec('Status', `${ui.seg(STATUSES, isMovie ? '' : f.status, `data-f="status" aria-label="Show status" ${isMovie ? 'aria-disabled="true"' : ''}`).replace('class="seg"', 'class="seg seg--block"')}${isMovie ? '<p class="help">For shows only.</p>' : ''}`, isMovie ? 'disabled' : '')}
    </div>`;
  }

  const moods = (f) => `<div class="disc-moods fchips fchips--scroll" role="group" aria-label="Moods">${MOODS.map(([v, l]) => ui.fchip(l, f.mood === v, null, `data-f="mood" data-v="${v}"`)).join('')}</div>`;

  function phoneChips(f) {
    const n = chips(f).length + (f.type !== 'all' ? 1 : 0);
    const quick = [
      ['type', 'tv', 'Shows', f.type === 'tv'], ['type', 'movie', 'Movies', f.type === 'movie'],
      ['anime', '1', 'Anime', f.anime], ['rating', '8', `Rated 8+`, f.rating === '8'], ['decade', '2020-2025', '2020s', f.from === '2020' && f.to === '2025'],
    ];
    return `<div class="disc-pchips fchips fchips--scroll" role="group" aria-label="Quick filters">
      <button class="fchip disc-filters-btn ${n ? 'is-on' : ''}" data-act="disc-sheet" aria-haspopup="dialog">${icon('sliders', { size: 16 })}Filters${n ? ` <span class="n">${n}</span>` : ''}</button>
      ${quick.map(([k, v, l, on]) => ui.fchip(l, on, null, `data-f="${k}" data-v="${v}"`)).join('')}
      ${f.genres.map((g) => `<button class="fchip is-on" data-rm="genre:${g}" aria-label="Remove ${GENRE_BY_SLUG[g]}">${GENRE_BY_SLUG[g]}<span class="x">${icon('x', { size: 12, stroke: 2.4 })}</span></button>`).join('')}
    </div>`;
  }

  function activeRow(f) {
    const c = chips(f);
    if (!c.length) return '';
    return `<div class="fchips disc-active" role="group" aria-label="Active filters">${c.map(([k, l]) => `<button class="fchip is-on" data-rm="${k}" aria-label="Remove filter: ${esc(l)}">${esc(l)}<span class="x">${icon('x', { size: 12, stroke: 2.4 })}</span></button>`).join('')}
      <button class="btn btn-ghost btn-sm" data-rm="all">Clear all</button></div>`;
  }

  const skelMore = () => `<div class="disc-more" aria-live="polite"><div class="poster-grid disc-skelrow" aria-hidden="true">${ui.skelPosters(10)}</div><p class="disc-more-label">${brand.eyes(30)}<span>Loading more…</span></p></div>`;

  function results(f, opts = {}) {
    const list = match(f);
    const n = total(f, list.length);
    const head = `<div class="disc-rhead">
        <div class="titles"><h2 class="t-title-2" id="disc-count">${opts.loading ? 'Finding titles…' : `${nf(n)} ${noun(f, n)}`}</h2>
          <p class="t-small c-3">${f.mood ? `${esc(MOODS.find((m) => m[0] === f.mood)?.[1])} picks from TMDB` : 'From TMDB, filtered live'}${chips(f).length ? ` · ${chips(f).length} filter${chips(f).length > 1 ? 's' : ''}` : ''}</p></div>
        <div class="disc-rtools">
          <button class="btn btn-ghost btn-sm desk-only" data-act="disc-link" aria-label="Copy a link to this view">${icon('link', { size: 16 })}Copy link</button>
          ${select('sort', SORTS, f.sort, 'Sort by', 'head').replace('class="input"', 'class="input disc-sort"')}
        </div>
      </div>${activeRow(f)}`;
    if (opts.loading) return `${head}<div class="poster-grid" aria-busy="true">${ui.skelPosters(18)}</div>`;
    if (!list.length) {
      const c = chips(f);
      const typeNoun = noun(f, 2);
      const fixes = c.map(([k, l]) => [k, l, total(without(f, k), match(f, k).length)]).sort((a, b) => b[2] - a[2])
        .map(([k, l, m]) => `<button class="fchip disc-fix${m ? ' has-results' : ''}" data-rm="${k}" aria-label="Remove ${esc(l)}: ${m ? `${nf(m)} ${typeNoun}` : `still no ${typeNoun}`}">${icon('x', { size: 13, stroke: 2.2 })}<span>${esc(l)}</span><span class="n">${m ? `→ ${nf(m)} ${typeNoun}` : 'still none'}</span></button>`).join('');
      return `${head.replace(activeRow(f), '')}${ui.empty({ cat: 'sit', title: `No ${typeNoun} match all ${c.length} filter${c.length === 1 ? '' : 's'}`, body: 'Remove one to widen the net. Each shows what you’d get back.', actions: `<div class="disc-fixes">${fixes}</div><div class="row gap-2 wrap center"><button class="btn btn-secondary" data-rm="all">Clear all filters</button><a class="btn btn-ghost" href="#/ask?q=${encodeURIComponent('an anime comedy that has finished airing')}">${brand.avatar(22)}Ask momoshtrem</a></div>` }).replace('class="empty"', 'class="empty disc-empty"')}`;
    }
    const first = list.slice(0, 18);
    return `${head}<div class="poster-grid disc-grid" id="disc-grid">${first.map((t) => ui.posterCard(t.id)).join('')}</div>${skelMore()}`;
  }

  // ---- route ---------------------------------------------------------------
  let F = blank();
  app.route('/discover', (p) => {
    let state = p.state || (hasParams(p) ? 'filtered' : 'default');
    const src = hasParams(p) ? p : (PRESETS[state] || {});
    F = fromParams(src);
    if (state === 'filtered' && !match(F).length) state = 'empty';
    const switcher = ui.protoStates('#/discover', [['default', 'Default'], ['filtered', 'Filtered'], ['empty', 'No matches'], ['loading', 'Loading']], state);
    const html = `${switcher}
      ${ui.largeTitle('Discover')}
      <header class="page disc-head desk-only"><h1 class="t-display">Discover</h1><p class="c-2">Everything on TMDB, filtered your way. The link keeps your filters, so a view can be bookmarked.</p></header>
      <div class="page disc-top">${moods(F)}<div class="phone-only">${phoneChips(F)}</div></div>
      <div class="page disc-layout">
        <aside class="disc-side desk-only" aria-label="Filters"><div class="disc-side-in">${panel(F, 'side')}</div></aside>
        <section class="disc-main" id="disc-main" aria-label="Results">${results(F, { loading: state === 'loading' })}</section>
      </div>`;
    return { title: 'Discover', tab: 'discover', html, mount };
  });

  let io = null;
  function mount(root) {
    if (io) io.disconnect();
    root.addEventListener('click', (e) => {
      const rm = e.target.closest('[data-rm]');
      if (rm && root.contains(rm)) { e.preventDefault(); apply(rm.dataset.rm === 'all' ? { ...blank(), type: F.type, sort: F.sort } : without(F, rm.dataset.rm)); return; }
      const b = e.target.closest('[data-v]');
      if (!b || !root.contains(b) || b.closest('.disc-panel[data-where="sheet"]')) return;
      const key = b.dataset.f || b.closest('[data-f]')?.dataset.f;
      if (!key) return;
      e.preventDefault();
      apply(toggle(F, key, b.dataset.v, !b.dataset.f));
    });
    root.addEventListener('change', (e) => {
      const el = e.target.closest('[data-f]');
      if (!el || !root.contains(el)) return;
      apply(toggle(F, el.dataset.f, el.type === 'checkbox' ? el.checked : el.value, true));
    });
    observe(root);
  }

  function toggle(f, key, v, set) {
    const g = { ...f, genres: [...f.genres] };
    if (key === 'genres') g.genres = g.genres.includes(v) ? g.genres.filter((x) => x !== v) : [...g.genres, v];
    else if (key === 'anime') g.anime = set ? !!v : !g.anime;
    else if (key === 'decade') { const [a, b] = v.split('-'); const on = g.from === a && g.to === b; g.from = on ? '' : a; g.to = on ? '' : b; }
    else if (key === 'type') g.type = set ? v : (g.type === v && v !== 'all' ? 'all' : v);
    else if (key === 'sort') g.sort = v;
    else if (set) g[key] = v;
    else g[key] = g[key] === v ? '' : v;
    if (g.from && g.to && Number(g.from) > Number(g.to)) [g.from, g.to] = [g.to, g.from];
    if (g.type === 'movie') g.status = '';
    return g;
  }

  // Apply live: URL first (replaceState keeps history clean), brief skeleton, then results.
  function apply(f) {
    F = f;
    const hash = toQuery(F);
    try { history.replaceState(null, '', hash); } catch (e) { /* sandbox */ }
    const root = document.getElementById('main');
    const main = root.querySelector('#disc-main');
    const ae = document.activeElement;
    let focusSel = null;
    if (ae && ae.closest && ae.closest('.disc-side')) {
      if (ae.dataset.f && ae.dataset.v != null) focusSel = `[data-f="${ae.dataset.f}"][data-v="${ae.dataset.v}"]`;
      else if (ae.dataset.v != null) focusSel = `[data-f="${ae.closest('[data-f]')?.dataset.f}"] [data-v="${ae.dataset.v}"]`;
      else if (ae.dataset.f) focusSel = `[data-f="${ae.dataset.f}"]`;
    }
    const side = root.querySelector('.disc-side-in');
    if (side) side.innerHTML = panel(F, 'side');
    root.querySelector('.disc-moods').outerHTML = moods(F);
    const pc = root.querySelector('.disc-pchips'); if (pc) pc.outerHTML = phoneChips(F);
    if (focusSel && side) { const el = side.querySelector(focusSel); if (el) el.focus({ preventScroll: true }); }
    const n = total(F, match(F).length);
    main.innerHTML = results(F, { loading: true });
    main.querySelector('#disc-count').textContent = `${nf(n)} ${noun(F, n)}`;
    clearTimeout(apply.t);
    apply.t = setTimeout(() => { main.innerHTML = results(F); observe(root); }, 380);
  }

  // Infinite scroll: when the footer comes into view, append the next page.
  function observe(root) {
    if (io) io.disconnect();
    const more = root.querySelector('.disc-more');
    const grid = root.querySelector('#disc-grid');
    if (!more || !grid || !('IntersectionObserver' in window)) return;
    io = new IntersectionObserver((entries) => {
      if (!entries.some((en) => en.isIntersecting)) return;
      const shown = grid.children.length;
      const list = match(F);
      if (shown >= list.length) return; // the rest arrives from TMDB; the footer keeps loading
      io.disconnect();
      setTimeout(() => {
        if (!document.contains(grid)) return;
        grid.insertAdjacentHTML('beforeend', list.slice(shown, shown + 18).map((t) => ui.posterCard(t.id).replace('class="pcard"', 'class="pcard disc-in"')).join(''));
        observe(root);
      }, 900);
    }, { rootMargin: '200px 0px' });
    io.observe(more);
  }

  // ---- phone filter sheet (staged; applies on "Show N results") -------------
  let draft = null;
  const sheetFoot = (f) => {
    const n = total(f, match(f).length);
    return `<button class="btn btn-ghost" data-act="disc-reset">Reset</button><button class="btn btn-primary grow" data-act="disc-apply" ${n ? '' : 'disabled'}>${n ? `Show ${nf(n)} ${noun(f, n)}` : `No ${noun(f, 2)} match`}</button>`;
  };
  app.sheet('discover-filters', () => {
    draft = { ...F, genres: [...F.genres] };
    return {
      cls: 'sheet--full disc-sheet',
      html: `<div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="discover-filters-title">Filters</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close filters" autofocus>${icon('x')}</button></div></div>
        <div class="sheet-body" id="disc-sheet-body">${panel(draft, 'sheet')}</div>
        <div class="sheet-foot" id="disc-sheet-foot">${sheetFoot(draft)}</div>`,
      mount(el) {
        const body = el.querySelector('#disc-sheet-body');
        const redraw = () => { const y = body.scrollTop; body.innerHTML = panel(draft, 'sheet'); body.scrollTop = y; el.querySelector('#disc-sheet-foot').innerHTML = sheetFoot(draft); };
        body.addEventListener('click', (e) => {
          const b = e.target.closest('[data-v]'); if (!b) return;
          const key = b.dataset.f || b.closest('[data-f]')?.dataset.f; if (!key) return;
          draft = toggle(draft, key, b.dataset.v, !b.dataset.f); redraw();
          const again = body.querySelector(b.dataset.f ? `[data-f="${b.dataset.f}"][data-v="${b.dataset.v}"]` : `[data-f="${key}"] [data-v="${b.dataset.v}"]`); if (again) again.focus({ preventScroll: true });
        });
        body.addEventListener('change', (e) => { const c = e.target.closest('[data-f]'); if (!c) return; draft = toggle(draft, c.dataset.f, c.type === 'checkbox' ? c.checked : c.value, true); redraw(); });
      },
    };
  });

  Object.assign(app.actions, {
    'disc-sheet': () => app.openSheet('discover-filters'),
    'disc-reset': () => { draft = { ...blank(), sort: draft.sort }; const b = document.getElementById('disc-sheet-body'); if (b) b.innerHTML = panel(draft, 'sheet'); const f = document.getElementById('disc-sheet-foot'); if (f) f.innerHTML = sheetFoot(draft); },
    'disc-apply': () => { const f = draft; app.close(); apply(f); window.scrollTo({ top: 0 }); },
    'disc-link': () => { try { navigator.clipboard.writeText(location.href).catch(() => {}); } catch (e) { /* ignore */ } app.toast(null, { kind: 'ready', icon: 'link', title: 'Link copied', body: 'It opens Discover with these filters.' }); },
  });

  MS.discover = { moods: MOODS, toQuery };
})();
