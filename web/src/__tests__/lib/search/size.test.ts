import { positionOf, rangeText, SIZE_SCALE, SIZE_STEPS, sizeAt, sizeLabel, sizeRange, withSizeRange } from '@/lib/search/size';

const tv = SIZE_SCALE.tv;
const movie = SIZE_SCALE.movie;

describe('the size scale', () => {
  it('runs from 100 MB to 20 GB per episode, and 500 MB to 100 GB for a movie', () => {
    expect([sizeAt(0, tv), sizeAt(SIZE_STEPS, tv)]).toEqual([0.1, 20]);
    expect([sizeAt(0, movie), sizeAt(SIZE_STEPS, movie)]).toEqual([0.5, 100]);
  });

  it('is logarithmic: the middle is the geometric mean', () => {
    expect(sizeAt(50, tv)).toBe(1.4);
    expect(sizeAt(50, movie)).toBe(7.1);
  });

  it('rounds to two significant figures, and finds a size’s position back', () => {
    for (let position = 0; position <= SIZE_STEPS; position += 7) {
      const gb = sizeAt(position, tv);
      expect(String(gb).replace(/^0\.0*|\./g, '').length).toBeLessThanOrEqual(2);
      expect(Math.abs(positionOf(gb, tv) - position)).toBeLessThanOrEqual(1);
    }
  });

  it('clamps sizes off the track to its ends', () => {
    expect(positionOf(0.01, tv)).toBe(0);
    expect(positionOf(500, tv)).toBe(SIZE_STEPS);
  });
});

describe('sizeLabel', () => {
  it('says MB under a gigabyte, and drops a trailing .0', () => {
    expect(sizeLabel(0.2)).toBe('200 MB');
    expect(sizeLabel(0.11)).toBe('110 MB');
    expect(sizeLabel(4)).toBe('4 GB');
    expect(sizeLabel(4.1)).toBe('4.1 GB');
    expect(sizeLabel(20)).toBe('20 GB');
  });
});

describe('filters and the track', () => {
  it('puts a missing bound at its end, and drops a bound whose thumb is at its end', () => {
    expect(sizeRange({}, tv)).toEqual([0, SIZE_STEPS]);
    const filters = withSizeRange({ resolution: ['1080p' as const] }, [30, SIZE_STEPS], tv);
    expect(filters).toEqual({ resolution: ['1080p'], min_size_gb: sizeAt(30, tv) });
    expect(sizeRange(filters, tv)).toEqual([30, SIZE_STEPS]);
    expect(withSizeRange({ min_size_gb: 1, max_size_gb: 4 }, [0, SIZE_STEPS], tv)).toEqual({});
  });

  it('reads each thumb, or "No minimum" and "No maximum" at the ends', () => {
    expect(rangeText(0, 'min', tv)).toBe('No minimum');
    expect(rangeText(SIZE_STEPS, 'max', tv)).toBe('No maximum');
    expect(rangeText(50, 'min', tv)).toBe('1.4 GB');
    expect(rangeText(0, 'max', tv)).toBe('100 MB');
  });
});
