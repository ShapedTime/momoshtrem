'use client';

import { useRouter } from 'next/navigation';
import { useMutation } from '@tanstack/react-query';
import { ListChecks, Magnet, Pause, Play, Trash2 } from 'lucide-react';
import { ActionMenu, Button, copyText, useToast } from '@/components/ui';
import type { TorrentDetail, TorrentTitle } from '@/lib/api/contract';
import { assignmentApi } from '@/lib/api/endpoints';
import { waitsOnItsOwn } from '@/lib/activity/downloads';
import { isActive } from '@/lib/activity/sections';

interface DetailActionsProps {
  detail: TorrentDetail;
  onPause: () => void;
  onResume: () => void;
  onRemove: () => void;
  busy: boolean;
}

/**
 * The detail page's actions (activity.js `td-actions`): Pause (also while its
 * downloads wait, #77) or Resume,
 * Re-run mapping for shows, Copy magnet, and a quiet Remove. On phone a
 * 2-column grid with Remove on its own row; one column in the desktop aside.
 */
export function DetailActions({ detail, onPause, onResume, onRemove, busy }: DetailActionsProps) {
  const router = useRouter();
  const { toast } = useToast();
  const shows = detail.titles.filter((title) => title.media_type === 'tv');

  // A new mapping job for the torrent the library already knows; 200 is the job already open for it
  const rerun = useMutation({
    mutationFn: (show: TorrentTitle) => assignmentApi.assignShow(show.library_id, { info_hash: detail.info_hash }),
    onSuccess: ({ job_id }) => router.push(`/mapping/${job_id}`),
    onError: (error, show) => toast({ tone: 'error', title: `Couldn’t re-run the mapping for ${show.title}`, body: error.message }),
  });

  const copyMagnet = async () => {
    if (!detail.magnet_uri) return;
    const ok = await copyText(detail.magnet_uri);
    toast(ok ? { tone: 'ready', icon: Magnet, title: 'Magnet link copied' } : { tone: 'error', title: 'Couldn’t copy', body: 'Select the text and copy it by hand.' });
  };

  const rerunLabel = (
    <>
      <ListChecks size={17} aria-hidden="true" /> Re-run mapping
    </>
  );

  return (
    <section className="panel panel-pad td-actions" aria-label="Actions">
      {detail.state === 'paused' ? (
        <Button variant="primary" onClick={onResume} loading={busy}>
          <Play size={17} aria-hidden="true" /> Resume
        </Button>
      ) : isActive(detail) || waitsOnItsOwn(detail) ? (
        <Button onClick={onPause} loading={busy}>
          <Pause size={17} aria-hidden="true" /> Pause
        </Button>
      ) : null}
      {shows.length === 1 && (
        <Button onClick={() => rerun.mutate(shows[0])} loading={rerun.isPending}>
          {rerunLabel}
        </Button>
      )}
      {shows.length > 1 && (
        <ActionMenu
          label="Re-run mapping for"
          items={shows.map((show) => ({
            key: String(show.library_id),
            label: show.scope ? `${show.title} · ${show.scope}` : show.title,
            onSelect: () => rerun.mutate(show),
          }))}
        >
          <Button loading={rerun.isPending} aria-haspopup="menu">
            {rerunLabel}
          </Button>
        </ActionMenu>
      )}
      {detail.magnet_uri && (
        <Button onClick={copyMagnet}>
          <Magnet size={17} aria-hidden="true" /> Copy magnet
        </Button>
      )}
      <Button variant="danger-quiet" className="td-remove" onClick={onRemove}>
        <Trash2 size={17} aria-hidden="true" /> Remove
      </Button>
    </section>
  );
}
