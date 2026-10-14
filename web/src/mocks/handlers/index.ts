import { assignmentHandlers } from './assignment';
import { legacyHandlers } from './legacy';
import { libraryHandlers } from './library';
import { recommendationsHandlers } from './recommendations';
import { searchHandlers } from './search';
import { settingsHandlers } from './settings';
import { systemHandlers } from './system';
import { titlesHandlers } from './titles';
import { torrentsHandlers } from './torrents';
import { traktHandlers } from './trakt';

/**
 * Mock handlers for every PLAN §6 endpoint, one file per domain (feature
 * issues extend their own). Paths start with `*` so they match any origin:
 * the browser's and Node's (where relative handler paths never match).
 */
export const handlers = [
  ...titlesHandlers,
  ...libraryHandlers,
  ...assignmentHandlers,
  ...searchHandlers,
  ...traktHandlers,
  ...recommendationsHandlers,
  ...settingsHandlers,
  ...systemHandlers,
  ...torrentsHandlers,
  ...legacyHandlers,
];
