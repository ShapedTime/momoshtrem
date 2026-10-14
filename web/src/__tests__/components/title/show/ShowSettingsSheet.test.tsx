import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider } from '@/components/ui';
import { ShowSettingsSheet } from '@/components/title/show';
import { titleKeys } from '@/lib/api/endpoints';
import { buildTitle } from '@/mocks/fixtures/titles';

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body }) as Response;
const GROUPS = {
  items: [
    { id: 'abs', name: 'Absolute order', type: 2, group_count: 1, episode_count: 38 },
    { id: 'cours', name: 'Cours', type: 6, group_count: 3, episode_count: 38 },
  ],
};

// Desktop presentation (Radix Dialog panel); jsdom can't run Vaul's drag physics.
beforeAll(() => {
  window.matchMedia = ((query: string) => ({
    matches: query.includes('min-width: 1024px'),
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  })) as typeof window.matchMedia;
});

afterEach(() => {
  global.fetch = originalFetch;
});

/** Frieren in the cache (library id 1); PATCH answers with `patchStatus`. */
function renderSheet(patchStatus = 200) {
  const patches: unknown[] = [];
  global.fetch = jest.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/shows/1/episode-groups') return Promise.resolve(answer(200, GROUPS));
    if (url === '/api/shows/1' && init?.method === 'PATCH') {
      patches.push(JSON.parse(String(init.body)));
      return Promise.resolve(
        patchStatus === 422
          ? answer(422, { error: { code: 'validation_error', message: 'Pick an episode group for this ordering.' } })
          : answer(200, buildTitle('tv', 209867))
      );
    }
    return Promise.resolve(answer(200, buildTitle('tv', 209867)));
  }) as typeof fetch;
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(titleKeys.detail('tv', 209867), buildTitle('tv', 209867));
  const close = jest.fn();
  render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <ShowSettingsSheet params={new URLSearchParams('sheet=show-settings&tmdb=209867')} close={close} />
      </ToastProvider>
    </QueryClientProvider>
  );
  return { close, patches };
}

describe('ShowSettingsSheet', () => {
  it('starts from the show’s settings, and the group picker only works for group ordering', async () => {
    renderSheet();
    expect(await screen.findByRole('dialog', { name: 'Show settings · Frieren: Beyond Journey’s End' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: /Absolute \(anime\)/ })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Episode group' })).toBeDisabled();
    expect(screen.getByRole('switch', { name: 'Include specials' })).toBeChecked();

    fireEvent.click(screen.getByRole('radio', { name: /TMDB episode group/ }));
    const groups = screen.getByRole('combobox', { name: 'Episode group' });
    expect(groups).toBeEnabled();
    expect(await screen.findByRole('option', { name: 'Cours · 38 episodes' })).toBeInTheDocument();
  });

  it('saves the PATCH, then closes', async () => {
    const { close, patches } = renderSheet();
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByRole('radio', { name: /TMDB episode group/ }));
    await screen.findByRole('option', { name: 'Cours · 38 episodes' });
    fireEvent.change(screen.getByRole('combobox', { name: 'Episode group' }), { target: { value: 'cours' } });
    fireEvent.click(screen.getByRole('switch', { name: 'Include specials' }));
    fireEvent.change(screen.getByRole('combobox', { name: 'File matching' }), { target: { value: 'rules_only' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(close).toHaveBeenCalled());
    expect(patches).toEqual([{ episode_ordering: 'episode_group', episode_group_id: 'cours', include_specials: false, classifier_mode: 'rules_only' }]);
    expect(await screen.findByText('Show settings saved')).toBeInTheDocument();
  });

  it('sends no group id for other orderings', async () => {
    const { patches } = renderSheet();
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByRole('radio', { name: /^Aired/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(patches).toHaveLength(1));
    expect(patches[0]).toMatchObject({ episode_ordering: 'aired', episode_group_id: null });
  });

  it('Cancel closes without saving', async () => {
    const { close, patches } = renderSheet();
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByRole('switch', { name: 'Include specials' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(close).toHaveBeenCalled();
    expect(patches).toEqual([]);
  });

  it('describes the specials switch', async () => {
    renderSheet();
    await screen.findByRole('dialog');
    expect(screen.getByRole('switch', { name: 'Include specials' })).toHaveAccessibleDescription('Season 0: recaps, OVAs, mini anime');
  });

  it('shows a 422 at the group picker and stays open, until another ordering is picked', async () => {
    const { close } = renderSheet(422);
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByRole('radio', { name: /TMDB episode group/ }));
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Pick an episode group for this ordering.')).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Episode group' })).toHaveAttribute('aria-invalid', 'true');
    expect(close).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('radio', { name: /^Aired/ }));
    await waitFor(() => expect(screen.queryByText('Pick an episode group for this ordering.')).not.toBeInTheDocument());
  });

  it('says when the show’s settings didn’t load, and retries', async () => {
    global.fetch = jest.fn(() => Promise.resolve(answer(500, { error: { code: 'internal_error', message: 'Database is locked.' } }))) as typeof fetch;
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <ToastProvider>
          <ShowSettingsSheet params={new URLSearchParams('sheet=show-settings&tmdb=209867')} close={jest.fn()} />
        </ToastProvider>
      </QueryClientProvider>
    );
    expect(await screen.findByRole('alert')).toHaveTextContent('Couldn’t load the show’s settings');
    expect(screen.queryByText(/Add the show to your library/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(global.fetch).toHaveBeenCalledTimes(2));
  });
});
