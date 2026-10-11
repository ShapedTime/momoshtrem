/* System states: /offline (backend unreachable), /not-found (404), /states (feedback patterns gallery) */
(function () {
  const { ui, app, icon, brand, data: D } = MS;
  const esc = ui.esc;

  // ---- /offline ---------------------------------------------------------------------------
  const TECH = `GET https://momoshtrem.duckdns.org/api/status
→ no response after 10 s (ETIMEDOUT)

Last good response   21:42:08 (3 min ago)
Failed attempts      6
WebDAV :36911        answered 21:44:31 (40 s ago)
App version          0.9.2 · PWA, iPhone`;

  app.route('/offline', (p) => {
    const html = `<div class="sys-offline">
      <div class="sys-offline-top">${brand.lockup(26)}<button class="btn btn-ghost btn-sm" data-act="howto">${icon('tv', { size: 16 })}Infuse setup</button></div>
      <div class="sys-offline-main">
        <div class="sys-dark" aria-hidden="true">${brand.eyes(96, { still: true })}</div>
        <div class="stack gap-3">
          <h1>momoshtrem isn't responding</h1>
          <p class="c-2 t-body-l">The server didn't answer for the last 3 minutes. Nothing is lost: your library, mappings and settings live on the server and will be right here when it's back.</p>
        </div>
        <ul class="panel sys-works" aria-label="What still works">
          <li><span class="ic ok">${icon('check', { size: 15, stroke: 2.4 })}</span><div><strong>Infuse keeps playing ready titles</strong><span class="t-small c-3">WebDAV answered 40 s ago. Only the app's API is quiet.</span></div></li>
          <li><span class="ic ok">${icon('check', { size: 15, stroke: 2.4 })}</span><div><strong>Downloads pick up where they stopped</strong><span class="t-small c-3">No need to re-add anything.</span></div></li>
          <li><span class="ic no">${icon('pause', { size: 14 })}</span><div><strong>Search, adding and Activity wait</strong><span class="t-small c-3">They need the server. Anything you were typing is kept on this device.</span></div></li>
        </ul>
        <div class="sys-retry">
          <button class="btn btn-primary btn-lg" data-act="sys-retry">${icon('refresh', { size: 18 })}Retry now</button>
          <span class="t-small c-3 t-num" data-sys-count role="status" aria-live="polite">Trying again in 15 s · attempt 7</span>
        </div>
        <details class="sys-tech"><summary>Technical details${icon('chevronDown', { size: 16 })}</summary>
          <pre>${esc(TECH)}</pre>
          <div class="row end" style="margin-top:var(--sp-2)"><button class="btn btn-ghost btn-sm" data-act="copy" data-copy="${esc(TECH)}">${icon('copy', { size: 15 })}Copy details</button></div>
        </details>
      </div>
    </div>`;
    return {
      title: 'Offline', bare: true, html,
      mount(root) {
        const lbl = root.querySelector('[data-sys-count]');
        let n = 15, attempt = 7;
        const id = setInterval(() => {
          if (!document.body.contains(lbl)) { clearInterval(id); return; }
          n -= 1;
          if (n === 0) { lbl.textContent = 'Checking…'; return; }
          if (n < 0) { attempt += 1; n = 30; }
          lbl.textContent = `Trying again in ${n} s · attempt ${attempt}`;
        }, 1000);
      },
    };
  });

  // ---- /not-found ---------------------------------------------------------------------------
  app.route('/not-found', (p) => ({
    title: 'Not found',
    html: `<div class="page sys-404">
      ${ui.cat('peek', 200)}
      <span class="code">404</span>
      <h1>Nothing here but the cat</h1>
      <p>${p.from ? `<span class="t-mono c-1">${esc(p.from)}</span> doesn't exist. ` : ''}The link may be from an older version, or the title was removed from your library.</p>
      <div class="row gap-2 wrap center"><a class="btn btn-primary" href="#/home">Go to For You</a><button class="btn btn-secondary" data-act="palette">${icon('search', { size: 17 })}Search</button></div>
    </div>`,
  }));

  // ---- /states gallery ----------------------------------------------------------------------
  const toast = (kind, ic, title, body, action) => `<div class="toast toast--${kind}" role="status">${icon(ic, { size: 20 })}<div class="grow"><strong>${title}</strong>${body ? `<span class="c-2">${body}</span>` : ''}</div>${action ? `<button class="btn btn-ghost btn-sm">${action}</button>` : ''}<button class="btn btn-ghost btn-sm btn-icon" aria-label="Dismiss">${icon('x', { size: 16 })}</button></div>`;
  const frame = (cap, body, note = '') => `<div class="sys-frame"><p class="cap"><b>${cap}</b>${note ? `<span>${note}</span>` : ''}</p>${body}</div>`;
  const section = (id, title, sub, body, extra = '') => `<section class="sys-sec" aria-labelledby="${id}"><div class="head"><div class="stack"><h2 class="t-title-2" id="${id}">${title}</h2><p class="t-small c-3">${sub}</p></div>${extra}</div>${body}</section>`;
  const ringFig = (pct, cap, size = 52) => `<figure><span class="sys-ring-wrap">${ui.ring(pct, size)}<span>${pct}%</span></span><figcaption>${cap}</figcaption></figure>`;

  app.route('/states', () => {
    const toasts = section('st-toasts', 'Toasts', 'Bottom-centre above the tab bar on phone, bottom-right on desktop. Success and info leave after 5 s; errors stay until dismissed.', `
      <div class="sys-grid sys-grid--3">
        ${frame('Success · with action', toast('ready', 'checkCheck', 'Added to library', 'The Good Place · 4 seasons, 53 episodes', 'Find a torrent'))}
        ${frame('Info · undo', toast('info', 'trash', 'Removed Bluey · Season 3', 'Undo within 10 seconds', 'Undo'))}
        ${frame('Error · sticky, with retry', toast('error', 'alertTriangle', 'Couldn\'t reach momoshtrem', 'Retrying in 10 s. Your changes are kept.', 'Retry now'))}
      </div>`, `<div class="row gap-2 wrap"><button class="btn btn-secondary btn-sm" data-act="sys-toast" data-k="added">Show success</button><button class="btn btn-secondary btn-sm" data-act="sys-toast" data-k="removed">Show info</button><button class="btn btn-secondary btn-sm" data-act="sys-toast" data-k="error">Show error</button></div>`);

    const dialog = section('st-dialog', 'Confirmation', 'Only for destructive actions. Says exactly what goes and what stays. A centred dialog everywhere; on phone the buttons stack with Cancel nearest the thumb.', `
      <div class="sys-grid sys-grid--2">
        ${frame('Dialog (rendered inline)', `<div class="dialog is-inline" role="group" aria-labelledby="st-dlg-t">
            <h3 class="t-title-3" id="st-dlg-t">Remove Severance from your library?</h3>
            <p class="c-2">This removes the show and its 19 episode mappings. Downloaded data for its 2 torrents is deleted too. Your Trakt history isn't touched.</p>
            <label class="row gap-3" style="min-height:44px"><input type="checkbox" checked style="width:20px;height:20px;accent-color:var(--accent)"> <span class="t-small c-2">Also delete downloaded data (58.3 GB)</span></label>
            <div class="actions"><button class="btn btn-secondary">Cancel</button><button class="btn btn-danger">Remove show</button></div></div>`, `<button class="btn btn-ghost btn-sm" data-act="sys-dialog">Open it for real</button>`)}
        ${frame('The trigger stays quiet', `<div class="stack gap-3">
            <p class="t-small c-2">Destructive triggers are ghost buttons or icon buttons, never bigger than their neighbours. The red lives in the dialog's confirm button.</p>
            <div class="row gap-2 wrap"><button class="btn btn-secondary btn-sm">${icon('captions', { size: 15 })}Subtitles</button><button class="btn btn-secondary btn-sm">${icon('refresh', { size: 15 })}Replace file</button><span class="grow"></span><button class="btn btn-ghost btn-sm btn-danger-quiet">${icon('trash', { size: 15 })}Remove</button></div>
            <div class="divider"></div>
            <div class="row gap-2"><span class="t-small c-3 grow">In a card: the trash icon sits last, apart from the rest.</span><button class="btn btn-ghost btn-sm btn-icon" aria-label="Remove torrent">${icon('trash', { size: 16 })}</button></div></div>`)}
      </div>`);

    const progress = section('st-progress', 'Progress', 'Determinate when we know how far along it is, indeterminate only for short waits, steps when the work has phases.', `
      <div class="sys-grid sys-grid--3">
        ${frame('Bars', `<div class="stack gap-4">
            <div class="stack gap-1"><div class="row between t-small"><span>Frieren · S01E29–E38</span><span class="c-3 t-num">64%</span></div>${ui.progress(64)}</div>
            <div class="stack gap-1"><div class="row between t-small"><span>Streaming buffer</span><span class="c-3 t-num">6 min ahead</span></div>${ui.progress(42, 'accent')}</div>
            <div class="stack gap-1"><div class="row between t-small"><span>Watched</span><span class="c-3 t-num">13 of 19</span></div>${ui.progress(68, 'watched')}</div>
            <div class="stack gap-1"><div class="row between t-small"><span>Searching 3 indexers</span><span class="c-3">a few seconds</span></div>${ui.progress(30, '', 'progress--indeterminate')}</div>
          </div>`)}
        ${frame('Rings and buttons', `<div class="stack gap-5">
            <div class="sys-ring-row">${ringFig(12, 'Starting')}${ringFig(64, 'Downloading')}${ringFig(100, 'Done')}<figure>${ui.pill('downloading', '42%', { pct: 42 })}<figcaption>In a pill</figcaption></figure></div>
            <div class="row gap-2 wrap"><button class="btn btn-primary is-loading" aria-label="Adding">Add</button><button class="btn btn-secondary is-loading" aria-label="Testing">Test connection</button><button class="btn btn-ready">${icon('check', { size: 18 })}Added</button></div>
          </div>`)}
        ${frame('Steps', `<div class="steps">
            <div class="step is-done"><span class="ic">${icon('check', { size: 14, stroke: 2.5 })}</span>Fetching metadata<span class="detail">13 files</span></div>
            <div class="step is-done"><span class="ic">${icon('check', { size: 14, stroke: 2.5 })}</span>Matching files<span class="detail">10 matched</span></div>
            <div class="step is-active"><span class="ic"></span>Asking AI about the tricky ones<span class="detail">2 left</span></div>
            <div class="step"><span class="ic"></span>Ready to review</div>
          </div>`)}
      </div>
      ${frame('Longer waits: Noir\'s eyes in the dark', ui.loader('Writing your picks…', 'Reading your history · 2 of 5 shelves done'))}`);

    const detailSkel = `<div class="stack gap-4"><div class="skel skel-wide" style="aspect-ratio:21/9"></div>
      <div class="sys-skel-detail"><div class="skel skel-poster"></div><div class="stack gap-3"><div class="skel skel-line" style="width:30%"></div><div class="skel" style="height:28px;width:70%;border-radius:8px"></div><div class="skel skel-line" style="width:55%"></div><div class="row gap-2"><div class="skel" style="height:44px;width:150px;border-radius:12px"></div><div class="skel" style="height:44px;width:96px;border-radius:12px"></div></div></div></div>
      <div class="stack gap-2"><div class="skel skel-line" style="width:92%"></div><div class="skel skel-line" style="width:86%"></div><div class="skel skel-line" style="width:60%"></div></div></div>`;
    const skeletons = section('st-skel', 'Skeleton loading', 'Same shapes and sizes as the content, so nothing jumps when it arrives. The shimmer stops under reduced motion.', `
      <div class="sys-grid sys-grid--2-1">
        ${frame('Grid (Library, Discover)', `<div class="sys-skel-grid">${ui.skelPosters(10)}</div>`)}
        ${frame('Detail page', detailSkel)}
      </div>
      ${frame('Rail of landscape cards (Up next)', `<div class="sys-skel-rail">${ui.skelWide(4)}</div>`)}`);

    const empties = section('st-empty', 'Empty states', 'Noir\'s silhouette, one plain sentence about why it\'s empty, and the next step.', `
      <div class="sys-grid sys-grid--3">
        ${frame('Empty library', ui.empty({ cat: 'sit', title: 'Your library is empty', body: 'Add a movie or show and it lands here, then in Infuse once it\'s downloaded.', actions: `<a class="btn btn-primary" href="#/discover">Browse Discover</a><button class="btn btn-ghost" data-act="palette">Search</button>` }))}
        ${frame('No results', `${ui.empty({ cat: 'peek', title: 'No torrents match these filters', body: '45 results are hidden by <strong class="c-1">2160p</strong> and <strong class="c-1">Dolby Vision</strong>.', actions: `<button class="fchip">Remove Dolby Vision <span class="n">+12</span></button><button class="fchip">Allow 1080p <span class="n">+31</span></button>` })}`)}
        ${frame('Trakt not connected', ui.empty({ cat: 'sleep', title: 'Up next needs Trakt', body: 'Connect it once and momoshtrem knows which episode is next for every show you watch. Read-only.', actions: `<a class="btn btn-primary" href="#/settings?section=trakt&state=off">${icon('link', { size: 17 })}Connect Trakt</a>` }))}
      </div>`);

    const errors = section('st-errors', 'Errors', 'Calm and specific: what failed, what still works, and one way forward. Never cute when something important broke.', `
      <div class="sys-grid sys-grid--2">
        ${frame('Backend offline (banner)', `<div class="banner banner--error" role="alert">${icon('wifiOff', { size: 20 })}<div class="grow"><strong>Can't reach momoshtrem</strong><span>Showing what it said at 21:42. Infuse can still play ready titles.</span></div><div class="actions"><button class="btn btn-secondary btn-sm">${icon('refresh', { size: 15 })}Retry</button></div></div>`, '<a class="btn btn-ghost btn-sm" href="#/offline">Full page</a>')}
        ${frame('Indexer failures (partial results)', `<div class="banner banner--warning" role="status">${icon('alertTriangle', { size: 20 })}<div class="grow"><strong>2 indexers timed out</strong><span>Showing 27 results from Nyaa. TorrentLeech and The Pirate Bay didn't answer within 8 s.</span></div><div class="actions"><button class="btn btn-secondary btn-sm">Retry them</button></div></div>`)}
        ${frame('Inline (a section failed to load)', `<div class="panel panel-pad row gap-3">${icon('alertCircle', { size: 20, cls: 'c-3' })}<div class="grow stack"><strong class="w-semibold">Couldn't load episodes for Season 21</strong><span class="t-small c-3">TMDB answered with an error. The rest of the page is fine.</span></div><button class="btn btn-secondary btn-sm">Try again</button></div>`)}
        ${frame('Field error', `<div class="field"><label class="label" for="st-budget">Monthly AI budget</label><div class="input-wrap"><input id="st-budget" class="input" value="-5" aria-invalid="true" aria-describedby="st-budget-e" style="padding-left:12px"></div><p class="error-text" id="st-budget-e">${icon('alertCircle', { size: 15 })}Enter an amount from $0 to $200.</p></div>`)}
      </div>`);

    return {
      title: 'Feedback & states',
      html: `<div class="page sys-gallery">
        <header><span class="t-micro c-accent">Design system</span><h1 class="t-display">Feedback &amp; states</h1><p class="c-2 t-body-l">How momoshtrem says "working on it", "done", "nothing here" and "something broke", shown in place with real content.</p></header>
        ${toasts}${dialog}${progress}${skeletons}${empties}${errors}
      </div>`,
    };
  });

  const TOASTS = {
    added: { kind: 'ready', title: 'Added to library', body: 'The Good Place · 4 seasons, 53 episodes', action: 'Find a torrent', act: 'find-goodplace' },
    removed: { kind: 'info', title: 'Removed Bluey · Season 3', body: 'Undo within 10 seconds', action: 'Undo', icon: 'trash' },
    error: { kind: 'error', title: 'Couldn\'t reach momoshtrem', body: 'Retrying in 10 s. Your changes are kept.', action: 'Retry now', sticky: true },
  };
  Object.assign(app.actions, {
    'sys-toast': (b) => app.toast(null, TOASTS[b.dataset.k]),
    'sys-dialog': () => app.openDialog('remove'),
    'sys-retry': (b) => {
      b.classList.add('is-loading');
      const l = document.querySelector('[data-sys-count]'); if (l) l.textContent = 'Checking…';
      setTimeout(() => { b.classList.remove('is-loading'); if (l) l.textContent = 'Still no answer. Trying again in 30 s'; }, 1400);
    },
  });
})();
