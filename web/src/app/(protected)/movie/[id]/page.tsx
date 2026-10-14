import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import { Suspense } from 'react';
import { MovieDetail } from '@/components/title';

export const metadata: Metadata = { title: 'Movie' };

/** /movie/{tmdb_id}: the movie's detail page (PLAN F2). */
export default async function MoviePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!/^\d+$/.test(id)) notFound();
  return (
    <Suspense fallback={null}>
      <MovieDetail tmdbId={Number(id)} />
    </Suspense>
  );
}
