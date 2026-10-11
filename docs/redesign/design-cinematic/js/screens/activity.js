/* Activity: "is everything working?" + per-torrent detail.
   Routes: /activity (states: default · empty · offline · loading)
           /activity/:hash (examples via ?state=: streaming · downloading · weak · paused · complete) */
(function () {
  const { ui, app, icon, data: D } = MS;
  const T = (id) => D.T[id];
  const esc = ui.esc;
  const fmt = (n) => Number(n).toLocaleString('en-US');

  // ---- Session-local torrent state (pause/resume/remove) --------------------
  const ov = {};        // hash -> { state, streaming }
  const removed = {};   // hash -> true
  let pendingRemove = null;
  const list = () => D.torrents.filter((t) => !removed[t.hash]).map((t) => ({ ...t, ...(ov[t.hash] || {}) }));
  const byHash = (h) => list().find((t) => t.hash === h);

  // ---- Extra fixtures: files, pieces, readers, trackers (GET /torrents/{hash}/files) ----
  const SHOGUN_EPS = ['Anjin', 'Servants of Two Masters', 'Tomorrow Is Tomorrow', 'The Eightfold Fence', 'Broken to the Fist', 'Ladies of the Willow World', 'A Stick of Time', 'The Abyss of Life', 'Crimson Sky', 'A Dream of a Dream'];
  const SEV_EPS = ['Hello, Ms. Cobel', 'Goodbye, Mrs. Selvig', 'Who Is Alive?', 'Woe\'s Hollow', 'Trojan\'s Horse', 'Attila', 'Chikhai Bardo', 'Sweet Vitriol', 'The After Hours', 'Cold Harbor'];
  const se = (s, e) => `S${String(s).padStart(2, '0')}E${String(e).padStart(2, '0')}`;
  const dot = (s) => s.replace(/['’?,]/g, '').replace(/\s+/g, '.');

  const DETAIL = {
    '7b20…11fa': {
      infohash: '7b20c4e18d93a6f2b0e5d71c4a8f3e926b5d11fa', pieceMB: 16, added: 'Oct 9, 21:04', ratio: 0.12, uploaded: '3.1 GB', got: '26.0 GB',
      root: 'Shogun.2024.S01.2160p.DSNP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX/',
      files: SHOGUN_EPS.map((n, i) => ({ p: `Shogun.2024.${se(1, i + 1)}.${dot(n)}.2160p.DSNP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv`, gb: [4.1, 4.4, 4.2, 4.3, 4.3, 4.2, 4.5, 4.2, 4.3, 4.1][i], pct: [100, 100, 100, 100, 44, 40, 36, 34, 30, 26][i], map: se(1, i + 1), name: n, reading: i === 4 })),
      readers: [{ who: 'Infuse', dev: 'Apple TV · Living room', file: 'S01E05', from: 1204, to: 1260, speed: '9.1 MB/s', pos: '18:42 / 58:10' }],
      trackers: [['udp://tracker.opentrackr.org:1337/announce', 'ok', '118 peers · 2 min ago'], ['udp://open.stealth.si:80/announce', 'ok', '64 peers · 4 min ago'], ['udp://exodus.desync.com:6969/announce', 'error', 'Timed out · retrying in 3 min']],
      health: ['ready', 'Healthy', '140 seeders, 23 connected. Pieces for S01E05 arrive faster than Infuse plays them, so playback stays about 6 minutes ahead. Episodes 6–10 fill in behind it.'],
    },
    'a1f3…9c2e': {
      infohash: 'a1f35e0b7c2d48e9916fd0a3b54c27e18d6a9c2e', pieceMB: 8, added: 'Today, 21:36', ratio: 0.09, uploaded: '780 MB', got: '8.4 GB',
      root: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]/',
      files: [29, 30, 31, 32, 33, 34, 35, 36, 37, 38].map((n, i) => ({ p: `[SubsPlease] Sousou no Frieren - ${n} (1080p) [${(0x8E3D5C21 + n * 977).toString(16).toUpperCase().slice(0, 8)}].mkv`, gb: [1.32, 1.28, 1.36, 1.29, 1.31, 1.33, 1.3, 1.28, 1.35, 1.32][i], pct: [100, 100, 100, 100, 100, 92, 61, 40, 22, 8][i], map: se(1, n) }))
        .concat([{ p: '[SubsPlease] Sousou no Frieren - NCOP2 (1080p).mkv', gb: 0.14, pct: 0, ignored: 'Ignored (NCOP)' }, { p: '[SubsPlease] Sousou no Frieren - NCED2 (1080p).mkv', gb: 0.14, pct: 0, ignored: 'Ignored (NCED)' }, { p: 'Fonts/OpenSans-Semibold.ttf', gb: 0.0002, pct: 0, ignored: 'Ignored (font)' }]),
      readers: [],
      trackers: [['http://nyaa.tracker.wf:7777/announce', 'ok', '212 seeders · 1 min ago'], ['udp://tracker.opentrackr.org:1337/announce', 'ok', '96 peers · 3 min ago']],
      health: ['ready', 'Healthy', '212 seeders, 47 connected. Downloading at full speed; the last episode should be playable in about 4 minutes.'],
    },
    'c9d4…a7b3': {
      infohash: 'c9d4a1e07f3b5268d4c19e0a7b2f6d3851e0a7b3', pieceMB: 32, added: 'Yesterday, 23:10', ratio: 0.01, uploaded: '210 MB', got: '26.5 GB',
      root: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT/',
      files: [{ p: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT.mkv', gb: 63.0, pct: 42, map: 'Movie' }, { p: 'Sample/interstellar.2014.2160p.remux-fgt.sample.mkv', gb: 0.09, pct: 0, ignored: 'Ignored (sample)' }, { p: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT.nfo', gb: 0.00001, pct: 100, ignored: 'Ignored (not video)' }],
      readers: [],
      trackers: [['udp://tracker.opentrackr.org:1337/announce', 'ok', '9 seeders · 5 min ago'], ['udp://tracker.torrent.eu.org:451/announce', 'warn', '0 peers · 12 min ago']],
      health: ['warning', 'Slow but moving', 'Only 9 seeders and 6 connected, so this one crawls at about 2 MB/s. Every piece is still available, so it will finish: about 1 h 52 min. Nothing for you to do.'],
    },
    '0e55…3d1c': {
      infohash: '0e55f2a9b18c47d3a06e5b9c2d71f84e6a0b3d1c', pieceMB: 16, added: 'Oct 4, 19:22', ratio: 0.31, uploaded: '12.6 GB', got: '40.8 GB',
      root: 'Severance.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX/',
      files: SEV_EPS.map((n, i) => ({ p: `Severance.${se(2, i + 1)}.${dot(n)}.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX.mkv`, gb: [5.6, 5.9, 5.5, 6.2, 5.8, 5.7, 5.1, 6.0, 6.1, 6.4][i], pct: [100, 100, 100, 100, 100, 100, 100, 0, 0, 0][i], map: se(2, i + 1), name: n })),
      readers: [],
      trackers: [['udp://tracker.opentrackr.org:1337/announce', 'idle', 'Paused'], ['udp://open.stealth.si:80/announce', 'idle', 'Paused']],
      health: ['neutral', 'Paused', 'You paused this on Oct 6. Episodes 1–7 are complete and play in Infuse; 8–10 wait until you resume.'],
    },
  };
  function completeDetail(tr) {
    const isBluey = tr.title === 'bluey';
    const n = isBluey ? 49 : tr.title === 'thebear' ? 10 : 1;
    const base = tr.name;
    const files = n === 1
      ? [{ p: `${base}.mkv`, gb: parseFloat(tr.size), pct: 100, map: 'Movie' }]
      : Array.from({ length: n }, (_, i) => ({ p: `${base.replace(/\.S0(\d)\./, (m, s) => `.S0${s}E${String(i + 1).padStart(2, '0')}.`)}.mkv`, gb: +(parseFloat(tr.size) / n).toFixed(2), pct: 100, map: se(isBluey ? 3 : 4, i + 1) }));
    return {
      infohash: (tr.hash.replace('…', 'd1e2c3b4a5968778695a4b3c2d1e0f') + '0000000000').slice(0, 40), pieceMB: 16, added: 'Sep 28, 20:15', ratio: 0.84, uploaded: `${(parseFloat(tr.size) * 0.84).toFixed(1)} GB`, got: tr.size,
      root: n === 1 ? '' : `${base}/`, files, readers: [],
      trackers: [['udp://tracker.opentrackr.org:1337/announce', 'idle', 'Not announcing while idle']],
      health: ['ready', 'Complete', `Everything is downloaded and on disk. Infuse can play it any time. It isn't seeding right now; it wakes up when Infuse opens a file. ${tr.lastPlayed}.`],
    };
  }
  const detailFor = (tr) => DETAIL[tr.hash] || completeDetail(tr);

  // ---- Live speed samples (last 60 s) ------------------------------------------
  const SCALE = 40; // MB/s, fixed axis so the line is drawn to scale
  let seed = 7;
  const rnd = () => { seed = (seed * 9301 + 49297) % 233280; return seed / 233280; };
  const samples = Array.from({ length: 60 }, (_, i) => 26 + Math.sin(i / 6) * 3 + rnd() * 3);
  samples[59] = 29.8;
  const totalDown = () => samples[samples.length - 1];

  function sparkPath(vals, w = 120, h = 40) {
    const pts = vals.map((v, i) => [(i / (vals.length - 1)) * w, h - (v / SCALE) * h]);
    const line = pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)} ${p[1].toFixed(1)}`).join('');
    return { line, area: `${line}L${w} ${h}L0 ${h}Z` };
  }
  function spark(vals, opts = {}) {
    const { line, area } = sparkPath(vals);
    return `<div class="act-spark ${opts.flat ? 'is-flat' : ''}" data-spark>
      <svg viewBox="0 0 120 40" preserveAspectRatio="none" role="img" aria-label="Total download speed over the last minute, scale 0 to ${SCALE} MB/s">
        <line x1="0" y1="0.5" x2="120" y2="0.5" class="grid" vector-effect="non-scaling-stroke"/>
        <line x1="0" y1="20" x2="120" y2="20" class="grid" vector-effect="non-scaling-stroke"/>
        <path class="area" d="${area}"/><path class="line" d="${line}" vector-effect="non-scaling-stroke"/>
      </svg>
      <span class="scale-top">${SCALE} MB/s</span>
      <span class="hover-dot" hidden></span><span class="hover-tip" hidden></span>
    </div>`;
  }

  // ---- Summary strip ----------------------------------------------------------------
  function summary(ts, opts = {}) {
    const streaming = ts.find((t) => t.streaming && t.state !== 'paused');
    const active = opts.empty ? 0 : ts.filter((t) => t.state === 'downloading' || (t.streaming && t.state !== 'paused')).length;
    const paused = opts.empty ? 0 : ts.filter((t) => t.state === 'paused').length;
    const idle = ts.filter((t) => t.state === 'idle').length;
    const stale = opts.offline;
    const down = opts.empty ? '0' : totalDown().toFixed(1);
    const up = opts.empty ? '0' : '1.7';
    const now = streaming && !opts.empty
      ? `<a class="act-now" href="#/activity/${encodeURIComponent(streaming.hash)}">
          <span class="ic">${icon('tv', { size: 22 })}</span>
          <span class="grow"><span class="t-micro">Streaming now</span><strong>Shōgun S01E05 on Apple TV</strong><span class="t-small c-2">Broken to the Fist · 18:42 of 58:10 · buffered 6 min ahead</span></span>
          ${icon('chevronRight', { size: 18, cls: 'c-3' })}</a>`
      : `<div class="act-now is-quiet"><span class="ic">${icon('tv', { size: 22 })}</span><span class="grow"><span class="t-micro">Streaming now</span><strong>Nothing playing</strong><span class="t-small c-3">Infuse shows up here when it opens a file</span></span></div>`;
    return `<section class="act-summary ${stale ? 'is-stale' : ''}" aria-label="Summary">
      ${now}
      <div class="act-tile act-tile--down"><span class="label">${icon('arrowDown', { size: 15 })}Download</span><span class="val t-num"><b data-live-down>${down}</b> MB/s</span>${spark(opts.empty ? samples.map(() => 0) : samples, { flat: opts.empty || stale })}</div>
      <div class="act-tile"><span class="label">${icon('arrowUp', { size: 15 })}Upload</span><span class="val t-num"><b>${up}</b> MB/s</span><span class="sub">${opts.empty ? 'Nothing seeding' : 'To 31 peers · limit 5 MB/s'}</span></div>
      <div class="act-tile"><span class="label">${icon('layers', { size: 15 })}Torrents</span><span class="val t-num"><b>${active}</b> ${opts.empty ? 'downloading' : 'active'}</span><span class="sub">${paused} paused · ${idle} idle</span></div>
      <div class="act-tile"><span class="label">${icon('hardDrive', { size: 15 })}Disk</span><span class="val t-num"><b>1.31</b> TB free</span>
        <span class="act-disk" role="img" aria-label="Disk: 235 GB in torrents, 455 GB other, 1.31 TB free of 2 TB"><i style="width:11.8%"></i><i style="width:22.7%"></i></span><span class="sub">235 GB torrents · 2 TB disk</span></div>
    </section>`;
  }

  // tcard with screen-scoped actions (keeps core's generic actions untouched)
  function card(tr) {
    const h = esc(tr.hash);
    return ui.tcard(tr)
      .replace('data-act="pause"', `data-act="act-pause" data-hash="${h}" aria-label="Pause ${esc(tr.label)}"`)
      .replace('data-act="resume"', `data-act="act-resume" data-hash="${h}" aria-label="Resume ${esc(tr.label)}"`)
      .replace('data-act="remove-torrent" aria-label="Remove torrent"', `data-act="act-remove" data-hash="${h}" aria-label="Remove ${esc(tr.label)} torrent"`);
  }
  function group(id, title, sub, items) {
    if (!items.length) return '';
    return `<section class="section act-group" aria-labelledby="${id}-h" id="${id}">
      <div class="section-head"><div class="titles"><h2 class="t-title-3" id="${id}-h">${esc(title)} <span class="c-3 t-num w-medium">${items.length}</span></h2>${sub ? `<p class="t-small c-3">${esc(sub)}</p>` : ''}</div></div>
      <div class="act-list">${items.map(card).join('')}</div></section>`;
  }
  function groups(ts, opts = {}) {
    const streaming = ts.filter((t) => t.streaming && t.state !== 'paused');
    const dl = ts.filter((t) => t.state === 'downloading' && !t.streaming);
    const paused = ts.filter((t) => t.state === 'paused');
    const idle = ts.filter((t) => t.state === 'idle');
    if (opts.empty) {
      return `${ui.empty({ cat: 'sleep', title: 'Nothing downloading.', body: 'Infuse can still play everything that\'s ready.', actions: `<a class="btn btn-secondary" href="#/library?filter=ready">See what's ready</a><button class="btn btn-ghost" data-act="howto">${icon('tv', { size: 18 })}How to play</button>` })}
        ${group('act-idle', 'Complete', 'Ready in Infuse · not seeding while idle', idle)}`;
    }
    const main = group('act-streaming', 'Streaming now', '', streaming)
      + group('act-active', 'Downloading', '', dl)
      + group('act-paused', 'Paused', '', paused);
    return `<div class="act-col-main">${main || `<p class="t-small c-3">Nothing downloading or paused.</p>`}</div>
      <div class="act-col-side">${group('act-idle', 'Complete', 'Ready in Infuse · not seeding while idle', idle)}</div>`;
  }
  function verdict(ts, opts) {
    if (opts.offline) return '';
    const weak = ts.find((t) => t.health === 'weak' && t.state === 'downloading');
    if (opts.empty) return `<p class="act-verdict">${icon('checkCheck', { size: 18, cls: 'ok' })}<span>All quiet. Nothing in the queue, and the server is fine.</span></p>`;
    return `<p class="act-verdict">${icon('checkCheck', { size: 18, cls: 'ok' })}<span><strong>Everything's working.</strong> Infuse is streaming smoothly${weak ? `; ${esc(T(weak.title).title)} is slow because it has few peers, but it will finish.` : '.'}</span></p>`;
  }

  function header(right = '') {
    return `<header class="large-title page act-title"><h1>Activity</h1><div class="actions">${right}<button class="avatar-btn phone-only" data-act="menu" aria-label="Account menu: settings, log out">${MS.brand.markMono(20, 'var(--text-2)')}</button></div></header>`;
  }

  function loadingView() {
    const tile = '<div class="act-tile"><div class="skel skel-line" style="width:50%"></div><div class="skel" style="height:26px;width:70%;border-radius:8px"></div><div class="skel skel-line" style="width:80%"></div></div>';
    const tcardSk = '<div class="tcard" aria-hidden="true"><div class="skel skel-poster" style="width:56px"></div><div class="stack gap-3"><div class="skel skel-line" style="width:60%"></div><div class="skel skel-line" style="width:90%"></div><div class="skel" style="height:4px"></div><div class="skel skel-line" style="width:45%"></div></div></div>';
    return `<div class="page act-body" aria-busy="true">
      <p class="act-verdict c-3" role="status"><span class="act-eyes">${MS.brand.eyes(30)}</span><span>Checking on your torrents…</span></p>
      <section class="act-summary" aria-hidden="true"><div class="act-now is-quiet"><div class="skel" style="width:44px;height:44px;border-radius:12px"></div><div class="stack gap-2 grow"><div class="skel skel-line" style="width:30%"></div><div class="skel skel-line" style="width:65%"></div></div></div>${tile}${tile}${tile}${tile}</section>
      <section class="section act-group"><div class="skel skel-line" style="width:140px;height:16px"></div><div class="act-list">${tcardSk}${tcardSk}</div></section>
      <section class="section act-group"><div class="skel skel-line" style="width:110px;height:16px"></div><div class="act-list">${tcardSk}${tcardSk}${tcardSk}</div></section>
    </div>`;
  }

  function offlineBanner() {
    return `<div class="banner act-offline" role="alert">${icon('wifiOff', { size: 20 })}
      <div class="grow"><strong>Can't reach momoshtrem</strong><span>Showing what it said at 21:42. Downloads may still be running on the server; Infuse can still play ready titles if WebDAV is up.</span><span class="c-3" data-retry-label>Retrying in 12 s</span></div>
      <div class="actions"><button class="btn btn-secondary btn-sm" data-act="act-retry">${icon('refresh', { size: 15 })}Retry now</button></div></div>`;
  }

  app.route('/activity', (p) => {
    const state = p.state || 'default';
    const switcher = ui.protoStates('#/activity', [['default', 'Live'], ['empty', 'Nothing downloading'], ['offline', 'Backend offline'], ['loading', 'Loading']], state);
    const ts = list();
    const opts = { empty: state === 'empty', offline: state === 'offline' };
    let body;
    if (state === 'loading') body = loadingView();
    else {
      body = `<div class="page act-body ${opts.offline ? 'is-offline' : ''}">
        ${opts.offline ? `${offlineBanner()}<p class="t-micro c-3 act-lastknown">Last known · 21:42</p>` : verdict(ts, opts)}
        ${summary(ts, opts)}
        <div class="act-groups ${opts.offline ? 'is-stale' : ''}" id="act-groups" ${opts.offline ? 'inert aria-label="Last known torrents (offline)"' : ''}>${groups(ts, opts)}</div>
      </div>`;
    }
    const pauseAll = state === 'default' ? `<button class="btn btn-ghost btn-sm" data-act="act-pause-all">${icon('pause', { size: 15 })}Pause all</button>` : '';
    return {
      title: 'Activity', tab: 'activity',
      html: `${switcher}${header(pauseAll)}${body}`,
      mount(root) {
        const sp = root.querySelector('.act-tile--down [data-spark]');
        if (sp && state === 'default') { live(sp, root); hover(sp); }
        if (state === 'offline') retryCountdown(root.querySelector('[data-retry-label]'), 12);
      },
    };
  });

  function live(sp, root) {
    const val = root.querySelector('[data-live-down]');
    const id = setInterval(() => {
      if (!document.body.contains(sp)) { clearInterval(id); return; }
      const last = samples[samples.length - 1];
      const next = Math.max(20, Math.min(34, last + (Math.random() - 0.5) * 3.2));
      samples.shift(); samples.push(next);
      const { line, area } = sparkPath(samples);
      sp.querySelector('.line').setAttribute('d', line);
      sp.querySelector('.area').setAttribute('d', area);
      if (val) val.textContent = next.toFixed(1);
    }, 1000);
  }
  function hover(sp) {
    const dotEl = sp.querySelector('.hover-dot'), tip = sp.querySelector('.hover-tip');
    const show = (e) => {
      const r = sp.getBoundingClientRect();
      const x = Math.max(0, Math.min(r.width, e.clientX - r.left));
      const i = Math.round((x / r.width) * (samples.length - 1));
      const v = samples[i];
      const px = (i / (samples.length - 1)) * r.width, py = r.height - (v / SCALE) * r.height;
      dotEl.hidden = tip.hidden = false;
      dotEl.style.transform = `translate(${px}px, ${py}px)`;
      tip.textContent = `${samples.length - 1 - i === 0 ? 'Now' : `${samples.length - 1 - i} s ago`} · ${v.toFixed(1)} MB/s`;
      tip.style.left = `${Math.max(0, Math.min(r.width - 120, px - 60))}px`;
    };
    sp.addEventListener('pointermove', show);
    sp.addEventListener('pointerdown', show);
    sp.addEventListener('pointerleave', () => { dotEl.hidden = tip.hidden = true; });
  }
  function retryCountdown(label, from) {
    if (!label) return;
    let n = from;
    const id = setInterval(() => {
      if (!document.body.contains(label)) { clearInterval(id); return; }
      n -= 1;
      if (n <= 0) { label.textContent = 'Checking…'; n = 31; return; }
      if (n === 30) label.textContent = 'Still not answering. Retrying in 30 s';
      else label.textContent = `Retrying in ${n} s`;
    }, 1000);
  }

  // ---- Per-torrent detail ----------------------------------------------------------------
  const EXAMPLES = [['streaming', 'Streaming', '7b20…11fa'], ['downloading', 'Downloading batch', 'a1f3…9c2e'], ['weak', 'Few peers', 'c9d4…a7b3'], ['paused', 'Paused', '0e55…3d1c'], ['complete', 'Complete', '9a02…77e4']];

  function fileRow(f, i, all) {
    const more = all.length > 14;
    const pct = f.pct;
    const map = f.ignored ? `<span class="chip">${esc(f.ignored)}</span>`
      : `<span class="chip chip--hi">${esc(f.map)}</span>${f.name ? `<span class="t-small c-3 clamp-1">${esc(f.name)}</span>` : ''}`;
    const size = f.gb >= 1 ? `${f.gb.toFixed(1)} GB` : f.gb >= 0.001 ? `${Math.round(f.gb * 1000)} MB` : `${Math.max(1, Math.round(f.gb * 1e6))} KB`;
    const prog = f.ignored ? '<span class="t-small c-3">Skipped</span>'
      : pct >= 100 ? `<span class="f-done">${icon('check', { size: 14, stroke: 2.4 })}Ready</span>`
      : `${ui.progress(pct, f.reading ? 'accent' : '')}<span class="t-num">${pct}%</span>`;
    return `<li class="td-file ${f.ignored ? 'is-ignored' : ''} ${f.reading ? 'is-reading' : ''}" ${more && i >= 10 ? 'data-more hidden' : ''}>
      <div class="f-name">${f.reading ? `<span class="pill pill--accent f-badge">${icon('tv', { size: 13 })}Infuse is reading</span>` : ''}${ui.path(f.p)}</div>
      <div class="f-map">${map}</div>
      <div class="f-size t-num">${size}</div>
      <div class="f-prog">${prog}</div>
    </li>`;
  }

  function stateFor(tr) {
    const st = tr.streaming && tr.state !== 'paused' ? 'streaming' : tr.state;
    if (st === 'streaming') return ui.pill('streaming', 'Streaming now', { lg: true });
    if (st === 'downloading') return ui.pill('downloading', `Downloading ${tr.pct}%`, { pct: tr.pct, lg: true });
    if (st === 'paused') return ui.pill('paused', 'Paused', { lg: true });
    return ui.pill('ready', 'Complete · ready in Infuse', { icon: 'check', lg: true });
  }

  app.route('/activity/:hash', (p) => {
    const ex = EXAMPLES.find((e) => e[0] === p.state);
    const hash = ex ? ex[2] : p.hash;
    const tr = byHash(hash) || (ex && D.torrents.find((t) => t.hash === hash));
    const current = ex ? ex[0] : (EXAMPLES.find((e) => e[2] === hash) || [])[0];
    const switcher = ui.protoStates(`#/activity/${encodeURIComponent(hash)}`, EXAMPLES.map((e) => [e[0], e[1]]), current || 'none');
    if (!tr) {
      return { title: 'Torrent not found', tab: 'activity', html: `<div class="page">${ui.empty({ cat: 'peek', title: 'That torrent is gone', body: 'It may have been removed. Your library entry stays put and shows "Needs a torrent".', actions: '<a class="btn btn-secondary" href="#/activity">Back to Activity</a>' })}</div>` };
    }
    const t = T(tr.title);
    const d = detailFor(tr);
    const st = tr.streaming && tr.state !== 'paused' ? 'streaming' : tr.state;
    const active = st === 'downloading' || st === 'streaming';
    const titleHref = `#/title/${t.type}/${t.id}`;
    const totalPieces = Math.round(d.files.reduce((a, f) => a + f.gb, 0) * 1024 / d.pieceMB);
    const havePieces = Math.round(totalPieces * tr.pct / 100);
    const stats = [
      active ? ['arrowDown', 'Down', tr.down] : null,
      active ? ['arrowUp', 'Up', tr.up] : null,
      active ? ['users', 'Peers', `${tr.peers} of ${tr.seeds} seeders`] : null,
      tr.eta ? ['clock', 'Time left', tr.eta] : st === 'streaming' ? ['clock', 'Time left', 'about 9 min'] : null,
      ['hardDrive', 'Size', tr.pct < 100 ? `${d.got} of ${tr.size}` : tr.size],
    ].filter(Boolean);
    const [hKind, hTitle, hWords] = d.health;
    const hIcon = hKind === 'warning' ? 'alertTriangle' : hKind === 'neutral' ? 'pause' : 'checkCheck';
    const videoFiles = d.files.filter((f) => !f.ignored).length;
    const ignored = d.files.length - videoFiles;

    const actions = `<section class="panel panel-pad td-actions" aria-label="Actions">
      ${st === 'paused' ? `<button class="btn btn-primary" data-act="act-resume" data-hash="${esc(tr.hash)}" data-detail="1">${icon('play', { size: 17 })}Resume</button>`
        : active ? `<button class="btn btn-secondary" data-act="act-pause" data-hash="${esc(tr.hash)}" data-detail="1">${icon('pause', { size: 17 })}Pause</button>` : ''}
      ${t.type === 'tv' ? `<a class="btn btn-secondary" href="#/mapping">${icon('listChecks', { size: 17 })}Re-run mapping</a>` : ''}
      <button class="btn btn-secondary" data-act="copy-magnet">${icon('magnet', { size: 17 })}Copy magnet</button>
      <button class="btn btn-ghost btn-danger-quiet td-remove" data-act="act-remove" data-hash="${esc(tr.hash)}" data-detail="1">${icon('trash', { size: 17 })}Remove</button>
    </section>`;

    const readers = d.readers.length
      ? d.readers.map((r) => `<li class="td-reader"><span class="ic">${icon('tv', { size: 18 })}</span><div class="grow"><strong>${esc(r.who)} · ${esc(r.file)} · reading pieces ${fmt(r.from)}–${fmt(r.to)} · ${esc(r.speed)}</strong><span class="t-small c-3">${esc(r.dev)} · at ${esc(r.pos)} · next 80 pieces prioritised</span></div></li>`).join('')
      : '<li class="t-small c-3">No one is reading this torrent right now. Readers appear when Infuse opens a file.</li>';

    const html = `${switcher}<div class="page td-page">
      <a class="btn btn-ghost btn-sm td-back" href="#/activity">${icon('chevronLeft', { size: 18 })}Activity</a>
      <header class="td-head">
        <a href="${titleHref}" class="td-poster" aria-label="${esc(t.title)}">${ui.poster(t.id)}</a>
        <div class="stack gap-3" style="min-width:0">
          <div class="row gap-2 wrap">${stateFor(tr)}${tr.health === 'weak' ? ui.pill('warning', 'Few peers') : ''}</div>
          <h1 class="t-title-1"><a href="${titleHref}" class="td-title-link">${esc(tr.label)}</a></h1>
          ${ui.releaseName(tr.name, 'p', 'td-release')}
          ${tr.pct < 100 ? `<div class="td-prog">${ui.progress(tr.pct, st === 'paused' ? '' : st === 'streaming' ? 'accent' : '', 'progress--lg')}<span class="t-small c-2 t-num">${tr.pct}% · ${d.got} of ${tr.size}</span></div>` : ''}
          <dl class="td-stats">${stats.map(([ic, k, v]) => `<div><dt>${icon(ic, { size: 14 })}${k}</dt><dd class="t-num">${esc(v)}</dd></div>`).join('')}</dl>
        </div>
      </header>

      <div class="td-grid">
        <aside class="td-aside">
          <section class="panel panel-pad td-health td-health--${hKind}" aria-labelledby="td-health-h">
            <div class="row gap-2"><span class="td-health-ic">${icon(hIcon, { size: 18 })}</span><h2 class="t-title-3" id="td-health-h">${esc(hTitle)}</h2></div>
            <p class="c-2">${esc(hWords)}</p>
          </section>
          ${tr.streaming && st === 'streaming' ? `<section class="banner banner--accent">${icon('tv', { size: 20 })}<div class="grow"><strong>${esc(tr.streaming.file)}</strong><span>${esc(tr.streaming.who)} · ${esc(tr.streaming.at)} · ${esc(tr.streaming.buffer)}</span></div></section>` : ''}
          ${actions}
        </aside>

        <div class="td-main">
          <section class="panel td-files" aria-labelledby="td-files-h">
            <div class="panel-head td-files-head"><div class="stack" style="min-width:0"><h2 class="t-title-3" id="td-files-h">Files <span class="c-3 t-num w-medium">${d.files.length}</span></h2>
              <span class="t-small c-3">${videoFiles} mapped${ignored ? ` · ${ignored} ignored` : ''}${d.root ? ` · in ${ui.path(d.root)}` : ''}</span></div></div>
            <div class="td-cols desk-only" aria-hidden="true"><span>File</span><span>Plays as</span><span>Size</span><span>Progress</span></div>
            <ul class="td-filelist">${d.files.map(fileRow).join('')}</ul>
            ${d.files.length > 14 ? `<div class="td-more"><button class="btn btn-ghost btn-sm" data-act="act-files-more">Show all ${d.files.length} files</button></div>` : ''}
          </section>

          <details class="panel td-adv" id="td-adv" ${p.advanced === '1' ? 'open' : ''}>
            <summary><span class="stack"><span class="t-title-3">Advanced</span><span class="t-small c-3">Piece map, readers, trackers, info hash</span></span>${icon('chevronDown', { size: 20, cls: 'chev' })}</summary>
            <div class="td-adv-body">
              <section class="stack gap-3" aria-labelledby="pm-h">
                <div class="row between wrap gap-2"><h3 class="w-semibold" id="pm-h">Piece map</h3><span class="t-small c-3 t-num">1 square = 1 piece (${d.pieceMB} MB) · ${fmt(havePieces)} of ${fmt(totalPieces)}</span></div>
                <div class="pm-wrap"><canvas class="pm" role="img" aria-label="Piece map: ${fmt(havePieces)} of ${fmt(totalPieces)} pieces downloaded${d.readers.length ? `; pieces ${fmt(d.readers[0].to + 1)} to ${fmt(d.readers[0].to + 80)} prioritised for streaming` : ''}"></canvas><span class="pm-tip" hidden></span></div>
                <ul class="pm-legend">
                  <li><i class="k-have"></i>Have</li><li><i class="k-dl"></i>Downloading</li><li><i class="k-prio"></i>Prioritised for streaming</li><li><i class="k-miss"></i>Missing</li>
                </ul>
              </section>
              <section class="stack gap-2" aria-labelledby="rd-h"><h3 class="w-semibold" id="rd-h">Active readers</h3><ul class="stack gap-2">${readers}</ul></section>
              <section class="stack gap-2" aria-labelledby="ih-h"><h3 class="w-semibold" id="ih-h">Info hash</h3>
                <div class="copy-field"><code class="path">${d.infohash}</code><button class="btn btn-secondary btn-sm" data-act="copy" data-copy="${d.infohash}">${icon('copy', { size: 15 })}Copy</button></div></section>
              <section class="stack gap-2" aria-labelledby="tk-h"><h3 class="w-semibold" id="tk-h">Trackers <span class="c-3 t-num w-medium">${d.trackers.length}</span></h3>
                <ul class="td-trackers">${d.trackers.map(([u, s, n]) => `<li><span class="path">${esc(u)}</span><span class="row gap-2 wrap">${s === 'ok' ? ui.pill('ready', 'Working', { icon: 'check' }) : s === 'error' ? ui.pill('error', 'Error') : s === 'warn' ? ui.pill('warning', 'No peers') : ui.pill('idle', 'Idle')}<span class="t-small c-3">${esc(n)}</span></span></li>`).join('')}</ul></section>
              <dl class="td-nerd">
                <div><dt>Ratio</dt><dd class="t-num">${d.ratio.toFixed(2)} <span class="c-3">(${esc(d.uploaded)} up)</span></dd></div>
                <div><dt>Piece size</dt><dd class="t-num">${d.pieceMB} MB</dd></div>
                <div><dt>Added</dt><dd>${esc(d.added)}</dd></div>
                <div><dt>Connected</dt><dd class="t-num">${active ? `${tr.peers} peers · ${tr.seeds} in swarm` : 'None'}</dd></div>
              </dl>
            </div>
          </details>
        </div>
      </div>
    </div>`;

    return {
      title: tr.label, tab: 'activity', html,
      mount(root) {
        const adv = root.querySelector('#td-adv');
        const canvas = root.querySelector('canvas.pm');
        const draw = () => drawPieces(canvas, tr, d, totalPieces);
        if (adv.open) requestAnimationFrame(draw);
        adv.addEventListener('toggle', () => { if (adv.open) draw(); });
        if (window.ResizeObserver) { const ro = new ResizeObserver(() => { if (adv.open) draw(); }); ro.observe(canvas.parentElement); }
        pieceHover(canvas, root.querySelector('.pm-tip'), tr, d, totalPieces);
      },
    };
  });

  // ---- Piece map: one square per piece, drawn on canvas -----------------------
  function pieceStates(tr, d, total) {
    const out = new Uint8Array(total); // 0 missing · 1 have · 2 downloading · 3 prioritised
    const gbTotal = d.files.reduce((a, f) => a + f.gb, 0);
    let start = 0; let r = 11;
    const rand = () => { r = (r * 16807) % 2147483647; return r / 2147483647; };
    d.files.forEach((f, fi) => {
      const n = fi === d.files.length - 1 ? total - start : Math.round((f.gb / gbTotal) * total);
      const pct = f.ignored ? (f.pct || 0) : f.pct;
      const rd = f.reading && d.readers[0];
      for (let i = 0; i < n && start + i < total; i++) {
        if (rd) { out[start + i] = start + i < rd.from ? 1 : 0; continue; }
        const frac = i / Math.max(1, n);
        // mostly sequential with a ragged edge, like a real swarm
        const have = pct >= 100 || frac < pct / 100 - 0.08 || (frac < pct / 100 + 0.06 && rand() < 0.55);
        out[start + i] = have ? 1 : 0;
      }
      start += n;
    });
    const active = tr.state === 'downloading' || (tr.streaming && tr.state !== 'paused');
    if (active) {
      let k = 0;
      for (let i = 0; i < total && k < 26; i++) if (out[i] === 0 && rand() < 0.04) { out[i] = 2; k++; }
    }
    (d.readers || []).forEach((rd) => {
      for (let i = rd.from; i <= rd.to && i < total; i++) if (out[i] !== 1) out[i] = 2;
      for (let i = rd.to + 1; i <= rd.to + 80 && i < total; i++) out[i] = out[i] === 1 ? 1 : 3;
    });
    return out;
  }
  const pmCache = new WeakMap();
  function drawPieces(canvas, tr, d, total) {
    if (!canvas) return;
    const wrap = canvas.parentElement;
    const w = wrap.clientWidth; if (!w) return;
    const cell = w < 520 ? 5 : 6, gap = 1, step = cell + gap;
    const cols = Math.floor((w + gap) / step);
    const rows = Math.ceil(total / cols);
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(cols * step * dpr); canvas.height = Math.round(rows * step * dpr);
    canvas.style.width = `${cols * step}px`; canvas.style.height = `${rows * step}px`;
    const ctx = canvas.getContext('2d'); ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    const cs = getComputedStyle(document.documentElement);
    const col = [cs.getPropertyValue('--surface-3').trim(), cs.getPropertyValue('--st-ready').trim(), cs.getPropertyValue('--st-downloading').trim(), cs.getPropertyValue('--accent').trim()];
    const states = pieceStates(tr, d, total);
    for (let i = 0; i < total; i++) {
      ctx.fillStyle = col[states[i]];
      ctx.fillRect((i % cols) * step, Math.floor(i / cols) * step, cell, cell);
    }
    pmCache.set(canvas, { cols, step, states });
  }
  function pieceHover(canvas, tip, tr, d, total) {
    if (!canvas || !tip) return;
    const gbTotal = d.files.reduce((a, f) => a + f.gb, 0);
    const fileOf = (i) => { let acc = 0; for (const f of d.files) { acc += Math.round((f.gb / gbTotal) * total); if (i < acc) return f; } return d.files[d.files.length - 1]; };
    const label = ['missing', 'have', 'downloading', 'prioritised for streaming'];
    const show = (e) => {
      const c = pmCache.get(canvas); if (!c) return;
      const r = canvas.getBoundingClientRect();
      const x = e.clientX - r.left, y = e.clientY - r.top;
      const i = Math.floor(y / c.step) * c.cols + Math.floor(x / c.step);
      if (i < 0 || i >= total) { tip.hidden = true; return; }
      const f = fileOf(i);
      tip.hidden = false;
      tip.textContent = `Piece ${fmt(i)} · ${f.ignored ? f.ignored : f.map} · ${label[c.states[i]]}`;
      tip.style.left = `${Math.max(0, Math.min(r.width - 220, x - 110))}px`;
      tip.style.top = `${Math.max(0, y - 40)}px`;
    };
    canvas.addEventListener('pointermove', show);
    canvas.addEventListener('pointerdown', show);
    canvas.addEventListener('pointerleave', () => { tip.hidden = true; });
  }

  // ---- Actions ---------------------------------------------------------------------
  function refresh(b) {
    if (b && b.dataset.detail) { app.go(location.hash, { noTransition: true }); return; }
    const g = document.getElementById('act-groups');
    if (g) g.innerHTML = groups(list());
    const sum = document.querySelector('.act-summary');
    if (sum) {
      const fresh = document.createElement('div'); fresh.innerHTML = summary(list());
      const t = sum.querySelectorAll('.act-tile')[2], nt = fresh.querySelectorAll('.act-tile')[2];
      if (t && nt) t.replaceWith(nt);
      const now = sum.querySelector('.act-now'), nn = fresh.querySelector('.act-now');
      if (now && nn) now.replaceWith(nn);
    }
  }
  Object.assign(app.actions, {
    'act-pause': (b) => {
      const tr = byHash(b.dataset.hash); if (!tr) return;
      ov[tr.hash] = { state: 'paused', streaming: false, _was: { state: tr.state, streaming: tr.streaming } };
      refresh(b);
      app.toast(null, tr.streaming ? { kind: 'info', title: `Paused ${tr.label}`, body: 'Infuse will stall once its buffer runs out.', icon: 'pause' } : { kind: 'info', title: `Paused ${tr.label}`, icon: 'pause' });
    },
    'act-resume': (b) => {
      const tr = byHash(b.dataset.hash); if (!tr) return;
      const was = ov[tr.hash] && ov[tr.hash]._was;
      ov[tr.hash] = was ? { state: was.state, streaming: was.streaming } : { state: 'downloading', down: '6.8 MB/s', up: '310 KB/s', peers: 14, seeds: 88, eta: '38 min' };
      refresh(b);
      app.toast(null, { kind: 'ready', title: `Resumed ${tr.label}`, icon: 'play' });
    },
    'act-pause-all': () => {
      list().filter((t) => t.state === 'downloading' || t.streaming).forEach((t) => { ov[t.hash] = { state: 'paused', streaming: false, _was: { state: t.state, streaming: t.streaming } }; });
      refresh();
      app.toast(null, { kind: 'info', title: 'Paused 3 torrents', body: 'Infuse keeps playing anything already downloaded.', icon: 'pause' });
    },
    'act-remove': (b) => {
      const tr = byHash(b.dataset.hash); if (!tr) return;
      pendingRemove = { hash: tr.hash, detail: !!b.dataset.detail };
      const t = T(tr.title);
      app.openDialog('remove-torrent', {
        title: 'Remove this torrent?',
        body: `Infuse will no longer be able to play ${esc(tr.label)}. ${t.type === 'tv' ? 'The show stays in your library and those episodes show “Needs a torrent”.' : 'The movie stays in your library and shows “Needs a torrent”.'}`,
        extra: `<label class="row gap-3" style="min-height:44px"><input type="checkbox" checked style="width:20px;height:20px;accent-color:var(--accent)"> <span class="t-small c-2">Also delete downloaded data (${esc(tr.size)})</span></label>`,
        confirm: 'Remove torrent', danger: true, act: 'act-removed',
      });
    },
    'act-removed': () => {
      app.close();
      if (!pendingRemove) return;
      const { hash, detail } = pendingRemove; const tr = byHash(hash);
      removed[hash] = true; pendingRemove = null;
      if (detail) app.go('#/activity'); else refresh();
      setTimeout(() => app.toast(null, { kind: 'info', title: `Removed ${tr ? tr.label : 'torrent'}`, body: 'Undo within 10 seconds', action: 'Undo', act: 'act-undo-remove', icon: 'trash' }), detail ? 350 : 0);
      app._actLastRemoved = hash;
    },
    'act-undo-remove': (b) => {
      delete removed[app._actLastRemoved];
      b.closest('.toast')?.remove();
      refresh();
    },
    'act-retry': (b) => {
      b.classList.add('is-loading');
      setTimeout(() => { b.classList.remove('is-loading'); const l = document.querySelector('[data-retry-label]'); if (l) l.textContent = 'Still not answering. Retrying in 30 s'; }, 1200);
    },
    'act-files-more': (b) => {
      b.closest('.td-files').querySelectorAll('[data-more]').forEach((li) => { li.hidden = false; });
      b.parentElement.remove();
    },
  });
})();
