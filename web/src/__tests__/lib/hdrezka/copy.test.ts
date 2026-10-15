import { ApiError } from '@/lib/api/client';
import { hdrezkaLink, hdrezkaProblem, replaceEpisodesConfirm, replaceFileConfirm, searchNameFor, shorterQuery } from '@/lib/hdrezka/copy';
import { clearDraft, loadDraft, saveDraft } from '@/lib/hdrezka/session';

describe('hdrezkaProblem', () => {
  it.each([
    [503, 'hdrezka_blocked', 'HDRezka is showing its bot check', true],
    [502, 'hdrezka_unavailable', 'HDRezka isn’t answering', true],
    [504, 'timeout', 'HDRezka took too long', true],
    [503, 'unavailable', 'HDRezka is off', false],
    [409, 'premium_only', 'This needs HDRezka Premium', false],
    [404, 'not_found', 'HDRezka has no such page', false],
  ])('%s %s: “%s”, retry %s, with the backend’s message', (status, code, title, retry) => {
    expect(hdrezkaProblem(new ApiError(status, code, 'The backend says why.'))).toEqual({ title, body: 'The backend says why.', retry });
  });

  it('falls back to its own words without a message', () => {
    expect(hdrezkaProblem(new ApiError(500, 'internal_error', ''))).toMatchObject({ title: 'Couldn’t reach HDRezka', retry: true });
  });
});

describe('hdrezkaLink', () => {
  it.each([
    ['https://rezka.ag/films/fiction/2259-interstellar-2014.html', 'https://rezka.ag/films/fiction/2259-interstellar-2014.html'],
    ['  http://hdrezka.me/series/thriller/47283-razdelenie-2022.html#t:111-s:1-e:1 ', 'http://hdrezka.me/series/thriller/47283-razdelenie-2022.html'],
    ['rezka.ag/animation/adventures/61478-friren-2023.html', 'https://rezka.ag/animation/adventures/61478-friren-2023.html'],
    ['/films/fiction/2259-interstellar-2014.html', '/films/fiction/2259-interstellar-2014.html'],
  ])('reads %s as a title page', (text, link) => {
    expect(hdrezkaLink(text)).toBe(link);
  });

  it.each(['Interstellar 2014', 'https://rezka.ag/', 'https://example.com/page.html'])('treats %s as a search', (text) => {
    expect(hdrezkaLink(text)).toBeNull();
  });
});

describe('searchNameFor', () => {
  it.each([
    [{ title: 'Interstellar', original_title: null }, 'Interstellar'],
    [{ title: 'Amélie', original_title: 'Le Fabuleux Destin d’Amélie Poulain' }, 'Le Fabuleux Destin d’Amélie Poulain'],
    [{ title: 'Brother', original_title: 'Брат' }, 'Брат'],
    [{ title: 'Frieren: Beyond Journey’s End', original_title: '葬送のフリーレン' }, 'Frieren: Beyond Journey’s End'],
  ])('%o → %s', (title, query) => {
    expect(searchNameFor(title)).toBe(query);
  });
});

describe('shorterQuery', () => {
  it.each([
    ['Interstellar 2014', 'Interstellar'],
    ['Dune: Part Two', 'Dune'],
    ['One Battle After Another', 'One Battle After'],
    ['Frieren', null],
  ])('%s → %s', (query, shorter) => {
    expect(shorterQuery(query)).toBe(shorter);
  });
});

describe('replaceFileConfirm', () => {
  it('warns that a torrent’s file becomes a new .mp4 in Infuse', () => {
    const words = replaceFileConfirm('Dune: Part Two', { torrent: true, stream: null }, { translator: 'Дубляж', quality: '1080p' });
    expect(words.title).toBe('Stream Dune: Part Two from HDRezka?');
    expect(words.description).toMatch(/new file \(it ends in \.mp4\), so Infuse may forget where you stopped/);
  });

  it('names the old and new voice-over when a stream changes', () => {
    const words = replaceFileConfirm('Dune: Part Two', { torrent: false, stream: { translator: 'Дубляж', quality: '720p' } }, { translator: 'Оригинал (+субтитры)', quality: '1080p' });
    expect(words).toMatchObject({
      title: 'Switch Dune: Part Two to Оригинал (+субтитры) in 1080p?',
      description: 'Infuse plays it from now on, instead of Дубляж in 720p.',
    });
  });
});

describe('replaceEpisodesConfirm', () => {
  it('counts the episodes that have a file, and warns about new .mp4 files', () => {
    const words = replaceEpisodesConfirm('Severance', { count: 9, withFile: 4, code: null });
    expect(words).toMatchObject({ title: 'Stream 9 episodes of Severance from HDRezka?', confirmLabel: 'Stream 9 episodes' });
    expect(words.description).toMatch(/^4 of them already have a file\. Infuse plays the HDRezka streams from now on\./);
    expect(words.description).toMatch(/new file \(it ends in \.mp4\), so Infuse may forget where you stopped/);
  });

  it('names a single episode', () => {
    expect(replaceEpisodesConfirm('Severance', { count: 1, withFile: 1, code: 'S02E05' })).toMatchObject({
      title: 'Stream S02E05 of Severance from HDRezka?',
      confirmLabel: 'Stream it',
    });
    expect(replaceEpisodesConfirm('Severance', { count: 2, withFile: 2, code: null }).description).toMatch(/^They all have a file already\./);
  });
});

describe('session drafts', () => {
  it('keeps a draft per context until it’s cleared', () => {
    const draft = { input: 'Dune', query: 'Dune', pick: null, translator: null, quality: null };
    saveDraft('movie:1', draft);
    expect(loadDraft('movie:1')).toEqual(draft);
    expect(loadDraft('movie:2')).toBeNull();
    clearDraft('movie:1');
    expect(loadDraft('movie:1')).toBeNull();
  });
});
