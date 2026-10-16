import type { StorageSettings } from '@/lib/api/contract';
import {
  belowCommittedCapCopy,
  belowCommittedCopy,
  configLine,
  crossesMidnight,
  customLimitHelp,
  diskBar,
  draftOf,
  fieldErrorCopy,
  fieldProblem,
  firstError,
  floorNote,
  focusAfterRemove,
  formatSizeInput,
  hasChanges,
  isStorageField,
  keepDefaultsLine,
  keptCapHelp,
  limitBounds,
  limitHelp,
  parseSize,
  SIZE_ERRORS,
  smartCapHelp,
  storageChanges,
  storageNavSummary,
  trimNote,
  windowsProblem,
  withEdits,
} from '@/lib/settings/storage';

const GiB = 1024 ** 3;

/** The mock install's (fixtures/settings.ts): a 1 TB disk, auto at 750 GB, smart downloads on. */
const AUTO: StorageSettings = {
  limit_bytes: null,
  source: 'auto',
  auto_bytes: 750e9,
  config_bytes: null,
  limit_effective_bytes: 750e9,
  effective_bytes: 750e9,
  floored: false,
  floor_free_bytes: 5 * GiB,
  min_bytes: 4 * GiB,
  max_bytes: 900e9,
  reserve_bytes: 20 * GiB,
  disk: { total_bytes: 1e12, free_bytes: 812e9 },
  usage: { kept_bytes: 48.768e9, smart_bytes: 1.92e9, cache_bytes: 11.712e9 },
  committed_kept_bytes: 86.4e9,
  kept: { max_bytes: null, effective_max_bytes: 375e9, default_until_watched: true, default_days: 30 },
  smart: { enabled: true, episodes_ahead: 2, max_bytes: null, effective_max_bytes: 112.5e9 },
  background: { enabled: true, pause_while_streaming: false, windows: [] },
};

/** config.yaml's global_cache_size: 53760 (MiB), before any PUT sent a limit. */
const CONFIG: StorageSettings = { ...AUTO, source: 'config', config_bytes: 53_760 * 1024 ** 2, limit_effective_bytes: 53_760 * 1024 ** 2 };

/** A custom 300 GB limit, explicit caps and two windows. */
const CUSTOM: StorageSettings = {
  ...AUTO,
  limit_bytes: 300e9,
  source: 'setting',
  limit_effective_bytes: 300e9,
  kept: { ...AUTO.kept, max_bytes: 120e9, effective_max_bytes: 120e9 },
  smart: { ...AUTO.smart, max_bytes: 37.5e9, effective_max_bytes: 37.5e9 },
  background: { ...AUTO.background, windows: [{ start: '23:00', end: '06:00' }, { start: '12:00', end: '13:30' }] },
};

/** Before #202: nothing kept or picked yet, a fresh install's defaults. */
const FRESH: StorageSettings = {
  ...AUTO,
  usage: { kept_bytes: 0, smart_bytes: 0, cache_bytes: 36 * GiB },
  committed_kept_bytes: 0,
  smart: { ...AUTO.smart, enabled: false },
  background: { ...AUTO.background, pause_while_streaming: true },
};

describe('parseSize', () => {
  it('reads gigabytes by default, and decimal or binary units', () => {
    expect(parseSize('150')).toBe(150e9);
    expect(parseSize(' 52.5 ')).toBe(52.5e9);
    expect(parseSize('52.5 GB')).toBe(52.5e9);
    expect(parseSize('.5')).toBe(0.5e9);
    expect(parseSize('1.2 TB')).toBe(1.2e12);
    expect(parseSize('500mb')).toBe(500e6);
    expect(parseSize('48 GiB')).toBe(48 * GiB);
    expect(parseSize('1 tib')).toBe(1024 * GiB);
  });

  it('rounds to whole bytes', () => {
    expect(parseSize('0.0000000001')).toBe(0);
    expect(Number.isInteger(parseSize('1.33333333 GB'))).toBe(true);
  });

  it('is null when empty and undefined for anything else', () => {
    expect(parseSize('')).toBeNull();
    expect(parseSize('   ')).toBeNull();
    for (const text of ['ten', '-5', '1,5', '5 PB', '1e3', '5 G B', '$5', '9'.repeat(30)]) expect(parseSize(text)).toBeUndefined();
  });
});

