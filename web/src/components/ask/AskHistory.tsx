'use client';

import { usePathname } from 'next/navigation';
import { ASK_NEW_EVENT, askRouteThread } from '@/lib/ask/route';
import { AskBudget, AskThreadList, NewConversation } from './AskThreads';

/** The phone's "Past conversations" sheet (ask.js `ask-history`): new, recent threads and the month's AI spend. */
export function AskHistory() {
  const pathname = usePathname();
  const current = pathname.startsWith('/ask/') ? askRouteThread(pathname) : null;
  return (
    <div className="ask-hist">
      <NewConversation onNew={() => window.dispatchEvent(new Event(ASK_NEW_EVENT))} />
      <AskThreadList current={current} />
      <AskBudget />
    </div>
  );
}
