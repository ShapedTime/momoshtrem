import type { AvailabilityStage, MissingRun, ShowTitle, Title } from '@/lib/api/contract';
import { formatBytes, formatSpeed, plural } from './format';
import { releaseHighlights } from './release';

/**
 * The primary action state machine on title pages (HANDOFF §5.3):
 * Add → Source → Download → Ready. The server decides the stage
 * (`library.availability_stage`); this only turns it into words and
 * buttons, with the prototype's copy (design-cinematic/js/screens/title.js).
 * A file comes from a torrent or, when it's set up, HDRezka (#63); the stage
 * keeps its name, `needs_torrent`.
 */

export const STAGE_STEPS = ['Add', 'Source', 'Download', 'Ready'] as const;

const STEP_INDEX: Record<AvailabilityStage, number> = { none: 0, needs_torrent: 1, downloading: 2, ready: 3 };

export type StageIcon = 'plus' | 'search' | 'activity' | 'tv' | 'stream' | 'keep' | 'kept';

/** What a button does; the page maps each intent to a handler. `keep` opens the Keep sheet (#206): a new Keep, or a change. */
export type StageIntent = 'add' | 'find' | 'hdrezka' | 'view_download' | 'how_to_play' | 'keep';

export interface StageButton {
  label: string;
  icon: StageIcon;
  intent: StageIntent;
  variant: 'primary' | 'secondary' | 'ghost';
  /** Navigation instead of an action. */
  href?: string;
}

export interface StageCopy {
  stage: AvailabilityStage;
  /** Index into STAGE_STEPS. */
  step: number;
  title: string;
  /** Short facts before the hint, joined with " · " ("2160p · 18.4 GB"). */
  facts: string[];
  /** A bold first phrase of the hint. */
  lead: string | null;
  hint: string;
  /** Downloading: 0–100. */
  pct: number | null;
  /** Ready: the WebDAV path to show, and the URL its Copy button copies. */
  path: { display: string; copy: string } | null;
  /** The panel's main button; on phone it moves to the action dock. */
  primary: StageButton;
  /** A quieter second action under the primary (desktop panel only). */
  secondary: StageButton | null;
  /** The phone dock: one button, plus one icon action: copy the path when ready, or HDRezka while a file is needed. */
  dock: { button: StageButton; copyPath: boolean; icon: StageButton | null };
}

export interface StageOptions {
  /** HDRezka is set up (`status.hdrezka.configured`): offer it next to torrents. */
  hdrezka?: boolean;
}

const ADD: StageButton = { label: 'Add to library', icon: 'plus', intent: 'add', variant: 'primary' };
const FIND: StageButton = { label: 'Find a torrent', icon: 'search', intent: 'find', variant: 'primary' };
const HDREZKA: StageButton = { label: 'Stream from HDRezka', icon: 'stream', intent: 'hdrezka', variant: 'secondary' };
const VIEW_DOWNLOAD: StageButton = { label: 'View download', icon: 'activity', intent: 'view_download', variant: 'secondary', href: '/activity' };
const HOW_TO_PLAY: StageButton = { label: 'How to play in Infuse', icon: 'tv', intent: 'how_to_play', variant: 'primary' };
const PLAY_IN_INFUSE: StageButton = { ...HOW_TO_PLAY, label: 'Play in Infuse' };
const KEEP: StageButton = { label: 'Download & keep', icon: 'keep', intent: 'keep', variant: 'secondary' };
const CHANGE_KEEP: StageButton = { label: 'Change duration', icon: 'kept', intent: 'keep', variant: 'secondary' };

/**
 * A ready movie's second action (#206): Download & keep, or once it's kept, change how long. A
 * stream can't be kept (HDRezka plays from the CDN), and shows keep from the ⋯ menu and seasons.
 */
function keepButton(title: Title): StageButton | null {
  if (title.media_type !== 'movie' || !title.library.torrent) return null;
  // `keep` is missing until the backend sends it (#204): nothing kept yet
  return title.library.keep?.reason === 'kept' ? CHANGE_KEEP : KEEP;
}

const FREE = 'Adding is free: nothing downloads until you pick a torrent.';
const STREAMS = 'Pick a release and it streams to Infuse while it downloads.';

const pad = (n: number) => String(n).padStart(2, '0');

/**
 * "Episodes 29–38" / "Episode 29": absolute numbers for absolute ordering,
 * in-season numbers for a single-season show, else "S02E08–E10".
 */
export function missingLabel(show: Pick<ShowTitle, 'episode_ordering' | 'seasons'>, missing: MissingRun): { subject: string; plural: boolean } {
  const many = missing.count > 1;
  if (show.episode_ordering === 'absolute' && missing.absolute_from !== null) {
    const to = missing.absolute_to ?? missing.absolute_from;
    return { subject: many ? `Episodes ${missing.absolute_from}–${to}` : `Episode ${missing.absolute_from}`, plural: many };
  }
  const regularSeasons = show.seasons.filter((season) => season.season_number > 0).length;
  if (regularSeasons <= 1) {
    return { subject: many ? `Episodes ${missing.from}–${missing.to}` : `Episode ${missing.from}`, plural: many };
  }
  const first = `S${pad(missing.season_number)}E${pad(missing.from)}`;
  return { subject: many ? `${first}–E${pad(missing.to)}` : first, plural: many };
}

