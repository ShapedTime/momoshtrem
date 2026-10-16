import type { ReactNode } from 'react';
import { act, fireEvent, render, renderHook, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { FeedbackRow, type RecFeedbackTarget, useRecFeedback } from '@/components/recommendations';
import { ToastProvider } from '@/components/ui';
import { resetLocalSignals } from '@/lib/recommendations/feedback-store';

/** useRecFeedback and FeedbackRow (#155, reused by Ask in #156): saved at once, Undo restores the previous signal. */

const originalFetch = global.fetch;
let calls: Array<{ method: string; url: string; body: unknown }>;
let failures = 0;

beforeEach(() => {
  resetLocalSignals();
  calls = [];
  failures = 0;
  global.fetch = jest.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ method: init?.method ?? 'GET', url: String(input), body: init?.body ? JSON.parse(String(init.body)) : undefined });
    if (failures > 0) {
      failures -= 1;
      const body = { error: { code: 'internal', message: 'momoshtrem didn’t answer.' } };
      return { ok: false, status: 500, json: async () => body, clone: () => ({ json: async () => body }) } as unknown as Response;
    }
    return { ok: true, status: 204, json: async () => undefined, clone: () => ({ json: async () => undefined }) } as unknown as Response;
  }) as typeof fetch;
});
afterEach(() => {
  global.fetch = originalFetch;
});

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

const ANDOR: RecFeedbackTarget = { media_type: 'tv', tmdb_id: 83867, title: 'Andor', feedback: null };
const PLURIBUS: RecFeedbackTarget = { media_type: 'tv', tmdb_id: 222766, title: 'Pluribus', feedback: 'like' };

const sent = () => calls.map(({ method, url, body }) => [method, url.replace('/api/recommendations/feedback', ''), body]);

function Row({ target, source = 'shelf' as const }: { target: RecFeedbackTarget; source?: 'shelf' | 'ask' }) {
  const feedback = useRecFeedback(target, source);
  return (
    <article data-dismissed={feedback.dismissed}>
      <FeedbackRow feedback={feedback} inLibrary={false} />
    </article>
  );
}

