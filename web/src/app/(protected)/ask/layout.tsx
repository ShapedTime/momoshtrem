import type { Metadata } from 'next';
import { type ReactNode, Suspense } from 'react';
import { AskScreen } from '@/components/ask';
import './ask.css';

export const metadata: Metadata = { title: 'Ask momoshtrem' };

/**
 * Ask momoshtrem (PLAN F5, #156). The screen lives here, not in the pages,
 * so it stays mounted from /ask to /ask/{thread}: a new conversation's
 * stream keeps going while its `start` moves the URL to the thread. A task
 * page: `.task-page` hides the phone bar and tab bar below 1024 px from the
 * first server render (shell.css); the screen draws its own header.
 */
export default function AskLayout({ children }: { children: ReactNode }) {
  return (
    <div className="task-page">
      <Suspense fallback={null}>
        <AskScreen />
      </Suspense>
      {children}
    </div>
  );
}
