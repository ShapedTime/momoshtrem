import { askHref, askThreadHref } from '@/lib/recommendations/ask-href';

describe('askHref', () => {
  it('is /ask without a question', () => {
    expect(askHref()).toBe('/ask');
    expect(askHref(null)).toBe('/ask');
    expect(askHref('   ')).toBe('/ask');
  });

  it('starts a conversation with the question, encoded and tidied', () => {
    expect(askHref('a 90-minute thriller for tonight')).toBe('/ask?q=a%2090-minute%20thriller%20for%20tonight');
    expect(askHref('  anime   with a\nFrieren vibe ')).toBe(`/ask?q=${encodeURIComponent('anime with a Frieren vibe')}`);
    expect(askHref('Tom & Jerry?')).toBe('/ask?q=Tom%20%26%20Jerry%3F');
  });

  it('keeps at most 500 characters, as Ask takes', () => {
    const q = decodeURIComponent(askHref('x'.repeat(600)).slice('/ask?q='.length));
    expect(q).toHaveLength(500);
  });
});

describe('askThreadHref', () => {
  it('opens a past conversation', () => {
    expect(askThreadHref(12)).toBe('/ask/12');
  });
});