/** Lower-case the leading "Episodes" for use after a verb ("Find episodes 29–38"). */
const asObject = (subject: string) => subject.replace(/^Episode/, 'episode');

function noneCopy(title: Title): Pick<StageCopy, 'title' | 'hint' | 'primary'> {
  if (title.media_type === 'movie') return { title: 'Not in your library', hint: `Add it, then pick a torrent. ${FREE}`, primary: ADD };
  const regular = title.seasons.filter((season) => season.season_number > 0);
  const episodes = regular.reduce((sum, season) => sum + season.episodes_total, 0);
  const size = regular.length ? `${plural(episodes, 'episode')} across ${plural(regular.length, 'season')}. ` : '';
  return { title: 'Not in your library', hint: `${size}${FREE}`, primary: ADD };
}

function needsTorrentCopy(title: Title): Pick<StageCopy, 'title' | 'hint' | 'primary'> {
  if (title.media_type === 'movie') return { title: 'In your library · no torrent yet', hint: STREAMS, primary: FIND };
  const { missing, episodes_ready } = title.library;
  if (!episodes_ready) {
    return { title: 'In your library · no episodes yet', hint: 'A season pack usually covers them all in one go.', primary: FIND };
  }
  if (!missing) return { title: 'Some episodes have no torrent', hint: STREAMS, primary: FIND };
  const { subject, plural: many } = missingLabel(title, missing);
  return {
    title: `${subject} ${many ? 'have' : 'has'} no torrent`,
    hint: many ? 'A season pack usually covers them all in one go.' : STREAMS,
    primary: { ...FIND, label: `Find ${asObject(subject)}` },
  };
}

function downloadingFacts(title: Title): string[] {
  const download = title.library.download;
  if (!download) return [];
  // Speeds stay 0 until B6 reports them, and seeders are unknown until the torrent loads
  return [
    download.speed_bps > 0 ? formatSpeed(download.speed_bps) : null,
    download.seeders > 0 ? plural(download.seeders, 'seeder') : null,
  ].filter((fact): fact is string => fact !== null);
}

function readyFacts(title: Title): string[] {
  if (title.media_type === 'tv') {
    const { episodes_ready, episodes_total } = title.library;
    return episodes_ready !== null && episodes_total !== null ? [`${episodes_ready} of ${plural(episodes_total, 'episode')} ready`] : [];
  }
  const { torrent, stream } = title.library;
  if (stream) return ['HDRezka', stream.translator, stream.quality, formatBytes(stream.size_bytes)];
  if (!torrent) return [];
  return [torrent.resolution, formatBytes(torrent.size_bytes), ...releaseHighlights(torrent.release_name)].filter(
    (fact): fact is string => Boolean(fact)
  );
}

/** The panel, buttons and dock for a title's stage. Pure: same title and options, same copy. */
export function primaryActionFor(title: Title, options: StageOptions = {}): StageCopy {
  const stage = title.library.availability_stage;
  const base = { stage, step: STEP_INDEX[stage], facts: [], lead: null, pct: null, path: null, secondary: null };

  switch (stage) {
    case 'none': {
      const copy = noneCopy(title);
      return { ...base, ...copy, dock: { button: copy.primary, copyPath: false, icon: null } };
    }
    case 'needs_torrent': {
      const copy = needsTorrentCopy(title);
      const hdrezka = options.hdrezka ? HDREZKA : null;
      return { ...base, ...copy, secondary: hdrezka, dock: { button: copy.primary, copyPath: false, icon: hdrezka } };
    }
    case 'downloading':
      // Torrents fetch on demand, so there's no finish time to promise: say it streams now
      return {
        ...base,
        title: 'Downloading',
        facts: downloadingFacts(title),
        lead: 'You can start watching in Infuse now;',
        hint: title.media_type === 'movie' ? 'it streams while it downloads.' : 'episodes stream while they download.',
        pct: Math.round(title.library.download?.pct ?? 0),
        primary: VIEW_DOWNLOAD,
        secondary: { ...HOW_TO_PLAY, variant: 'ghost' },
        dock: { button: VIEW_DOWNLOAD, copyPath: false, icon: null },
      };
    case 'ready': {
      const { webdav_path, webdav_url } = title.library;
      const path = webdav_path ? { display: webdav_path, copy: webdav_url ?? webdav_path } : null;
      return {
        ...base,
        title: 'Ready in Infuse',
        facts: readyFacts(title),
        hint: '',
        path,
        primary: HOW_TO_PLAY,
        secondary: keepButton(title),
        dock: { button: PLAY_IN_INFUSE, copyPath: path !== null, icon: null },
      };
    }
  }
}
