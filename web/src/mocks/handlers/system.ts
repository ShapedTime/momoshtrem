import { http, HttpResponse } from 'msw';
import type { BackendUnreachableBody, SystemStatus } from '@/lib/api/contract';
import { systemStatus } from '../fixtures/system';
import { scenario } from '../scenario';
import { latency } from './util';

/** The HDRezka scenarios show on the status page too. */
function hdrezkaStatus(): Pick<SystemStatus, 'hdrezka'> | undefined {
  const current = scenario();
  if (current === 'hdrezka-off') return { hdrezka: { configured: false, reachable: false } };
  if (current === 'hdrezka-blocked') return { hdrezka: { configured: true, reachable: false, error: 'HDRezka answered with its anti-bot page' } };
  if (current === 'hdrezka-down') return { hdrezka: { configured: true, reachable: false, error: 'timeout after 3s' } };
  return undefined;
}

export const systemHandlers = [
  http.get('*/api/status', async () => {
    await latency();
    if (scenario() === 'backend-down') {
      const body: BackendUnreachableBody = {
        error: { code: 'backend_unreachable', message: 'Can’t reach momoshtrem.' },
        web: { version: '2.0.0' },
        webdav: { reachable: false },
      };
      return HttpResponse.json(body, { status: 503 });
    }
    return HttpResponse.json<SystemStatus>({ ...systemStatus(), ...hdrezkaStatus(), web: { version: '2.0.0' } });
  }),
];
