'use client';

import Link from 'next/link';
import { Activity, ListChecks, Search, Tv } from 'lucide-react';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { Button, ButtonLink, Media, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { LibraryItem } from '@/lib/api/contract';
import { getSheet } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';
import { type LibraryRowAction, libraryRowAction, libraryRowPill, libraryRowSub } from '@/lib/title-search/library-row';
import type { LibraryHit } from '@/lib/title-search/results';
import { titleHref } from '@/lib/title/href';
import { Highlight } from './Highlight';

/**
 * A library title in search results (search.js `libRow`): poster, the title
 * with the match marked, "2024 · Movie · 2160p · 18.4 GB", the status pill,
 * and the one action its state calls for.
 */
export function LibraryResultRow({ hit, query }: { hit: LibraryHit; query: string }) {
  const { item, reason } = hit;
  const href = titleHref(item.media_type, item.tmdb_id);
  const pill = libraryRowPill(item);
  const action = libraryRowAction(item, { torrentSheet: getSheet('torrent') !== undefined });
  const sub = [item.year, item.media_type === 'tv' ? 'Show' : 'Movie', reason ?? libraryRowSub(item)].filter(Boolean).join(' · ');

  return (
    <article className="srch-row">
      <Link href={href} aria-hidden="true" tabIndex={-1} className="srch-thumb">
        <Media shape="poster" src={buildImageUrl(item.poster_path, 'poster', 'small')} sizes="52px" />
      </Link>
      <div className="info">
        <Link href={href} className="srch-title">
          <Highlight text={item.title} query={query} />
        </Link>
        <span className="sub">{sub}</span>
        <StatusPill status={pill.status} label={pill.label} pct={pill.pct} />
      </div>
      <div className="end">
        <RowAction item={item} action={action} />
      </div>
    </article>
  );
}

function RowAction({ item, action }: { item: LibraryItem; action: LibraryRowAction }) {
  const sheets = useSheet();
  switch (action.kind) {
    case 'play':
      return (
        <Button
          size="sm"
          aria-label={`Play ${item.title}: how to play it in Infuse`}
          onClick={() => sheets.open('infuse-setup', infuseSetupParams({ path: item.webdav_path, url: item.webdav_url }))}
        >
          <Tv size={15} aria-hidden="true" />
          {action.label}
        </Button>
      );
    case 'find': {
      const label = `${action.label}: ${item.title}`;
      const { target } = action;
      return target.kind === 'sheet' ? (
        <Button size="sm" aria-label={label} onClick={() => sheets.open('torrent', target.params)}>
          <Search size={15} aria-hidden="true" />
          {action.label}
        </Button>
      ) : (
        <ButtonLink size="sm" href={target.href} aria-label={label}>
          <Search size={15} aria-hidden="true" />
          {action.label}
        </ButtonLink>
      );
    }
    case 'activity':
      return (
        <ButtonLink variant="ghost" size="sm" href={action.href} aria-label={`${action.label}: ${item.title} is downloading`}>
          <Activity size={15} aria-hidden="true" />
          {action.label}
        </ButtonLink>
      );
    case 'review':
      return (
        <ButtonLink size="sm" href={action.href} aria-label={`${action.label} ${item.title} mapping`}>
          <ListChecks size={15} aria-hidden="true" />
          {action.label}
        </ButtonLink>
      );
  }
}
