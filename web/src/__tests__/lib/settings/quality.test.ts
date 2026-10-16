import type { QualityProfile } from '@/lib/api/contract';
import {
  addTag,
  maxAllowed,
  qualityPreview,
  qualitySummary,
  sameProfile,
  sizeExample,
  validateQualityProfile,
  withPreferredResolution,
} from '@/lib/settings/quality';
import { QUALITY_PROFILE } from '@/mocks/fixtures/settings';

const profile = (patch: Partial<QualityProfile> = {}): QualityProfile => ({ ...QUALITY_PROFILE, ...patch });

describe('qualityPreview', () => {
  it('reads like the design: resolution, DV and the size limit, then codec, groups and avoided terms', () => {
    expect(qualityPreview(profile())).toEqual({
      main: '2160p · DV · ≤ 8 GB/h',
      sub: 'x265 when there’s a choice · from FLUX or NTb +2 · never CAM, TS, HDCAM +3',
    });
  });

  it('names HDR when Dolby Vision is off, and neither when both are', () => {
    expect(qualityPreview(profile({ prefer_dolby_vision: false })).main).toBe('2160p · HDR · ≤ 8 GB/h');
    expect(qualityPreview(profile({ prefer_dolby_vision: false, prefer_hdr: false, max_gb_per_hour: 4.5 })).main).toBe('2160p · ≤ 4.5 GB/h');
  });

  it('never claims a source the profile can’t express', () => {
    expect(qualityPreview(profile()).main).not.toMatch(/WEB|BluRay/);
  });

  it('leaves out what’s empty, and counts only what doesn’t fit', () => {
    expect(qualityPreview(profile({ prefer_x265: false, preferred_groups: [] })).sub).toBe('never CAM, TS, HDCAM +3');
    expect(qualityPreview(profile({ preferred_groups: ['FLUX'], avoid_terms: ['CAM', 'TS'] })).sub).toBe('x265 when there’s a choice · from FLUX · never CAM, TS');
    expect(qualityPreview(profile({ prefer_x265: false, preferred_groups: [], avoid_terms: [] })).sub).toBe('');
  });
});

describe('qualitySummary and sizeExample', () => {
  it('summarises the nav line', () => {
    expect(qualitySummary(profile())).toBe('2160p preferred · ≤ 8 GB/h');
    expect(qualitySummary(profile({ preferred_resolution: '1080p', max_gb_per_hour: 12.5 }))).toBe('1080p preferred · ≤ 12.5 GB/h');
  });

  it('works out the example film’s size, rounder when it’s big', () => {
    expect(sizeExample(8)).toBe('8 GB/h ≈ 22 GB for a 2 h 46 m film');
    expect(sizeExample(1)).toBe('1 GB/h ≈ 2.8 GB for a 2 h 46 m film');
    expect(sizeExample(20)).toBe('20 GB/h ≈ 55 GB for a 2 h 46 m film');
  });
});

describe('resolutions', () => {
  it('never lets the maximum go below the preferred resolution', () => {
    expect(maxAllowed('1080p', '2160p')).toBe(false);
    expect(maxAllowed('1080p', '1080p')).toBe(true);
    expect(maxAllowed(null, '2160p')).toBe(true);
  });

  it('raises the maximum with a higher preferred resolution, and leaves it alone otherwise', () => {
    expect(withPreferredResolution(profile({ preferred_resolution: '1080p', max_resolution: '1080p' }), '2160p')).toMatchObject({
      preferred_resolution: '2160p',
      max_resolution: '2160p',
    });
    expect(withPreferredResolution(profile({ max_resolution: null }), '720p')).toMatchObject({ preferred_resolution: '720p', max_resolution: null });
    expect(withPreferredResolution(profile({ max_resolution: '2160p' }), '1080p').max_resolution).toBe('2160p');
  });
});

describe('addTag', () => {
  it('adds what was typed, trimmed', () => {
    expect(addTag(['CAM'], '  HDTS ', 'terms')).toEqual({ ok: true, list: ['CAM', 'HDTS'] });
  });

  it('ignores an empty entry', () => {
    const list = ['CAM'];
    expect(addTag(list, '   ', 'terms')).toEqual({ ok: true, list });
  });

  it('refuses a duplicate in any case, naming the one already there', () => {
    expect(addTag(['CAM'], 'cam', 'terms')).toEqual({ ok: false, error: 'CAM is already on the list.' });
  });

  it('refuses more than 30 characters and more than 30 entries', () => {
    expect(addTag([], 'x'.repeat(31), 'terms')).toEqual({ ok: false, error: 'Keep it to 30 characters or fewer.' });
    expect(addTag([], 'x'.repeat(30), 'terms').ok).toBe(true);
    const full = Array.from({ length: 30 }, (_, i) => `g${i}`);
    expect(addTag(full, 'FLUX', 'groups')).toEqual({ ok: false, error: 'That’s 30 groups already. Remove one to add another.' });
  });
});

describe('validateQualityProfile', () => {
  it('accepts the fixture and the defaults', () => {
    expect(validateQualityProfile(profile())).toBeNull();
    expect(validateQualityProfile(profile({ preferred_resolution: '1080p', max_resolution: null, avoid_terms: [], preferred_groups: [] }))).toBeNull();
  });

  it('names the field that’s wrong', () => {
    expect(validateQualityProfile(profile({ max_resolution: '1080p' }))?.field).toBe('max_resolution');
    expect(validateQualityProfile(profile({ max_gb_per_hour: 20.5 }))?.field).toBe('max_gb_per_hour');
    expect(validateQualityProfile(profile({ max_gb_per_hour: 7.25 }))?.field).toBe('max_gb_per_hour');
    expect(validateQualityProfile(profile({ avoid_terms: [''] }))?.field).toBe('avoid_terms');
    expect(validateQualityProfile(profile({ preferred_groups: Array.from({ length: 31 }, (_, i) => `g${i}`) }))?.field).toBe('preferred_groups');
  });
});

describe('sameProfile', () => {
  it('compares every field, lists in order', () => {
    expect(sameProfile(profile(), profile({ avoid_terms: [...QUALITY_PROFILE.avoid_terms] }))).toBe(true);
    expect(sameProfile(profile(), profile({ max_gb_per_hour: 8.5 }))).toBe(false);
    expect(sameProfile(profile(), profile({ avoid_terms: [...QUALITY_PROFILE.avoid_terms].reverse() }))).toBe(false);
  });
});
