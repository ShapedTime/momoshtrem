import { NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { TorrentStatus } from '@/types/torrent';

/**
 * GET /api/torrents
 * List all active torrents with their status.
 */
export async function GET(): Promise<NextResponse<{ torrents: TorrentStatus[] } | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const torrents = await momoshtremClient.getTorrents();
    return NextResponse.json({ torrents });
  } catch (error) {
    console.error('Torrents fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch torrents' },
      { status: 500 }
    );
  }
}
