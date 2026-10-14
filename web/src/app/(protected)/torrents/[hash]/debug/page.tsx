'use client';

import { use } from 'react';
import Link from 'next/link';
import { useTorrentDebug } from '@/hooks/useTorrentDebug';
import { decodePieces } from '@/types/torrent-debug';
import { DebugStatsBar } from '@/components/torrent/debug/DebugStatsBar';
import { PieceBitmap } from '@/components/torrent/debug/PieceBitmap';
import { PieceLegend } from '@/components/torrent/debug/PieceLegend';
import { FileProgressTable } from '@/components/torrent/debug/FileProgressTable';
import { ActiveReaders } from '@/components/torrent/debug/ActiveReaders';

interface PageProps {
  params: Promise<{ hash: string }>;
}

export default function TorrentDebugPage({ params }: PageProps) {
  const { hash } = use(params);
  const { data, error, isLoading } = useTorrentDebug(hash);

  if (isLoading && !data) {
    return (
      <main className="px-4 sm:px-6 lg:px-12 xl:px-16 py-6 sm:py-8 lg:py-12">
        <div className="animate-pulse space-y-6">
          <div className="h-8 bg-bg-elevated rounded w-1/3" />
          <div className="grid grid-cols-6 gap-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="h-16 bg-bg-elevated rounded-lg" />
            ))}
          </div>
          <div className="h-64 bg-bg-elevated rounded-lg" />
        </div>
      </main>
    );
  }

  if (error && !data) {
    return (
      <main className="px-4 sm:px-6 lg:px-12 xl:px-16 py-6 sm:py-8 lg:py-12">
        <div className="text-center py-16">
          <p className="text-accent-red mb-4">{error}</p>
          <Link
            href="/torrents"
            className="text-accent-blue hover:underline"
          >
            Back to Torrents
          </Link>
        </div>
      </main>
    );
  }

  if (!data) return null;

  const pieces = data.pieces ? decodePieces(data.pieces) : [];

  return (
    <main className="px-4 sm:px-6 lg:px-12 xl:px-16 py-6 sm:py-8 lg:py-12 space-y-8">
      {/* Header */}
      <header>
        <div className="flex items-center gap-3 mb-2">
          <Link
            href="/torrents"
            className="text-text-muted hover:text-white transition-colors"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </Link>
          <div>
            <h1 className="text-xl sm:text-2xl font-bold text-white truncate" title={data.name}>
              {data.name}
            </h1>
            <p className="text-sm text-text-muted font-mono mt-0.5">
              {data.info_hash}
            </p>
          </div>
        </div>
        {data.piece_length > 0 && (
          <p className="text-xs text-text-muted">
            {data.num_pieces.toLocaleString()} pieces × {(data.piece_length / (1024 * 1024)).toFixed(1)} MB
          </p>
        )}
      </header>

      {/* Stats bar */}
      <section>
        <DebugStatsBar stats={data.stats} />
      </section>

      {/* Piece bitmap */}
      {pieces.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">Piece Map</h2>
          <PieceLegend />
          <div className="bg-bg-elevated rounded-lg p-4">
            <PieceBitmap
              pieces={pieces}
              files={data.files}
              readers={data.readers}
              pieceLength={data.piece_length}
            />
          </div>
        </section>
      )}

      {/* Active readers */}
      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-white">Active Readers</h2>
        <ActiveReaders readers={data.readers} />
      </section>

      {/* File progress */}
      <section className="space-y-3">
        <h2 className="text-lg font-semibold text-white">Files</h2>
        <div className="bg-bg-elevated rounded-lg p-4">
          <FileProgressTable files={data.files} />
        </div>
      </section>
    </main>
  );
}
