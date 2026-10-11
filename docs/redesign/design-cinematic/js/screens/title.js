/* Movie + show detail. One primary action that moves through:
   Add to library → Find a torrent → Downloading / preparing → Ready in Infuse. */
(function () {
  const { ui, app, art, brand, icon, data: D } = MS;
  const T = (id) => D.T[id];
  const esc = ui.esc;

  const STEPS = ['Add', 'Torrent', 'Download', 'Ready'];
  const WEBDAV = 'https://momoshtrem.duckdns.org:36911';

  // Which availability stage a title is in
  function stageOf(t, forced) {
    if (forced) return forced;
    if (t.id === 'goodplace') return app.state.goodplace === 'added' ? 'needs_torrent' : 'none';
    if (t.id === 'frieren') return app.state.frieren === 'ready' ? 'ready' : app.state.frieren === 'mapping' ? 'downloading' : 'needs_torrent';
    return { none: 'none', needs_torrent: 'needs_torrent', downloading: 'downloading', ready: 'ready', partial: 'ready', needs_review: 'needs_torrent' }[t.lib] || 'none';
  }
  const stageIndex = { none: 0, needs_torrent: 1, downloading: 2, ready: 3 };

  function moviePath(t) { return `/Movies/${t.title.replace(':', '')} (${t.year})/${t.title.replace(/[:']/g, '').replace(/ /g, '.')}.${t.year}.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv`; }
  function showPath(t) { return `/Shows/${t.short} (${t.year})/`; }

  // The primary action panel. Returns { panel, dock } (dock = phone thumb-reach bar)
  function primary(t, stage, ctx = {}) {
    const i = stageIndex[stage];
    const steps = `<div class="state-steps" aria-hidden="true">${STEPS.map((s, k) => `<i class="${k < i ? 'done' : k === i ? 'on' : ''}"></i>`).join('')}</div>
      <div class="state-label"><span>${STEPS.map((s, k) => (k === i ? `<b class="c-1 w-semibold">${s}</b>` : s)).join(' · ')}</span><span>Step ${i + 1} of 4</span></div>`;
    let head = '', btn = '', extra = '', dockBtn = '';
    const isTv = t.type === 'tv';
    if (stage === 'none') {
      head = `<p class="pp-title">Not in your library</p><p class="t-small c-2">${isTv ? `${t.seasons ? t.seasons.reduce((a, s) => a + (s.n ? s.eps : 0), 0) : 53} episodes across ${t.seasons ? t.seasons.filter((s) => s.n).length : 4} seasons.` : 'Add it, then pick a torrent.'} Adding is free: nothing downloads until you pick a torrent.</p>`;
      btn = `<button class="btn btn-primary btn-lg btn-block" data-act="title-add" data-id="${t.id}">${icon('plus', { size: 20 })}Add to library</button>`;
    } else if (stage === 'needs_torrent') {
      const what = isTv ? (t.id === 'frieren' ? 'Episodes 29–38 have no torrent' : t.id === 'goodplace' ? 'In your library · no episodes yet' : 'Some episodes have no torrent') : 'In your library · no torrent yet';
      head = `<p class="pp-title">${what}</p><p class="t-small c-2">${isTv ? 'A season pack usually covers them all in one go.' : 'Pick a release and it streams to Infuse while it downloads.'}</p>`;
      const label = isTv ? (t.id === 'frieren' ? 'Find episodes 29–38' : 'Find a torrent') : 'Find a torrent';
      btn = `<button class="btn btn-primary btn-lg btn-block" data-act="title-find" data-id="${t.id}">${icon('search', { size: 20 })}${label}</button>`;
    } else if (stage === 'downloading') {
      const pct = t.id === 'frieren' ? 12 : Math.round((t.progress || 0.42) * 100);
      head = `<div class="row between gap-3"><p class="pp-title">${t.id === 'frieren' ? 'Preparing episodes 29–38' : 'Downloading'}</p><span class="t-num w-semibold" style="color:var(--st-downloading)">${pct}%</span></div>
        ${ui.progress(pct, '', 'progress--lg')}
        <p class="t-small c-2">${t.id === 'frieren' ? 'Fetching metadata · then I\'ll match files to episodes.' : '2.3 MB/s · 9 seeders · about 1 h 52 min left. <strong class="c-1">You can start watching in Infuse now</strong>; it streams while it downloads.'}</p>`;
      btn = `<a class="btn btn-secondary btn-lg btn-block" href="#/activity">${icon('activity', { size: 20 })}View download</a>`;
      extra = `<button class="btn btn-ghost btn-block" data-act="howto">${icon('tv', { size: 18 })}How to play in Infuse</button>`;
    } else {
      const path = isTv ? showPath(t) : moviePath(t);
      head = `<div class="ready-head">${icon('play', { size: 18, fill: true })}<div class="stack"><p class="pp-title">Ready in Infuse</p><p class="t-small c-2">${isTv ? `${t.ready || 38} of ${t.total || 38} episodes ready${ctx.nextUp ? ` · next: ${ctx.nextUp}` : ''}` : `${t.quality || '2160p'} · ${t.size || '18.4 GB'} · Dolby Vision · Atmos`}</p></div></div>
        <div class="stack gap-2"><span class="t-micro c-3">WebDAV path</span><div class="copy-field">${ui.path(path)}<button class="btn btn-secondary btn-sm" data-act="copy" data-copy="${WEBDAV}${path}">${icon('copy', { size: 15 })}Copy</button></div></div>`;
      btn = `<button class="btn btn-primary btn-lg btn-block" data-act="howto">${icon('tv', { size: 20 })}How to play in Infuse</button>`;
      extra = '';
      dockBtn = `<button class="btn btn-primary btn-lg grow" data-act="howto">${icon('tv', { size: 20 })}Play in Infuse</button><button class="btn btn-secondary btn-lg btn-icon" data-act="copy" data-copy="${WEBDAV}${path}" aria-label="Copy WebDAV path">${icon('copy', { size: 20 })}</button>`;
    }
    const panel = `<div class="primary-panel glass" id="primary">${steps}${head}<div class="desk-pp stack gap-2">${btn}${extra}</div></div>`;
    const dock = `<div class="action-dock phone-only">${dockBtn || btn.replace('btn-block', 'grow')}</div>`;
    return { panel, dock };
  }

  function heroBlock(t, stage, ctx) {
    const p = primary(t, stage, ctx);
    const isTv = t.type === 'tv';
    const credit = isTv ? (t.creator ? `<span>Created by ${esc(t.creator)}</span>` : '') : t.director ? `<span>Directed by ${esc(t.director)}</span>` : '';
    const trakt = ctx.trakt ? `<p class="trakt-line">${icon('checkCheck', { size: 16 })}<span>${ctx.trakt}</span></p>` : '';
    return {
      dock: p.dock,
      html: `<section class="hero detail-hero" style="--ambient:${art.ambient(t.art)}">
        <div class="phone-bar"><a class="btn btn-glass btn-icon" href="${ctx.back || '#/home'}" aria-label="Back">${icon('chevronLeft', { size: 22 })}</a><div class="row gap-2"><button class="btn btn-glass btn-icon" data-act="title-more" aria-label="More actions">${icon('more', { size: 20 })}</button></div></div>
        <div class="hero-glow"></div>
        <div class="hero-bg">${art.backdrop(t.art, '', 3)}</div>
        <div class="hero-scrim"></div>
        <div class="page detail-grid">
          <div class="poster-col">${ui.poster(t.id, '', 'hero-poster')}</div>
          <div class="detail-title">
            <p class="eyebrow">${isTv ? `${icon('tv', { size: 15 })}Series · ${esc(t.network || '')}` : `${icon('film', { size: 15 })}Movie`}${t.anime ? ' · Anime' : ''}</p>
            <h1 class="t-display">${esc(t.title)}</h1>
            <p class="meta-line"><span class="star">${icon('star', { size: 13, fill: true })}${t.rating}</span><span>${isTv ? t.years : t.year}</span><span>${esc(t.runtime || '')}</span><span>${t.genres.join(', ')}</span>${isTv && t.status ? `<span>${t.status}</span>` : ''}</p>
            <p class="overview clamp-3" id="overview">${esc(t.overview || '')}</p>
            <p class="meta-line credit">${credit}${t.cast ? `<span>${t.cast.slice(0, 3).map(esc).join(', ')}</span>` : ''}</p>
            ${trakt}
            ${ctx.why ? `<div class="why-card">${brand.avatar(28)}<p><span class="t-micro c-accent">Why it's for you</span><br>${esc(ctx.why)}</p></div>` : ''}
          </div>
          ${p.panel}
        </div>
      </section>`,
    };
  }

  function castRow(t) {
    if (!t.cast) return '';
    const roles = { dune2: ['Paul Atreides', 'Chani', 'Lady Jessica', 'Stilgar', 'Feyd-Rautha'], goodplace: ['Eleanor', 'Michael', 'Chidi', 'Tahani', 'Janet', 'Jason'], severance: ['Mark S.', 'Helly R.', 'Dylan G.', 'Mr. Milchick', 'Irving B.', 'Harmony Cobel'] }[t.id] || [];
    return `<section class="section" aria-labelledby="cast-h"><div class="section-head"><h2 class="t-title-3" id="cast-h">Cast</h2></div>
      <div class="cast-row">${t.cast.map((c, i) => `<div class="cast"><div class="media media--square">${art.portrait(c)}</div><span class="w-medium">${esc(c)}</span><span>${esc(roles[i] || '')}</span></div>`).join('')}</div></section>`;
  }

  function trailer(t) {
    return `<section class="section" aria-labelledby="tr-h"><div class="section-head"><h2 class="t-title-3" id="tr-h">Trailer</h2></div>
      <a class="trailer media media--wide" href="https://www.youtube.com/results?search_query=${encodeURIComponent(t.title + ' trailer')}" target="_blank" rel="noopener">${art.backdrop(t.art, '', 5)}<span class="play">${icon('play', { size: 26, fill: true })}</span><span class="tl">Official trailer · 2:41 · opens YouTube</span></a></section>`;
  }

  function torrentCard(t) {
    const name = t.type === 'movie' ? 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX' : D.severanceRelease.replace('S02E05.Trojans.Horse', 'S02');
    return `<section class="panel panel-pad stack gap-3 torrent-now" aria-labelledby="tn-h">
      <div class="row between gap-2"><h2 class="t-title-3" id="tn-h">Current torrent</h2>${ui.pill('ready', 'Healthy', { icon: 'check' })}</div>
      ${ui.releaseName(name)}
      <div class="chips">${['2160p', 'WEB-DL', 'Dolby Vision', 'HDR10', 'Atmos', 'H.265', 'FLUX'].map((c, i) => ui.chip(c, i < 1 || c === 'Dolby Vision' ? 'hi' : '')).join('')}</div>
      <div class="facts-grid">
        <div><span class="t-micro c-3">Size</span><span class="t-num">${t.type === 'movie' ? '18.4 GB' : '58.3 GB'}</span></div>
        <div><span class="t-micro c-3">Seeders</span><span class="t-num" style="color:var(--st-ready)">312</span></div>
        <div><span class="t-micro c-3">Added</span><span>Oct 2</span></div>
        <div><span class="t-micro c-3">Indexer</span><span>TorrentLeech</span></div>
      </div>
      <div class="row gap-2 wrap"><button class="btn btn-secondary btn-sm" data-act="title-find" data-id="${t.id}">${icon('refresh', { size: 15 })}Replace</button><a class="btn btn-ghost btn-sm" href="#/activity/4f1a%E2%80%A6be90">Files &amp; stats</a></div>
    </section>`;
  }

  function subtitles(t) {
    const S = D.subtitles;
    return `<section class="panel stack subs" aria-labelledby="subs-h">
      <div class="panel-head"><h2 class="t-title-3" id="subs-h">Subtitles</h2><span class="t-small c-3">${S.have.length} languages</span></div>
      <ul class="subs-have">${S.have.map((s) => `<li class="list-row"><span class="lang">${s.code}</span><div class="grow"><span class="w-medium">${s.lang}</span><span class="t-small c-3">${s.source} · ${s.note}</span></div>${s.source === 'Embedded' ? '' : `<button class="btn btn-ghost btn-sm btn-icon" aria-label="Remove ${s.lang} subtitles">${icon('x', { size: 16 })}</button>`}</li>`).join('')}</ul>
      <div class="panel-pad stack gap-3" style="border-top:1px solid var(--line-1)">
        <div class="row gap-2"><div class="input-wrap grow">${icon('search', { size: 17 })}<input class="input" id="sub-q" value="Azerbaijani" aria-label="Subtitle language"></div><button class="btn btn-secondary" data-act="sub-search">Search</button></div>
        <p class="t-small c-3">OpenSubtitles · ranked by how well they match your release</p>
        <ul class="stack gap-2">${S.results.map((r, i) => `<li class="sub-result"><div class="stack gap-1" style="min-width:0">${ui.releaseName(r.file, 'p', 'c-2')}<span class="row gap-2 wrap">${ui.pill(i === 0 ? 'ready' : 'warning', r.match, { icon: i === 0 ? 'check' : 'alertTriangle' })}<span class="t-small c-3">${r.downloads} downloads</span></span></div><button class="btn ${i === 0 ? 'btn-primary' : 'btn-secondary'} btn-sm" data-act="sub-get">${icon('download', { size: 15 })}Get</button></li>`).join('')}</ul>
      </div>
    </section>`;
  }

  function moreLike(ids, title = 'More like this') {
    return ui.rail({ id: 'more-like', title, items: ids.map((id) => ui.posterCard(id)).join('') });
  }

  function aside(t, ctx) {
    const facts = t.type === 'movie'
      ? [['Director', t.director], ['Runtime', t.runtime], ['Released', `${t.year}`], ['Original language', 'English'], ['TMDB', `${t.rating} · 6.1k votes`]]
      : [['Network', t.network], ['Seasons', t.seasons ? t.seasons.filter((s) => s.n).length : '—'], ['Status', t.status], ['Episode ordering', t.anime ? 'Absolute' : 'Aired'], ['TMDB', `${t.rating}`]];
    return `<aside class="aside-stack">
      ${ctx.traktCard || ''}
      <section class="panel"><div class="panel-head"><h2 class="t-title-3">Details</h2></div><dl class="facts-list">${facts.filter((f) => f[1]).map(([k, v]) => `<div><dt>${k}</dt><dd>${esc(v)}</dd></div>`).join('')}</dl></section>
    </aside>`;
  }

  function traktCard(lines) {
    return `<section class="panel panel-pad stack gap-3"><div class="row between"><h2 class="t-title-3">On Trakt</h2><span class="t-small c-3">read-only</span></div>${lines}</section>`;
  }

  // ---------------------------------------------------------------- movie
  function movie(t, p) {
    const stage = stageOf(t, p.state);
    const ctx = {
      back: '#/home',
      why: t.id === 'pastlives' ? 'You gave Perfect Days a 9 and loved Aftersun. Same patience, same ache, and it\'s only 1 h 46 m.' : t.id === 'perfectdays' ? 'Calm, observational, and set in Tokyo, like the slow cinema you rate highest.' : '',
      trakt: t.id === 'dune2' ? 'Watched Mar 14, 2024 · You rated it 9/10' : t.id === 'interstellar' ? 'Watched 3 times · last Jan 2023 · You rated it 10/10' : '',
      traktCard: t.id === 'dune2' ? traktCard(`<p class="row gap-2">${icon('checkCheck', { size: 18, cls: 'c-accent' })}Watched Mar 14, 2024</p><p class="row gap-2">${icon('star', { size: 18, cls: 'c-accent' })}Your rating: <strong>9</strong>/10</p><p class="t-small c-3">Infuse scrobbles plays to Trakt; momoshtrem only reads.</p>`) : '',
    };
    const h = heroBlock(t, stage, ctx);
    const switcher = ui.protoStates(`#/title/movie/${t.id}`, [['none', '1 · Add'], ['needs_torrent', '2 · Find torrent'], ['downloading', '3 · Downloading'], ['ready', '4 · Ready']], stage);
    const ready = stage === 'ready';
    return {
      title: t.title, tab: null, overArt: true, mainCls: 'has-hero',
      html: `${switcher}${h.html}
        <div class="page detail-body">
          <div class="page-sections">
            ${ready ? torrentCard(t) : ''}
            ${ready ? subtitles(t) : ''}
            ${castRow(t)}
            ${trailer(t)}
            ${moreLike(['arrival', 'interstellar', 'pastlives', 'columbus', 'aftersun', 'perfectdays', 'paprika'].filter((x) => x !== t.id))}
          </div>
          ${aside(t, ctx)}
        </div>${h.dock}`,
    };
  }

  // ---------------------------------------------------------------- show
  function episodesFor(t, season) {
    if (t.id === 'frieren' && season === 1) {
      return D.frierenEps.map((e) => {
        if (app.state.frieren === 'ready' && e.n >= 29) return { ...e, state: 'ready' };
        if (app.state.frieren === 'mapping' && e.n >= 29) return { ...e, state: 'downloading', pct: 12 };
        return e;
      });
    }
    if (t.id === 'frieren' && season === 0) return ['Recap: The Journey So Far', '“Frieren” Mini Anime 1', '“Frieren” Mini Anime 2', '“Frieren” Mini Anime 3', '“Frieren” Mini Anime 4', '“Frieren” Mini Anime 5'].map((name, i) => ({ n: i + 1, name, state: i < 4 ? 'ready' : i === 4 ? 'missing' : 'review', runtime: i ? 2 : 24 }));
    if (t.id === 'severance' && season === 2) return D.severanceS2.map((e) => ({ ...e, runtime: 52 }));
    if (t.id === 'severance' && season === 1) return ['Good News About Hell', 'Half Loop', 'In Perpetuity', 'The You You Are', 'The Grim Barbarity of Optics and Design', 'Hide and Seek', 'Defiant Jazz', 'What\'s for Dinner?', 'The We We Are'].map((name, i) => ({ n: i + 1, name, state: 'watched', runtime: 55 }));
    if (t.id === 'onepiece') {
      // Egghead (S22) starts at absolute #1086; show E15–E38 (#1100–#1123)
      return Array.from({ length: 24 }, (_, i) => { const n = 15 + i, abs = 1085 + n; return { n, abs, name: `Episode ${abs}`, state: abs <= 1110 ? 'watched' : abs <= 1118 ? 'ready' : abs === 1119 ? 'downloading' : 'missing', pct: 55, runtime: 24 }; });
    }
    if (t.id === 'doctorwho' && season === 0) return ['The Christmas Invasion', 'The Runaway Bride', 'Voyage of the Damned', 'The Next Doctor', 'Planet of the Dead', 'The Waters of Mars', 'The End of Time (1)', 'The End of Time (2)'].map((name, i) => ({ n: i + 1, name, state: i < 3 ? 'watched' : i === 4 ? 'review' : i < 6 ? 'ready' : 'missing', runtime: 60 }));
    const s = (t.seasons || []).find((x) => x.n === season) || { eps: 13 };
    return Array.from({ length: Math.min(s.eps, 14) }, (_, i) => ({ n: i + 1, name: `Episode ${i + 1}`, state: t.lib === 'none' ? 'missing' : 'ready', runtime: 22 }));
  }

  function show(t, p) {
    const stage = stageOf(t, p.state);
    const seasons = t.seasons || [{ n: 1, eps: 10 }];
    const defaultSeason = t.id === 'onepiece' ? 22 : t.id === 'severance' ? 2 : seasons.find((s) => s.n > 0)?.n ?? 1;
    const season = p.season != null ? Number(p.season) : defaultSeason;
    const eps = episodesFor(t, season);
    const nextEp = t.id === 'frieren' ? 29 : t.id === 'severance' ? 5 : t.id === 'onepiece' ? 26 : null;
    const nextUp = t.id === 'severance' ? 'S02E05' : t.id === 'frieren' ? 'S01E29' : t.id === 'thebear' ? 'S04E01' : '';
    const needsReview = t.id === 'frieren' || t.id === 'doctorwho';
    const ctx = {
      back: '#/home', nextUp,
      why: t.id === 'goodplace' ? 'Same "what is this place really?" mystery as Severance, played for big laughs. Bright, kind and fast.' : t.id === 'pluribus' ? 'Vince Gilligan, and dry sci-fi is your highest-rated corner of Trakt.' : '',
      trakt: t.id === 'severance' ? 'Watching · 13 of 19 episodes · last watched Oct 8' : t.id === 'frieren' ? 'Watching · 28 of 38 · You rated season 1 10/10' : '',
    };
    const h = heroBlock(t, stage, ctx);
    const inLib = stage !== 'none';
    const counts = (list) => ({ missing: list.filter((e) => e.state === 'missing').length, review: list.filter((e) => e.state === 'review').length });
    const c = counts(eps);

    // Season selector: tabs for few seasons, select + tabs for many; Specials always last
    const ordered = [...seasons.filter((s) => s.n > 0), ...seasons.filter((s) => s.n === 0)];
    const seasonTabs = `<div class="tabs season-tabs" role="tablist" aria-label="Seasons">${ordered.map((s) => `<a role="tab" aria-selected="${s.n === season}" href="#/title/tv/${t.id}?season=${s.n}" class="tab-a">${s.n === 0 ? 'Specials' : s.name ? `${s.name}` : `Season ${s.n}`} <span class="n">${s.eps}</span></a>`).join('')}</div>`;

    // Long-season tools: ranges + jump
    const long = eps.length > 20 || t.id === 'onepiece';
    let ranges = '';
    if (t.id === 'frieren' && season === 1) ranges = [['1–10', 1], ['11–20', 11], ['21–28', 21], ['29–38', 29]].map(([l, n], i) => `<button class="fchip" aria-pressed="${i === 3}" data-act="ep-jump" data-ep="${n}">${l}${i === 3 ? ' <span class="n">10 missing</span>' : ''}</button>`).join('');
    if (t.id === 'onepiece') ranges = [['Egghead · 1086–1155', true], ['Wano · 892–1085', false], ['Whole Cake · 783–877', false], ['All arcs', false]].map(([l, on]) => `<button class="fchip" aria-pressed="${on}">${l}</button>`).join('');

    const view = p.view || 'compact';
    const se = (n) => `S${String(season).padStart(2, '0')}E${String(n).padStart(2, '0')}`;
    const visible = t.id === 'frieren' && season === 1 ? eps.filter((e) => e.n >= 21) : eps;
    const list = `<ol class="ep-list">${visible.map((e) => ui.epRow(e, { next: e.n === nextEp && season === defaultSeason, se: t.id === 'onepiece' ? `#${e.abs} · ${se(e.n)}` : se(e.n), rich: view === 'rich', art: t.art })).join('')}</ol>
      ${t.id === 'frieren' && season === 1 ? `<button class="btn btn-ghost btn-block show-earlier" data-act="ep-jump" data-ep="1">${icon('chevronUp', { size: 16 })}Show episodes 1–20 (all watched)</button>` : ''}
      ${t.id === 'onepiece' ? `<p class="t-small c-3 virt-note">${icon('layers', { size: 14 })}Showing 24 of 70 in this arc. The list is virtualised; jump or scroll for more.</p>` : ''}`;

    const reviewBanner = needsReview && inLib && app.state.frieren !== 'ready' ? `<div class="banner banner--review">${icon('alertCircle', { size: 20 })}<div class="grow"><strong>${t.id === 'frieren' ? '2 files need a quick review' : '1 special needs review'}</strong><span>${t.id === 'frieren' ? 'A recap (28.5) and a v2 re-release didn\'t map cleanly.' : '"The Waters of Mars" matched two files.'}</span></div><div class="actions"><a class="btn btn-secondary btn-sm" href="#/mapping">Review mapping</a></div></div>` : '';

    const switcher = ui.protoStates(`#/title/tv/${t.id}`, [['', 'Live'], ['none', 'Not in library'], ['needs_torrent', 'Needs torrent'], ['downloading', 'Downloading'], ['ready', 'Ready']], p.state || '');

    const episodesSection = !inLib ? `<section class="section"><div class="section-head"><h2 class="t-title-2">Episodes</h2></div>${seasonTabs}
        <div class="empty" style="padding-block:var(--sp-8)">${ui.cat('peek', 120)}<h3>Add it to see what's ready</h3><p>Once it's in your library, each episode shows whether it's ready, downloading or missing.</p></div></section>`
      : `<section class="section episodes" aria-labelledby="eps-h">
        <div class="section-head"><div class="titles"><h2 class="t-title-2" id="eps-h">Episodes</h2><p class="t-small c-3">${t.id === 'onepiece' ? 'Absolute numbering · 1,124 episodes · 1,088 ready' : `${t.ready ?? eps.length} of ${t.total ?? eps.length} ready`}</p></div>
          <div class="actions"><button class="btn btn-ghost btn-sm" data-act="show-settings">${icon('settings', { size: 16 })}<span class="sm-up">Show settings</span></button></div></div>
        ${reviewBanner}
        ${seasonTabs}
        <div class="ep-tools">
          ${ranges ? `<div class="ep-range" role="group" aria-label="Episode ranges">${ranges}</div>` : ''}
          <div class="row gap-2 ep-tools-right">
            ${long ? `<label class="jump input-wrap">${icon('hash', { size: 16 })}<input class="input" inputmode="numeric" placeholder="Jump to episode" aria-label="Jump to episode number" data-act-input="jump"></label>` : ''}
            ${ui.seg([['compact', icon('list', { size: 16 }) + '<span class="sr-only">Compact</span>'], ['rich', icon('rows', { size: 16 }) + '<span class="sr-only">With stills</span>']], view, 'aria-label="Episode list density" data-seg="view"')}
          </div>
        </div>
        <div class="bulk row gap-2 wrap">
          ${c.missing ? `<button class="btn btn-secondary btn-sm" data-act="title-find" data-id="${t.id}" data-scope="season">${icon('layers', { size: 15 })}Find season pack</button><button class="btn btn-secondary btn-sm" data-act="title-find" data-id="${t.id}">${icon('search', { size: 15 })}Find missing episodes <span class="count">${c.missing}</span></button>` : `<span class="t-small c-3 row gap-2">${icon('check', { size: 15 })}Every episode this season is ready or watched</span>`}
        </div>
        ${list}
      </section>`;

    return {
      title: t.title, overArt: true, mainCls: 'has-hero',
      html: `${switcher}${h.html}
        <div class="page detail-body">
          <div class="page-sections">
            ${episodesSection}
            ${castRow(t)}
            ${moreLike(t.anime ? ['mushishi', 'dandadan', 'paprika', 'totoro', 'onepiece'] : ['silo', 'andor', 'mythicquest', 'onlymurders', 'shrinking', 'pushingdaisies', 'slowhorses'].filter((x) => x !== t.id))}
          </div>
          ${aside(t, ctx)}
        </div>${h.dock}`,
      mount(root) {
        root.querySelectorAll('[data-seg="view"] button').forEach((b) => b.addEventListener('click', () => app.go(`#/title/tv/${t.id}?season=${season}&view=${b.dataset.v}`, { noTransition: true })));
        const j = root.querySelector('[data-act-input="jump"]');
        if (j) j.addEventListener('keydown', (e) => { if (e.key === 'Enter') { const el = root.querySelector(`#ep-${j.value}`) || root.querySelector('.ep-list li:last-child'); if (el) { el.scrollIntoView({ block: 'center' }); el.classList.add('flash'); } } });
        if (p.howto) setTimeout(() => app.openSheet('howto'), 50);
      },
    };
  }

  app.route('/title/:type/:id', (p) => {
    const t = T(p.id) || T('dune2');
    return t.type === 'movie' ? movie(t, p) : show(t, p);
  });

  // --------------------------------------------------------------- sheets
  app.sheet('show-settings', () => ({
    cls: 'sheet--narrow',
    html: `<div class="sheet-grabber" aria-hidden="true"></div>
      <div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="show-settings-title">Show settings · Frieren</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
      <div class="sheet-body stack gap-6">
        <fieldset class="stack gap-3 fs"><legend class="label">Episode ordering</legend>
          ${[['aired', 'Aired', 'Seasons and episodes as TMDB lists them. Right for most shows.'], ['absolute', 'Absolute (anime)', 'One running number: #29 instead of S01E29. Matches fansub releases like SubsPlease.'], ['group', 'TMDB episode group', 'Use an alternative order, e.g. "Seasons (production)".']].map(([v, l, d], i) => `<label class="radio-card"><input type="radio" name="ord" value="${v}" ${i === 1 ? 'checked' : ''}><span class="stack gap-1"><span class="w-semibold">${l}</span><span class="t-small c-2">${d}</span></span></label>`).join('')}
          <select class="input" aria-label="Episode group" disabled><option>Choose an episode group…</option></select>
        </fieldset>
        <div class="row between gap-4"><div class="stack"><span class="w-medium">Include specials</span><span class="t-small c-3">Season 0: recaps, OVAs, mini anime</span></div>${MS.ui.switch('sw-specials', true, 'Include specials')}</div>
        <div class="field"><label class="label" for="cmode">File matching</label><select class="input" id="cmode"><option>Rules, then AI for tricky files (recommended)</option><option>Rules only</option><option>Always ask AI</option></select><span class="help">AI only sees file names and episode lists.</span></div>
      </div>
      <div class="sheet-foot"><button class="btn btn-secondary grow" data-act="close">Cancel</button><button class="btn btn-primary grow" data-act="close">Save</button></div>`,
  }));

  app.sheet('ep-menu', (p) => ({
    cls: 'sheet--narrow',
    html: `<div class="sheet-grabber" aria-hidden="true"></div>
      <div class="sheet-head"><div class="sheet-title"><div class="stack"><span class="t-small c-3">Severance · S02E${String(p.ep || 5).padStart(2, '0')}</span><h2 class="t-title-3" id="ep-menu-title">Trojan's Horse</h2></div><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
      <div class="sheet-body"><div class="menu" style="background:none;border:0;box-shadow:none;padding:0">
        <button data-act="howto">${icon('tv', { size: 18 })}Play in Infuse</button>
        <button data-act="close">${icon('captions', { size: 18 })}Subtitles<span class="hint">EN, RU</span></button>
        <button data-act="close">${icon('checkCheck', { size: 18 })}Mark as watched<span class="hint">local only</span></button>
        <button data-act="close">${icon('arrowRightLeft', { size: 18 })}Change file mapping</button>
        <button data-act="close">${icon('refresh', { size: 18 })}Replace torrent</button>
        <div class="sep"></div>
        <button class="danger" data-act="close">${icon('trash', { size: 18 })}Remove file from episode</button>
      </div></div>`,
  }));

  app.sheet('title-more', () => ({
    cls: 'sheet--narrow',
    html: `<div class="sheet-grabber" aria-hidden="true"></div><div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="title-more-title">More</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
      <div class="sheet-body"><div class="menu" style="background:none;border:0;box-shadow:none;padding:0">
        <button data-act="close">${icon('refresh', { size: 18 })}Refresh from TMDB</button><button data-act="close">${icon('external', { size: 18 })}Open on Trakt</button><button data-act="close">${icon('external', { size: 18 })}Open on TMDB</button>
        <div class="sep"></div><button class="danger" data-act="title-remove">${icon('trash', { size: 18 })}Remove from library…</button></div></div>`,
  }));

  Object.assign(app.actions, {
    'title-add': (b) => {
      const id = b.dataset.id;
      b.classList.add('is-loading');
      setTimeout(() => {
        if (id === 'goodplace') { app.state.goodplace = 'added'; app.save(); }
        app.go(`#/title/${T(id).type}/${id}?state=needs_torrent`, { noTransition: true });
        app.toast(null, { kind: 'ready', title: 'Added to library', body: `${T(id).title} · next, find a torrent`, icon: 'check' });
      }, 650);
    },
    add: (b) => app.toast(null, { kind: 'ready', title: 'Added to library', body: `${T(b.dataset.id).title} · find a torrent when you're ready`, action: 'Open', href: `#/title/tv/${b.dataset.id}` }),
    'title-find': (b) => {
      const id = b.dataset.id; const t = T(id);
      if (t.type === 'movie') app.openSheet('torrent', { movie: id });
      else if (id === 'frieren') app.openSheet('torrent', { show: 'frieren', se: 'S01E29', scope: b.dataset.scope || 'episode' });
      else if (id === 'severance') app.openSheet('torrent', { show: 'severance', se: 'S02E08', scope: b.dataset.scope || 'episode' });
      else app.openSheet('torrent', { show: id, scope: 'season', season: '1' });
    },
    'find-ep': (b) => {
      const n = Number(b.dataset.ep);
      const id = app.path.split('/').pop();
      app.openSheet('torrent', { show: id, se: `S${id === 'severance' ? '02' : '01'}E${String(n).padStart(2, '0')}`, scope: 'episode' });
    },
    'ep-menu': (b) => app.openSheet('ep-menu', { ep: b.dataset.ep }),
    'ep-jump': (b) => { const el = document.getElementById(`ep-${b.dataset.ep}`); if (el) el.scrollIntoView({ block: 'start', behavior: 'smooth' }); },
    'show-settings': () => app.openSheet('show-settings'),
    'title-more': () => app.openSheet('title-more'),
    'title-remove': () => { app.close(); app.openDialog('remove'); },
    'sub-search': () => {},
    'sub-get': (b) => { b.outerHTML = MS.ui.pill('ready', 'Downloaded', { icon: 'check' }); },
  });
})();
