import { activeFilterCount } from '@/lib/search/filters';
import { clearDraft, freshDraft, loadDraft, saveDraft } from '@/lib/search/session';

describe('the search draft', () => {
  afterEach(() => clearDraft('show:209867:1:29'));

  it('starts untouched: no typed query, the context’s scope, best first, no filters', () => {
    expect(freshDraft()).toEqual({ query: '', dirty: false, scope: null, sort: 'best', filters: {} });
  });

  it('keeps one draft per context until cleared', () => {
    expect(loadDraft('show:209867:1:29')).toBeNull();
    const draft = { ...freshDraft(), query: 'Frieren 29 1080p', dirty: true, scope: 'season' as const, filters: { resolution: ['1080p' as const] } };
    saveDraft('show:209867:1:29', draft);
    expect(loadDraft('show:209867:1:29')).toEqual(draft);
    expect(loadDraft('show:209867:1:30')).toBeNull();
    clearDraft('show:209867:1:29');
    expect(loadDraft('show:209867:1:29')).toBeNull();
  });
});

describe('activeFilterCount', () => {
  it('counts each selected value, the seeders floor and each size bound', () => {
    expect(activeFilterCount({})).toBe(0);
    expect(activeFilterCount({ resolution: ['2160p', '1080p'], hdr: ['DV'], min_seeders: 5, max_size_gb: 4 })).toBe(5);
  });
});
