// momoshtrem design tokens: the single source of truth.
// `node tools/build-tokens.mjs` writes tokens/tokens.json, tokens/tokens.css and tokens/theme.css.
// Names follow Tailwind v4 theme namespaces (--color-*, --font-*, --text-*, --radius-*, --shadow-*,
// --ease-*, --blur-*, --breakpoint-*), so the same variable names work in plain CSS and in Tailwind.

export const primitives = {
  cream:     { 50: '#FFFDF9', 100: '#FFF8F0', 200: '#F8ECDF', 300: '#F0DDCC', 400: '#E2C8B5', 500: '#C9A994' },
  espresso:  { 950: '#140E0B', 900: '#1A120F', 850: '#211814', 800: '#2A1E19', 750: '#33251F', 700: '#3F2E27', 600: '#574137' },
  ink:       { 900: '#2A1C16', 700: '#4A352C', 600: '#664C41', 500: '#7F6357' },
  peach:     { 100: '#FFE9E0', 200: '#FFD3C3', 300: '#FFB9A2', 400: '#FFA48D', 500: '#FF8B6E' },
  terracotta:{ 200: '#F79B76', 300: '#F2895F', 400: '#E2704D', 500: '#D45A3A', 600: '#B8482C', 700: '#A93F25', 800: '#8A321D' },
  butter:    { 100: '#FFF6D6', 200: '#FFEDA8', 300: '#FFE58A', 400: '#F9D25C' },
  matcha:    { 100: '#F2F6D8', 200: '#E3EAA0', 300: '#CFD866', 400: '#B7C83A', 500: '#9DB021', 600: '#7A900F', 700: '#5F7A08', 800: '#4F6B06' },
  sky:       { 100: '#DDEEFA', 300: '#7EC1F2', 500: '#3B92D1', 700: '#1D669C' },
  grape:     { 100: '#ECE5FC', 300: '#BBA6FF', 500: '#8B6CE8', 700: '#6343BE' },
  amber:     { 100: '#FFE9C4', 300: '#F6B44A', 500: '#E39A2A', 700: '#9A5300' },
  raspberry: { 100: '#FCE3E6', 300: '#FF8197', 500: '#D9435F', 700: '#B02A47' },
  cat:       { fur: '#F4F0E6', pink: '#FFB3B5', line: '#9C5548', shadow: '#FF8B6E' },
};

const p = primitives;

