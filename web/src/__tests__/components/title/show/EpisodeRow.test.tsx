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
    fireEvent.click(screen.getByRole('button', { name: /^S01E01 options/ }));
    expect(onOpen).toHaveBeenCalledWith(watched);
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
    renderRow(episode(209867, 1, 26));
    expect(screen.getByText('Needs review')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review the mapping for S01E26' })).toHaveAttribute('href', '/mapping/101');
  });

  it('needs review without a job: opens the episode instead', () => {
    const { onOpen } = renderRow(episode(209867, 1, 26), { reviewHref: null });
    fireEvent.click(screen.getByRole('button', { name: /^S01E26 options/ }));
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
  });
});
