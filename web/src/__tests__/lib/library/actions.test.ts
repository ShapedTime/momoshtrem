import { libraryActions } from '@/lib/library/actions';
import { libraryItem, libraryShow } from '@/mocks/fixtures/library-item';

const kinds = (actions: ReturnType<typeof libraryActions>) => actions.map((action) => action.kind);

describe('libraryActions', () => {
  it('offers a ready movie its details, its path and removal', () => {
    const actions = libraryActions(libraryItem(), { torrentSheet: false });
    expect(kinds(actions)).toEqual(['open', 'copy', 'remove']);
    expect(actions[0]).toEqual({ kind: 'open', label: 'Open details', href: '/movie/693134' });
    expect(actions[1]).toMatchObject({ kind: 'copy', label: 'Copy WebDAV path', url: expect.stringContaining('/Movies/') });
    expect(actions.at(-1)).toEqual({ kind: 'remove', label: 'Remove from library…' });
  });

  it('sends a show with gaps and files to review where they’re fixed', () => {
    const actions = libraryActions(libraryShow({ needs_review: 2, review_job_id: 101, download: { pct: 12, info_hash: 'h' } }), { torrentSheet: false });
    expect(kinds(actions)).toEqual(['open', 'review', 'find', 'copy', 'download', 'remove']);
    expect(actions[1]).toEqual({ kind: 'review', label: 'Review mapping', href: '/mapping/101', hint: '2 files' });
    expect(actions[2]).toEqual({ kind: 'find', label: 'Find missing episodes', hint: '10', target: { kind: 'page', href: '/tv/209867?season=1' } });
    expect(actions[4]).toEqual({ kind: 'download', label: 'See download', href: '/torrents' });
  });

  it('opens the torrent sheet once it exists (#6)', () => {
    expect(libraryActions(libraryShow(), { torrentSheet: true })[1]).toMatchObject({
      kind: 'find',
      target: { kind: 'sheet', params: { show: '209867', season: '1', se: 'S01E29' } },
    });
    const movie = libraryItem({ availability_stage: 'needs_torrent', webdav_path: null, webdav_url: null });
    expect(libraryActions(movie, { torrentSheet: true })).toEqual([
      { kind: 'open', label: 'Open details', href: '/movie/693134' },
      { kind: 'find', label: 'Find a torrent', target: { kind: 'sheet', params: { movie: '693134' } } },
      { kind: 'remove', label: 'Remove from library…' },
    ]);
    expect(libraryActions(movie, { torrentSheet: false })[1]).toMatchObject({ target: { kind: 'page', href: '/movie/693134' } });
  });

  it('skips review without a job to open', () => {
    expect(kinds(libraryActions(libraryShow({ needs_review: 1, review_job_id: null }), { torrentSheet: false }))).not.toContain('review');
  });
});
