import type { Keep } from '@/lib/api/contract';
import { DOWNLOAD_STATE_STATUS, STATUS } from '@/lib/status';
import {
  canChangeDuration,
  defaultKeepChoice,
  estimateCopy,
  keepCreateDuration,
  keepDuration,
  keepExpiryLabel,
  keepOf,
  keepOptions,
  keepRowPill,
  keepStateLabel,
  keepWantsBytes,
  storageFullLead,
} from '@/lib/title/keep';

// Local time: "Waits until 01:00" and the dates are read in the browser's zone
const NOW = new Date(2026, 9, 17, 12, 0);
const at = (month: number, day: number, hours = 12) => new Date(2026, month, day, hours).toISOString();

const keep = (fields: Partial<Keep> = {}): Keep => ({
  download_id: 5,
  reason: 'kept',
  scope: 'movie',
  state: 'downloading',
  pct: 42,
  until_watched: true,
  expires_at: null,
  ...fields,
});

describe('keepStateLabel', () => {
  it('reads the PLAN copy for every state', () => {
    expect(keepStateLabel('queued', 0, null)).toBe('Queued');
    expect(keepStateLabel('waiting_for_stream', 10, null)).toBe('Waits for the stream to end');
    expect(keepStateLabel('paused', 30, null)).toBe('Paused');
    expect(keepStateLabel('stalled', 37, at(9, 17))).toBe('No peers · retrying');
    expect(keepStateLabel('disk_full', 0, null)).toBe('Disk full');
    expect(keepStateLabel('storage_full', 0, null)).toBe('Kept space is full');
    expect(keepStateLabel('done', 100, null)).toBe('On disk');
    expect(keepStateLabel('no_file', 0, null)).toBe('No file yet');
    expect(keepStateLabel('unavailable', 0, null)).toBe('Can’t keep this file');
  });

  it('floors the percentage, so it never says 100% before it is on disk', () => {
    expect(keepStateLabel('downloading', 42.9, null)).toBe('Downloading 42%');
    expect(keepStateLabel('downloading', 99.7, null)).toBe('Downloading 99%');
    expect(keepStateLabel('downloading', 140, null)).toBe('Downloading 100%');
  });

  it('says when a window opens, in browser time, and falls back without one', () => {
    expect(keepStateLabel('waiting_for_window', 0, new Date(2026, 9, 18, 1, 0).toISOString())).toBe('Waits until 01:00');
    expect(keepStateLabel('waiting_for_window', 0, null)).toBe('Waits for its window');
  });

  it('maps every download state to a status key', () => {
    for (const status of Object.values(DOWNLOAD_STATE_STATUS)) expect(STATUS[status]).toBeDefined();
  });
});

describe('keepExpiryLabel', () => {
  it.each([
    [{ until_watched: true, expires_at: null }, 'Kept until watched'],
    [{ until_watched: false, expires_at: at(10, 16) }, 'Kept until Nov 16'],
    [{ until_watched: true, expires_at: at(10, 16) }, 'Kept until watched · by Nov 16'],
    [{ until_watched: false, expires_at: null }, 'Kept'],
    [{ until_watched: true, expires_at: new Date(2027, 0, 3).toISOString() }, 'Kept until watched · by Jan 3, 2027'],
  ])('%o reads %s', (fields, expected) => {
    expect(keepExpiryLabel(keep(fields), NOW)).toBe(expected);
  });

  it('names a smart pick as one', () => {
    expect(keepExpiryLabel(keep({ reason: 'smart', until_watched: true, expires_at: at(9, 31) }), NOW)).toBe('Smart pick · until watched · by Oct 31');
  });
});

describe('keep state rules', () => {
  it('treats a missing keep (the backend before #204) as none', () => {
    expect(keepOf(undefined)).toBeNull();
    expect(keepOf(null)).toBeNull();
    expect(keepRowPill(undefined)).toBeNull();
  });

  it('shows progress only while it still wants bytes', () => {
    expect(keepWantsBytes('downloading')).toBe(true);
    expect(keepWantsBytes('waiting_for_window')).toBe(true);
    expect(keepWantsBytes('paused')).toBe(true);
    expect(keepWantsBytes('done')).toBe(false);
    expect(keepWantsBytes('no_file')).toBe(false);
    expect(keepWantsBytes('unavailable')).toBe(false);
  });

  it('gives an episode row the reason, unless the file is not really held', () => {
    expect(keepRowPill(keep())).toEqual({ status: 'kept', label: 'Kept' });
    expect(keepRowPill(keep({ reason: 'smart', state: 'done' }))).toEqual({ status: 'smart', label: 'Smart' });
    expect(keepRowPill(keep({ state: 'unavailable' }))).toEqual({ status: 'not_keepable', label: 'Can’t keep' });
    expect(keepRowPill(keep({ state: 'storage_full' }))).toEqual({ status: 'storage_full', label: 'Kept space is full' });
    expect(keepRowPill(keep({ state: 'disk_full' }))).toEqual({ status: 'disk_full', label: 'Disk full' });
    expect(keepRowPill(keep({ state: 'no_file' }))).toBeNull();
  });

  it('changes the duration of kept downloads only', () => {
    expect(canChangeDuration(keep())).toBe(true);
    expect(canChangeDuration(keep({ reason: 'smart' }))).toBe(false);
  });
});

