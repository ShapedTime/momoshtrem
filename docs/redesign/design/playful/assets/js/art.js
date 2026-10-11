/* Placeholder artwork for the prototype: TMDB-style posters (2:3), backdrops and stills (16:9) and cast portraits.
   Each title gets its own palette and motif so the UI is tested against artwork of every colour.
   Real TMDB images drop into the same slots (see HANDOFF.md). */
(function () {
  const hash = (s) => { let h = 2166136261; for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 16777619); } return h >>> 0; };
  const rng = (seed) => { let s = seed || 1; return () => ((s = (Math.imul(s, 1664525) + 1013904223) >>> 0) / 4294967296); };
  const FONT = {
    serif: "Georgia, 'DejaVu Serif', 'Times New Roman', serif",
    sans: "'Helvetica Neue', Arial, 'Liberation Sans', sans-serif",
    round: "Fredoka, 'Nunito', 'Arial Rounded MT Bold', sans-serif",
    cond: "'Arial Narrow', 'Liberation Sans Narrow', 'Helvetica Neue', Arial, sans-serif",
    mono: "'Courier New', 'Liberation Mono', monospace",
  };
  const esc = (s) => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;');
  const grad = (id, a, b, vertical = true) => `<linearGradient id="${id}" x1="0" y1="0" x2="${vertical ? 0 : 1}" y2="${vertical ? 1 : 0}"><stop offset="0" stop-color="${a}"/><stop offset="1" stop-color="${b}"/></linearGradient>`;

  // Motifs draw into a w×h box. P = palette [bg-top, bg-bottom, c1, c2, c3]. R = seeded random.
  const M = {
    corridor(w, h, P, R, id) {
      const vx = w / 2, vy = h * 0.42, dw = w * 0.11, dh = h * 0.14;
      return `<rect width="${w}" height="${h}" fill="url(#${id}g)"/>
        <polygon points="0,0 ${w},0 ${vx + dw},${vy - dh} ${vx - dw},${vy - dh}" fill="${P[2]}" opacity=".95"/>
        <polygon points="0,${h} ${w},${h} ${vx + dw},${vy + dh} ${vx - dw},${vy + dh}" fill="${P[3]}"/>
        <polygon points="0,0 ${vx - dw},${vy - dh} ${vx - dw},${vy + dh} 0,${h}" fill="${P[2]}" opacity=".78"/>
        <polygon points="${w},0 ${vx + dw},${vy - dh} ${vx + dw},${vy + dh} ${w},${h}" fill="${P[2]}" opacity=".62"/>
        <rect x="${vx - dw}" y="${vy - dh}" width="${dw * 2}" height="${dh * 2}" fill="${P[4]}"/>
        <rect x="${vx - dw * 0.35}" y="${vy - dh * 0.6}" width="${dw * 0.7}" height="${dh * 1.6}" fill="${P[0]}" opacity=".9"/>
        ${[0.18, 0.32].map((t) => `<line x1="${t * w}" y1="${t * h * 0.42 / 0.42 * 0.42}" x2="${t * w}" y2="${h - t * h * 0.58}" stroke="${P[0]}" stroke-width="1" opacity=".25"/>`).join('')}`;
    },
    dunes(w, h, P, R) {
      const sun = `<circle cx="${w * (0.3 + R() * 0.4)}" cy="${h * 0.3}" r="${Math.min(w, h) * 0.17}" fill="${P[4]}" opacity=".95"/>`;
      let d = '';
      [0.55, 0.68, 0.8].forEach((y, i) => {
        const a = h * y, b = h * (y - 0.08 + R() * 0.1);
        d += `<path d="M0 ${a} Q ${w * 0.35} ${b} ${w * 0.6} ${a - 4} T ${w} ${a - h * 0.04} V ${h} H 0 Z" fill="${P[2 + (i % 2)]}" opacity="${0.75 + i * 0.12}"/>`;
      });
      const fig = `<rect x="${w * 0.62}" y="${h * 0.62}" width="${w * 0.012 + 1}" height="${h * 0.05}" rx="1" fill="${P[0]}" opacity=".8"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${sun}${d}${fig}`;
    },
    ring(w, h, P) {
      const cx = w / 2, cy = h * 0.42, r = Math.min(w, h) * 0.24;
      return `<rect width="${w}" height="${h}" fill="${P[0]}"/>
        <circle cx="${cx}" cy="${cy}" r="${r * 1.9}" fill="${P[2]}" opacity=".10"/>
        <ellipse cx="${cx}" cy="${cy}" rx="${r * 2.1}" ry="${r * 0.32}" fill="none" stroke="${P[3]}" stroke-width="${r * 0.16}" opacity=".85"/>
        <circle cx="${cx}" cy="${cy}" r="${r}" fill="none" stroke="${P[3]}" stroke-width="${r * 0.22}" opacity=".9"/>
        <circle cx="${cx}" cy="${cy}" r="${r * 0.86}" fill="${P[1]}"/>
        <path d="M ${cx - r * 2.1} ${cy} A ${r * 2.1} ${r * 0.32} 0 0 0 ${cx + r * 2.1} ${cy}" fill="none" stroke="${P[4]}" stroke-width="${r * 0.08}"/>`;
    },
    field(w, h, P, R) {
      let fl = '';
      for (let i = 0; i < 46; i++) fl += `<circle cx="${R() * w}" cy="${h * 0.62 + R() * h * 0.38}" r="${1 + R() * 2.4}" fill="${i % 3 ? P[4] : '#ffffff'}" opacity=".9"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>
        <path d="M0 ${h * 0.62} Q ${w * 0.5} ${h * 0.52} ${w} ${h * 0.64} V ${h} H 0 Z" fill="${P[2]}"/>
        <path d="M0 ${h * 0.74} Q ${w * 0.4} ${h * 0.68} ${w} ${h * 0.78} V ${h} H 0 Z" fill="${P[3]}"/>
        ${fl}
        <g transform="translate(${w * 0.58} ${h * 0.5})"><path d="M0 0 L ${-w * 0.035} ${h * 0.12} H ${w * 0.035} Z" fill="#fff"/>
        <circle cx="0" cy="${-h * 0.012}" r="${w * 0.018}" fill="#fff"/><line x1="${w * 0.04}" y1="${-h * 0.05}" x2="${w * 0.03}" y2="${h * 0.13}" stroke="${P[4]}" stroke-width="1.6"/></g>`;
    },
    sea(w, h, P) {
      let waves = '';
      for (let i = 0; i < 6; i++) { const y = h * (0.58 + i * 0.07); waves += `<path d="M0 ${y} q ${w / 8} -6 ${w / 4} 0 t ${w / 4} 0 t ${w / 4} 0 t ${w / 4} 0" fill="none" stroke="#fff" stroke-width="2" opacity="${0.5 - i * 0.06}"/>`; }
      const cx = w * 0.5, cy = h * 0.38, s = Math.min(w, h);
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/><rect y="${h * 0.56}" width="${w}" height="${h * 0.44}" fill="${P[1]}"/>${waves}
        <ellipse cx="${cx}" cy="${cy + s * 0.07}" rx="${s * 0.3}" ry="${s * 0.07}" fill="${P[2]}"/>
        <path d="M ${cx - s * 0.15} ${cy + s * 0.07} Q ${cx - s * 0.14} ${cy - s * 0.13} ${cx} ${cy - s * 0.13} Q ${cx + s * 0.14} ${cy - s * 0.13} ${cx + s * 0.15} ${cy + s * 0.07} Z" fill="${P[2]}"/>
        <path d="M ${cx - s * 0.15} ${cy + s * 0.03} Q ${cx} ${cy - s * 0.0} ${cx + s * 0.15} ${cy + s * 0.03} L ${cx + s * 0.15} ${cy + s * 0.07} Q ${cx} ${cy + s * 0.04} ${cx - s * 0.15} ${cy + s * 0.07} Z" fill="${P[3]}"/>`;
    },
    stripes(w, h, P) {
      let st = '';
      for (let i = 0; i < 9; i++) st += `<rect x="0" y="${h * 0.1 + i * h * 0.045}" width="${w}" height="${i % 3 === 1 ? h * 0.02 : h * 0.008}" fill="${P[2]}" opacity="${i % 3 === 1 ? 1 : 0.7}"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${st}
        <circle cx="${w * 0.5}" cy="${h * 0.66}" r="${Math.min(w, h) * 0.2}" fill="${P[3]}"/><circle cx="${w * 0.5}" cy="${h * 0.66}" r="${Math.min(w, h) * 0.14}" fill="${P[0]}"/>
        <rect x="${w * 0.66}" y="${h * 0.5}" width="${w * 0.025}" height="${h * 0.3}" rx="2" fill="${P[4]}" transform="rotate(18 ${w * 0.67} ${h * 0.65})"/>`;
    },
    sun(w, h, P) {
      const r = Math.min(w, h) * 0.3;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>
        <circle cx="${w * 0.5}" cy="${h * 0.4}" r="${r}" fill="${P[2]}"/>
        <line x1="${w * 0.12}" y1="${h * 0.78}" x2="${w * 0.92}" y2="${h * 0.12}" stroke="${P[3]}" stroke-width="${Math.max(2, w * 0.012)}" stroke-linecap="round"/>
        <line x1="${w * 0.12}" y1="${h * 0.78}" x2="${w * 0.24}" y2="${h * 0.68}" stroke="${P[4]}" stroke-width="${Math.max(4, w * 0.03)}" stroke-linecap="round"/>
        <path d="M0 ${h * 0.86} Q ${w * 0.3} ${h * 0.8} ${w * 0.55} ${h * 0.86} T ${w} ${h * 0.84} V ${h} H0 Z" fill="${P[1]}"/>`;
    },
    box(w, h, P, R) {
      let stars = '';
      for (let i = 0; i < 40; i++) stars += `<circle cx="${R() * w}" cy="${R() * h * 0.8}" r="${0.4 + R() * 1.2}" fill="#fff" opacity="${0.3 + R() * 0.6}"/>`;
      const bw = Math.min(w * 0.26, h * 0.3), bh = bw * 1.75, x = w / 2 - bw / 2, y = h * 0.58 - bh / 2;
      let win = '';
      for (let r = 0; r < 3; r++) for (let c = 0; c < 2; c++) win += `<rect x="${x + bw * (0.14 + c * 0.38)}" y="${y + bh * (0.2 + r * 0.24)}" width="${bw * 0.32}" height="${bh * 0.18}" rx="1" fill="${r === 0 ? P[4] : P[3]}" opacity="${r === 0 ? 0.95 : 0.55}"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${stars}
        <circle cx="${w / 2}" cy="${y - bh * 0.06}" r="${bw * 0.5}" fill="${P[4]}" opacity=".18"/>
        <rect x="${x - bw * 0.06}" y="${y}" width="${bw * 1.12}" height="${bh * 0.08}" fill="${P[2]}"/>
        <rect x="${x}" y="${y + bh * 0.08}" width="${bw}" height="${bh * 0.92}" fill="${P[2]}"/>${win}
        <rect x="${w / 2 - bw * 0.06}" y="${y - bh * 0.07}" width="${bw * 0.12}" height="${bh * 0.07}" fill="${P[4]}"/>`;
    },
    blobs(w, h, P, R) {
      const s = Math.min(w, h);
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>
        <circle cx="${w * 0.22}" cy="${h * 0.24}" r="${s * 0.16}" fill="${P[4]}"/>
        <path d="M ${w * 0.2} ${h} Q ${w * 0.22} ${h * 0.5} ${w * 0.5} ${h * 0.48} Q ${w * 0.8} ${h * 0.46} ${w * 0.84} ${h} Z" fill="${P[2]}"/>
        <circle cx="${w * 0.42}" cy="${h * 0.42}" r="${s * 0.1}" fill="${P[2]}"/><circle cx="${w * 0.62}" cy="${h * 0.42}" r="${s * 0.1}" fill="${P[2]}"/>
        <path d="M ${w * 0.36} ${h * 0.36} L ${w * 0.39} ${h * 0.22} L ${w * 0.45} ${h * 0.34} Z M ${w * 0.68} ${h * 0.36} L ${w * 0.65} ${h * 0.22} L ${w * 0.59} ${h * 0.34} Z" fill="${P[2]}"/>
        <ellipse cx="${w * 0.52}" cy="${h * 0.56}" rx="${s * 0.11}" ry="${s * 0.08}" fill="${P[3]}"/>
        <circle cx="${w * 0.47}" cy="${h * 0.5}" r="${s * 0.018}" fill="#1d2b48"/><circle cx="${w * 0.57}" cy="${h * 0.5}" r="${s * 0.018}" fill="#1d2b48"/>`;
    },
    crowd(w, h, P) {
      let c = '';
      const cols = 6, rows = 8, gx = w / cols, gy = h * 0.8 / rows;
      for (let r = 0; r < rows; r++) for (let k = 0; k < cols; k++) {
        const x = gx * (k + 0.5), y = h * 0.14 + gy * (r + 0.5), odd = r === 4 && k === 3;
        c += `<circle cx="${x}" cy="${y}" r="${Math.min(gx, gy) * 0.36}" fill="${odd ? P[3] : P[2]}"/>`;
        if (!odd) c += `<path d="M ${x - gx * 0.12} ${y + gy * 0.04} Q ${x} ${y + gy * 0.16} ${x + gx * 0.12} ${y + gy * 0.04}" stroke="${P[4]}" stroke-width="1.4" fill="none"/>`;
      }
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${c}`;
    },
    canopy(w, h, P, R) {
      let l = '';
      for (let i = 0; i < 60; i++) l += `<circle cx="${R() * w}" cy="${R() * h * 0.7}" r="${4 + R() * Math.min(w, h) * 0.08}" fill="${[P[2], P[3], P[1]][i % 3]}" opacity="${0.6 + R() * 0.4}"/>`;
      let d = '';
      for (let i = 0; i < 18; i++) d += `<circle cx="${R() * w}" cy="${R() * h * 0.75}" r="${1 + R() * 3}" fill="${P[4]}" opacity="${0.5 + R() * 0.5}"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${l}${d}<rect y="${h * 0.78}" width="${w}" height="${h * 0.22}" fill="${P[1]}"/>
        <rect x="${w * 0.47}" y="${h * 0.62}" width="${w * 0.06}" height="${h * 0.18}" fill="${P[1]}"/>`;
    },
    two(w, h, P, R) {
      let lights = '';
      for (let i = 0; i < 26; i++) lights += `<circle cx="${R() * w}" cy="${h * (0.55 + R() * 0.12)}" r="${0.8 + R() * 1.6}" fill="${P[4]}" opacity="${0.4 + R() * 0.6}"/>`;
      const fig = (x, flip) => `<g transform="translate(${x} ${h * 0.68}) scale(${flip ? -1 : 1} 1)"><circle cx="0" cy="${-h * 0.11}" r="${h * 0.028}" fill="${P[2]}"/>
        <path d="M ${-h * 0.03} ${h * 0.12} L ${-h * 0.035} ${-h * 0.06} Q 0 ${-h * 0.09} ${h * 0.035} ${-h * 0.06} L ${h * 0.03} ${h * 0.12} Z" fill="${P[2]}"/></g>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${lights}<rect y="${h * 0.78}" width="${w}" height="${h * 0.22}" fill="${P[3]}"/>${fig(w * 0.38, false)}${fig(w * 0.62, true)}`;
    },
    spiral(w, h, P) {
      let c = '';
      for (let i = 9; i > 0; i--) c += `<circle cx="${w / 2}" cy="${h * 0.45}" r="${i * Math.min(w, h) * 0.05}" fill="${i % 2 ? P[2] : P[3]}"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${c}<circle cx="${w / 2}" cy="${h * 0.45}" r="${Math.min(w, h) * 0.035}" fill="${P[4]}"/>`;
    },
    mountains(w, h, P, R) {
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/><circle cx="${w * (0.25 + R() * 0.5)}" cy="${h * 0.26}" r="${Math.min(w, h) * 0.08}" fill="${P[4]}"/>
        <path d="M0 ${h * 0.7} L ${w * 0.28} ${h * 0.38} L ${w * 0.5} ${h * 0.58} L ${w * 0.72} ${h * 0.34} L ${w} ${h * 0.62} V ${h} H0 Z" fill="${P[2]}"/>
        <path d="M0 ${h * 0.82} L ${w * 0.3} ${h * 0.6} L ${w * 0.58} ${h * 0.78} L ${w} ${h * 0.56} V ${h} H0 Z" fill="${P[3]}"/>`;
    },
    moon(w, h, P, R) {
      let st = '';
      for (let i = 0; i < 30; i++) st += `<circle cx="${R() * w}" cy="${R() * h * 0.7}" r="${0.5 + R()}" fill="#fff" opacity="${0.4 + R() * 0.5}"/>`;
      const r = Math.min(w, h) * 0.2;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${st}<circle cx="${w * 0.6}" cy="${h * 0.34}" r="${r}" fill="${P[4]}"/><circle cx="${w * 0.6 + r * 0.4}" cy="${h * 0.34 - r * 0.2}" r="${r * 0.9}" fill="${P[0]}"/>
        <path d="M0 ${h * 0.8} Q ${w / 2} ${h * 0.7} ${w} ${h * 0.8} V ${h} H0 Z" fill="${P[2]}"/>`;
    },
    city(w, h, P, R) {
      let b = '';
      let x = 0;
      while (x < w) { const bw = w * (0.06 + R() * 0.08), bh = h * (0.2 + R() * 0.4); b += `<rect x="${x}" y="${h * 0.86 - bh}" width="${bw}" height="${bh}" fill="${P[2]}"/>`;
        for (let j = 0; j < 5; j++) if (R() > 0.5) b += `<rect x="${x + bw * 0.25}" y="${h * 0.86 - bh + bh * (0.1 + j * 0.16)}" width="${bw * 0.18}" height="${bh * 0.05}" fill="${P[4]}"/>`; x += bw + 2; }
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/><circle cx="${w * 0.7}" cy="${h * 0.3}" r="${Math.min(w, h) * 0.13}" fill="${P[3]}" opacity=".9"/>${b}<rect y="${h * 0.86}" width="${w}" height="${h * 0.14}" fill="${P[1]}"/>`;
    },
    split(w, h, P) {
      return `<rect width="${w}" height="${h}" fill="${P[0]}"/><rect x="${w / 2}" width="${w / 2}" height="${h}" fill="${P[1]}"/>
        <circle cx="${w / 2}" cy="${h * 0.42}" r="${Math.min(w, h) * 0.22}" fill="${P[2]}"/><rect x="${w / 2}" y="${h * 0.42 - Math.min(w, h) * 0.22}" width="${Math.min(w, h) * 0.22}" height="${Math.min(w, h) * 0.44}" fill="${P[3]}"/>`;
    },
    halftone(w, h, P) {
      let d = '';
      for (let y = 0; y < 14; y++) for (let x = 0; x < 10; x++) { const r = Math.max(0.3, (1 - Math.hypot(x - 5, y - 6) / 8)) * Math.min(w / 10, h / 14) * 0.48; d += `<circle cx="${(x + 0.5) * w / 10}" cy="${(y + 0.5) * h / 14}" r="${r}" fill="${P[2]}"/>`; }
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${d}`;
    },
    arch(w, h, P) {
      const aw = w * 0.5, ax = w * 0.25, top = h * 0.2;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/><path d="M ${ax} ${h * 0.82} V ${top + aw / 2} A ${aw / 2} ${aw / 2} 0 0 1 ${ax + aw} ${top + aw / 2} V ${h * 0.82} Z" fill="${P[2]}"/>
        <circle cx="${w / 2}" cy="${top + aw * 0.55}" r="${aw * 0.2}" fill="${P[4]}"/><rect y="${h * 0.82}" width="${w}" height="${h * 0.18}" fill="${P[3]}"/>`;
    },
    waves(w, h, P) {
      let d = '';
      for (let i = 0; i < 7; i++) { const y = h * (0.2 + i * 0.11); d += `<path d="M0 ${y} C ${w * 0.25} ${y - h * 0.08} ${w * 0.5} ${y + h * 0.08} ${w} ${y - h * 0.02} V ${h} H0 Z" fill="${[P[2], P[3], P[4]][i % 3]}" opacity="${0.35 + i * 0.09}"/>`; }
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${d}`;
    },
    grid(w, h, P, R) {
      let d = '';
      const n = 6;
      for (let r = 0; r < n * 1.5; r++) for (let c = 0; c < n; c++) if (R() > 0.35) d += `<rect x="${c * w / n + 2}" y="${r * w / n + 2}" width="${w / n - 4}" height="${w / n - 4}" rx="3" fill="${[P[2], P[3], P[4]][(r + c) % 3]}" opacity="${0.5 + R() * 0.5}"/>`;
      return `<rect width="${w}" height="${h}" fill="url(#ID)"/>${d}`;
    },
  };

  const SPECS = {}; // filled by data.js through ART.define()
  const define = (id, spec) => { SPECS[id] = spec; };
  const spec = (id) => SPECS[id] || { title: id, motif: 'grid', palette: ['#5b4a42', '#2a1c16', '#d45a3a', '#ffe58a', '#f4f0e6'], font: 'sans' };

  function titleBlock(s, w, h) {
    if (s.noTitle) return '';
    const words = s.posterTitle || s.title;
    const font = FONT[s.font || 'sans'];
    const upper = s.upper !== false && s.font !== 'serif' && s.font !== 'round';
    const txt = upper ? words.toUpperCase() : words;
    const parts = txt.length > (upper ? 9 : 12) && txt.includes(' ') ? (() => { const a = txt.split(' '); const mid = Math.ceil(a.length / 2); return [a.slice(0, mid).join(' '), a.slice(mid).join(' ')]; })() : [txt];
    const longest = Math.max(...parts.map((p) => p.length));
    const factor = { cond: upper ? 0.64 : 0.5, sans: upper ? 0.78 : 0.6, serif: 0.56, round: 0.62, mono: 0.66 }[s.font || 'sans'];
    const size = Math.min(w * 0.16, (w * 0.84) / (longest * factor));
    const y0 = s.titleTop ? h * 0.1 + size : h * 0.9 - (parts.length - 1) * size * 1.02;
    const weight = s.font === 'serif' ? 400 : 700;
    const ls = upper ? size * 0.06 : 0;
    return parts.map((p, i) => `<text x="${w / 2}" y="${y0 + i * size * 1.02}" text-anchor="middle" font-family="${font}" font-weight="${weight}" font-size="${size.toFixed(1)}" letter-spacing="${ls.toFixed(1)}" fill="${s.ink || '#fff'}">${esc(p)}</text>`).join('')
      + (s.tagline ? `<text x="${w / 2}" y="${s.titleTop ? h * 0.1 - 4 : h * 0.96}" text-anchor="middle" font-family="${FONT.sans}" font-size="${(w * 0.04).toFixed(1)}" letter-spacing="1.5" fill="${s.ink || '#fff'}" opacity=".75">${esc(s.tagline.toUpperCase())}</text>` : '');
  }

  function draw(id, w, h, opts = {}) {
    const s = spec(id);
    const seed = hash(id + (opts.seed || ''));
    const R = rng(seed);
    const uid = 'a' + seed.toString(36) + (opts.k || '') + Math.floor(Math.random() * 1e6).toString(36);
    let P = s.palette;
    if (opts.shift) { // episode stills: rotate the palette a little so episodes differ
      const r = opts.shift % 4;
      P = r === 0 ? P : r === 1 ? [P[1], P[0], P[2], P[3], P[4]] : r === 2 ? [P[0], P[2], P[1], P[4], P[3]] : [P[3], P[1], P[2], P[0], P[4]];
    }
    const body = (M[s.motif] || M.grid)(w, h, P, R, uid).replace(/url\(#ID\)/g, `url(#${uid}g)`);
    const vignette = opts.vignette ? `<rect width="${w}" height="${h}" fill="url(#${uid}v)"/>` : '';
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" preserveAspectRatio="xMidYMid slice" role="img" aria-label="${esc(opts.label || s.title)}">
      <defs>${grad(uid + 'g', P[0], P[1])}<linearGradient id="${uid}v" x1="0" y1="0" x2="0" y2="1"><stop offset=".45" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity=".55"/></linearGradient></defs>
      ${body}${vignette}${opts.title ? titleBlock(s, w, h) : ''}</svg>`;
  }

  const poster = (id, label) => draw(id, 200, 300, { title: true, label: label || `${spec(id).title} poster` });
  const backdrop = (id) => draw(id, 480, 270, { k: 'b', label: `${spec(id).title} backdrop`, seed: 'backdrop' });
  const still = (id, n, label) => draw(id, 320, 180, { k: 's' + n, seed: 'ep' + n, shift: n, label: label || `${spec(id).title} episode still` });

  const SKIN = ['#E8B796', '#C98E6B', '#8D5B3E', '#F1C9A5', '#B47A57', '#6B4430'];
  const BG = ['#F7D9C4', '#DCE9C0', '#D6E6F5', '#EADCF7', '#F9E7B0', '#F3CFC9'];
  const HAIR = ['#2A1C16', '#5A3A28', '#8A5A3A', '#C7A27A', '#3B2A22', '#9C9C9C'];
  function face(name) {
    const h = hash(name), R = rng(h);
    const bg = BG[h % BG.length], skin = SKIN[(h >> 3) % SKIN.length], hair = HAIR[(h >> 6) % HAIR.length];
    const long = R() > 0.5;
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" role="img" aria-label="${esc(name)}"><rect width="100" height="100" fill="${bg}"/>
      ${long ? `<path d="M26 58 Q24 22 50 20 Q76 22 74 58 L74 78 H26 Z" fill="${hair}"/>` : ''}
      <path d="M18 100 Q20 74 50 72 Q80 74 82 100 Z" fill="${['#4F6B06', '#1D669C', '#A93F25', '#6343BE', '#2A1C16'][(h >> 9) % 5]}"/>
      <rect x="43" y="58" width="14" height="16" rx="6" fill="${skin}"/>
      <ellipse cx="50" cy="46" rx="18" ry="21" fill="${skin}"/>
      <path d="M31 42 Q32 22 50 22 Q68 22 69 42 Q60 32 50 33 Q40 32 31 42 Z" fill="${hair}"/></svg>`;
  }

  window.ART = { define, spec, poster, backdrop, still, face, hash };
})();
