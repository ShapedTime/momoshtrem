import { setMockScenario } from '../scenario';
import { assignmentHandlers, resetMockJobs } from './assignment';
import { discoverHandlers } from './discover';
import { hdrezkaHandlers, resetMockHDRezka } from './hdrezka';
import { legacyHandlers } from './legacy';
import { libraryHandlers } from './library';
import { recommendationsHandlers } from './recommendations';
import { resetMockSearch, searchHandlers } from './search';
import { resetMockSettings, settingsHandlers } from './settings';
import { resetMockSubtitles, subtitlesHandlers } from './subtitles';
import { systemHandlers } from './system';
import { titleSearchHandlers } from './title-search';
import { resetMockLibrary, titlesHandlers } from './titles';
import { resetMockTorrents, torrentsHandlers } from './torrents';
import { resetMockTrakt, traktHandlers } from './trakt';

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
  ...discoverHandlers,
  ...titleSearchHandlers,
  ...legacyHandlers,
];

/** Tests: forget what earlier requests changed (the library, mapping jobs, AI settings, the quality profile, stored subtitles, torrent pauses and the Trakt account live per page). */
export function resetMockState(): void {
  resetMockLibrary();
  resetMockSettings();
  resetMockJobs();
  resetMockSubtitles();
  resetMockHDRezka();
  resetMockSearch();
  resetMockTorrents();
  resetMockTrakt();
  setMockScenario(null);
}
