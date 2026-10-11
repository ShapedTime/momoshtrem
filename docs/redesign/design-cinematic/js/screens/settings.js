/* Settings. Route /settings?section=trakt|ai|quality|playback|status
   Trakt states via ?state=off|code|waiting|connected (default: app.state.trakt).
   Other sections: ?state=dirty shows the unsaved-changes bar. Playback: ?state=tested. */
(function () {
  const { ui, app, icon, brand } = MS;
  const esc = ui.esc;

  const SECTIONS = [
    { id: 'trakt', group: 'Integrations', label: 'Trakt', ic: 'link', desc: 'Your watch history powers Up next and personal picks.' },
    { id: 'ai', group: 'Preferences', label: 'AI & picks', ic: 'sparkles', desc: 'When Noir writes your picks, which model it uses, and what it may spend.' },
    { id: 'quality', group: 'Preferences', label: 'Quality profile', ic: 'sliders', desc: 'What "Best match" means when you search for a torrent.' },
    { id: 'playback', group: 'Playback', label: 'Infuse & WebDAV', ic: 'tv', desc: 'How Infuse on your Apple TV reaches momoshtrem.' },
    { id: 'status', group: 'System', label: 'System status', ic: 'server', desc: 'Every service momoshtrem depends on, checked just now.' },
  ];

  // Quality profile (GET/PUT /settings/quality-profile), kept for the session
  const Q = { pref: '2160p', max: '2160p', gbh: 8, x265: true, hdr: true, dv: true, avoid: ['CAM', 'TS', 'HDCAM', 'TELESYNC', 'HC', 'KORSUB'], groups: ['FLUX', 'NTb', 'SubsPlease', 'VARYG'] };
  const traktSummary = () => ({ off: 'Not connected', code: 'Waiting for code', waiting: 'Waiting for approval', syncing: 'First sync running', connected: 'Connected as teymur' }[app.state.trakt] || 'Connected as teymur');
  const SUMMARY = { trakt: traktSummary, ai: () => 'Nightly · $3.12 of $10.00', quality: () => `${Q.pref} preferred · ≤ ${Q.gbh} GB/h`, playback: () => 'WebDAV ready', status: () => '1 warning' };

  // ---- Shared bits -------------------------------------------------------------
  const row = (label, help, control, opts = {}) => `<div class="set-row ${opts.stack ? 'set-row--stack' : ''}" ${opts.attrs || ''}>
      <div class="set-row-text"><span class="set-row-label" ${opts.id ? `id="${opts.id}"` : ''}>${label}</span>${help ? `<span class="help">${help}</span>` : ''}</div>
      <div class="set-row-ctl">${control}</div></div>`;
  const card = (title, body, opts = {}) => `<section class="panel set-card ${opts.cls || ''}" ${title ? `aria-labelledby="${opts.hid}"` : ''}>
      ${title ? `<header class="set-card-head"><h3 class="t-title-3" id="${opts.hid}">${title}</h3>${opts.aside || ''}</header>` : ''}${body}</section>`;
  const sw = (id, on, label) => ui.switch(id, on, label);

  function nav(current, isIndex) {
    const names = [...new Set(SECTIONS.map((s) => s.group))];
    const item = (s) => `<li><a href="#/settings?section=${s.id}" ${!isIndex && s.id === current ? 'aria-current="page"' : ''} ${isIndex && s.id === current ? 'data-desk-current' : ''}>
          <span class="ic">${icon(s.ic, { size: 18 })}</span><span class="grow"><span class="nm">${s.label}</span><span class="sub">${esc(SUMMARY[s.id]())}</span></span>
          ${s.id === 'status' ? '<span class="dot-warn" aria-label="1 warning"></span>' : ''}${icon('chevronRight', { size: 18, cls: 'chev' })}</a></li>`;
    const groups = names.map((g) => `<div class="set-group"><p class="t-micro set-group-h">${g}</p><ul class="set-list">${SECTIONS.filter((s) => s.group === g).map(item).join('')}</ul></div>`).join('');
    return `<nav class="set-nav" aria-label="Settings sections">${groups}
      <div class="set-group phone-only"><ul class="set-list"><li><a href="#/signin" class="danger"><span class="ic">${icon('logout', { size: 18 })}</span><span class="grow"><span class="nm">Log out</span></span></a></li></ul></div>
      <p class="set-version t-small c-3">momoshtrem 0.9.2 · server and app up to date</p></nav>`;
  }

  // ---- Trakt ---------------------------------------------------------------------
  function traktOff() {
    return card('', `<div class="set-trakt-off">
        <div class="set-trakt-art">${ui.cat('peek', 120)}</div>
        <div class="stack gap-3" style="min-width:0">
          <h3 class="t-title-2">Connect Trakt</h3>
          <p class="c-2">momoshtrem reads your watch history, ratings and watchlist to build Up next and picks that fit you. It's read-only: nothing is ever posted to Trakt.</p>
          <ul class="set-imports">
            <li>${icon('history', { size: 16 })}History<span>what you've watched, for Up next</span></li>
            <li>${icon('star', { size: 16 })}Ratings<span>what you loved, for picks</span></li>
            <li>${icon('bookmark', { size: 16 })}Watchlist<span>shown on For You</span></li>
          </ul>
          <div class="row gap-3 wrap"><button class="btn btn-primary" data-act="set-trakt-connect">${icon('link', { size: 18 })}Connect Trakt</button><span class="t-small c-3">You'll get a code to enter on trakt.tv. No password here.</span></div>
        </div></div>`);
  }
  function traktCode() {
    return card('', `<div class="set-device">
        <div class="set-device-code">
          <span class="t-micro c-3">Your code</span>
          <div class="code" aria-label="Code 7 F 3 A, 9 1 C 2">7F3A-91C2</div>
          <button class="btn btn-secondary btn-sm" data-act="copy" data-copy="7F3A91C2">${icon('copy', { size: 15 })}Copy code</button>
          <div class="set-expiry"><div class="progress"><span data-exp-bar style="width:100%"></span></div><span class="t-small c-3 t-num" data-exp>Expires in 14:59</span></div>
        </div>
        <ol class="set-device-steps howto-steps stack gap-4">
          <li><span class="n">1</span><div class="stack gap-2" style="min-width:0"><strong>Go to <span class="t-mono c-accent">trakt.tv/activate</span></strong><span class="t-small c-2">On this phone, your laptop, anywhere you're signed in to Trakt.</span>
            <a class="btn btn-primary set-open-trakt" href="https://trakt.tv/activate" target="_blank" rel="noopener">${icon('external', { size: 17 })}Open trakt.tv/activate</a></div></li>
          <li><span class="n">2</span><div class="stack gap-1"><strong>Enter the code and approve</strong><span class="t-small c-2">Trakt will ask to let momoshtrem read your history. Say yes.</span></div></li>
          <li><span class="n">3</span><div class="row gap-3 set-noir"><span class="eyes-box">${brand.eyes(34)}</span><span class="t-small c-2">I'll notice as soon as you approve. Keep this page open.</span></div></li>
        </ol>
      </div>
      <div class="set-card-foot"><button class="btn btn-ghost" data-act="set-trakt-cancel">Cancel</button><button class="btn btn-secondary" data-act="set-trakt-approved">${icon('check', { size: 17 })}I've approved it</button></div>`, { cls: 'set-card--code' });
  }
  function traktWaiting() {
    return card('', `<div class="set-waiting" data-trakt-waiting>
        ${ui.loader('Waiting for you to approve on Trakt…', 'Checking every 5 seconds · code 7F3A-91C2 expires in 13:40')}
        <div class="steps steps--inline set-wait-steps">
          <div class="step is-done"><span class="ic">${icon('check', { size: 12, stroke: 2.6 })}</span>Code shown</div>
          <div class="step is-active"><span class="ic"></span>Approval on trakt.tv</div>
          <div class="step"><span class="ic"></span>First sync</div>
        </div>
      </div>
      <div class="set-card-foot"><button class="btn btn-ghost" data-act="set-trakt-cancel">Cancel</button><button class="btn btn-ghost" data-act="set-trakt-back">Show the code again</button></div>`);
  }
  function traktConnected() {
    const syncing = app.state.trakt === 'syncing';
    return `${syncing ? `<div class="banner banner--accent" role="status">${icon('refresh', { size: 18 })}<div class="grow"><strong>First sync in progress</strong><span>Imported your history; writing your shelves (2 of 5). For You fills in as it goes.</span></div><div class="actions"><a class="btn btn-secondary btn-sm" href="#/home">See For You</a></div></div>` : ''}
      ${card('', `<div class="set-profile">
        <span class="set-avatar" aria-hidden="true">T</span>
        <div class="grow stack gap-1"><div class="row gap-2 wrap"><strong class="t-title-3">teymur</strong>${ui.pill('ready', 'Connected', { icon: 'check' })}</div>
          <span class="t-small c-3" data-last-sync>Last sync 4 min ago · syncs every 30 min</span></div>
        <div class="set-profile-actions"><button class="btn btn-secondary" data-act="set-trakt-sync">${icon('refresh', { size: 17 })}Sync now</button></div>
      </div>
      <dl class="set-counts">
        <div><dt>${icon('history', { size: 15 })}History</dt><dd class="t-num">2,184 <span>plays</span></dd></div>
        <div><dt>${icon('star', { size: 15 })}Ratings</dt><dd class="t-num">312</dd></div>
        <div><dt>${icon('bookmark', { size: 15 })}Watchlist</dt><dd class="t-num">41 <span>titles</span></dd></div>
      </dl>`)}
      ${card('What momoshtrem uses', `
        ${row('Ratings shape your picks', 'Titles you rated 8+ steer "Because you loved…" shelves.', sw('sw-ratings', true, 'Use ratings for picks'))}
        ${row('Watchlist on For You', '41 titles · 6 already in your library', sw('sw-watchlist', true, 'Show watchlist on For You'))}
        ${row('Up next includes paused shows', 'Shows you haven\'t touched in 6 months are hidden.', sw('sw-paused', false, 'Include paused shows in Up next'))}`, { hid: 'trakt-uses' })}
      <div class="set-quiet-zone"><div class="stack gap-1"><strong class="w-semibold">Disconnect Trakt</strong><span class="t-small c-3">Deletes imported history, ratings and watchlist from momoshtrem. Nothing changes on Trakt.</span></div>
        <button class="btn btn-ghost btn-danger-quiet" data-act="set-disconnect">${icon('link', { size: 17 })}Disconnect</button></div>`;
  }

  // ---- AI ------------------------------------------------------------------------
  function ai() {
    const spend = [['Recommendations', 2.18, 'k1'], ['Ask momoshtrem', 0.53, 'k2'], ['Mapping classifier', 0.41, 'k3']];
    const total = spend.reduce((a, s) => a + s[1], 0);
    return `${card('Personal picks', `
        ${row('Write picks automatically', 'Noir reads your Trakt history, checks what\'s new and writes 4–6 shelves.', sw('sw-recs', true, 'Write picks automatically'))}
        ${row('Schedule', 'Nightly runs at 03:00, weekly on Sunday. Next run tonight; the last one took 41 s and cost $0.36.', ui.seg([['nightly', 'Nightly'], ['weekly', 'Weekly'], ['manual', 'Manual']], 'nightly', 'aria-label="Recommendation schedule"'), { stack: true })}
        <div class="set-row set-row--stack"><div class="set-row-text"><span class="set-row-label" id="model-l">Model</span><span class="help">Used for picks and Ask momoshtrem.</span></div>
          <div class="set-models" role="radiogroup" aria-labelledby="model-l">
            <label class="set-model"><input type="radio" name="model" value="opus" checked><span class="box"><span class="row between gap-2"><strong>Opus 5.5</strong><span class="chip chip--accent">Deeper</span></span><span class="t-small c-2">Sharper reasons, reads more of your history.</span><span class="t-small c-3 t-num">≈ $0.36 a run</span></span></label>
            <label class="set-model"><input type="radio" name="model" value="sonnet"><span class="box"><span class="row between gap-2"><strong>Sonnet 5.5</strong><span class="chip">Cheaper</span></span><span class="t-small c-2">Good enough most nights, a bit more generic.</span><span class="t-small c-3 t-num">≈ $0.07 a run</span></span></label>
          </div></div>`, { hid: 'ai-picks', aside: `<button class="btn btn-ghost btn-sm" data-act="set-run-now">${icon('play', { size: 15 })}Run now</button>` })}
      ${card('Monthly budget', `
        <div class="set-spend">
          <div class="row between wrap gap-2 set-spend-top"><p><span class="big t-num">$${total.toFixed(2)}</span> <span class="c-2 t-num">of $10.00 this month</span></p><span class="t-small c-3">Resets Nov 1</span></div>
          <div class="set-spend-bar" role="img" aria-label="Spent $${total.toFixed(2)} of $10.00: ${spend.map((s) => `${s[0]} $${s[1].toFixed(2)}`).join(', ')}">${spend.map((s) => `<i class="${s[2]}" style="width:${(s[1] / 10) * 100}%"></i>`).join('')}</div>
          <ul class="set-spend-legend">${spend.map((s) => `<li><i class="${s[2]}"></i>${s[0]}<span class="t-num">$${s[1].toFixed(2)}</span></li>`).join('')}</ul>
          <p class="t-small c-3">At this pace you'll spend about $4.80 by the end of October. If the budget runs out, AI pauses until the 1st and mapping falls back to rules.</p>
        </div>
        ${row('Budget', 'Per calendar month', `<div class="set-money"><span aria-hidden="true">$</span><input class="input" type="text" inputmode="decimal" value="10.00" aria-label="Monthly budget in US dollars"></div>`)}`, { hid: 'ai-budget' })}
      ${card('Episode mapping', `
        ${row('Ask AI about tricky files', 'Only for files the rules can\'t place: absolute numbering, specials, odd names. Sonnet 5.5, about $0.01 a torrent.', sw('sw-classifier', true, 'Mapping classifier'))}
        ${row('Auto-apply high-confidence mappings', 'Applied without review. Medium and low confidence always wait for you.', sw('sw-autoapply', true, 'Auto-apply high-confidence mappings'))}`, { hid: 'ai-map' })}`;
  }

  // ---- Quality ---------------------------------------------------------------------
  const preview = () => `${Q.pref} WEB-DL${Q.dv ? ' DV' : Q.hdr ? ' HDR' : ''} ≤ ${Q.gbh} GB/h`;
  const previewSub = () => `${Q.x265 ? 'x265 when there\'s a choice · ' : ''}${Q.groups.length ? `from ${Q.groups.slice(0, 2).join(' or ')}${Q.groups.length > 2 ? ` +${Q.groups.length - 2}` : ''}` : 'any group'} · never ${Q.avoid.slice(0, 3).join(', ')}${Q.avoid.length > 3 ? ` +${Q.avoid.length - 3}` : ''}`;
  const tags = (key, label) => `<div class="set-tags" data-tags="${key}">
      <ul class="chips" aria-label="${label}">${Q[key].map((t) => `<li class="set-tag"><span>${esc(t)}</span><button data-act="set-tag-rm" data-key="${key}" data-v="${esc(t)}" aria-label="Remove ${esc(t)}">${icon('x', { size: 13 })}</button></li>`).join('')}</ul>
      <div class="input-wrap">${icon('plus', { size: 16 })}<input class="input" type="text" placeholder="${key === 'avoid' ? 'Add a term, e.g. HDTS' : 'Add a group, e.g. Erai-raws'}" data-tag-input="${key}" aria-label="Add to ${label}" enterkeyhint="done"></div></div>`;
  function quality() {
    return `<div class="set-preview" role="status" aria-live="polite">${icon('sparkle', { size: 18 })}<div class="grow"><span class="t-small c-2">Best match would prefer</span><strong data-q-preview>${esc(preview())}</strong><span class="t-small c-3" data-q-preview-sub>${esc(previewSub())}</span></div></div>
      ${card('Resolution and size', `
        ${row('Preferred resolution', 'Ranked first when seeders are healthy.', ui.seg([['720p', '720p'], ['1080p', '1080p'], ['2160p', '2160p']], Q.pref, 'data-q="pref" aria-label="Preferred resolution"'), { stack: true })}
        ${row('Maximum resolution', 'Anything above is hidden from results.', ui.seg([['1080p', '1080p'], ['2160p', '2160p'], ['any', 'No limit']], Q.max, 'data-q="max" aria-label="Maximum resolution"'), { stack: true })}
        <div class="set-row set-row--stack"><div class="set-row-text"><label class="set-row-label" for="q-gbh">Max size per hour</label><span class="help" data-gbh-help>${Q.gbh} GB/h ≈ ${(Q.gbh * 2.77).toFixed(0)} GB for Dune: Part Two (2 h 46 m)</span></div>
          <div class="set-range"><input type="range" id="q-gbh" min="1" max="20" step="1" value="${Q.gbh}" style="--v:${((Q.gbh - 1) / 19) * 100}%"><output for="q-gbh" class="t-num" data-gbh>${Q.gbh} GB/h</output></div>
          <div class="set-range-scale t-small c-3 t-num" aria-hidden="true">${[1, 5, 10, 15, 20].map((v) => `<span style="--at:${(v - 1) / 19}">${v}</span>`).join('')}</div></div>`, { hid: 'q-res' })}
      ${card('Video', `
        ${row('Prefer x265 / HEVC', 'Smaller files at the same quality. Apple TV 4K plays it natively.', sw('q-x265', Q.x265, 'Prefer x265'))}
        ${row('Prefer HDR', 'HDR10 and HDR10+ when available.', sw('q-hdr', Q.hdr, 'Prefer HDR'))}
        ${row('Prefer Dolby Vision', 'Infuse plays Dolby Vision on Apple TV 4K.', sw('q-dv', Q.dv, 'Prefer Dolby Vision'))}`, { hid: 'q-video' })}
      ${card('Terms to avoid', `<p class="t-small c-3 set-card-sub">Releases with these words are hidden. CAM and TS are never worth it.</p>${tags('avoid', 'Terms to avoid')}`, { hid: 'q-avoid' })}
      ${card('Preferred groups', `<p class="t-small c-3 set-card-sub">Get a small boost in ranking. They never override seeders or your size limit.</p>${tags('groups', 'Preferred groups')}`, { hid: 'q-groups' })}`;
  }

  // ---- Playback --------------------------------------------------------------------
  const WEBDAV = 'https://momoshtrem.duckdns.org:36911/';
  function testResult() {
    return `<div class="set-test-result" role="status">
      <div class="row gap-2">${ui.pill('ready', 'Connection works', { icon: 'check' })}<span class="t-small c-3">Just now</span></div>
      <ul class="set-checks">
        <li>${icon('check', { size: 16 })}WebDAV answered in 38 ms</li>
        <li>${icon('check', { size: 16 })}Login accepted for teymur</li>
        <li>${icon('check', { size: 16 })}Infuse will see 14 shows and 9 movies</li>
        <li>${icon('check', { size: 16 })}Read speed from the server: 112 MB/s, plenty for 2160p remuxes</li>
      </ul></div>`;
  }
  function playback(state) {
    return `${card('WebDAV address', `
        <div class="stack gap-3">
          <div class="copy-field"><code class="path">${WEBDAV}</code><button class="btn btn-secondary btn-sm" data-act="copy" data-copy="${WEBDAV}">${icon('copy', { size: 15 })}Copy</button></div>
          <p class="t-small c-3">Username <span class="c-1 w-medium">teymur</span> · password is the same as for this app.</p>
        </div>`, { hid: 'pb-addr' })}
      ${card('Set up Infuse (once)', `<ol class="howto-steps stack gap-4">
          <li><span class="n">1</span><div><strong>On Apple TV, open Infuse</strong><p class="t-small c-2">Settings › Shares › Add share › WebDAV</p></div></li>
          <li><span class="n">2</span><div><strong>Paste the address above</strong><p class="t-small c-2">Name it "momoshtrem". Use your username and password from here.</p></div></li>
          <li><span class="n">3</span><div><strong>Add it to the Library</strong><p class="t-small c-2">Infuse sorts Movies and Shows by itself. New downloads appear after its next refresh; pull down to refresh sooner.</p></div></li>
        </ol>`, { hid: 'pb-guide' })}
      ${card('Test connection', `<div class="stack gap-3" data-test-box>
          <p class="t-small c-2">Checks the address, your login and what Infuse will see, from the server's side.</p>
          ${state === 'tested' ? testResult() : '<p class="t-small c-3">Last test 2 days ago: all fine.</p>'}
          <div><button class="btn btn-secondary" data-act="set-test">${icon('zap', { size: 17 })}Test connection</button></div></div>`, { hid: 'pb-test' })}`;
  }

  // ---- Status ---------------------------------------------------------------------
  function status() {
    const off = app.state.trakt === 'off';
    const svc = [
      ['server', 'momoshtrem backend', 'v0.9.2 · up 6 days · database 48 MB', ui.pill('ready', 'OK', { icon: 'check' })],
      ['tv', 'WebDAV', ':36911 · 1 client now (Apple TV)', ui.pill('ready', 'OK', { icon: 'check' })],
      ['search', 'Jackett', 'v0.22.1 · 2 of 3 indexers OK', ui.pill('warning', 'Degraded'), `<ul class="set-indexers">
          <li>${icon('check', { size: 15, cls: 'ok' })}<span class="grow">Nyaa</span><span class="t-num c-3">210 ms</span></li>
          <li>${icon('check', { size: 15, cls: 'ok' })}<span class="grow">TorrentLeech</span><span class="t-num c-3">340 ms</span></li>
          <li class="is-warn">${icon('alertTriangle', { size: 15 })}<span class="grow stack"><span>The Pirate Bay</span><span class="c-3">Timing out for 2 h; searches skip it after 8 s</span></span><a class="btn btn-ghost btn-sm" href="http://localhost:9117" target="_blank" rel="noopener">Open Jackett</a></li></ul>`],
      ['globe', 'TMDB', 'API v3 · 112 ms', ui.pill('ready', 'OK', { icon: 'check' })],
      ['link', 'Trakt', off ? 'Not connected' : 'Connected as teymur · synced 4 min ago', off ? `<a class="btn btn-secondary btn-sm" href="#/settings?section=trakt">Connect</a>` : ui.pill('ready', 'OK', { icon: 'check' })],
      ['sparkles', 'AI', 'Anthropic key configured · Opus 5.5 & Sonnet 5.5 · $3.12 this month', ui.pill('ready', 'OK', { icon: 'check' })],
      ['captions', 'OpenSubtitles', 'Key configured · 14 of 20 downloads left today', ui.pill('ready', 'OK', { icon: 'check' })],
    ];
    return `<div class="banner banner--warning">${icon('alertTriangle', { size: 18 })}<div class="grow"><strong>Mostly fine</strong><span>One indexer is timing out. Search still works with the other two.</span></div></div>
      ${card('', `<ul class="set-services">${svc.map(([ic, name, detail, pill, extra]) => `<li class="set-svc"><span class="ic">${icon(ic, { size: 18 })}</span><div class="grow stack gap-1"><span class="w-semibold">${name}</span><span class="t-small c-3">${detail}</span></div><div class="end">${pill}</div>${extra ? `<div class="full">${extra}</div>` : ''}</li>`).join('')}</ul>
        <div class="set-card-foot"><span class="t-small c-3 grow">Checked 21:44:02</span><button class="btn btn-ghost btn-sm" data-act="copy" data-copy="momoshtrem 0.9.2; jackett 0.22.1 (tpb timeout); tmdb ok; trakt ok; ai ok">${icon('clipboard', { size: 15 })}Copy diagnostics</button><button class="btn btn-secondary btn-sm" data-act="set-recheck">${icon('refresh', { size: 15 })}Re-check</button></div>`, { cls: 'set-card--flush' })}`;
  }

  // ---- Route -----------------------------------------------------------------------
  app.route('/settings', (p) => {
    const isIndex = !p.section;
    const sec = SECTIONS.find((s) => s.id === p.section) || SECTIONS[0];
    let state = p.state;
    let switcher;
    let content;
    const base = `#/settings?section=${sec.id}`;
    if (sec.id === 'trakt') {
      const ts = state || (app.state.trakt === 'syncing' ? 'connected' : app.state.trakt) || 'connected';
      state = ts;
      switcher = ui.protoStates(base, [['off', 'Not connected'], ['code', 'Device code'], ['waiting', 'Waiting'], ['connected', 'Connected']], ts);
      content = ts === 'off' ? traktOff() : ts === 'code' ? traktCode() : ts === 'waiting' ? traktWaiting() : traktConnected();
    } else if (sec.id === 'ai') { switcher = ui.protoStates(base, [['default', 'Default'], ['dirty', 'Unsaved changes']], state); content = ai(); }
    else if (sec.id === 'quality') { switcher = ui.protoStates(base, [['default', 'Default'], ['dirty', 'Unsaved changes']], state); content = quality(); }
    else if (sec.id === 'playback') { switcher = ui.protoStates(base, [['default', 'Default'], ['tested', 'Test passed']], state); content = playback(state); }
    else { switcher = ui.protoStates(base, [['default', 'One indexer down']], state); content = status(); }

    const html = `${switcher}
      ${isIndex ? ui.largeTitle('Settings') : `<div class="page set-phonehead phone-only"><a class="btn btn-ghost btn-sm" href="#/settings">${icon('chevronLeft', { size: 18 })}Settings</a></div>`}
      <header class="page set-top desk-only"><h1 class="t-title-1">Settings</h1></header>
      <div class="page set-layout ${isIndex ? 'is-index' : 'has-section'}">
        ${nav(sec.id, isIndex)}
        <div class="set-content" data-section="${sec.id}">
          <header class="set-head"><h2 class="set-title">${sec.label}</h2><p class="c-2">${sec.desc}</p></header>
          <div class="set-body" id="set-body">${content}</div>
        </div>
      </div>
      <div class="set-savebar" ${state === 'dirty' ? '' : 'hidden'} role="region" aria-label="Unsaved changes">
        <span class="grow"><strong>Unsaved changes</strong><span class="t-small c-3 sm-up">Saved settings apply to the next search and the next picks run.</span></span>
        <button class="btn btn-ghost" data-act="set-discard">Discard</button><button class="btn btn-primary" data-act="set-save">Save</button></div>`;

    return {
      title: isIndex ? 'Settings' : `${sec.label} · Settings`,
      html,
      mount(root) {
        const body = root.querySelector('#set-body');
        const bar = document.querySelector('.set-savebar');
        const dirty = () => { if (bar) bar.hidden = false; };
        // segmented controls
        body.addEventListener('click', (e) => {
          const b = e.target.closest('.seg button'); if (!b) return;
          b.parentElement.querySelectorAll('button').forEach((x) => x.setAttribute('aria-selected', String(x === b)));
          const k = b.parentElement.dataset.q; if (k) { Q[k] = b.dataset.v; updPreview(root); }
          dirty();
        });
        body.addEventListener('input', (e) => {
          const t = e.target;
          if (t.id === 'q-gbh') { Q.gbh = +t.value; t.style.setProperty('--v', `${((Q.gbh - 1) / 19) * 100}%`); root.querySelector('[data-gbh]').textContent = `${Q.gbh} GB/h`; root.querySelector('[data-gbh-help]').textContent = `${Q.gbh} GB/h ≈ ${(Q.gbh * 2.77).toFixed(0)} GB for Dune: Part Two (2 h 46 m)`; updPreview(root); }
          if (t.dataset.tagInput) return;
          dirty();
        });
        body.addEventListener('change', (e) => {
          const t = e.target;
          if (t.id === 'q-x265') Q.x265 = t.checked; if (t.id === 'q-hdr') Q.hdr = t.checked; if (t.id === 'q-dv') Q.dv = t.checked;
          updPreview(root);
          if (!t.dataset.tagInput) dirty();
        });
        body.addEventListener('keydown', (e) => {
          const t = e.target; if (!t.dataset || !t.dataset.tagInput || e.key !== 'Enter') return;
          e.preventDefault();
          const v = t.value.trim(); if (!v) return;
          const k = t.dataset.tagInput; if (!Q[k].includes(v)) Q[k].push(v);
          t.value = '';
          redrawTags(root, k); updPreview(root); dirty();
          root.querySelector(`[data-tag-input="${k}"]`).focus();
        });
        if (sec.id === 'trakt' && state === 'code') codeTimers(root);
        if (sec.id === 'trakt' && state === 'waiting') waitTimers(root);
      },
    };
  });

  function updPreview(root) {
    const a = root.querySelector('[data-q-preview]'), b = root.querySelector('[data-q-preview-sub]');
    if (a) a.textContent = preview(); if (b) b.textContent = previewSub();
  }
  function redrawTags(root, k) {
    const box = root.querySelector(`[data-tags="${k}"]`); if (!box) return;
    const tmp = document.createElement('div'); tmp.innerHTML = tags(k, k === 'avoid' ? 'Terms to avoid' : 'Preferred groups');
    box.replaceWith(tmp.firstElementChild);
  }
  const embedded = () => document.documentElement.dataset.embed === '1';

  function codeTimers(root) {
    const lbl = root.querySelector('[data-exp]'), bar = root.querySelector('[data-exp-bar]');
    let left = 899; const start = left; let ticks = 0;
    const id = setInterval(() => {
      if (!document.body.contains(lbl)) { clearInterval(id); return; }
      left -= 1; ticks += 1;
      lbl.textContent = `Expires in ${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}`;
      bar.style.width = `${(left / start) * 100}%`;
      // The backend polls Trakt; the prototype pretends you approved after a few seconds
      if (ticks === 8 && !embedded()) { clearInterval(id); app.actions['set-trakt-approved'](); }
    }, 1000);
  }
  function waitTimers(root) {
    const box = root.querySelector('[data-trakt-waiting]');
    if (embedded()) return;
    setTimeout(() => {
      if (!document.body.contains(box)) return;
      box.innerHTML = `<div class="loader set-success" role="status"><span class="set-success-ic">${icon('checkCheck', { size: 30 })}</span><div class="stack gap-1"><p class="w-medium c-1">Connected as teymur</p><p class="t-small c-3">Starting your first sync…</p></div></div>`;
      setTimeout(() => {
        if (!document.body.contains(box)) return;
        MS.app.state.trakt = 'syncing'; MS.app.save();
        MS.app.go('#/home');
        setTimeout(() => app.toast(null, { kind: 'ready', title: 'Trakt connected', body: 'Importing 2,184 plays. Your picks fill in over the next minute.', icon: 'link' }), 450);
      }, 1100);
    }, 3200);
  }

  Object.assign(app.actions, {
    'set-trakt-connect': (b) => { b.classList.add('is-loading'); setTimeout(() => { app.state.trakt = 'code'; app.save(); app.go('#/settings?section=trakt'); }, 600); },
    'set-trakt-approved': () => { app.state.trakt = 'waiting'; app.save(); app.go('#/settings?section=trakt'); },
    'set-trakt-back': () => { app.state.trakt = 'code'; app.save(); app.go('#/settings?section=trakt'); },
    'set-trakt-cancel': () => { app.state.trakt = 'off'; app.save(); app.go('#/settings?section=trakt'); },
    'set-trakt-sync': (b) => {
      b.classList.add('is-loading');
      setTimeout(() => { b.classList.remove('is-loading'); const l = document.querySelector('[data-last-sync]'); if (l) l.textContent = 'Last sync just now · syncs every 30 min'; app.toast(null, { kind: 'ready', title: 'Synced with Trakt', body: '3 new plays, 1 new rating', icon: 'refresh' }); }, 1400);
    },
    'set-disconnect': () => app.openDialog('disconnect'),
    'set-run-now': (b) => { b.classList.add('is-loading'); setTimeout(() => { b.classList.remove('is-loading'); app.toast(null, { kind: 'info', title: 'Writing your picks', body: 'About a minute. Opus 5.5 · ≈ $0.36', action: 'Watch', href: '#/home?state=refreshing' }); }, 700); },
    'set-tag-rm': (b) => {
      const k = b.dataset.key; Q[k] = Q[k].filter((x) => x !== b.dataset.v);
      const root = document.getElementById('main'); redrawTags(root, k); updPreview(root);
      const bar = document.querySelector('.set-savebar'); if (bar) bar.hidden = false;
      root.querySelector(`[data-tag-input="${k}"]`)?.focus();
    },
    'set-save': (b) => { b.classList.add('is-loading'); setTimeout(() => { b.classList.remove('is-loading'); b.closest('.set-savebar').hidden = true; app.toast(null, { kind: 'ready', title: 'Settings saved', icon: 'check' }); }, 700); },
    'set-discard': () => { const h = location.hash.replace(/([?&])state=dirty&?/, '$1').replace(/[?&]$/, ''); app.go(h || '#/settings', { noTransition: true }); },
    'set-test': (b) => {
      b.classList.add('is-loading');
      setTimeout(() => {
        b.classList.remove('is-loading');
        const box = document.querySelector('[data-test-box]'); if (!box) return;
        const old = box.querySelector('.set-test-result, p.c-3'); const tmp = document.createElement('div'); tmp.innerHTML = testResult();
        if (old) old.replaceWith(tmp.firstElementChild);
      }, 1300);
    },
    'set-recheck': (b) => { b.classList.add('is-loading'); setTimeout(() => { b.classList.remove('is-loading'); app.toast(null, { kind: 'info', title: 'Re-checked', body: 'The Pirate Bay still times out. Everything else is fine.', icon: 'refresh' }); }, 1200); },
  });
})();
