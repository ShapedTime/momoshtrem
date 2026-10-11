/* Sign in. States: default · error · ratelimited · submitting
   A real <form method="post"> with username/current-password fields so password
   managers save and fill it. Small cat moment: Noir's eyes follow what you type
   in the username field and close while you type the password ("it won't look"). */
(function () {
  const { ui, app, art, brand, icon } = MS;
  const esc = ui.esc;
  const G = brand.G;

  // Dim poster wall behind the form (decorative)
  const WALL = [
    ['severance', 'pastlives', 'frieren', 'dune2', 'thebear'],
    ['shogun', 'perfectdays', 'pluribus', 'bluey', 'arrival'],
    ['interstellar', 'onepiece', 'aftersun', 'goodplace', 'silo'],
    ['doctorwho', 'mushishi', 'andor', 'paprika', 'columbus'],
    ['slowhorses', 'totoro', 'shrinking', 'dandadan', 'studio'],
    ['mythicquest', 'onlymurders', 'severance', 'pastlives', 'frieren'],
    ['dune2', 'bluey', 'perfectdays', 'shogun', 'pluribus'],
  ];
  const wall = () => `<div class="auth-wall" aria-hidden="true"><div class="auth-wall-in">${WALL.map((col, i) => `<div class="auth-col${i % 2 ? ' is-down' : ''}">${col.map((k) => `<div class="media media--poster">${art.poster(k)}</div>`).join('')}</div>`).join('')}</div></div>`;

  // Noir: dark head, amber eyes with pupils that can look and close.
  const pup = (c) => `<ellipse class="pupil" cx="${c[0]}" cy="${c[1]}" rx="1.25" ry="3.1" fill="#07080b"/>`;
  const noir = (size) => `<svg class="noir" width="${size}" height="${size}" viewBox="0 0 64 64" aria-hidden="true" focusable="false">
      <defs><radialGradient id="noir-back" cx="0.5" cy="0.62" r="0.5"><stop offset="0" stop-color="#f2b33d" stop-opacity="0.28"/><stop offset="0.55" stop-color="#c9772a" stop-opacity="0.08"/><stop offset="1" stop-color="#c9772a" stop-opacity="0"/></radialGradient></defs>
      <circle cx="32" cy="38" r="32" fill="url(#noir-back)"/>
      <path d="${G.head}" fill="#07080b" stroke="#ffd27a" stroke-opacity="0.32" stroke-width="0.6"/>
      <g class="noir-eye"><g class="noir-blink"><path d="${G.eyeL}" fill="#ffc95e"/>${pup(G.pupL)}</g></g>
      <g class="noir-eye"><g class="noir-blink"><path d="${G.eyeR}" fill="#ffc95e"/>${pup(G.pupR)}</g></g>
      <g class="noir-lids" fill="none" stroke="#f2b33d" stroke-width="1.3" stroke-linecap="round"><path d="M15.5 37.6q6.4 3.6 12.4 0"/><path d="M36.1 37.6q6.4 3.6 12.4 0"/></g>
    </svg>`;

  const fmt = (s) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;

  app.route('/signin', (p) => {
    const state = ['error', 'ratelimited', 'submitting'].includes(p.state) ? p.state : 'default';
    const err = state === 'error';
    const limited = state === 'ratelimited';
    const user = state === 'default' ? '' : 'teymur';
    const pass = state === 'default' ? '' : 'correct-horse-battery';
    const switcher = ui.protoStates('#/signin', [['default', 'Default'], ['error', 'Wrong password'], ['ratelimited', 'Rate limited'], ['submitting', 'Submitting']], state);

    const banner = err
      ? `<div class="banner banner--error auth-msg" role="alert" id="auth-msg">${icon('alertCircle', { size: 20 })}<div class="grow"><strong>Wrong username or password</strong><span>Check that caps lock is off. You have 2 more tries before a short pause.</span></div></div>`
      : limited
        ? `<div class="banner auth-msg auth-msg--calm" role="status" id="auth-msg">${icon('clock', { size: 20 })}<div class="grow"><strong>Too many attempts</strong><span>Try again in <b class="t-num" data-count>4:32</b>. This keeps guessers out. Your account and library are fine.</span></div></div>`
        : '';

    const btnLabel = limited ? `<span>Try again in <span class="t-num" data-count>4:32</span></span>` : 'Sign in';

    const html = `${switcher}
      <div class="auth" data-state="${state}" data-eyes="${limited ? 'sleepy' : 'open'}">
        ${wall()}
        <div class="auth-scrim" aria-hidden="true"></div>
        <div class="auth-center">
          <section class="auth-card" aria-labelledby="auth-h">
            <div class="auth-cat">${noir(92)}<p class="auth-caption t-small" aria-live="polite" id="auth-caption"></p></div>
            <div class="auth-titles">
              <h1 class="t-title-1" id="auth-h">Evening.</h1>
              <p class="c-2">Sign in to <span class="c-1 w-semibold">momoshtrem</span> to pick tonight's watch.</p>
            </div>
            ${banner}
            <form class="auth-form" method="post" action="/api/auth" id="auth-form" novalidate>
              <div class="field">
                <label class="label" for="auth-user">Username</label>
                <input class="input input-lg" id="auth-user" name="username" type="text" value="${esc(user)}" autocomplete="username" autocapitalize="none" autocorrect="off" spellcheck="false" enterkeyhint="next" required ${err ? 'aria-invalid="true" aria-describedby="auth-msg"' : ''}>
              </div>
              <div class="field">
                <label class="label" for="auth-pass">Password</label>
                <div class="input-wrap auth-pass">
                  <input class="input input-lg" id="auth-pass" name="password" type="password" value="${esc(pass)}" autocomplete="current-password" autocapitalize="none" autocorrect="off" spellcheck="false" enterkeyhint="go" required ${err ? 'aria-invalid="true" aria-describedby="auth-msg"' : ''}>
                  <div class="input-end"><button type="button" class="btn btn-ghost btn-icon auth-reveal" data-act="auth-reveal" aria-controls="auth-pass" aria-pressed="false" aria-label="Show password">${icon('eye', { size: 20 })}</button></div>
                </div>
              </div>
              <p class="error-text" id="auth-empty" hidden>${icon('alertCircle', { size: 15 })}<span>Enter your username and password.</span></p>
              <button type="submit" class="btn btn-primary btn-lg btn-block auth-submit${state === 'submitting' ? ' is-loading' : ''}" id="auth-submit" ${limited || state === 'submitting' ? 'disabled' : ''} ${state === 'submitting' ? 'aria-busy="true"' : ''}>${btnLabel}</button>
              ${state === 'submitting' ? '<p class="sr-only" role="status">Signing in…</p>' : ''}
            </form>
            <p class="auth-foot t-small c-3">Forgot it? Reset it on the server with <code>momoshtrem passwd</code>.</p>
          </section>
          <p class="auth-legal t-small c-3">${brand.mark(18)}<span>momoshtrem · self-hosted · v2.0</span></p>
        </div>
      </div>`;

    return {
      title: 'Sign in', bare: true, html,
      mount(root) {
        const wrap = root.querySelector('.auth');
        const form = root.querySelector('#auth-form');
        const u = root.querySelector('#auth-user');
        const pw = root.querySelector('#auth-pass');
        const btn = root.querySelector('#auth-submit');
        const cap = root.querySelector('#auth-caption');
        const svg = root.querySelector('.noir');
        const baseEyes = wrap.dataset.eyes;

        // Pupils follow the caret while typing the username
        const look = () => { const x = Math.max(-1.6, Math.min(1.6, (u.value.length - 8) * 0.22)); svg.style.setProperty('--look', `${x}px`); };
        u.addEventListener('focus', () => { wrap.dataset.eyes = 'open'; look(); cap.textContent = ''; });
        u.addEventListener('input', look);
        u.addEventListener('blur', () => svg.style.setProperty('--look', '0px'));
        // ...and close for the password. It won't look.
        pw.addEventListener('focus', () => { wrap.dataset.eyes = 'closed'; cap.textContent = 'Noir won’t look.'; });
        pw.addEventListener('blur', () => { wrap.dataset.eyes = baseEyes; cap.textContent = ''; });
        [u, pw].forEach((el) => el.addEventListener('input', () => { el.removeAttribute('aria-invalid'); root.querySelector('#auth-empty').hidden = true; }));

        // Rate limit countdown (button stays disabled until it ends)
        if (limited) {
          let s = 272;
          const tick = () => {
            if (!document.contains(btn)) return clearInterval(t);
            s -= 1;
            root.querySelectorAll('[data-count]').forEach((n) => { n.textContent = fmt(Math.max(s, 0)); });
            if (s <= 0) { clearInterval(t); btn.disabled = false; btn.textContent = 'Sign in'; wrap.dataset.eyes = 'open'; const m = root.querySelector('#auth-msg'); if (m) m.remove(); }
          };
          const t = setInterval(tick, 1000);
        }

        form.addEventListener('submit', (e) => {
          e.preventDefault(); // prototype: the real form posts to /api/auth
          if (btn.disabled) return;
          if (!u.value.trim() || !pw.value) {
            root.querySelector('#auth-empty').hidden = false;
            (!u.value.trim() ? u : pw).setAttribute('aria-invalid', 'true');
            (!u.value.trim() ? u : pw).focus();
            return;
          }
          // Submitting disables only the button; fields stay editable.
          btn.disabled = true; btn.classList.add('is-loading'); btn.setAttribute('aria-busy', 'true');
          setTimeout(() => {
            if (pw.value.toLowerCase() === 'wrong') { app.go('#/signin?state=error', { noTransition: true }); return; }
            app.go('#/home');
          }, 900);
        });

        if (err) setTimeout(() => { pw.focus({ preventScroll: true }); pw.select(); }, 60);
      },
    };
  });

  Object.assign(app.actions, {
    'auth-reveal': (b) => {
      const pw = document.getElementById('auth-pass'); if (!pw) return;
      const show = pw.type === 'password';
      pw.type = show ? 'text' : 'password';
      b.setAttribute('aria-pressed', String(show));
      b.setAttribute('aria-label', show ? 'Hide password' : 'Show password');
      b.innerHTML = icon(show ? 'eyeOff' : 'eye', { size: 20 });
      pw.focus({ preventScroll: true });
      const cap = document.getElementById('auth-caption');
      if (cap) cap.textContent = show ? 'Still not looking.' : 'Noir won’t look.';
    },
  });
})();
