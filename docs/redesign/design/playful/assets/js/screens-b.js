/* Screens: Movie detail · Show detail · Torrent search sheet (+ filters, inspect) · small sheets */
(function () {
  const U = UI;
  const { icon, momo, btn, iconBtn, chip, esc } = U;
  const D = DATA;
  const T = (id) => D.T[id];
  const pad = (n) => String(n).padStart(2, '0');
  const fmtRun = (m) => (m >= 60 ? `${Math.floor(m / 60)}h ${m % 60}m` : `${m} min`);
  const vfsName = (t) => `${t.title.replace(/:/g, '-')} (${t.year})`;

  // ------------------------------------------------------------------ shared hero
  const hero = (t, o = {}) => `<section class="hero">
      <div class="hero-bg" aria-hidden="true">${ART.backdrop(t.id)}</div>
      <div class="hero-inner page">
        <div class="hero-bar">${iconBtn('arrow-left', 'Back', { act: 'back', cls: 'glass' })}<span class="spacer"></span>${iconBtn('share-2', 'Share', { cls: 'glass', act: 'toast', arg: 'Link copied' })}${t.trailer ? iconBtn('square-play', `Play trailer (${t.trailer})`, { cls: 'glass hide-desk', act: 'trailer' }) : ''}</div>
        <div class="hero-grid">
          <div class="hero-poster">${U.poster(t.id)}</div>
          <div class="hero-info">
            <p class="eyebrow">${t.type === 'movie' ? 'Movie' : `Show · ${t.network || ''}`}</p>
            <h1 class="t-display hero-title">${esc(t.title)}</h1>
            <p class="hero-meta">${[t.years || t.year, t.runtime ? (t.type === 'movie' ? fmtRun(t.runtime) : `${t.runtime} min episodes`) : null, t.rating ? `<span class="rating">${icon('star', 'ico ico-xs')}${t.rating}</span>` : null, t.status && t.type === 'show' ? t.status : null].filter(Boolean).join('<span class="sep">·</span>')}</p>
            <div class="chips hero-genres">${t.genres.map((g) => `<a class="tag" href="#/discover?genres=${encodeURIComponent(g)}">${g}</a>`).join('')}${t.anime ? '<span class="tag hi">Anime</span>' : ''}</div>
            <p class="hero-credit">${t.director ? `Directed by <b>${esc(t.director)}</b>` : t.creator ? `Created by <b>${esc(t.creator)}</b>` : ''}</p>
            ${t.trakt ? `<p class="hero-trakt">${icon('check-check', 'ico ico-sm')}${t.trakt.watched ? `Watched ${t.trakt.watched}` : esc(t.trakt.progress || '')}${t.trakt.rating ? ` · You rated it <b>${t.trakt.rating}/10</b>` : ''}</p>` : ''}
            <div class="row gap-2 wrap hero-actions">${o.actions || ''}${t.trailer ? btn(`Trailer · ${t.trailer}`, { variant: 'ghost', icon: 'square-play', act: 'trailer', cls: 'hide-phone' }) : ''}</div>
          </div>
        </div>
      </div>
    </section>`;

  const castRow = (t) => `<section class="block" aria-labelledby="cast-h"><h2 class="block-title" id="cast-h">Cast</h2>
      <ul class="cast">${(t.cast || []).map(([n, r]) => `<li><span class="cast-face">${ART.face(n)}</span><b>${esc(n)}</b><span>${esc(r)}</span></li>`).join('')}</ul></section>`;
  const whyCard = (t) => (t.reason ? `<section class="why" aria-labelledby="why-h"><span class="why-av">${momo('avatar')}</span><div><h2 id="why-h" class="t-label">Why Momo picked it</h2><p>${esc(t.reason)}</p></div></section>` : '');
  const moreLike = (ids) => U.shelf('More like this', null, ids.map((id) => U.posterCard(id)).join(''));

  const pathCard = (t, file) => {
    const p = t.type === 'movie' ? `/Movies/${vfsName(t)}/${vfsName(t)}.mkv` : file;
    return `<div class="path"><span class="t-micro ink-3">WebDAV path</span><code class="machine">${U.breakable(p)}</code>${btn('Copy path', { variant: 'secondary', size: 'sm', icon: 'copy', act: 'toast', arg: 'path' })}</div>`;
  };

  // availability state machine: Add → Find a torrent → Downloading → Ready in Infuse
  const availCard = (t, st) => {
    const steps = ['Add', 'Torrent', 'Download', 'Ready'];
    const at = { none: 0, added: 1, downloading: 2, ready: 3 }[st];
    const track = `<ol class="avail-steps" aria-label="Progress: ${steps[at]}">${steps.map((s, i) => `<li class="${i < at ? 'done' : i === at ? 'now' : ''}">${icon(i < at ? 'check' : 'paw-print', 'ico ico-xs')}<span>${s}</span></li>`).join('')}</ol>`;
    let body;
    if (st === 'none') body = `<p class="avail-head">${icon('bookmark', 'ico')}Not in your library</p><p class="ink-2 t-callout">Add it and momoshtrem will keep a folder ready for it in Infuse.</p>${btn('Add to library', { variant: 'primary', block: true, icon: 'plus', act: 'movie-state', arg: `${t.id}:added`, flow: 'add-lib' })}`;
    if (st === 'added') body = `<p class="avail-head">${icon('magnet', 'ico')}In your library, no file yet</p><p class="ink-2 t-callout">Pick a release and Momo assigns it. It’s playable in Infuse within a minute or two.</p>${btn('Find a torrent', { variant: 'primary', block: true, icon: 'search', act: 'tsearch', arg: t.id, flow: 'find-movie' })}`;
    if (st === 'downloading') body = `<div class="avail-dl">${U.ring(t.dl || 63, 64)}<div><p class="avail-head">Preparing… ${t.dl || 63}%</p><p class="ink-2 t-callout">${icon('arrow-down', 'ico ico-xs')} 5.2 MB/s · about 7 min left. You can start watching in Infuse now; it streams while it downloads.</p></div></div><div class="row gap-2">${btn('View in Activity', { variant: 'secondary', size: 'sm', href: '#/activity', icon: 'activity' })}${btn('Change torrent', { variant: 'ghost', size: 'sm', act: 'tsearch', arg: t.id })}</div>`;
    if (st === 'ready') body = `<p class="avail-head ready">${icon('tv-minimal-play', 'ico')}Ready in Infuse</p><p class="ink-2 t-callout">Open <b>Infuse → momoshtrem → Movies</b> on your Apple TV.</p>${btn('How to play', { variant: 'primary', block: true, icon: 'tv', act: 'howto', arg: t.id })}${pathCard(t)}`;
    return `<section class="avail card card-pad ${st}" aria-label="Availability">${track}${body}</section>`;
  };

  const torrentCard = (t, o = {}) => (t.torrent ? `<section class="card card-pad tcard" aria-labelledby="tc-h">
      <div class="row gap-2"><h2 class="block-title grow" id="tc-h">${o.title || 'Current torrent'}</h2>${iconBtn('ellipsis', 'Torrent options', { cls: 'sm', act: 'tmenu', arg: t.id })}</div>
      <p class="machine tcard-name">${U.machine(t.torrent.name)}</p>
      <div class="tags">${t.torrent.q.map((x) => U.tag(x, /2160p|DV|Atmos/.test(x) ? 'hi' : '')).join('')}</div>
      <dl class="tcard-stats"><div><dt>Size</dt><dd>${t.torrent.size}</dd></div><div><dt>Health</dt><dd><span class="dot ready"></span>${t.torrent.health}</dd></div>${t.torrent.progress ? `<div><dt>Downloaded</dt><dd>${t.torrent.progress}%</dd></div>` : ''}</dl>
      <div class="row gap-2 wrap">${btn('Replace', { variant: 'secondary', size: 'sm', icon: 'replace', act: 'tsearch', arg: t.id })}${btn('Activity', { variant: 'ghost', size: 'sm', href: '#/activity' })}</div>
    </section>` : '');

  const subsPanel = (t) => `<section class="card card-pad subs" aria-labelledby="subs-h">
      <h2 class="block-title" id="subs-h">Subtitles</h2>
      <ul class="subs-list">${(t.subs || ['English']).map((l, i) => `<li>${icon('captions', 'ico ico-sm')}<span class="grow"><b>${l}</b><small>${i === 0 ? 'Matches FLUX WEB-DL · OpenSubtitles' : 'Embedded in the release'}</small></span>${iconBtn('trash-2', `Remove ${l} subtitles`, { cls: 'sm', act: 'toast', arg: `${l} subtitles removed` })}</li>`).join('')}</ul>
      <div class="row gap-2">${btn('Search OpenSubtitles', { variant: 'secondary', size: 'sm', icon: 'search', act: 'subs' })}<label class="sr-only" for="sublang">Language</label><select class="input sub-lang" id="sublang"><option>English</option><option>Azerbaijani</option><option>Russian</option><option>Turkish</option></select></div>
    </section>`;

  // ================================================================== 6 · Movie detail
  SCREENS.movie = (ctx) => {
    const t = T(ctx.arg) || T('dune-part-two');
    const st = ctx.state || S.movieState[t.id] || (S.added[t.id] ? 'added' : t.lib) || 'none';
    const tt = { ...t, torrent: st === 'downloading' ? t.torrent || T('perfect-days').torrent : st === 'ready' ? t.torrent || T('dune-part-two').torrent : null };
    return {
      title: t.title, nav: null,
      html: `${hero(t)}
        <div class="page detail">
          <div class="detail-cols">
            <div class="col col-main">
              <section class="block o-2"><h2 class="sr-only">Overview</h2><p class="overview">${esc(t.overview)}</p></section>
              <div class="o-3">${whyCard(t)}</div>
              <div class="o-5">${castRow(t)}</div>
            </div>
            <aside class="col col-side">
              <div class="o-1">${availCard(tt, st)}</div>
              <div class="o-4">${torrentCard(tt)}</div>
              <div class="o-6">${st === 'ready' ? subsPanel(t) : ''}</div>
            </aside>
          </div>
          ${moreLike(['arrival', 'interstellar', 'dune-part-three', 'the-odyssey', 'prisoners', 'heat'].filter((x) => x !== t.id))}
        </div>`,
    };
  };
  ACT['movie-state'] = (arg) => { const [id, st] = arg.split(':'); S.movieState[id] = st; if (st === 'added') S.added[id] = true; render(); toast({ text: `${T(id).title} added to your library`, sub: 'Next: find a torrent.', momo: 'proud' }); };
  ACT.trailer = () => toast({ text: 'Trailer opens on YouTube', icon: 'square-play' });

  // ================================================================== 7 · Show detail
  const seasonLabel = (s) => (s.n === 0 ? 'Specials' : `Season ${s.n}`);
  const findEp = (t, s, n) => t.seasons?.find((x) => x.n === s)?.eps.find((e) => e.n === n);
  SCREENS.show = (ctx) => {
    const t = T(ctx.arg) || T('severance');
    const p = ctx.p;
    // apply flow results
    if (t.id === 'frieren' && S.frierenReady) t.seasons[1].eps.forEach((e) => { if (e.n > 28) e.state = 'ready'; });
    const abs = t.ordering === 'Absolute' && !t.seasons;
    const next = t.next;
    const seasons = t.seasons || [];
    const sn = p.has('s') ? Number(p.get('s')) : next?.s ?? seasons[seasons.length - 1]?.n ?? 1;
    const season = seasons.find((s) => s.n === sn) || seasons[0];
    const nextEp = next && !abs ? findEp(t, next.s, next.n) : null;

    // primary card: up next for this show
    let up = '';
    if (abs) {
      up = `<section class="avail card card-pad"><p class="eyebrow">Up next</p><h2 class="t-title-3">Episode 1160</h2><p class="ink-2 t-callout">Elbaph arc · ${U.pill('airs', 'Airs Sun, Oct 18', { sm: true })}</p><p class="ink-2 t-callout">You’re caught up. Momo will look for a release on Sunday evening.</p></section>`;
    } else if (nextEp) {
      const st = nextEp.state;
      up = `<section class="avail card card-pad ${st === 'ready' ? 'ready' : ''}" aria-label="Up next">
        <div class="up-mini">${U.wide(ART.still(t.id, nextEp.n, ''))}<div class="grow"><p class="eyebrow">Up next</p><h2 class="t-headline">${next.s === 0 ? 'Special' : `S${pad(next.s)}E${pad(next.n)}`} · ${esc(nextEp.title)}</h2>${U.pill(st === 'ready' ? 'ready' : st === 'review' ? 'review' : st === 'downloading' ? 'downloading' : 'needs_torrent', null, { sm: true, pct: nextEp.progress })}</div></div>
        ${st === 'ready' ? `${btn('How to play', { variant: 'primary', block: true, icon: 'tv', act: 'howto', arg: t.id })}${pathCardShow(t, nextEp)}` : st === 'review' ? btn('Review mapping', { variant: 'primary', block: true, icon: 'scan-search', href: `#/review/${t.id}` }) : st === 'downloading' ? `<p class="ink-2 t-callout">${nextEp.progress}% · streams while it downloads.</p>` : btn('Find a torrent', { variant: 'primary', block: true, icon: 'search', act: 'tsearch', arg: `${t.id}:${next.s}:${next.n}` })}
      </section>`;
    }

    // episode list (compact rows, ranges, collapsed watched)
    let list = '';
    if (abs) {
      const arc = p.get('arc') ? t.arcs.find((a) => a[0] === p.get('arc')) : t.arcs[t.arcs.length - 1];
      const [name, from, to] = arc;
      const page = Number(p.get('page') || 0);
      const per = 40;
      const start = Math.max(from, to - per * (page + 1) + 1);
      const eps = [];
      for (let n = to; n >= start; n--) eps.push({ s: 22, n: n - 1155, abs: n, title: `Episode ${n}`, state: n === 1160 ? 'upcoming' : 'watched', airs: 'Airs Sun', runtime: 24 });
      list = `<div class="ep-tools">
          <label class="sr-only" for="arc">Arc</label><select class="input arc-select" id="arc" data-act="noop" onchange="setParams({arc:this.value,page:null},true)">${t.arcs.map(([a, f, to2]) => `<option value="${a}" ${a === name ? 'selected' : ''}>${a} · ${f}–${to2}</option>`).join('')}</select>
          <div class="input-wrap jump">${icon('hash', 'ico ico-sm')}<label class="sr-only" for="jump">Jump to episode</label><input class="input" id="jump" inputmode="numeric" placeholder="Jump to episode" value="${p.get('jump') || ''}"></div>
        </div>
        <div class="arc-strip" role="group" aria-label="Arcs">${t.arcs.slice(-6).map(([a, f, to2]) => `<button class="arc ${a === name ? 'on' : ''}" data-act="arc" data-arg="${a}"><b>${a}</b><span>${f}–${to2}</span></button>`).join('')}</div>
        <p class="t-caption ink-3 list-note">${name} · episodes ${from}–${to} · newest first</p>
        <ol class="ep-list">${eps.map((e) => U.epRow(e, t.id, { abs: true, next: e.abs === 1160 })).join('')}</ol>
        ${start > from ? `<button class="btn btn-secondary btn-block more-eps" data-act="eps-more">Show ${Math.min(per, start - from)} older episodes</button>` : ''}`;
    } else if (season) {
      const showWatched = p.get('watched') === '1';
      const eps = season.eps;
      const watched = eps.filter((e) => e.state === 'watched');
      const long = eps.length > 20;
      const ranges = long ? [[1, 12], [13, 24], [25, eps.length]] : null;
      const range = ranges ? (p.get('r') ? ranges[Number(p.get('r'))] : ranges.find(([a, b]) => next && next.s === season.n && next.n >= a && next.n <= b) || ranges[0]) : null;
      let visible = range ? eps.filter((e) => e.n >= range[0] && e.n <= range[1]) : eps;
      const hidden = !showWatched && watched.length > 3 ? visible.filter((e) => e.state === 'watched') : [];
      if (hidden.length) visible = visible.filter((e) => e.state !== 'watched');
      const missing = eps.filter((e) => e.state === 'missing').length;
      const ready = eps.filter((e) => e.state === 'ready' || e.state === 'watched').length;
      list = `<div class="season-head">
          <div class="grow"><h3 class="t-title-3">${seasonLabel(season)}</h3><p class="t-caption ink-3">${eps.length} episodes · ${ready} ready${missing ? ` · ${missing} missing` : ''}${season.year ? ` · ${season.year}` : ''}</p></div>
          <div class="row gap-2 wrap">${missing ? btn(`Find missing (${missing})`, { variant: 'secondary', size: 'sm', icon: 'search', act: 'tsearch', arg: `${t.id}:${season.n}:missing` }) : ''}${btn('Season pack', { variant: 'ghost', size: 'sm', icon: 'package', act: 'tsearch', arg: `${t.id}:${season.n}:pack` })}</div>
        </div>
        ${ranges ? `<div class="ep-tools"><div class="seg ranges" role="group" aria-label="Episode range">${ranges.map(([a, b], i) => `<button aria-pressed="${range[0] === a}" data-act="range" data-arg="${i}">${a}–${b}</button>`).join('')}</div>
          <div class="input-wrap jump">${icon('hash', 'ico ico-sm')}<label class="sr-only" for="jump">Jump to episode</label><input class="input" id="jump" inputmode="numeric" placeholder="Jump to…"></div></div>` : ''}
        ${hidden.length ? `<button class="collapsed-row" data-act="show-watched">${icon('check-check', 'ico ico-sm')}<span class="grow">${hidden.length} watched episode${hidden.length > 1 ? 's' : ''} (E${pad(hidden[0].n)}–E${pad(hidden[hidden.length - 1].n)})</span><span class="link-btn">Show</span></button>` : ''}
        <ol class="ep-list">${visible.map((e) => U.epRow(e, t.id, { next: next && next.s === season.n && next.n === e.n })).join('')}</ol>
        ${showWatched && watched.length > 3 ? `<button class="btn btn-ghost btn-sm" data-act="hide-watched">${icon('chevron-up')}Hide watched</button>` : ''}`;
    }

    const seasonTabs = seasons.length > 1 ? U.tabs(seasons.map((s) => [String(s.n), s.n === 0 ? 'Specials' : `Season ${s.n}`, s.n === 0 ? s.eps.length : undefined]), String(sn), { act: 'season', label: 'Seasons', scroll: true }) : '';
    const reviewBanner = t.review || (t.id === 'frieren' && false) ? `<div class="o-0">${U.banner('review', 'scan-search', `${t.review.count} files need a quick look`, 'Some specials matched with low confidence. Momo won’t guess with your library.', btn('Review', { variant: 'secondary', size: 'sm', href: `#/review/${t.review.job}`, flow: 'review-banner' }))}</div>` : '';
    const sceneNote = t.sceneNote && !S.frierenReady ? `<p class="scene-note">${icon('info', 'ico ico-sm')}${esc(t.sceneNote)}</p>` : '';

    return {
      title: t.title, nav: null,
      html: `${hero(t)}
        <div class="page detail">
          <div class="detail-cols">
            <div class="col col-main">
              ${reviewBanner}
              <section class="block o-3 episodes" aria-labelledby="eps-h">
                <div class="row gap-2"><h2 class="block-title grow" id="eps-h">Episodes</h2>${t.ordering ? `<span class="pill pill-neutral sm">${icon('list-ordered', 'ico')}${t.ordering} order</span>` : ''}${iconBtn('settings', 'Show settings', { act: 'show-settings', arg: t.id, cls: 'sm' })}</div>
                ${seasonTabs}${sceneNote}
                ${list}
              </section>
              <section class="block o-4"><h2 class="block-title">About</h2><p class="overview">${esc(t.overview)}</p></section>
              <div class="o-5">${castRow(t)}</div>
            </div>
            <aside class="col col-side">
              <div class="o-1">${up}</div>
              <div class="o-6">${torrentCard(t, { title: 'Torrents for this show' })}</div>
            </aside>
          </div>
          ${moreLike(['silo', 'dark', 'station-eleven', 'the-leftovers', 'devs', 'maniac'].filter((x) => x !== t.id))}
        </div>`,
      after: () => { const j = document.getElementById('jump'); if (j) j.addEventListener('keydown', (e) => { if (e.key === 'Enter') toast({ text: `Jumped to episode ${j.value}`, icon: 'hash' }); }); },
    };
  };
  function pathCardShow(t, e) {
    return `<div class="path"><span class="t-micro ink-3">WebDAV path</span><code class="machine">${U.breakable(`/TV Shows/${vfsName(t)}/Season ${pad(e.s)}/${t.title.replace(/:/g, '-')} - S${pad(e.s)}E${pad(e.n)} - ${e.title}.mkv`)}</code>${btn('Copy path', { variant: 'secondary', size: 'sm', icon: 'copy', act: 'toast', arg: 'path' })}</div>`;
  }
  Object.assign(ACT, {
    season: (v) => setParams({ s: v, r: null, watched: null }, true),
    range: (i) => setParams({ r: i }, true),
    'show-watched': () => setParams({ watched: '1' }, true),
    'hide-watched': () => setParams({ watched: null }, true),
    arc: (a) => setParams({ arc: a, page: null }, true),
    'eps-more': () => setParams({ page: Number(parseRoute().params.get('page') || 0) + 1 }, true),
    'show-settings': (id) => setParams({ sheet: 'showsettings', sid: id }),
    'ep-menu': (arg) => setParams({ sheet: 'epmenu', ep: arg }),
    tmenu: () => setParams({ sheet: 'tmenu' }),
    howto: (id) => setParams({ sheet: 'howto', hid: id }),
    subs: () => setParams({ sheet: 'subs' }),
  });

  SHEETS.showsettings = (ctx) => {
    const t = T(ctx.p.get('sid')) || T('frieren');
    return U.sheet({ id: 'ss', title: 'Show settings', sub: esc(t.title), body: `<div class="stack gap-6">
      <fieldset class="fgroup"><legend>Episode ordering</legend>${U.seg([['aired', 'Aired'], ['absolute', 'Absolute'], ['group', 'Episode group']], t.anime ? 'absolute' : 'aired', { block: true, label: 'Episode ordering' })}
        <p class="field-hint">Absolute numbers episodes 1, 2, 3… across seasons, like most anime releases. An episode group follows a TMDB alternative order.</p>
        <label class="sr-only" for="eg">Episode group</label><select class="input" id="eg" disabled><option>TMDB group: “Seasons (production order)”</option></select></fieldset>
      <label class="switch-row"><span class="grow"><b>Include specials</b><small>Shown as “Specials”; in Infuse as Season 00.</small></span><input type="checkbox" class="switch" checked aria-label="Include specials"></label>
      <fieldset class="fgroup"><legend>Matching</legend>${U.seg([['auto', 'Auto'], ['rules', 'Rules only'], ['ai', 'Always ask AI']], 'auto', { block: true, label: 'Classifier mode' })}<p class="field-hint">Auto uses rules first and asks AI only about tricky files.</p></fieldset>
      ${btn('Re-run mapping for this show', { variant: 'secondary', icon: 'rotate-cw', block: true, href: `#/review/${t.id}` })}
    </div>`, foot: `<span class="spacer"></span>${btn('Done', { variant: 'primary', act: 'close-sheet' })}` });
  };
  SHEETS.epmenu = (ctx) => {
    const [show, s, n] = (ctx.p.get('ep') || 'severance:2:6').split(':');
    const t = T(show), e = findEp(t, Number(s), Number(n)) || { title: '', s, n };
    return U.sheet({ id: 'em', title: `${Number(s) === 0 ? 'Special' : `S${pad(s)}E${pad(n)}`} · ${esc(e.title)}`, sub: esc(t.title), cls: 'menu-sheet', body: `<div class="stack gap-1">
      <button class="menu-item" data-act="howto" data-arg="${show}">${icon('tv')}How to play</button>
      <button class="menu-item" data-act="toast" data-arg="path">${icon('copy')}Copy WebDAV path</button>
      <button class="menu-item" data-act="tsearch" data-arg="${show}:${s}:${n}">${icon('replace')}Replace file</button>
      <button class="menu-item" data-act="toast" data-arg="Subtitles: searching OpenSubtitles">${icon('captions')}Subtitles</button>
      <div class="menu-sep"></div>
      <button class="menu-item danger" data-act="confirm-unassign" data-arg="${show}:${s}:${n}">${icon('unplug')}Unassign file…</button></div>` });
  };
  ACT['confirm-unassign'] = () => { setParams({ sheet: null }); S.dialog = () => U.dialog('Unassign this file?', 'The episode goes back to “Missing”. The torrent stays in Activity, so you can reassign it later.', `${btn('Cancel', { act: 'close-dialog' })}${btn('Unassign', { variant: 'danger-solid', act: 'close-dialog' })}`); render(); };
  SHEETS.tmenu = () => U.sheet({ id: 'tm', title: 'Torrent options', cls: 'menu-sheet', body: `<div class="stack gap-1">
      <button class="menu-item" data-act="toast" data-arg="magnet">${icon('magnet')}Copy magnet link</button>
      <a class="menu-item" href="#/activity/b1c9e4">${icon('activity')}Open in Activity</a>
      <a class="menu-item" href="#/review/severance">${icon('scan-search')}Review mapping</a>
      <div class="menu-sep"></div>
      <button class="menu-item danger" data-act="confirm-remove-torrent">${icon('trash-2')}Remove torrent…</button></div>` });
  ACT['confirm-remove-torrent'] = () => { setParams({ sheet: null }); S.dialog = () => U.dialog('Remove this torrent?', 'Episodes that use it go back to “Missing”. Your library entries and watch history stay.', `${btn('Cancel', { act: 'close-dialog' })}${btn('Remove torrent', { variant: 'danger-solid', act: 'close-dialog' })}`, { extra: '<label class="check" style="margin-top:12px"><input type="checkbox"> Also delete downloaded data (41.3 GB)</label>' }); render(); };
  SHEETS.howto = (ctx) => {
    const t = T(ctx.p.get('hid')) || T('dune-part-two');
    return U.sheet({ id: 'ht', title: 'Play it in Infuse', sub: esc(t.title), body: `<div class="howto">
      <div class="howto-art">${momo('proud')}</div>
      <ol class="howto-steps">
        <li><span class="check-num">1</span><div><b>Open Infuse on your Apple TV</b><span>It’s already connected to momoshtrem over WebDAV.</span></div></li>
        <li><span class="check-num">2</span><div><b>Go to ${t.type === 'movie' ? 'Movies' : 'TV Shows'} → ${esc(vfsName(t))}</b><span>New files show up after Infuse refreshes its library, usually within a minute.</span></div></li>
        <li><span class="check-num">3</span><div><b>Press play</b><span>It streams while it downloads; Momo fetches the part you’re watching first.</span></div></li>
      </ol>
      ${t.type === 'movie' ? `<div class="path"><span class="t-micro ink-3">WebDAV path</span><code class="machine">${U.breakable(`/Movies/${vfsName(t)}/${vfsName(t)}.mkv`)}</code>${btn('Copy path', { variant: 'secondary', size: 'sm', icon: 'copy', act: 'toast', arg: 'path' })}</div>` : ''}
      <p class="t-caption ink-3">Not set up yet? <a href="#/settings?section=playback">Infuse setup guide</a></p></div>`,
      foot: `<span class="spacer"></span>${btn('Got it', { variant: 'primary', act: 'close-sheet' })}` });
  };
  SHEETS.subs = () => U.sheet({ id: 'sb', title: 'Subtitles', sub: 'OpenSubtitles · ranked by match to your release', body: `<div class="input-wrap search-field">${icon('search')}<input class="input" value="Dune Part Two 2024" aria-label="Subtitle search"></div>
      <ul class="subs-results">${[['English', 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX', 'Exact release match', 98, '41k'], ['English', 'Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-FLUX', 'Same group and source', 86, '88k'], ['English (SDH)', 'Dune Part Two 2024 BluRay', 'Different source; may drift', 54, '12k']].map(([l, n, why, sc, dl], i) => `<li class="sub-res"><div class="grow"><b>${l}</b><p class="machine t-caption ink-2">${U.breakable(n)}</p><p class="t-caption"><span class="dot ${sc > 80 ? 'ready' : 'warning'}"></span> ${why} · ${dl} downloads</p></div>${btn(i === 0 ? 'Get' : 'Get', { variant: i === 0 ? 'primary' : 'secondary', size: 'sm', act: 'toast', arg: 'English subtitles added' })}</li>`).join('')}</ul>` });

  // ================================================================== 8 · Torrent search
  const tsKey = (arg) => {
    if (!arg) return S.ts.ctx;
    if (arg.startsWith('frieren')) return 'frieren-s01e29';
    if (arg.startsWith('severance')) return 'severance-s02e05';
    if (arg === 'dune-part-two' || arg === 'past-lives' || arg === 'interstellar') return 'dune-part-two';
    return `gen:${arg}`;
  };
  const genReleases = (arg) => {
    const [id, a, b] = arg.split(':');
    const t = T(id) || T('severance');
    const nm = t.title.replace(/[:'’!,]/g, '').replace(/\s+/g, '.');
    const isMovie = t.type === 'movie';
    const tagS = isMovie ? `${t.year}` : a === 'pack' || b === 'pack' || b === 'missing' ? 'S01' : `S${pad(a)}E${pad(b)}`;
    const items = [
      { raw: `${nm}.${tagS}.1080p.WEB-DL.DDP5.1.H.264-NTb`, emph: `${nm}.${tagS}`, size: isMovie ? '7.9 GB' : '18.2 GB', se: 412, le: 12, age: '1 y', idx: 'TorrentLeech', tags: [['1080p'], ['WEB-DL'], ['x264'], ['NTb', 'group']], best: true, why: ['1080p WEB-DL', 'complete season', '412 seeders', 'matches your quality profile'] },
      { raw: `${nm}.${tagS}.2160p.WEB-DL.DDP5.1.HDR.H.265-FLUX`, emph: `${nm}.${tagS}`, size: isMovie ? '16.1 GB' : '44.0 GB', se: 160, le: 8, age: '1 y', idx: 'IPTorrents', tags: [['2160p', 'hi'], ['WEB-DL'], ['x265'], ['HDR'], ['FLUX', 'group']] },
      { raw: `${nm}.${tagS}.720p.WEB.h264-GROUP`, emph: `${nm}.${tagS}`, size: isMovie ? '2.1 GB' : '6.4 GB', se: 58, le: 2, age: '1 y', idx: 'EZTV', tags: [['720p'], ['WEB'], ['x264']] },
    ];
    if (!isMovie) items.forEach((r) => r.tags.unshift(['Season pack', 'pack']));
    return { ctx: { title: id, label: `${t.title}${isMovie ? ` (${t.year})` : ' · Season 1'}`, scope: isMovie ? 'movie' : 'season', query: `${t.title.replace(/[:'’]/g, '')} ${isMovie ? t.year : 'S01'}` }, total: 31, items, facets: D.releases['severance-s02e05'].facets };
  };
  const getSet = (key) => (key.startsWith('gen:') ? genReleases(key.slice(4)) : D.releases[key]);

  ACT.tsearch = (arg) => {
    const key = tsKey(arg);
    if (S.ts.ctx !== key) { const set = getSet(key); S.ts = { ctx: key, query: set.ctx.query, scope: set.ctx.scope, filters: {}, sort: 'best', shown: Math.min(8, set.items.length), arg }; }
    S.ts.searching = true;
    setParams({ sheet: 'torrents', nested: null, tstate: null });
    clearTimeout(window._tsT);
    window._tsT = setTimeout(() => { S.ts.searching = false; if (parseRoute().params.get('sheet') === 'torrents') render(); }, 1300);
  };
  const tsFilters = (set, compact) => {
    const f = S.ts.filters;
    return Object.entries(set.facets).map(([group, vals]) => `<fieldset class="facet"><legend>${group}</legend>
      ${vals.map(([v, c]) => `<label class="check"><input type="checkbox" ${f[group]?.includes(v) ? 'checked' : ''} data-act="tsf" data-arg="${group}|${v}" ${c === 0 ? 'disabled' : ''}><span>${v}</span><span class="count">${c}</span></label>`).join('')}</fieldset>`).join('')
      + `<fieldset class="facet"><legend>Size</legend><div class="range-row"><input class="input" aria-label="Minimum size in GB" value="0.5"><span class="ink-3">to</span><input class="input" aria-label="Maximum size in GB" value="60"><span class="ink-3">GB</span></div></fieldset>
         <fieldset class="facet"><legend>Minimum seeders</legend>${U.seg([['0', 'Any'], ['5', '5+'], ['20', '20+'], ['100', '100+']], '5', { label: 'Minimum seeders', block: true })}</fieldset>`;
  };
  const filterCount = () => Object.values(S.ts.filters).reduce((a, v) => a + v.length, 0);

  SHEETS.torrents = (ctx) => {
    const pk = ctx.p.get('ctx');
    if (pk && S.ts.ctx !== pk) { const s0 = getSet(pk); S.ts = { ctx: pk, query: s0.ctx.query, scope: s0.ctx.scope, filters: {}, sort: 'best', shown: Math.min(8, s0.items.length), arg: pk }; }
    const set = getSet(S.ts.ctx);
    const st = ctx.p.get('tstate') || (S.ts.searching ? 'searching' : 'results');
    if (st === 'filtered' && !Object.keys(S.ts.filters).length) S.ts.filters = { Resolution: ['2160p'], 'HDR / DV': ['DV'], Indexer: ['EZTV'] };
    const t = T(set.ctx.title);
    const f = S.ts.filters;
    const active = Object.entries(f).flatMap(([g, vs]) => vs.map((v) => [g, v]));
    // filtering (only a couple of facets are modelled; enough to show the states)
    let items = set.items.filter((r) => {
      if (f.Resolution?.length && !f.Resolution.some((x) => r.tags.some(([tg]) => tg === x))) return false;
      if (f['HDR / DV']?.length && !f['HDR / DV'].some((x) => r.tags.some(([tg]) => tg === x))) return false;
      return true;
    });
    if (st === 'filtered') items = [];
    const sortLabel = { best: 'Best', seeders: 'Seeders', size: 'Size', newest: 'Newest' }[S.ts.sort];
    const head = `<div class="sheet-sticky ts-sticky">
        <div class="ts-query">
          <div class="input-wrap search-field grow">${icon('search')}<label class="sr-only" for="tsq">Search releases</label><input class="input" id="tsq" value="${esc(S.ts.query)}" autocomplete="off" spellcheck="false"><span class="input-end">${iconBtn('x', 'Clear', { cls: 'sm', act: 'tsq-clear' })}</span></div>
        </div>
        ${set.ctx.scope !== 'movie' ? U.seg([['episode', 'Episode'], ['season', 'Season pack'], ['any', 'Any']], S.ts.scope, { act: 'ts-scope', label: 'Scope', block: true }) : ''}
        <div class="ts-chips">
          ${chip(`Filters${filterCount() ? ` · ${filterCount()}` : ''}`, { icon: 'sliders-horizontal', act: 'ts-filters', cls: `outline filters-chip ${filterCount() ? 'has' : ''}` })}
          ${active.map(([g, v]) => chip(v, { on: true, check: false, x: true, act: 'tsf', arg: `${g}|${v}` })).join('')}
          ${chip(`Sort: ${sortLabel}`, { caret: true, act: 'ts-sort', cls: 'outline' })}
        </div>
      </div>`;
    let results;
    if (st === 'searching') {
      results = `<div class="ts-status" role="status">${U.pawTrack(4, 2)}<span>Searching 4 indexers…</span></div>
        <ul class="idx-list">${[['Nyaa', 'done', 13], ['TorrentLeech', 'done', 6], ['IPTorrents', 'wait'], ['The Pirate Bay', 'wait']].map(([n, s, c]) => `<li class="${s}">${s === 'done' ? icon('check', 'ico ico-xs') : '<span class="spin"></span>'}${n}${c ? ` <b>${c}</b>` : ''}</li>`).join('')}</ul>
        ${Array.from({ length: 4 }, () => '<div class="rel skel-rel" aria-hidden="true"><div class="skel skel-line" style="width:92%"></div><div class="skel skel-line" style="width:60%"></div><div class="row gap-1"><div class="skel" style="width:52px;height:24px"></div><div class="skel" style="width:60px;height:24px"></div><div class="skel" style="width:44px;height:24px"></div></div><div class="skel skel-line" style="width:70%"></div></div>').join('')}`;
    } else if (st === 'none') {
      results = U.emptyState('detective', `Nothing for “${esc(S.ts.query)}”`, 'Momo asked 4 indexers. Releases often use the original title or season packs.', `${btn('Try Season pack', { variant: 'primary', size: 'sm', act: 'ts-scope', arg: 'season' })}${btn('Search “Sousou no Frieren”', { variant: 'secondary', size: 'sm' })}`, { compact: true });
    } else if (!items.length) {
      results = U.emptyState('detective', 'Your filters hide all results', `${set.total} releases found, but none are ${active.map((a) => a[1]).join(' + ') || '2160p + DV'}.`, `${(active.length ? active : [['Resolution', '2160p'], ['HDR / DV', 'DV']]).map(([g, v]) => chip(`Remove ${v}`, { act: 'tsf', arg: `${g}|${v}`, x: true })).join('')}${btn('Clear filters', { variant: 'secondary', size: 'sm', act: 'ts-clear' })}`, { compact: true });
    } else {
      const shown = items.slice(0, S.ts.shown);
      results = `${st === 'partial' ? U.banner('warn', 'triangle-alert', '2 indexers timed out', 'IPTorrents and 1337x didn’t answer in 8 s. Showing results from the other 3.', btn('Retry', { variant: 'secondary', size: 'sm', act: 'tsearch', arg: S.ts.arg })) : ''}
        <div class="ts-status"><span>Showing <b>${shown.length}</b> of <b>${set.total}</b> · ${set.ctx.also ? esc(set.ctx.also) : '4 indexers'}</span></div>
        <div class="rel-list">${shown.map((r, i) => U.releaseCard(r, { key: `${S.ts.ctx}|${set.items.indexOf(r)}`, flow: S.ts.ctx === 'frieren-s01e29' ? 'best' : S.ts.ctx.startsWith('gen:') ? 'best-gen' : undefined })).join('')}</div>
        <div class="load-more-row">${btn(`Load more · ${set.total - shown.length} left`, { variant: 'secondary', block: true, act: 'ts-more' })}<p class="t-caption ink-3">Sorted by best match: quality profile, seeders, coverage and size sanity.</p></div>`;
    }
    const sub = `for <b>${esc(t.short || t.title)}</b> · ${esc(set.ctx.label.replace(/^.*?(S\d|Season|\()/, '$1'))}`;
    return U.sheet({
      id: 'ts', title: 'Find a torrent', sub, cls: 'full xl ts-sheet',
      body: `${head}<div class="ts-cols"><aside class="ts-facets" aria-label="Filters">${tsFilters(set)}</aside><div class="ts-results" aria-live="polite">${results}</div></div>`,
    });
  };
  SHEETS.tsfilters = (ctx, nested) => {
    const set = getSet(S.ts.ctx);
    return U.sheet({ id: 'tf', nested, title: 'Filters', sub: `${set.total} releases · counts update as you pick`, cls: 'tf-sheet', back: 'close-nested', body: `<div class="facets-sheet">${tsFilters(set, true)}</div>`,
      foot: `${btn('Reset', { variant: 'ghost', act: 'ts-clear' })}<span class="spacer"></span>${btn(`Show ${Math.max(1, set.items.length - filterCount())} results`, { variant: 'primary', act: 'close-nested' })}` });
  };
  Object.assign(ACT, {
    'ts-filters': () => setParams({ nested: 'tsfilters' }),
    'ts-scope': (v) => { S.ts.scope = v; S.ts.searching = true; render(); setTimeout(() => { S.ts.searching = false; render(); }, 900); },
    'ts-sort': () => { const o = ['best', 'seeders', 'size', 'newest']; S.ts.sort = o[(o.indexOf(S.ts.sort) + 1) % o.length]; render(); },
    'ts-more': () => { S.ts.shown += 8; render(); toast({ text: 'Loaded 8 more', icon: 'list' }); },
    'ts-clear': () => { S.ts.filters = {}; render(); },
    'tsq-clear': () => { S.ts.query = ''; render(); document.getElementById('tsq')?.focus(); },
    tsf: (arg) => { const [g, v] = arg.split('|'); const cur = S.ts.filters[g] || []; S.ts.filters[g] = cur.includes(v) ? cur.filter((x) => x !== v) : [...cur, v]; if (!S.ts.filters[g].length) delete S.ts.filters[g]; render(); },
    inspect: (key) => setParams({ nested: 'inspect', rel: key }),
    'add-release': (key) => addRelease(key || parseRoute().params.get('rel')),
  });
  document.addEventListener('input', (e) => { if (e.target.id === 'tsq') S.ts.query = e.target.value; });

  // Inspect: dry-run file preview before adding
  SHEETS.inspect = (ctx, nested) => {
    const key = ctx.p.get('rel') || `${S.ts.ctx}|0`;
    const [setKey, idx] = key.split('|');
    const set = getSet(setKey);
    const r = set.items[Number(idx)] || set.items[0];
    const job = setKey === 'frieren-s01e29' ? D.jobs.frieren : null;
    const rows = job ? [...job.groups.matched.slice(0, 4), ...job.groups.needs_review, ...job.groups.matched.slice(4), ...job.groups.extras, ...job.groups.unmatched] : null;
    const preview = rows ? `<ul class="insp-files">${rows.map((f) => `<li><span class="insp-ico">${icon(f.kind === 'Extra' ? 'file' : 'file-video', 'ico ico-sm')}</span><div class="grow"><p class="machine mono t-code">${U.breakable(f.file)}</p><p class="t-caption ink-3">${f.size}</p></div><div class="insp-target">${f.target ? `<b>S${pad(f.target.s)}E${pad(f.target.n)}</b>${f.conflict ? U.pill('review', 'Duplicate', { sm: true }) : ''}` : f.kind ? '<span class="ink-3">Extra · skip</span>' : U.pill('missing', 'No match', { sm: true })}</div></li>`).join('')}</ul>`
      : `<ul class="insp-files"><li><span class="insp-ico">${icon('file-video', 'ico ico-sm')}</span><div class="grow"><p class="machine mono t-code">${U.breakable(r.raw + '.mkv')}</p><p class="t-caption ink-3">${r.size}</p></div><div class="insp-target"><b>${T(set.ctx.title).type === 'movie' ? 'Movie' : 'S01'}</b></div></li><li><span class="insp-ico">${icon('file-text', 'ico ico-sm')}</span><div class="grow"><p class="machine mono t-code">Subs/English.srt</p><p class="t-caption ink-3">84 KB</p></div><div class="insp-target"><span class="ink-3">Subtitle</span></div></li></ul>`;
    return U.sheet({ id: 'in', nested, title: 'Inspect', back: 'close-nested', sub: 'Dry run. Nothing downloads until you add it.', cls: 'full insp-sheet', body: `
      <p class="machine insp-name">${U.machine(r.raw, r.emph)}</p>
      <div class="tags" style="margin:8px 0 14px">${r.tags.map(([x, k]) => U.tag(x, k)).join('')}</div>
      ${job ? `<div class="insp-sum">${U.pill('ready_short', '10 episodes → S01E29–E38', {})}${U.pill('review', '1 duplicate', {})}${U.pill('neutral', '2 extras', {})}${U.pill('missing', '1 unmatched', {})}</div>
        <p class="insp-note">${momo('avatar')}<span>Momo matched the “S2” numbering to TMDB season 1 (episode 29 on). You’ll confirm the duplicate after adding.</span></p>` : `<p class="insp-note">${momo('avatar')}<span>One video file. It maps straight to this title.</span></p>`}
      <h3 class="t-label" style="margin:16px 0 8px">Files · ${job ? job.files : 2}</h3>${preview}`,
      foot: `${btn('Back', { variant: 'ghost', act: 'close-nested' })}<span class="spacer"></span>${btn('Add & assign', { variant: 'primary', icon: 'plus', act: 'add-release', arg: key, flow: 'add-inspect' })}` });
  };

  function addRelease(key) {
    const [setKey] = (key || '').split('|');
    const set = getSet(setKey || S.ts.ctx);
    const t = T(set.ctx.title);
    if (t.type === 'movie') {
      S.movieState[t.id] = 'downloading';
      setParams({ sheet: null, nested: null });
      toast({ text: 'Added. Momo’s on it.', sub: `${t.title} is downloading; it’ll be ready in Infuse soon.`, momo: 'proud' });
      return;
    }
    S.job[t.id] = { phase: 0, resolved: {}, applied: false };
    location.hash = `#/review/${t.id}`;
    runJob(t.id);
  }
  function runJob(id) {
    clearInterval(window._jobT);
    window._jobT = setInterval(() => {
      const j = S.job[id];
      if (!j) return clearInterval(window._jobT);
      j.phase++;
      if (j.phase >= 3) { clearInterval(window._jobT); j.phase = 3; }
      if (parseRoute().parts[0] === 'review') render();
    }, 1200);
  }
  window.runJob = runJob;
})();
