import { MOMOSHTREM_CONFIG } from '@/config/momoshtrem';
import packageJson from '../../../package.json';
import type { SystemStatus } from './contract';
import { BACKEND_AUTH_CODE, BACKEND_AUTH_MESSAGE, TOKEN_HEADER } from './momoshtrem';

export const WEB_VERSION: string = packageJson.version;

/** Where the web app can reach WebDAV directly (same host as the API, port 36911 by default). */
export function webdavInternalUrl(): string {
  if (process.env.WEBDAV_INTERNAL_URL) return process.env.WEBDAV_INTERNAL_URL;
  const url = new URL(MOMOSHTREM_CONFIG.baseUrl);
  url.port = '36911';
  url.pathname = '/';
  return url.toString();
}

/**
 * Is WebDAV answering? Any HTTP answer counts, including 401 (auth on): the
 * point is whether Infuse can still play while the REST API is quiet. WebDAV
 * runs in the same process as the API, so usually both are up or both down.
 */
export async function probeWebdav(timeoutMs = 2000): Promise<boolean> {
  try {
    await fetch(webdavInternalUrl(), { method: 'OPTIONS', signal: AbortSignal.timeout(timeoutMs), cache: 'no-store' });
    return true;
  } catch {
    return false;
  }
}

export type WebStatusResult =
  | { ok: true; status: SystemStatus }
  | { ok: false; httpStatus: number; body: { error: { code: string; message: string }; web: { version: string }; webdav?: { reachable: boolean } } };

/**
 * GET /status from the backend, plus the web app's own version. When the
 * backend can't be reached, say whether WebDAV still answers, so the offline
 * page can tell the truth about Infuse.
 */
export async function getWebStatus(refresh: boolean): Promise<WebStatusResult> {
  const url = new URL(`/api/status${refresh ? '?refresh=1' : ''}`, MOMOSHTREM_CONFIG.baseUrl);
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (process.env.BACKEND_API_TOKEN) headers[TOKEN_HEADER] = process.env.BACKEND_API_TOKEN;
  const web = { version: WEB_VERSION };

  let response: Response;
  try {
    response = await fetch(url, { headers, cache: 'no-store', signal: AbortSignal.timeout(10_000) });
  } catch {
    return {
      ok: false,
      httpStatus: 503,
      body: { error: { code: 'backend_unreachable', message: 'Can’t reach momoshtrem.' }, web, webdav: { reachable: await probeWebdav() } },
    };
  }

  if (response.status === 401 || response.status === 403) {
    return { ok: false, httpStatus: 502, body: { error: { code: BACKEND_AUTH_CODE, message: BACKEND_AUTH_MESSAGE }, web } };
  }

  const body = (await response.json().catch(() => null)) as SystemStatus | null;
  if (!body || typeof body.version !== 'string') {
    return { ok: false, httpStatus: 502, body: { error: { code: 'bad_gateway', message: 'momoshtrem answered with something unexpected.' }, web } };
  }
  // 503 means the database is down; the status body still explains the rest
  return { ok: true, status: { ...body, web } };
}
