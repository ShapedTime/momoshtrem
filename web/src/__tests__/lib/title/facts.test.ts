import type { ShowTitle } from '@/lib/api/contract';
import { titleFacts } from '@/lib/title/facts';
import { buildTitle } from '@/mocks/fixtures/titles';

function frieren(networks: string[]): ShowTitle {
  const title = buildTitle('tv', 209867);
  if (title?.media_type !== 'tv') throw new Error('No Frieren fixture');
  return { ...title, networks };
}

const network = (show: ShowTitle) => titleFacts(show).find((fact) => fact.label.startsWith('Network'));

describe('titleFacts: networks', () => {
  it('names one network', () => {
    expect(network(frieren(['Nippon TV']))).toEqual({ label: 'Network', value: 'Nippon TV' });
  });

  it('names the first of many and counts the rest (anime airs on dozens of stations)', () => {
    const stations = ['Nippon TV', 'YTV', 'FBS', ...Array.from({ length: 27 }, (_, i) => `Station ${i + 1}`)];
    expect(network(frieren(stations))).toEqual({ label: 'Networks', value: 'Nippon TV +29' });
  });

  it('leaves the row out without networks', () => {
    expect(network(frieren([]))).toBeUndefined();
  });
});
