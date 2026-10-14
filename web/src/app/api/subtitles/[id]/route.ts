import { NextRequest, NextResponse } from 'next/server';
import { momoshtremClient } from '@/lib/api/momoshtrem';
import { isAppError } from '@/lib/errors';
import { requireAuth } from '@/lib/api/auth-guard';

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * DELETE /api/subtitles/:id
 * Delete a subtitle.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse<null | { error: string }>> {
  const authError = await requireAuth();
  if (authError) return authError;

  try {
    const { id } = await params;
    const subtitleId = parseInt(id, 10);

    if (isNaN(subtitleId)) {
      return NextResponse.json({ error: 'Invalid subtitle ID' }, { status: 400 });
    }

    await momoshtremClient.deleteSubtitle(subtitleId);

    return new NextResponse(null, { status: 204 });
  } catch (error) {
    console.error('Subtitle delete error:', error);

    if (isAppError(error)) {
      return NextResponse.json({ error: error.message }, { status: error.statusCode });
    }

    return NextResponse.json({ error: 'Failed to delete subtitle' }, { status: 500 });
  }
}
