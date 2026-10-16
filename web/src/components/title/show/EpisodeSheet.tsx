'use client';

import { useQuery } from '@tanstack/react-query';
import { HardDriveDownload, MonitorPlay, Pin, RefreshCw, Search, Trash2 } from 'lucide-react';
import { ReplaceMenu } from '@/components/source/ReplaceMenu';
import { Banner, Button, CopyField, FilePath, LoadingRegion, Sheet, Skeleton, SkeletonText, StatusPill } from '@/components/ui';
import { useHDRezkaEnabled } from '@/hooks/useHDRezka';
import { useItemSubtitles } from '@/hooks/useItemSubtitles';
import { useRemoveEpisodeFile } from '@/hooks/useTitle';
import type { EpisodeSummary, Keep, ShowTitle } from '@/lib/api/contract';
import { seasonQuery, titleQuery } from '@/lib/api/endpoints';
import type { SheetComponentProps } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';
import { showSourceParams } from '@/lib/source/context';
import { episodeCode, parseEpisodeCode } from '@/lib/title/episodes';
import { formatDate } from '@/lib/title/format';
import { keepOf } from '@/lib/title/keep';
import { episodeKeepContext, heldByCopy, keepSheetParams } from '@/lib/title/keep-sheet';
import { KeepStatus } from '../KeepStatus';
import { SubtitlesPanel } from '../SubtitlesPanel';

/** `?sheet=episode` params for one episode of a show. */
export const episodeSheetParams = (show: Pick<ShowTitle, 'tmdb_id'>, episode: Pick<EpisodeSummary, 'season_number' | 'episode_number'>) => ({
  tmdb: String(show.tmdb_id),
  se: episodeCode(episode.season_number, episode.episode_number),
});

const hasFile = (episode: EpisodeSummary) => episode.webdav_path !== null;

/** Where the episode's file comes from: "Torrent", or "HDRezka · LostFilm · 1080p" (#63). */
function sourceLine(episode: EpisodeSummary): string | null {
  if (!hasFile(episode)) return null;
  if (episode.source === 'hdrezka') return ['HDRezka', episode.stream?.translator, episode.stream?.quality].filter(Boolean).join(' · ');
  return episode.source === 'torrent' ? 'Torrent' : null;
}

/** "/TV Shows/Frieren (2023)/Season 01/…mkv" → "TV Shows › Frieren (2023) › Season 01". */
const folderTrail = (path: string) => path.split('/').filter(Boolean).slice(0, -1).join(' › ');

function EpisodeBody({ show, episode, close }: { show: ShowTitle; episode: EpisodeSummary; close: () => void }) {
  const code = episodeCode(episode.season_number, episode.episode_number);
  const subtitles = useItemSubtitles('episode', hasFile(episode) ? episode.id : null, episode.webdav_path);
  const { remove, removing } = useRemoveEpisodeFile();
  const hdrezka = useHDRezkaEnabled();
  const sheets = useSheet();
  const scope = { season: episode.season_number, episode: episode.episode_number };
  // The source sheet's tabs, over this sheet in history: Back (or closing it) comes back here
  const find = () => sheets.open('torrent', showSourceParams(show.tmdb_id, scope));
  const streamFromHDRezka = () => sheets.open('hdrezka', showSourceParams(show.tmdb_id, scope));
  const source = sourceLine(episode);
  // Watched wins the state (B5): the file, when there is one, still plays
  const watched = episode.state === 'watched';
  const facts = [episode.runtime ? `${episode.runtime} min` : null, episode.air_date ? formatDate(episode.air_date) : null].filter(Boolean);
  // Download & keep (#206), over this sheet in history too; with a file, or once something keeps it
  const kept = keepOf(episode.keep);
  const keepSection = (hasFile(episode) || kept) && (
    <EpisodeKeep
      episode={episode}
      name={`${show.title} · ${code}`}
      onOpen={() => sheets.open('keep', keepSheetParams(episodeKeepContext(show.tmdb_id, episode)))}
    />
  );

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center gap-2">
        <StatusPill status={episode.state === 'upcoming' && episode.air_date ? 'airs_on' : episode.state} label={episode.state === 'downloading' ? `${Math.round(episode.pct ?? 0)}%` : undefined} />
        {facts.length > 0 && <span className="t-small text-fg-3">{facts.join(' · ')}</span>}
      </div>
      {episode.overview && <p className="text-fg-2">{episode.overview}</p>}

      {episode.webdav_path ? (
        <>
          <section className="flex min-w-0 flex-col gap-2" aria-labelledby="episode-play-h">
            <h3 className="font-semibold" id="episode-play-h">
              Play in Infuse
            </h3>
            <p className="t-small text-fg-2">{folderTrail(episode.webdav_path)}. New episodes appear after Infuse’s next library refresh.</p>
            <CopyField value={episode.webdav_path} label="WebDAV path of this episode" hint="Find it in Infuse under the same folders">
              <FilePath path={episode.webdav_path} />
            </CopyField>
            {source && (
              <p className="t-small text-fg-2">
                Source: <span lang={episode.source === 'hdrezka' ? 'ru' : undefined}>{source}</span>
              </p>
            )}
          </section>
          {keepSection}
          <SubtitlesPanel {...subtitles} />
          <div className="flex flex-wrap gap-2">
            {hdrezka ? (
              <ReplaceMenu onFindTorrent={find} onStreamFromHDRezka={streamFromHDRezka} nested />
            ) : (
              <Button onClick={find}>
                <RefreshCw size={16} aria-hidden="true" /> {episode.source === 'hdrezka' ? 'Replace with a torrent' : 'Replace torrent'}
              </Button>
            )}
            <Button
              variant="danger-quiet"
              loading={removing}
              onClick={async () => {
                if (await remove(episode.id, code)) close();
              }}
            >
              <Trash2 size={16} aria-hidden="true" /> Remove file from episode
            </Button>
          </div>
        </>
      ) : episode.state === 'upcoming' ? (
        <p className="text-fg-2">It hasn’t aired yet. Find a torrent once it’s out.</p>
      ) : (
        <div className="flex flex-col gap-3">
          {watched && <p className="text-fg-2">Trakt has it as watched, but there’s no file for it here. Find one to watch it again in Infuse.</p>}
          <div className="flex flex-wrap gap-2">
            {/* Watched: finding it again isn't the next thing to do, so it's not the amber button */}
            <Button variant={watched ? 'secondary' : 'primary'} onClick={find}>
              <Search size={16} aria-hidden="true" /> Find a torrent for {code}
            </Button>
            {hdrezka && (
              <Button onClick={streamFromHDRezka}>
                <MonitorPlay size={16} aria-hidden="true" /> Stream from HDRezka
              </Button>
            )}
          </div>
        </div>
      )}
      {/* Without a file, only a season or show keep (or a Keep made before it was found) shows here */}
      {!hasFile(episode) && keepSection}
    </div>
  );
}