describe('the Keep sheet’s choices', () => {
  it('lists the spec’s options, with until watched first and 30 days at most', () => {
    expect(keepOptions(undefined, NOW)).toEqual([
      { value: 'until_watched', label: 'Until I watch it', detail: '30 days at most' },
      { value: 'days_7', label: '7 days', detail: 'Until Oct 24' },
      { value: 'days_30', label: '30 days', detail: 'Until Nov 16' },
      { value: 'days_90', label: '90 days', detail: 'Until Jan 15, 2027' },
      { value: 'until_removed', label: 'Until I remove it', detail: 'No time limit' },
    ]);
  });

  it('starts on the storage settings’ default', () => {
    expect(defaultKeepChoice()).toBe('until_watched');
    expect(defaultKeepChoice({ default_until_watched: true, default_days: null })).toBe('until_watched');
    expect(defaultKeepChoice({ default_until_watched: false, default_days: 7 })).toBe('days_7');
    expect(defaultKeepChoice({ default_until_watched: false, default_days: null })).toBe('until_removed');
  });

  it('takes "until watched"’s maximum from the defaults', () => {
    expect(keepOptions({ default_until_watched: true, default_days: 90 }, NOW)[0].detail).toBe('90 days at most');
    expect(keepOptions({ default_until_watched: true, default_days: null }, NOW)[0].detail).toBe('No time limit');
    // A default that isn't until watched leaves the spec's 30 days
    expect(keepOptions({ default_until_watched: false, default_days: 7 }, NOW)[0].detail).toBe('30 days at most');
  });

  it('turns a choice into a duration from now', () => {
    const in30 = new Date(NOW.getTime() + 30 * 86_400_000).toISOString();
    expect(keepDuration('until_watched', undefined, NOW)).toEqual({ until_watched: true, expires_at: in30 });
    expect(keepDuration('until_watched', { default_until_watched: true, default_days: null }, NOW)).toEqual({ until_watched: true, expires_at: null });
    expect(keepDuration('days_30', undefined, NOW)).toEqual({ until_watched: false, expires_at: in30 });
    expect(keepDuration('until_removed', undefined, NOW)).toEqual({ until_watched: false, expires_at: null });
  });

  it('leaves the default to the server on POST, and sends anything else', () => {
    expect(keepCreateDuration('until_watched', null, NOW)).toEqual({});
    expect(keepCreateDuration('days_7', { default_until_watched: false, default_days: 7 }, NOW)).toEqual({});
    expect(keepCreateDuration('until_removed', null, NOW)).toEqual({ until_watched: false, expires_at: null });
  });
});

describe('estimateCopy', () => {
  const estimate = { wanted_bytes: 14_200_000_000, needed_bytes: 14_200_000_000, available_bytes: 61_000_000_000, downloadable: true };

  it('says what it needs against what is left', () => {
    expect(estimateCopy(estimate)).toEqual({ kind: 'fits', text: 'Needs 14.2 GB · 61.0 GB left for kept downloads' });
  });

  it('says when another keep already has the files, or nothing is assigned yet', () => {
    expect(estimateCopy({ ...estimate, needed_bytes: 0 })).toEqual({ kind: 'fits', text: 'Needs no more space: another keep has these files · 61.0 GB left for kept downloads' });
    expect(estimateCopy({ ...estimate, wanted_bytes: 0, needed_bytes: 0 })).toEqual({
      kind: 'fits',
      text: 'Nothing to download yet: it starts once there’s a file · 61.0 GB left for kept downloads',
    });
  });

  it('says it doesn’t fit before the Keep is sent, with the 409’s numbers', () => {
    expect(estimateCopy({ ...estimate, available_bytes: 1_200_000_000 })).toEqual({ kind: 'full', needed: 14_200_000_000, available: 1_200_000_000 });
    expect(storageFullLead(14_200_000_000, 1_200_000_000)).toBe('Not enough space: needs 14.2 GB, 1.2 GB left.');
    expect(storageFullLead(1_000, -5)).toBe('Not enough space: needs 1.0 KB, 0 B left.');
  });

  it('says an HDRezka stream can’t be kept', () => {
    expect(estimateCopy({ ...estimate, downloadable: false })).toEqual({ kind: 'not_downloadable' });
  });
});
