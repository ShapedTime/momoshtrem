import type { HDRezkaCandidate, HDRezkaQuality, HDRezkaTitle, HDRezkaTranslator } from '@/lib/api/contract';
import {
  bestCandidate,
  candidateFacts,
  categoryLabel,
  defaultQuality,
  defaultTranslator,
  matchReasons,
  qualityNote,
  subtitlesLine,
  translatorBlocked,
  translatorChips,
  translatorCut,
  translatorKey,
  weakMatchNote,
} from '@/lib/hdrezka/pick';

const hit = (patch: Partial<HDRezkaCandidate>): HDRezkaCandidate => ({
  url: 'https://rezka.ag/films/fiction/2259-interstellar-2014.html',
  path: '/films/fiction/2259-interstellar-2014.html',
  title: 'Интерстеллар',
  original_title: 'Interstellar',
  year: 2014,
  kind: 'movie',
  category: 'films',
  rating: 8.6,
  match: 'exact',
  ...patch,
});

const voice = (id: number, name: string, flags: Partial<HDRezkaTranslator> = {}): HDRezkaTranslator => ({
  id,
  name,
  default: false,
  premium: false,
  camrip: false,
  ads: false,
  director: false,
  ...flags,
});

const quality = (label: string, resolution: number, patch: Partial<HDRezkaQuality> = {}): HDRezkaQuality => ({
  label,
  resolution,
  premium: false,
  available: true,
  size_bytes: resolution * 4_000_000,
  ...patch,
});

const interstellar = { media_type: 'movie' as const, title: 'Interstellar', original_title: null, year: 2014 };

describe('bestCandidate', () => {
  it('takes the first exact hit, else the first likely one, never a weak one', () => {
    const likely = hit({ match: 'likely', title: 'Наука «Интерстеллара»' });
    const exact = hit({});
    expect(bestCandidate([likely, exact])).toBe(exact);
    expect(bestCandidate([hit({ match: 'weak' }), likely])).toBe(likely);
    expect(bestCandidate([hit({ match: 'weak' })])).toBeNull();
    expect(bestCandidate([])).toBeNull();
  });
});

describe('matchReasons', () => {
  it('says why the best match is: "Exact title · 2014 · Film · Rated 8.6"', () => {
    expect(matchReasons(hit({}), interstellar)).toEqual(['Exact title', '2014', 'Film', 'Rated 8.6']);
  });

  it('matches on either name, ignoring case and punctuation', () => {
    const dune = { media_type: 'movie' as const, title: 'Dune: Part Two', original_title: null, year: 2024 };
    expect(matchReasons(hit({ title: 'Дюна: Часть вторая', original_title: 'Dune Part Two', year: 2024, rating: null }), dune)).toEqual([
      'Exact title',
      '2024',
      'Film',
    ]);
  });

  it('calls out a similar title and a year that’s off by more than one', () => {
    const reasons = matchReasons(hit({ title: 'Дюна', original_title: 'Dune', year: 2021, match: 'likely', rating: 7.84 }), {
      media_type: 'movie',
      title: 'Dune: Part Two',
      original_title: null,
      year: 2024,
    });
    expect(reasons).toEqual(['Similar title', '2021, not 2024', 'Film', 'Rated 7.8']);
  });

  it('labels anime and series by category, and keeps facts short on plain cards', () => {
    expect(categoryLabel('animation')).toBe('Anime');
    expect(candidateFacts(hit({ category: 'series', year: null, rating: null }))).toEqual(['Series']);
    expect(weakMatchNote({ ...interstellar, media_type: 'tv' })).toBe('Weak match: check it’s the right show');
  });
});

