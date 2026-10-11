/* Torrent search sheet. Opens from a movie, a season or an episode.
   Phone: full-height bottom sheet, sticky search + filter chips, nested filter sheet.
   Desktop: right-hand panel. States: results · searching · none · filtered-empty · partial */
(function () {
  const { ui, app, icon, brand, data: D } = MS;
  const T = (id) => D.T[id];
  const esc = ui.esc;

  // ---- result sets per context ------------------------------------------
  const R = (id, raw, size, sizeB, seeders, leechers, age, indexer, attrs, pack, score, extra = {}) => ({ id, raw, size, sizeB, seeders, leechers, age, indexer, attrs, pack, score, ...extra });
  const SEV = [
    R('s1', D.severanceRelease.replace('S02E05.Trojans.Horse', 'S02E08.Sweet.Vitriol'), '4.1 GB', 4.1, 312, 21, '8 mo', 'TorrentLeech', ['2160p', 'WEB-DL', 'DV', 'HDR10', 'Atmos', 'H.265', 'FLUX'], 'episode', 97, { best: true, why: ['2160p WEB-DL', 'Dolby Vision', '4.1 GB', '312 seeders', 'matches your quality profile'] }),
    R('s2', 'Severance.S02E08.Sweet.Vitriol.1080p.ATVP.WEB-DL.DDP5.1.H.264-NTb', '2.6 GB', 2.6, 540, 12, '8 mo', 'TorrentLeech', ['1080p', 'WEB-DL', 'DDP 5.1', 'H.264', 'NTb'], 'episode', 88),
    R('s3', 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', '41.8 GB', 41.8, 198, 30, '7 mo', 'TorrentLeech', ['2160p', 'WEB-DL', 'DV', 'Atmos', 'FLUX', 'Season 2 pack'], 'season', 90, { note: 'Includes episodes you already have; they\'ll be skipped, not re-downloaded.' }),
    R('s4', 'Severance S02E08 720p HEVC x265-MeGusta', '410 MB', 0.41, 77, 3, '8 mo', 'The Pirate Bay', ['720p', 'x265', 'MeGusta'], 'episode', 61),
  ];
  const GP = [
    R('g1', 'The.Good.Place.S01.1080p.NF.WEB-DL.DDP5.1.x264-NTb', '14.2 GB', 14.2, 141, 6, '5 yr', 'TorrentLeech', ['1080p', 'WEB-DL', 'DDP 5.1', 'x264', 'NTb', 'Season 1 pack'], 'season', 94, { best: true, why: ['Whole season 1', '1080p WEB-DL from NTb, a group you prefer', '141 seeders', '1.1 GB per episode'] }),
    R('g2', 'The.Good.Place.S01-S04.COMPLETE.1080p.BluRay.x265.10bit.AAC5.1-YAWNiX', '22.7 GB', 22.7, 88, 14, '3 yr', 'The Pirate Bay', ['1080p', 'BluRay', 'x265', '10-bit', 'YAWNiX', 'Seasons 1–4'], 'complete', 89, { note: 'Covers all 4 seasons, 53 episodes' }),
    R('g3', 'The Good Place Season 1 Complete 720p WEB x264 [i_c]', '4.9 GB', 4.9, 37, 2, '7 yr', 'The Pirate Bay', ['720p', 'WEB', 'x264', 'Season 1 pack'], 'season', 70),
    R('g4', 'The.Good.Place.S01.2160p.HDR.WEB.H265-CAKES', '31.0 GB', 31.0, 4, 1, '2 yr', 'TorrentLeech', ['2160p', 'WEB', 'HDR10', 'H.265', 'CAKES', 'Season 1 pack'], 'season', 55, { warn: 'Only 4 seeders · may be slow' }),
  ];
  const MOVIE = (t) => {
    const n = t.title.replace(/[:']/g, '').replace(/ /g, '.');
    return [
      R('m1', `${n}.${t.year}.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX`, '15.6 GB', 15.6, 286, 19, '1 yr', 'TorrentLeech', ['2160p', 'WEB-DL', 'DV', 'HDR10', 'Atmos', 'H.265', 'FLUX'], 'movie', 96, { best: true, why: ['2160p WEB-DL', 'Dolby Vision', '15.6 GB (7.5 GB/h)', '286 seeders', 'matches your quality profile'] }),
      R('m2', `${n}.${t.year}.1080p.BluRay.x264-SPARKS`, '9.8 GB', 9.8, 410, 8, '1 yr', 'The Pirate Bay', ['1080p', 'BluRay', 'x264', 'SPARKS'], 'movie', 85),
      R('m3', `${n}.${t.year}.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.Atmos.7.1-FGT`, '61.2 GB', 61.2, 52, 30, '11 mo', 'TorrentLeech', ['2160p', 'REMUX', 'DV', 'TrueHD Atmos', 'FGT'], 'movie', 64, { flags: ['size_outlier'], warn: 'Over your 8 GB/hour limit' }),
      R('m4', `${n} ${t.year} HDCAM x264 AAC`, '1.2 GB', 1.2, 900, 45, '1 yr', 'The Pirate Bay', ['CAM', 'x264'], 'movie', 4, { flags: ['cam'], warn: 'CAM recording · filmed in a cinema' }),
    ];
  };

  function context(p) {
    if (p.movie) {
      const t = T(p.movie);
      return { key: `m-${t.id}`, t, label: t.title, sub: `${t.year} · ${t.runtime}`, q: `${t.title.replace(':', '')} ${t.year}`, scopes: null, set: MOVIE(t), facets: null };
    }
    const t = T(p.show || 'frieren');
    if (t.id === 'frieren') {
      const ep = (p.se || 'S01E29').slice(-2);
      return { key: 'frieren', t, label: 'Frieren', sub: `S01E${ep} · ${D.frierenEps[Number(ep) - 1]?.name || ''}`, q: `Frieren ${Number(ep)}`, also: [`Sousou no Frieren ${Number(ep)}`, `Frieren S02E${String(Number(ep) - 28).padStart(2, '0')} (scene numbering)`], scopes: ['episode', 'season', 'any'], set: D.releases, facets: D.facets,
        scopeSet: { episode: ['r1', 'r3', 'r5', 'r6'], season: ['r1', 'r2', 'r4'], any: ['r1', 'r2', 'r3', 'r4', 'r5', 'r6'] }, total: { episode: 27, season: 13, any: 45 } };
    }
    if (t.id === 'severance') return { key: 'severance', t, label: 'Severance', sub: `${p.se || 'S02E08'} · Sweet Vitriol`, q: `Severance ${p.se || 'S02E08'}`, scopes: ['episode', 'season', 'any'], set: SEV, scopeSet: { episode: ['s1', 's2', 's4'], season: ['s3'], any: ['s1', 's2', 's3', 's4'] }, total: { episode: 31, season: 6, any: 52 } };
    return { key: t.id, t, label: t.title, sub: `Season ${p.season || 1} · ${t.seasons?.[0]?.eps || 13} episodes`, q: `${t.title} S0${p.season || 1}`, scopes: ['episode', 'season', 'any'], set: GP, scopeSet: { episode: [], season: ['g1', 'g2', 'g3', 'g4'], any: ['g1', 'g2', 'g3', 'g4'] }, total: { episode: 0, season: 18, any: 64 } };
  }

  const QUICK = [['res:2160p', '2160p'], ['res:1080p', '1080p'], ['codec:x265', 'x265'], ['hdr:dv', 'Dolby Vision'], ['seed:1', 'Has seeders']];
  const match = (r, f) => {
    const a = r.attrs.join(' ');
    const [k, v] = f.split(':');
    if (k === 'res') return a.includes(v);
    if (k === 'codec') return /x265|HEVC|H\.265/.test(a);
    if (k === 'hdr') return /DV|Dolby Vision/.test(a);
    if (k === 'seed') return r.seeders > 0;
    if (k === 'pack') return r.pack === v;
    if (k === 'src') return a.includes(v);
    return true;
  };
  const facetCount = (ctx, f) => {
    const map = { 'res:2160p': 4, 'res:1080p': 31, 'codec:x265': 16, 'hdr:dv': 1, 'seed:1': 41 };
    return ctx.key === 'frieren' ? map[f] : ctx.set.filter((r) => match(r, f)).length * 3 + (f === 'seed:1' ? 2 : 0);
  };

  // ---- rendering -----------------------------------------------------------
  function results(ctx, st) {
    let list = ctx.scopes ? ctx.set.filter((r) => ctx.scopeSet[st.scope].includes(r.id)) : ctx.set;
    const before = list.length;
    st.filters.forEach((f) => { list = list.filter((r) => match(r, f)); });
    if (st.sort === 'seeders') list = [...list].sort((a, b) => b.seeders - a.seeders);
    if (st.sort === 'size') list = [...list].sort((a, b) => a.sizeB - b.sizeB);
    return { list, before, total: ctx.scopes ? ctx.total[st.scope] : 38 };
  }

  function body(ctx, st) {
    if (st.view === 'searching') {
      return `<div class="ts-searching" role="status" aria-live="polite">
        <div class="row gap-3">${brand.eyes(44)}<div class="stack"><strong>Searching 3 indexers…</strong><span class="t-small c-3">${esc(st.q)}${ctx.also ? ` · also ${esc(ctx.also[0])}` : ''}</span></div></div>
        <ul class="ts-indexers">
          <li>${icon('check', { size: 15 })}<span>Nyaa</span><span class="t-num c-3">26 results</span></li>
          <li>${icon('check', { size: 15 })}<span>TorrentLeech</span><span class="t-num c-3">11 results</span></li>
          <li class="is-wait"><span class="mini-spin"></span><span>The Pirate Bay</span><span class="c-3">waiting · 6 s</span></li>
        </ul>
        <div class="stack gap-3">${Array.from({ length: 3 }, () => `<div class="release"><div class="stack gap-3"><div class="skel skel-line" style="width:92%"></div><div class="skel skel-line" style="width:60%"></div><div class="row gap-2"><div class="skel" style="width:52px;height:22px"></div><div class="skel" style="width:60px;height:22px"></div><div class="skel" style="width:44px;height:22px"></div></div></div></div>`).join('')}</div>
      </div>`;
    }
    if (st.view === 'none') {
      return `<div class="empty" style="padding-block:var(--sp-8)">${ui.cat('peek', 120)}<h3>No releases for "${esc(st.q)}"</h3>
        <p>3 indexers searched. Release names are often shorter than titles.</p>
        <div class="stack gap-2" style="width:100%;max-width:340px">
          <button class="btn btn-secondary btn-block" data-act="ts-q" data-q="Frieren">${icon('search', { size: 17 })}Try "Frieren"</button>
          <button class="btn btn-secondary btn-block" data-act="ts-scope" data-v="any">${icon('layers', { size: 17 })}Search any pack, not just episode 29</button>
          <button class="btn btn-ghost btn-block" data-act="ts-q" data-q="Sousou no Frieren 29">Use the Japanese title</button>
        </div></div>`;
    }
    const { list, before, total } = results(ctx, st);
    const names = st.filters.map((f) => QUICK.find((q) => q[0] === f)?.[1] || f.split(':')[1]);
    if (!list.length && before) {
      return `<div class="empty" style="padding-block:var(--sp-8)">${ui.cat('blank', 96)}<h3>Your filters hide all ${total} results</h3>
        <p>${names.map((n) => `<strong class="c-1">${esc(n)}</strong>`).join(' and ')} together match nothing for this ${st.scope === 'season' ? 'season' : 'episode'}.</p>
        <div class="actions">${st.filters.map((f, i) => `<button class="fchip is-on" data-act="ts-filter" data-f="${f}">${esc(names[i])}<span class="x">${icon('x', { size: 12, stroke: 2.5 })}</span></button>`).join('')}</div>
        <button class="btn btn-secondary" data-act="ts-clear">Clear all filters</button></div>`;
    }
    if (!list.length) return body(ctx, { ...st, view: 'none' });
    const warn = st.view === 'partial' || st.partial ? `<div class="banner banner--warning">${icon('alertTriangle', { size: 18 })}<div class="grow"><strong>2 of 3 indexers answered</strong><span>The Pirate Bay timed out after 15 s. Results may be incomplete.</span></div><div class="actions"><button class="btn btn-secondary btn-sm" data-act="ts-retry">Retry</button></div></div>` : '';
    const pack = ctx.key === 'frieren' && st.scope === 'episode' ? `<div class="banner banner--accent">${icon('layers', { size: 18 })}<div class="grow"><strong>10 episodes are missing</strong><span>A pack gets 29–38 in one go. The best match below already does.</span></div><div class="actions"><button class="btn btn-secondary btn-sm" data-act="ts-scope" data-v="season">Season packs</button></div></div>` : '';
    const best = list.find((r) => r.best && st.sort === 'best');
    const rest = list.filter((r) => r !== best);
    const shown = list.length + (st.more ? 6 : 0);
    return `${warn}
      <p class="ts-status t-small c-3"><span>${total} results · ${ctx.key === 'frieren' ? 'Nyaa, TorrentLeech, The Pirate Bay' : '3 indexers'}</span>${ctx.also ? `<span class="ts-also">Also searched: ${ctx.also.map(esc).join(' · ')}</span>` : ''}</p>
      ${pack}
      ${best ? ui.releaseCard(best, { added: st.added === best.id }) : ''}
      ${rest.length ? `<h3 class="t-micro c-3 ts-sub">${best ? 'Other results' : 'Results'} · sorted by ${st.sort === 'best' ? 'best' : st.sort}</h3>` : ''}
      <div class="stack gap-3">${rest.map((r) => ui.releaseCard(r, { noBest: true, added: st.added === r.id })).join('')}${st.more ? MORE.map((r) => ui.releaseCard(r, { noBest: true })).join('') : ''}</div>
      <div class="ts-pager"><span class="t-small c-3 t-num">Showing ${shown} of ${total}</span>${shown < total ? `<button class="btn btn-secondary" data-act="ts-more">Load more</button>` : ''}</div>`;
  }
  const MORE = [
    R('x1', '[Judas] Sousou no Frieren - S01E29 [1080p][HEVC x265 10bit][Multi-Subs]', '402 MB', 0.4, 41, 2, '9 mo', 'Nyaa', ['1080p', 'x265', '10-bit', 'Judas'], 'episode', 74),
    R('x2', '[EMBER] Frieren (2023) (Season 1) [1080p] [HEVC WEBRip DDP] (Sousou no Frieren)', '19.4 GB', 19.4, 66, 7, '5 mo', 'Nyaa', ['1080p', 'WEBRip', 'HEVC', 'EMBER', 'Season pack'], 'season', 77),
  ];

  function head(ctx, st) {
    const active = st.filters.length;
    const scopeLabels = { episode: 'Episode', season: 'Season pack', any: 'Any' };
    return `<div class="sheet-title">
        <div class="stack" style="min-width:0"><span class="t-small c-3">Find a torrent · ${esc(ctx.label)}</span><h2 class="t-title-3 clamp-1" id="torrent-title">${esc(ctx.sub)}</h2></div>
        <button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close search (your query is kept)">${icon('x')}</button>
      </div>
      <form class="ts-form" role="search" data-ts-form>
        <div class="input-wrap grow">${icon('search', { size: 18 })}<input class="input" id="ts-q" name="q" value="${esc(st.q)}" autocomplete="off" enterkeyhint="search" aria-label="Search releases">
          <span class="input-end">${st.q ? `<button type="button" class="btn btn-ghost btn-sm btn-icon" data-act="ts-q" data-q="" aria-label="Clear search">${icon('x', { size: 16 })}</button>` : ''}</span></div>
      </form>
      ${ctx.scopes ? `<div class="seg seg--block" role="tablist" aria-label="Search scope">${ctx.scopes.map((s) => `<button role="tab" aria-selected="${st.scope === s}" data-act="ts-scope" data-v="${s}">${scopeLabels[s]}</button>`).join('')}</div>` : ''}
      <div class="ts-chips" role="toolbar" aria-label="Filters">
        <button class="fchip fchip--menu" data-act="ts-filters">${icon('sliders', { size: 16 })}Filters${active ? ` <span class="n">${active}</span>` : ''}</button>
        <button class="fchip fchip--menu ts-sortbtn" data-act="ts-sort" aria-label="Sort">${icon('arrowUpDown', { size: 16 })}<span class="sm-up">${{ best: 'Best', seeders: 'Seeders', size: 'Size', newest: 'Newest' }[st.sort]}</span></button>
        ${QUICK.slice(0, 4).map(([f, l]) => `<button class="fchip ${f.startsWith('hdr') || f.startsWith('codec') ? 'xs-hide' : ''}" aria-pressed="${st.filters.includes(f)}" data-act="ts-filter" data-f="${f}" ${facetCount(ctx, f) === 0 && !st.filters.includes(f) ? 'disabled' : ''}>${l} <span class="n">${facetCount(ctx, f)}</span></button>`).join('')}
      </div>`;
  }

  // Session memory: query, scope and filters survive close/reopen
  function stateFor(ctx, p) {
    const saved = app.state.ts && app.state.ts[ctx.key];
    const st = saved ? { ...saved } : { q: ctx.q, scope: p.scope || (ctx.scopes ? 'episode' : null), filters: [], sort: 'best', view: 'results' };
    if (p.scope && !saved) st.scope = p.scope;
    if (p.state) { st.view = p.state; if (p.state === 'filtered-empty') st.filters = ['res:2160p', 'hdr:dv']; if (p.state === 'partial') st.partial = true; }
    st.restored = !!saved;
    return st;
  }
  function persist(ctx, st) { app.state.ts = app.state.ts || {}; app.state.ts[ctx.key] = { q: st.q, scope: st.scope, filters: st.filters, sort: st.sort, view: 'results' }; app.save(); }

  app.sheet('torrent', (p) => {
    const ctx = context(p);
    const st = stateFor(ctx, p);
    let root;
    const draw = (part) => {
      if (!root) return;
      if (part !== 'body') { root.querySelector('.ts-head').innerHTML = head(ctx, st); wireForm(); }
      root.querySelector('.ts-body').innerHTML = body(ctx, st);
      persist(ctx, st);
    };
    const search = () => { st.view = 'searching'; draw('body'); setTimeout(() => { st.view = /zzz/.test(st.q) ? 'none' : 'results'; draw('body'); }, 1100); };
    function wireForm() {
      const f = root.querySelector('[data-ts-form]');
      f.addEventListener('submit', (e) => { e.preventDefault(); st.q = f.q.value; search(); });
    }
    app._ts = { ctx, st, draw, search };
    return {
      cls: 'sheet--full ts-sheet',
      html: `<div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head ts-head">${head(ctx, st)}</div>
        <div class="sheet-body ts-body" aria-live="polite">${st.restored && st.view === 'results' ? `<p class="t-small c-3 ts-restored">${icon('history', { size: 14 })}Picked up where you left off</p>` : ''}${body(ctx, st)}</div>`,
      mount(el) { root = el; wireForm(); },
    };
  });

  // Nested: full filters with facet counts
  app.sheet('ts-filters', () => {
    const { ctx, st } = app._ts;
    const F = D.facets;
    const group = (title, key, map, prefix) => `<fieldset class="ts-fgroup"><legend class="label">${title}</legend><div class="fchips">${Object.entries(map).map(([k, n]) => {
      const f = `${prefix}:${k.split(' ')[0].replace('Dolby', 'dv').replace('HDR10', 'hdr10')}`;
      return `<button class="fchip" aria-pressed="${st.filters.includes(f) || (prefix === 'res' && st.filters.includes(`res:${k}`))}" data-act="ts-filter2" data-f="${prefix === 'res' ? `res:${k}` : f}" ${n === 0 ? 'disabled' : ''}>${esc(k)} <span class="n">${n}</span></button>`;
    }).join('')}</div></fieldset>`;
    return {
      cls: 'sheet--nested',
      html: `<div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="ts-filters-title">Filters</h2><button class="btn btn-ghost btn-sm" data-act="ts-clear2">Reset</button></div></div>
        <div class="sheet-body stack gap-6">
          ${group('Resolution', 'resolution', F.resolution, 'res')}
          ${group('Source', 'source', F.source, 'src')}
          ${group('Codec', 'codec', F.codec, 'codec')}
          ${group('HDR / Dolby Vision', 'hdr', F.hdr, 'hdr')}
          ${group('Audio', 'audio', F.audio, 'audio')}
          ${group('Pack type', 'pack', F.pack, 'pack')}
          <fieldset class="ts-fgroup"><legend class="label">Size per episode</legend><div class="range"><div class="range-track"><div class="range-fill" style="left:8%;right:35%"></div><div class="range-thumb" style="left:8%" role="slider" tabindex="0" aria-label="Minimum size" aria-valuetext="200 MB"></div><div class="range-thumb" style="left:65%" role="slider" tabindex="0" aria-label="Maximum size" aria-valuetext="4 GB"></div></div><div class="range-values"><span>200 MB</span><span>4 GB</span></div></div></fieldset>
          <fieldset class="ts-fgroup"><legend class="label">Minimum seeders</legend>${ui.seg([['0', 'Any'], ['5', '5+'], ['20', '20+'], ['100', '100+']], '5', 'aria-label="Minimum seeders"')}</fieldset>
          ${group('Indexer', 'indexer', F.indexer, 'idx')}
          ${group('Release group', 'group', F.group, 'grp')}
        </div>
        <div class="sheet-foot"><button class="btn btn-secondary" data-act="close">Cancel</button><button class="btn btn-primary grow" data-act="ts-apply">Show ${results(ctx, st).total} results</button></div>`,
    };
  });

  app.sheet('ts-sort', () => {
    const { st } = app._ts;
    return {
      cls: 'sheet--nested sheet--narrow',
      html: `<div class="sheet-grabber" aria-hidden="true"></div><div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="ts-sort-title">Sort by</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
        <div class="sheet-body stack gap-2">${[['best', 'Best match', 'Quality profile, health and size together'], ['seeders', 'Seeders', 'Fastest to start'], ['size', 'Size', 'Smallest first'], ['newest', 'Newest', 'Most recently published']].map(([v, l, d]) => `<label class="radio-card"><input type="radio" name="sort" value="${v}" ${st.sort === v ? 'checked' : ''} data-act="ts-sortpick"><span class="stack"><span class="w-semibold">${l}</span><span class="t-small c-2">${d}</span></span></label>`).join('')}</div>`,
    };
  });

  // Nested: Inspect = dry-run mapping preview before adding
  app.sheet('ts-inspect', (p) => {
    const { ctx } = app._ts;
    const r = [...ctx.set, ...MORE].find((x) => x.id === p.rid) || ctx.set[0];
    const isFrieren = ctx.key === 'frieren';
    const M = D.mapping.groups;
    const rows = isFrieren
      ? `<h3 class="t-micro c-3">Will map · 9 episodes</h3><ul class="insp-list">${M.matched.slice(0, 4).map((m) => `<li>${ui.path(m.file)}<span class="insp-to">${icon('arrowRight', { size: 14 })}<b>S01E${m.target.e}</b> ${esc(m.target.name)}</span></li>`).join('')}<li class="t-small c-3">+ 5 more, all high confidence</li></ul>
         <h3 class="t-micro c-3">You'll review · 2 files</h3><ul class="insp-list">${M.needs_review.map((m) => `<li>${ui.path(m.file)}<span class="insp-to review">${icon('alertCircle', { size: 14 })}${esc(m.reason)}</span></li>`).join('')}</ul>
         <h3 class="t-micro c-3">Specials · 1 &nbsp;·&nbsp; Ignored · 3</h3><ul class="insp-list"><li>${ui.path(M.specials[0].file)}<span class="insp-to">${icon('arrowRight', { size: 14 })}<b>S00E06</b> Specials</span></li>${M.extras.map((m) => `<li class="is-ignored">${ui.path(m.file)}<span class="insp-to">${esc(m.kind)}</span></li>`).join('')}</ul>`
      : `<h3 class="t-micro c-3">Will map · 1 file</h3><ul class="insp-list"><li>${ui.path(r.raw + '.mkv')}<span class="insp-to">${icon('arrowRight', { size: 14 })}<b>${ctx.t.type === 'movie' ? esc(ctx.t.title) : 'S01E01–E13'}</b></span></li><li class="is-ignored">${ui.path('Sample/sample.mkv')}<span class="insp-to">Sample · ignored</span></li></ul>`;
    return {
      cls: 'sheet--nested',
      html: `<div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head"><div class="sheet-title"><div class="stack"><span class="t-small c-3">Inspect · nothing downloads yet</span><h2 class="t-title-3" id="ts-inspect-title">${isFrieren ? '16 files · 13.2 GB' : '2 files · ' + r.size}</h2></div><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Back to results">${icon('x')}</button></div></div>
        <div class="sheet-body stack gap-4">
          ${ui.releaseName(r.raw)}
          <div class="steps steps--inline"><div class="step is-done"><span class="ic">${icon('check', { size: 12, stroke: 3 })}</span>Fetched file list</div><div class="step is-done"><span class="ic">${icon('check', { size: 12, stroke: 3 })}</span>Matched files</div><div class="step is-done"><span class="ic">${icon('check', { size: 12, stroke: 3 })}</span>Asked AI about 3 tricky ones</div></div>
          ${rows}
        </div>
        <div class="sheet-foot"><button class="btn btn-secondary" data-act="close">Back</button><button class="btn btn-primary grow" data-act="add-release" data-rid="${r.id}">${icon('plus', { size: 18 })}${isFrieren ? 'Add & review mapping' : 'Add to library & download'}</button></div>`,
    };
  });

  Object.assign(app.actions, {
    'ts-scope': (b) => { const s = app._ts.st; s.scope = b.dataset.v; s.view = 'results'; app._ts.draw(); },
    'ts-filter': (b) => { const s = app._ts.st; const f = b.dataset.f; s.filters = s.filters.includes(f) ? s.filters.filter((x) => x !== f) : [...s.filters, f]; s.view = 'results'; app._ts.draw(); },
    'ts-filter2': (b) => { b.setAttribute('aria-pressed', b.getAttribute('aria-pressed') !== 'true'); },
    'ts-apply': () => { const s = app._ts.st; const on = [...document.querySelectorAll('[data-act="ts-filter2"][aria-pressed="true"]')].map((x) => x.dataset.f).filter((f) => /^(res|codec|hdr|pack|src)/.test(f)); s.filters = on; app.close(); app._ts.draw(); },
    'ts-clear': () => { app._ts.st.filters = []; app._ts.draw(); },
    'ts-clear2': () => document.querySelectorAll('[data-act="ts-filter2"]').forEach((x) => x.setAttribute('aria-pressed', 'false')),
    'ts-filters': () => app.openSheet('ts-filters'),
    'ts-sort': () => app.openSheet('ts-sort'),
    'ts-sortpick': (b) => { app._ts.st.sort = b.value; app.close(); app._ts.draw(); },
    'ts-q': (b) => { app._ts.st.q = b.dataset.q; app._ts.draw(); if (b.dataset.q) app._ts.search(); else document.getElementById('ts-q')?.focus(); },
    'ts-retry': () => { app._ts.st.partial = false; app._ts.search(); },
    'ts-more': (b) => { b.classList.add('is-loading'); setTimeout(() => { app._ts.st.more = true; app._ts.draw('body'); }, 700); },
    inspect: (b) => app.openSheet('ts-inspect', { rid: b.dataset.rid }),
    'add-release': (b) => {
      const { ctx, st } = app._ts;
      b.classList.add('is-loading');
      setTimeout(() => {
        if (ctx.key === 'frieren') { app.state.frieren = 'mapping'; app.save(); app.closeAll(); app.go('#/mapping?flow=1'); return; }
        st.added = b.dataset.rid; app.closeAll();
        if (ctx.t.type === 'movie') app.toast(null, { kind: 'ready', title: 'Added · downloading', body: 'It streams to Infuse while it downloads.', action: 'Activity', href: '#/activity' });
        else app.toast(null, { kind: 'ready', title: 'Added · mapped 13 of 13 episodes', body: 'Season 1 starts downloading now.', action: 'View', href: `#/title/tv/${ctx.t.id}` });
      }, 700);
    },
  });
})();
