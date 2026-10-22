import {
  addRecentSearch,
  clearRecentSearches,
  MAX_RECENT_SEARCHES,
  RECENT_SEARCHES_KEY,
  recentSearchesKey,
  recentSearches,
  removeRecentSearch,
  resetRecentSearchesCache,
  subscribeRecentSearches,
} from '@/lib/title-search/recent';

beforeEach(() => {
  window.localStorage.clear();
  resetRecentSearchesCache();
  jest.restoreAllMocks();
});

const stored = () => JSON.parse(window.localStorage.getItem(RECENT_SEARCHES_KEY) ?? 'null') as unknown;

describe('recent searches', () => {
  it('keeps queries newest first, once whatever their case, cleaned', () => {
    addRecentSearch('frieren');
    addRecentSearch('  past   lives ');
    addRecentSearch('FRIEREN');
    expect(recentSearches()).toEqual(['FRIEREN', 'past lives']);
    expect(stored()).toEqual(['FRIEREN', 'past lives']);
  });

  it('ignores blank and over-long queries', () => {
    addRecentSearch('   ');
    addRecentSearch('x'.repeat(101));
    expect(recentSearches()).toEqual([]);
  });

  it(`keeps the latest ${MAX_RECENT_SEARCHES}`, () => {
    for (let i = 1; i <= MAX_RECENT_SEARCHES + 2; i++) addRecentSearch(`query ${i}`);
    expect(recentSearches()).toHaveLength(MAX_RECENT_SEARCHES);
    expect(recentSearches()[0]).toBe(`query ${MAX_RECENT_SEARCHES + 2}`);
  });

  it('forgets one, or all', () => {
    addRecentSearch('frieren');
    addRecentSearch('severance s02');
    removeRecentSearch('frieren');
    expect(recentSearches()).toEqual(['severance s02']);
    clearRecentSearches();
    expect(recentSearches()).toEqual([]);
    expect(stored()).toEqual([]);
  });

  it('returns the same list until it changes', () => {
    addRecentSearch('frieren');
    expect(recentSearches()).toBe(recentSearches());
  });

  it('tells subscribers, and hears other tabs', () => {
    const listener = jest.fn();
    const unsubscribe = subscribeRecentSearches(listener);
    addRecentSearch('frieren');
    expect(listener).toHaveBeenCalledTimes(1);

    window.localStorage.setItem(RECENT_SEARCHES_KEY, JSON.stringify(['from another tab']));
    window.dispatchEvent(new StorageEvent('storage', { key: RECENT_SEARCHES_KEY }));
    expect(listener).toHaveBeenCalledTimes(2);
    expect(recentSearches()).toEqual(['from another tab']);

    unsubscribe();
    addRecentSearch('dune');
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it('reads nothing from a corrupt or odd entry', () => {
    window.localStorage.setItem(RECENT_SEARCHES_KEY, '{not json');
    expect(recentSearches()).toEqual([]);
    resetRecentSearchesCache();
    window.localStorage.setItem(RECENT_SEARCHES_KEY, JSON.stringify(['ok', 3, '', null, 'fine']));
    expect(recentSearches()).toEqual(['ok', 'fine']);
  });

  it('survives storage that throws (private windows, blocked site data)', () => {
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('full', 'QuotaExceededError');
    });
    expect(recentSearches()).toEqual([]);
    expect(() => addRecentSearch('frieren')).not.toThrow();
    // Not kept, but the page's list still updates
    expect(recentSearches()).toEqual(['frieren']);
    expect(() => clearRecentSearches()).not.toThrow();
  });
});

describe('recent searches per household member (#239)', () => {
  afterEach(() => {
    document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
  });

  it('each member keeps their own list, under a key with their id', () => {
    document.cookie = 'momoshtrem_member=2; Path=/';
    expect(recentSearchesKey()).toBe('momoshtrem:recent-searches:2');
    addRecentSearch('the bear');
    expect(JSON.parse(window.localStorage.getItem('momoshtrem:recent-searches:2') ?? 'null')).toEqual(['the bear']);
    expect(window.localStorage.getItem(RECENT_SEARCHES_KEY)).toBeNull();

    // Switching reloads the page, which starts a fresh cache
    document.cookie = 'momoshtrem_member=1; Path=/';
    resetRecentSearchesCache();
    expect(recentSearches()).toEqual([]);
  });

  it('never hands the old shared list to a member; it’s read only without one', () => {
    window.localStorage.setItem(RECENT_SEARCHES_KEY, JSON.stringify(['from before households']));
    document.cookie = 'momoshtrem_member=1; Path=/';
    expect(recentSearches()).toEqual([]);
    document.cookie = 'momoshtrem_member=; Path=/; Max-Age=0';
    resetRecentSearchesCache();
    expect(recentSearchesKey()).toBe(RECENT_SEARCHES_KEY);
    expect(recentSearches()).toEqual(['from before households']);
  });

  it('hears other tabs only for this member’s key', () => {
    document.cookie = 'momoshtrem_member=1; Path=/';
    const listener = jest.fn();
    const unsubscribe = subscribeRecentSearches(listener);
    window.dispatchEvent(new StorageEvent('storage', { key: 'momoshtrem:recent-searches:2' }));
    expect(listener).not.toHaveBeenCalled();
    window.dispatchEvent(new StorageEvent('storage', { key: 'momoshtrem:recent-searches:1' }));
    expect(listener).toHaveBeenCalledTimes(1);
    unsubscribe();
  });
});
