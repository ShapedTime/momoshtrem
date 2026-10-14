import { http, HttpResponse } from 'msw';
import type { EpisodeGroup, ShowSettingsPatch } from '@/lib/api/contract';
import { buildTitle } from '../fixtures/titles';
import { latency, notFound } from './util';

export const titlesHandlers = [
  http.get('*/api/titles/:mediaType/:tmdbId', async ({ params }) => {
    await latency();
    const title = buildTitle(params.mediaType as 'movie' | 'tv', Number(params.tmdbId));
    return title ? HttpResponse.json(title) : notFound('No such title');
  }),
  http.patch('*/api/shows/:id', async ({ request }) => {
    const patch = (await request.json()) as ShowSettingsPatch;
    const show = buildTitle('tv', 209867);
    return HttpResponse.json({ ...show, ...patch });
  }),
  http.get('*/api/shows/:id/episode-groups', async () => {
    await latency();
    const items: EpisodeGroup[] = [
      { id: '5eb730dd0d3f2c0020b4e3d4', name: 'Absolute order', type: 2, group_count: 1, episode_count: 38 },
      { id: '6501a1b2c3d4e5f6a7b8c9d0', name: 'Cours', type: 6, group_count: 3, episode_count: 38 },
    ];
    return HttpResponse.json({ items });
  }),
];
