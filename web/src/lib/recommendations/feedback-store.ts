import type { RecSignal } from '@/lib/api/contract';

/**
 * The feedback given on this page, per title (`recFeedbackKey`), shared by
 * every card that shows the title: the same pick on two shelves, the hero,
 * an Ask card. It lives at module level, like the pending removals, so a
 * signal stays put while you move around the app and the server's copy of
 * /home is older than what you just did. `null` means cleared.
 */

const signals = new Map<string, RecSignal | null>();
/** What the server last accepted from this page, per title: a failed request goes back to it. */
const confirmed = new Map<string, RecSignal | null>();
/** The Undo toast on screen for a title, if any: a newer change to the title closes it. */
const undoToasts = new Map<string, number>();
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

/** The signal the server last accepted from this page, or undefined when it hasn't accepted one (its own stands). */
export function confirmedSignal(key: string): RecSignal | null | undefined {
  return confirmed.has(key) ? confirmed.get(key) : undefined;
}

export function setConfirmedSignal(key: string, signal: RecSignal | null): void {
  confirmed.set(key, signal);
}

/** The id of the title's Undo toast, while it's on screen. */
export const undoToast = (key: string): number | undefined => undoToasts.get(key);

export function setUndoToast(key: string, id: number): void {
  undoToasts.set(key, id);
}

/** The toast went: forget it, unless a newer one for the title replaced it. */
export function clearUndoToast(key: string, id: number): void {
  if (undoToasts.get(key) === id) undoToasts.delete(key);
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
  confirmed.clear();
  undoToasts.clear();
  queues.clear();
  listeners.forEach((listener) => listener());
}
