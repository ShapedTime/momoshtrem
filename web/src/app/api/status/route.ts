import { NextResponse, type NextRequest } from 'next/server';
import { requestMemberId } from '@/lib/api/backend-headers';
import { getWebStatus } from '@/lib/api/status';

export const dynamic = 'force-dynamic';

/**
 * GET /api/status: the backend's /status plus the web version. Shadows the
 * catch-all proxy so an unreachable backend still gets a useful answer (503
 * with whether WebDAV answers).
 */
export async function GET(request: NextRequest): Promise<NextResponse> {
  const result = await getWebStatus(request.nextUrl.searchParams.get('refresh') === '1', requestMemberId(request));
  if (result.ok) return NextResponse.json(result.status, { headers: { 'Cache-Control': 'no-store' } });
  return NextResponse.json(result.body, { status: result.httpStatus, headers: { 'Cache-Control': 'no-store' } });
}
