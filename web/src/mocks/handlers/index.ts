import { setMockScenario } from '../scenario';
import { assignmentHandlers, resetMockJobs } from './assignment';
import { hdrezkaHandlers, resetMockHDRezka } from './hdrezka';
import { legacyHandlers } from './legacy';
import { libraryHandlers } from './library';
import { recommendationsHandlers } from './recommendations';
import { searchHandlers } from './search';
import { resetMockSettings, settingsHandlers } from './settings';
import { resetMockSubtitles, subtitlesHandlers } from './subtitles';
import { systemHandlers } from './system';
import { resetMockLibrary, titlesHandlers } from './titles';
import { torrentsHandlers } from './torrents';
import { traktHandlers } from './trakt';

/**
 * Mock handlers for every PLAN §6 endpoint, one file per domain (feature
 * issues extend their own). Paths start with `*` so they match any origin:
 * the browser's and Node's (where relative handler paths never match).
 */
export const handlers = [
  ...titlesHandlers,
  ...subtitlesHandlers,
  ...libraryHandlers,
  ...assignmentHandlers,
  ...searchHandlers,
  ...traktHandlers,
  ...recommendationsHandlers,
  ...settingsHandlers,
  ...systemHandlers,
  ...torrentsHandlers,
  ...hdrezkaHandlers,
  ...legacyHandlers,
];

/** Tests: forget what earlier requests changed (the library, mapping jobs, AI settings, the quality profile and stored subtitles live per page). */
export function resetMockState(): void {
  resetMockLibrary();
  resetMockSettings();
  resetMockJobs();
  resetMockSubtitles();
  resetMockHDRezka();
  setMockScenario(null);
}