describe('formatSizeInput and limitBounds', () => {
  it('writes GB with at most one decimal and no grouping', () => {
    expect(formatSizeInput(300e9)).toBe('300');
    expect(formatSizeInput(53_760 * 1024 ** 2)).toBe('56.4');
    expect(formatSizeInput(1.25e12)).toBe('1250');
  });

  it('rounds the minimum up and the maximum down, so what the help says fits', () => {
    const bounds = limitBounds({ min_bytes: 4 * GiB, max_bytes: 189.47e9 });
    expect(bounds).toEqual({ min: '4.3 GB', max: '189.4 GB' });
    expect(parseSize('4.3')!).toBeGreaterThanOrEqual(4 * GiB);
    expect(parseSize('189.4')!).toBeLessThanOrEqual(189.47e9);
    expect(customLimitHelp(AUTO)).toBe('From 4.3 GB to 900 GB (90% of the disk).');
  });
});

describe('windows', () => {
  it('takes up to 4 windows of two different HH:MM times, crossing midnight or not', () => {
    expect(windowsProblem([])).toBeNull();
    expect(windowsProblem([{ start: '23:00', end: '06:00' }, { start: '00:00', end: '23:59' }])).toBeNull();
  });

  it('names the window with a problem', () => {
    expect(windowsProblem([{ start: '01:00', end: '07:00' }, { start: '', end: '07:00' }])).toEqual({ index: 1, message: 'Window 2 needs a start and an end time, as HH:MM.' });
    expect(windowsProblem([{ start: '24:00', end: '07:00' }])).toEqual({ index: 0, message: 'Window 1 needs a start and an end time, as HH:MM.' });
    expect(windowsProblem([{ start: '7:00', end: '08:00' }])?.index).toBe(0);
    expect(windowsProblem([{ start: '05:00', end: '05:00' }])).toEqual({ index: 0, message: 'Window 1 starts when it ends. Pick two different times.' });
    const five = Array.from({ length: 5 }, (_, i) => ({ start: `0${i}:00`, end: `0${i}:30` }));
    expect(windowsProblem(five)).toEqual({ index: 4, message: 'Keep it to 4 windows or fewer.' });
  });

  it('moves focus to the next Remove once one goes, else the previous one, else Add', () => {
    expect(focusAfterRemove(1, 3)).toBe(1);
    expect(focusAfterRemove(0, 2)).toBe(0);
    expect(focusAfterRemove(2, 3)).toBe(1);
    expect(focusAfterRemove(0, 1)).toBe('add');
  });

  it('knows a window that ends the next day', () => {
    expect(crossesMidnight({ start: '23:00', end: '06:00' })).toBe(true);
    expect(crossesMidnight({ start: '01:00', end: '07:00' })).toBe(false);
    expect(crossesMidnight({ start: '', end: '06:00' })).toBe(false);
  });
});

describe('draftOf', () => {
  it('maps the source to the limit mode, and starts a custom size from the limit that applies', () => {
    expect(draftOf(AUTO)).toMatchObject({ limitMode: 'auto', limit: '750', keptMax: '', smartMax: '', days: 30, untilWatched: true, episodesAhead: 2 });
    expect(draftOf(CONFIG)).toMatchObject({ limitMode: 'config', limit: '56.4' });
    expect(draftOf(CUSTOM)).toMatchObject({ limitMode: 'custom', limit: '300', keptMax: '120', smartMax: '37.5' });
  });

  it('holds the custom prefill inside the bounds it says, for a limit clamped at the maximum', () => {
    const clamped = { ...CONFIG, config_bytes: 300e9, limit_effective_bytes: 189.67e9, max_bytes: 189.67e9 };
    expect(customLimitHelp(clamped)).toBe('From 4.3 GB to 189.6 GB (90% of the disk).');
    expect(draftOf(clamped).limit).toBe('189.6');
    expect(storageChanges({ limitMode: 'custom' }, clamped)).toEqual({ patch: { limit_bytes: 189.6e9 }, errors: {}, windowIndex: null });
    expect(draftOf({ ...CONFIG, limit_effective_bytes: 4 * GiB }).limit).toBe('4.3');
  });

  it('copies the windows, so editing the draft never changes the query’s data', () => {
    const draft = draftOf(CUSTOM);
    draft.windows[0].start = '22:00';
    expect(CUSTOM.background.windows[0].start).toBe('23:00');
  });

  it('puts this page’s edits over the saved settings', () => {
    expect(withEdits(AUTO, { smartEnabled: false, windows: [{ start: '01:00', end: '07:00' }] })).toMatchObject({
      smartEnabled: false,
      windows: [{ start: '01:00', end: '07:00' }],
      limitMode: 'auto',
    });
  });
});

