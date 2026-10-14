'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Clipboard, RefreshCw, WifiOff } from 'lucide-react';
import { Banner, Button, copyText, Panel, Skeleton, StatusPill, useToast } from '@/components/ui';
import { isApiError } from '@/lib/api/client';
import { statusQuery, systemApi, systemKeys } from '@/lib/api/endpoints/system';
import { serviceRows, summarize } from './status-model';

const timeFormat = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });

/** System status (screens/settings/status): every service momoshtrem depends on. */
export function StatusSection() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const { data, isPending, error } = useQuery(statusQuery());
  const [rechecking, setRechecking] = useState(false);

  async function recheck() {
    setRechecking(true);
    try {
      await queryClient.fetchQuery({ queryKey: systemKeys.status, queryFn: () => systemApi.status(true), staleTime: 0 });
    } catch {
      // the query's error state shows it
    } finally {
      setRechecking(false);
    }
  }

  if (isPending) {
    return (
      <div className="flex flex-col gap-3" aria-busy="true">
        <Skeleton height={64} />
        <Skeleton height={420} />
      </div>
    );
  }

  const unreachable = isApiError(error) && (error.code === 'backend_unreachable' || error.code === 'timeout');
  const errorBanner = (
    <Banner
      tone="error"
      role="alert"
      icon={WifiOff}
      title={unreachable ? 'Can’t reach momoshtrem' : 'Couldn’t check the services'}
      action={
        unreachable ? (
          <Link href="/offline?from=%2Fsettings%3Fsection%3Dstatus" className="btn btn-secondary btn-sm">
            Details
          </Link>
        ) : (
          <Button size="sm" loading={rechecking} onClick={recheck}>
            Try again
          </Button>
        )
      }
    >
      {data ? `Below is the last successful check, from ${timeFormat.format(new Date(data.checked_at))}.` : error?.message}
    </Banner>
  );

  if (!data) return errorBanner;

  const rows = serviceRows(data);
  const summary = summarize(rows);

  return (
    <div className="flex flex-col gap-5">
      {/* A failed re-check keeps the last rows, but never the last "All good" */}
      {error ? (
        errorBanner
      ) : (
        <Banner tone={summary.tone} role="status" title={summary.title}>
          {summary.body}
        </Banner>
      )}
      <Panel className="overflow-hidden">
        <ul aria-label="Services">
          {rows.map((row) => {
            const Icon = row.icon;
            return (
              <li key={row.id} className="grid grid-cols-[36px_minmax(0,1fr)_auto] items-center gap-3 border-b border-line-1 px-4 py-3 sm:px-5">
                <span className="grid size-9 place-items-center rounded-sm bg-surface-2 text-fg-2" aria-hidden="true">
                  <Icon size={18} />
                </span>
                <div className="flex min-w-0 flex-col">
                  <span className="font-semibold text-fg-1">{row.name}</span>
                  <span className="t-small wrap-any text-fg-3">{row.detail}</span>
                </div>
                <StatusPill status={row.pill.status} label={row.pill.label} />
              </li>
            );
          })}
        </ul>
        <div className="flex flex-wrap items-center gap-2 px-4 py-3 sm:px-5">
          <p className="t-small mr-auto text-fg-3">Checked {timeFormat.format(new Date(data.checked_at))}</p>
          <Button
            variant="ghost"
            size="sm"
            onClick={async () => {
              const ok = await copyText(JSON.stringify(data, null, 2));
              toast(ok ? { tone: 'ready', icon: Clipboard, title: 'Diagnostics copied', body: 'No keys or passwords are included.' } : { tone: 'error', title: 'Couldn’t copy' });
            }}
          >
            <Clipboard size={15} aria-hidden="true" /> Copy diagnostics
          </Button>
          <Button size="sm" loading={rechecking} onClick={recheck}>
            <RefreshCw size={15} aria-hidden="true" /> Re-check
          </Button>
        </div>
      </Panel>
    </div>
  );
}
