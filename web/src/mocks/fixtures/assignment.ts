import type { AssignmentJobDetail, Proposal } from '@/lib/api/contract';
import { iso } from './catalog';

const dir = 'Frieren.S02.1080p.WEB-DL.x264-SubsPlease/';

function proposal(id: number, file: string, partial: Partial<Proposal>): Proposal {
  return {
    id,
    file_path: `${dir}${file}`,
    file_size: 1_400_000_000,
    kind: 'episode',
    target: null,
    confidence: 'high',
    source: 'rules',
    reason: 'S02E01 → TMDB S01E29 (season split)',
    decision: 'pending',
    ...partial,
  };
}

export function assignmentJob(jobId: number): AssignmentJobDetail {
  const matched = Array.from({ length: 8 }, (_, i) =>
    proposal(i + 1, `Frieren.S02E0${i + 1}.1080p.mkv`, {
      target: { episode_id: 1029 + i, season: 1, episode: 29 + i, name: `Episode ${29 + i}` },
      reason: `S02E0${i + 1} → S01E${29 + i} (scene season split)`,
    })
  );
  return {
    job: {
      id: jobId,
      item_type: 'show',
      item_id: 1,
      info_hash: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
      torrent_name: 'Frieren.S02.1080p.WEB-DL.x264-SubsPlease',
      status: 'needs_review',
      dry_run: false,
      error: null,
      created_at: iso(0),
      updated_at: iso(0),
    },
    groups: {
      matched,
      needs_review: [
        proposal(20, 'Frieren.S02E09.1080p.mkv', {
          target: { episode_id: 1037, season: 1, episode: 37, name: 'Episode 37' },
          confidence: 'low',
          source: 'ai',
          reason: 'AI: numbering continues the split; two files claim this episode',
        }),
      ],
      specials: [
        proposal(30, 'Extras/Frieren.NCOP.mkv', { kind: 'extra', target: null, confidence: null, reason: 'Creditless opening' }),
      ],
      extras: [],
      unmatched: [proposal(40, 'Frieren.S02E10.1080p.mkv', { target: null, confidence: null, reason: 'No episode 38 in TMDB yet' })],
    },
    summary: { files: 11, matched: 8, needs_review: 1, ignored: 1 },
  };
}
