/* For You (home). States: default · trakt-off · refreshing · empty (first time) · syncing */
(function () {
  const { ui, app, art, brand, icon, data: D } = MS;
  const T = (id) => D.T[id];
  const esc = ui.esc;

  const PICKS = [
    { id: 'pastlives', kicker: 'Tonight\'s pick', reason: 'You gave Perfect Days a 9 and finished Aftersun twice. A quiet, devastating 1\u00a0h\u00a046\u00a0m, and it fits before midnight.' },
    { id: 'pluribus', kicker: 'New for you', reason: 'Vince Gilligan\'s new series, and sci-fi with a dry sense of humour is your highest-rated corner of Trakt.' },
    { id: 'dune2', kicker: 'Ready in Infuse', reason: 'Already downloaded in 2160p Dolby Vision. You stopped Part One with 20 minutes left in 2022.' },
  ];

  function hero(pick, opts = {}) {
    const t = T(pick.id);
    const inLib = t.lib === 'ready';
    const primary = inLib
      ? `<button class="btn btn-primary btn-lg" data-act="howto">${icon('play', { size: 18, fill: true })}Play in Infuse</button>`
      : `<button class="btn btn-primary btn-lg" data-act="add-pick" data-id="${t.id}">${icon('plus', { size: 20 })}Add to library</button>`;
    return `<section class="hero home-hero" style="--ambient:${art.ambient(t.art)}" aria-label="${esc(pick.kicker)}: ${esc(t.title)}">${ui.phoneBar()}
      <div class="hero-glow"></div>
      <div class="hero-bg parallax" data-parallax>${art.backdrop(t.art)}</div>
      <div class="hero-scrim"></div>
      <div class="page"><div class="hero-content">
        <p class="eyebrow">${opts.trending ? icon('zap', { size: 16 }) : brand.avatar(22)}<span>${esc(pick.kicker)}</span></p>
        <h1 class="t-display-xl">${esc(t.title)}</h1>
        <p class="meta-line"><span class="star">${icon('star', { size: 13, fill: true })}${t.rating}</span><span>${t.year}</span><span>${t.runtime || t.years}</span><span>${t.genres.slice(0, 2).join(', ')}</span></p>
        ${pick.reason ? `<p class="t-body-l overview why-line">${esc(pick.reason)}</p>` : ''}
        <div class="actions">${primary}<a class="btn btn-glass btn-lg" href="#/title/${t.type}/${t.id}">Details</a>
          ${opts.trending ? '' : `<button class="btn btn-glass btn-lg btn-icon" data-act="fbmenu" aria-label="Feedback on this pick">${icon('more', { size: 20 })}</button>`}</div>
        <div class="hero-pager" role="tablist" aria-label="Tonight's picks">${PICKS.map((p, i) => `<button role="tab" aria-selected="${p.id === pick.id}" aria-label="${esc(T(p.id).title)}" data-act="pick" data-i="${i}"><i></i></button>`).join('')}</div>
      </div></div>
    </section>`;
  }

  const askEntry = () => `<a class="ask-entry" href="#/ask">${brand.avatar(40)}<span class="grow"><strong>Ask momoshtrem…</strong><span class="t-small">"a 90-minute thriller for tonight"</span></span><span class="btn btn-secondary btn-icon btn-sm" aria-hidden="true">${icon('arrowRight', { size: 18 })}</span></a>`;

  function upNextItems() {
    return D.upNext.map((u) => {
      if (u.show === 'frieren' && app.state.frieren === 'ready') return ui.landCard({ ...u, availability: 'ready' });
      if (u.show === 'frieren' && app.state.frieren === 'mapping') return ui.landCard({ ...u, availability: 'downloading', pct: 12 });
      return ui.landCard(u);
    }).join('');
  }

  function shelfRail(s, i) {
    return ui.rail({ id: `shelf-${i}`, title: s.title, subtitle: s.subtitle, items: s.items.map((it) => ui.posterCard(it.id, { reason: it.reason, feedback: true, badge: it.badge ? ui.pill('upcoming', it.badge, { onArt: true }) : '' })).join('') });
  }

  function connectCard() {
    return `<section class="connect-card panel" aria-labelledby="connect-h">
      <div class="connect-art">${ui.cat('peek', 132)}</div>
      <div class="stack gap-3" style="min-width:0">
        <h2 class="t-title-2" id="connect-h">Make this page yours</h2>
        <p class="c-2">Connect Trakt and momoshtrem learns from what you've watched. You get Up next for your shows and picks with a reason for each. It's read-only: nothing is posted to Trakt.</p>
        <div class="row gap-2 wrap"><a class="btn btn-primary" href="#/settings?section=trakt&state=code">${icon('link', { size: 18 })}Connect Trakt</a><a class="btn btn-ghost" href="#/discover">Browse instead</a></div>
      </div>
    </section>`;
  }

  function refreshing() {
    return `<div class="refresh-banner glass" role="status">
      <span class="eyes-sm">${brand.eyes(40)}</span>
      <div class="grow"><strong>Refreshing your picks</strong><span class="t-small c-2">Reading your history · searching what's new · 2 of 5 shelves done</span></div>
      <span class="t-small c-3 sm-up">You can keep browsing</span>
    </div>`;
  }

  function firstTime() {
    return `<section class="welcome page">
      <div class="welcome-art">${ui.cat('sit', 120)}</div>
      <h1 class="t-display">Evening. I'm Noir.</h1>
      <p class="t-body-l c-2" style="max-width:52ch">I find things to watch, get them ready, and hand them to Infuse on your Apple TV. Three steps and this page fills up.</p>
      <ol class="welcome-steps">
        <li><span class="n">1</span><div><strong>Connect Trakt</strong><p class="t-small c-2">Your history powers Up next and personal picks.</p></div><a class="btn btn-primary btn-sm" href="#/settings?section=trakt&state=code">Connect</a></li>
        <li><span class="n">2</span><div><strong>Add something to your library</strong><p class="t-small c-2">Search for a title or browse Discover.</p></div><button class="btn btn-secondary btn-sm" data-act="palette">Search</button></li>
        <li><span class="n">3</span><div><strong>Point Infuse at momoshtrem</strong><p class="t-small c-2">One WebDAV share, set up once.</p></div><button class="btn btn-secondary btn-sm" data-act="howto">Show me</button></li>
      </ol>
    </section>`;
  }

  app.route('/home', (p) => {
    const state = p.state || (app.state.trakt === 'off' ? 'trakt-off' : app.state.trakt === 'syncing' ? 'syncing' : 'default');
    const pick = PICKS[Number(p.pick || 0)] || PICKS[0];
    const switcher = ui.protoStates('#/home', [['default', 'Personal'], ['trakt-off', 'Trakt not connected'], ['refreshing', 'Refreshing picks'], ['syncing', 'First sync'], ['empty', 'First time']], state);
    let html = '';

    if (state === 'empty') {
      html = `${switcher}<div class="page-sections" style="padding-top:var(--sp-10)">${firstTime()}
        <div class="page">${ui.rail({ id: 'trending', title: 'Trending this week', subtitle: 'From TMDB, while I get to know you', items: D.trending.map((id) => ui.posterCard(id, { showLib: false })).join('') })}</div></div>`;
    } else if (state === 'trakt-off') {
      html = `${switcher}${hero({ id: 'pluribus', kicker: 'Trending this week', reason: '' }, { trending: true })}
        <div class="page page-sections home-body">
          ${connectCard()}
          ${ui.rail({ id: 'trending', title: 'Trending this week', items: D.trending.map((id) => ui.posterCard(id)).join('') })}
          ${ui.rail({ id: 'anime', title: 'Popular anime', items: ['frieren', 'dandadan', 'onepiece', 'mushishi', 'paprika', 'totoro'].map((id) => ui.posterCard(id)).join('') })}
          ${ui.rail({ id: 'movies', title: 'Popular movies', items: ['dune2', 'pastlives', 'interstellar', 'perfectdays', 'arrival', 'aftersun', 'columbus'].map((id) => ui.posterCard(id)).join('') })}
        </div>`;
    } else if (state === 'syncing') {
      html = `${switcher}<div class="page-sections" style="padding-top:var(--sp-8)">
        <section class="page sync-hero">
          ${ui.loader('First sync with Trakt', 'Imported 2,184 plays, 312 ratings and 41 watchlist items. Building your picks…')}
          <div class="steps sync-steps">
            <div class="step is-done"><span class="ic">${icon('check', { size: 14, stroke: 2.5 })}</span>Imported your history<span class="detail">2,184 plays</span></div>
            <div class="step is-done"><span class="ic">${icon('check', { size: 14, stroke: 2.5 })}</span>Found Up next for 6 shows</div>
            <div class="step is-active"><span class="ic"></span>Writing your shelves<span class="detail">2 of 5</span></div>
          </div>
          <a class="btn btn-secondary" href="#/home?state=default&st.trakt=connected">Skip ahead (prototype)</a>
        </section>
        <div class="page">${ui.rail({ id: 'up-next', title: 'Up next', subtitle: 'Ready now', kind: 'wide', items: upNextItems() })}</div>
        <div class="page section"><div class="section-head"><div class="titles"><h2 class="t-title-2">Because you loved Severance</h2><p class="t-small c-3">Writing…</p></div></div><div class="rail rail--posters">${ui.skelPosters(8)}</div></div>
      </div>`;
    } else {
      html = `${switcher}${hero(pick)}
        <div class="page page-sections home-body">
          ${state === 'refreshing' ? refreshing() : ''}
          <div class="phone-only">${askEntry()}</div>
          ${ui.rail({ id: 'up-next', title: 'Up next', subtitle: 'From your Trakt progress', kind: 'wide', items: upNextItems() })}
          ${ui.rail({ id: 'new-eps', title: 'New episodes', subtitle: 'For shows you watch', kind: 'wide', items: D.newEpisodes.map((u) => ui.landCard(u, { when: u.when })).join('') })}
          ${shelfRail(D.shelves[0], 0)}
          <section class="ask-band desk-only" aria-label="Ask momoshtrem">${askEntry()}<div class="ask-suggest">${['something like Severance but lighter', 'a 90-minute thriller for tonight', 'anime with a Frieren vibe'].map((q) => `<a class="fchip" href="#/ask?q=${encodeURIComponent(q)}">${esc(q)}</a>`).join('')}</div></section>
          ${state === 'refreshing' ? `<section class="section"><div class="section-head"><div class="titles"><h2 class="t-title-2">New this month for you</h2><p class="t-small c-3">Refreshing…</p></div></div><div class="rail rail--posters">${ui.skelPosters(8)}</div></section>` : shelfRail(D.shelves[1], 1)}
          ${shelfRail(D.shelves[2], 2)}
          ${shelfRail(D.shelves[3], 3)}
          ${ui.rail({ id: 'watchlist', title: 'Your Trakt watchlist', subtitle: '41 titles · 6 in your library', items: D.watchlist.map((id) => ui.posterCard(id)).join(''), seeAll: '#/library?filter=watchlist' })}
          ${shelfRail(D.shelves[4], 4)}
          <p class="picks-foot t-small c-3">${brand.avatar(20)}Picks written by Noir at 03:00 from your Trakt history. <a class="c-2" href="#/settings?section=ai">Change schedule</a></p>
        </div>`;
    }

    return {
      title: 'For You', tab: 'home', overArt: state !== 'empty' && state !== 'syncing', mainCls: state !== 'empty' && state !== 'syncing' ? 'has-hero' : '',
      html,
      mount(root) {
        // gentle parallax on the hero backdrop (disabled for reduced motion)
        const bg = root.querySelector('[data-parallax] > svg');
        if (bg && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
          const onScroll = () => { const y = Math.min(window.scrollY, 600); bg.style.transform = `scale(1.06) translateY(${y * 0.18}px)`; };
          window.addEventListener('scroll', onScroll, { passive: true });
        }
      },
    };
  });

  Object.assign(app.actions, {
    pick: (b) => app.go(`#/home?pick=${b.dataset.i}`),
    'add-pick': (b) => { b.classList.add('is-loading'); setTimeout(() => { b.classList.remove('is-loading'); b.outerHTML = `<button class="btn btn-primary btn-lg" data-act="find" data-show="${b.dataset.id}">${icon('search', { size: 18 })}Find a torrent</button>`; app.toast(null, { kind: 'ready', title: 'Added to library', body: 'Next: find a torrent' }); }, 700); },
  });

  // Feedback sheet (like / dislike / not interested / already seen)
  app.sheet('feedback', (p) => {
    const t = T(p.id || 'pastlives') || T('pastlives');
    return {
      cls: 'sheet--narrow',
      html: `<div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head"><div class="sheet-title"><div class="row gap-3" style="min-width:0"><div style="width:40px">${ui.poster(t.id)}</div><div class="stack" style="min-width:0"><h2 class="t-title-3 clamp-1" id="feedback-title">${esc(t.title)}</h2><span class="t-small c-3">Tune your picks</span></div></div><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div></div>
        <div class="sheet-body"><div class="menu" style="background:none;border:0;box-shadow:none;padding:0">
          <button data-act="fb-done">${icon('thumbsUp', { size: 18 })}More like this</button>
          <button data-act="fb-done">${icon('thumbsDown', { size: 18 })}Less like this</button>
          <button data-act="fb-done">${icon('ban', { size: 18 })}Not interested<span class="hint">Hides it everywhere</span></button>
          <button data-act="fb-done">${icon('checkCheck', { size: 18 })}Already seen<span class="hint">Counts as watched</span></button>
          <div class="sep"></div>
          <button data-act="fb-done">${icon('bookmark', { size: 18 })}Add to library</button>
        </div></div>`,
    };
  });
  app.actions['fb-done'] = () => { app.close(); app.toast('feedback'); };
})();
