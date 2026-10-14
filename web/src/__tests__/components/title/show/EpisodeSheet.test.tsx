import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfirmProvider, ToastProvider } from '@/components/ui';
import { EpisodeSheet } from '@/components/title/show';

const originalFetch = global.fetch;
const answer = (status: number, body?: unknown) => ({ ok: status < 300, status, json: async () => body }) as Response;

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

describe('EpisodeSheet', () => {
  it('says when the episode didn’t load, instead of “not in your library”, and retries', async () => {
    global.fetch = jest.fn(() => Promise.resolve(answer(502, { error: { code: 'tmdb_unavailable', message: 'TMDB isn’t answering.' } }))) as typeof fetch;
    render(
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <ToastProvider>
          <ConfirmProvider>
            <EpisodeSheet params={new URLSearchParams('sheet=episode&tmdb=209867&se=S01E28')} close={jest.fn()} />
          </ConfirmProvider>
        </ToastProvider>
      </QueryClientProvider>
    );
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Couldn’t load this episode');
    expect(alert).toHaveTextContent('TMDB isn’t answering.');
    expect(screen.queryByText(/isn’t in your library/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
    await waitFor(() => expect(global.fetch).toHaveBeenCalledTimes(2));
  });
});
