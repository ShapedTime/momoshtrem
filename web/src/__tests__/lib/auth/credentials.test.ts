/** @jest-environment node */
import { getConfiguredUsername, verifyCredentials } from '@/lib/auth/credentials';

describe('verifyCredentials', () => {
  const env = process.env;
  beforeEach(() => {
    process.env = { ...env, APP_USERNAME: 'teymur', APP_PASSWORD: 'correct horse battery' };
  });
  afterAll(() => {
    process.env = env;
  });

  it('accepts the configured username and password', () => {
    expect(verifyCredentials('teymur', 'correct horse battery')).toBe(true);
  });

  it('ignores surrounding whitespace in the username only', () => {
    expect(verifyCredentials('  teymur ', 'correct horse battery')).toBe(true);
    expect(verifyCredentials('teymur', 'correct horse battery ')).toBe(false);
  });

  it.each([
    ['wrong username', 'admin', 'correct horse battery'],
    ['wrong password', 'teymur', 'nope'],
    ['both wrong', 'x', 'y'],
    ['empty password', 'teymur', ''],
    ['password prefix', 'teymur', 'correct horse'],
  ])('rejects %s', (_label, username, password) => {
    expect(verifyCredentials(username, password)).toBe(false);
  });

  it('defaults the username to admin', () => {
    delete process.env.APP_USERNAME;
    expect(getConfiguredUsername()).toBe('admin');
    expect(verifyCredentials('admin', 'correct horse battery')).toBe(true);
  });

  it('refuses every sign-in without APP_PASSWORD', () => {
    delete process.env.APP_PASSWORD;
    const spy = jest.spyOn(console, 'error').mockImplementation(() => {});
    expect(verifyCredentials('teymur', '')).toBe(false);
    spy.mockRestore();
  });
});
