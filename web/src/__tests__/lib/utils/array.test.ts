import { getRandomItem } from '@/lib/utils/array';

describe('getRandomItem', () => {
  it('returns null for empty array', () => {
    expect(getRandomItem([])).toBeNull();
  });

  it('returns the only item for single-element array', () => {
    expect(getRandomItem(['only'])).toBe('only');
  });

  it('returns an item from the array', () => {
    const items = ['a', 'b', 'c', 'd', 'e'];
    const result = getRandomItem(items);
    expect(items).toContain(result);
  });

  it('works with numbers', () => {
    const numbers = [1, 2, 3, 4, 5];
    const result = getRandomItem(numbers);
    expect(numbers).toContain(result);
  });

  it('works with objects', () => {
    const objects = [{ id: 1 }, { id: 2 }, { id: 3 }];
    const result = getRandomItem(objects);
    expect(objects).toContain(result);
  });

  it('returns different items over multiple calls (probabilistic)', () => {
    const items = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j'];
    const results = new Set<string | null>();

    // Run 100 times - should get at least 2 different results
    for (let i = 0; i < 100; i++) {
      results.add(getRandomItem(items));
    }

    expect(results.size).toBeGreaterThan(1);
  });
});