describe('storageChanges', () => {
  it('sends nothing without edits, and nothing for edits back to what was loaded', () => {
    expect(storageChanges({}, AUTO)).toEqual({ patch: {}, errors: {}, windowIndex: null });
    const back = storageChanges({ limitMode: 'auto', limit: '300', keptMax: '', smartEnabled: true, episodesAhead: 2, windows: [] }, AUTO);
    expect(back.patch).toEqual({});
    expect(hasChanges(back)).toBe(false);
  });

  it('never echoes limit_bytes: GET’s null under config.yaml stays unsent while the limit isn’t touched', () => {
    const changes = storageChanges({ smartEnabled: false, pauseWhileStreaming: true }, CONFIG);
    expect(changes.patch).toEqual({ smart: { enabled: false }, background: { pause_while_streaming: true } });
    expect('limit_bytes' in changes.patch).toBe(false);
    expect(storageChanges({ limitMode: 'config' }, CONFIG).patch).toEqual({});
  });

  it('sends limit_bytes when the limit control changed: Use auto is null, a custom size is bytes', () => {
    expect(storageChanges({ limitMode: 'auto' }, CONFIG).patch).toEqual({ limit_bytes: null });
    expect(storageChanges({ limitMode: 'auto' }, CUSTOM).patch).toEqual({ limit_bytes: null });
    expect(storageChanges({ limitMode: 'custom' }, AUTO).patch).toEqual({ limit_bytes: 750e9 });
    expect(storageChanges({ limitMode: 'custom', limit: '300' }, CONFIG).patch).toEqual({ limit_bytes: 300e9 });
    expect(storageChanges({ limit: '250 GB' }, CUSTOM).patch).toEqual({ limit_bytes: 250e9 });
  });

  it('keeps a custom size still showing the saved one unchanged (the input rounds to 0.1 GB)', () => {
    const odd = { ...CUSTOM, limit_bytes: 300_040_000_000, limit_effective_bytes: 300_040_000_000 };
    expect(storageChanges({ limit: '300', limitMode: 'custom' }, odd).patch).toEqual({});
    expect(storageChanges({ limit: '300 GB' }, CUSTOM).patch).toEqual({});
  });

  it('holds a custom size that doesn’t read or is out of range, with a reason', () => {
    expect(storageChanges({ limitMode: 'custom', limit: '' }, AUTO).errors).toEqual({ limit_bytes: SIZE_ERRORS.limit });
    expect(storageChanges({ limitMode: 'custom', limit: 'lots' }, AUTO).errors).toEqual({ limit_bytes: SIZE_ERRORS.limit });
    expect(storageChanges({ limitMode: 'custom', limit: '3' }, AUTO).errors).toEqual({ limit_bytes: 'Enter a size from 4.3 GB to 900 GB.' });
    const tooBig = storageChanges({ limitMode: 'custom', limit: '1 TB' }, AUTO);
    expect(tooBig.errors.limit_bytes).toBe('Enter a size from 4.3 GB to 900 GB.');
    expect(tooBig.patch).toEqual({});
    expect(hasChanges(tooBig)).toBe(true);
    // Choosing Auto again drops the problem
    expect(storageChanges({ limitMode: 'auto', limit: '3' }, AUTO).errors).toEqual({});
  });

  it('sends the caps as bytes, and empty as the default share (null)', () => {
    expect(storageChanges({ keptMax: '200', smartMax: '25.5' }, AUTO).patch).toEqual({ kept: { max_bytes: 200e9 }, smart: { max_bytes: 25.5e9 } });
    expect(storageChanges({ keptMax: '', smartMax: ' ' }, CUSTOM).patch).toEqual({ kept: { max_bytes: null }, smart: { max_bytes: null } });
    expect(storageChanges({ keptMax: '120 GB' }, CUSTOM).patch).toEqual({});
    expect(storageChanges({ keptMax: 'half', smartMax: '-1' }, AUTO).errors).toEqual({ 'kept.max_bytes': SIZE_ERRORS.kept, 'smart.max_bytes': SIZE_ERRORS.smart });
  });

  it('merges nested fields per object: only the ones that differ', () => {
    const changes = storageChanges({ untilWatched: false, days: null, episodesAhead: 4, smartEnabled: true, backgroundEnabled: false }, AUTO);
    expect(changes.patch).toEqual({ kept: { default_until_watched: false, default_days: null }, smart: { episodes_ahead: 4 }, background: { enabled: false } });
  });

  it('sends the windows whole when they differ, and holds a bad one with its index', () => {
    expect(storageChanges({ windows: [{ start: '23:00', end: '06:00' }, { start: '12:00', end: '13:30' }] }, CUSTOM).patch).toEqual({});
    expect(storageChanges({ windows: [{ start: '23:00', end: '06:00' }] }, CUSTOM).patch).toEqual({ background: { windows: [{ start: '23:00', end: '06:00' }] } });
    expect(storageChanges({ windows: [] }, CUSTOM).patch).toEqual({ background: { windows: [] } });
    const bad = storageChanges({ windows: [{ start: '01:00', end: '07:00' }, { start: '09:00', end: '09:00' }], pauseWhileStreaming: true }, AUTO);
    expect(bad.errors).toEqual({ 'background.windows': 'Window 2 starts when it ends. Pick two different times.' });
    expect(bad.windowIndex).toBe(1);
    expect(bad.patch).toEqual({ background: { pause_while_streaming: true } });
  });

  it('names the first problem in the page’s order', () => {
    expect(firstError({ 'background.windows': 'x', 'kept.max_bytes': 'y' })).toBe('kept.max_bytes');
    expect(firstError({})).toBeNull();
  });
});

