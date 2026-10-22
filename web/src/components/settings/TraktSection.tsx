'use client';

import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Link2, RefreshCw } from 'lucide-react';
import { useMembers } from '@/components/members/MembersGate';
import { Banner, Button, LoadingRegion, Skeleton, useConfirm, useToast } from '@/components/ui';
import { usePendingTraktSettings, useTraktConnect, useTraktDisconnect, useTraktSettings, useTraktSync } from '@/hooks/useTraktActions';
import { useTraktIntegration } from '@/hooks/useTraktIntegration';
import type { TraktConnect, TraktIntegration, TraktSettings } from '@/lib/api/contract';
import { systemKeys } from '@/lib/api/endpoints';
import {
  CONNECTED_TOAST,
  connectErrorCopy,
  disconnectConfirmCopy,
  DISCONNECTED_TOAST,
  syncedToast,
  traktEvent,
  traktScreen,
  type TraktScreen,
} from '@/lib/settings/trakt';
import { TraktConnected } from './trakt/TraktConnected';
import { TraktDeviceCode } from './trakt/TraktDeviceCode';
import { TraktMemberNote } from './trakt/TraktMemberNote';
import { TraktOff } from './trakt/TraktOff';
import { TraktWaiting } from './trakt/TraktWaiting';

/** "I've approved it" is remembered for one code: the same code string comes back with a new expiry after a new connect. */
const codeKey = (code: TraktConnect) => `${code.user_code}@${code.expires_at}`;

function TraktSkeleton() {
  return (
    <LoadingRegion label="Loading Trakt" className="flex flex-col gap-4 lg:gap-5">
      <Skeleton height={184} className="rounded-lg" />
      <Skeleton height={288} className="rounded-lg" />
      <Skeleton height={64} className="rounded-lg" />
    </LoadingRegion>
  );
}

/**
 * Toasts for what polling saw happen: a code approved ("Trakt connected"),
 * a sync finished ("Synced with Trakt", with what it brought since it began).
 */
function useTraktToasts(data: TraktIntegration | undefined) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const previous = useRef<TraktIntegration | undefined>(undefined);
  const countsAtStart = useRef<TraktIntegration['counts'] | null>(null);

  useEffect(() => {
    const before = previous.current;
    previous.current = data;
    if (!data) return;
    if (data.sync.status === 'syncing' && !countsAtStart.current) countsAtStart.current = before?.counts ?? data.counts;
    const event = before ? traktEvent(before, data) : null;
    if (event === 'connected') {
      toast({ tone: 'ready', icon: Link2, ...CONNECTED_TOAST });
      void queryClient.invalidateQueries({ queryKey: systemKeys.status });
    } else if (event === 'synced' && before) {
      toast({ tone: 'ready', icon: RefreshCw, ...syncedToast(data, countsAtStart.current ?? before.counts) });
    }
    if (data.sync.status !== 'syncing') countsAtStart.current = null;
  }, [data, toast, queryClient]);
}

/**
 * When the screen changes under the user's hands (Connect, Cancel, a code
 * approved), the button they pressed is gone: focus moves to the new
 * screen's heading, unless they're already somewhere else on the page.
 */
function useScreenFocus(screen: TraktScreen | null) {
  const container = useRef<HTMLDivElement>(null);
  const shown = useRef(screen);
  useEffect(() => {
    if (shown.current === screen) return;
    const first = shown.current === null;
    shown.current = screen;
    const active = document.activeElement;
    if (first || (active && active !== document.body && !container.current?.contains(active))) return;
    container.current?.querySelector<HTMLElement>('[data-screen-focus]')?.focus();
  }, [screen]);
  return container;
}

/**
 * Settings › Trakt (screens/settings/trakt-*, flow 3): connect with a device
 * code, wait for the approval, then the account, its sync and what
 * momoshtrem uses. The integration (PLAN §6 "Trakt (B5)") polls while a
 * code waits or a sync runs. It's the current member's Trakt (#239): with two
 * or more people a note says whose, and the disconnect names them.
 */
