import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError, ValidationError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';
import type { ApiErrorBody } from '@/lib/api/contract';

interface RouteParams {
  params: Promise<{ hash: string }>;
}

/**
 * DELETE /api/torrents/:hash
 * Remove a torrent.
 * Query params:
 *   - delete_data: If 'true', also deletes downloaded data
 */
export async function DELETE(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<{ success: boolean } | { error: string } | ApiErrorBody>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { hash } = await params;

    if (!hash) {
      throw new ValidationError('Torrent hash is required');
    }

    const deleteData = request.nextUrl.searchParams.get('delete_data') === 'true';

    await momoshtremClient.removeTorrent(hash, deleteData);
    return NextResponse.json({ success: true });
  } catch (error) {
    console.error('Torrent remove error:', error);

    if (isAppError(error)) {
      return NextResponse.json(
        { error: error.message },
        { status: error.statusCode }
      );
    }

    return NextResponse.json(
      { error: 'Failed to remove torrent' },
      { status: 500 }
    );
  }
}
