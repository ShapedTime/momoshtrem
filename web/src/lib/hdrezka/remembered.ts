import type { TranslatorRef } from './pick';

/**
 * The voice-over the last HDRezka stream was assigned with (id and cut), so
 * the next title starts on it when it has one: the person mostly watches
 * "Оригинал (+субтитры)". Per browser; storage can be missing or refuse
 * (private mode, blocked site data), and then nothing is remembered.
 */

const KEY = 'momoshtrem:hdrezka-voice-over';

function isRef(value: unknown): value is TranslatorRef {
  if (typeof value !== 'object' || value === null) return false;
  const ref = value as Record<string, unknown>;
  const flag = (name: string) => ref[name] === undefined || typeof ref[name] === 'boolean';
  return typeof ref.id === 'number' && flag('camrip') && flag('ads') && flag('director');
}

export function rememberedVoiceOver(): TranslatorRef | null {
  try {
    const raw = window.localStorage.getItem(KEY);
    if (!raw) return null;
    const value: unknown = JSON.parse(raw);
    return isRef(value) ? { id: value.id, camrip: Boolean(value.camrip), ads: Boolean(value.ads), director: Boolean(value.director) } : null;
  } catch {
    return null;
  }
}

/** After a stream is assigned with it. */
export function rememberVoiceOver(translator: TranslatorRef): void {
  try {
    const { id, camrip = false, ads = false, director = false } = translator;
    window.localStorage.setItem(KEY, JSON.stringify({ id, camrip, ads, director }));
  } catch {
    // Storage is off: the next title starts on its own default
  }
}
