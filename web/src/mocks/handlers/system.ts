import { http, HttpResponse } from 'msw';
import type { BackendUnreachableBody } from '@/lib/api/contract';
import { systemStatus } from '../fixtures/system';
import { scenario } from '../scenario';
import { latency } from './util';

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
    return HttpResponse.json({ ...systemStatus(), web: { version: '2.0.0' } });
  }),
];
