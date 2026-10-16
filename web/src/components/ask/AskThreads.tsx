'use client';

import Link from 'next/link';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { MessageCircle, Plus } from 'lucide-react';
import { ButtonLink, Progress, Skeleton } from '@/components/ui';
import { aiUsageQuery, askThreadsQuery } from '@/lib/api/endpoints';
import { formatUsd, threadWhen } from '@/lib/ask/copy';
import { ASK_ROUTE, askThreadHref } from '@/lib/recommendations/ask-href';

/** The rail, the history sheet and the composer read usage; none of them needs it polled. */
export const askUsageQuery = () => ({ ...aiUsageQuery(), refetchInterval: false as const });

/** Recent conversations (ask.js `threadList`), the newest first. */
export function AskThreadList({ current, onPick }: { current: number | null; onPick?: () => void }) {
  const threads = useQuery(askThreadsQuery());
  const [now] = useState(() => Date.now());
  if (threads.isPending) {
    return (
      <div className="ask-threads" aria-hidden="true">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} height={52} />
        ))}
      </div>
    );
  }
  if (threads.isError) return <p className="ask-threads-none t-small text-fg-3">Couldn’t load your conversations.</p>;
  if (threads.data.items.length === 0) return <p className="ask-threads-none t-small text-fg-3">No conversations yet.</p>;
  return (
    <ul className="ask-threads">
      {threads.data.items.map((thread) => (
        <li key={thread.id}>
          <Link href={askThreadHref(thread.id)} aria-current={thread.id === current ? 'page' : undefined} onClick={onPick}>
            <MessageCircle size={16} aria-hidden="true" />
            <span className="clamp-2">{thread.title}</span>
            <span className="when">{threadWhen(thread.updated_at, now)}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/** "AI this month · $1.84 of $5.00" with its bar and a way to Settings › AI (ask.js `budget`). */
export function AskBudget() {
  const usage = useQuery(askUsageQuery());
  if (usage.isError) return null;
  if (!usage.data) return <Skeleton className="ask-budget" height={104} />;
  const { total_usd: total, budget_usd: budget } = usage.data;
  const pct = budget ? Math.min(100, (total / budget) * 100) : 0;
  return (
    <div className="ask-budget">
      <div className="flex items-center justify-between gap-2 t-small">
        <span className="text-fg-2">AI this month</span>
        <span className="t-num text-fg-1">{budget === null ? `${formatUsd(total)} · no cap` : `${formatUsd(total)} of ${formatUsd(budget)}`}</span>
      </div>
      {budget !== null && <Progress value={pct} tone="accent" label={`AI spend this month: ${formatUsd(total)} of ${formatUsd(budget)}`} />}
      <Link className="t-small text-fg-3" href="/settings?section=ai">
        Settings › AI
      </Link>
    </div>
  );
}

/** "New conversation": on /ask it also clears what's there (a link to the same URL changes nothing). */
export function NewConversation({ onNew, className }: { onNew: () => void; className?: string }) {
  return (
    <ButtonLink href={ASK_ROUTE} block className={className} onClick={onNew}>
      <Plus size={18} aria-hidden="true" />
      New conversation
    </ButtonLink>
  );
}
