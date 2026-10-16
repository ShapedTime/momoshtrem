'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { ChevronRight, Plus, Search, Tv } from 'lucide-react';
import { infuseSetupParams } from '@/components/shell/InfuseSetupSheet';
import { Button, ButtonLink, EpisodeCard, StatusPill, useToast } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import { libraryKeys, titleKeys, titlesApi, traktKeys } from '@/lib/api/endpoints';
import type { EpisodeCardAction, EpisodeCardModel } from '@/lib/for-you/episode-card';
import { useSheet } from '@/lib/sheets/useSheet';

/**
 * Add the show from a "Not in library" card. Up next and New episodes read
 * the library, so once they refetch the card says Find instead.
 */
function useAddShow() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  return useMutation({
    mutationFn: ({ tmdbId }: { tmdbId: number; show: string }) => titlesApi.addShow(tmdbId),
    onSuccess: async (_, { show }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: traktKeys.me }),
        queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
        queryClient.invalidateQueries({ queryKey: titleKeys.all }),
      ]);
      toast({ tone: 'ready', title: 'Added to library', body: `${show} · next, find a torrent` });
    },
    onError: (error, { show }) => toast({ tone: 'error', title: `Couldn’t add ${show}`, body: error.message }),
  });
}

function CardAction({ action }: { action: EpisodeCardAction }) {
  const sheets = useSheet();
  const add = useAddShow();
  switch (action.intent) {
    case 'play':
      return (
        <Button variant="ghost" size="sm" aria-label={action.ariaLabel} onClick={() => sheets.open('infuse-setup', infuseSetupParams(action))}>
          <Tv size={16} aria-hidden="true" />
          {action.label}
        </Button>
      );
    case 'details':
      return (
        <ButtonLink href={action.href} variant="ghost" size="sm" aria-label={action.ariaLabel}>
          {action.label}
        </ButtonLink>
      );
    case 'review':
      return (
        <ButtonLink href={action.href} variant="secondary" size="sm" aria-label={action.ariaLabel}>
          {action.label}
          <ChevronRight size={16} aria-hidden="true" />
        </ButtonLink>
      );
    case 'find':
      return (
        <Button variant="secondary" size="sm" aria-label={action.ariaLabel} onClick={() => sheets.open('torrent', action.params)}>
          <Search size={16} aria-hidden="true" />
          {action.label}
        </Button>
      );
    case 'add':
      return (
        <Button
          variant="secondary"
          size="sm"
          aria-label={action.ariaLabel}
          loading={add.isPending}
          onClick={() => add.mutate({ tmdbId: action.tmdbId, show: action.show })}
        >
          <Plus size={16} aria-hidden="true" />
          {action.label}
        </Button>
      );
  }
}

/** An Up next or New episodes card: the shared EpisodeCard with the action its availability calls for. */
export function ForYouEpisodeCard({ card }: { card: EpisodeCardModel }) {
  const still = card.image ? buildImageUrl(card.image.path, card.image.kind, card.image.kind === 'still' ? 'large' : 'medium') : null;
  return (
    <EpisodeCard
      href={card.href}
      show={card.show}
      name={card.name}
      code={card.code}
      stillSrc={still}
      when={card.when}
      downloadPct={card.downloadPct}
      status={<StatusPill status={card.pill.status} label={card.pill.label} pct={card.pill.pct} />}
      action={card.action && <CardAction action={card.action} />}
    />
  );
}
