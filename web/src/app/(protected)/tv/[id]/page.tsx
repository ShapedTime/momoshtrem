import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { Suspense } from 'react';
import { ShowDetail } from '@/components/title';

export const metadata: Metadata = { title: 'Show' };

/** /tv/{tmdb_id}: the show's detail page (PLAN F2); `?season=` picks the season tab. */
export default async function ShowPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!/^\d+$/.test(id)) notFound();
  return (
    <Suspense fallback={null}>
      <ShowDetail tmdbId={Number(id)} />
    </Suspense>
  );
}
