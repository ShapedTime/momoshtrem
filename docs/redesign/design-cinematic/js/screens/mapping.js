/* Episode mapping review. Route /mapping (no tab).
   States (?state=): review (default) · progress · recheck · resolved · applied · onepiece · doctorwho
   Flow 1: #/mapping?flow=1 → progress → review → resolve the 2 "Needs review" rows → Apply
           → app.state.frieren = 'ready' → #/home?toast=ready
   Data shape mirrors GET /assignment-jobs/{id} (PLAN.md §6): groups of proposals
   {id, file, size, target:{s,e,name}|null, conf, source, reason}. */
(function () {
  const { ui, app, icon, brand, data: D } = MS;
  const esc = ui.esc;
  const cls = (...a) => a.filter(Boolean).join(' ');
  const pad = (n) => String(n).padStart(2, '0');
  const SE = (t) => (t ? `S${pad(t.s)}E${pad(t.e)}` : '');
  const tkey = (t) => `${t.s}-${t.e}`;
  const isDesk = () => window.matchMedia('(min-width: 1024px)').matches;
  const range = (n, f) => Array.from({ length: n }, (_, i) => f(i + 1));

  // ---- Episode catalogs (what the picker offers; `lib` = already has a file) ----
  const frierenSpecials = ['“Frieren” Mini Anime 1', '“Frieren” Mini Anime 2', 'Recap: The Journey So Far', '“Frieren” Mini Anime 3', '“Frieren” Mini Anime 4', '“Frieren” Mini Anime 5'];
  const opSpecials = { 33: 'Episode of Sabo', 34: 'Adventure of Nebulandia', 35: 'Heart of Gold', 36: 'Episode of East Blue', 37: 'Episode of Sky Island', 38: 'Cry Heart', 39: 'Episode of Arabasta (Remastered)', 40: 'Recap: The Road to Wano', 41: 'One Piece Fan Letter', 42: 'Recap: Gear Five' };
  const opTitles = { 1071: 'Luffy\'s Peak! Attained! Gear Five', 1072: 'The Ridiculous Power! Gear Five in Full Play', 1073: 'Don\'t Die! Kaido\'s Fury', 1074: 'The Beginning of a New Era', 1075: 'Bonds Beyond Generations', 1076: 'The Battle Ends' };
  const dwSpecials = ['Born Again', 'Attack of the Graske', 'The Christmas Invasion', 'Tardisodes', 'The Infinite Quest (Part 1)', 'The Infinite Quest (Part 2)', 'The Runaway Bride', 'Music of the Spheres', 'Time Crash', 'Voyage of the Damned', 'The Next Doctor', 'Planet of the Dead', 'The Waters of Mars', 'The End of Time (Part One)', 'The End of Time (Part Two)'];
  const dwS4 = ['Partners in Crime', 'The Fires of Pompeii', 'Planet of the Ood', 'The Sontaran Stratagem', 'The Poison Sky', 'The Doctor\'s Daughter', 'The Unicorn and the Wasp', 'Silence in the Library', 'Forest of the Dead', 'Midnight', 'Turn Left', 'The Stolen Earth', 'Journey\'s End'];

  // ---- Extra fixtures: One Piece (absolute numbering) and Doctor Who (specials) ----
  const op = (abs, e, extra = {}) => ({ id: `op${abs}`, file: `One.Piece.${abs}.1080p.WEB.x264-GROUP.mkv`, size: `${(1.18 + (abs % 4) * 0.03).toFixed(2)} GB`, target: { s: 21, e, name: opTitles[abs] }, conf: 'high', source: 'rules', reason: `Absolute #${abs} → S21E${pad(e)} (same offset as #1071)`, ...extra });
  const dw = (id, file, size, e, conf, source, reason) => ({ id, file, size, target: { s: 0, e, name: dwSpecials[e - 1] }, conf, source, reason });

  const JOBS = {
    frieren: {
      show: 'frieren', torrent: D.mapping.torrent, facts: ['13.2 GB', '16 files', 'Nyaa', 'SubsPlease'], back: '#/title/tv/frieren', backLabel: 'Frieren',
      groups: D.mapping.groups,
      catalog: [
        { n: 0, label: 'Specials · Season 0', short: 'Specials', eps: frierenSpecials.map((name, i) => ({ e: i + 1, name, lib: i < 2 })) },
        { n: 1, label: 'Season 1', short: 'Season 1', eps: D.frierenEps.map((ep) => ({ e: ep.n, name: ep.name, lib: ep.n <= 28 || ep.n === 35 })) },
      ],
      steps: ['Frieren · 2 seasons, 44 episodes', '16 files · 13 videos', '3 files'],
      hints: ['these are absolute numbers', '28.5 is a recap', 'v2 replaces v1'],
      recheck: {
        nr1: { conf: 'high', reason: 'AI: v2 fixes the subtitle timing in #35. Use it instead of the v1 file already in your library.' },
        nr2: { conf: 'high', reason: 'AI: 28.5 is the recap that aired Mar 15 between #28 and #29 → S00E03' },
        un1: { reason: 'AI: looks like a Blu-ray bonus that TMDB doesn\'t list. Ignoring it is safe.' },
      },
    },
    onepiece: {
      show: 'onepiece', torrent: 'One.Piece.1071-1076.+.Fan.Letter.1080p.WEB.x264-GROUP', facts: ['8.1 GB', '8 files', 'TorrentLeech'], back: '#/title/tv/onepiece', backLabel: 'One Piece',
      groups: {
        matched: [op(1072, 80), op(1073, 81), op(1074, 82), op(1075, 83), op(1076, 84)],
        needs_review: [{ id: 'op1071', file: 'One.Piece.1071.1080p.WEB.x264-GROUP.mkv', size: '1.21 GB', target: { s: 21, e: 79, name: opTitles[1071] }, conf: 'medium', source: 'ai', reason: 'AI: absolute #1071 → S21E79 by air order. TMDB splits Wano differently from the broadcast arcs, so check this one.' }],
        specials: [{ id: 'opfl', file: 'One.Piece.Fan.Letter.1080p.WEB.x264-GROUP.mkv', size: '1.02 GB', target: { s: 0, e: 41, name: opSpecials[41] }, conf: 'high', source: 'ai', reason: 'AI: "Fan Letter" is the Oct 2024 TV special → S00E41 by title' }],
        extras: [{ id: 'opsm', file: 'Sample/One.Piece.1071.sample.mkv', size: '48 MB', kind: 'Sample', reason: 'Sample clip; ignored by default' }],
        unmatched: [],
      },
      catalog: [
        { n: 0, label: 'Specials · Season 0', short: 'Specials', eps: range(42, (e) => ({ e, name: opSpecials[e] || `TV Special ${e}`, lib: e <= 39 })) },
        { n: 21, label: 'Season 21 · Wano Country', short: 'S21 Wano', eps: range(94, (e) => ({ e, abs: 992 + e, name: opTitles[992 + e] || `Episode ${992 + e}`, lib: 992 + e <= 1070 })) },
        { n: 22, label: 'Season 22 · Egghead', short: 'S22 Egghead', eps: range(66, (e) => ({ e, abs: 1086 + e, name: `Episode ${1086 + e}`, lib: false })) },
      ],
      steps: ['One Piece · 3 seasons, 1,124 episodes', '8 files · 7 videos', '2 files'],
      hints: ['these are absolute numbers', 'use TMDB air order', 'Fan Letter is a special'],
      recheck: { op1071: { conf: 'high', reason: 'AI: absolute #1071 aired Aug 6 2023 → S21E79 by air date; the title matches TMDB too' } },
    },
    doctorwho: {
      show: 'doctorwho', torrent: 'Doctor.Who.2005.Specials.2005-2008.720p.BluRay.x264-SHORTBREHD', facts: ['9.6 GB', '7 files', 'TorrentLeech'], back: '#/title/tv/doctorwho', backLabel: 'Doctor Who',
      groups: {
        matched: [],
        needs_review: [dw('dw05', 'Doctor.Who.2005.S00E05.The.Runaway.Bride.720p.mkv', '1.42 GB', 7, 'medium', 'ai', 'AI: the file says S00E05, but on TMDB "The Runaway Bride" is S00E07; matched by title, not number')],
        specials: [
          dw('dw04', 'Doctor.Who.2005.S00E04.The.Christmas.Invasion.720p.mkv', '1.71 GB', 3, 'high', 'rules', 'Title "The Christmas Invasion" → S00E03 (numbers differ from the file)'),
          dw('dw06', 'Doctor.Who.2005.S00E06.Voyage.of.the.Damned.720p.mkv', '1.88 GB', 10, 'high', 'rules', 'Title "Voyage of the Damned" → S00E10'),
          dw('dw07', 'Doctor.Who.2005.S00E07.Time.Crash.720p.mkv', '210 MB', 9, 'high', 'rules', 'Title "Time Crash" → S00E09'),
          dw('dw08', 'Doctor.Who.2005.S00E08.The.Next.Doctor.720p.mkv', '1.69 GB', 11, 'high', 'rules', 'Title "The Next Doctor" → S00E11'),
        ],
        extras: [{ id: 'dwnfo', file: 'Doctor.Who.2005.Specials.720p.BluRay.x264-SHORTBREHD.nfo', size: '6 KB', kind: 'Info file', reason: 'Not a video' }],
        unmatched: [{ id: 'dwconf', file: 'Doctor.Who.Confidential.Christmas.Special.2007.720p.mkv', size: '640 MB', target: null, conf: 'none', source: 'rules', reason: 'Doctor Who Confidential is a separate show on TMDB. Pick an episode or ignore it.' }],
      },
      catalog: [
        { n: 0, label: 'Specials · Season 0', short: 'Specials', eps: dwSpecials.map((name, i) => ({ e: i + 1, name, lib: i < 2 })) },
        { n: 4, label: 'Season 4', short: 'Season 4', eps: dwS4.map((name, i) => ({ e: i + 1, name, lib: true })) },
      ],
      steps: ['Doctor Who · 5 seasons incl. 160 specials', '7 files · 6 videos', '1 file'],
      hints: ['match specials by title', 'Confidential is a different show'],
      recheck: { dw05: { conf: 'high', reason: 'AI: TMDB lists "The Runaway Bride" (Dec 25 2006) as S00E07; title and air date agree' } },
    },
  };

  // Group order is by urgency, not by the API's order: decisions first, the long tail last.
  const GROUPS = [
    ['needs_review', 'Needs review', 'review', 'Noir wasn\'t sure about these. Accept the suggestion or pick another episode.'],
    ['unmatched', 'Unmatched', 'warning', 'No episode found. Pick one, or ignore the file. Unmapped files stay in the torrent but not in Infuse.'],
    ['specials', 'Specials', 'upcoming', 'Mapped to Season 0. Infuse lists them under Specials.'],
    ['matched', 'Matched', 'ready', 'Confident matches. Open to spot-check.'],
    ['extras', 'Extras & ignored', 'missing', 'Openings, endings, samples and non-video files stay out of Infuse.'],
  ];

  // ---- Working state ---------------------------------------------------------
  let work = null;

  function makeWork(jobKey) {
    const job = JOBS[jobKey] || JOBS.frieren;
    const items = [];
    Object.entries(job.groups).forEach(([g, list]) => list.forEach((it) => items.push({
      ...it, group: g, target: it.target ? { ...it.target } : null,
      decision: g === 'needs_review' || g === 'unmatched' ? 'pending' : g === 'extras' ? 'ignored' : 'accepted',
    })));
    const firstPending = items.find((i) => i.decision === 'pending') || items[0];
    const desk = isDesk();
    return { jobKey, job, items, sel: firstPending.id, open: { matched: desk, extras: desk }, q: '', freeOnly: false, ai: { open: false, hint: '', scope: 'unresolved', phase: 'idle' }, phase: 'review', step: 0, flash: null };
  }
  const byId = (id) => work.items.find((i) => i.id === id);
  const cur = () => (work ? byId(work.sel) : null);
  const isOpen = (g) => work.open[g] !== false;

  function counts() {
    const c = { matched: 0, review: 0, special: 0, ignored: 0, unmatched: 0, apply: 0 };
    work.items.forEach((i) => {
      if (i.decision === 'ignored') c.ignored++;
      else if (!i.target) c.unmatched++;
      else { c.apply++; if (i.decision === 'pending') c.review++; else if (i.target.s === 0) c.special++; else c.matched++; }
    });
    return c;
  }
  function usedMap() {
    const m = {};
    work.items.forEach((i) => { if (i.decision !== 'ignored' && i.target) (m[tkey(i.target)] = m[tkey(i.target)] || []).push(i); });
    return m;
  }
  function conflicts() {
    const out = {};
    Object.values(usedMap()).forEach((arr) => { if (arr.length > 1) arr.forEach((i) => { out[i.id] = arr.filter((o) => o !== i); }); });
    return out;
  }
  // "the #30 file" style short label for a file
  function shortName(f) {
    const base = f.split('/').pop();
    const m = base.match(/ - (.+?) \(/) || base.match(/\.(\d{3,4})\./) || base.match(/(S\d\dE\d\d)/);
    if (!m) return base.replace(/\.mkv$/, '');
    return /^[\d.v]+$/.test(m[1]) ? `#${m[1]}` : m[1];
  }
  function ranges(nums) {
    const s = [...nums].sort((a, b) => a - b); const out = [];
    for (let i = 0; i < s.length; i++) {
      let j = i; while (j + 1 < s.length && s[j + 1] === s[j] + 1) j++;
      out.push(j > i + 1 ? `E${pad(s[i])}–E${pad(s[j])}` : j === i + 1 ? `E${pad(s[i])}, E${pad(s[j])}` : `E${pad(s[i])}`);
      i = j;
    }
    return out.join(', ');
  }
  const findEp = (s, e) => { const se = work.job.catalog.find((c) => c.n === s); return se && se.eps.find((x) => x.e === e); };

  // ---- Pieces ---------------------------------------------------------------
  function summaryHTML() {
    const c = counts();
    const parts = [
      [c.matched, 'matched', 'ready'],
      [c.review, c.review === 1 ? 'needs review' : c.review ? 'need review' : 'left to review', 'review', true],
      [c.special, c.special === 1 ? 'special' : 'specials', 'upcoming'],
      [c.ignored, 'ignored', 'missing'],
      [c.unmatched, 'unmatched', 'warning'],
    ].filter(([n, , , keep]) => n > 0 || keep);
    return parts.map(([n, l, k]) => `<span class="${cls('ms', `ms--${k}`, !n && 'is-zero')}"><i aria-hidden="true"></i><b class="t-num">${n}</b>&nbsp;${l}</span>`).join('');
  }

  function headHTML(withSummary = true) {
    const t = D.T[work.job.show];
    return `<header class="map-head page">
      <a class="btn btn-ghost btn-sm map-back" href="${work.job.back}">${icon('chevronLeft', { size: 18 })}${esc(work.job.backLabel)}</a>
      <div class="map-head-main">
        <div class="map-poster sm-up">${ui.poster(t.id)}</div>
        <div class="map-titles">
          <p class="map-eyebrow">${icon('listChecks', { size: 15 })}<span>Episode mapping · ${esc(t.title)}</span></p>
          <h1 class="map-h1">Check the episodes</h1>
          <div class="map-torrent">${icon('magnet', { size: 15 })}${ui.releaseName(work.job.torrent, 'p')}</div>
          <p class="map-facts dot-sep">${work.job.facts.map((f) => `<span>${esc(f)}</span>`).join('')}</p>
          ${withSummary ? `<div class="map-summary" data-map-summary aria-live="polite">${summaryHTML()}</div>` : ''}
        </div>
      </div>
    </header>`;
  }

  function reasonHTML(it) {
    if (it.source === 'you') return `<p class="mt-reason mt-reason--you">${icon('user', { size: 14 })}<span>You picked this episode</span></p>`;
    if (!it.reason) return '';
    if (it.source === 'ai') {
      const txt = it.reason.replace(/^AI:\s*/, '');
      return `<p class="mt-reason mt-reason--ai">${icon('sparkle', { size: 14 })}<span><b>AI${it.hinted ? ', with your hint' : ''}:</b> ${esc(txt)}</span></p>`;
    }
    return `<p class="mt-reason">${icon('listChecks', { size: 14 })}<span>${esc(it.reason)}</span></p>`;
  }

  function rowHTML(it, cf) {
    const pending = it.decision === 'pending', ignored = it.decision === 'ignored';
    const full = (it.group === 'needs_review' || it.group === 'unmatched') && !ignored;
    const others = cf[it.id];
    const id = `data-id="${it.id}"`;
    const selected = work.sel === it.id;

    let target;
    if (ignored) {
      target = `<div class="mt-ep mt-ep--ignored">${icon('eyeOff', { size: 15 })}<span>${it.group === 'extras' ? esc(it.kind || 'Extra') : 'Ignored'}</span><span class="c-3">· not in Infuse</span></div>`;
    } else if (it.target) {
      target = `<div class="mt-ep"><span class="mt-se t-num">${SE(it.target)}</span><span class="mt-name">${esc(it.target.name)}</span></div>`;
    } else {
      target = `<div class="mt-ep mt-ep--none">${icon('alertCircle', { size: 15 })}<span>No episode yet</span></div>`;
    }
    const meta = ignored ? '' : `<div class="mt-meta">${it.source === 'you' ? '' : ui.conf(it.conf)}${it.updated ? '<span class="chip chip--accent">Updated</span>' : ''}</div>`;
    const conflict = others
      ? `<p class="mt-conflict">${icon('alertTriangle', { size: 14 })}<span>Same episode as the <b>${esc(shortName(others[0].file))}</b> file. Keep one of them.</span></p>`
      : it.conflict && !ignored ? `<p class="mt-note">${icon('arrowRightLeft', { size: 14 })}<span>${esc(it.conflict)}</span></p>` : '';

    let actions;
    if (ignored) {
      actions = `<button class="btn btn-ghost btn-sm" data-act="map-unignore" ${id}>${icon(it.group === 'extras' ? 'plus' : 'refresh', { size: 16 })}${it.group === 'extras' ? 'Include' : 'Undo'}</button>`;
    } else if (full) {
      const accept = !it.target ? ''
        : pending ? `<button class="btn btn-secondary btn-sm mt-accept" data-act="map-accept" ${id}>${icon('check', { size: 16 })}Accept</button>`
        : `<button class="btn btn-ready btn-sm" data-act="map-unaccept" ${id} aria-pressed="true" aria-label="Accepted. Tap to undo">${icon('check', { size: 16 })}Accepted</button>`;
      actions = `<button class="btn ${it.target ? 'btn-ghost' : 'btn-secondary'} btn-sm" data-act="map-change" ${id}>${icon('arrowRightLeft', { size: 16 })}${it.target ? 'Change' : 'Pick episode'}</button>
        <button class="btn btn-ghost btn-sm" data-act="map-ignore" ${id}>${icon('eyeOff', { size: 16 })}Ignore</button>
        <span class="grow"></span>${accept}`;
    } else {
      actions = `<button class="btn btn-ghost btn-sm btn-icon" data-act="map-change" ${id} aria-label="Change episode for ${esc(shortName(it.file))}">${icon('arrowRightLeft', { size: 17 })}</button>
        <button class="btn btn-ghost btn-sm btn-icon" data-act="map-ignore" ${id} aria-label="Ignore ${esc(shortName(it.file))}">${icon('eyeOff', { size: 17 })}</button>`;
    }

    return `<li class="${cls('mrow', full ? 'mrow--full' : 'mrow--compact', it.group === 'needs_review' && pending && 'is-review', it.group === 'needs_review' && !pending && !ignored && 'is-resolved', it.group === 'unmatched' && pending && 'is-unmatched', ignored && 'is-ignored', others && 'is-conflict', selected && 'is-selected', work.flash === it.id && 'is-flash')}" data-row="${it.id}">
      <div class="mrow-file">
        <button class="mrow-name" data-act="map-select" ${id} aria-label="${esc(it.file)}: choose its episode">${ui.path(it.file)}</button>
        <span class="mrow-size t-num">${esc(it.size)}</span>
      </div>
      <span class="mrow-arrow" aria-hidden="true">${icon('corner', { size: 16, cls: 'a-stack' })}${icon('arrowRight', { size: 16, cls: 'a-wide' })}</span>
      <div class="mrow-target">${target}${meta}${reasonHTML(it)}${conflict}</div>
      <div class="mrow-actions">${actions}</div>
    </li>`;
  }

  function groupsHTML() {
    const cf = conflicts();
    return GROUPS.map(([g, title, tone, desc]) => {
      const list = work.items.filter((i) => i.group === g);
      if (!list.length) return '';
      const open = isOpen(g);
      const pendingN = list.filter((i) => i.decision === 'pending' && i.target).length;
      let peek = '';
      if (g === 'needs_review') peek = pendingN ? `<span class="mg-peek mg-peek--review">${pendingN} to decide</span>` : `<span class="mg-peek mg-peek--ok">${icon('check', { size: 14 })}All decided</span>`;
      else if (g === 'matched' && !open) {
        const by = {}; list.filter((i) => i.target && i.decision !== 'ignored').forEach((i) => (by[i.target.s] = by[i.target.s] || []).push(i.target.e));
        peek = `<span class="mg-peek">${Object.entries(by).map(([s, es]) => `S${pad(s)} ${ranges(es)}`).join(' · ')}</span>`;
      } else if (g === 'extras' && !open) peek = `<span class="mg-peek">${list.map((i) => esc(i.kind || 'Extra')).slice(0, 3).join(', ')}</span>`;
      return `<section class="${cls('mgroup', `mgroup--${tone}`, open && 'is-open')}" id="mg-${g}">
        <div class="mgroup-head">
          <button class="mgroup-toggle" data-act="map-toggle" data-g="${g}" aria-expanded="${open}" aria-controls="mg-${g}-list">
            <span class="mg-dot" aria-hidden="true"></span><span class="mg-title">${title}</span><span class="mg-n t-num">${list.length}</span>${peek}
            <span class="mg-chev" aria-hidden="true">${icon('chevronDown', { size: 18 })}</span>
          </button>
        </div>
        <div class="mgroup-body" id="mg-${g}-list" ${open ? '' : 'hidden'}>
          <div class="mgroup-desc"><p>${desc}</p>${g === 'needs_review' && pendingN > 1 ? `<button class="btn btn-ghost btn-sm" data-act="map-accept-all">${icon('checkCheck', { size: 16 })}Accept all ${pendingN}</button>` : ''}</div>
          <ul class="mlist">${list.map((i) => rowHTML(i, cf)).join('')}</ul>
        </div>
      </section>`;
    }).join('');
  }

  function aiHTML() {
    const a = work.ai;
    if (a.phase === 'working') {
      return `<section class="map-ai panel is-working" role="status" aria-live="polite">
        <div class="map-ai-top"><span class="map-ai-eyes">${brand.eyes(40)}</span>
          <div class="grow stack gap-1"><strong>Re-checking ${a.n} file${a.n === 1 ? '' : 's'}${a.hint ? ' with your hint' : ''}…</strong><span class="t-small c-3">${a.hint ? `“${esc(a.hint)}”` : 'Comparing names, air dates and runtimes'}</span></div></div>
        <div class="steps steps--inline"><div class="step is-done"><span class="ic">${icon('check', { size: 12, stroke: 2.5 })}</span>Read file names</div><div class="step is-active"><span class="ic"></span>Comparing with TMDB air dates</div><div class="step"><span class="ic"></span>Updating suggestions</div></div>
      </section>`;
    }
    const unresolved = work.items.filter((i) => i.decision === 'pending').length;
    const done = a.phase === 'done' ? `<p class="map-ai-done">${icon('checkCheck', { size: 16 })}<span>Updated ${a.changed} suggestion${a.changed === 1 ? '' : 's'}, marked <b>Updated</b> below. Accept the ones that look right.</span></p>` : '';
    return `<section class="${cls('map-ai panel', a.open && 'is-open')}">
      <div class="map-ai-top">
        ${brand.avatar(36)}
        <div class="grow stack"><strong>Not quite right?</strong><span class="t-small c-2">Noir can take another look. A hint helps with odd numbering.</span></div>
        <button class="btn btn-secondary btn-sm map-ai-btn" data-act="map-ai-toggle" aria-expanded="${a.open}" aria-controls="map-ai-body">${icon('sparkle', { size: 16 })}<span>Re-check<span class="sm-up"> with AI</span></span></button>
      </div>
      ${done}
      <div class="map-ai-body" id="map-ai-body" ${a.open ? '' : 'hidden'}>
        <label class="field"><span class="label">Hint for Noir <span class="c-3">(optional)</span></span>
          <input class="input" data-map-hint value="${esc(a.hint)}" placeholder="e.g. these are absolute numbers" autocomplete="off" enterkeyhint="go"></label>
        <div class="fchips" aria-label="Example hints">${work.job.hints.map((h) => `<button class="fchip" data-act="map-hint" data-h="${esc(h)}" aria-pressed="${a.hint === h}">${esc(h)}</button>`).join('')}</div>
        <div class="map-ai-run">
          ${ui.seg([['unresolved', `Unresolved · ${unresolved}`], ['all', `All files · ${work.items.length}`]], a.scope, 'data-map-scope aria-label="Which files to re-check"')}
          <button class="btn btn-secondary" data-act="map-ai-run">${icon('sparkle', { size: 17 })}Re-check</button>
        </div>
      </div>
    </section>`;
  }

  // ---- Episode picker (pane on desktop, bottom sheet on phone) ---------------
  function matches(ep, season, q) {
    if (!q) return true;
    const n = q.replace(/\s+/g, '');
    if (/^#?\d+$/.test(n)) { const v = Number(n.replace('#', '')); return ep.e === v || ep.abs === v || (ep.abs && String(ep.abs).startsWith(n.replace('#', ''))); }
    if (/^s\d+(e\d*)?$/.test(n)) { const se = `s${pad(season.n)}e${pad(ep.e)}`; const m = n.match(/^s(\d+)(?:e(\d*))?$/); return Number(m[1]) === season.n && (!m[2] || se.startsWith(`s${pad(season.n)}e${m[2].length === 1 ? m[2] : pad(Number(m[2]))}`)); }
    if (/^(special|sp)/.test(n) && season.n === 0) return true;
    return ep.name.toLowerCase().includes(q);
  }

  function pickerListHTML() {
    const it = cur(); const q = work.q.trim().toLowerCase(); const used = usedMap();
    let total = 0;
    const html = work.job.catalog.map((season) => {
      const eps = season.eps.filter((ep) => {
        const k = `${season.n}-${ep.e}`;
        const mine = it && it.target && tkey(it.target) === k;
        const taken = ep.lib || (used[k] || []).some((o) => o !== it);
        return matches(ep, season, q) && (!work.freeOnly || mine || !taken);
      });
      total += eps.length;
      if (!eps.length) return '';
      return `<div class="pk-season" data-season="${season.n}">
        <h3 class="pk-shead"><span>${esc(season.label)}</span><span class="t-num">${season.eps.length} ep</span></h3>
        <div class="pk-eps" role="listbox" aria-label="${esc(season.label)}">${eps.map((ep) => {
          const k = `${season.n}-${ep.e}`;
          const isCur = !!(it && it.target && tkey(it.target) === k);
          const others = (used[k] || []).filter((o) => o !== it);
          let tag = '';
          if (isCur) tag = `<span class="pk-tag pk-tag--cur">${icon('check', { size: 13, stroke: 2.4 })}Current</span>`;
          else if (others.length) tag = `<span class="pk-tag pk-tag--batch">Taken · ${esc(shortName(others[0].file))}</span>`;
          else if (ep.lib) tag = '<span class="pk-tag">In library</span>';
          return `<button class="${cls('pk-ep', isCur && 'is-current', (others.length || ep.lib) && !isCur && 'is-taken')}" role="option" aria-selected="${isCur}" data-act="map-pick" data-s="${season.n}" data-e="${ep.e}" ${it ? '' : 'disabled'}>
            <span class="no t-num">${season.n === 0 ? 'SP' : 'E'}${pad(ep.e)}</span>
            <span class="nm"><span class="t">${esc(ep.name)}</span>${ep.abs ? `<span class="abs t-num">Absolute #${ep.abs}</span>` : ''}</span>${tag}
          </button>`;
        }).join('')}</div>
      </div>`;
    }).join('');
    return total ? html : `<div class="pk-empty">${icon('search', { size: 20 })}<p>No episode matches “${esc(work.q)}”.</p><p class="t-small c-3">Try a number like 35, an absolute number, or S00E03.</p></div>`;
  }

  function pickerToolsHTML() {
    const it = cur();
    const curSeason = it && it.target ? it.target.s : work.job.catalog[work.job.catalog.length - 1].n;
    return `<div class="input-wrap pk-search">${icon('search', { size: 18 })}<input class="input" type="search" data-pk-q value="${esc(work.q)}" placeholder="Number, absolute # or title" aria-label="Search episodes" autocomplete="off" enterkeyhint="search"></div>
      <div class="pk-tools">${ui.seg(work.job.catalog.map((s) => [String(s.n), s.short]), String(curSeason), 'data-pk-seasons aria-label="Jump to season"')}
        ${ui.fchip('Hide taken', work.freeOnly, null, 'data-act="map-free"')}</div>`;
  }

  function pickerPaneHTML() {
    const it = cur();
    return `<div class="picker" data-picker>
      <div class="pk-head">
        <div class="pk-for"><span class="t-micro c-3">${it ? 'Episode for' : 'Episodes'}</span>
          ${it ? `<p class="pk-file">${ui.path(it.file)}</p>` : '<p class="t-small c-2">Pick a file on the left to change its episode.</p>'}</div>
        ${pickerToolsHTML()}
      </div>
      <div class="pk-list" data-pk-list>${pickerListHTML()}</div>
      <p class="pk-legend t-small c-3">${icon('info', { size: 14 })}Click an episode to assign it to the selected file.</p>
    </div>`;
  }

  function scrollToCurrent(list, smooth) {
    if (!list) return;
    const el = list.querySelector('.pk-ep.is-current');
    if (el) list.scrollTo({ top: Math.max(0, el.offsetTop - Math.min(160, list.clientHeight / 3)), behavior: smooth && !window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'smooth' : 'auto' });
  }
  function renderLists() {
    document.querySelectorAll('[data-picker]').forEach((pk) => {
      const list = pk.querySelector('[data-pk-list]'); if (list) list.innerHTML = pickerListHTML();
      const chip = pk.querySelector('[data-act="map-free"]'); if (chip) chip.setAttribute('aria-pressed', String(work.freeOnly));
    });
  }
  function bindPicker(scope) {
    scope.addEventListener('input', (e) => {
      if (e.target.matches('[data-pk-q]')) { work.q = e.target.value; renderLists(); }
    });
    scope.addEventListener('click', (e) => {
      const b = e.target.closest('[data-pk-seasons] button'); if (!b) return;
      const pk = b.closest('[data-picker]'); const list = pk && pk.querySelector('[data-pk-list]');
      b.parentElement.querySelectorAll('button').forEach((x) => x.setAttribute('aria-selected', String(x === b)));
      const h = list && list.querySelector(`[data-season="${b.dataset.v}"]`);
      if (h) list.scrollTo({ top: h.offsetTop, behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
    });
  }

  app.sheet('ep-picker', () => {
    const it = cur();
    return {
      cls: 'map-sheet',
      html: `<div data-picker class="pk-sheet">
        <div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head">
          <div class="sheet-title"><h2 class="t-title-3" id="ep-picker-title">Choose an episode</h2><button class="btn btn-ghost btn-icon" data-act="close" aria-label="Close">${icon('x')}</button></div>
          ${it ? `<div class="pk-for"><span class="t-micro c-3">For</span><p class="pk-file">${ui.path(it.file)}</p></div>` : ''}
          ${pickerToolsHTML()}
        </div>
        <div class="sheet-body pk-list" data-pk-list>${pickerListHTML()}</div>
        <div class="sheet-foot"><button class="btn btn-ghost" data-act="map-ignore" data-id="${it ? it.id : ''}">${icon('eyeOff', { size: 17 })}Ignore file</button><span class="grow"></span><button class="btn btn-secondary" data-act="close">${it && it.target ? 'Keep current' : 'Not now'}</button></div>
      </div>`,
      mount(el) {
        bindPicker(el);
        const list = el.querySelector('[data-pk-list]');
        requestAnimationFrame(() => scrollToCurrent(list));
        // Don't pop the keyboard over the list on phones: focus the current option instead.
        setTimeout(() => { const c = el.querySelector('.pk-ep.is-current') || el.querySelector('.pk-ep'); if (c) c.focus({ preventScroll: true }); }, 60);
      },
      onClose() { work.q = ''; },
    };
  });

  // ---- Footer -----------------------------------------------------------------
  function footHTML() {
    const c = counts(); const cf = Object.keys(conflicts()).length;
    const blocked = c.review > 0 || cf > 0;
    const msg = cf ? `${icon('alertTriangle', { size: 16 })}<span>Two files point at the same episode. Keep one.</span>`
      : c.review ? `${icon('alertCircle', { size: 16 })}<span>Decide on ${c.review} file${c.review === 1 ? '' : 's'} to apply</span>`
      : c.unmatched ? `${icon('info', { size: 16 })}<span>${c.unmatched} file stays unmapped. You can map it later.</span>`
      : `${icon('checkCheck', { size: 16 })}<span>Everything has a place</span>`;
    return `<div class="map-foot-in">
      <p class="${cls('map-foot-msg', cf ? 'is-conflict' : blocked ? 'is-blocked' : 'is-ok')}" id="map-foot-msg">${msg}</p>
      <div class="map-foot-actions">
        <a class="btn btn-ghost" href="${work.job.back}">Cancel</a>
        <button class="btn btn-primary map-apply" data-act="map-apply" aria-disabled="${blocked}" aria-describedby="map-foot-msg">${icon('check', { size: 18, stroke: 2.2 })}Apply ${c.apply} mapping${c.apply === 1 ? '' : 's'}</button>
      </div>
    </div>`;
  }

  // ---- Phases -------------------------------------------------------------------
  function progressHTML() {
    const labels = ['Fetching metadata', 'Matching files', 'Asking AI about the tricky ones'];
    const st = work.step;
    return `${headHTML(false)}
      <section class="map-progress page" aria-live="polite">
        ${ui.loader(st >= 3 ? 'Done. Here\'s the plan.' : 'Mapping files to episodes', 'Usually takes a few seconds')}
        <div class="steps map-steps" data-map-steps>${labels.map((l, i) => `<div class="${cls('step', i < st && 'is-done', i === st && 'is-active')}"><span class="ic">${i < st ? icon('check', { size: 14, stroke: 2.5 }) : ''}</span><span class="grow">${l}</span><span class="detail">${esc(work.job.steps[i])}</span></div>`).join('')}</div>
        <ul class="map-ghost" aria-hidden="true">${Array.from({ length: 4 }, (_, i) => `<li><div class="skel skel-line" style="width:${[72, 64, 80, 58][i]}%"></div><div class="skel skel-line" style="width:${[38, 44, 30, 40][i]}%"></div></li>`).join('')}</ul>
      </section>`;
  }

  function reviewHTML() {
    return `${headHTML(true)}
      <div class="map-layout page">
        <div class="map-main">
          <div data-map-ai>${aiHTML()}</div>
          <div class="map-groups" data-map-groups>${groupsHTML()}</div>
        </div>
        <aside class="map-pane desk-only" aria-label="Episodes" data-map-pane>${pickerPaneHTML()}</aside>
      </div>
      <div class="map-foot" data-map-foot>${footHTML()}</div>`;
  }

  function appliedHTML() {
    const t = D.T[work.job.show];
    const acc = work.items.filter((i) => i.decision !== 'ignored' && i.target);
    const by = {}; acc.forEach((i) => (by[i.target.s] = by[i.target.s] || []).push(i.target.e));
    const seasons = Object.keys(by).map(Number).sort((a, b) => (a === 0) - (b === 0) || a - b);
    const ign = work.items.filter((i) => i.decision === 'ignored').length;
    const unm = work.items.filter((i) => i.decision !== 'ignored' && !i.target).length;
    const left = [ign && `${ign} ignored`, unm && `${unm} unmapped`].filter(Boolean).join(' · ');
    return `<section class="map-done page" aria-labelledby="done-h">
      <div class="done-art" aria-hidden="true"><span class="done-glow"></span>${ui.cat('sit', 112)}<span class="done-check">${icon('check', { size: 18, stroke: 2.6 })}</span></div>
      <p class="t-small c-3 w-medium">${esc(t.title)}</p>
      <h1 class="t-display" id="done-h">Ready in Infuse.</h1>
      <p class="t-body-l c-2 done-lede">${acc.length} files mapped. They stream while the rest downloads, and show up after Infuse's next library refresh.</p>
      <ul class="panel done-list">
        ${seasons.map((s) => `<li class="list-row"><span class="done-ic">${icon('play', { size: 15, fill: true })}</span><div class="grow"><strong>${s === 0 ? 'Specials' : `Season ${s}`}</strong><span class="t-small c-3 t-num">${ranges(by[s])}</span></div>${ui.pill('ready', `${by[s].length} ready`)}</li>`).join('')}
        ${left ? `<li class="list-row"><span class="done-ic done-ic--muted">${icon('eyeOff', { size: 15 })}</span><div class="grow"><strong class="c-2">Left out</strong><span class="t-small c-3">${left} · still in the torrent, hidden from Infuse</span></div></li>` : ''}
      </ul>
      <div class="done-actions">
        <button class="btn btn-primary btn-lg" data-act="howto">${icon('tv', { size: 20 })}How to play in Infuse</button>
        <a class="btn btn-secondary btn-lg" href="${work.job.back}">Back to ${esc(work.job.backLabel)}</a>
      </div>
      <button class="btn btn-ghost btn-sm" data-act="map-edit">${icon('arrowRightLeft', { size: 15 })}Edit mapping</button>
    </section>`;
  }

  function pageHTML() {
    const body = work.phase === 'progress' ? progressHTML() : work.phase === 'applied' ? appliedHTML() : reviewHTML();
    return `${work.switcher}${body}`;
  }

  // ---- Painting -----------------------------------------------------------------
  const root = () => document.getElementById('map-root');
  function paintAll() {
    const r = root(); if (!r) return;
    r.innerHTML = pageHTML();
    if (work.phase === 'review') requestAnimationFrame(() => scrollToCurrent(r.querySelector('[data-map-pane] [data-pk-list]')));
  }
  function paint(opts = {}) {
    const r = root(); if (!r || work.phase !== 'review') return;
    const set = (sel, html) => { const el = r.querySelector(sel); if (el) el.innerHTML = html; };
    set('[data-map-summary]', summaryHTML());
    set('[data-map-groups]', groupsHTML());
    set('[data-map-foot]', footHTML());
    if (opts.ai) set('[data-map-ai]', aiHTML());
    const pane = r.querySelector('[data-map-pane]');
    if (pane) {
      const list = pane.querySelector('[data-pk-list]'); const top = list ? list.scrollTop : 0;
      const focused = document.activeElement && document.activeElement.matches('[data-pk-q]');
      pane.innerHTML = pickerPaneHTML();
      const nl = pane.querySelector('[data-pk-list]');
      if (opts.scrollPicker) requestAnimationFrame(() => scrollToCurrent(nl, true)); else if (nl) nl.scrollTop = top;
      if (focused) { const q = pane.querySelector('[data-pk-q]'); q.focus(); q.setSelectionRange(q.value.length, q.value.length); }
    }
    if (work.flash) { const id = work.flash; setTimeout(() => { if (work && work.flash === id) { work.flash = null; const el = r.querySelector(`[data-row="${id}"]`); if (el) el.classList.remove('is-flash'); } }, 1200); }
  }
  function nextPending(afterId) {
    const list = work.items.filter((i) => i.decision === 'pending' && i.target);
    return list.find((i) => i.id !== afterId);
  }

  // ---- Route ----------------------------------------------------------------------
  app.route('/mapping', (p) => {
    const state = p.state || (p.flow === '1' ? 'progress' : 'review');
    const jobKey = p.job || (state === 'onepiece' || state === 'doctorwho' ? state : 'frieren');
    work = makeWork(jobKey);
    work.flow = p.flow === '1';
    work.switcher = ui.protoStates('#/mapping', [['review', 'Review'], ['progress', 'Working'], ['recheck', 'Re-checking with AI'], ['resolved', 'Resolved'], ['applied', 'Applied'], ['onepiece', 'One Piece (absolute)'], ['doctorwho', 'Doctor Who (specials)']], state);
    const acceptPending = () => work.items.forEach((i) => { if (i.decision === 'pending' && i.target) i.decision = 'accepted'; });
    if (state === 'progress') { work.phase = 'progress'; work.step = 0; }
    if (state === 'resolved') { acceptPending(); work.sel = 'nr2'; }
    if (state === 'applied') { acceptPending(); work.phase = 'applied'; }
    if (state === 'recheck') work.ai = { open: true, hint: '28.5 is a recap', scope: 'unresolved', phase: 'working', n: 3, hold: true };

    return {
      title: 'Episode mapping', noTabbar: true, mainCls: 'main--map',
      html: `<div class="map-page" id="map-root">${pageHTML()}</div>`,
      mount(main) {
        const W = work;
        const alive = () => work === W && root();
        if (W.flow && app.state.frieren !== 'ready') { app.state.frieren = 'mapping'; app.save(); }
        bindPicker(main);
        main.addEventListener('input', (e) => {
          if (e.target.matches('[data-map-hint]')) {
            W.ai.hint = e.target.value;
            main.querySelectorAll('[data-act="map-hint"]').forEach((c) => c.setAttribute('aria-pressed', String(c.dataset.h === W.ai.hint)));
          }
        });
        main.addEventListener('keydown', (e) => { if (e.key === 'Enter' && e.target.matches('[data-map-hint]')) { e.preventDefault(); app.actions['map-ai-run'](); } });
        main.addEventListener('click', (e) => {
          const b = e.target.closest('[data-map-scope] button'); if (!b) return;
          W.ai.scope = b.dataset.v;
          b.parentElement.querySelectorAll('button').forEach((x) => x.setAttribute('aria-selected', String(x === b)));
        });
        if (W.phase === 'progress') {
          [[1, 1100], [2, 2300], [3, 3800]].forEach(([s, ms]) => setTimeout(() => { if (!alive()) return; W.step = s; paintAll(); }, ms));
          setTimeout(() => { if (!alive()) return; W.phase = 'review'; paintAll(); }, 4500);
        } else if (W.phase === 'review') {
          requestAnimationFrame(() => scrollToCurrent(main.querySelector('[data-map-pane] [data-pk-list]')));
        }
      },
    };
  });

  // ---- Actions ----------------------------------------------------------------------
  function select(id, focusSearch) {
    work.sel = id; work.q = '';
    if (isDesk()) {
      paint({ scrollPicker: true });
      if (focusSearch) { const q = document.querySelector('[data-map-pane] [data-pk-q]'); if (q) q.focus({ preventScroll: true }); }
    } else {
      paint();
      app.openSheet('ep-picker');
    }
  }

  Object.assign(app.actions, {
    'map-select': (b) => select(b.dataset.id, false),
    'map-change': (b) => select(b.dataset.id, true),
    'map-accept': (b) => {
      const it = byId(b.dataset.id); it.decision = 'accepted'; work.flash = it.id;
      const nx = nextPending(it.id); if (nx && isDesk()) work.sel = nx.id;
      paint({ scrollPicker: !!nx });
    },
    'map-unaccept': (b) => { const it = byId(b.dataset.id); it.decision = 'pending'; paint(); },
    'map-accept-all': () => { work.items.forEach((i) => { if (i.group === 'needs_review' && i.decision === 'pending' && i.target) i.decision = 'accepted'; }); paint(); },
    'map-ignore': (b) => {
      const it = byId(b.dataset.id); if (!it) return;
      if (b.closest('.sheet')) app.close();
      it.decision = 'ignored'; work.flash = it.id; paint();
    },
    'map-unignore': (b) => {
      const it = byId(b.dataset.id);
      if (it.group === 'extras' && !it.target) { work.sel = it.id; it.decision = 'pending'; if (isDesk()) paint({ scrollPicker: true }); else { paint(); app.openSheet('ep-picker'); } return; }
      it.decision = it.group === 'needs_review' || !it.target ? 'pending' : 'accepted'; work.flash = it.id; paint();
    },
    'map-pick': (b) => {
      const it = cur(); if (!it) return;
      const s = Number(b.dataset.s), e = Number(b.dataset.e);
      const same = it.target && it.target.s === s && it.target.e === e;
      if (!same) { const ep = findEp(s, e); it.target = { s, e, name: ep ? ep.name : `Episode ${e}` }; it.source = 'you'; it.conf = 'high'; it.updated = false; }
      it.decision = 'accepted'; work.flash = it.id;
      if (b.closest('.sheet')) app.close();
      paint();
      const row = document.querySelector(`[data-row="${it.id}"]`);
      if (row && !isDesk()) row.scrollIntoView({ block: 'center', behavior: 'auto' });
    },
    'map-toggle': (b) => { const g = b.dataset.g; work.open[g] = !isOpen(g); paint(); },
    'map-free': () => { work.freeOnly = !work.freeOnly; renderLists(); },
    'map-ai-toggle': () => { work.ai.open = !work.ai.open; if (work.ai.phase === 'done') work.ai.phase = 'idle'; paint({ ai: true }); if (work.ai.open) setTimeout(() => { const i = document.querySelector('[data-map-hint]'); if (i && isDesk()) i.focus(); }, 30); },
    'map-hint': (b) => { work.ai.hint = work.ai.hint === b.dataset.h ? '' : b.dataset.h; const i = document.querySelector('[data-map-hint]'); if (i) i.value = work.ai.hint; document.querySelectorAll('[data-act="map-hint"]').forEach((c) => c.setAttribute('aria-pressed', String(c.dataset.h === work.ai.hint))); },
    'map-ai-run': () => {
      const W = work; const a = W.ai;
      const targets = W.items.filter((i) => (a.scope === 'all' ? true : i.decision === 'pending'));
      a.n = targets.length; a.phase = 'working'; paint({ ai: true });
      setTimeout(() => {
        if (work !== W) return;
        let changed = 0;
        targets.forEach((i) => {
          const upd = W.job.recheck[i.id]; if (!upd || i.source === 'you') return;
          if (upd.reason) i.reason = upd.reason; if (upd.conf) i.conf = upd.conf;
          i.source = i.source === 'rules' && upd.reason.startsWith('AI') ? 'ai' : i.source;
          i.updated = true; i.hinted = !!a.hint; changed++;
        });
        a.phase = 'done'; a.open = false; a.changed = changed; paint({ ai: true });
      }, 2400);
    },
    'map-apply': (b) => {
      if (b.getAttribute('aria-disabled') === 'true') {
        const cf = conflicts();
        const it = work.items.find((i) => cf[i.id]) || work.items.find((i) => i.decision === 'pending' && i.target);
        if (!it) return;
        work.open[it.group] = true; work.flash = it.id; work.sel = it.id; paint({ scrollPicker: true });
        const row = document.querySelector(`[data-row="${it.id}"]`);
        if (row) { row.scrollIntoView({ block: 'center', behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' }); const btn = row.querySelector('[data-act="map-accept"], [data-act="map-change"]'); if (btn) btn.focus({ preventScroll: true }); }
        return;
      }
      b.classList.add('is-loading');
      const W = work;
      setTimeout(() => {
        if (work !== W) return;
        if (W.flow || W.jobKey === 'frieren') { app.state.frieren = 'ready'; app.save(); }
        if (W.flow) { app.go('#/home?toast=ready'); return; }
        W.phase = 'applied'; paintAll(); window.scrollTo(0, 0);
      }, 900);
    },
    'map-edit': () => { work.phase = 'review'; paintAll(); window.scrollTo(0, 0); },
  });
})();
