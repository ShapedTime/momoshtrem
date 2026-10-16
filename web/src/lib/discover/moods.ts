import { clearFilters, type DiscoverFilters, type MoodId, normalizeFilters } from './filters';

/**
 * Moods are filter presets (decided on #7): each fills in a fixed set of
 * filters, which stay editable. The type and the sort are yours, so a mood
 * works for movies, shows or both. A mood shows as selected only while the
 * filters equal its preset; `mood` in the URL is just for a readable link.
 * They stay presets (#9 dropped AI picks here); the empty state offers Ask momoshtrem.
 */

type PresetKey = 'genres' | 'from' | 'to' | 'rating' | 'runtime' | 'lang' | 'country' | 'anime' | 'status';
export type MoodPreset = Partial<Pick<DiscoverFilters, PresetKey>>;

export interface Mood {
  id: MoodId;
  label: string;
  preset: MoodPreset;
}

export const MOODS: Mood[] = [
  // Gentle and warm: family comedies people love
  { id: 'cosy', label: 'Cosy', preset: { genres: ['comedy', 'family'], rating: 7 } },
  // Puzzles: science fiction that's also a mystery
  { id: 'mind', label: 'Mind-bending', preset: { genres: ['mystery', 'sci-fi'], rating: 7 } },
  // Quiet, well-made dramas of an ordinary length (TV: 31–50 min episodes)
  { id: 'slow', label: 'Slow cinema', preset: { genres: ['drama'], rating: 7, runtime: 'standard' } },
  { id: 'laugh', label: 'Laugh-out-loud', preset: { genres: ['comedy'], rating: 7 } },
  // Under 100 minutes, or 30-minute episodes
  { id: 'short', label: 'Short & sharp', preset: { runtime: 'short', rating: 7 } },
  // Long adventures: over 2 h 20 m, or hour-long episodes
  { id: 'epic', label: 'Epic nights', preset: { genres: ['adventure'], runtime: 'long', rating: 7 } },
];

const PRESET_KEYS: PresetKey[] = ['genres', 'from', 'to', 'rating', 'runtime', 'lang', 'country', 'anime', 'status'];

export const moodById = (id: MoodId): Mood | undefined => MOODS.find((mood) => mood.id === id);

/** The mood's filters on top of the current type and sort. */
export function applyMood(filters: DiscoverFilters, id: MoodId): DiscoverFilters {
  const mood = moodById(id);
  if (!mood) return filters;
  return normalizeFilters({ ...clearFilters(filters), ...mood.preset, mood: id });
}

/** Whether these filters are exactly the mood's preset (ignoring type, sort and `mood`). */
export function moodMatches(id: MoodId, filters: DiscoverFilters): boolean {
  const mood = moodById(id);
  if (!mood) return false;
  const preset = normalizeFilters({ ...clearFilters(filters), ...mood.preset });
  const current = normalizeFilters(filters);
  return PRESET_KEYS.every((key) =>
    key === 'genres' ? preset.genres.join(',') === current.genres.join(',') : preset[key] === current[key]
  );
}

/** The mood these filters equal, if any. */
export const selectedMood = (filters: DiscoverFilters): Mood | undefined => MOODS.find((mood) => moodMatches(mood.id, filters));
