import { keepActions } from '@/components/title/TitleMoreMenu';
import type { Keep, MovieTitle, ShowTitle, Title } from '@/lib/api/contract';
import { assignMovieStream, buildTitle, libraryKey, seedLibrary } from '@/mocks/fixtures/titles';

/** The ⋯ menu's Download & keep items (#206): what each title gets, and none without downloads. */

function title(mediaType: 'movie' | 'tv', tmdbId: number, library = seedLibrary()): Title {
  const built = buildTitle(mediaType, tmdbId, library);
  if (!built) throw new Error(`No fixture for ${mediaType} ${tmdbId}`);
  return built;
}
const labels = (t: Title) => keepActions(t, () => {}, () => {}).map((action) => action.label);
const withKeep = <T extends Title>(t: T, keep: Keep | null | undefined): T => {
  const library = { ...t.library, keep } as T['library'];
  if (keep === undefined) delete (library as Partial<T['library']>).keep;
  return { ...t, library };
};

describe('keepActions', () => {
  it('offers Download & keep on a movie or a show nothing keeps', () => {
    expect(labels(title('movie', 693134))).toEqual(['Download & keep…']);
    expect(labels(title('tv', 95396) as ShowTitle)).toEqual(['Download & keep show…']);
  });

  it('offers Change duration and Stop keeping once it’s kept', () => {
    expect(labels(title('movie', 157336))).toEqual(['Change duration…', 'Stop keeping…']);
  });

  it('offers no new Keep for a movie that streams from HDRezka (it would be refused)', () => {
    const library = seedLibrary();
    const row = library.get(libraryKey('movie', 693134));
    if (!row) throw new Error('No Dune');
    assignMovieStream(row, { title: 'Дюна: Часть вторая', url: 'https://rezka.ag/films/fiction/1-dune.html', translator: 'Дубляж', quality: '1080p', size_bytes: 4_000_000_000 });
    expect(labels(title('movie', 693134, library))).toEqual([]);
  });

  it('offers nothing on a backend without downloads (no `keep` field), or outside the library', () => {
    expect(labels(withKeep(title('movie', 693134) as MovieTitle, undefined))).toEqual([]);
    expect(labels(withKeep(title('tv', 95396) as ShowTitle, undefined))).toEqual([]);
    expect(labels(title('movie', 666277))).toEqual([]);
  });
});
