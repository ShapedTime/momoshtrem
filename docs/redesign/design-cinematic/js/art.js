/* Placeholder artwork. Stylised posters, backdrops, stills and portraits drawn as
   inline SVG so the prototype is self-contained. Each title gets its own palette
   (deliberately including very bright ones) to prove the UI stays legible over
   artwork of any colour. In the app these are TMDB images. */
(function () {
  let uid = 0;
  const id = (p) => `${p}${++uid}`;

  // Palette + motif per artwork key
  const A = {
    severance:   { bg: ['#0f3a37', '#0a1c1d', '#050c0d'], fg: '#e8f4f1', acc: '#7fe0c8', motif: 'hall', type: 'wide', title: 'SEVERANCE', ambient: '#2f8f7f' },
    frieren:     { bg: ['#d8ebf5', '#8fb8d2', '#d6b46b'], fg: '#1d2b3a', acc: '#f7e1a6', motif: 'field', type: 'serif', title: 'Frieren', ambient: '#88b5cf' },
    onepiece:    { bg: ['#ffd56b', '#ff9d3d', '#1b78c9'], fg: '#ffffff', acc: '#d8262c', motif: 'hat', type: 'cond', title: 'ONE PIECE', ambient: '#f29b38' },
    thebear:     { bg: ['#1c4aa0', '#11285a', '#081124'], fg: '#ffffff', acc: '#f2f2f2', motif: 'plate', type: 'cond', title: 'THE BEAR', ambient: '#2c5ed0' },
    shogun:      { bg: ['#1a0606', '#0a0505', '#000000'], fg: '#f3e9e0', acc: '#c71f1f', motif: 'sun', type: 'serif', title: 'SHŌGUN', ambient: '#9b1a1a' },
    doctorwho:   { bg: ['#1f56d6', '#0c2167', '#040a24'], fg: '#ffffff', acc: '#9cc4ff', motif: 'vortex', type: 'wide', title: 'DOCTOR WHO', ambient: '#2f5fe0' },
    bluey:       { bg: ['#9fdcfa', '#6cc3ef', '#f7a54a'], fg: '#123a63', acc: '#ffcf5a', motif: 'hills', type: 'round', title: 'Bluey', ambient: '#78c8f0' },
    pluribus:    { bg: ['#ffe36b', '#f6c533', '#e3a915'], fg: '#1a2a6c', acc: '#2f49c8', motif: 'smile', type: 'wide', title: 'PLUR1BUS', ambient: '#e9b928' },
    dune2:       { bg: ['#f2a65a', '#c4622d', '#4a1a0c'], fg: '#fff3e2', acc: '#ffe0b0', motif: 'dunes', type: 'wide', title: 'DUNE', sub: 'PART TWO', ambient: '#d2733a' },
    interstellar:{ bg: ['#0b0b0e', '#050507', '#000000'], fg: '#f4f1ea', acc: '#ffd28a', motif: 'ring', type: 'wide', title: 'INTERSTELLAR', ambient: '#a07a3a' },
    perfectdays: { bg: ['#9cc79a', '#3f7656', '#163024'], fg: '#f7f3e3', acc: '#fff3b8', motif: 'leaves', type: 'serif', title: 'PERFECT DAYS', ambient: '#4f8a63' },
    pastlives:   { bg: ['#f2b6a8', '#b37a96', '#3a4878'], fg: '#ffffff', acc: '#ffe6d6', motif: 'pair', type: 'serif', title: 'Past Lives', ambient: '#c58b9a' },
    goodplace:   { bg: ['#d6f0c6', '#9ed69a', '#4ea67a'], fg: '#183b2b', acc: '#ffe066', motif: 'clouds', type: 'round', title: 'the good place', ambient: '#8ccf8e' },
    shrinking:   { bg: ['#ffcf9e', '#f59a6b', '#c45a5a'], fg: '#2c1520', acc: '#fff1d6', motif: 'sun', type: 'round', title: 'SHRINKING', ambient: '#f0956a' },
    onlymurders: { bg: ['#f1d27a', '#c48a32', '#2b1a0e'], fg: '#2b1a0e', acc: '#7a1c1c', motif: 'windows', type: 'serif', title: 'Only Murders in the Building', ambient: '#c99a45' },
    mythicquest: { bg: ['#6a3cf0', '#3a1aa8', '#140845'], fg: '#ffffff', acc: '#43f0c0', motif: 'grid', type: 'cond', title: 'MYTHIC QUEST', ambient: '#5a35d8' },
    pushingdaisies:{ bg: ['#ffe46b', '#f5b021', '#3d8a3a'], fg: '#2b2008', acc: '#ffffff', motif: 'daisy', type: 'serif', title: 'Pushing Daisies', ambient: '#e8b52c' },
    dandadan:    { bg: ['#ff5aa8', '#7a2cff', '#120638'], fg: '#ffffff', acc: '#ffe94a', motif: 'vortex', type: 'cond', title: 'DAN DA DAN', ambient: '#c23ad8' },
    mushishi:    { bg: ['#cfe0b8', '#7fa27a', '#2d3d2a'], fg: '#1f2a1c', acc: '#f8f1d0', motif: 'leaves', type: 'serif', title: 'Mushishi', ambient: '#86a77d' },
    silo:        { bg: ['#4a4f55', '#23272c', '#0c0e10'], fg: '#e9e4d8', acc: '#d1a24a', motif: 'hall', type: 'wide', title: 'SILO', ambient: '#6b6450' },
    andor:       { bg: ['#2f3a48', '#141a22', '#06080b'], fg: '#e6e9ee', acc: '#e05a2f', motif: 'sun', type: 'wide', title: 'ANDOR', ambient: '#c0532d' },
    slowhorses:  { bg: ['#39424d', '#1b2128', '#0a0d10'], fg: '#f0ece2', acc: '#c9a85a', motif: 'windows', type: 'serif', title: 'Slow Horses', ambient: '#4c5866' },
    studio:      { bg: ['#ffffff', '#e8e4dc', '#c9c2b4'], fg: '#111111', acc: '#d4312a', motif: 'ring', type: 'wide', title: 'THE STUDIO', ambient: '#bdb4a2' },
    arrival:     { bg: ['#c9ccc8', '#7a8180', '#2a2f31'], fg: '#ffffff', acc: '#ffffff', motif: 'ring', type: 'wide', title: 'ARRIVAL', ambient: '#7d8584' },
    aftersun:    { bg: ['#7fd0d8', '#3a91a8', '#183a4f'], fg: '#ffffff', acc: '#ffd0a0', motif: 'pair', type: 'serif', title: 'Aftersun', ambient: '#48a2b6' },
    paprika:     { bg: ['#ff6b4a', '#c7254d', '#3a0b3d'], fg: '#ffffff', acc: '#ffd34a', motif: 'vortex', type: 'round', title: 'PAPRIKA', ambient: '#e0484b' },
    columbus:    { bg: ['#e8e2d0', '#b9b29a', '#5a6a5e'], fg: '#20251f', acc: '#2e6a4f', motif: 'windows', type: 'serif', title: 'Columbus', ambient: '#b0aa92' },
    totoro:      { bg: ['#8ad0a0', '#3d8a64', '#163a2c'], fg: '#ffffff', acc: '#f7f0c8', motif: 'leaves', type: 'round', title: 'Totoro', ambient: '#4a9a70' },
  };

  function grad(gid, stops, vertical = true) {
    const [x2, y2] = vertical ? [0, 1] : [1, 0];
    return `<linearGradient id="${gid}" x1="0" y1="0" x2="${x2}" y2="${y2}">${stops.map((c, i) => `<stop offset="${(i / (stops.length - 1)).toFixed(2)}" stop-color="${c}"/>`).join('')}</linearGradient>`;
  }

  // Motifs drawn into a W×H box. `s` = seed for variation.
  function motif(a, W, H, s) {
    const c = a.acc, f = a.fg;
    const r = (n) => ((Math.sin(s * 999 + n * 77.7) + 1) / 2);
    switch (a.motif) {
      case 'hall': { // receding corridor
        const cx = W * (0.5 + (r(1) - 0.5) * 0.2), cy = H * 0.52, w = W * 0.08, h = H * 0.12;
        return `<g opacity="0.9" stroke="${f}" stroke-opacity="0.25" stroke-width="1"><path d="M0 ${H * 0.18} L${cx - w} ${cy - h} M${W} ${H * 0.18} L${cx + w} ${cy - h} M0 ${H} L${cx - w} ${cy + h} M${W} ${H} L${cx + w} ${cy + h}"/></g>
          <rect x="${cx - w}" y="${cy - h}" width="${w * 2}" height="${h * 2}" fill="${c}" opacity="0.55"/>
          <rect x="${cx - w * 0.15}" y="${cy + h * 0.1}" width="${w * 0.3}" height="${h * 0.9}" rx="${w * 0.1}" fill="#06100f"/>
          <rect x="0" y="${H * 0.16}" width="${W}" height="${H * 0.02}" fill="${f}" opacity="0.12"/>`;
      }
      case 'field': {
        let flowers = '';
        for (let i = 0; i < 40; i++) flowers += `<circle cx="${(r(i) * W).toFixed(1)}" cy="${(H * 0.72 + r(i + 50) * H * 0.28).toFixed(1)}" r="${(1 + r(i + 9) * 2.4).toFixed(1)}" fill="${i % 3 ? '#ffffff' : c}" opacity="0.85"/>`;
        return `<path d="M0 ${H * 0.7} Q ${W * 0.4} ${H * 0.62} ${W} ${H * 0.68} V${H} H0Z" fill="#e2c27a" opacity="0.9"/>
          <path d="M0 ${H * 0.78} Q ${W * 0.6} ${H * 0.7} ${W} ${H * 0.8} V${H} H0Z" fill="#c9a259" opacity="0.8"/>
          ${flowers}
          <g transform="translate(${W * (0.42 + r(3) * 0.16)} ${H * 0.56})"><path d="M0 0 l-5 26 h10z" fill="#f4f1ea"/><circle cx="0" cy="-4" r="5" fill="#f4f1ea"/><path d="M-6 -6 q6 -10 12 0" fill="#f7f4ee"/><path d="M6 2 l10 -18" stroke="#6b4a2a" stroke-width="1.6"/></g>`;
      }
      case 'hat': {
        const cx = W / 2, cy = H * 0.5;
        return `<path d="M0 ${H * 0.66} Q ${W / 2} ${H * 0.6} ${W} ${H * 0.66} V${H} H0Z" fill="#1866b4"/>
          <path d="M0 ${H * 0.72} Q ${W / 2} ${H * 0.68} ${W} ${H * 0.74}" stroke="#ffffff" stroke-opacity="0.4" stroke-width="2" fill="none"/>
          <ellipse cx="${cx}" cy="${cy + 10}" rx="${W * 0.34}" ry="${W * 0.07}" fill="#f2cf73"/>
          <path d="M${cx - W * 0.17} ${cy + 8} Q ${cx - W * 0.16} ${cy - W * 0.18} ${cx} ${cy - W * 0.19} Q ${cx + W * 0.16} ${cy - W * 0.18} ${cx + W * 0.17} ${cy + 8}Z" fill="#f2cf73"/>
          <path d="M${cx - W * 0.17} ${cy + 1} Q ${cx} ${cy - 6} ${cx + W * 0.17} ${cy + 1} L ${cx + W * 0.17} ${cy + 8} Q ${cx} ${cy + 2} ${cx - W * 0.17} ${cy + 8}Z" fill="${c}"/>`;
      }
      case 'plate': {
        const cx = W * 0.5, cy = H * 0.5, R = Math.min(W, H) * 0.3;
        return `<circle cx="${cx}" cy="${cy}" r="${R}" fill="${c}" opacity="0.95"/><circle cx="${cx}" cy="${cy}" r="${R * 0.72}" fill="none" stroke="#c9d3e6" stroke-width="1.5"/>
          <path d="M${cx - R * 0.35} ${cy - R * 0.1} q ${R * 0.3} ${-R * 0.35} ${R * 0.7} 0 q ${-R * 0.3} ${R * 0.4} ${-R * 0.7} 0z" fill="#d9532c"/>
          <rect x="${cx + R * 1.15}" y="${cy - R}" width="${R * 0.12}" height="${R * 2}" rx="3" fill="#cfd6e2"/>`;
      }
      case 'sun': {
        const cx = W * (0.62 - r(2) * 0.24), cy = H * 0.42, R = Math.min(W, H) * 0.26;
        return `<circle cx="${cx}" cy="${cy}" r="${R}" fill="${c}"/>
          <path d="M${W * 0.15} ${H} C ${W * 0.3} ${H * 0.7} ${W * 0.25} ${H * 0.5} ${W * 0.38} ${H * 0.3}" stroke="${f}" stroke-opacity="0.18" stroke-width="${W * 0.05}" stroke-linecap="round" fill="none"/>
          <path d="M0 ${H * 0.8} L${W * 0.3} ${H * 0.66} L${W * 0.5} ${H * 0.74} L${W * 0.8} ${H * 0.6} L${W} ${H * 0.7} V${H} H0Z" fill="#000" opacity="0.55"/>`;
      }
      case 'vortex': {
        let rings = '';
        for (let i = 0; i < 9; i++) rings += `<ellipse cx="${W / 2}" cy="${H * 0.45}" rx="${(W * 0.08) * (i + 1)}" ry="${(H * 0.04) * (i + 1)}" fill="none" stroke="${c}" stroke-opacity="${(0.55 - i * 0.05).toFixed(2)}" stroke-width="1.5" transform="rotate(${-12 + i * 3} ${W / 2} ${H * 0.45})"/>`;
        return rings + `<rect x="${W / 2 - 6}" y="${H * 0.45 - 10}" width="12" height="20" rx="1.5" fill="#1d3f9e" stroke="#c9dcff" stroke-width="1"/>`;
      }
      case 'hills':
        return `<circle cx="${W * 0.78}" cy="${H * 0.22}" r="${W * 0.1}" fill="#fff6c8"/>
          <path d="M0 ${H * 0.66} Q ${W * 0.3} ${H * 0.5} ${W * 0.6} ${H * 0.64} T ${W} ${H * 0.6} V${H} H0Z" fill="#7cc46a"/>
          <path d="M0 ${H * 0.78} Q ${W * 0.4} ${H * 0.66} ${W} ${H * 0.8} V${H} H0Z" fill="#5aa94f"/>
          <g transform="translate(${W * 0.36} ${H * 0.6})"><ellipse cx="0" cy="0" rx="16" ry="11" fill="#4f8fd6"/><circle cx="14" cy="-10" r="9" fill="#4f8fd6"/><path d="M10 -18 l3 -9 l4 8z M17 -17 l5 -8 l2 9z" fill="#2f5f9e"/><circle cx="17" cy="-11" r="1.6" fill="#1a2a44"/></g>
          <g transform="translate(${W * 0.6} ${H * 0.62})"><ellipse cx="0" cy="0" rx="13" ry="9" fill="#f29a4a"/><circle cx="12" cy="-8" r="7.5" fill="#f29a4a"/><circle cx="14" cy="-9" r="1.4" fill="#4a2a10"/></g>`;
      case 'smile':
        return `<circle cx="${W / 2}" cy="${H * 0.46}" r="${Math.min(W, H) * 0.3}" fill="none" stroke="${c}" stroke-width="${Math.min(W, H) * 0.05}"/>
          <path d="M${W / 2 - W * 0.14} ${H * 0.5} Q ${W / 2} ${H * 0.62} ${W / 2 + W * 0.14} ${H * 0.5}" stroke="${c}" stroke-width="${Math.min(W, H) * 0.035}" fill="none" stroke-linecap="round"/>
          <circle cx="${W / 2 - W * 0.08}" cy="${H * 0.4}" r="${W * 0.025}" fill="${c}"/><circle cx="${W / 2 + W * 0.08}" cy="${H * 0.4}" r="${W * 0.025}" fill="${c}"/>`;
      case 'dunes':
        return `<circle cx="${W * 0.7}" cy="${H * 0.28}" r="${W * 0.09}" fill="#fff0d0" opacity="0.9"/><circle cx="${W * 0.82}" cy="${H * 0.2}" r="${W * 0.045}" fill="#fff0d0" opacity="0.7"/>
          <path d="M0 ${H * 0.62} Q ${W * 0.35} ${H * 0.5} ${W * 0.6} ${H * 0.6} T ${W} ${H * 0.56} V${H} H0Z" fill="#a8471f"/>
          <path d="M0 ${H * 0.76} Q ${W * 0.5} ${H * 0.62} ${W} ${H * 0.78} V${H} H0Z" fill="#6b2a12"/>
          <path d="M${W * 0.46} ${H * 0.7} l3 -16 l3 16z" fill="#1a0a05"/>`;
      case 'ring': {
        const cx = W / 2, cy = H * 0.44, R = Math.min(W, H) * 0.24;
        return `<ellipse cx="${cx}" cy="${cy}" rx="${R * 1.9}" ry="${R * 0.28}" fill="none" stroke="${c}" stroke-width="3" opacity="0.85"/>
          <circle cx="${cx}" cy="${cy}" r="${R}" fill="#000" stroke="${c}" stroke-width="4"/>
          <path d="M${cx - R * 1.9} ${cy} A ${R * 1.9} ${R * 0.28} 0 0 0 ${cx + R * 1.9} ${cy}" fill="none" stroke="${c}" stroke-width="3"/>`;
      }
      case 'leaves': {
        let d = '';
        for (let i = 0; i < 26; i++) d += `<circle cx="${(r(i) * W).toFixed(1)}" cy="${(r(i + 30) * H * 0.55).toFixed(1)}" r="${(W * 0.04 + r(i + 7) * W * 0.12).toFixed(1)}" fill="${i % 4 ? '#1d4a32' : '#2e6a48'}" opacity="0.7"/>`;
        for (let i = 0; i < 14; i++) d += `<circle cx="${(r(i + 90) * W).toFixed(1)}" cy="${(H * 0.25 + r(i + 60) * H * 0.6).toFixed(1)}" r="${(1.2 + r(i) * 3).toFixed(1)}" fill="${c}" opacity="0.8"/>`;
        return d;
      }
      case 'pair':
        return `<path d="M0 ${H * 0.74} H${W} V${H} H0Z" fill="#000" opacity="0.25"/>
          <g fill="#1a1f33" opacity="0.9"><rect x="${W * 0.36}" y="${H * 0.52}" width="${W * 0.05}" height="${H * 0.22}" rx="3"/><circle cx="${W * 0.385}" cy="${H * 0.5}" r="${W * 0.03}"/>
          <rect x="${W * 0.58}" y="${H * 0.53}" width="${W * 0.05}" height="${H * 0.21}" rx="3"/><circle cx="${W * 0.605}" cy="${H * 0.51}" r="${W * 0.028}"/></g>`;
      case 'clouds':
        return [0.2, 0.55, 0.85].map((x, i) => `<g fill="#ffffff" opacity="0.9"><ellipse cx="${W * x}" cy="${H * (0.3 + i * 0.12)}" rx="${W * 0.14}" ry="${W * 0.05}"/><ellipse cx="${W * x + W * 0.05}" cy="${H * (0.3 + i * 0.12) - W * 0.03}" rx="${W * 0.08}" ry="${W * 0.05}"/></g>`).join('')
          + `<path d="M0 ${H * 0.82} Q ${W / 2} ${H * 0.74} ${W} ${H * 0.82} V${H} H0Z" fill="#3d8a62"/>`;
      case 'windows': {
        let g = '';
        for (let y = 0; y < 6; y++) for (let x = 0; x < 5; x++) g += `<rect x="${W * 0.12 + x * W * 0.16}" y="${H * 0.3 + y * H * 0.09}" width="${W * 0.1}" height="${H * 0.06}" rx="1" fill="${(x + y * 3 + Math.round(r(x + y) * 3)) % 4 === 0 ? c : '#000'}" opacity="${(x + y) % 4 === 0 ? 0.85 : 0.35}"/>`;
        return `<rect x="${W * 0.08}" y="${H * 0.26}" width="${W * 0.84}" height="${H * 0.74}" fill="#000" opacity="0.25"/>` + g;
      }
      case 'grid': {
        let g = '';
        for (let i = 0; i < 12; i++) g += `<path d="M${(i / 11) * W} ${H * 0.55} L${W / 2 + (i / 11 - 0.5) * W * 3} ${H}" stroke="${c}" stroke-opacity="0.6"/>`;
        for (let i = 0; i < 6; i++) g += `<path d="M0 ${H * 0.55 + i * i * H * 0.014} H${W}" stroke="${c}" stroke-opacity="0.6"/>`;
        return g + `<circle cx="${W / 2}" cy="${H * 0.4}" r="${W * 0.14}" fill="#ff5ab0" opacity="0.85"/>`;
      }
      case 'daisy': {
        const cx = W / 2, cy = H * 0.46; let p = '';
        for (let i = 0; i < 14; i++) p += `<ellipse cx="${cx}" cy="${cy - W * 0.13}" rx="${W * 0.035}" ry="${W * 0.11}" fill="#fff" transform="rotate(${i * (360 / 14)} ${cx} ${cy})"/>`;
        return p + `<circle cx="${cx}" cy="${cy}" r="${W * 0.07}" fill="#f5b021"/>`;
      }
      default: return '';
    }
  }

  function titleText(a, W, H, scale = 1) {
    const fam = { serif: "'Cormorant Garamond', Georgia, serif", cond: "'Bebas Neue', Impact, sans-serif", wide: "'Geist', sans-serif", round: "'Geist', sans-serif" }[a.type];
    const weight = { serif: 600, cond: 400, wide: 300, round: 800 }[a.type];
    const ls = { serif: 0.5, cond: 2, wide: 6, round: -1 }[a.type];
    const words = a.title.split(' ');
    const lines = [];
    let cur = '';
    const maxChars = a.type === 'wide' ? 11 : 12;
    words.forEach((w) => { if ((cur + ' ' + w).trim().length > maxChars && cur) { lines.push(cur); cur = w; } else cur = (cur + ' ' + w).trim(); });
    if (cur) lines.push(cur);
    const longest = Math.max(...lines.map((l) => l.length));
    const cw = { cond: 0.42, serif: 0.56, round: 0.62, wide: 0.6 }[a.type] + (a.type === 'wide' ? 0.35 : 0);
    const want = (a.type === 'cond' ? 34 : a.type === 'wide' ? 17 : a.type === 'serif' ? 28 : 30) * scale * (W / 200);
    const base = Math.min(want, (W * 0.84) / (longest * cw));
    const lh = base * (a.type === 'wide' ? 1.5 : 1.0);
    const y0 = H * 0.86 - (lines.length - 1) * lh;
    const t = lines.map((l, i) => `<text x="${W / 2}" y="${y0 + i * lh}" text-anchor="middle" font-family="${fam}" font-weight="${weight}" font-size="${base}" letter-spacing="${ls * (W / 200)}" fill="${a.fg}">${l}</text>`).join('');
    const sub = a.sub ? `<text x="${W / 2}" y="${y0 + lines.length * lh * 0.95}" text-anchor="middle" font-family="'Geist', sans-serif" font-weight="500" font-size="${base * 0.42}" letter-spacing="${4 * (W / 200)}" fill="${a.fg}" opacity="0.85">${a.sub}</text>` : '';
    return t + sub;
  }

  function grain(gid) {
    return `<filter id="${gid}"><feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" stitchTiles="stitch"/><feColorMatrix values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 0.06 0"/></filter>`;
  }

  function svgFrame(W, H, a, s, withTitle, cls) {
    const g = id('g'), v = id('v'), n = id('n');
    return `<svg class="${cls || ''}" viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid slice" aria-hidden="true" focusable="false">
      <defs>${grad(g, a.bg)}<radialGradient id="${v}" cx="0.5" cy="0.45" r="0.75"><stop offset="0.55" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity="0.45"/></radialGradient>${grain(n)}</defs>
      <rect width="${W}" height="${H}" fill="url(#${g})"/>
      ${motif(a, W, H, s)}
      <rect width="${W}" height="${H}" fill="url(#${v})"/>
      <rect width="${W}" height="${H}" filter="url(#${n})" opacity="0.6"/>
      ${withTitle ? titleText(a, W, H) : ''}
    </svg>`;
  }

  function get(key) { return A[key] || A.severance; }

  const art = {
    palette: A,
    ambient: (key) => get(key).ambient,
    poster: (key, cls) => svgFrame(200, 300, get(key), 1, true, cls),
    backdrop: (key, cls, seed = 2) => svgFrame(640, 360, get(key), seed, false, cls),
    still: (key, n = 1, cls) => svgFrame(320, 180, get(key), n * 1.37, false, cls),
    portrait: (name, cls) => {
      const h = [...name].reduce((x, c) => x + c.charCodeAt(0), 0);
      const hue = (h * 37) % 360;
      const initials = name.split(' ').map((w) => w[0]).slice(0, 2).join('');
      const g = id('p');
      return `<svg class="${cls || ''}" viewBox="0 0 80 80" aria-hidden="true"><defs><linearGradient id="${g}" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="hsl(${hue} 30% 32%)"/><stop offset="1" stop-color="hsl(${(hue + 40) % 360} 30% 16%)"/></linearGradient></defs><rect width="80" height="80" fill="url(#${g})"/><circle cx="40" cy="32" r="14" fill="hsl(${hue} 20% 70% / 0.35)"/><path d="M14 80c2-16 13-24 26-24s24 8 26 24z" fill="hsl(${hue} 20% 70% / 0.3)"/><text x="40" y="76" text-anchor="middle" font-family="Geist, sans-serif" font-size="11" font-weight="600" fill="#fff" opacity="0.7">${initials}</text></svg>`;
    },
  };

  window.MS = window.MS || {};
  window.MS.art = art;
})();