// Semantic colours. Every hue means one thing:
// terracotta = the one primary action · matcha = ready / success (the TV is on) · sky = downloading / info
// grape = needs your review · amber = warning · raspberry = error · dashed neutral = missing · muted ink = watched
export const semantic = {
  light: {
    'bg': p.cream[100],
    'bg-tint': '#FCEFE2',
    'surface': p.cream[50],
    'surface-2': p.cream[200],
    'surface-3': '#F2E2D3',
    'raised': '#FFFFFF',
    'line': '#EEDCCB',
    'line-strong': p.cream[400],
    'ink': p.ink[900],
    'ink-2': p.ink[600],
    'ink-3': p.ink[500],
    'ink-inverse': p.cream[100],
    'scrim': 'rgb(42 28 22 / 0.42)',
    'glass': 'rgb(255 248 240 / 0.82)',

    'primary': p.terracotta[600],
    'primary-hover': p.terracotta[700],
    'primary-press': p.terracotta[800],
    'primary-lip': p.terracotta[800],
    'on-primary': p.cream[100],
    'primary-ink': p.terracotta[700],
    'primary-soft': '#FFE4D8',
    'primary-soft-ink': p.terracotta[800],

    'accent': p.matcha[500],
    'accent-soft': '#EEF3C6',
    'accent-ink': p.matcha[800],
    'butter': p.butter[300],
    'butter-soft': p.butter[100],
    'peach': p.peach[400],
    'on-sticker': p.ink[900],
    'focus': p.sky[700],

    'ready': p.matcha[800], 'ready-bg': '#ECF2C6', 'ready-solid': '#8FAE1B',
    'downloading': p.sky[700], 'downloading-bg': p.sky[100], 'downloading-solid': p.sky[500],
    'missing': p.ink[500], 'missing-bg': 'transparent', 'missing-line': '#D7BBA9',
    'review': p.grape[700], 'review-bg': p.grape[100], 'review-solid': p.grape[500],
    'watched': p.ink[600], 'watched-bg': '#F1E6DB', 'watched-solid': '#9C8072',
    'upcoming': p.ink[600], 'upcoming-bg': p.cream[200],
    'warning': p.amber[700], 'warning-bg': p.amber[100], 'warning-solid': p.amber[500],
    'error': p.raspberry[700], 'error-bg': p.raspberry[100], 'error-solid': p.raspberry[500],

    'cat-fur': p.cat.fur, 'cat-pink': p.cat.pink, 'cat-line': p.cat.line, 'cat-shadow': p.cat.shadow,
    'sticker-shadow': p.peach[500],
  },
  dark: {
    'bg': p.espresso[900],
    'bg-tint': '#201713',
    'surface': '#241915',
    'surface-2': '#2E211C',
    'surface-3': '#382922',
    'raised': '#2B1F1A',
    'line': p.espresso[700],
    'line-strong': p.espresso[600],
    'ink': '#FBEEE3',
    'ink-2': '#D5BDAF',
    'ink-3': '#B09588',
    'ink-inverse': p.ink[900],
    'scrim': 'rgb(8 5 4 / 0.62)',
    'glass': 'rgb(26 18 15 / 0.80)',

    'primary': p.terracotta[300],
    'primary-hover': p.terracotta[200],
    'primary-press': p.terracotta[400],
    'primary-lip': p.terracotta[600],
    'on-primary': '#2A1209',
    'primary-ink': p.terracotta[200],
    'primary-soft': '#3D2219',
    'primary-soft-ink': p.peach[300],

    'accent': '#C3D65A',
    'accent-soft': '#2C3012',
    'accent-ink': '#D6E57C',
    'butter': '#F2D06A',
    'butter-soft': '#33290F',
    'peach': p.peach[400],
    'on-sticker': p.ink[900],
    'focus': p.sky[300],

    'ready': '#C3D65A', 'ready-bg': '#2A3010', 'ready-solid': '#A9C23A',
    'downloading': '#8ECBF5', 'downloading-bg': '#13283A', 'downloading-solid': '#5AA9E6',
    'missing': '#B09588', 'missing-bg': 'transparent', 'missing-line': p.espresso[600],
    'review': p.grape[300], 'review-bg': '#2A2142', 'review-solid': '#9C82F5',
    'watched': '#C4AC9F', 'watched-bg': '#2E211C', 'watched-solid': '#8A7266',
    'upcoming': '#D5BDAF', 'upcoming-bg': '#2E211C',
    'warning': p.amber[300], 'warning-bg': '#3A2810', 'warning-solid': '#E9A23B',
    'error': p.raspberry[300], 'error-bg': '#3A1820', 'error-solid': '#F0607A',

    'cat-fur': '#EFE9DD', 'cat-pink': p.cat.pink, 'cat-line': p.cat.line, 'cat-shadow': '#C2573C',
    'sticker-shadow': '#8A321D',
  },
};

export const fonts = {
  // var(--font-fredoka) etc. are set by next/font in the app; the quoted name is the fallback for plain HTML.
  display: 'var(--font-fredoka, "Fredoka"), "Nunito", ui-rounded, system-ui, sans-serif',
  sans: 'var(--font-atkinson, "Atkinson Hyperlegible Next"), ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif',
  mono: 'var(--font-atkinson-mono, "Atkinson Hyperlegible Mono"), ui-monospace, "SF Mono", Menlo, Consolas, monospace',
};