describe('useRecFeedback', () => {
  it('dislike dims at once, saves, and Undo DELETEs when there was no signal before', async () => {
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Less like this' }));
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'true');
    expect(screen.getByRole('button', { name: 'Less like this' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('status')).toHaveTextContent('Got it, fewer like this');
    expect(screen.getByRole('status')).toHaveTextContent('Andor won’t be picked again.');
    await waitFor(() => expect(sent()).toEqual([['POST', '', { media_type: 'tv', tmdb_id: 83867, signal: 'dislike', source: 'shelf' }]]));

    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'false');
    await waitFor(() => expect(sent()).toHaveLength(2));
    expect(sent()[1]).toEqual(['DELETE', '/tv/83867', undefined]);
  });

  it('Undo over a like POSTs the like again', async () => {
    const { container } = render(<Row target={PLURIBUS} source="ask" />, { wrapper });
    expect(screen.getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'Less like this' }));
    expect(screen.getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'false');
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    await waitFor(() => expect(sent()).toHaveLength(2));
    expect(sent()).toEqual([
      ['POST', '', { media_type: 'tv', tmdb_id: 222766, signal: 'dislike', source: 'ask' }],
      ['POST', '', { media_type: 'tv', tmdb_id: 222766, signal: 'like', source: 'ask' }],
    ]);
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'false');
    expect(screen.getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('thumbs up toggles, never dims and shows no toast; a second tap DELETEs', async () => {
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    const like = screen.getByRole('button', { name: 'More like this' });
    fireEvent.click(like);
    expect(like).toHaveAttribute('aria-pressed', 'true');
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'false');
    expect(screen.queryByRole('status')).toBeNull();
    fireEvent.click(like);
    expect(like).toHaveAttribute('aria-pressed', 'false');
    await waitFor(() => expect(sent()).toHaveLength(2));
    expect(sent().map(([method]) => method)).toEqual(['POST', 'DELETE']);
  });

  it('every card showing the title agrees', () => {
    render(
      <>
        <Row target={ANDOR} />
        <Row target={ANDOR} />
      </>,
      { wrapper }
    );
    fireEvent.click(screen.getAllByRole('button', { name: 'More like this' })[0]);
    for (const button of screen.getAllByRole('button', { name: 'More like this' })) expect(button).toHaveAttribute('aria-pressed', 'true');
  });

  it('a failed save puts the signal back and says so', async () => {
    failures = 1;
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Less like this' }));
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'true');
    await waitFor(() => expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'false'));
    expect(await screen.findByRole('alert')).toHaveTextContent('Couldn’t save your feedback on Andor');
  });

  it('a newer change closes the stale Undo: 👎 then 👍 keeps the like, and nothing DELETEs it', async () => {
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Less like this' }));
    expect(screen.getByRole('button', { name: 'Undo' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'More like this' }));
    // A like has no toast of its own, and the dislike's Undo is gone with it
    expect(screen.queryByRole('button', { name: 'Undo' })).toBeNull();
    expect(screen.queryByText('Got it, fewer like this')).toBeNull();
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'false');
    expect(screen.getByRole('button', { name: 'More like this' })).toHaveAttribute('aria-pressed', 'true');
    await waitFor(() => expect(sent()).toHaveLength(2));
    expect(sent().map(([method, , body]) => [method, (body as { signal?: string } | undefined)?.signal])).toEqual([
      ['POST', 'dislike'],
      ['POST', 'like'],
    ]);
  });

  it('a failed save and a failed Undo go back to what the server has, never to a dismissal it didn’t get', async () => {
    failures = 2;
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Less like this' }));
    // Undo before the POST has failed: the DELETE queues behind it, and fails too
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    await waitFor(() => expect(sent()).toHaveLength(2));
    expect(sent().map(([method]) => method)).toEqual(['POST', 'DELETE']);
    expect(await screen.findByText('Couldn’t undo your feedback on Andor')).toBeInTheDocument();
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'false');
    expect(screen.getByRole('button', { name: 'Less like this' })).toHaveAttribute('aria-pressed', 'false');
  });

  it('a failed Undo after a saved dislike keeps the dislike the server has', async () => {
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'Less like this' }));
    await waitFor(() => expect(sent()).toHaveLength(1));
    failures = 1;
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    expect(await screen.findByText('Couldn’t undo your feedback on Andor')).toBeInTheDocument();
    expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'true');
  });

  it('Not interested and Already seen dismiss too; the newest signal wins', () => {
    const { result } = renderHook(() => useRecFeedback(ANDOR, 'hero'), { wrapper });
    act(() => result.current.toggle('seen'));
    expect(result.current).toMatchObject({ signal: 'seen', dismissed: true, liked: false, disliked: false });
    act(() => result.current.toggle('not_interested'));
    expect(result.current).toMatchObject({ signal: 'not_interested', dismissed: true });
  });

  it('the "…" sheet offers every signal and Add to library, and saves the one chosen', async () => {
    const { container } = render(<Row target={ANDOR} />, { wrapper });
    fireEvent.click(screen.getByRole('button', { name: 'More feedback' }));
    const sheet = await screen.findByRole('dialog', { name: 'Andor' });
    expect(sheet).toHaveTextContent('Tune your picks');
    for (const name of ['More like this', 'Less like this', /Not interested/, /Already seen/, 'Add to library']) {
      expect(within(sheet).getByRole('button', { name })).toBeInTheDocument();
    }
    expect(within(sheet).getByRole('button', { name: /Not interested/ })).toHaveTextContent('Hides it everywhere');
    fireEvent.click(within(sheet).getByRole('button', { name: /Already seen/ }));
    await waitFor(() => expect(container.querySelector('article')).toHaveAttribute('data-dismissed', 'true'));
    await waitFor(() => expect(sent()).toEqual([['POST', '', { media_type: 'tv', tmdb_id: 83867, signal: 'seen', source: 'shelf' }]]));
  });
});
