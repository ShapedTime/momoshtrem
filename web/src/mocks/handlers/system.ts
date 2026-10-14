import { http, HttpResponse } from 'msw';
import { systemStatus } from '../fixtures/system';
import { latency } from './util';

export const systemHandlers = [
  http.get('*/api/status', async () => {
    await latency();
    return HttpResponse.json(systemStatus());
  }),
];
