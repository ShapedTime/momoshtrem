'use client';

import { useState } from 'react';
import { Bookmark, History, Link2Off, RefreshCw, Star } from 'lucide-react';
import { Banner, Button, ButtonLink, Panel, StatusPill, Steps, Switch } from '@/components/ui';
import { useNow } from '@/hooks/useNow';
import type { TraktIntegration, TraktSettings } from '@/lib/api/contract';
import { importSteps, isFirstSync, lastSyncLine, REAUTH_TITLE, reauthBody, syncErrorCopy, watchlistHelp } from '@/lib/settings/trakt';
import { formatNumber } from '@/lib/title/format';
import { SettingRow, SettingsCard } from '../SettingsCard';

interface TraktConnectedProps {
  integration: TraktIntegration;
  /** Sync now was pressed and its POST is on its way. */
  starting: boolean;
  onSync: () => void;
  reconnecting: boolean;
  onReconnect: () => void;
  /** Saves one switch; rejects when the server refuses. */
  onSetting: (field: keyof TraktSettings, value: boolean) => Promise<void>;
  disconnecting: boolean;
  onDisconnect: () => void;
}

const SWITCHES: Array<{ field: keyof TraktSettings; label: string; help: (integration: TraktIntegration) => string }> = [
  { field: 'use_ratings', label: 'Ratings shape your picks', help: () => 'Titles you rated 8+ steer “Because you loved…” shelves.' },
  { field: 'show_watchlist', label: 'Watchlist on For You', help: (integration) => watchlistHelp(integration.counts) },
  { field: 'include_paused', label: 'Up next includes paused shows', help: () => 'Shows you haven’t touched in 6 months are hidden.' },
];

/**
 * A switch that moves on the tap itself. The change on its way reaches the
 * props a moment later (TanStack notifies on the next task); until then the
 * switch holds what was asked, and lets go once the props catch up or the
 * save fails (the props then say what's saved).
 */
function SettingSwitch({ id, checked, onChange }: { id: string; checked: boolean; onChange: (checked: boolean) => Promise<void> }) {
  const [wanted, setWanted] = useState<boolean | null>(null);
  if (wanted !== null && wanted === checked) setWanted(null);
  return (
    <Switch
      id={`${id}-input`}
      describedBy={`${id}-help`}
      checked={wanted ?? checked}
      onCheckedChange={(next) => {
        setWanted(next);
        onChange(next).catch(() => setWanted(null));
      }}
    />
  );
}

/** While the first import runs: its steps, and For You filling in behind it. */
function FirstSyncBanner({ integration }: { integration: TraktIntegration }) {
  return (
    <div className="banner banner--accent" role="status">
      <RefreshCw size={20} aria-hidden="true" />
      <div className="grow">
        <strong>First sync in progress</strong>
        <span>For You fills in as each part comes in.</span>
        <Steps inline className="mt-2" steps={importSteps(integration.sync)} />
      </div>
      <div className="actions">
        <ButtonLink href="/" size="sm">
          See For You
        </ButtonLink>
      </div>
    </div>
  );
}

/**
 * Connected (screens/settings/trakt-connected): the account, Sync now and
 * what's imported, the three switches, and Disconnect in a quiet zone. In
 * `reauth` the same, with a banner to sign in again instead of Sync now.
 */
export function TraktConnected({ integration, starting, onSync, reconnecting, onReconnect, onSetting, disconnecting, onDisconnect }: TraktConnectedProps) {
  const now = useNow(30_000);
  const reauth = integration.state === 'reauth';
  const { sync, counts, settings } = integration;
  const username = integration.username ?? 'Trakt';
  const syncError = !reauth && sync.status === 'error' ? syncErrorCopy(sync) : null;

  return (
    <>
      {reauth && (
        <Banner
          tone="warning"
          role="alert"
          title={REAUTH_TITLE}
          action={
            <Button size="sm" loading={reconnecting} onClick={onReconnect}>
              Reconnect
            </Button>
          }
        >
          {reauthBody(integration)}
        </Banner>
      )}
      {isFirstSync(integration) && <FirstSyncBanner integration={integration} />}
      {syncError && (
        <Banner
          tone="error"
          role="alert"
          title={syncError.title}
          action={
            <Button size="sm" loading={starting} onClick={onSync}>
              Retry
            </Button>
          }
        >
          {syncError.body}
        </Banner>
      )}

      <Panel as="section" aria-labelledby="trakt-user-h" className="set-card">
        <div className="set-profile">
          <span className="set-avatar" aria-hidden="true">
            {username.charAt(0).toUpperCase()}
          </span>
          <div className="flex min-w-40 grow flex-col gap-1">
            <div className="flex flex-wrap items-center gap-2">
              <h3 id="trakt-user-h" className="t-title-3 wrap-any" tabIndex={-1} data-screen-focus>
                <span className="sr-only">Trakt account </span>
                {username}
              </h3>
              {reauth ? <StatusPill status="warning" label="Sign in again" /> : <StatusPill status="ok" label="Connected" />}
            </div>
            <span className="t-small text-fg-3">{lastSyncLine(integration, now)}</span>
          </div>
          {!reauth && (
            <div className="set-profile-actions">
              {sync.status === 'syncing' ? (
                // A sync can take a while (the first import): say so, rather than a bare spinner
                <Button disabled>
                  <RefreshCw size={17} aria-hidden="true" className="animate-spin motion-reduce:animate-none" /> Syncing…
                </Button>
              ) : (
                <Button loading={starting} onClick={onSync}>
                  <RefreshCw size={17} aria-hidden="true" /> Sync now
                </Button>
              )}
            </div>
          )}
        </div>
        <dl className="set-counts">
          <div>
            <dt>
              <History size={15} aria-hidden="true" /> History
            </dt>
            <dd className="tabular-nums">
              {formatNumber(counts.history)} <span>{counts.history === 1 ? 'play' : 'plays'}</span>
            </dd>
          </div>
          <div>
            <dt>
              <Star size={15} aria-hidden="true" /> Ratings
            </dt>
            <dd className="tabular-nums">{formatNumber(counts.ratings)}</dd>
          </div>
          <div>
            <dt>
              <Bookmark size={15} aria-hidden="true" /> Watchlist
            </dt>
            <dd className="tabular-nums">
              {formatNumber(counts.watchlist)} <span>{counts.watchlist === 1 ? 'title' : 'titles'}</span>
            </dd>
          </div>
        </dl>
      </Panel>

      <SettingsCard id="trakt-uses" title="What momoshtrem uses">
        {SWITCHES.map(({ field, label, help }) => {
          const id = `trakt-${field}`;
          return (
            <SettingRow key={field} id={id} label={label} labelFor={`${id}-input`} help={help(integration)}>
              <SettingSwitch id={id} checked={settings[field]} onChange={(checked) => onSetting(field, checked)} />
            </SettingRow>
          );
        })}
      </SettingsCard>

      <div className="set-quiet-zone">
        <div className="flex min-w-0 flex-col gap-1">
          <strong className="font-semibold text-fg-1">Disconnect Trakt</strong>
          <span className="t-small text-fg-3">Deletes imported history, ratings and watchlist from momoshtrem. Nothing changes on Trakt.</span>
        </div>
        <Button variant="danger-quiet" loading={disconnecting} onClick={onDisconnect}>
          <Link2Off size={17} aria-hidden="true" /> Disconnect
        </Button>
      </div>
    </>
  );
}
