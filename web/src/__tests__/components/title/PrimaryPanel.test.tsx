import { fireEvent, render, screen } from '@testing-library/react';
import { ToastProvider } from '@/components/ui';
import { ActionDock, PrimaryPanel } from '@/components/title';
import type { Title } from '@/lib/api/contract';
import { primaryActionFor, type StageIntent } from '@/lib/title/stage-copy';
import { buildTitle } from '@/mocks/fixtures/titles';

function movie(tmdbId: number): Title {
  const title = buildTitle('movie', tmdbId);
  if (!title) throw new Error(`No fixture for movie ${tmdbId}`);
  return title;
}

function renderPanel(title: Title, busy: StageIntent | null = null) {
  const onAction = jest.fn();
  render(
    <ToastProvider>
      <PrimaryPanel copy={primaryActionFor(title)} onAction={onAction} busy={busy} titleName={title.title} />
    </ToastProvider>
  );
  return onAction;
}

describe('PrimaryPanel', () => {
  it('not in library: step 1 of 4 and Add to library', () => {
    const onAction = renderPanel(movie(666277));
    expect(screen.getByRole('heading', { name: 'Not in your library' })).toBeInTheDocument();
    expect(screen.getByText('Step 1 of 4')).toBeInTheDocument();
    expect(screen.getByText('Add', { selector: 'b' })).toHaveAttribute('aria-current', 'step');
    fireEvent.click(screen.getByRole('button', { name: 'Add to library' }));
    expect(onAction).toHaveBeenCalledWith('add');
  });

  it('keeps the label while adding', () => {
    renderPanel(movie(666277), 'add');
    expect(screen.getByRole('button', { name: 'Add to library' })).toHaveAttribute('aria-busy', 'true');
  });

  it('needs a torrent: Find a torrent', () => {
    const onAction = renderPanel(movie(1054867));
    expect(screen.getByRole('heading', { name: 'In your library · no torrent yet' })).toBeInTheDocument();
    expect(screen.getByText('Step 2 of 4')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Find a torrent' }));
    expect(onAction).toHaveBeenCalledWith('find');
  });

  it('needs a torrent, with HDRezka set up: Stream from HDRezka under Find a torrent', () => {
    const onAction = jest.fn();
    render(
      <ToastProvider>
        <PrimaryPanel copy={primaryActionFor(movie(1054867), { hdrezka: true })} onAction={onAction} titleName="One Battle After Another" />
      </ToastProvider>
    );
    expect(screen.getByText('Source', { selector: 'b' })).toHaveAttribute('aria-current', 'step');
    fireEvent.click(screen.getByRole('button', { name: 'Stream from HDRezka' }));
    expect(onAction).toHaveBeenCalledWith('hdrezka');
  });

  it('downloading: progress, View download and How to play', () => {
    const onAction = renderPanel(movie(157336));
    expect(screen.getByRole('heading', { name: 'Downloading' })).toBeInTheDocument();
    expect(screen.getByRole('progressbar', { name: 'Interstellar download' })).toHaveAttribute('aria-valuenow', '42');
    expect(screen.getByText('You can start watching in Infuse now;')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'View download' })).toHaveAttribute('href', '/torrents');
    fireEvent.click(screen.getByRole('button', { name: 'How to play in Infuse' }));
    expect(onAction).toHaveBeenCalledWith('how_to_play');
  });

  it('ready: what is ready, the WebDAV path and How to play', () => {
    const dune = movie(693134);
    const onAction = renderPanel(dune);
    expect(screen.getByRole('heading', { name: 'Ready in Infuse' })).toBeInTheDocument();
    expect(screen.getByText('2160p · 18.4 GB · Dolby Vision · Atmos')).toBeInTheDocument();
    expect(screen.getByLabelText('WebDAV path')).toHaveTextContent(dune.library.webdav_path ?? '');
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'How to play in Infuse' }));
    expect(onAction).toHaveBeenCalledWith('how_to_play');
  });
});

describe('ActionDock', () => {
  it('offers Play in Infuse and Copy path when ready, one button otherwise', () => {
    const { unmount } = render(
      <ToastProvider>
        <ActionDock copy={primaryActionFor(movie(693134))} onAction={jest.fn()} />
      </ToastProvider>
    );
    expect(screen.getByRole('button', { name: 'Play in Infuse' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy WebDAV path' })).toBeInTheDocument();
    unmount();

    render(
      <ToastProvider>
        <ActionDock copy={primaryActionFor(movie(157336))} onAction={jest.fn()} />
      </ToastProvider>
    );
    expect(screen.getByRole('link', { name: 'View download' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Copy WebDAV path' })).not.toBeInTheDocument();
  });

  it('puts Stream from HDRezka beside Find a torrent while HDRezka is set up', () => {
    const onAction = jest.fn();
    render(
      <ToastProvider>
        <ActionDock copy={primaryActionFor(movie(1054867), { hdrezka: true })} onAction={onAction} />
      </ToastProvider>
    );
    expect(screen.getByRole('button', { name: 'Find a torrent' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Stream from HDRezka' }));
    expect(onAction).toHaveBeenCalledWith('hdrezka');
  });
});
