import { fireEvent, render, screen } from '@testing-library/react';
import { EpisodeRow } from '@/components/title/show';
import type { EpisodeSummary } from '@/lib/api/contract';
import { buildSeason } from '@/mocks/fixtures/titles';

function episode(tmdbId: number, season: number, number: number): EpisodeSummary {
  const found = buildSeason(tmdbId, season)?.episodes.find((e) => e.episode_number === number);
  if (!found) throw new Error(`No fixture for ${tmdbId} S${season}E${number}`);
  return found;
}

function renderRow(ep: EpisodeSummary, extra: { isNext?: boolean; reviewHref?: string | null; ordering?: 'aired' | 'absolute' } = {}) {
  const onFind = jest.fn();
  const onOpen = jest.fn();
  render(
    <ol>
      <EpisodeRow episode={ep} ordering={extra.ordering ?? 'aired'} isNext={extra.isNext} reviewHref={extra.reviewHref === undefined ? '/mapping/101' : extra.reviewHref} onFind={onFind} onOpen={onOpen} />
    </ol>
  );
  return { onFind, onOpen };
}

describe('EpisodeRow', () => {
  it('watched and ready: a pill and the episode options', () => {
    const watched = episode(209867, 1, 1);
    const { onOpen } = renderRow(watched);
    expect(screen.getByText('Watched')).toBeInTheDocument();
    expect(screen.getByRole('listitem')).toHaveClass('is-watched');
    fireEvent.click(screen.getByRole('button', { name: 'S01E01 options: how to play, subtitles, keep, replace, remove' }));
    expect(onOpen).toHaveBeenCalledWith(watched);
  });

  it('names no keep when the backend has no downloads (no `keep`)', () => {
    const older: Partial<EpisodeSummary> = { ...episode(209867, 1, 1) };
    delete older.keep;
    renderRow(older as EpisodeSummary);
    expect(screen.getByRole('button', { name: 'S01E01 options: how to play, subtitles, replace, remove' })).toBeInTheDocument();
  });

  it('names no keep for an HDRezka episode nothing keeps (its sheet offers none)', () => {
    renderRow({ ...episode(209867, 1, 1), source: 'hdrezka', keep: null });
    expect(screen.getByRole('button', { name: 'S01E01 options: how to play, subtitles, replace, remove' })).toBeInTheDocument();
  });

  it('watched without a file (a Trakt play): the pill, and the sheet offers to find one', () => {
    const watched = { ...episode(209867, 1, 1), webdav_path: null, source: null };
    const { onOpen, onFind } = renderRow(watched);
    expect(screen.getByText('Watched')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'S01E01 options: find a torrent' }));
    expect(onOpen).toHaveBeenCalledWith(watched);
    expect(onFind).not.toHaveBeenCalled();
  });

  it('up next on a watched or ready row: the amber rule and the label', () => {
    renderRow(episode(209867, 1, 27), { isNext: true });
    expect(screen.getByRole('listitem')).toHaveClass('is-next');
    expect(screen.getByText('Up next')).toBeInTheDocument();
    expect(screen.getByText('Ready')).toBeInTheDocument();
  });

  it('missing: finds a torrent for that episode', () => {
    const missing = episode(209867, 1, 29);
    const { onFind, onOpen } = renderRow(missing, { isNext: true });
    expect(screen.getByText('Missing')).toBeInTheDocument();
    expect(screen.getByText('Up next')).toBeInTheDocument();
    expect(screen.getByRole('listitem')).toHaveClass('is-next');
    fireEvent.click(screen.getByRole('button', { name: 'Find a torrent for S01E29' }));
    expect(onFind).toHaveBeenCalledWith(missing);
    expect(onOpen).not.toHaveBeenCalled();
  });

  it('needs review: links to the mapping review', () => {
    renderRow(episode(209867, 1, 35));
    expect(screen.getByText('Needs review')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review the mapping for S01E35' })).toHaveAttribute('href', '/mapping/101');
  });

  it('needs review without a job: opens the episode instead', () => {
    const { onOpen } = renderRow(episode(209867, 1, 35), { reviewHref: null });
    fireEvent.click(screen.getByRole('button', { name: /^S01E35 options/ }));
    expect(onOpen).toHaveBeenCalled();
  });

  it('downloading: the percentage in the pill', () => {
    const downloading = buildSeason(247767, 1)?.episodes.find((e) => e.state === 'downloading');
    if (!downloading) throw new Error('No downloading fixture');
    renderRow(downloading);
    expect(screen.getByText(`${Math.round(downloading.pct ?? 0)}%`)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /options/ })).toBeInTheDocument();
  });

  it('upcoming: its air date and no action', () => {
    renderRow(episode(37854, 22, 37), { ordering: 'absolute' });
    expect(screen.getByText(/^Airs /)).toBeInTheDocument();
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    expect(screen.getByText(/#1122 · S22E37/)).toBeInTheDocument();
    // Narrow rows keep the air date's words (title.css: .pill--labelled)
    expect(screen.getByText(/^Airs /).closest('.pill')).toHaveClass('pill--labelled');
  });

  it('a keep pill beside the state (#206): a smart pick reads Smart, and goes icon-only on narrow rows like the rest', () => {
    // Frieren S01E27 is a smart pick in the seeds
    renderRow(episode(209867, 1, 27));
    const smart = screen.getByText('Smart').closest('.pill');
    expect(smart).toHaveClass('pill--upcoming');
    expect(smart).not.toHaveClass('pill--labelled');
  });
});
