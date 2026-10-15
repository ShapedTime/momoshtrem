import { rememberedVoiceOver, rememberVoiceOver } from '@/lib/hdrezka/remembered';

const KEY = 'momoshtrem:hdrezka-voice-over';

afterEach(() => {
  window.localStorage.clear();
  jest.restoreAllMocks();
});

describe('the remembered voice-over', () => {
  it('keeps the last one a stream was assigned with, cut and all', () => {
    expect(rememberedVoiceOver()).toBeNull();
    rememberVoiceOver({ id: 238 });
    expect(rememberedVoiceOver()).toEqual({ id: 238, camrip: false, ads: false, director: false });
    rememberVoiceOver({ id: 111, director: true });
    expect(rememberedVoiceOver()).toEqual({ id: 111, camrip: false, ads: false, director: true });
  });

  it('ignores what it can’t read', () => {
    window.localStorage.setItem(KEY, '{nope');
    expect(rememberedVoiceOver()).toBeNull();
    window.localStorage.setItem(KEY, JSON.stringify({ id: 'x' }));
    expect(rememberedVoiceOver()).toBeNull();
  });

  it('carries on without storage (private mode, blocked site data)', () => {
    jest.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('denied', 'SecurityError');
    });
    jest.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('full', 'QuotaExceededError');
    });
    expect(() => rememberVoiceOver({ id: 238 })).not.toThrow();
    expect(rememberedVoiceOver()).toBeNull();
  });
});
