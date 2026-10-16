import type { Metadata } from 'next';
import { Suspense } from 'react';
import { TorrentDetailView } from '@/components/activity';
import '../activity.css';

export const metadata: Metadata = { title: 'Torrent' };

/**
 * /activity/{info_hash}: one torrent's health, files and pieces (PLAN F7, #102).
 * The tab names it once it loads; `?advanced=1` opens Advanced.
 */
export default async function TorrentPage({ params }: { params: Promise<{ hash: string }> }) {
  const { hash } = await params;
  return (
    <Suspense fallback={null}>
      <TorrentDetailView hash={decodeURIComponent(hash)} />
    </Suspense>
  );
}
