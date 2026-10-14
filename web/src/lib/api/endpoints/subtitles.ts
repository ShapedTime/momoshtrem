import { queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type { DownloadSubtitleRequest, Subtitle, SubtitleItemType, SubtitleSearchResult } from '../contract';

/** Subtitles for one movie or episode (PLAN §6 "Subtitles"; #4). */
export const subtitleKeys = {
  all: ['subtitles'] as const,
  item: (type: SubtitleItemType, id: number) => ['subtitles', type, id] as const,
  /** Not under `item`: invalidating the list after a download shouldn't search OpenSubtitles again. */
  search: (type: SubtitleItemType, id: number, languages: string[]) => ['subtitles', 'search', type, id, languages.join(',')] as const,
};

const itemPath = (type: SubtitleItemType, id: number) => `/api/${type === 'movie' ? 'movies' : 'episodes'}/${id}/subtitles`;

export const subtitlesApi = {
  list: (type: SubtitleItemType, id: number) => apiJson<{ items: Subtitle[] }>(itemPath(type, id)),
  search: (type: SubtitleItemType, id: number, languages: string[]) =>
    apiJson<SubtitleSearchResult>(`${itemPath(type, id)}/search${toQuery({ languages: languages.join(',') })}`),
  download: (type: SubtitleItemType, id: number, body: DownloadSubtitleRequest) =>
    apiJson<Subtitle>(itemPath(type, id), { method: 'POST', json: body }),
  remove: (type: SubtitleItemType, id: number, subtitleId: number) =>
    apiJson<void>(`${itemPath(type, id)}/${subtitleId}`, { method: 'DELETE' }),
};

export const itemSubtitlesQuery = (type: SubtitleItemType, id: number) =>
  queryOptions({ queryKey: subtitleKeys.item(type, id), queryFn: () => subtitlesApi.list(type, id) });

/** Results only change when OpenSubtitles does; keep them for 10 minutes. */
export const subtitleSearchQuery = (type: SubtitleItemType, id: number, languages: string[]) =>
  queryOptions({
    queryKey: subtitleKeys.search(type, id, languages),
    queryFn: () => subtitlesApi.search(type, id, languages),
    staleTime: 10 * 60_000,
  });