export function TraktSection() {
  const query = useTraktIntegration();
  const members = useMembers();
  const { toast } = useToast();
  const confirm = useConfirm();
  const connect = useTraktConnect();
  const sync = useTraktSync();
  const disconnect = useTraktDisconnect();
  const settings = useTraktSettings();
  const [approved, setApproved] = useState<string | null>(null);
  // The switches show what's being saved, not only what was
  const shownSettings = usePendingTraktSettings(query.data?.settings);
  const data = query.data && shownSettings ? { ...query.data, settings: shownSettings } : query.data;
  const screen = data ? traktScreen(data, data.pending && approved === codeKey(data.pending) ? data.pending.user_code : null) : null;
  const container = useScreenFocus(screen);
  useTraktToasts(query.data);

  if (query.isPending) return <TraktSkeleton />;

  if (!data) {
    return (
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load Trakt"
        action={
          <Button size="sm" onClick={() => void query.refetch()}>
            <RefreshCw size={15} aria-hidden="true" /> Try again
          </Button>
        }
      >
        {query.error?.message}
      </Banner>
    );
  }

  const startConnect = () => connect.mutate();
  const startSync = () =>
    sync.mutate(undefined, { onError: (error) => toast({ tone: 'error', title: 'Couldn’t start a sync', body: error.message }) });
  const cancel = () =>
    disconnect.mutate(undefined, {
      onSuccess: () => setApproved(null),
      onError: (error) => toast({ tone: 'error', title: 'Couldn’t cancel the code', body: error.message }),
    });
  const changeSetting = (field: keyof TraktSettings, value: boolean) =>
    settings.mutateAsync({ [field]: value }).then(
      () => undefined,
      (error: Error) => {
        toast({ tone: 'error', title: 'Couldn’t save that setting', body: error.message });
        throw error;
      }
    );
  const disconnectTrakt = async () => {
    const named = members.current && members.members.length >= 2 ? members.current.name : null;
    const confirmed = await confirm({ ...disconnectConfirmCopy(named), confirmLabel: 'Disconnect', tone: 'danger' });
    if (!confirmed) return;
    disconnect.mutate(undefined, {
      onSuccess: () => toast({ tone: 'info', icon: Link2, ...DISCONNECTED_TOAST }),
      onError: (error) => toast({ tone: 'error', title: 'Couldn’t disconnect Trakt', body: error.message }),
    });
  };

  // Why the last code didn't connect, or why there's no code at all
  const connectProblem =
    screen === 'off' || screen === 'reauth' || screen === 'unconfigured'
      ? connect.error
        ? { tone: 'error' as const, title: 'Couldn’t get a code from Trakt', body: `${connect.error.message} Try again in a moment.` }
        : data.connect_error
          ? connectErrorCopy(data.connect_error)
          : null
      : null;

  return (
    <div ref={container} className="flex flex-col gap-4 lg:gap-5">
      <TraktMemberNote />
      {connectProblem && (
        <Banner tone={connectProblem.tone} role={connectProblem.tone === 'error' ? 'alert' : 'status'} title={connectProblem.title}>
          {connectProblem.body}
        </Banner>
      )}
      {(screen === 'off' || screen === 'unconfigured') && <TraktOff unconfigured={screen === 'unconfigured'} connecting={connect.isPending} onConnect={startConnect} />}
      {screen === 'code' && data.pending && (
        <TraktDeviceCode
          code={data.pending}
          cancelling={disconnect.isPending}
          onCancel={cancel}
          onApproved={() => data.pending && setApproved(codeKey(data.pending))}
        />
      )}
      {screen === 'waiting' && data.pending && (
        <TraktWaiting code={data.pending} cancelling={disconnect.isPending} onCancel={cancel} onShowCode={() => setApproved(null)} />
      )}
      {(screen === 'connected' || screen === 'reauth') && (
        <TraktConnected
          integration={data}
          starting={sync.isPending}
          onSync={startSync}
          reconnecting={connect.isPending}
          onReconnect={startConnect}
          onSetting={changeSetting}
          disconnecting={disconnect.isPending}
          onDisconnect={() => void disconnectTrakt()}
        />
      )}
    </div>
  );
}
