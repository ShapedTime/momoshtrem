import { pollWhileDownloading } from '@/hooks/useTitle';
import type { Title } from '@/lib/api/contract';
import { buildTitle } from '@/mocks/fixtures/titles';

function title(mediaType: 'movie' | 'tv', tmdbId: number): Title {
  const built = buildTitle(mediaType, tmdbId);
  if (!built) throw new Error(`No fixture for ${mediaType} ${tmdbId}`);
  return built;
}

describe('pollWhileDownloading', () => {
  it('polls every 5 s while a movie or show downloads', () => {
    expect(pollWhileDownloading(title('movie', 157336))).toBe(5000);
    expect(pollWhileDownloading(title('tv', 247767))).toBe(5000);
  });

  it('stops once the stage moves on, and before anything loaded', () => {
    const interstellar = title('movie', 157336);
    const ready: Title = { ...interstellar, library: { ...interstellar.library, availability_stage: 'ready', download: null } };
    expect(pollWhileDownloading(ready)).toBe(false);
    expect(pollWhileDownloading(title('movie', 693134))).toBe(false);
    expect(pollWhileDownloading(title('movie', 666277))).toBe(false);
    expect(pollWhileDownloading(undefined)).toBe(false);
  });
});