describe('voice-overs', () => {
  const title = (translators: HDRezkaTranslator[], translator_id: number): HDRezkaTitle => ({
    url: 'https://rezka.ag/x.html',
    path: '/films/x/1-x.html',
    post_id: 1,
    kind: 'movie',
    title: 'X',
    original_title: null,
    year: null,
    translators,
    translator_id,
    seasons: [],
  });

  it('tells cuts of one translator apart by their flags', () => {
    expect(translatorKey(voice(56, 'Дубляж'))).toBe('56');
    expect(translatorKey(voice(56, 'Дубляж', { camrip: true, director: true }))).toBe('56-c-d');
  });

  it('shows Premium, CAMRip, Ads and Director’s cut as chips, and why premium can’t be picked', () => {
    expect(translatorChips(voice(1, 'A', { premium: true, ads: true, director: true, camrip: true })).map((c) => c.label)).toEqual([
      'Premium',
      'CAMRip',
      'Ads',
      'Director’s cut',
    ]);
    expect(translatorBlocked(voice(1, 'A', { premium: true }))).toMatch(/Premium account/);
    expect(translatorBlocked(voice(1, 'A'))).toBeNull();
  });

  it('sends a cut’s flags with its id', () => {
    expect(translatorCut(voice(111, 'HDrezka Studio', { director: true }))).toEqual({ camrip: false, ads: false, director: true });
  });

  describe('defaultTranslator: session pick > what streams now > remembered > page default > first without Premium', () => {
    const studio = voice(111, 'HDrezka Studio', { default: true });
    const studioCut = voice(111, 'HDrezka Studio', { director: true });
    const original = voice(238, 'Оригинал (+субтитры)');
    const lostfilm = voice(10, 'LostFilm');
    const premium = voice(1, 'Дубляж (Ultra HD)', { premium: true });
    const all = title([studio, original, lostfilm, premium, studioCut], 111);

    it('keeps what was picked earlier this session, over everything else', () => {
      expect(defaultTranslator(all, { keep: '111-d', current: 'LostFilm', remembered: { id: 238 } })).toBe(studioCut);
    });

    it('then what the title streams now', () => {
      expect(defaultTranslator(all, { current: 'LostFilm', remembered: { id: 238 } })).toBe(lostfilm);
    });

    it('then the voice-over remembered from the last assign, cut and all', () => {
      expect(defaultTranslator(all, { remembered: { id: 238 } })).toBe(original);
      expect(defaultTranslator(all, { remembered: { id: 111, director: true } })).toBe(studioCut);
    });

    it('then the page’s default', () => {
      expect(defaultTranslator(all)).toBe(studio);
      expect(defaultTranslator(all, { remembered: { id: 99 }, current: 'Gone' })).toBe(studio);
      expect(defaultTranslator(title([original, lostfilm], 10))).toBe(lostfilm);
    });

    it('never a premium one, whoever asks for it', () => {
      expect(defaultTranslator(all, { keep: '1', current: 'Дубляж (Ultra HD)', remembered: { id: 1 } })).toBe(studio);
      expect(defaultTranslator(title([premium, original], 1))).toBe(original);
      expect(defaultTranslator(title([premium], 1))).toBeNull();
    });
  });
});

describe('qualities', () => {
  const list = [
    quality('360p', 360),
    quality('720p', 720),
    quality('1080p', 1080),
    quality('1080p Ultra', 1080, { premium: true, size_bytes: null }),
    quality('4K', 2160, { available: false, size_bytes: null }),
  ];

  it('starts on the best one that plays without an account', () => {
    expect(defaultQuality(list)?.label).toBe('1080p');
    expect(defaultQuality(list.slice(0, 2))?.label).toBe('720p');
    expect(defaultQuality([list[3], list[4]])).toBeNull();
  });

  it('keeps the earlier pick while it’s still offered', () => {
    expect(defaultQuality(list, '720p')?.label).toBe('720p');
    expect(defaultQuality(list, '1080p Ultra')?.label).toBe('1080p');
  });

  it('notes the size, or why it can’t be picked', () => {
    expect(list.map((q) => qualityNote(q))).toEqual(['1.4 GB', '2.9 GB', '4.3 GB', 'Premium', 'Not offered']);
    expect(qualityNote(list[2], true)).toBe('4.3 GB an episode');
  });

  it('lists the stream’s subtitles', () => {
    expect(subtitlesLine([{ language_name: 'English' }, { language_name: 'Русский' }])).toBe('Subtitles: English, Русский');
    expect(subtitlesLine([])).toBe('No subtitles with this voice-over');
  });
});
