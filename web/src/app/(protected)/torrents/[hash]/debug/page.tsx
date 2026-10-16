import { redirect } from 'next/navigation';

/** The legacy debug page became the torrent's detail on Activity (#102). */
export default async function TorrentDebugPage({ params }: { params: Promise<{ hash: string }> }) {
  const { hash } = await params;
  redirect(`/activity/${encodeURIComponent(hash)}`);
}
