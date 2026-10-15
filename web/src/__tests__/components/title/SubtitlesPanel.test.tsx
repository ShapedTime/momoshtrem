import { fireEvent, render, screen, within } from '@testing-library/react';
import { SubtitlesPanel, type SubtitlesPanelProps } from '@/components/title';
import type { Subtitle } from '@/lib/api/contract';
import { hdrezkaSubtitles, seedSubtitles, subtitleItemKey, subtitleSearch } from '@/mocks/fixtures/subtitles';

const DUNE_RELEASE = 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX';
const duneSubtitles = (): Subtitle[] => seedSubtitles().get(subtitleItemKey('movie', 1)) ?? [];
const azResults = () => subtitleSearch('movie', 1, ['az'], DUNE_RELEASE).items;

function renderPanel(overrides: Partial<SubtitlesPanelProps> = {}) {
  const props: SubtitlesPanelProps = {
    stored: duneSubtitles(),
    storedError: false,
    onRetryStored: jest.fn(),
    onRemove: jest.fn(),
    removingId: null,
    language: 'az',
    onLanguageChange: jest.fn(),
    onSearch: jest.fn(),
    search: { status: 'success', language: 'az', items: azResults() },
    onGet: jest.fn(),
    gettingFileId: null,
    ...overrides,
  };
  render(<SubtitlesPanel {...props} />);
  return props;
}

describe('SubtitlesPanel', () => {
  it('keeps subtitles that came with the torrent and lets downloaded ones go', () => {
    const props = renderPanel();
    const yours = screen.getByRole('list', { name: 'Your subtitles' });
    const rows = within(yours).getAllByRole('listitem');
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining('From the torrent · SRT'),
      expect.stringContaining('OpenSubtitles · Exact release'),
    ]);
    expect(within(rows[0]).queryByRole('button')).not.toBeInTheDocument();
    fireEvent.click(within(rows[1]).getByRole('button', { name: 'Remove Russian subtitles' }));
    expect(props.onRemove).toHaveBeenCalledWith(expect.objectContaining({ language_code: 'ru', source: 'opensubtitles' }));
    expect(screen.getByText('2 languages')).toBeInTheDocument();
  });

  it('labels the subtitles that came with an HDRezka stream and lets them go', () => {
    const stream = { translator: 'Оригинал (+субтитры)', quality: '1080p' };
    const props = renderPanel({ stored: hdrezkaSubtitles(stream, 10) });
    const rows = within(screen.getByRole('list', { name: 'Your subtitles' })).getAllByRole('listitem');
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining('EnglishFrom HDRezka · SRT'),
      expect.stringContaining('RussianFrom HDRezka · SRT'),
    ]);
    fireEvent.click(within(rows[0]).getByRole('button', { name: 'Remove English subtitles' }));
    expect(props.onRemove).toHaveBeenCalledWith(expect.objectContaining({ language_code: 'en', source: 'hdrezka' }));
    expect(screen.getByText('2 languages')).toBeInTheDocument();
  });

  it('lists results in ranked order with their match pills', () => {
    renderPanel();
    const results = within(screen.getByRole('list', { name: 'Azerbaijani subtitles on OpenSubtitles' })).getAllByRole('listitem');
    expect(results.map((row) => row.querySelector('.release-name')?.getAttribute('title'))).toEqual(azResults().map((c) => c.file_name));
    expect(results.map((row) => row.querySelector('.pill')?.textContent)).toEqual(['Exact release', 'Same source', 'Different cut timing']);
  });

  it('gets a result, and marks one already stored as added', () => {
    const [best, ...rest] = azResults();
    const stored: Subtitle[] = [
      ...duneSubtitles(),
      { ...duneSubtitles()[1], id: 9, language_code: 'az', language_name: 'Azerbaijani', release_name: rest[0].release_name },
    ];
    const props = renderPanel({ stored });
    const results = within(screen.getByRole('list', { name: 'Azerbaijani subtitles on OpenSubtitles' })).getAllByRole('listitem');
    expect(within(results[1]).getByText('Added')).toBeInTheDocument();
    expect(within(results[1]).queryByRole('button', { name: /Get/ })).not.toBeInTheDocument();
    fireEvent.click(within(results[0]).getByRole('button', { name: /^Get/ }));
    expect(props.onGet).toHaveBeenCalledWith(best);
  });

  it('searches the chosen language', () => {
    const props = renderPanel({ search: { status: 'idle' } });
    fireEvent.change(screen.getByRole('combobox', { name: 'Subtitle language' }), { target: { value: 'tr' } });
    expect(props.onLanguageChange).toHaveBeenCalledWith('tr');
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));
    expect(props.onSearch).toHaveBeenCalled();
  });

  it('says when a search fails and offers to try again', () => {
    const retry = jest.fn();
    renderPanel({ search: { status: 'error', language: 'az', message: 'OpenSubtitles isn’t set up.', retry } });
    expect(screen.getByRole('alert')).toHaveTextContent('Couldn’t search OpenSubtitles');
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    expect(retry).toHaveBeenCalled();
  });
});