/** What the episode's Keep button does: a new Keep, its own (a change, or a smart pick to keep), or its season's or show's. */
function keepButtonLabel(keep: Keep | null, seasonNumber: number): string {
  if (!keep) return 'Download & keep';
  if (keep.scope === 'episode') return keep.reason === 'smart' ? 'Keep it' : 'Change duration';
  if (keep.scope === 'show') return 'Change the show’s keep';
  return seasonNumber === 0 ? 'Change the Specials keep' : `Change season ${seasonNumber}’s keep`;
}

/**
 * Keep on disk (#206): what holds the episode ("Kept until watched · by Nov 16",
 * "On disk"), and the button for its Keep sheet: Download & keep, Change
 * duration, Keep it (a smart pick), or the season's or show's keep that holds it.
 */
function EpisodeKeep({ episode, name, onOpen }: { episode: EpisodeSummary; name: string; onOpen: () => void }) {
  const keep = keepOf(episode.keep);
  return (
    <section className="flex min-w-0 flex-col gap-2" aria-labelledby="episode-keep-h">
      <h3 className="font-semibold" id="episode-keep-h">
        {keep ? 'Kept on disk' : 'Download & keep'}
      </h3>
      {keep ? (
        <KeepStatus keep={keep} name={name} />
      ) : (
        <p className="t-small text-fg-2">It streams on demand. Keep it to download it in the background and hold it on disk.</p>
      )}
      {keep && keep.scope !== 'episode' && <p className="t-small text-fg-2">{heldByCopy(keep, episode.season_number)}</p>}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" onClick={onOpen} aria-haspopup="dialog">
          {keep ? <Pin size={15} aria-hidden="true" /> : <HardDriveDownload size={15} aria-hidden="true" />} {keepButtonLabel(keep, episode.season_number)}
        </Button>
      </div>
    </section>
  );
}

/**
 * One episode (`?sheet=episode&tmdb=…&se=S01E05`): how to play it, its
 * subtitles, Replace torrent and Remove file. Reads the show and season
 * the page already has cached, so it also works as a deep link.
 */
export function EpisodeSheet({ params, close }: SheetComponentProps) {
  const tmdbId = Number(params.get('tmdb'));
  const code = parseEpisodeCode(params.get('se'));
  const valid = Number.isInteger(tmdbId) && tmdbId > 0 && code !== null;
  const showQuery = useQuery({ ...titleQuery('tv', tmdbId), enabled: valid });
  const show = showQuery.data?.media_type === 'tv' ? showQuery.data : undefined;
  const seasonResult = useQuery({ ...seasonQuery(tmdbId, code?.season ?? 0), enabled: valid && Boolean(show?.library.in_library) });
  const episode = seasonResult.data?.episodes.find((e) => e.episode_number === code?.episode);
  const failed = showQuery.isError || seasonResult.isError;
  const loading = valid && !failed && (showQuery.isPending || (Boolean(show?.library.in_library) && seasonResult.isPending));
  const label = code ? episodeCode(code.season, code.episode) : 'Episode';
  const retry = () => {
    if (showQuery.isError) void showQuery.refetch();
    if (seasonResult.isError) void seasonResult.refetch();
  };

  return (
    <Sheet
      open
      onOpenChange={(open) => !open && close()}
      title={episode?.name || label}
      description={show ? `${show.title} · ${label}` : undefined}
      size="narrow"
    >
      {failed ? (
        <Banner
          tone="error"
          role="alert"
          title="Couldn’t load this episode"
          action={
            <Button size="sm" onClick={retry}>
              <RefreshCw size={15} aria-hidden="true" /> Try again
            </Button>
          }
        >
          {(showQuery.error ?? seasonResult.error)?.message}
        </Banner>
      ) : loading ? (
        <LoadingRegion label="Loading the episode" className="flex flex-col gap-4">
          <Skeleton shape="line" width={120} />
          <SkeletonText lines={3} />
          <Skeleton height={44} />
        </LoadingRegion>
      ) : show && episode ? (
        <EpisodeBody show={show} episode={episode} close={close} />
      ) : (
        <p className="text-fg-2">This episode isn’t in your library. Add the show, then its episodes show up here.</p>
      )}
    </Sheet>
  );
}