// [size, line-height, letter-spacing, weight, family]
export const typeScale = {
  'display':  ['clamp(2.5rem, 1.9rem + 2.2vw, 3.5rem)', '1.05', '-0.01em', 600, 'display'],
  'title-1':  ['clamp(1.875rem, 1.6rem + 1vw, 2.5rem)', '1.1', '-0.005em', 600, 'display'],
  'title-2':  ['1.5rem', '1.2', '0', 600, 'display'],
  'title-3':  ['1.25rem', '1.25', '0', 600, 'display'],
  'headline': ['1.0625rem', '1.4', '0', 700, 'sans'],
  'body':     ['1rem', '1.5', '0', 400, 'sans'],
  'callout':  ['0.9375rem', '1.45', '0', 400, 'sans'],
  'label':    ['0.875rem', '1.3', '0.005em', 600, 'sans'],
  'caption':  ['0.8125rem', '1.35', '0.005em', 500, 'sans'],
  'micro':    ['0.75rem', '1.3', '0.015em', 700, 'sans'],
  'code':     ['0.8125rem', '1.5', '0', 400, 'mono'],
};

// 4 px base. Tailwind: --spacing: 0.25rem (p-4 = 16 px). Named steps for plain CSS.
export const space = { 0: '0', 0.5: '2px', 1: '4px', 1.5: '6px', 2: '8px', 3: '12px', 4: '16px', 5: '20px', 6: '24px', 8: '32px', 10: '40px', 12: '48px', 16: '64px', 20: '80px', 24: '96px' };

export const radius = { xs: '6px', sm: '10px', md: '14px', lg: '20px', xl: '28px', '2xl': '36px', pill: '999px', poster: '12px' };

export const shadow = {
  light: {
    '1': '0 1px 2px rgb(74 38 20 / 0.07), 0 1px 1px rgb(74 38 20 / 0.04)',
    '2': '0 6px 14px -4px rgb(74 38 20 / 0.12), 0 2px 4px rgb(74 38 20 / 0.06)',
    '3': '0 16px 36px -10px rgb(74 38 20 / 0.22), 0 4px 10px rgb(74 38 20 / 0.06)',
    '4': '0 28px 72px -16px rgb(74 38 20 / 0.34), 0 8px 20px rgb(74 38 20 / 0.08)',
    'sticker': '3px 3px 0 var(--color-sticker-shadow)',
    'lip': '0 3px 0 var(--color-primary-lip)',
  },
  dark: {
    '1': '0 1px 2px rgb(0 0 0 / 0.40), inset 0 1px 0 rgb(255 240 228 / 0.03)',
    '2': '0 8px 18px -6px rgb(0 0 0 / 0.55), inset 0 1px 0 rgb(255 240 228 / 0.04)',
    '3': '0 18px 40px -12px rgb(0 0 0 / 0.65), inset 0 1px 0 rgb(255 240 228 / 0.05)',
    '4': '0 30px 80px -16px rgb(0 0 0 / 0.75), inset 0 1px 0 rgb(255 240 228 / 0.06)',
    'sticker': '3px 3px 0 var(--color-sticker-shadow)',
    'lip': '0 3px 0 var(--color-primary-lip)',
  },
};

export const blur = { glass: '18px', scrim: '4px', artwork: '40px' };

export const motion = {
  duration: { instant: '90ms', quick: '150ms', base: '220ms', slow: '320ms', sheet: '420ms', celebrate: '900ms' },
  ease: {
    out: 'cubic-bezier(0.22, 1, 0.36, 1)',
    in: 'cubic-bezier(0.55, 0, 1, 0.45)',
    'in-out': 'cubic-bezier(0.65, 0, 0.35, 1)',
    spring: 'cubic-bezier(0.34, 1.56, 0.64, 1)',
    sheet: 'cubic-bezier(0.32, 0.72, 0, 1)',
  },
};

export const z = { base: 0, raised: 1, sticky: 10, nav: 20, dropdown: 30, sheet: 40, dialog: 50, toast: 60, palette: 70, tooltip: 80 };

export const breakpoints = { tablet: '40rem', laptop: '64rem', desktop: '90rem' }; // 640 · 1024 · 1440

export const layout = {
  'page-max': '1360px',
  'gutter': 'clamp(16px, 2.5vw, 40px)',
  'topbar-h': '68px',
  'tabbar-h': '64px',
  'tap': '44px',
  'poster-min': 'clamp(104px, 28vw, 172px)',
};
