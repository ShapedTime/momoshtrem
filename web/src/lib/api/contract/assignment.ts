import type { Timestamp } from './common';

/** Assignment jobs and mapping review (B2/B3; #5). */

export type JobStatus =
  | 'queued'
  | 'fetching_metadata'
  | 'classifying'
  | 'needs_review'
  | 'ready'
  | 'applied'
  | 'failed'
  | 'cancelled';

export type ProposalKind = 'episode' | 'special' | 'movie' | 'extra' | 'sample' | 'subtitle' | 'unknown';
export type MatchConfidence = 'high' | 'medium' | 'low';
export type MatchSource = 'rules' | 'ai' | 'manual';
export type ProposalDecision = 'pending' | 'accepted' | 'rejected' | 'edited';

export interface ProposalTarget {
  episode_id: number;
  season: number;
  episode: number;
  name: string;
}

export interface Proposal {
  id: number;
  file_path: string;
  file_size: number;
  kind: ProposalKind;
  target: ProposalTarget | null;
  confidence: MatchConfidence | null;
  source: MatchSource;
  /** ≤ 120 characters; AI reasons are prefixed "AI:" in the UI. */
  reason: string;
  decision: ProposalDecision;
}

export interface AssignmentJob {
  id: number;
  item_type: 'movie' | 'show';
  item_id: number;
  info_hash: string;
  torrent_name: string;
  status: JobStatus;
  dry_run: boolean;
  error: string | null;
  created_at: Timestamp;
  updated_at: Timestamp;
}

/** GET /assignment-jobs/{id} */
export interface AssignmentJobDetail {
  job: AssignmentJob;
  groups: {
    matched: Proposal[];
    needs_review: Proposal[];
    specials: Proposal[];
    extras: Proposal[];
    unmatched: Proposal[];
  };
  summary: { files: number; matched: number; needs_review: number; ignored: number };
}

/** POST /shows/{id}/assign-torrent (202 → JobAccepted) */
export interface AssignShowTorrentRequest {
  magnet_uri: string;
  hint?: { season?: number; episode?: number };
}

/** POST /movies/{id}/assign-torrent (202 → JobAccepted) */
export interface AssignMovieTorrentRequest {
  magnet_uri: string;
  file_path?: string;
}

/** PATCH /assignment-jobs/{id}/proposals/{pid} */
export type ProposalPatch = { episode_id: number } | { kind: 'ignore' };

/** POST /assignment-jobs/{id}/reclassify */
export interface ReclassifyRequest {
  scope: 'unresolved' | 'all';
  hint?: string;
}

/** PUT /episodes/{id}/assignment and PUT /movies/{id}/assignment */
export interface ManualAssignment {
  info_hash: string;
  file_path: string;
}

/** GET /torrents/{hash}/files */
export interface TorrentFileEntry {
  path: string;
  size: number;
  mapped_to: ProposalTarget | { movie_id: number } | null;
}