describe('errors', () => {
  it('knows the fields a 422 names', () => {
    expect(isStorageField('kept.max_bytes')).toBe(true);
    expect(isStorageField('background.windows')).toBe(true);
    expect(isStorageField('kept')).toBe(false);
    expect(isStorageField('toString')).toBe(false);
    expect(isStorageField(undefined)).toBe(false);
  });

  it('says the ranges in words, never the server’s field names', () => {
    for (const field of ['limit_bytes', 'kept.max_bytes', 'smart.max_bytes', 'background.windows'] as const) {
      expect(fieldErrorCopy(field, 'kept.max_bytes can be at most 728.5 GB')).not.toMatch(/_bytes|\.windows/);
    }
    expect(fieldErrorCopy('smart.max_bytes', '')).toMatch(/leave it empty for 15% of the limit\.$/);
    expect(fieldErrorCopy('kept.default_days', 'kept.default_days must be 7, 30 or 90')).toBe('kept.default_days must be 7, 30 or 90');
  });

  it('tells a limit out of the bounds (the disk shrank since they were read) from one that squeezes the caps', () => {
    const shrunk = { min_bytes: 4 * GiB, max_bytes: 180e9 };
    expect(fieldErrorCopy('limit_bytes', '', { sent: 200e9, settings: shrunk })).toBe('Enter a size from 4.3 GB to 180 GB.');
    expect(fieldErrorCopy('limit_bytes', '', { sent: 150e9, settings: shrunk })).toMatch(/^This limit leaves too little for the space you set/);
    expect(fieldErrorCopy('limit_bytes', '', { sent: null, settings: shrunk })).toMatch(/^This limit leaves too little/);
    expect(fieldErrorCopy('limit_bytes', '')).toMatch(/^This limit leaves too little/);
  });

  it('says what kept downloads need on 409 limit_below_committed: in the bar for a limit, on the field for the kept cap', () => {
    expect(belowCommittedCopy(86.4e9)).toBe('Kept downloads need 86.4 GB. Stop keeping some, or set a limit that leaves room for them.');
    expect(belowCommittedCapCopy(86.4e9)).toBe('Kept downloads need 86.4 GB. Enter at least that here, or stop keeping some.');
  });

  it('places a refused PUT’s problem on its field, or leaves it to the bar', () => {
    const conflict = { status: 409, code: 'limit_below_committed', message: 'Kept downloads need 86.4 GB…', details: { committed_kept_bytes: 86.4e9 } };
    expect(fieldProblem(conflict, { kept: { max_bytes: 50e9 } })).toEqual({ field: 'kept.max_bytes', text: belowCommittedCapCopy(86.4e9) });
    expect(fieldProblem(conflict, { kept: { max_bytes: null }, limit_bytes: 100e9 })?.field).toBe('kept.max_bytes');
    expect(fieldProblem(conflict, { limit_bytes: 100e9 })).toBeNull();
    expect(fieldProblem(conflict, { kept: { default_days: 7 } })).toBeNull();

    const invalid = { status: 422, code: 'validation_error', field: 'smart.max_bytes', message: 'smart.max_bytes can be at most 2 GB', details: {} };
    expect(fieldProblem(invalid, { smart: { max_bytes: 9e9 } })).toEqual({ field: 'smart.max_bytes', text: fieldErrorCopy('smart.max_bytes', '') });
    const range = { status: 422, code: 'validation_error', field: 'limit_bytes', message: 'limit_bytes must be from…', details: {} };
    expect(fieldProblem(range, { limit_bytes: 950e9 }, AUTO)?.text).toBe('Enter a size from 4.3 GB to 900 GB.');
    expect(fieldProblem({ ...invalid, field: 'kept' }, {})).toBeNull();
    expect(fieldProblem({ status: 500, code: 'internal_error', message: 'disk', details: {} }, {})).toBeNull();
  });
});

