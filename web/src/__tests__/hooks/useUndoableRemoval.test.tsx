import type { ReactNode } from 'react';
import { act, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ToastProvider, useToast } from '@/components/ui';
import { UNCERTAIN_HOLD_MS, usePendingRemovals, useUndoableRemoval, UNDO_MS } from '@/hooks/useUndoableRemoval';
import { addPendingRemoval, commitPendingRemoval, pendingRemovals, resetPendingRemovals, undoPendingRemoval } from '@/lib/undo/removals';

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return (
    <QueryClientProvider client={client}>
      <ToastProvider>{children}</ToastProvider>
    </QueryClientProvider>
  );
}

/** A removal whose DELETE resolves or rejects when the test says so. */
function removal(key = 'library:movie:1') {
  let settle: { resolve: () => void; reject: (error: Error) => void } = { resolve: () => {}, reject: () => {} };
  const commit = jest.fn(() => new Promise<void>((resolve, reject) => (settle = { resolve, reject })));
  const keepalive = jest.fn();
  return { removal: { key, label: 'Dune', commit, keepalive }, commit, keepalive, settle: () => settle };
}

/** Renders the hook plus a way to show another toast and read the pending keys. */
function setup() {
  return renderHook(
    () => ({ remove: useUndoableRemoval(), toast: useToast().toast, pending: usePendingRemovals() }),
    { wrapper }
  );
}

beforeEach(() => {
  jest.useFakeTimers();
  resetPendingRemovals();
});
afterEach(() => jest.useRealTimers());

describe('useUndoableRemoval', () => {
  it('hides the item at once and commits when the 10 s toast times out', async () => {
    const { result } = setup();
    const r = removal();
    act(() => result.current.remove(r.removal));
    expect(result.current.pending.has('library:movie:1')).toBe(true);
    expect(screen.getByRole('status')).toHaveTextContent('Removed Dune');
    expect(screen.getByRole('status')).toHaveTextContent('Undo within 10 seconds');

    act(() => jest.advanceTimersByTime(5_000));
    expect(r.commit).not.toHaveBeenCalled();
    act(() => jest.advanceTimersByTime(UNDO_MS - 5_000));
    expect(r.commit).toHaveBeenCalledTimes(1);
    // Still hidden while the DELETE is on its way, then released once the lists refetched
    expect(result.current.pending.has('library:movie:1')).toBe(true);
    await act(async () => r.settle().resolve());
    expect(result.current.pending.size).toBe(0);
  });

  it('Undo brings it back and sends nothing', () => {
    const { result } = setup();
    const r = removal();
    act(() => result.current.remove(r.removal));
    fireEvent.click(screen.getByRole('button', { name: 'Undo' }));
    expect(result.current.pending.size).toBe(0);
    act(() => jest.advanceTimersByTime(UNDO_MS * 2));
    expect(r.commit).not.toHaveBeenCalled();
  });

  it('commits at once when dismissed, or when another toast replaces it', () => {
    const { result } = setup();
    const first = removal('library:movie:1');
    act(() => result.current.remove(first.removal));
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
    expect(first.commit).toHaveBeenCalledTimes(1);

    const second = removal('library:tv:2');
    act(() => result.current.remove(second.removal));
    act(() => {
      result.current.toast({ title: 'Something else' });
    });
    expect(second.commit).toHaveBeenCalledTimes(1);
  });

  it('brings the item back with an error toast when the DELETE fails', async () => {
    const { result } = setup();
    const r = removal();
    act(() => result.current.remove(r.removal));
    act(() => jest.advanceTimersByTime(UNDO_MS));
    await act(async () => r.settle().reject(new Error('Movie not found')));
    expect(result.current.pending.size).toBe(0);
    expect(screen.getByRole('alert')).toHaveTextContent('Couldn’t remove Dune');
    expect(screen.getByRole('alert')).toHaveTextContent('Movie not found · It’s back where it was.');
  });

  it('says what happens in its own words when it isn’t a removal (Stop keeping)', async () => {
    const { result } = setup();
    const r = removal('download:5');
    const copy = { title: 'Stopped keeping Dune', body: 'What’s downloaded stays as cache.', failed: 'Couldn’t stop keeping Dune' };
    act(() => result.current.remove({ ...r.removal, copy }));
    expect(screen.getByRole('status')).toHaveTextContent('Stopped keeping Dune');
    expect(screen.getByRole('status')).toHaveTextContent('What’s downloaded stays as cache. Undo within 10 seconds.');
    act(() => jest.advanceTimersByTime(UNDO_MS));
    await act(async () => r.settle().reject(new Error('Server error')));
    expect(screen.getByRole('alert')).toHaveTextContent('Couldn’t stop keeping Dune');
  });

  it('keeps the item hidden when the failure may still succeed (a 504), then lets go after a while', async () => {
    const { result } = setup();
    const r = removal('torrent:abc');
    const mayStillSucceed = jest.fn((error: unknown) => error instanceof Error && error.message === 'timeout');
    act(() => result.current.remove({ ...r.removal, mayStillSucceed }));
    act(() => jest.advanceTimersByTime(UNDO_MS));
    await act(async () => r.settle().reject(new Error('timeout')));
    expect(mayStillSucceed).toHaveBeenCalled();
    expect(screen.getByRole('status')).toHaveTextContent('Still removing Dune…');
    expect(result.current.pending.has('torrent:abc')).toBe(true);
    await act(async () => jest.advanceTimersByTime(UNCERTAIN_HOLD_MS));
    expect(result.current.pending.size).toBe(0);
  });
});

describe('the pending-removals store', () => {
  it('sends what is still waiting with keepalive when the page goes away', () => {
    const waiting = removal('a');
    const undone = removal('b');
    const committing = removal('c');
    addPendingRemoval(waiting.removal);
    addPendingRemoval(undone.removal);
    addPendingRemoval(committing.removal);
    undoPendingRemoval('b');
    void commitPendingRemoval('c');

    window.dispatchEvent(new Event('pagehide'));
    expect(waiting.keepalive).toHaveBeenCalledTimes(1);
    expect(waiting.commit).not.toHaveBeenCalled();
    expect(undone.keepalive).not.toHaveBeenCalled();
    expect(committing.keepalive).not.toHaveBeenCalled();
    // Once sent, closing its toast later doesn't send it again
    return commitPendingRemoval('a').then((outcome) => expect(outcome).toEqual({ result: 'skipped' }));
  });

  it('skips commits for removals that were undone or never added', async () => {
    expect(await commitPendingRemoval('nothing')).toEqual({ result: 'skipped' });
    expect(undoPendingRemoval('nothing')).toBe(false);
    expect(pendingRemovals().size).toBe(0);
  });

  it('keeps hiding the item for every list, across renders', () => {
    const r = removal('library:movie:9');
    addPendingRemoval(r.removal);
    function List() {
      const pending = usePendingRemovals();
      return <p>{pending.has('library:movie:9') ? 'hidden' : 'shown'}</p>;
    }
    render(<List />);
    expect(screen.getByText('hidden')).toBeInTheDocument();
    act(() => {
      undoPendingRemoval('library:movie:9');
    });
    expect(screen.getByText('shown')).toBeInTheDocument();
  });
});
