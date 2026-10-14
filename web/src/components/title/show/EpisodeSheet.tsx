'use client';

import { useQuery } from '@tanstack/react-query';
import { RefreshCw, Search, Trash2 } from 'lucide-react';
import { Banner, Button, CopyField, FilePath, LoadingRegion, Sheet, Skeleton, SkeletonText, StatusPill } from '@/components/ui';
import { useItemSubtitles } from '@/hooks/useItemSubtitles';
import { openLegacySearch } from '@/hooks/useLegacySearch';
import { useRemoveEpisodeFile } from '@/hooks/useTitle';
import type { EpisodeSummary, ShowTitle } from '@/lib/api/contract';
import { seasonQuery, titleQuery } from '@/lib/api/endpoints';
import type { SheetComponentProps } from '@/lib/sheets/registry';
import { episodeCode, parseEpisodeCode } from '@/lib/title/episodes';
import { formatDate } from '@/lib/title/format';
import { legacySearchContext } from '@/lib/title/legacy-search';
import { SubtitlesPanel } from '../SubtitlesPanel';

/** `?sheet=episode` params for one episode of a show. */
export const episodeSheetParams = (show: Pick<ShowTitle, 'tmdb_id'>, episode: Pick<EpisodeSummary, 'season_number' | 'episode_number'>) => ({
  tmdb: String(show.tmdb_id),
  se: episodeCode(episode.season_number, episode.episode_number),
});

const hasFile = (episode: EpisodeSummary) => episode.webdav_path !== null;

/** "/TV Shows/Frieren (2023)/Season 01/…mkv" → "TV Shows › Frieren (2023) › Season 01". */
const folderTrail = (path: string) => path.split('/').filter(Boolean).slice(0, -1).join(' › ');

function EpisodeBody({ show, episode, close }: { show: ShowTitle; episode: EpisodeSummary; close: () => void }) {
  const code = episodeCode(episode.season_number, episode.episode_number);
  const subtitles = useItemSubtitles('episode', hasFile(episode) ? episode.id : null, episode.webdav_path);
  const { remove, removing } = useRemoveEpisodeFile();
  const find = () => {
    close();
    // The page shows the search once the sheet has gone (a sheet traps focus)
    openLegacySearch(legacySearchContext(show, { season: episode.season_number, episode: episode.episode_number }));
  };
  const facts = [episode.runtime ? `${episode.runtime} min` : null, episode.air_date ? formatDate(episode.air_date) : null].filter(Boolean);

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
          </section>
          <SubtitlesPanel {...subtitles} />
          <div className="flex flex-wrap gap-2">
            <Button onClick={find}>
              <RefreshCw size={16} aria-hidden="true" /> Replace torrent
            </Button>
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
        <Button variant="primary" onClick={find} className="self-start">
          <Search size={16} aria-hidden="true" /> Find a torrent for {code}
        </Button>
      )}
    </div>
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
