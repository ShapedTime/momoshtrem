/**
 * Removals waiting out their Undo window (HANDOFF §4.4: "Removal from a list
 * offers Undo for 10 s"). Undo is a delayed commit: the item hides at once,
 * and the DELETE goes out only when the window closes. There's no
 * server-side restore.
 *
 * The store lives at module level, not in a component, so a pending item stays
 * hidden from every list while you move around the app, and the removal still
 * goes out when its toast closes on another page. Generic: the Library uses it
 * for titles, Activity (#102) for torrents.
 */

export interface PendingRemoval {
  /** Unique per thing removed, e.g. `library:movie:12`. Lists hide items whose key is pending. */
  key: string;
  /** Its name, for the toasts: "Removed {label}". */
  label: string;
  /** Sends the removal. */
  commit: () => Promise<unknown>;
  /**
   * Sends it while the page unloads (`pagehide`), when a promise can't finish:
   * a `keepalive` request (`apiKeepalive`). Without one, `commit` runs and may be cut off.
   */
  keepalive?: () => void;
  /** Whatever a list needs to account for the hidden item (e.g. to adjust its counts). */
  payload?: unknown;
  /**
   * The commit failed in a way that may still succeed on the server (a 504: the
   * proxy gave up waiting, the backend didn't). The item then stays hidden
   * instead of coming back at once (`uncertain`).
   */
  mayStillSucceed?: (error: unknown) => boolean;
}

type Entry = { removal: PendingRemoval; state: 'waiting' | 'committing' };

export type CommitResult =
  | { result: 'committed' }
  | { result: 'failed'; error: unknown }
  | { result: 'uncertain'; error: unknown }
  | { result: 'skipped' };

const entries = new Map<string, Entry>();
const listeners = new Set<() => void>();
let snapshot: ReadonlyMap<string, PendingRemoval> = new Map();
let pagehideBound = false;

function emit() {
  snapshot = new Map([...entries].map(([key, entry]) => [key, entry.removal]));
  listeners.forEach((listener) => listener());
}

/** `pagehide`: whatever is still waiting goes out now, with keepalive, so leaving the app doesn't undo it. */
function flushOnPageHide() {
  for (const entry of entries.values()) {
    if (entry.state !== 'waiting') continue;
    entry.state = 'committing';
    if (entry.removal.keepalive) entry.removal.keepalive();
    else void entry.removal.commit().catch(() => undefined);
  }
}

/** Start a removal's Undo window: its item hides everywhere until it's undone or released. */
export function addPendingRemoval(removal: PendingRemoval): void {
  if (!pagehideBound && typeof window !== 'undefined') {
    window.addEventListener('pagehide', flushOnPageHide);
    pagehideBound = true;
  }
  entries.set(removal.key, { removal, state: 'waiting' });
  emit();
}

/** Undo: the item comes back and nothing is sent. False once the removal is on its way. */
export function undoPendingRemoval(key: string): boolean {
  if (entries.get(key)?.state !== 'waiting') return false;
  entries.delete(key);
  emit();
  return true;
}

/**
 * The window closed: send the removal. On failure the item comes back, unless
 * `mayStillSucceed` says the server may still be at it (`uncertain`). On success
 * (or uncertain) it stays hidden until `releasePendingRemoval`, once the lists
 * have refetched without it. Skipped when it was undone or is already on its way.
 */
export async function commitPendingRemoval(key: string): Promise<CommitResult> {
  const entry = entries.get(key);
  if (!entry || entry.state !== 'waiting') return { result: 'skipped' };
  entry.state = 'committing';
  try {
    await entry.removal.commit();
    return { result: 'committed' };
  } catch (error) {
    if (entry.removal.mayStillSucceed?.(error)) return { result: 'uncertain', error };
    entries.delete(key);
    emit();
    return { result: 'failed', error };
  }
}

/** Stop hiding a committed removal's item (the lists no longer have it). */
export function releasePendingRemoval(key: string): void {
  if (entries.delete(key)) emit();
}

/** The pending removals by key (a stable snapshot for useSyncExternalStore). */
export function pendingRemovals(): ReadonlyMap<string, PendingRemoval> {
  return snapshot;
}

export function subscribePendingRemovals(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/** Tests: forget everything. */
export function resetPendingRemovals(): void {
  entries.clear();
  emit();
}
