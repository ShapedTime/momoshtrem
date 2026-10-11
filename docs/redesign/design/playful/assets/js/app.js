/* The prototype app: router, state, shell, overlays, actions and guided flows.
   URL:  app.html?theme=light|dark&safe=1&flow=1|2|3&motion=reduce  #/route/arg?state=…&sheet=…  */
(function () {
  const { icon, momo, btn, iconBtn, esc } = UI;
  const qs = new URLSearchParams(location.search);
  const root = document.documentElement;
  const sysDark = matchMedia('(prefers-color-scheme: dark)').matches;
  root.dataset.theme = qs.get('theme') || (sysDark ? 'dark' : 'light');
  if (qs.get('motion')) root.dataset.motion = qs.get('motion');
  if (qs.get('safe')) { root.style.setProperty('--sat', '47px'); root.style.setProperty('--sab', '34px'); root.classList.add('sim-safe'); }
  const FLOW = qs.get('flow');
  // In a guided flow on phone, the guide card takes the top: push the layout down so nothing hides under it.
  if (FLOW && innerWidth < 1024) { const top = qs.get('safe') ? 47 : 0; root.style.setProperty('--flow-top', `${top}px`); root.style.setProperty('--sat', `${top + 92}px`); }

  // ------------------------------------------------------------------ state (lives for the session)
  const S = (window.S = {
    trakt: qs.get('trakt') || 'connected', // off | code | waiting | syncing | connected
    picks: qs.get('picks') || 'ready', // ready | refreshing | empty
    movieState: {}, // per-movie overrides: none | added | downloading | ready
    added: {}, // titles added from Ask / Discover
    frierenReady: false,
    ts: { ctx: 'severance-s02e05', query: 'Severance S02E05', scope: 'episode', filters: {}, sort: 'best', shown: 8, arg: 'severance:2:5' },
    job: {}, // per job: { phase, resolved, applied, picks }
    ask: { thread: null, phase: 'idle', step: 0 },
    feedback: {},
    lib: { view: 'grid', filter: 'all', removed: {} },
    discover: { open: false },
    toasts: [],
    dialog: null,
    menu: null,
    flowStep: 0,
  });

  // ------------------------------------------------------------------ routing
  const parse = () => {
    const h = location.hash.slice(1) || '/for-you';
    const [path, query] = h.split('?');
    return { parts: path.split('/').filter(Boolean), params: new URLSearchParams(query || ''), path };
  };
  const hashOf = (parts, params) => `#/${parts.join('/')}${params && [...params].length ? `?${params}` : ''}`;
  const setParams = (changes, replace) => {
    const r = parse();
    Object.entries(changes).forEach(([k, v]) => (v === null || v === undefined ? r.params.delete(k) : r.params.set(k, v)));
    const h = hashOf(r.parts, r.params);
    if (replace) { history.replaceState(null, '', h); render(); } else location.hash = h;
  };
  const go = (h) => { location.hash = h; };

  const SCREENS = (window.SCREENS = window.SCREENS || {});
  const SHEETS = (window.SHEETS = window.SHEETS || {});
  const ACT = (window.ACT = window.ACT || {});

  // ------------------------------------------------------------------ shell
  const NAV = [
    ['for-you', 'For You', 'heart'],
    ['discover', 'Discover', 'compass'],
    ['search', 'Search', 'search'],
    ['library', 'Library', 'library-big'],
    ['activity', 'Activity', 'activity'],
  ];
  const mark = () => '<img class="brand-mark" src="brand/logo/mark.svg" alt="" width="38" height="38">';
  const topbar = (active) => `<header class="topbar"><div class="topbar-inner">
      <a class="brand" href="#/for-you" aria-label="momoshtrem, For You">${mark()}<span class="brand-word">momoshtrem</span></a>
      <nav class="nav" aria-label="Primary">${NAV.map(([id, label, ic]) => `<a class="nav-item" href="#/${id}" ${id === active ? 'aria-current="page"' : ''}><span class="ears" aria-hidden="true"></span>${icon(ic, 'ico ico-sm')}${label}</a>`).join('')}</nav>
      <button class="topbar-search" data-act="palette" aria-label="Search titles, library and actions (Command K)">${icon('search', 'ico ico-sm')}<span>Search titles, library, actions</span>${UI.kbd('⌘')}${UI.kbd('K')}</button>
      <a class="btn btn-tonal ask-btn" href="#/ask" data-flow="ask-top">${icon('sparkles')}<span>Ask momoshtrem</span></a>
      ${catMenuBtn()}
    </div></header>`;
  const tabbar = (active) => `<nav class="tabbar" aria-label="Primary">${NAV.map(([id, label, ic]) => `<a class="tab" href="#/${id}" ${id === active ? 'aria-current="page"' : ''}><span class="tab-ico"><span class="ears" aria-hidden="true"></span>${icon(ic)}</span><span class="tab-label">${label}</span></a>`).join('')}</nav>`;
  const catMenuBtn = () => `<button class="avatar-btn" data-act="menu" data-arg="cat" aria-label="Momo menu: settings and log out" aria-haspopup="menu" aria-expanded="${S.menu === 'cat'}">${momo('avatar')}</button>`;
  window.catMenuBtn = catMenuBtn;

  const catMenu = () => `<div class="menu cat-menu" role="menu" aria-label="Momo menu">
      <div class="cat-menu-head">${momo('avatar')}<div><b>momoshtrem</b><span>Trakt: ${S.trakt === 'connected' ? `@${DATA.user.trakt}` : 'not connected'}</span></div></div>
      <a class="menu-item" role="menuitem" href="#/settings">${icon('settings')}Settings</a>
      <button class="menu-item" role="menuitem" data-act="theme">${icon(root.dataset.theme === 'dark' ? 'sun' : 'moon')}${root.dataset.theme === 'dark' ? 'Light theme' : 'Dark theme'}</button>
      <div class="menu-sep"></div>
      <a class="menu-item" role="menuitem" href="#/signin">${icon('log-out')}Log out</a>
    </div>`;

  // ------------------------------------------------------------------ render
  let lastKey = '';
  function render() {
    const r = parse();
    const name = r.parts[0] || 'for-you';
    const screen = SCREENS[name] || SCREENS['for-you'];
    const ctx = { r, p: r.params, state: r.params.get('state'), S, arg: r.parts[1], arg2: r.parts[2] };
    const out = screen(ctx) || {};
    const key = r.path;
    const keepScroll = key === lastKey;
    const y = window.scrollY;
    const sheetBody = document.querySelector('.sheet:not(.nested) .sheet-body');
    const sy = sheetBody ? sheetBody.scrollTop : 0;
    const focusId = document.activeElement && document.activeElement.id;
    const selStart = document.activeElement && document.activeElement.selectionStart;

    const sheetName = r.params.get('sheet');
    const sheetHtml = sheetName && SHEETS[sheetName] ? SHEETS[sheetName](ctx) : '';
    const nested = r.params.get('nested');
    const nestedHtml = nested && SHEETS[nested] ? SHEETS[nested](ctx, true) : '';
    const palette = r.params.get('palette') ? SHEETS.palette(ctx) : '';
    const dlg = S.dialog ? S.dialog() : '';

    document.title = `${out.title ? out.title + ' · ' : ''}momoshtrem`;
    const app = document.getElementById('app');
    app.innerHTML = out.chrome === false
      ? `<main id="main" class="app-main">${out.html}</main>`
      : `<div class="app ${out.focus ? 'focus' : ''}">${topbar(out.nav)}<main id="main" class="app-main" tabindex="-1">${out.html}</main>${tabbar(out.nav)}</div>`;
    document.getElementById('overlays').innerHTML = sheetHtml + nestedHtml + palette + dlg + (S.menu === 'cat' ? `<div class="menu-scrim" data-act="close-menu"></div>${catMenu()}` : '') + (S.menu && S.menu !== 'cat' && SHEETS.menu ? SHEETS.menu(ctx) : '');
    document.body.classList.toggle('has-sheet', !!(sheetHtml || palette || dlg));
    renderToasts();
    if (keepScroll) window.scrollTo(0, y); else window.scrollTo(0, 0);
    const nb = document.querySelector('.sheet:not(.nested) .sheet-body');
    if (nb && sheetBody) nb.scrollTop = sy;
    if (focusId) { const el = document.getElementById(focusId); if (el) { el.focus({ preventScroll: true }); if (selStart != null && el.setSelectionRange) try { el.setSelectionRange(selStart, selStart); } catch (e) { /* not a text field */ } } }
    lastKey = key;
    if (out.after) out.after(ctx);
    flowGuide(ctx);
  }
  window.render = render;
  window.go = go;
  window.setParams = setParams;
  window.parseRoute = parse;

  // ------------------------------------------------------------------ toasts
  function toast(o) {
    const id = Math.random().toString(36).slice(2);
    S.toasts.push({ id, ...o });
    renderToasts();
    setTimeout(() => { S.toasts = S.toasts.filter((t) => t.id !== id); renderToasts(); }, o.ms || 4200);
  }
  function renderToasts() {
    const el = document.getElementById('toasts');
    el.innerHTML = S.toasts.map((t) => `<div class="toast ${t.kind || 'ok'}" role="status">
        ${t.momo ? momo(t.momo) : `<span class="toast-ico">${icon(t.icon || (t.kind === 'err' ? 'circle-alert' : 'check'))}</span>`}
        <div class="toast-text">${t.text}${t.sub ? `<small>${t.sub}</small>` : ''}</div>
        ${t.action ? `<button class="btn btn-ghost btn-sm" data-act="${t.act || 'noop'}">${t.action}</button>` : ''}
      </div>`).join('');
  }
  window.toast = toast;

  // ------------------------------------------------------------------ events
  document.addEventListener('click', (ev) => {
    const el = ev.target.closest('[data-act]');
    if (!el) { if (S.menu) { S.menu = null; render(); } return; }
    const act = el.dataset.act;
    if (!act || act === 'noop') return;
    const fn = ACT[act];
    if (fn) { ev.preventDefault(); fn(el.dataset.arg, el, ev); }
  });
  document.addEventListener('keydown', (ev) => {
    if ((ev.metaKey || ev.ctrlKey) && ev.key.toLowerCase() === 'k') { ev.preventDefault(); setParams({ palette: '1' }); }
    if (ev.key === 'Escape') {
      const r = parse();
      if (S.dialog) { S.dialog = null; render(); } else if (S.menu) { S.menu = null; render(); } else if (r.params.get('palette')) setParams({ palette: null }); else if (r.params.get('nested')) setParams({ nested: null }); else if (r.params.get('sheet')) setParams({ sheet: null });
    }
  });
  window.addEventListener('hashchange', () => { S.menu = null; render(); });

  // shared actions
  Object.assign(ACT, {
    'close-sheet': () => setParams({ sheet: null, nested: null }),
    'close-nested': () => setParams({ nested: null }),
    'close-dialog': () => { S.dialog = null; render(); },
    'close-menu': () => { S.menu = null; render(); },
    menu: (arg) => { S.menu = S.menu === arg ? null : arg; render(); },
    palette: () => setParams({ palette: '1' }),
    'close-palette': () => setParams({ palette: null }),
    theme: () => { root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark'; S.menu = null; render(); },
    toast: (arg) => {
      const T = {
        magnet: ['Magnet link copied', 'Paste it anywhere a magnet works.', 'link'],
        path: ['WebDAV path copied', 'Paste it into Infuse, or keep it handy.', 'clipboard-check'],
        paused: ['Paused', 'Infuse can still read what’s already downloaded.', 'pause'],
        resumed: ['Resumed', null, 'play'],
        code: ['Code copied', null, 'clipboard-check'],
        liked: ['Noted. More like this.', 'Momo will use it in tonight’s picks.', 'thumbs-up'],
      }[arg] || [arg, null, 'check'];
      toast({ text: T[0], sub: T[1], icon: T[2] });
    },
    shelf: (dir, el) => { const s = el.closest('.shelf-sec').querySelector('.shelf'); s.scrollBy({ left: Number(dir) * s.clientWidth * 0.85, behavior: 'smooth' }); },
    back: () => history.back(),
  });

  // ------------------------------------------------------------------ guided flows (prototype)
  const FLOWS = (window.FLOWS = window.FLOWS || {});
  function flowGuide(ctx) {
    const el = document.getElementById('flow');
    document.querySelectorAll('.flow-hl').forEach((n) => n.classList.remove('flow-hl'));
    if (!FLOW || !FLOWS[FLOW]) { el.innerHTML = ''; return; }
    const f = FLOWS[FLOW];
    let i = 0;
    f.steps.forEach((s, k) => { if (s.when(ctx)) i = k; });
    const step = f.steps[i];
    el.innerHTML = `<div class="flow-card" role="status" aria-live="polite">
        <div class="flow-top"><span class="flow-num">${f.name}</span><span class="flow-count">${i + 1}/${f.steps.length}</span></div>
        <p>${step.text}</p>
        <div class="flow-dots">${f.steps.map((_, k) => `<i class="${k < i ? 'done' : k === i ? 'now' : ''}"></i>`).join('')}</div>
        <button class="flow-restart" data-act="flow-restart">${icon('rotate-cw', 'ico ico-xs')}Restart</button>
      </div>`;
    if (step.target) {
      requestAnimationFrame(() => {
        const t = document.querySelector(step.target);
        if (t) { t.classList.add('flow-hl'); if (step.scroll !== false) t.scrollIntoView({ block: 'nearest', behavior: 'smooth' }); }
      });
    }
  }
  ACT['flow-restart'] = () => { const f = FLOWS[FLOW]; Object.keys(S.job).forEach((k) => delete S.job[k]); S.frierenReady = false; S.added = {}; S.ask = { thread: null, phase: 'idle', step: 0 }; S.trakt = f.startTrakt || S.trakt; if (f.reset) f.reset(S); location.hash = f.start; render(); };

  window.addEventListener('DOMContentLoaded', () => {
    if (FLOW && FLOWS[FLOW]) { const f = FLOWS[FLOW]; if (f.reset) f.reset(S); if (!location.hash) location.hash = f.start; }
    render();
  });
})();
