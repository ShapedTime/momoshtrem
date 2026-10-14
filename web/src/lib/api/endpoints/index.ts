/**
 * Typed, browser-side calls for every PLAN §6 endpoint, plus TanStack Query
 * keys and `queryOptions` factories. One file per domain, so each feature
 * issue owns its own. Use with `useQuery(titleQuery('tv', 209867))`.
 */
export * from './titles';
export * from './library';
export * from './assignment';
export * from './search';
export * from './trakt';
export * from './recommendations';
export * from './settings';
export * from './system';
export * from './torrents';
