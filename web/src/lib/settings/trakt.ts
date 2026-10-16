import type { TraktConnectError, TraktIntegration, TraktSync, TraktSyncStep } from '@/lib/api/contract';
import { formatNumber, plural } from '@/lib/title/format';

/**
 * Settings › Trakt (screens/settings/trakt-*, flow 3; PLAN §6 "Trakt (B5)"):
 * which screen the integration shows, the device code's countdown, and the
 * section's words. Pure, so Jest covers it; components/settings/TraktSection
 * renders it, and the ⌘K palette and the settings nav read their lines here.
 */

/**
 * `code` and `waiting` are both `pending`: "I've approved it" only changes
 * what this page shows (a local flag), so a reload lands on the code again.
 * `reauth` keeps the connected layout, with a banner to sign in again.
 */
export type TraktScreen = 'unconfigured' | 'off' | 'code' | 'waiting' | 'connected' | 'reauth';

/** Trakt checks for changes every 30 minutes (PLAN §7 B5). */
export const SYNC_EVERY_MINUTES = 30;

/** Trakt's device codes last 10 minutes (`expires_in: 600`); the expiry bar runs over that. */
export const CODE_LIFETIME_SECONDS = 600;

/** What the server needs before Trakt can connect (PLAN §4). */
export const TRAKT_ENV = ['TRAKT_CLIENT_ID', 'TRAKT_CLIENT_SECRET', 'SECRET_KEY'] as const;

/** `approvedCode`: the code "I've approved it" was pressed for, if any. */
export function traktScreen(integration: TraktIntegration, approvedCode: string | null): TraktScreen {
  if (!integration.configured) return 'unconfigured';
  switch (integration.state) {
    case 'pending':
      if (!integration.pending) return integration.username ? 'reauth' : 'off';
      return integration.pending.user_code === approvedCode ? 'waiting' : 'code';
    case 'connected':
      return 'connected';
    case 'reauth':
      return 'reauth';
    default:
      return 'off';
  }
}

// ---- The device code ---------------------------------------------------------------

/** Whole seconds until the code expires, never below 0. */
export function secondsLeft(expiresAt: string, now: number): number {
  return Math.max(0, Math.ceil((Date.parse(expiresAt) - now) / 1000));
}

