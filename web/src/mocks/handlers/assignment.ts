import { http, HttpResponse } from 'msw';
import type { AssignmentJobDetail, TorrentFileEntry } from '@/lib/api/contract';
import { assignmentJob } from '../fixtures/assignment';
import { accepted, latency } from './util';

export const assignmentHandlers = [
  http.post('*/api/shows/:id/assign-torrent', async () => accepted({ job_id: 101 })),
  http.post('*/api/movies/:id/assign-torrent', async () => accepted({ job_id: 102 })),
  http.get('*/api/assignment-jobs/:id', async ({ params }) => {
    await latency();
    return HttpResponse.json(assignmentJob(Number(params.id)));
  }),
  http.patch('*/api/assignment-jobs/:id/proposals/:pid', async ({ params }) => HttpResponse.json(assignmentJob(Number(params.id)))),
  http.post('*/api/assignment-jobs/:id/reclassify', async ({ params }) => accepted({ job_id: Number(params.id) })),
  http.post('*/api/assignment-jobs/:id/apply', async ({ params }) => {
    const detail = assignmentJob(Number(params.id));
    return HttpResponse.json<AssignmentJobDetail>({ ...detail, job: { ...detail.job, status: 'applied' } });
  }),
  http.put('*/api/episodes/:id/assignment', () => new HttpResponse(null, { status: 204 })),
  http.put('*/api/movies/:id/assignment', () => new HttpResponse(null, { status: 204 })),
  http.get('*/api/torrents/:hash/files', async () => {
    const items: TorrentFileEntry[] = assignmentJob(101).groups.matched.map((p) => ({ path: p.file_path, size: p.file_size, mapped_to: p.target }));
    return HttpResponse.json<{ items: TorrentFileEntry[] }>({ items });
  }),
];
