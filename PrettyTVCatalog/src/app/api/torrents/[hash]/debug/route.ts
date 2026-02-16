import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { TorrentDebugInfo } from '@/types/torrent-debug';

interface RouteParams {
  params: Promise<{ hash: string }>;
}

/**
 * GET /api/torrents/:hash/debug
 * Get detailed piece-level debug info for a torrent.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<TorrentDebugInfo | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { hash } = await params;

    if (!hash) {
      throw new ValidationError('Torrent hash is required');
    }

    const debugInfo = await momoshtremClient.getTorrentDebugInfo(hash);
    return NextResponse.json(debugInfo);
  } catch (error) {
    console.error('Torrent debug fetch error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to fetch torrent debug info' },
      { status: 500 }
    );
  }
}
