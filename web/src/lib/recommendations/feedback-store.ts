import type { RecSignal } from '@/lib/api/contract';

/**
 * The feedback given on this page, per title (`recFeedbackKey`), shared by
 * every card that shows the title: the same pick on two shelves, the hero,
 * an Ask card. It lives at module level, like the pending removals, so a
 * signal stays put while you move around the app and the server's copy of
 * /home is older than what you just did. `null` means cleared.
 */

const signals = new Map<string, RecSignal | null>();
const listeners = new Set<() => void>();
/** Each title's requests, in the order they were made, so an Undo never overtakes the signal it undoes. */
const queues = new Map<string, Promise<unknown>>();

/** The signal set on this page, `null` once cleared, or undefined when nothing was (the server's stands). */
export function localSignal(key: string): RecSignal | null | undefined {
  return signals.has(key) ? signals.get(key) : undefined;
}

export function setLocalSignal(key: string, signal: RecSignal | null): void {
  signals.set(key, signal);
  listeners.forEach((listener) => listener());
}

export function subscribeLocalSignals(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Runs `send` once this title's earlier requests have settled (whether they failed or not). */
export function inOrder<T>(key: string, send: () => Promise<T>): Promise<T> {
  const previous = queues.get(key) ?? Promise.resolve();
  const next = previous.then(send, send);
  const settled = next.catch(() => undefined);
  queues.set(key, settled);
  void settled.then(() => {
    if (queues.get(key) === settled) queues.delete(key);
  });
  return next;
}

/** Tests: forget everything. */
export function resetLocalSignals(): void {
  signals.clear();
  queues.clear();
  listeners.forEach((listener) => listener());
}
