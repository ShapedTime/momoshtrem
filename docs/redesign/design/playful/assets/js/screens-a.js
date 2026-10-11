/* Screens: Sign in · For You · Ask momoshtrem · Discover · Search (+ ⌘K palette) */
(function () {
  const U = UI;
  const { icon, momo, btn, iconBtn, chip, esc } = U;
  const D = DATA;
  const T = (id) => D.T[id];

  const phead = (title, o = {}) => `<header class="phead">
      <div class="grow">${o.eyebrow ? `<p class="eyebrow">${o.eyebrow}</p>` : ''}<h1 class="t-title-1">${title}</h1>${o.sub ? `<p class="phead-sub">${o.sub}</p>` : ''}</div>
      <div class="phead-actions">${o.actions || ''}<span class="hide-desk">${window.catMenuBtn()}</span></div>
    </header>`;
  window.phead = phead;

  // ================================================================== 1 · Sign in
  SCREENS.signin = (ctx) => {
    const st = ctx.state || 'default';
    const err = st === 'error';
    const limited = st === 'limited';
    const busy = st === 'submitting';
    return {
      title: 'Sign in', chrome: false,
      html: `<div class="signin">
        <div class="signin-frame" aria-hidden="true"></div>
        <div class="signin-card ${err ? 'shake' : ''}">
          <div class="signin-peek ${st === 'password' ? 'shy' : ''}" aria-hidden="true">${momo(st === 'password' ? 'peeking-shy' : 'peeking')}</div>
          <div class="signin-brand"><img src="brand/logo/mark.svg" alt="" width="56" height="56"><span class="brand-word">momoshtrem</span></div>
          <h1 class="t-title-2">Welcome back</h1>
          <p class="ink-2 signin-sub">${st === 'password' ? 'Momo isn’t looking.' : 'Sign in to your couch concierge.'}</p>
          ${err ? U.banner('err', 'circle-alert', 'Wrong username or password', 'Check both and try again. Your password manager can fill them in.') : ''}
          ${limited ? U.banner('warn', 'hourglass', 'Too many attempts', 'For safety, sign-in is paused. Try again in <b class="mono">4:59</b>.') : ''}
          <form class="stack gap-4" method="post" action="/api/auth" data-act="noop" novalidate>
            <div class="field"><label class="field-label" for="username">Username</label>
              <input class="input ${err ? 'is-error' : ''}" id="username" name="username" type="text" autocomplete="username" autocapitalize="none" spellcheck="false" value="${st === 'default' ? '' : 'teymur'}" ${limited ? 'aria-describedby="rl"' : ''}></div>
            <div class="field"><label class="field-label" for="password">Password</label>
              <div class="input-wrap"><input class="input ${err ? 'is-error' : ''} ${st === 'password' ? 'is-focus' : ''}" id="password" name="password" type="password" autocomplete="current-password" value="${st === 'default' ? '' : 'correct-horse'}" ${err ? 'aria-invalid="true" aria-describedby="pw-err"' : ''}>
                <span class="input-end">${iconBtn('eye', 'Show password', { cls: 'sm', act: 'pw-toggle' })}</span></div>
              ${err ? '<p class="field-error" id="pw-err">Wrong username or password.</p>' : ''}</div>
            <button class="btn btn-primary btn-lg btn-block ${busy ? 'is-loading' : ''}" type="submit" ${limited ? 'disabled aria-disabled="true"' : ''} data-act="signin">${busy ? 'Signing in…' : limited ? 'Try again in 4:59' : 'Sign in'}</button>
            ${limited ? '<p class="field-hint" id="rl" style="text-align:center">5 wrong attempts from this device in 10 minutes.</p>' : ''}
          </form>
        </div>
        <p class="signin-foot">momoshtrem · self-hosted · <span class="mono">v2.0</span></p>
      </div>`,
    };
  };
  Object.assign(ACT, {
    signin: () => setParams({ state: 'submitting' }) || setTimeout(() => go('#/for-you'), 900),
    'pw-toggle': (a, el) => { const i = document.getElementById('password'); i.type = i.type === 'password' ? 'text' : 'password'; el.setAttribute('aria-label', i.type === 'password' ? 'Show password' : 'Hide password'); el.innerHTML = icon(i.type === 'password' ? 'eye' : 'eye-off'); },
  });
  document.addEventListener('focusin', (e) => { if (e.target.id === 'password') document.querySelector('.signin-peek')?.classList.add('shy'); });
  document.addEventListener('focusout', (e) => { if (e.target.id === 'password') document.querySelector('.signin-peek')?.classList.remove('shy'); });

  // ================================================================== 2 · For You
  const askEntry = (o = {}) => `<section class="ask-entry ${o.cls || ''}" aria-label="Ask momoshtrem">
      <a class="ask-bar" href="#/ask" data-flow="ask-entry">
        <span class="ask-avatar">${momo('avatar')}</span>
        <span class="grow"><b>Ask momoshtrem…</b><span class="ask-hint">“a 90-minute thriller for tonight”</span></span>
        <span class="ask-go">${icon('sparkles')}</span>
      </a>
      <div class="ask-chips">${['Something like Severance but lighter', 'Short anime, under 13 episodes', 'A film we can finish by 11'].map((q) => `<a class="chip outline" href="#/ask?q=${encodeURIComponent(q)}">${esc(q)}</a>`).join('')}</div>
    </section>`;
  window.askEntry = askEntry;

  const fbSummary = (id) => (S.feedback[id] ? `<span class="pill pill-neutral sm">${icon({ like: 'thumbs-up', dislike: 'thumbs-down', not: 'eye-off', seen: 'check-check' }[S.feedback[id]], 'ico')}${{ like: 'Liked', dislike: 'Not for me', not: 'Hidden', seen: 'Seen' }[S.feedback[id]]}</span>` : '');

  SCREENS['for-you'] = (ctx) => {
    const st = ctx.state || (S.trakt === 'connected' ? (S.picks === 'refreshing' ? 'refreshing' : 'default') : 'trakt-off');
    const hour = 'Good evening';
    if (st === 'empty') {
      return { title: 'For You', nav: 'for-you', html: `<div class="page">${phead('For you')}
        <div class="first-run card">
          <div class="first-run-art">${momo('yawn')}</div>
          <div class="first-run-body">
            <h2 class="t-title-2">Nothing here yet. Let’s fix that.</h2>
            <p class="ink-2">momoshtrem fills this page from what you watch. Three small steps and Momo will have picks for tonight.</p>
            <ol class="checklist">
              <li class="is-next"><span class="check-num">1</span><div class="grow"><b>Connect Trakt</b><span>So Momo knows your history and what’s next.</span></div>${btn('Connect', { variant: 'primary', size: 'sm', href: '#/settings?trakt=start' })}</li>
              <li><span class="check-num">2</span><div class="grow"><b>Add something to your library</b><span>Browse, or just ask.</span></div>${btn('Discover', { size: 'sm', href: '#/discover' })}</li>
              <li><span class="check-num">3</span><div class="grow"><b>Set up Infuse</b><span>Point it at your WebDAV address once.</span></div>${btn('Guide', { size: 'sm', href: '#/settings#playback' })}</li>
            </ol>
          </div>
        </div>
        ${askEntry({ cls: 'section' })}
      </div>` };
    }

    const upnext = D.upNext.map((it) => {
      if (it.show === 'frieren' && S.frierenReady) return U.upCard({ ...it, av: 'ready' });
      return U.upCard(it);
    }).join('');
    const readyCount = D.upNext.filter((u) => u.av === 'ready').length + (S.frierenReady ? 1 : 0);

    const shelves = (st === 'refreshing' ? D.shelves.slice(0, 2) : D.shelves).map((sh) => U.shelf(
      `${sh.title}`, sh.sub,
      st === 'refreshing' ? Array.from({ length: 7 }, () => '<div class="stack gap-2"><div class="skel skel-poster"></div><div class="skel skel-line" style="width:80%"></div><div class="skel skel-line" style="width:50%"></div></div>').join('')
        : sh.items.map((id) => U.posterCard(id, { reason: true, feedback: true, status: fbSummary(id) || (T(id).comingSoon ? U.pill('airs', T(id).comingSoon, { sm: true }) : '') })).join(''),
      { more: '#/discover' },
    )).join('');

    const html = st === 'trakt-off' ? `<div class="page">
        ${phead('For you', { sub: 'Trending this week. Connect Trakt for picks that are actually yours.' })}
        <section class="connect-card" aria-labelledby="ct">
          <div class="connect-art">${momo('peeking')}</div>
          <div class="grow">
            <h2 id="ct" class="t-title-3">Let Momo see what you’ve watched</h2>
            <p class="ink-2">Connect Trakt (read-only) and this page becomes yours: what’s next in your shows, whether it’s ready, and picks with a reason.</p>
            <div class="row gap-2 wrap" style="margin-top:14px">${btn('Connect Trakt', { variant: 'primary', icon: 'link', href: '#/settings?trakt=start', flow: 'connect' })}${btn('What does it read?', { variant: 'ghost', act: 'toast', arg: 'History, ratings, watchlist and show progress. Nothing is written back.' })}</div>
          </div>
        </section>
        ${askEntry({ cls: 'section' })}
        ${U.shelf('Trending this week', 'Popular everywhere, not personal yet', D.trending.map((id) => U.posterCard(id)).join(''), { more: '#/discover' })}
        ${U.shelf('Popular anime', null, ['frieren', 'one-piece', 'delicious-in-dungeon', 'apothecary-diaries', 'vinland-saga', 'odd-taxi', 'mushishi'].map((id) => U.posterCard(id)).join(''))}
        ${U.shelf('Acclaimed films', null, ['dune-part-two', 'past-lives', 'perfect-days', 'interstellar', 'aftersun', 'drive-my-car', 'arrival'].map((id) => U.posterCard(id)).join(''))}
      </div>` : `<div class="page">
        ${phead(hour, { eyebrow: 'For you', sub: `${readyCount} episodes ready for tonight · picks refreshed 04:02` })}
        ${askEntry()}
        <section class="section" aria-labelledby="upnext-h">
          <div class="shelf-head"><div><h2 class="shelf-title" id="upnext-h">Up next</h2><p class="shelf-sub">From your Trakt progress · ${D.upNext.length} shows</p></div>
            <div class="shelf-nav">${iconBtn('chevron-left', 'Scroll Up next left', { cls: 'sm filled', act: 'shelf', arg: '-1' })}${iconBtn('chevron-right', 'Scroll Up next right', { cls: 'sm filled', act: 'shelf', arg: '1' })}</div></div>
          <div class="shelf-sec"><div class="shelf-wrap"><div class="shelf shelf-wide" tabindex="0">${upnext}</div></div></div>
        </section>
        <section class="section" aria-labelledby="newep-h">
          <div class="shelf-head"><div><h2 class="shelf-title" id="newep-h">New episodes for your shows</h2></div></div>
          <ul class="list newep">${D.newEpisodes.map((n) => {
            const t = T(n.show);
            const ready = n.av === 'ready' || (n.show === 'frieren' && S.frierenReady);
            return `<li class="list-row">${U.poster(n.show, { cls: 'thumb' })}<div class="grow"><a class="newep-title" href="#/show/${n.show}">${esc(t.short || t.title)}</a><p class="ink-3 t-caption">${esc(n.label)} · ${esc(n.title)} · ${esc(n.when)}</p></div>
              ${ready ? U.pill('ready_short', 'Ready', { sm: true }) : btn('Find', { size: 'sm', icon: 'search', act: 'tsearch', arg: `${n.show}:pack` })}</li>`;
          }).join('')}</ul>
        </section>
        ${st === 'refreshing' ? `<div class="section">${U.banner('info', 'sparkles', 'Momo is picking tonight’s shelves', 'Reading your history · searching for new releases · checking TMDB. Takes about a minute; your old picks stay until then.', `<span class="refresh-paws">${U.pawTrack(5, 2)}</span>`)}</div>` : ''}
        ${shelves}
        ${U.shelf('Your Trakt watchlist', `${D.watchlist.length} titles`, D.watchlist.map((id) => U.posterCard(id, { status: id === 'interstellar' ? U.pill('not_in_library', 'Not in library', { sm: true }) : '' })).join(''), { more: '#/library' })}
        <p class="picks-foot">${icon('sparkles', 'ico ico-sm')}Picks by Claude Opus 5.5 at 04:02 · $0.61 · ${btn('Refresh now', { variant: 'ghost', size: 'sm', icon: 'refresh-cw', act: 'refresh-picks' })}</p>
      </div>`;
    return { title: 'For You', nav: 'for-you', html };
  };
  ACT['refresh-picks'] = () => { S.picks = 'refreshing'; render(); setTimeout(() => { S.picks = 'ready'; render(); toast({ text: 'Fresh picks are in', sub: '5 shelves · 34 titles', momo: 'proud' }); }, 2600); };
  ACT.feedback = (id) => setParams({ sheet: 'feedback', fid: id });
  SHEETS.feedback = (ctx) => {
    const id = ctx.p.get('fid');
    const t = T(id);
    const opt = (k, ic, label, sub) => `<button class="menu-item fb-item ${S.feedback[id] === k ? 'on' : ''}" data-act="fb" data-arg="${id}:${k}">${icon(ic)}<span class="grow"><b>${label}</b><small>${sub}</small></span>${S.feedback[id] === k ? icon('check') : ''}</button>`;
    return U.sheet({ id: 'fb', title: esc(t.title), sub: t.reason ? `“${esc(t.reason)}”` : '', cls: 'fb-sheet', body: `<div class="stack gap-1">
      ${opt('like', 'thumbs-up', 'Like', 'More picks like this')}${opt('dislike', 'thumbs-down', 'Dislike', 'Fewer picks like this')}
      ${opt('not', 'eye-off', 'Not interested', 'Hide this title')}${opt('seen', 'check-check', 'Already seen', 'Mark as watched for picks')}
      <div class="menu-sep"></div>${btn('Add to library', { variant: 'primary', block: true, icon: 'plus', act: 'add-lib', arg: id })}</div>` });
  };
  ACT.fb = (arg) => { const [id, k] = arg.split(':'); S.feedback[id] = k; setParams({ sheet: null, fid: null }); toast({ text: { like: 'Liked. Momo will find more like this.', dislike: 'Got it. Fewer like this.', not: 'Hidden from your picks', seen: 'Marked as seen' }[k], icon: 'check', action: 'Undo', act: 'fb-undo' }); window._lastFb = id; };
  ACT['fb-undo'] = () => { delete S.feedback[window._lastFb]; render(); };

  // ================================================================== 3 · Ask momoshtrem
  const STEPS = [
    ['Reading your history…', '1,284 plays · you finish slow-burn mysteries and rate comedies 8+'],
    ['Searching the web for new releases…', '6 searches · reviews, new seasons, festival buzz'],
    ['Checking TMDB…', 'Matching 14 candidates, dropping 5 you’ve seen'],
    ['Picking the best six', ''],
  ];
  const askThreads = (active) => `<aside class="ask-threads" aria-label="Past threads">
      <div class="row gap-2" style="justify-content:space-between"><h2 class="t-headline">Threads</h2>${btn('New', { variant: 'ghost', size: 'sm', icon: 'plus', href: '#/ask?new=1' })}</div>
      <ul>${D.threads.map((t) => `<li><a class="thread ${active === t.id ? 'on' : ''}" href="#/ask?thread=${t.id}"><span class="clamp-2">${esc(t.q)}</span><small>${t.when}</small></a></li>`).join('')}</ul>
    </aside>`;
  const recCard = (id) => {
    const t = T(id);
    const added = S.added[id];
    return `<article class="rec" data-flow="${id === 'the-good-place' ? 'rec-goodplace' : ''}">
      <a href="#/${t.type}/${id}" class="rec-art" aria-label="${esc(t.title)}">${U.poster(id)}</a>
      <div class="rec-body">
        <a href="#/${t.type}/${id}" class="rec-title">${esc(t.title)}</a>
        <p class="pc-meta">${U.meta(t)} · ${t.genres.slice(0, 2).join(', ')}${t.rating ? ` · ★ ${t.rating}` : ''}</p>
        <p class="rec-reason">${esc(t.reason || '')}</p>
      </div>
        <div class="rec-actions">
          ${added ? btn('Find a torrent', { variant: 'primary', size: 'sm', icon: 'search', act: 'tsearch', arg: `${id}:pack`, flow: 'rec-find' }) : btn('Add to library', { variant: 'secondary', size: 'sm', icon: 'plus', act: 'ask-add', arg: id, flow: id === 'the-good-place' ? 'rec-add' : undefined })}
          <span class="spacer"></span>
          ${iconBtn('thumbs-up', `Like ${t.title}`, { cls: `sm ${S.feedback[id] === 'like' ? 'on' : ''}`, act: 'fb-inline', arg: `${id}:like`, pressed: S.feedback[id] === 'like' ? 'true' : 'false' })}
          ${iconBtn('thumbs-down', `Dislike ${t.title}`, { cls: `sm ${S.feedback[id] === 'dislike' ? 'on' : ''}`, act: 'fb-inline', arg: `${id}:dislike`, pressed: S.feedback[id] === 'dislike' ? 'true' : 'false' })}
          ${iconBtn('ellipsis', `More for ${t.title}`, { cls: 'sm', act: 'feedback', arg: id })}
        </div>
    </article>`;
  };
  SCREENS.ask = (ctx) => {
    const q = ctx.p.get('q');
    const forced = ctx.state; // start | working | results
    let thread = ctx.p.get('thread');
    if (q && S.ask.q !== q) { S.ask = { q, phase: 'working', step: 0, thread: 'lighter' }; runAsk(); }
    if (ctx.p.get('new')) S.ask = { phase: 'idle', step: 0 };
    let phase = forced || (thread ? 'results' : S.ask.phase);
    if (forced === 'working') S.ask.step = 2;
    const qtext = q || (thread ? D.threads.find((t) => t.id === thread)?.q : S.ask.q) || 'something like Severance but lighter';
    const tid = thread || S.ask.thread || 'lighter';
    const momoMsg = (body, pose = 'avatar') => `<div class="msg momo-msg"><span class="msg-av">${momo(pose)}</span><div class="msg-body">${body}</div></div>`;
    let convo = '';
    if (phase === 'idle' || phase === 'start') {
      convo = `<div class="ask-hello">
          <div class="ask-hello-art">${momo('detective')}</div>
          <h2 class="t-title-2">What are we in the mood for?</h2>
          <p class="ink-2">Tell Momo a mood, a length or a show you loved. Momo reads your history, searches the web for what’s new and explains every pick.</p>
          <div class="ask-examples">${['something like Severance but lighter', 'a 90-minute thriller for tonight', 'anime with a Frieren vibe', 'a comfort rewatch that isn’t The Office'].map((x, i) => `<a class="example" href="#/ask?q=${encodeURIComponent(x)}" ${i === 0 ? 'data-flow="ask-example"' : ''}>${icon('sparkle', 'ico ico-sm')}<span>${esc(x)}</span>${icon('arrow-up-right', 'ico ico-sm')}</a>`).join('')}</div>
        </div>`;
    } else {
      const step = phase === 'working' ? S.ask.step : STEPS.length;
      const steps = STEPS.map(([label, detail], i) => ({ label, detail: i < step ? detail : '', state: i < step ? 'done' : i === step ? 'now' : 'todo' }));
      convo = `<div class="msg me-msg"><div class="msg-body">${esc(qtext)}</div></div>
        ${momoMsg(`<div class="ask-steps ${phase === 'results' ? 'is-done' : ''}">${phase === 'results' ? `<details><summary>${icon('check', 'ico ico-sm')} Researched in 38 s · 6 web searches · $0.09</summary>${U.pawSteps(steps)}</details>` : `<p class="ask-working">Momo is on it</p>${U.pawSteps(steps)}`}</div>`, phase === 'working' ? 'detective' : 'avatar')}
        ${phase === 'working' ? `<div class="rec-grid">${Array.from({ length: 4 }, () => '<div class="rec skel-rec"><div class="skel skel-poster"></div><div class="stack gap-2 grow"><div class="skel skel-line" style="width:70%"></div><div class="skel skel-line" style="width:40%"></div><div class="skel skel-line"></div><div class="skel skel-line" style="width:85%"></div></div></div>').join('')}</div>` : `
        ${momoMsg(`<p><b>Momo thinks you’d love these.</b> Same puzzle-box hook as Severance, about half the dread. Two are short enough to finish this week.</p>`)}
        <div class="rec-grid stagger">${(D.askResults[tid] || D.askResults.lighter).map(recCard).join('')}</div>
        <div class="refine"><span class="t-label ink-2">Refine</span><div class="chips">${['Lighter', 'Shorter', 'Newer', 'Only movies', 'Only anime'].map((c) => chip(c, { act: 'refine', arg: c })).join('')}</div></div>`}`;
    }
    return {
      title: 'Ask momoshtrem', nav: null, focus: true,
      html: `<div class="ask-page">
        ${askThreads(thread || (phase !== 'idle' ? tid : null))}
        <div class="ask-main">
          <header class="ask-head">${iconBtn('arrow-left', 'Back', { act: 'back', cls: 'hide-desk' })}<div class="grow"><h1 class="t-title-3">Ask momoshtrem</h1><p class="t-caption ink-3">Momo uses your Trakt history · picks cost a few cents</p></div>${iconBtn('history', 'Past threads', { act: 'toast', arg: 'Threads: Today, Yesterday, Last week', cls: 'hide-desk' })}</header>
          <div class="ask-convo" aria-live="polite">${convo}</div>
          <form class="composer" data-act="noop" onsubmit="return false">
            <label class="sr-only" for="ask-input">Ask momoshtrem</label>
            <textarea id="ask-input" rows="1" placeholder="${phase === 'results' ? 'Ask a follow-up…' : 'Ask for a mood, a length, a vibe…'}"></textarea>
            ${iconBtn('send', 'Send', { cls: 'send', act: 'ask-send' })}
          </form>
        </div>
      </div>`,
    };
  };
  function runAsk() {
    clearInterval(window._askTimer);
    window._askTimer = setInterval(() => {
      S.ask.step++;
      if (S.ask.step >= STEPS.length) { clearInterval(window._askTimer); S.ask.phase = 'results'; }
      if ((parseRoute().parts[0] || '') === 'ask') render();
    }, 1100);
  }
  Object.assign(ACT, {
    'ask-send': () => { const v = document.getElementById('ask-input').value.trim() || 'something like Severance but lighter'; go(`#/ask?q=${encodeURIComponent(v)}`); },
    refine: (c) => { S.ask = { q: `${S.ask.q || 'something like Severance but lighter'} · ${c.toLowerCase()}`, phase: 'working', step: 0, thread: 'lighter' }; runAsk(); render(); },
    'ask-add': (id) => { S.added[id] = true; render(); toast({ text: `${T(id).title} is in your library`, sub: 'Next: find a torrent for it.', momo: 'proud' }); },
    'fb-inline': (arg) => { const [id, k] = arg.split(':'); S.feedback[id] = S.feedback[id] === k ? undefined : k; render(); },
    'add-lib': (id) => { S.added[id] = true; setParams({ sheet: null, fid: null }); toast({ text: `${T(id).title} added to your library`, sub: 'Find a torrent whenever you’re ready.', momo: 'proud', action: 'Find torrent', act: 'noop' }); },
  });

  // ================================================================== 4 · Discover
  const GENRES = ['Drama', 'Comedy', 'Sci-Fi', 'Mystery', 'Thriller', 'Animation', 'Fantasy', 'Adventure', 'Crime', 'Romance', 'Family', 'Documentary'];
  const MOODS = [['cozy', 'Cozy', 'popcorn'], ['twisty', 'Twisty', 'shapes'], ['short', 'Under 90 min', 'timer'], ['bingeable', 'Bingeable', 'list-video'], ['funny', 'Funny', 'flame'], ['beautiful', 'Beautiful', 'gem'], ['late', 'Late-night', 'moon']];
  const DISCOVER_POOL = ['severance', 'the-pitt', 'frieren', 'slow-horses', 'andor', 'dune-part-two', 'past-lives', 'perfect-days', 'silo', 'dark', 'pluribus', 'murderbot', 'the-good-place', 'delicious-in-dungeon', 'apothecary-diaries', 'shogun', 'the-bear', 'aftersun', 'arrival', 'mushishi', 'station-eleven', 'russian-doll', 'pachinko', 'drive-my-car', 'interstellar', 'vinland-saga', 'columbus', 'heat'];
  const discoverFilters = (p, inSheet) => {
    const g = (p.get('genres') || '').split(',').filter(Boolean);
    const type = p.get('type') || 'all';
    return `<div class="filters ${inSheet ? 'in-sheet' : ''}">
      <fieldset class="fgroup"><legend>Type</legend>${U.seg([['all', 'All'], ['movie', 'Movies'], ['tv', 'Shows']], type, { act: 'dfilter-type', label: 'Type', block: true })}</fieldset>
      <fieldset class="fgroup"><legend>Genres <span class="ink-3">· pick any</span></legend><div class="chips">${GENRES.map((x) => chip(x, { on: g.includes(x), act: 'dfilter-genre', arg: x })).join('')}</div></fieldset>
      <fieldset class="fgroup"><legend>Year</legend><div class="range-row"><label class="sr-only" for="y1">From year</label><input class="input" id="y1" inputmode="numeric" value="${p.get('from') || '2015'}"><span class="ink-3">to</span><label class="sr-only" for="y2">To year</label><input class="input" id="y2" inputmode="numeric" value="${p.get('to') || '2026'}"></div></fieldset>
      <fieldset class="fgroup"><legend>Rating <span class="ink-3">· TMDB ${p.get('rating') || '7.0'}+</span></legend><input class="range" type="range" min="0" max="9" step="0.5" value="${p.get('rating') || 7}" aria-label="Minimum rating"></fieldset>
      <fieldset class="fgroup"><legend>Runtime</legend><div class="chips">${['Under 90 min', '90–120 min', 'Over 2 h', 'Episodes < 30 min'].map((x, i) => chip(x, { on: i === 0 && p.get('runtime') === 'short' })).join('')}</div></fieldset>
      <fieldset class="fgroup"><legend>Language &amp; country</legend>
        <label class="switch-row"><span class="grow"><b>Anime</b><small>Japanese animation only</small></span><input type="checkbox" class="switch" ${p.get('anime') ? 'checked' : ''} data-act="dfilter-anime" aria-label="Anime only"></label>
        <label class="field-label sr-only" for="lang">Original language</label><select class="input" id="lang"><option>Any language</option><option>Japanese</option><option>Korean</option><option>English</option><option>German</option></select></fieldset>
      <fieldset class="fgroup"><legend>Status</legend><div class="chips">${['Returning', 'Ended', 'In production'].map((x) => chip(x)).join('')}</div></fieldset>
    </div>`;
  };
  SCREENS.discover = (ctx) => {
    const p = ctx.p;
    const g = (p.get('genres') || '').split(',').filter(Boolean);
    const mood = p.get('mood');
    const empty = ctx.state === 'empty';
    const active = [...g.map((x) => [x, 'genre']), ...(p.get('anime') ? [['Anime', 'anime']] : []), ...(p.get('type') && p.get('type') !== 'all' ? [[p.get('type') === 'movie' ? 'Movies' : 'Shows', 'type']] : [])];
    let pool = DISCOVER_POOL.filter((id) => !g.length || g.some((x) => T(id).genres.includes(x)));
    if (p.get('anime')) pool = pool.filter((id) => T(id).anime);
    if (p.get('type') === 'movie') pool = pool.filter((id) => T(id).type === 'movie');
    if (p.get('type') === 'tv') pool = pool.filter((id) => T(id).type === 'show');
    if (empty) pool = [];
    const grid = pool.length ? `<div class="grid-posters stagger">${pool.map((id) => U.posterCard(id, { status: D.libState[id] ? U.pill('added', 'In library', { sm: true }) : '' })).join('')}
        ${Array.from({ length: 6 }, () => '<div class="stack gap-2" aria-hidden="true"><div class="skel skel-poster"></div><div class="skel skel-line" style="width:75%"></div></div>').join('')}</div>
        <p class="load-more" role="status">${U.pawTrack(4, 2)} Loading more…</p>`
      : U.emptyState('detective', 'Nothing matches all of that', `No titles are ${active.map((a) => a[0]).join(' + ')} from ${p.get('from') || 2015}–${p.get('to') || 2026}. Loosen one filter:`, active.slice(0, 3).map((a) => chip(a[0], { x: true, act: 'dfilter-remove', arg: a.join(':') })).join('') + btn('Clear all', { variant: 'ghost', size: 'sm', act: 'dfilter-clear' }));
    return {
      title: 'Discover', nav: 'discover',
      html: `<div class="page">
        ${phead('Discover', { sub: 'Browse everything on TMDB, filtered your way.' })}
        <div class="moods" role="group" aria-label="Moods">${MOODS.map(([k, label, ic]) => `<button class="mood ${mood === k ? 'on' : ''}" data-act="mood" data-arg="${k}" aria-pressed="${mood === k}">${icon(ic)}<span>${label}</span></button>`).join('')}</div>
        <div class="discover">
          <aside class="discover-side" aria-label="Filters"><h2 class="t-headline">Filters</h2>${discoverFilters(p)}</aside>
          <div class="discover-main">
            <div class="discover-bar">
              ${btn(`Filters${active.length ? ` <span class="count-badge">${active.length}</span>` : ''}`, { variant: 'secondary', size: 'sm', icon: 'sliders-horizontal', act: 'dfilters-open', cls: 'filters-btn' })}
              <span class="t-label ink-2 result-count">${empty ? 0 : 248} titles</span>
              <span class="spacer"></span>
              <label class="sr-only" for="dsort">Sort</label><select class="input sort-select" id="dsort"><option>Popular</option><option>Top rated</option><option>Newest</option><option>For you</option></select>
            </div>
            ${active.length ? `<div class="chips active-filters">${active.map((a) => chip(a[0], { on: true, check: false, x: true, act: 'dfilter-remove', arg: a.join(':') })).join('')}${btn('Clear', { variant: 'ghost', size: 'sm', act: 'dfilter-clear' })}</div>` : ''}
            ${grid}
          </div>
        </div>
      </div>`,
    };
  };
  SHEETS.dfilters = (ctx) => U.sheet({ id: 'df', title: 'Filters', cls: 'full', body: discoverFilters(ctx.p, true), foot: `${btn('Clear all', { variant: 'ghost', act: 'dfilter-clear' })}<span class="spacer"></span>${btn('Show 248 titles', { variant: 'primary', act: 'close-sheet' })}` });
  const editList = (key, val, on) => { const r = parseRoute(); const cur = (r.params.get(key) || '').split(',').filter(Boolean); const next = on ? [...new Set([...cur, val])] : cur.filter((x) => x !== val); return next.length ? next.join(',') : null; };
  Object.assign(ACT, {
    'dfilters-open': () => setParams({ sheet: 'dfilters' }),
    'dfilter-type': (v) => setParams({ type: v === 'all' ? null : v }, true),
    'dfilter-genre': (v) => { const on = !(parseRoute().params.get('genres') || '').split(',').includes(v); setParams({ genres: editList('genres', v, on) }, true); },
    'dfilter-anime': (a, el) => setParams({ anime: el.checked ? '1' : null }, true),
    'dfilter-remove': (arg) => { const [v, k] = arg.split(':'); if (k === 'genre') setParams({ genres: editList('genres', v, false) }, true); if (k === 'anime') setParams({ anime: null }, true); if (k === 'type') setParams({ type: null }, true); },
    'dfilter-clear': () => setParams({ genres: null, anime: null, type: null, mood: null, state: null }, true),
    mood: (k) => setParams({ mood: parseRoute().params.get('mood') === k ? null : k }, true),
  });

  // ================================================================== 5 · Search (+ palette)
  const searchPool = (q) => Object.values(D.T).filter((t) => t.title.toLowerCase().includes(q.toLowerCase()) || (t.original || '').toLowerCase().includes(q.toLowerCase()));
  const resultRow = (t) => `<li><a class="sr-row" href="#/${t.type}/${t.id}">${U.poster(t.id, { cls: 'thumb' })}<span class="grow"><b>${esc(t.title)}</b><span>${t.type === 'movie' ? 'Movie' : 'Show'} · ${t.year}${t.original ? ` · ${esc(t.original)}` : ''}</span></span>${D.libState[t.id] ? U.pill('added', 'In library', { sm: true }) : ''}${icon('chevron-right', 'ico ico-sm ink-3')}</a></li>`;
  SCREENS.search = (ctx) => {
    const q = ctx.p.get('q') ?? (ctx.state === 'results' ? 'fri' : ctx.state === 'none' ? 'freiren' : '');
    const tab = ctx.p.get('tab') || 'all';
    const res = q ? searchPool(q) : [];
    const movies = res.filter((t) => t.type === 'movie'), shows = res.filter((t) => t.type === 'show');
    const inLib = res.filter((t) => D.libState[t.id]);
    const list = tab === 'movies' ? movies : tab === 'shows' ? shows : tab === 'library' ? inLib : res;
    let body;
    if (!q) {
      body = `<section class="section"><h2 class="t-headline">Recent</h2><div class="chips" style="margin-top:12px">${['Frieren', 'Dune', 'Severance s2', 'Bluey'].map((x) => `<a class="chip outline" href="#/search?q=${encodeURIComponent(x)}">${icon('history', 'ico ico-sm')}${x}</a>`).join('')}</div></section>
        <section class="section"><h2 class="t-headline">Trending searches</h2><ul class="list" style="margin-top:12px">${['pluribus', 'the-pitt', 'murderbot', 'slow-horses'].map((id) => resultRow(T(id))).join('')}</ul></section>`;
    } else if (!res.length) {
      body = U.emptyState('detective', `No matches for “${esc(q)}”`, `Momo checked titles and original names. Did you mean <a href="#/search?q=Frieren">Frieren</a>?`, btn('Ask momoshtrem instead', { variant: 'secondary', icon: 'sparkles', href: `#/ask?q=${encodeURIComponent(q)}` }));
    } else {
      body = `${U.tabs([['all', 'All', res.length], ['movies', 'Movies', movies.length], ['shows', 'Shows', shows.length], ['library', 'In library', inLib.length]], tab, { act: 'stab', label: 'Result type', scroll: true })}
        <ul class="list search-list">${list.map(resultRow).join('')}</ul>
        <div class="grid-posters search-grid">${list.map((t) => U.posterCard(t.id, { status: D.libState[t.id] ? U.pill('added', 'In library', { sm: true }) : '' })).join('')}</div>`;
    }
    return {
      title: 'Search', nav: 'search',
      html: `<div class="page">
        ${phead('Search')}
        <div class="input-wrap search-field big">${icon('search')}<label class="sr-only" for="q">Search movies and shows</label><input class="input" id="q" type="search" placeholder="Movies, shows, original titles" value="${esc(q)}" autocomplete="off" data-act="noop">
          ${q ? `<span class="input-end">${iconBtn('x', 'Clear search', { cls: 'sm', act: 'sclear' })}</span>` : ''}</div>
        ${body}
      </div>`,
      after: () => { const i = document.getElementById('q'); if (i && !i._wired) { i._wired = true; i.addEventListener('input', () => { clearTimeout(i._t); i._t = setTimeout(() => setParams({ q: i.value || null }, true), 180); }); } },
    };
  };
  Object.assign(ACT, { stab: (v) => setParams({ tab: v }, true), sclear: () => setParams({ q: null, tab: null }, true) });

  SHEETS.palette = (ctx) => {
    const q = ctx.p.get('pq') || '';
    const titles = (q ? searchPool(q) : ['severance', 'dune-part-two', 'frieren'].map(T)).slice(0, 4);
    const lib = (q ? searchPool(q).filter((t) => D.libState[t.id]) : ['the-bear', 'shogun'].map(T)).slice(0, 3);
    const actions = [['sparkles', 'Ask momoshtrem', '#/ask'], ['activity', 'Go to Activity', '#/activity'], ['link', 'Connect Trakt', '#/settings?trakt=start'], ['moon', 'Switch theme', null]].filter((a) => !q || a[1].toLowerCase().includes(q.toLowerCase()));
    let k = 0;
    const item = (inner, href, act) => `<${href ? `a href="${href}"` : `button data-act="${act}"`} class="palette-item" role="option" aria-selected="${k++ === 0}">${inner}</${href ? 'a' : 'button'}>`;
    return `<div class="scrim" style="z-index:calc(var(--z-palette) - 1)" data-act="close-palette"></div>
      <section class="palette" role="dialog" aria-modal="true" aria-label="Command palette">
        <div class="palette-input">${icon('search')}<label class="sr-only" for="pq">Search titles, library and actions</label><input id="pq" value="${esc(q)}" placeholder="Search titles, your library, actions…" autocomplete="off">${UI.kbd('esc')}</div>
        <div class="palette-body" role="listbox">
          ${titles.length ? `<p class="palette-group">Titles</p>${titles.map((t) => item(`${U.poster(t.id)}<span class="pi-text"><b>${esc(t.title)}</b><span>${t.type === 'movie' ? 'Movie' : 'Show'} · ${t.year}</span></span>${D.libState[t.id] ? U.pill('added', 'In library', { sm: true }) : ''}`, `#/${t.type}/${t.id}`)).join('')}` : ''}
          ${lib.length ? `<p class="palette-group">In your library</p>${lib.map((t) => item(`${U.poster(t.id)}<span class="pi-text"><b>${esc(t.title)}</b><span>${D.libState[t.id]?.ready ?? ''}${D.libState[t.id]?.total ? `/${D.libState[t.id].total} ready` : 'Ready'}</span></span>`, `#/${t.type}/${t.id}`)).join('')}` : ''}
          ${actions.length ? `<p class="palette-group">Actions</p>${actions.map(([ic, label, href]) => item(`<span class="pi-ico">${icon(ic)}</span><span class="pi-text"><b>${label}</b></span>`, href, href ? null : 'theme')).join('')}` : ''}
        </div>
        <div class="palette-foot"><span>${UI.kbd('↑')}${UI.kbd('↓')} move</span><span>${UI.kbd('↵')} open</span><span>${UI.kbd('esc')} close</span></div>
      </section>`;
  };
  document.addEventListener('input', (e) => { if (e.target.id === 'pq') { clearTimeout(e.target._t); e.target._t = setTimeout(() => setParams({ pq: e.target.value || null }, true), 120); } });
})();