describe('the words', () => {
  it('reads the limit by source (PLAN §6 UI copy)', () => {
    expect(limitHelp('auto', AUTO)).toBe('Auto (75% of the disk) · 750 GB. It follows the disk when it grows.');
    expect(limitHelp('custom', AUTO)).toBe('Custom: the cache stays within the size below.');
    expect(limitHelp('config', AUTO)).toBe('Choose Auto (75% of the disk · 750 GB) or a custom size.');
    expect(configLine(CONFIG)).toBe('From config.yaml · 56.4 GB');
  });

  it('says what each cap is now, and only what downloads need or hold when they do', () => {
    expect(keptCapHelp(AUTO)).toBe('Empty: half the limit. Now 375 GB; kept downloads need 86.4 GB.');
    expect(smartCapHelp(AUTO)).toBe('Empty: 15% of the limit. Now 113 GB; smart downloads hold 1.9 GB.');
    expect(keptCapHelp(FRESH)).toBe('Empty: half the limit. Now 375 GB.');
    expect(smartCapHelp(FRESH)).toBe('Empty: 15% of the limit. Now 113 GB.');
  });

  it('says a new keep’s default', () => {
    expect(keepDefaultsLine(true, 30)).toBe('New keeps last until you watch them, 30 days at most.');
    expect(keepDefaultsLine(true, null)).toBe('New keeps last until you watch them.');
    expect(keepDefaultsLine(false, 7)).toBe('New keeps last 7 days.');
    expect(keepDefaultsLine(false, null)).toBe('New keeps stay until you remove them.');
  });

  it('notes the floor, and what it holds the cache at while it does', () => {
    expect(floorNote(AUTO)).toEqual({ text: 'The cache always leaves 5.4 GB of the disk free.', warn: false });
    expect(floorNote({ ...AUTO, floored: true, effective_bytes: 869e9 })).toEqual({ text: 'Holding the cache at 869 GB to keep 5.4 GB of the disk free.', warn: true });
  });

  it('says the cache is trimmed while it holds more than it may', () => {
    expect(trimNote(AUTO)).toBeNull();
    expect(trimNote(CONFIG)).toBeNull();
    expect(trimNote({ ...CONFIG, effective_bytes: CONFIG.limit_effective_bytes })).toBe('Trimming the cache down to 56.4 GB.');
  });

  it('sums up the limit for the settings nav', () => {
    expect(storageNavSummary(AUTO)).toEqual({ text: '750 GB · auto' });
    expect(storageNavSummary(CUSTOM)).toEqual({ text: '300 GB · custom' });
    expect(storageNavSummary(CONFIG)).toEqual({ text: '56.4 GB · config.yaml' });
    expect(storageNavSummary(undefined)).toEqual({ text: '' });
  });
});

describe('diskBar', () => {
  it('splits the disk into kept, smart, cache, other files and free, with the limit’s marker', () => {
    const bar = diskBar(AUTO);
    expect(bar.segments.map((s) => [s.part, s.bytes])).toEqual([
      ['kept', 48.768e9],
      ['smart', 1.92e9],
      ['cache', 11.712e9],
      ['other', 125.6e9],
      ['free', 812e9],
    ]);
    expect(bar.segments.reduce((sum, s) => sum + s.pct, 0)).toBeCloseTo(100);
    expect(bar.filled).toBe(62.4e9);
    expect(bar.limitPct).toBe(75);
    expect(bar.label).toBe('Disk of 1.0 TB: Kept 48.8 GB, Smart 1.9 GB, Cache 11.7 GB, Other files 126 GB, Free 812 GB. Limit 750 GB.');
  });

  it('reads well before any download: zero kept and smart are listed, and take no width', () => {
    const bar = diskBar(FRESH);
    expect(bar.segments.find((s) => s.part === 'kept')).toMatchObject({ bytes: 0, pct: 0 });
    expect(bar.segments.find((s) => s.part === 'smart')).toMatchObject({ bytes: 0, pct: 0 });
    expect(bar.label).toMatch(/^Disk of 1\.0 TB: Kept 0 B, Smart 0 B, Cache 38\.7 GB, /);
  });

  it('never draws a negative part or past the disk', () => {
    const odd = diskBar({ disk: { total_bytes: 100e9, free_bytes: 90e9 }, usage: { kept_bytes: 0, smart_bytes: 0, cache_bytes: 20e9 }, limit_effective_bytes: 150e9 });
    expect(odd.segments.find((s) => s.part === 'other')?.bytes).toBe(0);
    expect(odd.limitPct).toBe(100);
  });
});
