import { cleanQuery, fold, hasEveryWord, highlight, normalize, queryWords } from '@/lib/title-search/text';

const marked = (text: string, query: string) =>
  highlight(text, query)
    .map((segment) => (segment.match ? `[${segment.text}]` : segment.text))
    .join('');

describe('folding and normalising', () => {
  it('drops accents and case, one character for one', () => {
    expect(fold('Shōgun')).toBe('shogun');
    expect(fold('Amélie')).toHaveLength('Amélie'.length);
  });

  it('drops apostrophes and turns other punctuation into single spaces', () => {
    expect(normalize('Frieren: Beyond Journey’s End')).toBe('frieren beyond journeys end');
    expect(normalize('  Paris,   Texas! ')).toBe('paris texas');
    expect(normalize('葬送のフリーレン')).toBe('葬送のフリーレン');
  });

  it('splits a query into words, none when blank', () => {
    expect(queryWords('Severance  S02')).toEqual(['severance', 's02']);
    expect(queryWords('  ')).toEqual([]);
  });

  it('finds every word anywhere', () => {
    expect(hasEveryWord('Go to Library 11 titles', 'lib tit')).toBe(true);
    expect(hasEveryWord('Go to Library', 'lib discover')).toBe(false);
    expect(hasEveryWord('Go to Library', '')).toBe(false);
  });

  it('cleans a query as the route takes it', () => {
    expect(cleanQuery('  sousou   no frieren ')).toBe('sousou no frieren');
    expect(cleanQuery('   ')).toBeNull();
    expect(cleanQuery(null)).toBeNull();
    expect(cleanQuery('x'.repeat(101))).toBeNull();
    expect(cleanQuery('x'.repeat(100))).toHaveLength(100);
  });
});

describe('highlight', () => {
  it('marks the whole query where it starts a word', () => {
    expect(marked('Severance', 'sev')).toBe('[Sev]erance');
    expect(marked('Find a torrent for Severance S02E06', 'sev')).toBe('Find a torrent for [Sev]erance S02E06');
    expect(marked('Jodorowsky’s Dune', 'dune')).toBe('Jodorowsky’s [Dune]');
  });

  it('prefers a word start to the middle of a word', () => {
    expect(marked('Unsevered Seven', 'sev')).toBe('Unsevered [Sev]en');
    expect(marked('Unsevered', 'sev')).toBe('Un[sev]ered');
  });

  it('ignores accents and case, and keeps the original characters', () => {
    expect(marked('Shōgun', 'shogun')).toBe('[Shōgun]');
    expect(marked('AMÉLIE', 'ame')).toBe('[AMÉ]LIE');
  });

  it('marks each word when the whole query isn’t there', () => {
    expect(marked('Dune: Prophecy', 'dune prophecy')).toBe('[Dune]: [Prophecy]');
    expect(marked('Frieren: Beyond Journey’s End', 'journey frieren')).toBe('[Frieren]: Beyond [Journey]’s End');
  });

  it('leaves text alone when nothing matches or the query is blank', () => {
    expect(highlight('Interstellar', 'dune')).toEqual([{ text: 'Interstellar', match: false }]);
    expect(highlight('Interstellar', '  ')).toEqual([{ text: 'Interstellar', match: false }]);
  });

  it('merges overlapping words', () => {
    expect(marked('Better Call Saul', 'call ca')).toBe('Better [Call] Saul');
  });
});
