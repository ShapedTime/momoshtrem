import { keepPreviousData, queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type { LibraryItem, LibraryParams, Paginated } from '../contract';

/** Library (PLAN §6; #7). */
export const libraryKeys = {
  all: ['library'] as const,
  list: (params: LibraryParams) => ['library', params] as const,
};

export const libraryApi = {
  list: (params: LibraryParams = {}) => apiJson<Paginated<LibraryItem>>(`/api/library${toQuery(params)}`),
};

export const libraryQuery = (params: LibraryParams = {}) =>
  queryOptions({ queryKey: libraryKeys.list(params), queryFn: () => libraryApi.list(params), placeholderData: keepPreviousData });
