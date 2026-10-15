import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { Suspense } from 'react';
import { MappingReview } from '@/components/mapping';
import './mapping.css';

export const metadata: Metadata = { title: 'Episode mapping' };

/** /mapping/{job_id}: check how a torrent's files map to episodes, then apply (PLAN F4). */
export default async function MappingPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!/^\d+$/.test(id)) notFound();
  return (
    <Suspense fallback={null}>
      <MappingReview jobId={Number(id)} />
    </Suspense>
  );
}
