import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { Suspense } from 'react';
import { MappingReview } from '@/components/mapping';
import './mapping.css';

export const metadata: Metadata = { title: 'Episode mapping' };

/**
 * /mapping/{job_id}: check how a torrent's files map to episodes, then apply
 * (PLAN F4). A task page: `.task-page` hides the shell's phone bar and tab bar
 * below 1024 px from the first server render (shell.css).
 */
export default async function MappingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!/^\d+$/.test(id)) notFound();
  return (
    <div className="task-page">
      <Suspense fallback={null}>
        <MappingReview jobId={Number(id)} />
      </Suspense>
    </div>
  );
}
