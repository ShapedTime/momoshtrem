import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useAskConversation } from '@/hooks/useAskConversation';
import { ApiError } from '@/lib/api/client';
import type { AskEvent, AskRequest } from '@/lib/api/contract';
import { askStream } from '@/lib/api/endpoints';

/** useAskConversation (#156): Stop aborts the request, and an `ask_busy` right after your own Stop is retried once. */

jest.mock('@/lib/api/endpoints', () => ({ ...jest.requireActual('@/lib/api/endpoints'), askStream: jest.fn() }));
const stream = askStream as jest.MockedFunction<typeof askStream>;

const START: AskEvent = { event: 'start', data: { thread_id: 13, turn_id: 46, title: 'A thriller' } };
const DONE: AskEvent = {
  event: 'done',
  data: { thread_id: 13, turn_id: 47, elapsed_ms: 4000, refinements: [], usage: { input_tokens: 1, output_tokens: 1, web_searches: 0, cost_usd: 0.01 } },
};
const BUSY = new ApiError(409, 'ask_busy', 'Noir is still answering.');

/** A stream that starts, then holds until it's aborted (ask-hold). */
async function* holding(_body: AskRequest, signal?: AbortSignal): AsyncGenerator<AskEvent> {
  yield START;
  await new Promise((_, reject) => signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError'))));
}
async function* finishing(): AsyncGenerator<AskEvent> {
  yield { ...START, data: { ...START.data, turn_id: 47 } };
  yield DONE;
}
async function* refusing(): AsyncGenerator<AskEvent> {
  throw BUSY;
}

const originalFetch = global.fetch;
beforeEach(() => {
  stream.mockReset();
  // The thread the page reads once `start` names it
  const thread = { id: 13, title: 'A thriller', created_at: '2026-10-16T09:00:00Z', updated_at: '2026-10-16T09:00:00Z', turn_count: 1, model: 'claude-opus-5-5', turns: [] };
  global.fetch = jest.fn(async () => ({ ok: true, status: 200, json: async () => thread, clone() { return this; } }) as unknown as Response);
});
afterEach(() => {
  global.fetch = originalFetch;
});

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  return renderHook(() => useAskConversation(null), { wrapper });
}

describe('useAskConversation', () => {
  it('Stop aborts the stream and cancels the turn; the next ask retries once on ask_busy', async () => {
    stream.mockImplementationOnce(holding).mockImplementationOnce(refusing).mockImplementationOnce(finishing);
    const { result } = setup();

    act(() => void result.current.send('a thriller'));
    await waitFor(() => expect(result.current.session.live[0]?.phase).toBe('streaming'));
    expect(result.current.session.threadId).toBe(13);
    const firstSignal = stream.mock.calls[0][1];

    act(() => result.current.stop());
    expect(firstSignal?.aborted).toBe(true);
    expect(result.current.session.live[0].phase).toBe('cancelled');

    act(() => void result.current.send('something shorter'));
    await waitFor(() => expect(result.current.session.live[1]?.phase).toBe('done'), { timeout: 3000 });
    expect(stream).toHaveBeenCalledTimes(3);
    // The follow-up goes to the stopped turn's thread
    expect(stream.mock.calls[1][0]).toEqual({ thread_id: 13, message: 'something shorter' });
    expect(result.current.session.live[1].failure).toBeNull();
  });

  it('an ask_busy without a Stop before it is the answer: no retry', async () => {
    stream.mockImplementationOnce(refusing);
    const { result } = setup();
    act(() => void result.current.send('a thriller'));
    await waitFor(() => expect(result.current.session.live[0]?.phase).toBe('error'));
    expect(result.current.session.live[0].failure).toMatchObject({ status: 409, code: 'ask_busy' });
    expect(stream).toHaveBeenCalledTimes(1);
  });

  it('a 429 pauses asks with its reset time', async () => {
    stream.mockImplementationOnce(async function* () {
      throw new ApiError(429, 'budget_exceeded', 'Used up', undefined, '2026-11-01T00:00:00+03:00');
    });
    const { result } = setup();
    act(() => void result.current.send('a thriller'));
    await waitFor(() => expect(result.current.session.paused).toEqual({ code: 'budget_exceeded', resetsAt: '2026-11-01T00:00:00+03:00' }));
    expect(result.current.session.threadId).toBeNull();
  });
});