/** 899 → "14:59", 5 → "0:05". */
export function formatCountdown(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** How much of the code's life is left, 0–100 (a longer-lived code starts full). */
export function expiryPct(seconds: number): number {
  return Math.min(100, Math.max(0, (seconds / Math.max(CODE_LIFETIME_SECONDS, seconds, 1)) * 100));
}

/** The countdown for screen readers, which shouldn't hear every second: "about 9 minutes". */
export function spokenTimeLeft(seconds: number): string {
  if (seconds <= 0) return 'now';
  if (seconds < 60) return 'in less than a minute';
  const minutes = Math.round(seconds / 60);
  return `in about ${plural(minutes, 'minute')}`;
}

/** Trakt's 8-character codes read in two halves, as the design shows them: "7F3A91C2" → "7F3A-91C2". */
export function displayCode(code: string): string {
  return /^[a-z0-9]{8}$/i.test(code) ? `${code.slice(0, 4)}-${code.slice(4)}` : code;
}

/** One character at a time for screen readers: "7 F 3 A, 9 1 C 2". */
export function spokenCode(code: string): string {
  return displayCode(code)
    .split('-')
    .map((part) => part.split('').join(' '))
    .join(', ');
}

/** "https://trakt.tv/activate" → "trakt.tv/activate". */
export function displayUrl(url: string): string {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
}

/** The waiting screen's line: "Checking every 5 seconds · code 7F3A-91C2 expires in 13:40". */
export function waitingLine(interval: number, code: string, seconds: number): string {
  const every = interval === 1 ? 'every second' : `every ${interval} seconds`;
  return seconds > 0 ? `Checking ${every} · code ${displayCode(code)} expires in ${formatCountdown(seconds)}` : `Checking ${every} · code ${displayCode(code)} expired`;
}

// ---- Sync -----------------------------------------------------------------------------

/** "just now", "4 min ago", "2 h ago", "yesterday", "3 days ago". */
export function timeAgo(timestamp: string, now: number): string {
  const seconds = Math.max(0, (now - Date.parse(timestamp)) / 1000);
  if (seconds < 60) return 'just now';
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.floor(hours / 24);
  return days === 1 ? 'yesterday' : `${days} days ago`;
}

/** Under the username: "Last sync 4 min ago · syncs every 30 min"; "Syncing now…" while one runs. */
export function lastSyncLine(integration: TraktIntegration, now: number): string {
  const { sync, last_sync_at: last } = integration;
  const lastSync = last ? `Last sync ${timeAgo(last, now)}` : 'Not synced yet';
  // Nothing syncs until you sign in again
  if (integration.state === 'reauth') return lastSync;
  if (sync.status === 'syncing') return sync.kind === 'initial' ? 'First sync running…' : 'Syncing now…';
  return `${lastSync} · syncs every ${SYNC_EVERY_MINUTES} min`;
}

/** The palette's line under "Sync Trakt now": "Last synced 4 min ago". */
export function lastSyncedLine(integration: TraktIntegration, now: number): string {
  if (integration.sync.status === 'syncing') return 'Syncing now…';
  return integration.last_sync_at ? `Last synced ${timeAgo(integration.last_sync_at, now)}` : 'Not synced yet';
}

/** The first import's steps, in order (PLAN §6 "Trakt (B5)"). */
export const IMPORT_STEPS: readonly TraktSyncStep[] = ['history', 'ratings', 'watchlist', 'progress', 'details'];

const STEP_LABEL: Record<TraktSyncStep, string> = {
  history: 'History',
  ratings: 'Ratings',
  watchlist: 'Watchlist',
  progress: 'Show progress',
  details: 'Artwork',
};

const STEP_NOUN: Record<TraktSyncStep, string> = {
  history: 'your history',
  ratings: 'your ratings',
  watchlist: 'your watchlist',
  progress: 'your show progress',
  details: 'the artwork',
};

export interface ImportStep {
  key: TraktSyncStep;
  label: string;
  /** What it brought: "2,184 plays", or "1,200 of 2,184" while history imports. */
  detail?: string;
  state: 'done' | 'active' | 'pending';
}

function stepDetail(step: TraktSyncStep, progress: TraktSync['progress'], state: ImportStep['state']): string | undefined {
  if (state === 'pending') return undefined;
  switch (step) {
    case 'history':
      if (state === 'active') return progress.plays_total > 0 ? `${formatNumber(progress.plays)} of ${formatNumber(progress.plays_total)}` : undefined;
      return plural(progress.plays, 'play');
    case 'ratings':
      return state === 'done' ? formatNumber(progress.ratings) : undefined;
    case 'watchlist':
      return state === 'done' ? plural(progress.watchlist, 'title') : undefined;
    case 'progress':
      return state === 'done' ? plural(progress.shows, 'show') : undefined;
    default:
      return undefined;
  }
}

/** The first-sync banner's steps: done up to the running one. */
export function importSteps(sync: TraktSync): ImportStep[] {
  const at = sync.step ? IMPORT_STEPS.indexOf(sync.step) : sync.status === 'idle' ? IMPORT_STEPS.length : 0;
  return IMPORT_STEPS.map((key, index) => {
    const state = index < at ? 'done' : index === at ? 'active' : 'pending';
    return { key, label: STEP_LABEL[key], detail: stepDetail(key, sync.progress, state), state };
  });
}

/** The first import is running: For You fills in as it goes. */
export const isFirstSync = (integration: TraktIntegration): boolean =>
  integration.state === 'connected' && integration.sync.status === 'syncing' && integration.sync.kind === 'initial';

/** The last sync failed: the banner with Retry. */
export function syncErrorCopy(sync: TraktSync): { title: string; body: string } {
  const reason = sync.error ?? 'Trakt didn’t answer.';
  if (sync.kind === 'initial') {
    return { title: 'The first import didn’t finish', body: `${reason} Retry picks up from ${STEP_NOUN[sync.step ?? 'history']}.` };
  }
  return { title: 'The last sync didn’t finish', body: `${reason} Your imported history is still here.` };
}

// ---- Copy ------------------------------------------------------------------------------

/** Why the last code didn't connect, said calmly (denied may be Trakt's 2-app limit). */
export function connectErrorCopy(error: TraktConnectError): { tone: 'neutral' | 'warning'; title: string; body: string } {
  switch (error) {
    case 'expired':
      return { tone: 'neutral', title: 'That code expired', body: 'Trakt’s codes only last a few minutes. Connect again for a new one.' };
    case 'denied':
      return {
        tone: 'warning',
        title: 'Trakt didn’t let momoshtrem in',
        body: 'The code was declined. Free Trakt accounts can connect only 2 apps: if yours is at the limit, disconnect one in Trakt’s settings, then connect again.',
      };
    default:
      return { tone: 'warning', title: 'Couldn’t connect to Trakt', body: 'Something went wrong while Trakt checked the code. Connect again to get a new one.' };
  }
}

export const REAUTH_TITLE = 'Trakt needs you to sign in again';

/** The reauth banner's line: the server's reason, or what stays. */
export function reauthBody(integration: TraktIntegration): string {
  return integration.reauth_reason ?? 'Your imported history is still here. Reconnect to keep it up to date.';
}

/** The watchlist switch's help: "41 titles · 6 already in your library". */
export function watchlistHelp(counts: TraktIntegration['counts']): string {
  if (counts.watchlist === 0) return 'Nothing on your Trakt watchlist yet';
  const inLibrary = counts.watchlist_in_library > 0 ? ` · ${formatNumber(counts.watchlist_in_library)} already in your library` : '';
  return `${plural(counts.watchlist, 'title')}${inLibrary}`;
}

/** The settings nav's line under Trakt. Reauth and a failed sync get the attention dot. */
export function traktNavSummary(integration: TraktIntegration | undefined): { text: string; attention?: boolean } {
  if (!integration) return { text: '' };
  if (!integration.configured) return { text: 'Not set up' };
  switch (integration.state) {
    case 'pending':
      return { text: 'Waiting for approval' };
    case 'reauth':
      return { text: 'Sign in again', attention: true };
    case 'connected':
      if (isFirstSync(integration)) return { text: 'First sync running' };
      if (integration.sync.status === 'error') return { text: 'Last sync failed', attention: true };
      return { text: integration.username ? `Connected as ${integration.username}` : 'Connected' };
    default:
      return { text: 'Not connected' };
  }
}

// ---- Toasts ----------------------------------------------------------------------------

export interface TraktToastCopy {
  title: string;
  body?: string;
}

export const CONNECTED_TOAST: TraktToastCopy = { title: 'Trakt connected', body: 'Importing your history. For You fills in over the next minute.' };
export const DISCONNECTED_TOAST: TraktToastCopy = { title: 'Trakt disconnected' };
export const SYNC_STARTED_TOAST: TraktToastCopy = { title: 'Syncing with Trakt', body: 'Up next updates in a moment.' };

/** What a poll says happened since the last one: a code was approved, or a sync finished. */
export function traktEvent(before: TraktIntegration, after: TraktIntegration): 'connected' | 'synced' | null {
  if (before.state === 'pending' && after.state === 'connected') return 'connected';
  if (before.sync.status === 'syncing' && after.sync.status === 'idle' && after.state === 'connected') return 'synced';
  return null;
}

/**
 * "Synced with Trakt": what the first import brought ("Imported 2,184 plays,
 * 312 ratings and 41 watchlist titles."), or what's new since `before`.
 */
export function syncedToast(after: TraktIntegration, before: TraktIntegration['counts']): TraktToastCopy {
  const { counts } = after;
  if (after.sync.kind === 'initial') {
    return {
      title: 'Synced with Trakt',
      body: `Imported ${plural(counts.history, 'play')}, ${plural(counts.ratings, 'rating')} and ${plural(counts.watchlist, 'watchlist title')}.`,
    };
  }
  const news = [
    [counts.history - before.history, 'new play'],
    [counts.ratings - before.ratings, 'new rating'],
    [counts.watchlist - before.watchlist, 'watchlist addition'],
  ] as const;
  const parts = news.filter(([delta]) => delta > 0).map(([delta, noun]) => plural(delta, noun));
  return { title: 'Synced with Trakt', body: parts.length ? `${parts.join(', ')}.` : 'Nothing new since the last sync.' };
}
