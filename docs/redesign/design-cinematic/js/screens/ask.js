/* Ask momoshtrem: conversational discovery with Noir.
   Routes: /ask, /ask/:thread. States: empty · working (mid-stream) · results · error
   #/ask?q=… streams live: progress steps, then the intro, then cards one by one
   (mirrors POST /recommendations/ask: status → text → item… → done). */
(function () {
  const { ui, app, brand, icon, data: D } = MS;
  const esc = ui.esc;
  const T = (id) => D.T[id];
  const reduced = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ---- local fixtures ------------------------------------------------------
  const SETS = {
    severance: { intro: 'Five picks with the same “what is this place, really?” itch as Severance, but warmer and funnier. Lightest first.', items: D.askResults },
    thriller: { intro: 'Three that fit before midnight. One is exactly 90 minutes; the others earn their extra time.', items: [
      { id: 'paprika', match: '90 min · dream thriller', reason: 'Exactly 90 minutes. A detective chases a stolen dream machine through Tokyo, and Satoshi Kon never lets the pace drop.' },
      { id: 'arrival', match: 'Tense · quiet', reason: 'Just under two hours and tense all the way. You gave Dune: Part Two a 9, and this is Villeneuve at his most precise.' },
      { id: 'slowhorses', match: 'Two episodes = one film', reason: 'Each episode is about 45 minutes; two make a tight spy-thriller evening. Same dry humour you liked in Severance.' },
    ] },
    anime: { intro: 'Gentle, episodic and quietly moving, like Frieren. One is already in your library.', items: [
      { id: 'mushishi', match: 'Calm · episodic wonder', reason: 'The closest thing to Frieren’s unhurried wandering. Each episode is a self-contained folk tale about strange, beautiful life.' },
      { id: 'totoro', match: 'In your library', reason: 'Already downloaded and still unwatched. Ninety minutes of the same warmth Frieren has between its fights.' },
      { id: 'dandadan', match: 'Same craft, more energy', reason: 'Louder and sillier, but with the attention to small character moments you rated highly in Frieren.' },
      { id: 'paprika', match: 'A 90-minute dream', reason: 'If you want one evening rather than a series: hand-drawn, dreamlike and over in 90 minutes.' },
    ] },
    parents: { intro: 'Four films that work across generations: no gore, nothing awkward, and something to talk about after.', items: [
      { id: 'perfectdays', match: 'Gentle · moving', reason: 'A quiet life in Tokyo told with enormous warmth. It plays well to anyone who likes a slow Sunday.' },
      { id: 'pastlives', match: 'Bittersweet', reason: 'Small, adult and devastating in the best way. Subtitled in parts, mostly English.' },
      { id: 'totoro', match: 'In your library', reason: 'Ghibli is safe for every age, and it’s already ready in Infuse.' },
      { id: 'arrival', match: 'Smart sci-fi', reason: 'Big ideas without a single explosion. Good for a family that argues about endings.' },
    ] },
  };
  const REFINE = {
    lighter: { label: 'Lighter', intro: 'Even lighter: comfort first, no dread at all.', items: [
      { id: 'mythicquest', match: 'Office comedy · zero dread', reason: 'All of Severance’s corporate absurdity, none of the menace. Half-hour episodes that end on a laugh.' },
      { id: 'goodplace', match: 'Mystery, played for laughs', reason: 'Still the best match: the mystery is there, but it’s kind and very funny.' },
      { id: 'bluey', match: '7-minute joy', reason: 'You already have every episode. Grown-up enough to be the funniest thing in your library.' },
    ] },
    shorter: { label: 'Shorter', intro: 'All under 30 minutes an episode, so one more is always an option.', items: [
      { id: 'goodplace', match: '22-min episodes', reason: 'Fast and twisty; most seasons end in under five hours.' },
      { id: 'mythicquest', match: '30-min episodes', reason: 'Tight workplace comedy, four short seasons.' },
      { id: 'studio', match: '30-min episodes', reason: 'Long takes, short episodes, and you rated Seth Rogen’s last show highly.' },
    ] },
    newer: { label: 'Newer', intro: 'Everything here is from the last three years.', items: [
      { id: 'studio', match: '2025 · new season', reason: 'Season 2 just started. Workplace satire with Severance-level production design.' },
      { id: 'pluribus', match: '2025 · Vince Gilligan', reason: 'Strange, dry and funny. Your highest-rated corner of Trakt is exactly this.' },
      { id: 'shrinking', match: '2023 · warm', reason: 'Season 3 is airing. Warm and funny about work and grief.' },
    ] },
    movies: { label: 'Only movies', intro: 'One evening, no commitment.', items: [
      { id: 'arrival', match: 'Quiet mystery', reason: 'Severance’s unease about memory, solved in two hours, and gentler than you’d expect.' },
      { id: 'paprika', match: 'Mind-bending · 90 min', reason: 'Dreams leaking into the office. Playful rather than bleak.' },
      { id: 'totoro', match: 'Pure comfort', reason: 'As light as it gets, and it’s already in your library.' },
    ] },
    anime: { label: 'Only anime', intro: 'Anime with the same “something is off here” hook, kept light.', items: [
      { id: 'dandadan', match: 'Weird · funny', reason: 'Supernatural mysteries played as a comedy. High energy, kind heart.' },
      { id: 'mushishi', match: 'Gentle mystery', reason: 'Each episode solves a small, strange mystery. Calm rather than tense.' },
      { id: 'paprika', match: 'One evening', reason: 'A 90-minute film about dreams invading work. Very Severance, much brighter.' },
    ] },
  };
  const THREAD_Q = Object.fromEntries(D.askThreads.map((t) => [t.id, t.title]));
  const setFor = (q) => (/anime|frieren/i.test(q) ? SETS.anime : /thriller|90/i.test(q) ? SETS.thriller : /parent|family/i.test(q) ? SETS.parents : SETS.severance);
  const threadFor = (q) => D.askThreads.find((t) => t.title.toLowerCase() === String(q).toLowerCase().trim());

  const STEPS = [
    ['Reading your history…', 'Read your history', '2,184 plays · you loved Severance, The Bear, Ted Lasso'],
    ['Searching the web for new releases…', 'Searched the web for new releases', '14 new since Oct 1'],
    ['Checking TMDB…', 'Checked TMDB', '23 candidates · 6 already seen'],
    ['Writing reasons…', 'Wrote reasons', '5 picks'],
  ];
  const REFINE_STEPS = [['Re-ranking the candidates…', 'Re-ranked the candidates', '23 checked'], ['Writing reasons…', 'Wrote reasons', '3 picks']];
  const EXAMPLES = ['something like Severance but lighter', 'a 90-minute thriller for tonight', 'anime with a Frieren vibe', 'a cosy show to fall asleep to', 'films to watch with my parents', 'what’s new this month for me?'];

  // ---- pieces ---------------------------------------------------------------
  const userMsg = (text) => `<div class="msg msg--me"><p>${esc(text)}</p></div>`;
  const step = (s, st, n) => `<div class="step ${st === 'done' ? 'is-done' : st === 'active' ? 'is-active' : ''}" data-step="${n}"><span class="ic">${st === 'done' ? icon('check', { size: 14, stroke: 2.5 }) : ''}</span><span>${st === 'done' ? s[1] : s[0]}</span>${st === 'done' && s[2] ? `<span class="detail">${esc(s[2])}</span>` : ''}</div>`;
  const stepsLive = (steps, done, active) => steps.map((s, i) => step(s, i < done ? 'done' : i === active ? 'active' : 'pending', i)).join('');
  const trace = (steps, secs) => `<details class="ask-trace"><summary>${icon('checkCheck', { size: 15 })}<span>${steps.length === 4 ? 'History, web and TMDB checked' : 'Re-ranked your picks'} · ${secs} s</span>${icon('chevronDown', { size: 15, cls: 'chev' })}</summary><div class="steps">${steps.map((s, i) => step(s, 'done', i)).join('')}</div></details>`;

  const inLib = (t) => (t.id === 'goodplace' ? app.state.goodplace === 'added' : t.lib && t.lib !== 'none');
  function cardActions(t) {
    if (inLib(t)) {
      const pill = t.lib && t.lib !== 'none' ? ui.libPill(t) : ui.pill('ready', 'In library', { icon: 'check' });
      const next = t.lib && t.lib !== 'none' ? `<a class="btn btn-secondary btn-sm" href="#/title/${t.type}/${t.id}">Open</a>` : `<button class="btn btn-secondary btn-sm" data-act="${t.id === 'goodplace' ? 'find-goodplace' : 'find'}" data-show="${t.id}">${icon('search', { size: 15 })}Find a torrent</button>`;
      return `<span class="ask-inlib">${pill}${next}</span>`;
    }
    return `<button class="btn btn-primary btn-sm" data-act="ask-add" data-id="${t.id}">${icon('plus', { size: 16 })}Add to library</button>`;
  }
  function card(it, i) {
    const t = T(it.id); if (!t) return '';
    const href = `#/title/${t.type}/${t.id}`;
    const meta = [t.type === 'tv' ? (t.years || t.year) : t.year, t.type === 'tv' ? (t.seasons ? `${t.seasons.length} season${t.seasons.length > 1 ? 's' : ''}` : 'Show') : 'Movie', t.runtime, `<span class="star">${icon('star', { size: 12, fill: true })}${t.rating}</span>`].filter(Boolean);
    return `<article class="ask-card" data-id="${t.id}" style="--i:${i}">
      <a class="ask-poster" href="${href}" aria-label="${esc(t.title)} details">${ui.poster(t.id)}</a>
      <div class="ask-card-body">
        <span class="ask-match">${icon('sparkle', { size: 13 })}${esc(it.match)}</span>
        <h3 class="t-title-3"><a href="${href}">${esc(t.title)}</a></h3>
        <p class="ask-meta">${meta.map((m) => `<span>${m}</span>`).join('')}</p>
        <p class="ask-reason">${esc(it.reason)}</p>
        <div class="ask-actions">
          <span class="ask-primary">${cardActions(t)}</span>
          <a class="btn btn-ghost btn-sm" href="${href}">Details</a>
          <span class="ask-fb" role="group" aria-label="Feedback on ${esc(t.title)}">
            <button class="btn btn-ghost btn-sm btn-icon" data-act="ask-fb" data-fb="like" aria-pressed="false" aria-label="More like this">${icon('thumbsUp', { size: 16 })}</button>
            <button class="btn btn-ghost btn-sm btn-icon" data-act="ask-fb" data-fb="dislike" aria-pressed="false" aria-label="Less like this">${icon('thumbsDown', { size: 16 })}</button>
            <button class="btn btn-ghost btn-sm btn-icon" data-act="ask-fbmore" data-id="${t.id}" aria-label="More feedback: not interested, already seen">${icon('more', { size: 16 })}</button>
          </span>
        </div>
      </div>
    </article>`;
  }
  const skelCard = () => `<div class="ask-card is-skel" aria-hidden="true"><div class="skel skel-poster"></div><div class="stack gap-3 grow"><div class="skel skel-line" style="width:34%"></div><div class="skel skel-line" style="width:62%;height:18px"></div><div class="skel skel-line" style="width:90%"></div><div class="skel skel-line" style="width:76%"></div></div></div>`;
  const refineRow = (used) => `<div class="ask-refine" role="group" aria-label="Refine these picks"><span class="t-small c-3">Refine</span>${Object.entries(REFINE).map(([k, r]) => `<button class="fchip" data-act="ask-refine" data-r="${k}" aria-pressed="${used === k}">${esc(r.label)}</button>`).join('')}</div>`;

  function noirTurn({ set, steps = STEPS, phase = 'done', secs = 9, refined }) {
    const who = `<div class="msg-who">${brand.avatar(30)}<span class="w-semibold">Noir</span><span class="t-small c-3" data-status>${phase === 'done' ? 'just now' : 'Thinking…'}</span></div>`;
    if (phase === 'done') {
      return `<article class="msg msg--noir">${who}<div class="msg-body">${trace(steps, secs)}<p class="ask-intro">${esc(set.intro)}</p><div class="ask-cards">${set.items.map(card).join('')}</div>${refineRow(refined)}</div></article>`;
    }
    if (phase === 'working') {
      return `<article class="msg msg--noir" aria-busy="true">${who}<div class="msg-body"><div class="steps ask-steps">${stepsLive(steps, 2, 2)}</div><div class="ask-cards">${card(set.items[0], 0)}${skelCard()}</div></div></article>`;
    }
    // stream: everything pending, mount fills it in
    return `<article class="msg msg--noir" aria-busy="true" data-stream>${who}<div class="msg-body"><div class="steps ask-steps" data-steps>${stepsLive(steps, 0, 0)}</div><p class="ask-intro" data-intro hidden></p><div class="ask-cards" data-cards></div><div data-refine></div></div></article>`;
  }

  function errorTurn() {
    return `<article class="msg msg--noir">
      <div class="msg-who">${brand.avatar(30)}<span class="w-semibold">Noir</span><span class="t-small c-3">just now</span></div>
      <div class="msg-body"><div class="ask-error" role="alert">
        <span class="ask-error-ic">${icon('coins', { size: 22 })}</span>
        <div class="stack gap-2" style="min-width:0">
          <h3 class="t-title-3">This month’s AI budget is used up</h3>
          <p class="c-2">You set $5.00 for October and it’s all spent, so asks and new picks pause until Nov 1. Everything else keeps working: your library, torrents and Infuse.</p>
          <div class="row gap-2 wrap" style="margin-top:var(--sp-2)"><a class="btn btn-secondary btn-sm" href="#/settings?section=ai">${icon('settings', { size: 15 })}Settings › AI</a><a class="btn btn-ghost btn-sm" href="#/discover?mood=cosy">Browse Discover instead</a></div>
        </div>
      </div></div>
    </article>`;
  }

  function hello() {
    return `<div class="ask-hello">
      <div class="ask-hello-cat">${brand.avatar(88)}</div>
      <h1 class="t-display">What are we in the mood for?</h1>
      <p class="c-2 t-body-l">Ask in your own words. I read your Trakt history, check what’s new, and tell you why each pick fits.</p>
      <div class="ask-examples" role="list" aria-label="Example asks">${EXAMPLES.map((q, i) => `<a role="listitem" class="ask-example${i < 3 ? ' is-main' : ''}" href="#/ask?q=${encodeURIComponent(q)}">${icon(i < 3 ? 'sparkle' : 'message', { size: 15 })}<span>${esc(q)}</span></a>`).join('')}</div>
    </div>`;
  }

  const budget = (err) => `<div class="ask-budget"><div class="row between t-small"><span class="c-2">AI this month</span><span class="t-num ${err ? 'c-1' : 'c-2'}">${err ? '$5.00' : '$1.84'} of $5.00</span></div>${ui.progress(err ? 100 : 37, err ? 'accent' : 'accent')}<a class="t-small c-3" href="#/settings?section=ai">Settings › AI</a></div>`;
  const threadList = (cur) => `<ul class="ask-threads">${D.askThreads.map((t) => `<li><a href="#/ask/${t.id}" ${cur === t.id ? 'aria-current="page"' : ''}>${icon('message', { size: 16 })}<span class="clamp-2">${esc(t.title)}</span><span class="t-small c-3">${esc(t.when)}</span></a></li>`).join('')}</ul>`;

  // ---- route ----------------------------------------------------------------
  function view(p) {
    const thread = p.thread;
    let state = p.state || (thread ? 'results' : p.q ? (p.done ? 'results' : 'stream') : 'empty');
    let q = p.q || (thread ? THREAD_Q[thread] : '') || '';
    if (!q && ['working', 'results'].includes(state)) q = 'Something like Severance but lighter';
    if (!q && state === 'error') q = 'A 90-minute thriller for tonight';
    const cur = thread || (q && threadFor(q)?.id) || null;
    const set = setFor(q);
    const switcher = ui.protoStates('#/ask', [['empty', 'Empty'], ['working', 'Working'], ['results', 'Results'], ['error', 'Budget reached']], state === 'stream' ? 'working' : state);

    let log = '';
    if (state === 'empty') log = hello();
    else if (state === 'error') log = `${userMsg(q)}${errorTurn()}`;
    else log = `${userMsg(q)}${noirTurn({ set, phase: state === 'results' ? 'done' : state })}`;

    const err = state === 'error';
    const html = `<div class="ask-screen" data-state="${state}">${switcher}
      <header class="ask-bar phone-only">
        <a class="btn btn-ghost btn-icon" href="#/home" aria-label="Back to For You">${icon('chevronLeft', { size: 24 })}</a>
        <div class="ask-bar-title">${brand.avatar(28)}<div class="stack"><span class="w-semibold">Ask momoshtrem</span><span class="t-small c-3">${err ? 'Paused until Nov 1' : 'Noir · knows your Trakt'}</span></div></div>
        <button class="btn btn-ghost btn-icon" data-act="ask-history" aria-label="Past conversations">${icon('history', { size: 22 })}</button>
        <a class="btn btn-ghost btn-icon ask-bar-new" href="#/ask" aria-label="New conversation">${icon('plus', { size: 22 })}</a>
      </header>
      <div class="ask-layout">
        <aside class="ask-rail desk-only" aria-label="Conversations">
          <div class="ask-rail-in">
            <a class="btn btn-secondary btn-block ask-new" href="#/ask">${icon('plus', { size: 18 })}New conversation</a>
            <p class="t-micro c-3 ask-rail-h">Recent</p>
            ${threadList(cur)}
            ${budget(err)}
          </div>
        </aside>
        <section class="ask-main" aria-label="Conversation">
          <div class="ask-log" id="ask-log" aria-live="polite">${log}</div>
          <form class="ask-composer" id="ask-form" autocomplete="off">
            <div class="ask-composer-in">
              <label class="sr-only" for="ask-input">Ask momoshtrem</label>
              <textarea id="ask-input" rows="1" placeholder="${err ? 'Asks are paused until Nov 1' : state === 'empty' ? 'Ask for anything to watch…' : 'Ask a follow-up, or refine…'}" enterkeyhint="send" ${err ? 'disabled' : ''}></textarea>
              <button type="submit" class="btn btn-primary btn-icon ask-send" aria-label="Send" ${err ? 'disabled' : ''}>${icon('arrowUp', { size: 20, stroke: 2.2 })}</button>
            </div>
            <p class="ask-note t-small c-3 desk-only">Noir uses your Trakt history and TMDB. Picks cost about $0.02 each.</p>
          </form>
        </section>
      </div>
    </div>`;

    return {
      title: 'Ask momoshtrem', noTabbar: true, html, mainCls: 'main--ask',
      mount(root) { mountAsk(root, { state, q, set }); },
    };
  }
  app.route('/ask', view);
  app.route('/ask/:thread', view);

  // ---- streaming -------------------------------------------------------------
  let run = 0;
  const later = (fn, ms, token) => setTimeout(() => { if (token === run) fn(); }, ms);
  function stream(turn, set, steps, token, done) {
    const fast = reduced();
    const STEP = fast ? 260 : 820, CARD = fast ? 120 : 460;
    const sEl = turn.querySelector('[data-steps]');
    const status = turn.querySelector('[data-status]');
    const t0 = Date.now();
    let i = 0;
    const next = () => {
      if (!document.contains(turn)) return;
      if (i < steps.length) {
        sEl.innerHTML = stepsLive(steps, i, i);
        i += 1;
        later(next, STEP + (i === 2 ? 300 : 0), token);
        return;
      }
      sEl.innerHTML = stepsLive(steps, steps.length, -1);
      status.textContent = 'Writing…';
      // intro text streams word by word
      const intro = turn.querySelector('[data-intro]');
      intro.hidden = false;
      const words = set.intro.split(' ');
      let w = 0;
      const typeWord = () => {
        if (fast) { intro.textContent = set.intro; w = words.length; }
        else { w += 1; intro.textContent = words.slice(0, w).join(' '); }
        if (w < words.length) later(typeWord, 28, token); else later(cards, 180, token);
      };
      typeWord();
    };
    const cardsEl = turn.querySelector('[data-cards]');
    let c = 0;
    const cards = () => {
      if (!document.contains(turn)) return;
      const sk = cardsEl.querySelector('.is-skel'); if (sk) sk.remove();
      if (c < set.items.length) {
        cardsEl.insertAdjacentHTML('beforeend', card(set.items[c], c).replace('class="ask-card"', 'class="ask-card is-new"'));
        c += 1;
        if (c < set.items.length) cardsEl.insertAdjacentHTML('beforeend', skelCard());
        later(cards, CARD, token);
        return;
      }
      const secs = Math.max(4, Math.round((Date.now() - t0) / 1000) + 3);
      sEl.outerHTML = trace(steps, secs);
      turn.querySelector('[data-refine]').outerHTML = refineRow();
      status.textContent = 'just now';
      turn.removeAttribute('aria-busy'); turn.removeAttribute('data-stream');
      if (done) done();
    };
    next();
  }

  function mountAsk(root, { state, q, set }) {
    run += 1;
    const token = run;
    const log = root.querySelector('#ask-log');
    const form = root.querySelector('#ask-form');
    const input = root.querySelector('#ask-input');

    const grow = () => { input.style.height = 'auto'; input.style.height = `${Math.min(input.scrollHeight, 160)}px`; };
    input.addEventListener('input', grow);
    input.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); form.requestSubmit(); } });
    form.addEventListener('submit', (e) => {
      e.preventDefault();
      const text = input.value.trim(); if (!text || input.disabled) return;
      if (state === 'empty') { app.go(`#/ask?q=${encodeURIComponent(text)}`); return; }
      input.value = ''; grow();
      followUp(log, text, setFor(text), STEPS, token);
    });

    if (state === 'stream') {
      const turn = log.querySelector('[data-stream]');
      stream(turn, set, STEPS, token, () => {
        const ps = root.querySelector('.proto-states');
        if (ps) { ps.querySelector('summary span').textContent = 'State: Results'; ps.querySelectorAll('a').forEach((a) => a.setAttribute('aria-current', String(/state=results/.test(a.getAttribute('href'))))); }
        const th = threadFor(q);
        try { history.replaceState(null, '', th ? `#/ask/${th.id}` : `#/ask?q=${encodeURIComponent(q)}&done=1`); } catch (e) { /* sandbox */ }
      });
    }
    if (state === 'empty' && window.matchMedia('(min-width: 1024px)').matches) setTimeout(() => input.focus({ preventScroll: true }), 50);
  }

  function scrollEnd(log, smooth) {
    const last = log.lastElementChild; if (!last) return;
    const top = last.getBoundingClientRect().top + window.scrollY - (window.matchMedia('(min-width: 1024px)').matches ? 88 : 72);
    if (top > window.scrollY + 40) window.scrollTo({ top, behavior: smooth && !reduced() ? 'smooth' : 'auto' });
  }

  function followUp(log, text, set, steps, token, refined) {
    log.insertAdjacentHTML('beforeend', `${userMsg(text)}${noirTurn({ set, steps, phase: 'stream' })}`);
    const turns = log.querySelectorAll('[data-stream]');
    const turn = turns[turns.length - 1];
    scrollEnd(log, true);
    stream(turn, set, steps, token, () => {
      if (refined) { const b = turn.querySelector(`[data-r="${refined}"]`); if (b) b.setAttribute('aria-pressed', 'true'); }
    });
  }

  // ---- actions ------------------------------------------------------------------
  Object.assign(app.actions, {
    'ask-add': (b) => {
      const id = b.dataset.id; const t = T(id); if (!t) return;
      b.classList.add('is-loading'); b.disabled = true;
      setTimeout(() => {
        if (id === 'goodplace') { app.state.goodplace = 'added'; app.save(); }
        document.querySelectorAll(`.ask-card[data-id="${id}"] .ask-primary`).forEach((el) => {
          el.innerHTML = `<span class="ask-inlib">${ui.pill('ready', 'In library', { icon: 'check' })}<button class="btn btn-secondary btn-sm" data-act="${id === 'goodplace' ? 'find-goodplace' : 'find'}" data-show="${id}">${icon('search', { size: 15 })}Find a torrent</button></span>`;
        });
        if (id === 'goodplace') app.toast('added');
        else app.toast(null, { kind: 'ready', title: 'Added to library', body: `${t.title}${t.seasons ? ` · ${t.seasons.length} season${t.seasons.length > 1 ? 's' : ''}` : ''}`, action: 'Open', href: `#/title/${t.type}/${t.id}` });
      }, 650);
    },
    'ask-fb': (b) => {
      const on = b.getAttribute('aria-pressed') !== 'true';
      b.parentElement.querySelectorAll('[data-act="ask-fb"]').forEach((x) => x.setAttribute('aria-pressed', 'false'));
      b.setAttribute('aria-pressed', String(on));
      if (!on) return;
      const t = T(b.closest('.ask-card')?.dataset.id);
      if (b.dataset.fb === 'dislike') { b.closest('.ask-card')?.classList.add('is-dimmed'); app.toast('feedback'); }
      else app.toast(null, { kind: 'info', icon: 'thumbsUp', title: `Noted. More like ${t ? t.title : 'this'}`, body: 'Your next picks will lean this way.' });
    },
    'ask-fbmore': (b) => app.openSheet('feedback', { id: b.dataset.id }),
    'ask-refine': (b) => {
      if (b.getAttribute('aria-pressed') === 'true') return;
      const r = REFINE[b.dataset.r]; const log = document.getElementById('ask-log'); if (!r || !log) return;
      b.closest('.ask-refine').querySelectorAll('.fchip').forEach((x) => { x.disabled = true; });
      b.setAttribute('aria-pressed', 'true');
      followUp(log, r.label, r, REFINE_STEPS, run, b.dataset.r);
    },
    'ask-history': () => app.openSheet('ask-history'),
  });

  app.sheet('ask-history', () => ({
    cls: 'sheet--narrow ask-hist-sheet',
    html: `<div class="sheet-grabber" aria-hidden="true"></div>
      <div class="sheet-head"><div class="sheet-title"><h2 class="t-title-3" id="ask-history-title">Conversations</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close" autofocus>${icon('x')}</button></div></div>
      <div class="sheet-body stack gap-4">
        <a class="btn btn-secondary btn-block" href="#/ask">${icon('plus', { size: 18 })}New conversation</a>
        ${threadList(app.path.startsWith('/ask/') ? app.path.split('/')[2] : null)}
        ${budget(app.params.state === 'error')}
      </div>`,
  }));
})();
