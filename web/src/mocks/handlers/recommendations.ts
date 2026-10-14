import { http, HttpResponse } from 'msw';
import { ASK_EVENTS, REC_HOME, REC_RUN, THREADS, thread } from '../fixtures/recommendations';
import { accepted, latency } from './util';

export const recommendationsHandlers = [
  http.get('*/api/recommendations/home', async () => {
    await latency();
    return HttpResponse.json(REC_HOME);
  }),
  http.post('*/api/recommendations/refresh', () => accepted({ run_id: REC_RUN.id })),
  http.get('*/api/recommendations/runs/:id', () => HttpResponse.json(REC_RUN)),
  http.get('*/api/recommendations/threads', () => HttpResponse.json({ items: THREADS })),
  http.get('*/api/recommendations/threads/:id', ({ params }) => HttpResponse.json(thread(Number(params.id)))),
  http.post('*/api/recommendations/feedback', () => new HttpResponse(null, { status: 204 })),
  // Ask streams Server-Sent Events: status → item → text deltas → done
  http.post('*/api/recommendations/ask', () => {
    const encoder = new TextEncoder();
    const stream = new ReadableStream({
      async start(controller) {
        for (const { event, data } of ASK_EVENTS) {
          controller.enqueue(encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`));
          await new Promise((resolve) => setTimeout(resolve, typeof window === 'undefined' ? 0 : 300));
        }
        controller.close();
      },
    });
    return new HttpResponse(stream, { headers: { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache, no-transform' } });
  }),
];
